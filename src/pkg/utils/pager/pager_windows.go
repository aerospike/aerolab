package pager

import (
	"os/exec"
	"syscall"
)

func setCmdParams(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
