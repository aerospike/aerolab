package dispatcher

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"regexp"
	"time"
)

// nodeIDClusterRe matches the periodic ticker line emitted by every
// Aerospike server, e.g.:
//
//	Aug 20 2024 12:34:56 GMT: INFO (info): (ticker.c:184)  NODE-ID bb9... CLUSTER-SIZE 3 CLUSTER-NAME bob
//
// The regex is the same one used by ingest.Config.FindClusterNameNodeIdRegex
// (see [src/pkg/agi/ingest/struct.go]) so the dispatcher and the AGI
// listener stay aligned.
var nodeIDClusterRe = regexp.MustCompile(`NODE-ID (?P<NodeId>[^ ]+) CLUSTER-SIZE \d+( CLUSTER-NAME (?P<ClusterName>[^ \r\n]+))?`)

// scanLogForIdentity opens path, scans up to maxWait for a line that
// matches the NODE-ID/CLUSTER-NAME ticker, and returns the captured
// values. It does NOT consume the file from EOF — it scans from the
// start so that even a freshly-rotated log file still surfaces the
// identity on its first ticker.
//
// On context cancel, scanner EOF without a hit, or maxWait expiry it
// returns an error so the caller can fall through to the next
// identity-discovery method.
func scanLogForIdentity(ctx context.Context, path string, maxWait time.Duration) (nodeID, cluster string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	tctx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	r := bufio.NewReaderSize(f, 64*1024)
	deadline := time.Now().Add(maxWait)
	for {
		if tctx.Err() != nil {
			return "", "", tctx.Err()
		}
		line, rerr := r.ReadString('\n')
		if len(line) > 0 {
			if nid, cn, ok := matchIdentity(line); ok {
				return nid, cn, nil
			}
		}
		if rerr == nil {
			continue
		}
		if !errors.Is(rerr, io.EOF) {
			return "", "", rerr
		}
		if time.Now().After(deadline) {
			return "", "", errors.New("scanLogForIdentity: timed out without seeing NODE-ID line")
		}
		// At EOF; pause briefly waiting for the server to emit
		// another line.
		select {
		case <-tctx.Done():
			return "", "", tctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// matchIdentity tests a single log line for the NODE-ID/CLUSTER-NAME
// ticker pattern and returns the capture groups when present.
// Exposed unexported so the unit test can drive it directly.
func matchIdentity(line string) (nodeID, cluster string, ok bool) {
	m := nodeIDClusterRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	for i, name := range nodeIDClusterRe.SubexpNames() {
		switch name {
		case "NodeId":
			nodeID = m[i]
		case "ClusterName":
			cluster = m[i]
		}
	}
	return nodeID, cluster, nodeID != ""
}
