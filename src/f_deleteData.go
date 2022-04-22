package main

import (
	"os"
	"path/filepath"
)

func findExec() string {
	ex, err := os.Executable()
	if err != nil {
		return ""
	}

	exReal, err := filepath.EvalSymlinks(ex)
	if err != nil {
		return ""
	}
	return exReal
}

func (c *config) F_deleteData() (ret int64, err error) {
	if c.DeleteData.Version == "4" {
		return c.F_deleteData4()
	}
	return c.F_deleteData5()
}
