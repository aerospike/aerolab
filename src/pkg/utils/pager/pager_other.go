//go:build !windows

package pager

import (
	"os/exec"
)

func setCmdParams(cmd *exec.Cmd) {
}
