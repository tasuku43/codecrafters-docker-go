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

	containerDir, err := os.MkdirTemp("", "containers-root")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(containerDir)

	files, err := os.ReadDir(imagesDir)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tar") {
			tarPath := filepath.Join(imagesDir, file.Name())

			cmd := exec.Command("tar", "-xvf", tarPath, "-C", containerDir)
			if err := cmd.Run(); err != nil {
				fmt.Printf("Error extracting %s: %s\n", tarPath, err)
			}
		}
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
	defer devNull.Close()

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
