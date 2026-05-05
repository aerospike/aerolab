package dispatcher

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// asinfoBinary runs `asinfo -h <host> -p <port> -v <verb>` and returns
// stdout's first line, trimmed. Returns an empty string + error when
// asinfo is not on $PATH or the command fails — both normal early-
// boot conditions where the dispatcher silently falls through to the
// log-scan based identity discovery path.
//
// We deliberately shell out instead of speaking the asinfo wire
// protocol natively because:
//   - asinfo's text mode requires a length-prefixed proto header that
//     we'd otherwise have to vendor; the binary already exists on
//     every Aerospike server install.
//   - This is a one-shot probe at startup; sub-second exec cost is
//     acceptable.
//   - Falling through to log-scan is the always-correct backup path,
//     so a missing asinfo binary is a non-issue.
func asinfoBinary(ctx context.Context, host string, port int, verb string) (string, error) {
	const bin = "asinfo"
	out, err := runCommand(ctx, bin, "-h", host, "-p", fmt.Sprintf("%d", port), "-v", verb)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	if idx := strings.IndexByte(out, '\n'); idx >= 0 {
		out = out[:idx]
	}
	if out == "" {
		return "", fmt.Errorf("asinfo %q returned empty value", verb)
	}
	return out, nil
}

// asinfoNodeAndCluster returns the local node-id and cluster-name as
// reported by asinfo, or an error if asinfo is not available. It is
// the fast path for identity discovery; on failure, the dispatcher
// falls back to log-scan based discovery.
//
// Both node and cluster-name are queried separately because asinfo
// only accepts one verb per invocation. cluster-name failures are
// non-fatal: the function returns the (already-resolved) nodeID
// alongside a cluster="" value so the caller can decide whether to
// use the CLI default.
func asinfoNodeAndCluster(ctx context.Context) (nodeID string, cluster string, err error) {
	const host = "127.0.0.1"
	const port = 3000
	nodeID, err = asinfoBinary(ctx, host, port, "node")
	if err != nil {
		return "", "", fmt.Errorf("asinfo node: %w", err)
	}
	cluster, err = asinfoBinary(ctx, host, port, "cluster-name")
	if err != nil {
		return nodeID, "", fmt.Errorf("asinfo cluster-name: %w", err)
	}
	return nodeID, cluster, nil
}

// dialableTCP returns true if the given address accepts a TCP connect
// within the given timeout. Used as a quick "is the service even up?"
// probe before invoking asinfo.
func dialableTCP(ctx context.Context, addr string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
