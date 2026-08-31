package sshexec

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/citrusleaf/aerolab/pkg/termutil"
	"github.com/citrusleaf/aerolab/pkg/utils/shutdown"
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
	// Dialer, if non-nil, is used to obtain the underlying net.Conn instead of
	// the default net.Dial("tcp", host:port). Host/Port are still passed to
	// ssh.NewClientConn for the host-key entry / known_hosts. Used for IAP and
	// other tunnels that produce a pre-connected net.Conn.
	Dialer func(ctx context.Context) (net.Conn, error)

	// HostKeyStore, when set together with HostKeyID, enables trust-on-first-use
	// host key verification. Leaving either empty preserves the unverified
	// behaviour, which is what external hosts AeroLab does not manage still get.
	HostKeyStore *HostKeyStore
	// HostKeyID is the stable identity the host key is remembered against,
	// typically "<backend>/<clusterUUID>/<nodeNo>".
	HostKeyID string
	// HostKeyStrict refuses the connection on a key mismatch instead of
	// warning through HostKeyLogf and relearning.
	HostKeyStrict bool
	// HostKeyLogf receives host key warnings. When nil, warnings are dropped.
	HostKeyLogf func(format string, args ...any)
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

// Exec prepares and runs a command over SSH with retry semantics (see
// ExecWithRetry). It does not register a shutdown cleanup job; callers that need
// signal-driven interruption should use ExecWithRetry with a cleanup name.
func Exec(i *ExecInput) *ExecOutput {
	return ExecWithRetry(i, "")
}

