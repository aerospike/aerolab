package main

import (
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var restoreTerminalLock = new(sync.Mutex)
var restoreTerminalState *term.State

func init() {
	addShutdownHandler("restore-terminal", backendRestoreHandler)
}

func backendRestoreHandler(o os.Signal) {
	backendRestoreTerminal()
}

func backendRestoreTerminal() {
	restoreTerminalLock.Lock()
	defer restoreTerminalLock.Unlock()
	if restoreTerminalState != nil {
		err := term.Restore(int(os.Stdout.Fd()), restoreTerminalState)
		if err != nil {
			log.Printf("FAILED to restore terminal state, run 'reset': %s", err)
		}
		restoreTerminalState = nil
	}
}

func (ssh_client *SSH) RunAttachCmd(cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer, isInteractive bool) error {
	ssh_client.session.Stdin = stdin
	ssh_client.session.Stdout = stdout
	ssh_client.session.Stderr = stderr
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // enable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	var err error
	if stdout == os.Stdout {
		fileDescriptor := int(os.Stdout.Fd())
		if term.IsTerminal(fileDescriptor) {
			restoreTerminalLock.Lock()
			if restoreTerminalState == nil && isInteractive {
				originalState, err := term.MakeRaw(fileDescriptor)
				if err != nil {
					return err
				}
				restoreTerminalState = originalState
			}
			restoreTerminalLock.Unlock()

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
		go func() {
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGWINCH)
			defer signal.Stop(sigs)
			for range sigs {
				if _, err := ssh_client.session.SendRequest("window-change", false, termSize(os.Stdin.Fd())); err != nil {
					log.Print(err)
				}
			}
		}()
	}
	err = ssh_client.session.Run(cmd)
	return err
}
