package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rglonek/logger"
)

type ShowcommandsCmd struct {
	DestDir string  `short:"d" long:"destination" default:"/usr/local/bin/"`
	DryRun  bool    `short:"n" long:"dry-run" description:"Do not run the command, just print it"`
	Help    HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ShowcommandsCmd) Execute(args []string) error {
	system, err := Initialize(&Init{InitBackend: false}, []string{"showcommands"}, c, args...)
	if err != nil {
		return Error(err, system, []string{"showcommands"}, c, args)
	}
	system.Logger.Info("Running showcommands")
	err = c.InstallShowcommands(system.Logger)
	if err != nil {
		return Error(err, system, []string{"showcommands"}, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, []string{"showcommands"}, c, args)
}

func (c *ShowcommandsCmd) InstallShowcommands(log *logger.Logger) error {
	cur, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get absolute path os self: %s", err)
	}
	log.Info("Discovered absolute path: %s", cur)
	for _, dest := range []string{"showconf", "showsysinfo", "showinterrupts", "aerolab-ansible"} {
		d := filepath.Join(c.DestDir, dest)
		log.Info("> ln -s %s %s", cur, d)
		if c.DryRun {
			continue
		}
		if _, err := os.Stat(d); err == nil {
			os.Remove(d)
		}
		err = os.Symlink(cur, d)
		if err != nil {
			log.Error("ERROR symlinking %s->%s : %s", cur, d, err)
		}
	}
	return nil
}
