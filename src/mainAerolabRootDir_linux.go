package main

import (
	"os"
	"path"
)

func (a *aerolab) aerolabRootDir() (dirPath string, err error) {
	var home string
	home, err = os.UserHomeDir()
	if err != nil {
		return
	}
	dirPath = path.Join(home, ".aerolab")
	return
}
