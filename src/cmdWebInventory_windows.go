package main

import (
	"log"

	"github.com/gabemarshall/pty"
)

func sysResizeWindow(tty pty.Pty, resizeMessage windowSize) (errno int) {
	err := pty.Setsize(tty, &pty.Winsize{
		Rows: resizeMessage.Rows,
		Cols: resizeMessage.Cols,
		X:    resizeMessage.X,
		Y:    resizeMessage.Y,
	})
	if err != nil {
		log.Print(err)
		return -1
	}
	return 0
}
