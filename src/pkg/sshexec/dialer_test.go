package sshexec

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// TestExecPrepare_DialerErrorIsReturned validates that when a custom Dialer
// returns an error, ExecPrepare surfaces it (after the retry loop) rather than
// silently falling back to direct TCP. This is the whole reason the Dialer
// field exists -- callers must be able to swap the underlying transport.
func TestExecPrepare_DialerErrorIsReturned(t *testing.T) {
	called := 0
	dialerErr := errors.New("synthetic dialer failure")
	in := &ExecInput{
		ClientConf: ClientConf{
			Host:           "10.0.0.1",
			Port:           22,
			Username:       "anyone",
			ConnectTimeout: 100 * time.Millisecond,
			Dialer: func(ctx context.Context) (net.Conn, error) {
				called++
				return nil, dialerErr
			},
		},
	}
	_, _, err := ExecPrepare(in)
	if err == nil {
		t.Fatal("expected error from custom dialer, got nil")
	}
	if called == 0 {
		t.Fatal("custom dialer was never invoked")
	}
}

// TestExecPrepare_NoDialerAttemptsTCPConnect validates that when Dialer is nil
// the existing behavior (ssh.Dial over plain TCP) is preserved -- i.e. we did
// not accidentally make Dialer mandatory.
func TestExecPrepare_NoDialerAttemptsTCPConnect(t *testing.T) {
	in := &ExecInput{
		ClientConf: ClientConf{
			// 0.0.0.0:0 / a tight timeout means the dial will fail fast on any
			// platform without invoking our custom dialer (because there isn't one).
			Host:           "127.0.0.1",
			Port:           1, // privileged port; should refuse / time out
			Username:       "anyone",
			ConnectTimeout: 100 * time.Millisecond,
		},
	}
	_, _, err := ExecPrepare(in)
	if err == nil {
		t.Fatal("expected error from doomed direct TCP dial, got nil")
	}
}

// TestDialSSH_DialerContextIsNotCancelledOnSuccess pins the contract that
// dialSSH does NOT use a context-with-timeout for the dialer. Some dialers
// (notably cedws/iapc) bind the resulting net.Conn's long-lived I/O to the
// dial context; cancelling it after a successful handshake breaks the conn
// and panics on the next write. This test asserts the ctx the dialer receives
// is still alive after the dial returns.
func TestDialSSH_DialerContextIsNotCancelledOnSuccess(t *testing.T) {
	var dialedCtx context.Context
	cc := &ClientConf{
		Host:           "127.0.0.1",
		Port:           1,
		Username:       "anyone",
		ConnectTimeout: 100 * time.Millisecond,
		Dialer: func(ctx context.Context) (net.Conn, error) {
			dialedCtx = ctx
			// Return an error so dialSSH doesn't proceed to ssh.NewClientConn
			// (we don't actually have a server). We only care that ctx is
			// observed and survives past dialSSH's return.
			return nil, errors.New("dialer-not-actually-dialing")
		},
	}
	_, _ = dialSSH(cc, "127.0.0.1:1", nil)

	if dialedCtx == nil {
		t.Fatal("dialer was never invoked")
	}
	// Give any (broken) defer-cancel time to fire if it existed.
	time.Sleep(50 * time.Millisecond)
	select {
	case <-dialedCtx.Done():
		t.Fatalf("dialer ctx was cancelled after dialSSH returned: %s", dialedCtx.Err())
	default:
	}
}

// TestDialSSH_HangingDialerIsTimedOut asserts that a dialer that never
// returns is killed by ConnectTimeout. We must not lose the timeout safety
// when removing the context-based timeout.
func TestDialSSH_HangingDialerIsTimedOut(t *testing.T) {
	cc := &ClientConf{
		Host:           "127.0.0.1",
		Port:           1,
		Username:       "anyone",
		ConnectTimeout: 50 * time.Millisecond,
		Dialer: func(ctx context.Context) (net.Conn, error) {
			// Sleep a little longer than the timeout so we test the timeout
			// path. We don't honour ctx cancellation on purpose -- iapc
			// likewise doesn't bail out of an in-progress WebSocket dial just
			// because some unrelated context is cancelled.
			time.Sleep(500 * time.Millisecond)
			return nil, errors.New("late")
		},
	}
	start := time.Now()
	_, err := dialSSH(cc, "127.0.0.1:1", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("dialSSH did not honour ConnectTimeout, took %s", elapsed)
	}
}
