package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/jessevdk/go-flags"
)

var completionBash = `_aerolab() {
    # All arguments except the first one
	%s
    args=("${COMP_WORDS[@]:1:$COMP_CWORD}")

    # Only split on newlines
    local IFS=$'\n'

    # Call completion (note that the first element of COMP_WORDS is
    # the executable itself)
    COMPREPLY=($(GO_FLAGS_COMPLETION=1 ${COMP_WORDS[0]} "${args[@]}"))
    return 0
}

%s
complete -F _aerolab aerolab
complete -F _aerolab aerolab-macos
complete -F _aerolab aerolab-linux
`

type completionBashCmd struct {
	NoInstall  bool           `short:"n" long:"no-install" description:"Print the completion script to screen instead of installing it in .bashrc"`
	CustomPath flags.Filename `short:"c" long:"custom-path" description:"Install the script in a custom location"`
	Simple     bool           `short:"s" long:"simple" description:"Simple completion disables backend lookup of items such as ClusterName when double-tab is pressed"`
	Help       helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *completionBashCmd) Execute(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}

	extra := "export AEROLAB_COMPLETION_BACKEND=1"
	if c.Simple {
		extra = ""
	}
	completionBash = fmt.Sprintf(completionBash, "", extra)

	if c.NoInstall {
		fmt.Println(completionBash)
		return nil
	}

	h, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	if c.CustomPath == "" {
		c.CustomPath = flags.Filename(path.Join(h, ".aerolab.completion.bash"))
	}

	// script
	fd, err := os.OpenFile(string(c.CustomPath), os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
	if err != nil {
		return err
	}
	defer fd.Close()
	_, err = fd.Write([]byte(completionBash))
	if err != nil {
		return err
	}

	// bashrc
	bashrc := path.Join(h, ".bashrc")
	fdBash, err := os.OpenFile(bashrc, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0755)
	if err != nil {
		return err
	}
	defer fdBash.Close()

	bashString := "\n# aerolab bash completion\nsource " + string(c.CustomPath) + "\n"

	// read whole file and check if soursing is already there
	bash, err := io.ReadAll(fdBash)
	if err != nil {
		return err
	}
	if strings.Contains(string(bash), bashString) {
		fmt.Println("OK, completion file written\nTo initialize, reload your shell or run: source ~/.bashrc")
		return nil
	}

	_, err = fdBash.Seek(0, 0)
	if err != nil {
		return err
	}
	_, err = fdBash.Write([]byte(bashString))
	if err != nil {
		return err
	}
	fmt.Println("OK, completion file written\nTo initialize, reload your shell or run: source ~/.bashrc")
	return nil
}
