//go:build linux

package main

import (
	"errors"
	"fmt"
	"github.com/codecrafters-io/docker-starter-go/pkg/images"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Usage: your_docker.sh run <images> <command> <arg1> <arg2> ...
func main() {
	image := os.Args[2]
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]

	imageRetriever, _ := images.NewOCIImageRetriever(images.ParseImageString(image))
	imagesDir, err := imageRetriever.Pull()
	defer removeDirectory(imagesDir)

	containerDir, err := os.MkdirTemp("", "containers-root")
	if err != nil {
		log.Fatal(err)
	}
	defer removeDirectory(containerDir)

	err = extractTarFiles(imagesDir, containerDir)
	if err != nil {
		log.Fatal(err)
	}

	if err := syscall.Chroot(containerDir); err != nil {
		log.Fatal(err)
	}

	if err := os.Chdir("/"); err != nil {
		log.Fatal(err)
	}

	devNull, err := os.Create("/dev/null")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err := devNull.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	cmd := exec.Command(command, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID,
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			fmt.Println(err)
			os.Exit(exitError.ExitCode())
		} else {
			fmt.Println(err)
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func extractTarFiles(sourceDir string, targetDir string) error {
	files, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tar") {
			tarPath := filepath.Join(sourceDir, file.Name())
			cmd := exec.Command("tar", "-xvf", tarPath, "-C", targetDir)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("error extracting %s: %w", tarPath, err)
			}
		}
	}
	return nil
}

func removeDirectory(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		log.Printf("Failed to remove directory %s: %v", dir, err)
	}
}
