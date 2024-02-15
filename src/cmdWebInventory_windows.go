package main

import (
	"log"

	"github.com/gabemarshall/pty"
	"golang.org/x/sys/windows/registry"
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

func getWindowsBuild() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	cb, _, err := k.GetStringValue("CurrentBuild")
	if err != nil {
		return "", err
	}
	return cb, nil
}
