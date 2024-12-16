package main

import (
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/containerd/console"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var restoreTerminalLock = new(sync.Mutex)
var restoreTerminalState *console.Console

func sttyReset() {
	return
	/*
		cmd := exec.Command("reset")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	*/
}

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
		st := *restoreTerminalState
		st.Reset()
		restoreTerminalState = nil
	}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		sttyReset()
	}
}

func (ssh_client *SSH) RunAttachCmd(cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer, isInteractive bool) error {
	ssh_client.session.Stdin = stdin
	ssh_client.session.Stdout = stdout
	ssh_client.session.Stderr = stderr
	var err error
	termWidth := 80
	termHeight := 24
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // enable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	if stdout == os.Stdout {
		fileDescriptor := int(os.Stdout.Fd())
		if term.IsTerminal(fileDescriptor) {
			termWidth, termHeight, err = term.GetSize(fileDescriptor)
			if err != nil {
				return err
			}

			err = ssh_client.session.RequestPty("vt100", termHeight, termWidth, modes)
			if err != nil {
				return err
			}
		} else {
			err = ssh_client.session.RequestPty("vt100", 24, 80, modes)
			if err != nil {
				return err
			}
		}
		restoreTerminalLock.Lock()
		if restoreTerminalState == nil && isInteractive {
			current := console.Current()
			defer current.Reset()

			if err := current.SetRaw(); err != nil {
			}
			ws, err := current.Size()
			if err != nil {
			}
			current.Resize(ws)
			restoreTerminalState = &current
			isExit := make(chan struct{}, 1)
			defer func() {
				isExit <- struct{}{}
			}()
			go func() {
				var oldSize console.WinSize
				for {
					time.Sleep(time.Second)
					winSize, err := current.Size()
					if err != nil {
						continue
					}
					if oldSize.Height == 0 && oldSize.Width == 0 {
						oldSize = winSize
						continue
					}
					if oldSize.Height == winSize.Height && oldSize.Width == winSize.Width {
						continue
					}
					if _, err := ssh_client.session.SendRequest("window-change", false, termSize(os.Stdout.Fd())); err != nil {
						log.Print(err)
					} else {
						oldSize = winSize
					}
				}
			}()
		}
		restoreTerminalLock.Unlock()
	}
	err = ssh_client.session.Run(cmd)
	return err
}
