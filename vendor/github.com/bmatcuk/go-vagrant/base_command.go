package vagrant

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
)

// BaseCommand adds base functionality and fields for all commands constructed
// from the VagrantClient.
type BaseCommand struct {
	OutputParser

	// Context for the running command - nil means none
	Context context.Context

	// Additional arguments to pass to the command. go-vagrant attempts to define
	// each argument as a field on the struct, but future versions of vagrant may
	// add options that didn't exist at the time of authoring. You can use this
	// to pass options to vagrant that go-vagrant doesn't know about.
	AdditionalArgs []string

	// Env is merged with the current process's environment and passed to the
	// vagrant command. Each entry is a "key=value" pair with later keys taking
	// precedence in the case of duplicates (keys here will take precedence over
	// keys in the current environment, too).
	Env []string

	// The underlying process, once it has been started with Run() or Start()
	Process *os.Process

	// ProcessState contains information about the process after it has exited.
	// Available after Run() or Wait().
	ProcessState *os.ProcessState

	client  *VagrantClient
	cmd     *exec.Cmd
	readers *sync.WaitGroup
}

func newBaseCommand(client *VagrantClient) BaseCommand {
	return BaseCommand{client: client}
}

func (b *BaseCommand) init(handler outputHandler, subcommand string, args ...string) error {
	if b.cmd != nil {
		return fmt.Errorf("vagrant %v: already started", subcommand)
	}

	// setup the command
	arguments := b.client.buildArguments(subcommand)
	arguments = append(arguments, args...)
	if b.AdditionalArgs != nil && len(b.AdditionalArgs) > 0 {
		arguments = append(arguments, b.AdditionalArgs...)
	}

	if b.Context == nil {
		b.cmd = exec.Command(b.client.executable, arguments...)
	} else {
		b.cmd = exec.CommandContext(b.Context, b.client.executable, arguments...)
	}
	b.cmd.Dir = b.client.VagrantfileDir
	if b.Env != nil {
		b.cmd.Env = append(os.Environ(), b.Env...)
	}

	// setup the output parser
	stdout, err := b.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	b.cmd.Stderr = b.cmd.Stdout

	b.readers = &sync.WaitGroup{}
	b.readers.Add(1)
	go b.startParser(stdout, handler, b.readers.Done)

	return nil
}

// Run the command.
func (b *BaseCommand) Run() error {
	if err := b.Start(); err != nil {
		return err
	}
	return b.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (b *BaseCommand) Start() error {
	if b.Verbose {
		log.Printf("Running %v", b.cmd.Args)
	}

	if err := b.cmd.Start(); err != nil {
		return err
	}
	b.Process = b.cmd.Process
	return nil
}

// Wait is used to wait on a command that was started with Start().
func (b *BaseCommand) Wait() error {
	b.readers.Wait()
	err := b.cmd.Wait()
	b.ProcessState = b.cmd.ProcessState
	return err
}
