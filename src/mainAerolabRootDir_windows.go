package main

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (a *aerolab) aerolabRootDir() (dirPath string, err error) {
	if customEnv, ok := os.LookupEnv("AEROLAB_HOME"); ok && customEnv != "" {
		return customEnv, nil
	}
	var home string
	home, err = os.UserHomeDir()
	if err != nil {
		return
	}
	dirPath = path.Join(home, ".aerolab")
	instPath := filepath.Join(home, "AppData", "Local", "Aerospike", "AeroLab", "bin")
	myPath, _ := filepath.Split(os.Args[0])
	myPath = strings.TrimSuffix(myPath, "\\")
	if myPath == instPath {
		dirPath = filepath.Join(home, "AppData", "Local", "Aerospike", "AeroLab")
	}
	return
}
