package cmd

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	flags "github.com/rglonek/go-flags"
)

type CompletionZshCmd struct {
	NoInstall  bool           `short:"n" long:"no-install" description:"Print the completion script to screen instead of installing it in .zshrc"`
	CustomPath flags.Filename `short:"c" long:"custom-path" description:"Install the script in a custom location"`
	Simple     bool           `short:"s" long:"simple" description:"Simple completion disables backend lookup of items such as ClusterName when double-tab is pressed"`
	Help       HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CompletionZshCmd) Execute(args []string) error {
	system, err := Initialize(&Init{InitBackend: false}, []string{"completion", "zsh"}, c, args...)
	if err != nil {
		return Error(err, system, []string{"completion", "zsh"}, c, args)
	}

	extra := "export AEROLAB_COMPLETION_BACKEND=1"
	if c.Simple {
		extra = ""
	}
	completionBash = fmt.Sprintf(completionBash, "[ ${#COMP_WORDS[@]} -eq ${COMP_CWORD} ] && COMP_WORDS+=(\"\")", extra)

	if c.NoInstall {
		fmt.Println("--- SCRIPT START ---")
		fmt.Println(completionBash)
		fmt.Println("\n--- RC FILE .zshrc CONTENTS ---")
		fmt.Println("autoload -Uz compinit bashcompinit && compinit && bashcompinit\nsource ${HOME}/.aerolab.completion.zsh")
		fmt.Println("\n--- END ---")
		return nil
	}

	h, err := os.UserHomeDir()
	if err != nil {
		return Error(err, system, []string{"completion", "zsh"}, c, args)
	}

	if c.CustomPath == "" {
		c.CustomPath = flags.Filename(path.Join(h, ".aerolab.completion.zsh"))
	}

	// script
	fd, err := os.OpenFile(string(c.CustomPath), os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
	if err != nil {
		return Error(err, system, []string{"completion", "zsh"}, c, args)
	}
	defer fd.Close()
	_, err = fd.Write([]byte(completionBash))
	if err != nil {
		return Error(err, system, []string{"completion", "zsh"}, c, args)
	}

	// bashrc
	bashrc := path.Join(h, ".zshrc")
	fdBash, err := os.OpenFile(bashrc, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0755)
	if err != nil {
		return Error(err, system, []string{"completion", "zsh"}, c, args)
	}
	defer fdBash.Close()

	bashString := "\n# aerolab zsh completion\nsource " + string(c.CustomPath) + "\n"

	// read whole file and check if soursing is already there
	bash, err := io.ReadAll(fdBash)
	if err != nil {
		return Error(err, system, []string{"completion", "zsh"}, c, args)
	}

	if strings.Contains(string(bash), bashString) && strings.Contains(string(bash), "autoload -Uz compinit bashcompinit && compinit && bashcompinit") {
		fmt.Println("OK, completion file written\nTo initialize, reload your shell or run: source ~/.zshrc")
		return nil
	}

	_, err = fdBash.Seek(0, 0)
	if err != nil {
		return Error(err, system, []string{"completion", "zsh"}, c, args)
	}
	if !strings.Contains(string(bash), "autoload -Uz compinit bashcompinit && compinit && bashcompinit") {
		_, err = fdBash.Write([]byte("\nautoload -Uz compinit bashcompinit && compinit && bashcompinit\n"))
		if err != nil {
			return Error(err, system, []string{"completion", "zsh"}, c, args)
		}
	}
	if !strings.Contains(string(bash), bashString) {
		_, err = fdBash.Write([]byte(bashString))
		if err != nil {
			return Error(err, system, []string{"completion", "zsh"}, c, args)
		}
	}
	fmt.Println("OK, completion file written\nTo initialize, reload your shell or run: source ~/.zshrc")
	return Error(nil, system, []string{"completion", "zsh"}, c, args)
}
