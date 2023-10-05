package main

import (
	"os/exec"
	"strings"
)

func getPaginationCommand() (string, []string) {
	less, err := exec.Command("/bin/bash", "-c", "which less").CombinedOutput()
	if err == nil {
		return strings.Trim(string(less), "\r\n\t "), []string{"-S", "-r"}
	}
	less, err = exec.Command("/bin/bash", "-c", "which more").CombinedOutput()
	if err == nil {
		return strings.Trim(string(less), "\r\n\t "), []string{"-r"}
	}
	return "", nil
}