// ExecWithRetry prepares and runs a command over SSH, honoring i.MaxRetries /
// i.RetrySleep. On failure it retries with a freshly-dialed connection, which
// lets it recover from transient issues such as a mid-command channel teardown
// ("wait: remote command exited without exit status or exit signal") that
// happens when a node's SSH connection briefly drops while the script is still
// running. cleanupName, when non-empty, registers an early shutdown cleanup job
// that force-closes the live connection on signal (SIGINT/SIGTERM); once that
// fires the loop stops retrying and the returned error is reported as
// "interrupted".
//
// This is the entry point the backends use for instance command execution so
// that the documented --max-retries / --retry-sleep behavior actually applies
// to the command itself and not just to the initial dial and SFTP transfers.
func ExecWithRetry(i *ExecInput, cleanupName string) *ExecOutput {
	maxRetries := max(i.MaxRetries, 0)
	retrySleep := i.RetrySleep
	if retrySleep <= 0 {
		retrySleep = 5 * time.Second
	}

	var (
		mu          sync.Mutex
		curSession  *ssh.Session
		curConn     *ssh.Client
		interrupted bool
	)
	if cleanupName != "" {
		shutdown.AddEarlyCleanupJob(cleanupName, func(isSignal bool) {
			if !isSignal {
				return
			}
			mu.Lock()
			interrupted = true
			s, c := curSession, curConn
			mu.Unlock()
			if s != nil {
				s.Close()
			}
			if c != nil {
				c.Close()
			}
		})
	}
	isInterrupted := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return interrupted
	}

	var lastOutput *ExecOutput
	for attempt := 0; attempt <= maxRetries; attempt++ {
		session, conn, err := ExecPrepare(i)
		if err != nil {
			lastOutput = &ExecOutput{Err: err}
			if isInterrupted() {
				lastOutput.Err = errors.New("interrupted")
				return lastOutput
			}
			if attempt < maxRetries {
				time.Sleep(retrySleep)
				continue
			}
			break
		}
		mu.Lock()
		curSession, curConn = session, conn
		mu.Unlock()

		out := ExecRun(session, conn, i)
		// Carry the earlier attempts' output forward. A retry usually fails
		// differently from the attempt that triggered it -- when a node drops
		// mid-script the retry cannot even dial, so it produces nothing -- and
		// returning only the last attempt threw away the script log that
		// explained the failure, leaving callers (scriptlog.SaveFailure among
		// them) to report "no output captured".
		if lastOutput != nil {
			out.Stdout = joinAttemptOutput(lastOutput.Stdout, out.Stdout, attempt)
			out.Stderr = joinAttemptOutput(lastOutput.Stderr, out.Stderr, attempt)
			out.Warn = append(lastOutput.Warn, out.Warn...)
		}
		lastOutput = out

		if isInterrupted() {
			lastOutput.Err = errors.New("interrupted")
			return lastOutput
		}
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

// joinAttemptOutput concatenates the output of a retried command's attempts,
// labelling the boundary so a reader can tell which attempt produced what.
// attempt is the zero-based index of the newer output. Either side may be
// empty: a stream that produced nothing contributes nothing, and when the
// caller redirected the stream (i.Stdout / i.Stderr set) both sides are nil.
func joinAttemptOutput(prev []byte, cur []byte, attempt int) []byte {
	if len(prev) == 0 {
		return cur
	}
	if len(cur) == 0 {
		return prev
	}
	sep := fmt.Sprintf("\n--- retry %d ---\n", attempt)
	joined := make([]byte, 0, len(prev)+len(sep)+len(cur))
	joined = append(joined, prev...)
	joined = append(joined, sep...)
	joined = append(joined, cur...)
	return joined
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
		session.Setenv("TERM", "xterm-256color") //nolint:errcheck
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

// Dial opens an SSH connection using the shared client configuration. Callers
// that need a raw *ssh.Client should use this rather than ssh.Dial directly,
// so they inherit host key verification and any custom dialer (such as GCP
// IAP) configured on the ClientConf.
func Dial(i *ClientConf) (*ssh.Client, error) {
	config, err := makeClientConfig(i)
	if err != nil {
		return nil, err
	}
	return dialSSH(i, fmt.Sprintf("%s:%d", i.Host, i.Port), config)
}

func ExecPrepare(i *ExecInput) (session *ssh.Session, conn *ssh.Client, err error) {
	// get client config
	config, err := makeClientConfig(&i.ClientConf)
	if err != nil {
		return nil, nil, err
	}

	// ssh dial
	addr := fmt.Sprintf("%s:%d", i.Host, i.Port)
	currentTimeout := i.ConnectTimeout
	start := time.Now()
	for {
		conn, err = dialSSH(&i.ClientConf, addr, config)
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

// dialSSH establishes an *ssh.Client. When ClientConf.Dialer is set, the
// underlying net.Conn is obtained via that custom dialer (e.g. IAP); otherwise
// a plain net.Dial("tcp", addr) is used. In both cases ssh.NewClientConn is
// driven over the resulting net.Conn so the rest of the call site (sessions,
// tunnels, etc.) is unchanged.
//
// The dial-timeout for custom dialers is enforced via a goroutine race rather
// than via context.WithTimeout. This is intentional and necessary: some
// dialers (notably cedws/iapc) bind the long-lived connection's WebSocket
// read/write to the SAME context that was passed to the dial. If we cancelled
// that context via defer cancel() after a successful handshake, the websocket
// would tear down, the SSH transport's read loop would see EOF and call
// underlying-conn Close to unblock its reader, and the iap conn's internal
// sendNbCh would close. The very next outbound SSH packet (e.g.
// conn.NewSession()) would then panic with "send on closed channel" inside
// iapc.(*Conn).Write. By passing context.Background() to the dialer and
// timing out the dial via a separate goroutine, the conn's I/O context stays
// alive for as long as the conn itself is in use.
func dialSSH(cc *ClientConf, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	if cc.Dialer == nil {
		return ssh.Dial("tcp", addr, config)
	}

	deadline := time.Time{}
	if cc.ConnectTimeout > 0 {
		deadline = time.Now().Add(cc.ConnectTimeout)
	}

	type dialResult struct {
		nc  net.Conn
		err error
	}
	resCh := make(chan dialResult, 1)
	go func() {
		nc, err := cc.Dialer(context.Background())
		resCh <- dialResult{nc: nc, err: err}
	}()

	var nc net.Conn
	var err error
	if cc.ConnectTimeout > 0 {
		timer := time.NewTimer(cc.ConnectTimeout)
		select {
		case res := <-resCh:
			timer.Stop()
			nc, err = res.nc, res.err
		case <-timer.C:
			// The dial goroutine may still complete after we return; close
			// the late-arriving conn so we don't leak a tunnel.
			go func() {
				late := <-resCh
				if late.nc != nil {
					_ = late.nc.Close()
				}
			}()
			return nil, fmt.Errorf("dial timeout after %s", cc.ConnectTimeout)
		}
	} else {
		res := <-resCh
		nc, err = res.nc, res.err
	}

	if err != nil {
		return nil, err
	}

	// The handshake needs the same bound as the dial. ssh.ClientConfig.Timeout
	// only covers the TCP dial ssh.Dial performs itself, so with a custom
	// dialer nothing limits ssh.NewClientConn. Over an IAP tunnel that is the
	// phase that hangs: the WebSocket dial to the tunnel endpoint succeeds
	// straight away and IAP only reports that it cannot reach the VM ("code
	// 4003 (failed to connect to backend)", i.e. sshd is not up yet) about 30
	// seconds later, so a caller polling a booting instance on a 5s budget got
	// six times fewer attempts than it asked for.
	type handshakeResult struct {
		conn  ssh.Conn
		chans <-chan ssh.NewChannel
		reqs  <-chan *ssh.Request
		err   error
	}
	hsCh := make(chan handshakeResult, 1)
	go func() {
		sshConn, chans, reqs, err := ssh.NewClientConn(nc, addr, config)
		hsCh <- handshakeResult{conn: sshConn, chans: chans, reqs: reqs, err: err}
	}()

	var hs handshakeResult
	if !deadline.IsZero() {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			_ = nc.Close()
			return nil, fmt.Errorf("dial timeout after %s", cc.ConnectTimeout)
		}
		timer := time.NewTimer(remaining)
		select {
		case hs = <-hsCh:
			timer.Stop()
		case <-timer.C:
			// Closing the conn unblocks the handshake goroutine; drain it so
			// a late success does not leak the tunnel.
			_ = nc.Close()
			go func() {
				late := <-hsCh
				if late.conn != nil {
					_ = late.conn.Close()
				}
			}()
			return nil, fmt.Errorf("ssh handshake timeout after %s", cc.ConnectTimeout)
		}
	} else {
		hs = <-hsCh
	}

	if hs.err != nil {
		_ = nc.Close()
		return nil, hs.err
	}
	return ssh.NewClient(hs.conn, hs.chans, hs.reqs), nil
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
					term.Restore(fileDescriptor, savedTermState) //nolint:errcheck
					logger.SetRawTerminalMode(false)
				}
			})
		}
	}
	// init window resizer
	go winResize()
}

func makeClientConfig(i *ClientConf) (*ssh.ClientConfig, error) {
	// Hosts AeroLab tracks get trust-on-first-use verification against the
	// host key store. Everything else (user-supplied external hosts) keeps the
	// previous unverified behaviour.
	hostKeyCallback := ssh.InsecureIgnoreHostKey()
	if i.HostKeyStore != nil && i.HostKeyID != "" {
		hostKeyCallback = i.HostKeyStore.callback(i.HostKeyID, i.HostKeyStrict, i.HostKeyLogf)
	}
	config := &ssh.ClientConfig{
		User:            i.Username,
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: hostKeyCallback,
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
