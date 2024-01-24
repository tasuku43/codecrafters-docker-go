//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]

	isolatedEnv, err := createIsolatedEnv(command, args)
	if err != nil {
		fmt.Println("error creating isolated env: ", err)
		os.Exit(1)
	}
	defer isolatedEnv.deferFunc()
	cmd := isolatedEnv.cmd

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			os.Exit(exitError.ExitCode())
		} else {
			os.Exit(1)
		}
	}
	os.Exit(0)
}

type IsolatedEnv struct {
	cmd     *exec.Cmd
	cleanup []func()
}

func (env *IsolatedEnv) deferFunc() {
	for _, cleanup := range env.cleanup {
		cleanup()
	}
}

func createIsolatedEnv(binPath string, args []string) (*IsolatedEnv, error) {
	var cleanup []func()
	tempDir, err := os.MkdirTemp("", "isolated-root")
	if err != nil {
		return nil, err
	}

	cleanup = append(cleanup, func() {
		if err := os.RemoveAll(tempDir); err != nil {
			fmt.Println("error removing temp dir: ", err)
		}
	})

	binDir := filepath.Dir(binPath)
	execPath := filepath.Join(tempDir, binDir)
	if err := os.MkdirAll(execPath, 0755); err != nil {
		return nil, err
	}

	binName := filepath.Base(binPath)
	isolatedBinPath := filepath.Join(execPath, binName)
	if err := os.Link(binPath, isolatedBinPath); err != nil {
		return nil, fmt.Errorf("error linking %s: %w", binName, err)
	}

	if err := syscall.Chroot(tempDir); err != nil {
		return nil, err
	}

	if err := os.Chdir("/"); err != nil {
		return nil, err
	}

	if err := os.Mkdir("/dev", 0755); err != nil {
		return nil, err
	}

	devNull, err := os.Create("/dev/null")
	if err != nil {
		return nil, err
	}
	cleanup = append(cleanup, func() {
		if err := devNull.Close(); err != nil {
			fmt.Println("error closing dev null: ", err)
		}
	})

	cmd := exec.Command(binPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID,
	}

	return &IsolatedEnv{cmd, cleanup}, nil
}
