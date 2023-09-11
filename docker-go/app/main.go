package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"syscall"
	"time"
)

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	image := os.Args[2]
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]

	newroot := isolate(image, command)
	defer func() {
		if err := os.RemoveAll(newroot); err != nil {
			log.Fatalf("Error removing path: %v", err)
		}
	}()

	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		exitErr := err.(*exec.ExitError)
		fmt.Printf("Error running cmd: %v", err)
		os.Exit(exitErr.ExitCode())
	}
}

func isolate(image, command string) string {
	rootPath, err := os.MkdirTemp("", "mydocker")
	if err != nil {
		log.Fatalf("Error making temp dir: %v", err)
	}

	if _, err := createDevNull(rootPath); err != nil {
		log.Fatalf("Error creating /dev/null: %v", err)
	}

	binDirPath := path.Join(rootPath, path.Dir(command))
	if err := os.MkdirAll(binDirPath, 0755); err != nil {
		log.Fatalf("Error making dir: %v", err)
	}

	binFilePath := path.Join(binDirPath, path.Base(command))
	if err := copyFile(command, binFilePath); err != nil {
		log.Fatalf("Error copying file: %v", err)
	}

	token, err := getRegistryToken(image)
	if err != nil {
		log.Fatalf("Error getting token: %v", err)
	}

	manifest, err := getImageManifest(image, token.Token)
	if err != nil {
		log.Fatalf("Error getting image manifest: %v", err)
	}

	layers, err := getImageLayers(image, token.Token, manifest)
	if err != nil {
		log.Fatalf("Error getting image layers: %v", err)
	}
	defer func() {
		for _, layer := range *layers {
			layer.Data.Close()
		}
	}()

	if err := extractLayers(rootPath, layers); err != nil {
		log.Fatalf("Error extracting image layers: %v", err)
	}

	if err := syscall.Chroot(rootPath); err != nil {
		log.Fatalf("Error chrooting: %v", err)
	}

	if err := syscall.Unshare(syscall.CLONE_NEWPID); err != nil {
		log.Fatalf("Error unsharing process: %v", err)
	}

	return rootPath
}

func createDevNull(basePath string) (string, error) {
	devDirPath := path.Join(basePath, "/dev")
	if err := os.Mkdir(devDirPath, 0755); err != nil {
		return "", err
	}

	nullFilePath := path.Join(devDirPath, "/null")
	nullFile, err := os.Create(nullFilePath)
	if err != nil {
		return "", err
	}
	defer nullFile.Close()

	if err := nullFile.Chmod(0755); err != nil {
		return "", err
	}

	return nullFile.Name(), nil
}

func copyFile(srcPath, dstPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	if err := dstFile.Chmod(0755); err != nil {
		return err
	}

	return nil
}

type registryToken struct {
	Token       string    `json:"token"`
	AccessToken string    `json:"access_token"`
	ExpiresIn   int       `json:"expires_in"`
	IssuedAt    time.Time `json:"issued_at"`
}

func getRegistryToken(image string) (registryToken, error) {
	ret := registryToken{}

	url := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/%s:pull,push", image)
	resp, err := http.Get(url)
	if err != nil {
		return ret, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ret, err
	}

	err = json.Unmarshal(body, &ret)
	if err != nil {
		return ret, err
	}

	return ret, nil
}

type imageManifest struct {
	SchemaVersion int
	MediaType     string
	Config        struct {
		MediaType string
		Size      int
		Digest    string
	}
	Layers []struct {
		MediaType string
		Size      int
		Digest    string
	}
}

func getImageManifest(image, token string) (imageManifest, error) {
	ret := imageManifest{}

	url := fmt.Sprintf("https://registry.hub.docker.com/v2/library/%s/manifests/latest", image)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ret, err
	}
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Add("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return ret, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ret, err
	}

	err = json.Unmarshal(body, &ret)
	if err != nil {
		return ret, err
	}

	return ret, nil
}

type layerBlob struct {
	Digest string
	Data   io.ReadCloser
}

func getImageLayers(image, token string, manifest imageManifest) (*[]layerBlob, error) {
	ret := make([]layerBlob, len(manifest.Layers))

	for i, layer := range manifest.Layers {
		url := fmt.Sprintf("https://registry.hub.docker.com/v2/library/%s/blobs/%s", image, layer.Digest)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Add("Authorization", "Bearer "+token)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		ret[i] = layerBlob{
			Digest: layer.Digest,
			Data:   resp.Body,
		}
	}

	return &ret, nil
}

func extractLayers(dstPath string, layers *[]layerBlob) error {
	for _, layer := range *layers {
		// sha256:<hash>
		name := layer.Digest[7:] + ".tar.gz"
		f, err := os.Create(name)
		if err != nil {
			return err
		}
		defer f.Close()
		defer os.Remove(name)

		if _, err := io.Copy(f, layer.Data); err != nil {
			return err
		}

		cmd := exec.Command("tar", "-xf", name, "-C", dstPath)
		err = cmd.Run()
		if err != nil {
			return err
		}
	}

	return nil
}
