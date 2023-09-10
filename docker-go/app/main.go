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
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]

	newroot := chroot(command)
	defer func() {
		if err := os.RemoveAll(newroot); err != nil {
			log.Fatalf("Error removing path: %v", err)
		}
	}()

	if err := syscall.Unshare(syscall.CLONE_NEWPID); err != nil {
		log.Fatalf("Error unsharing: %v", err)
	}

	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		exitErr := err.(*exec.ExitError)
		fmt.Printf("Error: %v", err)
		os.Exit(exitErr.ExitCode())
	}
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

func chroot(commandPath string) string {
	rootPath, err := os.MkdirTemp("", "mydocker")
	if err != nil {
		log.Fatalf("Error making temp dir: %v", err)
	}

	if _, err := createDevNull(rootPath); err != nil {
		log.Fatalf("Error creating /dev/null: %v", err)
	}

	binDirPath := path.Join(rootPath, path.Dir(commandPath))
	if err := os.MkdirAll(binDirPath, 0755); err != nil {
		log.Fatalf("Error making dir: %v", err)
	}

	binFilePath := path.Join(binDirPath, path.Base(commandPath))
	if err := copyFile(commandPath, binFilePath); err != nil {
		log.Fatalf("Error copying file: %v", err)
	}

	if err := syscall.Chroot(rootPath); err != nil {
		log.Fatalf("Error chrooting: %v", err)
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
