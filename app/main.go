package main

import (

	// Uncomment this block to pass the first stage!

	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
)

const (
	EXE_FP = "/usr/local/bin/docker-explorer"
)

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]

	tmpDir, createTmpDirErr := ioutil.TempDir("", "docker")
	if createTmpDirErr != nil {
		panic(createTmpDirErr)
	}
	defer os.RemoveAll(tmpDir)

	if mkdirErr := os.MkdirAll(tmpDir+"/usr/local/bin", os.ModePerm); mkdirErr != nil {
		panic(mkdirErr)
	}

	if copyError := copyFile(EXE_FP, fmt.Sprintf("%s%s", tmpDir, EXE_FP)); copyError != nil {
		panic(copyError)
	}

	syscall.Chroot(tmpDir)
	syscall.Chdir("/")

	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		} else {
			os.Exit(1)
		}
	}
}

func copyFile(src string, dest string) error {
	input, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(dest, input, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}
