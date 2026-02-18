package sshexec

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aerospike/aerolab/pkg/termutil"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
	"github.com/google/uuid"
	"github.com/rglonek/logger"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type ExecInput struct {
	ClientConf
	ExecDetail
}

type ExecDetail struct {
	Command        []string      // command to run; for interactive, leave command empty, and set Stdin/out/err to os.Stdin/out/err
	Stdin          io.ReadCloser // stdin if required
	Stdout         io.Writer     // stdout, leave empty for the system to capture
	Stderr         io.Writer     // stderr, leave empty for the system to capture; this will be empty and all output will go to stdout if Terminal=true
	SessionTimeout time.Duration // timeout after which the connected running session will be forcibly terminated
	Env            []*Env        // environment variables; for this to work, the pattern must be accepted in /etc/ssh/sshd_config: AcceptEnv (ex: AcceptEnv *)
	Terminal       bool          // request a terminal
}

type ClientConf struct {
	Host           string        // host
	Port           int           // port
	Username       string        // auth - username to use
	Password       string        // auth - password to use
	PrivateKey     []byte        // auth - private key to use
	ConnectTimeout time.Duration // connect timeout
	MaxRetries     int           // max retries for operations (default: 0 = no retries)
	RetrySleep     time.Duration // sleep between retries (default: 5s if MaxRetries > 0)
}

type Env struct {
	Key   string
	Value string
}

type ExecOutput struct {
	Stdout []byte
	Stderr []byte
	Err    error
	Warn   []string
}

func (o *ExecOutput) addWarn(f string, params ...any) {
	o.Warn = append(o.Warn, fmt.Sprintf(f, params...))
}

func Exec(i *ExecInput) *ExecOutput {
	maxRetries := max(i.MaxRetries, 0)
	retrySleep := i.RetrySleep
	if retrySleep <= 0 {
		retrySleep = 5 * time.Second
	}

	var lastOutput *ExecOutput
	for attempt := 0; attempt <= maxRetries; attempt++ {
		session, conn, err := ExecPrepare(i)
		if err != nil {
			lastOutput = &ExecOutput{
				Err: err,
			}
			if attempt < maxRetries {
				time.Sleep(retrySleep)
				continue
			}
			return lastOutput
		}
		lastOutput = ExecRun(session, conn, i)
		if lastOutput.Err == nil {
			return lastOutput
		}
		if attempt < maxRetries {
			time.Sleep(retrySleep)
		}
	}
	if maxRetries > 0 && lastOutput != nil && lastOutput.Err != nil {
		lastOutput.Err = fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastOutput.Err)
	}
	return lastOutput
}

func ExecRun(session *ssh.Session, conn *ssh.Client, i *ExecInput) *ExecOutput {
	defer session.Close()
	defer conn.Close()
	// make bash script
	var script string
	if len(i.Command) > 0 {
		script = makeScript(i.Command)
	}
	var err error
	// define outputs
	out := &ExecOutput{}

	// set env
	for _, kv := range i.Env {
		err = session.Setenv(kv.Key, kv.Value)
		if err != nil {
			out.addWarn("Failed to set env: %s", err)
		}
	}

	// Set the terminal
	if i.Terminal {
		session.Setenv("TERM", "xterm-256color")
		modes := ssh.TerminalModes{
			ssh.ECHO:          1,     // Enable echoing
			ssh.TTY_OP_ISPEED: 14400, // Input speed = 14.4kbaud
			ssh.TTY_OP_OSPEED: 14400, // Output speed = 14.4kbaud
		}
		if err := session.RequestPty("xterm-256color", 80, 80, modes); err != nil {
			return &ExecOutput{
				Err:  fmt.Errorf("failed to request pty: %s", err),
				Warn: out.Warn,
			}
		}
	}
	restoreCount.Add(1)
	defer restore()

	// Set up stdin, stdout, and stderr for the session
	session.Stdin = i.Stdin
	session.Stdout = i.Stdout
	session.Stderr = i.Stderr
	var stdout, stderr bytes.Buffer
	if i.Stdout == nil {
		session.Stdout = &stdout
	}
	if i.Stderr == nil {
		session.Stderr = &stderr
	}

	// Handle window resize
	sessid := uuid.New().String()
	if i.Terminal {
		resize(session)
	} else {
		resize(nil)
	}
	sessionsLock.Lock()
	sessions[sessid] = session
	sessionsLock.Unlock()

	// session and output cleanup
	defer func() {
		sessionsLock.Lock()
		delete(sessions, sessid)
		sessionsLock.Unlock()
		if i.Stdout == nil {
			out.Stdout = stdout.Bytes()
		}
		if i.Stderr == nil {
			out.Stderr = stderr.Bytes()
		}
	}()

	// session timeout handling
	if i.SessionTimeout != 0 {
		tout := make(chan struct{}, 1)
		defer func() {
			tout <- struct{}{}
		}()
		start := time.Now()
		go func() {
			for time.Since(start) < i.SessionTimeout {
				time.Sleep(time.Second)
				if len(tout) > 0 {
					return
				}
			}
			out.Err = errors.New("session timeout")
			session.Close()
			conn.Close()
		}()
	}

	if len(i.Command) > 0 {
		// Run the script
		if err := session.Run(script); err != nil {
			// Try to extract the script path for better error messages
			if scriptPath := extractScriptPath(i.Command); scriptPath != "" {
				out.Err = errors.Join(out.Err, fmt.Errorf("session failed executing remote script %s: %s", scriptPath, err))
			} else {
				out.Err = errors.Join(out.Err, fmt.Errorf("session: %s", err))
			}
			return out
		}
	} else {
		// Start an interactive shell
		if err := session.Shell(); err != nil {
			out.Err = errors.Join(out.Err, fmt.Errorf("session-start: %s", err))
			return out
		}
		// Wait for the session to finish
		if err := session.Wait(); err != nil {
			out.Err = errors.Join(out.Err, fmt.Errorf("session: %s", err))
			return out
		}
	}

	// done
	return out
}

