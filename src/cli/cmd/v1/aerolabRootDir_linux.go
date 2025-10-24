package cmd

import (
	"os"
	"path"
)

func AerolabRootDir() (dirPath string, err error) {
	if customEnv, ok := os.LookupEnv("AEROLAB_HOME"); ok && customEnv != "" {
		return customEnv, nil
	}
	var home string
	home, err = os.UserHomeDir()
	if err != nil {
		return
	}
	dirPath = path.Join(home, ".config", "aerolab")
	return
}

func AerolabRootDirOld() (dirPath string, err error) {
	if customEnv, ok := os.LookupEnv("AEROLAB_HOME"); ok && customEnv != "" {
		return customEnv, nil
	}
	var home string
	home, err = os.UserHomeDir()
	if err != nil {
		return
	}
	dirPath = path.Join(home, ".aerolab")
	return
}
