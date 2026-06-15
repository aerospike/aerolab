// Package iap provides a thin adapter around github.com/cedws/iapc/iap that
// hides the third-party import behind aerolab-shaped types. It exposes a
// MakeDialer factory that produces a func(context.Context) (net.Conn, error)
// suitable for sshexec.ClientConf.Dialer.
//
// The IAP TCP-forwarding protocol opens a WebSocket to tunnel.cloudproxy.app
// per call, authenticates with a Google OAuth2 token, and tunnels TCP frames
// to (project, zone, instance, port). The underlying *iap.Conn implements
// net.Conn so it plugs straight into ssh.NewClientConn.
package iap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	iaplib "github.com/cedws/iapc/iap"
	"golang.org/x/oauth2"
)

// Target identifies a single GCE instance reachable via IAP TCP forwarding.
// Interface defaults to "nic0" when empty.
type Target struct {
	Project   string
	Zone      string
	Instance  string
	Interface string
	Port      int
}

// Dialer is the function shape consumed by sshexec.ClientConf.Dialer.
type Dialer func(ctx context.Context) (net.Conn, error)

// MakeDialer returns a Dialer that opens a fresh IAP tunnel per call.
//
// One tunnel is opened per ssh.Dial. This matches gcloud's behavior and lets
// each parallel SSH session have its own tunnel. The caller is responsible for
// closing the returned net.Conn -- ssh.NewClientConn / ssh.Client.Close will
// take care of this in the normal flow.
func MakeDialer(ts oauth2.TokenSource, t Target) Dialer {
	iface := t.Interface
	if iface == "" {
		iface = "nic0"
	}
	return func(ctx context.Context) (net.Conn, error) {
		conn, err := iaplib.Dial(ctx,
			iaplib.WithProject(t.Project),
			iaplib.WithInstance(t.Instance, t.Zone, iface),
			iaplib.WithPort(strconv.Itoa(t.Port)),
			iaplib.WithTokenSource(&ts),
		)
		if err != nil {
			return nil, annotateDialError(t, err)
		}
		return conn, nil
	}
}

// annotateDialError adds operator-actionable hints to the most common IAP
// failure modes. The original error is preserved via errors.Is/As.
func annotateDialError(t Target, err error) error {
	msg := err.Error()
	lc := strings.ToLower(msg)

	var hint string
	switch {
	case strings.Contains(lc, "403"),
		strings.Contains(lc, "forbidden"),
		strings.Contains(lc, "permission"),
		strings.Contains(lc, "unauthorized"),
		strings.Contains(lc, "401"):
		hint = "IAP dial denied. Check that the calling principal has " +
			"`roles/iap.tunnelResourceAccessor` on the project or instance, " +
			"that the IAP API (iap.googleapis.com) is enabled, and that " +
			"firewall rules allow tcp:22 from 35.235.240.0/20."
	case strings.Contains(lc, "iap.googleapis.com"),
		strings.Contains(lc, "service has not been used"),
		strings.Contains(lc, "api has not been used"):
		hint = "Enable the IAP API for this project: " +
			"`gcloud services enable iap.googleapis.com --project=" + t.Project + "`."
	case strings.Contains(lc, "not found"),
		strings.Contains(lc, "404"):
		hint = fmt.Sprintf("IAP could not find instance %q in zone %q of project %q. "+
			"Verify the instance is running and that the zone is correct.",
			t.Instance, t.Zone, t.Project)
	}

	if hint == "" {
		return fmt.Errorf("iap dial %s/%s/%s:%d: %w",
			t.Project, t.Zone, t.Instance, t.Port, err)
	}
	return fmt.Errorf("iap dial %s/%s/%s:%d: %w (%s)",
		t.Project, t.Zone, t.Instance, t.Port, err, hint)
}

// ErrNoTokenSource is returned by callers that try to build a dialer without
// a usable oauth2.TokenSource.
var ErrNoTokenSource = errors.New("iap: no oauth2 token source supplied")
