package main

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/aerospike/aerolab/cli/cmd"
	"github.com/aerospike/aerolab/pkg/eks/eksexpiry"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
)

func main() {
	// windows: install self and add to path
	if installSelf() {
		return
	}

	exitCode := 0
	// handle busybox style special cases and main application point runner
	_, command := path.Split(os.Args[0])
	switch command {
	case "showsysinfo", "showconf", "showinterrupts":
		cmd.ShowcommandsBusybox()
	case "eksexpiry":
		eksexpiry.Expiry()
	case "aerolab-ansible":
		os.Args = []string{os.Args[0], "inventory", "ansible"}
		fallthrough
	default:
		args := os.Args[1:]
		err := run(args)
		if err != nil {
			if !errors.Is(err, cmd.ErrExecuteError) {
				fmt.Println(err)
			}
			exitCode = 1
		}
	}
	shutdown.WaitJobs()
	os.Exit(exitCode)
}

func run(args []string) error {
	if len(args) == 0 {
		args = []string{"help"}
	}
	// first create the home directory if it doesn't exist
	err := createHomeDir()
	if err != nil {
		return err
	}

	// first init call: used to run the correct Execute function only
	_, err = cmd.Initialize(&cmd.Init{
		InitBackend:        false,
		RunExecuteFunction: true,
		UpgradeCheck:       false,
	}, nil, nil, args...)
	if err != nil {
		return err
	}

	return nil
}

func createHomeDir() error {
	ahome, err := cmd.AerolabRootDir()
	if err != nil {
		return fmt.Errorf("could not determine user's home directory: %s", err)
	}
	if _, err := os.Stat(ahome); err != nil {
		err = os.MkdirAll(ahome, 0700)
		if err != nil {
			return fmt.Errorf("could not create %s, configuration files may not be available: %s", ahome, err)
		}
	}
	return nil
}
