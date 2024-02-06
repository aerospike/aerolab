package main

import (
	"log"
	"os"
)

type templateCmd struct {
	List   templateListCmd   `command:"list" subcommands-optional:"true" description:"List available templates" webicon:"fas fa-list"`
	Delete templateDeleteCmd `command:"destroy" subcommands-optional:"true" description:"Delete a template image" webicon:"fas fa-trash" invwebforce:"true"`
	Create templateCreateCmd `command:"create" subcommands-optional:"true" description:"Create a new template" webicon:"fas fa-circle-plus" invwebforce:"true"`
	Vacuum templateVacuumCmd `command:"vacuum" subcommands-optional:"true" description:"Remove danging unfinished templates left over on failure" webicon:"fas fa-broom"`
	Help   helpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *templateCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type templateVacuumCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *templateVacuumCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running template.vacuum")
	err := b.VacuumTemplates()
	if err != nil {
		return err
	}
	log.Println("Done")
	return nil
}
