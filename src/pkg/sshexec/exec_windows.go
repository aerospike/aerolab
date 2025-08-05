package sshexec

import (
	"os"
	"time"

	"golang.org/x/term"
)

func winResize() {
	fd := int(os.Stdout.Fd())

	lastW, lastH, _ := term.GetSize(fd)

	go func() {
		for {
			time.Sleep(time.Second)

			w, h, err := term.GetSize(fd)
			if err != nil {
				continue
			}

			if w != lastW || h != lastH {
				lastW, lastH = w, h
				resize(nil)
			}
		}
	}()
}
