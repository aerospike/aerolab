package main

import (
	"os"
	"os/exec"
	"strings"
)

func getPagerCommand() (string, []string) {
	less, err := exec.Command("cmd", "/c", "WHERE less").CombinedOutput()
	if err == nil {
		return strings.Trim(string(less), "\r\n\t "), []string{"-S", "-R"}
	}
	gnuWin := "C:\\Program Files (x86)\\GnuWin32\\bin\\less.exe"
	if _, err := os.Stat(gnuWin); err == nil {
		return strings.Trim(string(gnuWin), "\r\n\t "), []string{"-S", "-R"}
	}
	less, err = exec.Command("cmd", "/c", "WHERE more").CombinedOutput()
	if err == nil {
		return strings.Trim(string(less), "\r\n\t "), []string{}
	}
	return "", nil
}