func ExecPrepare(i *ExecInput) (session *ssh.Session, conn *ssh.Client, err error) {
	// get client config
	config, err := makeClientConfig(&i.ClientConf)
	if err != nil {
		return nil, nil, err
	}

	// ssh dial
	currentTimeout := i.ConnectTimeout
	start := time.Now()
	for {
		conn, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", i.Host, i.Port), config)
		if err == nil {
			break
		}
		currentTimeout -= time.Since(start)
		if currentTimeout <= 0 && i.ConnectTimeout > 0 {
			return nil, nil, fmt.Errorf("failed to dial: %s", err)
		}
		time.Sleep(time.Second)
	}

	// Create a session
	session, err = conn.NewSession()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("failed to create session: %s", err)
	}
	return session, conn, nil
}

var sessionsLock = new(sync.RWMutex)
var sessions = make(map[string]*ssh.Session)

// handle window resizing adjusts the terminal size dynamically
func resize(session *ssh.Session) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		width, height, err := term.GetSize(fd)
		if err != nil {
			log.Printf("handleWindowResize: failed to get terminal size: %s", err)
			return
		}
		if session != nil {
			if _, err := term.MakeRaw(fd); err == nil {
				logger.SetRawTerminalMode(true)
			}
			if err := session.WindowChange(height, width); err != nil {
				log.Printf("handleWindowResize: failed to set window size: %s", err)
			}
		} else {
			sessionsLock.RLock()
			for _, session := range sessions {
				if err := session.WindowChange(height, width); err != nil {
					log.Printf("handleWindowResize: failed to set window size: %s", err)
				}
			}
			sessionsLock.RUnlock()
		}
	}
}

var restore = func() {}
var restoreCount atomic.Int64

func AddRestoreRequest() {
	restoreCount.Add(1)
}

func RestoreTerminal() {
	restore()
}

// savedTermState stores the original terminal state for signal-based restoration
var savedTermState *term.State

func init() {
	// handle restoring of terminal state
	fileDescriptor := int(os.Stdin.Fd())
	if term.IsTerminal(fileDescriptor) && termutil.IsForegroundNoError(uintptr(fileDescriptor), true) {
		var err error
		termState, err := term.GetState(fileDescriptor)
		if err != nil {
			log.Printf("Could not store terminal state, terminal may become corrupt: %s", err)
		} else {
			savedTermState = termState
			restore = func() {
				if restoreCount.Add(-1) == 0 {
					err := term.Restore(int(os.Stdin.Fd()), termState)
					if err != nil {
						log.Printf("FAILED to restore terminal state, run 'reset' or 'stty sane': %s", err)
					}
					logger.SetRawTerminalMode(false)
				}
			}
			// Register terminal restore with shutdown handler for signal-based cleanup
			shutdown.AddEarlyCleanupJob("terminal-restore", func(isSignal bool) {
				if savedTermState != nil {
					term.Restore(fileDescriptor, savedTermState)
					logger.SetRawTerminalMode(false)
				}
			})
		}
	}
	// init window resizer
	go winResize()
}

func makeClientConfig(i *ClientConf) (*ssh.ClientConfig, error) {
	config := &ssh.ClientConfig{
		User:            i.Username,
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	if len(i.PrivateKey) > 0 {
		signer, err := ssh.ParsePrivateKey(i.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("unable to parse private key: %v", err)
		}
		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	}
	if len(i.Password) > 0 {
		config.Auth = append(config.Auth, ssh.Password(i.Password))
	}
	if i.ConnectTimeout != 0 {
		config.Timeout = i.ConnectTimeout
	}
	return config, nil
}

func makeScript(command []string) string {
	bashArray := "args=(" + strings.Join(escapeForBash(command), " ") + ")"
	base64Command := base64.StdEncoding.EncodeToString([]byte(bashArray))
	return fmt.Sprintf(`
	decoded=$(echo %s | base64 -d)
	eval "$decoded"
	"${args[@]}"
	`, base64Command)
}

func escapeForBash(args []string) []string {
	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return escaped
}

// extractScriptPath extracts the script path from a command if it looks like a script execution.
// Returns empty string if no script path can be determined.
func extractScriptPath(command []string) string {
	if len(command) == 0 {
		return ""
	}

	// Common shell interpreters that take a script path as the first argument
	shells := []string{"bash", "sh", "/bin/bash", "/bin/sh", "/usr/bin/bash", "/usr/bin/sh"}

	// Check if command[0] is a shell and command[1] looks like a path
	for _, shell := range shells {
		if command[0] == shell && len(command) > 1 {
			// command[1] should be the script path
			path := command[1]
			if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "./") {
				return path
			}
		}
	}

	// Check if command[0] itself is a script path (direct execution)
	if strings.HasPrefix(command[0], "/") || strings.HasPrefix(command[0], "./") {
		// Only return if it looks like a script (has an extension or is in a scripts directory)
		if strings.Contains(command[0], "/scripts/") || strings.HasSuffix(command[0], ".sh") {
			return command[0]
		}
	}

	return ""
}
