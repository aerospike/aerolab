package pager

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type Pager struct {
	cmd       *exec.Cmd
	writer    io.WriteCloser
	hasColors bool
}

// New returns a new pager that shells out to `less -R -S`
func New(out io.Writer) (*Pager, error) {
	cmd, hasColors, err := pagerPath()
	if err != nil {
		return nil, err
	}

	cmd.Stdout = out
	cmd.Stderr = out

	pipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	return &Pager{
		cmd:       cmd,
		writer:    pipe,
		hasColors: hasColors,
	}, nil
}

func (p *Pager) HasColors() bool {
	if p == nil {
		return true
	}
	return p.hasColors
}

// Start launches the less process
func (p *Pager) Start() error {
	return p.cmd.Start()
}

// Write sends output to the pager
func (p *Pager) Write(data []byte) (int, error) {
	return p.writer.Write(data)
}

// Close closes the writer and waits for the process to finish
func (p *Pager) Close() error {
	_ = p.writer.Close()
	return p.cmd.Wait()
}

// returns: cmd, has colors, error
func pagerPath() (*exec.Cmd, bool, error) {
	var pagerPath string
	var err error

	// check for less
	if pagerPath, err = exec.LookPath("less"); err == nil {
		return exec.Command(pagerPath, "-R", "-S"), true, nil
	}

	// I hate windows
	if runtime.GOOS == "windows" {
		// but I still feel lucky
		if pagerPath, err = exec.LookPath("less.exe"); err == nil {
			cmd := exec.Command(pagerPath, "-R", "-S")
			setCmdParams(cmd)
			return cmd, true, nil
		}
		// lets see if less.exe is installed somewhere outside of path in known locations
		candidates := []string{
			`C:\ProgramData\chocolatey\bin\less.exe`,
			`C:\Program Files\Git\usr\bin\less.exe`,
			`C:\Program Files (x86)\Git\usr\bin\less.exe`,
			`C:\msys64\usr\bin\less.exe`,
			`C:\msys32\usr\bin\less.exe`,
			`C:\mingw64\bin\less.exe`,
			`C:\mingw32\bin\less.exe`,
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, `scoop\apps\less\current\less.exe`))
		}
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				cmd := exec.Command(path, "-R", "-S")
				setCmdParams(cmd)
				return cmd, true, nil
			}
		}
	}

	// check for more
	if pagerPath, err = exec.LookPath("more"); err == nil {
		return exec.Command(pagerPath), false, nil
	}

	// you live in the void
	return nil, false, errors.New("no pager found: neither 'less' nor 'more' is available")
}
