package main

import (

	// Uncomment this block to pass the first stage!

	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const (
	EXE_FP = "/usr/local/bin/docker-explorer"
)

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	image := os.Args[2]
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]

	tmpDir, createTmpDirErr := ioutil.TempDir("", "docker")
	if createTmpDirErr != nil {
		panic(createTmpDirErr)
	}
	defer os.RemoveAll(tmpDir)

	docker := Registry{
		AUTHENTICATION_URL: "https://auth.docker.io/token",
		REGISTRY_URL:       "registry.hub.docker.com",

		chroot: tmpDir,
	}

	name, tag := extractImageInfo(image)
	handleErr(docker.Authenticate(name))

	handleErr(docker.PullImage(name, tag))
	handleErr(os.MkdirAll(tmpDir+"/usr/local/bin", os.ModePerm))
	handleErr(copyFile(EXE_FP, fmt.Sprintf("%s%s", tmpDir, EXE_FP)))

	handleErr(syscall.Chroot(tmpDir))
	handleErr(syscall.Chdir("/"))

	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID,
	}

	if err := cmd.Run(); err != nil {
		fmt.Println(err)
		if exitError, ok := err.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		} else {
			os.Exit(1)
		}
	}
}

func handleErr(err error) {
	if err != nil {
		panic(err)
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

func extractImageInfo(image string) (string, string) {
	split := strings.Split(image, ":")

	imageName := fmt.Sprintf("library/%s", split[0])
	tag := "latest"
	if len(split) > 1 {
		tag = split[1]
	}

	return imageName, tag
}
