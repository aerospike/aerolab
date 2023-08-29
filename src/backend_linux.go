package main

import (
	"io"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func (ssh_client *SSH) RunAttachCmd(cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	ssh_client.session.Stdin = stdin
	ssh_client.session.Stdout = stdout
	ssh_client.session.Stderr = stderr
	fileDescriptor := int(os.Stdout.Fd())
	var err error
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // enable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	if term.IsTerminal(fileDescriptor) {
		originalState, err := term.MakeRaw(fileDescriptor)
		if err != nil {
			return err
		}
		defer func() { _ = term.Restore(fileDescriptor, originalState) }()

		termWidth, termHeight, err := term.GetSize(fileDescriptor)
		if err != nil {
			return err
		}

		err = ssh_client.session.RequestPty("xterm-256color", termHeight, termWidth, modes)
		if err != nil {
			return err
		}
	} else {
		err = ssh_client.session.RequestPty("vt100", 24, 80, modes)
		if err != nil {
			return err
		}
	}
	err = ssh_client.session.Run(cmd)
	return err
}
