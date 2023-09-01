package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"syscall"
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

func chroot(commandPath string) string {
	rootPath, err := os.MkdirTemp("", "mydocker")
	if err != nil {
		log.Fatalf("Error making temp dir: %v", err)
	}

	if _, err := createDevNull(rootPath); err != nil {
		log.Fatalf("Error creating /dev/null: %v", err)
	}

	binDirPath := path.Join(rootPath, path.Dir(commandPath))
	if err := os.MkdirAll(binDirPath, 755); err != nil {
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
	if err := os.Mkdir(devDirPath, 755); err != nil {
		return "", err
	}

	nullFilePath := path.Join(devDirPath, "/null")
	nullFile, err := os.Create(nullFilePath)
	if err != nil {
		return "", err
	}
	defer nullFile.Close()

	if err := nullFile.Chmod(755); err != nil {
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

	if err := dstFile.Chmod(755); err != nil {
		return err
	}

	return nil
}
