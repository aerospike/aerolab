package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/bestmethod/inslice"
	ps "github.com/mitchellh/go-ps"
	"golang.org/x/sys/windows/registry"
)

func installSelf() (isGUI bool) {
	p, err := ps.FindProcess(os.Getppid())
	if err != nil {
		return false
	}
	pName := p.Executable()
	if pName == "pwsh.exe" || pName == "aerolab.exe" {
		return false
	}
	defer func() {
		fmt.Println("Press ENTER to exit")
		var input string
		fmt.Scanln(&input)
	}()
	f := newLogSplit("aerolab.installer.log")
	if f != nil {
		defer f.Close()
		log.SetOutput(f)
	}
	log.Println("Starting installer...")
	isGUI = true
	home, err := os.UserHomeDir()
	if err != nil {
		installSelfDrawErrorWindow(fmt.Errorf("E0:%s", err))
		return
	}
	aerolabHome := filepath.Join(home, "AppData", "Local", "Aerospike", "AeroLab")
	binDir := filepath.Join(aerolabHome, "bin")
	bin := filepath.Join(binDir, "aerolab.exe")
	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		log.Printf("Creating %s", binDir)
		err = os.MkdirAll(binDir, 0755)
		if err != nil {
			installSelfDrawErrorWindow(fmt.Errorf("E1:%s", err))
			return
		}
	}
	log.Printf("Copying %s to %s", os.Args[0], bin)
	if err := installSelfCopy(bin); err != nil {
		installSelfDrawErrorWindow(fmt.Errorf("E2:%s", err))
		return
	}
	log.Print("Adding aerolab to PATH in registry:HKEY_CURRENT_USER\\Environment\\Path")
	if err := installSelfRegistry(binDir); err != nil {
		installSelfDrawErrorWindow(fmt.Errorf("E3:%s", err))
		return
	}
	log.Println("AeroLab successfully installed")
	log.Println("To get started, open a new PowerShell window and run: aerolab")
	return
}

func installSelfRegistry(binDir string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("E1:%s", err)
	}
	defer k.Close()

	s, _, err := k.GetStringValue("Path")
	if err != nil && !strings.Contains(err.Error(), "The system cannot find the file specified") {
		return fmt.Errorf("E2:%s", err)
	}
	ss := filepath.SplitList(s)
	if inslice.HasString(ss, binDir) {
		return nil
	}
	ss = append(ss, binDir)
	s = strings.Join(ss, ";")
	err = k.SetStringValue("Path", s)
	if err != nil {
		return fmt.Errorf("E3:%s", err)
	}
	HWND_BROADCAST := uintptr(0xffff)
	WM_SETTINGCHANGE := uintptr(0x001A)
	syscall.NewLazyDLL("user32.dll").NewProc("SendMessageW").Call(HWND_BROADCAST, WM_SETTINGCHANGE, 0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("ENVIRONMENT"))))
	return nil
}

func installSelfCopy(bin string) error {
	r, err := os.Open(os.Args[0])
	if err != nil {
		return fmt.Errorf("E1:%s", err)
	}
	defer r.Close()
	w, err := os.Create(bin)
	if err != nil {
		return fmt.Errorf("E2:%s", err)
	}
	defer w.Close()
	_, err = io.Copy(w, r)
	if err != nil {
		return fmt.Errorf("E3:%s", err)
	}
	return nil
}

func installSelfDrawErrorWindow(err error) error {
	log.Printf("ERROR: %s", err)
	return nil
}

type logSplit struct {
	file *os.File
	out  io.Writer
}

func (l *logSplit) Write(b []byte) (int, error) {
	l.file.Write(b)
	return l.out.Write(b)
}

func (l *logSplit) Close() {
	l.file.Close()
}

func newLogSplit(f string) *logSplit {
	fh, err := os.Create(f)
	if err != nil {
		return nil
	}
	return &logSplit{
		out:  os.Stderr,
		file: fh,
	}
}
