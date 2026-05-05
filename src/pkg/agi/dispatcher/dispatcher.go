package dispatcher

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config configures a Dispatcher. All fields are optional except
// Target and Token; the rest are auto-detected at runtime.
//
// The dispatcher pulls in NO CLI deps so it can be unit-tested in
// isolation; cmdAgiExecDispatch.go (in cli/cmd/v1) is responsible
// for translating CLI flags into a Config.
type Config struct {
	// Target is the AGI instance URL (e.g. https://10.0.0.5:443).
	// Required.
	Target string

	// Token is the bearer token to authenticate POSTs with. Required.
	// Read from a token-file by the CLI rather than passed on argv.
	Token string

	// ClusterName, NodeID — optional overrides; auto-detected via
	// asinfo, with log-scan fallback, when empty.
	ClusterName string
	NodeID      string

	// AerospikeConf is the path to aerospike.conf on the cluster
	// node. Used by ResolveSource for log-destination auto-discovery.
	// Defaults to /etc/aerospike/aerospike.conf when empty.
	AerospikeConf string

	// SourceFile / SourceJournal are explicit overrides for the log
	// source. Only one should be set; if both are empty, the
	// dispatcher inspects AerospikeConf.
	SourceFile    string
	SourceJournal string

	// StateFile is where the dispatcher persists its byte-offset
	// (and journal-cursor) checkpoint across restarts. Empty
	// disables persistence (useful for tests).
	StateFile string

	// InsecureTLS disables TLS verification for the POST to Target.
	// AGI's self-signed cert is the typical case; production
	// deployments should use a trusted CA + InsecureTLS=false.
	InsecureTLS bool

	// BackfillFromStart, when true, opens file sources at offset 0
	// rather than at EOF on first start. Useful for one-shot
	// backfills of an existing log file.
	BackfillFromStart bool

	// Logger is the optional output stream for diagnostic messages.
	// Nil sends them to log.Default().
	Logger *log.Logger

	// HTTPClient, if non-nil, is used instead of a constructed
	// default. Tests inject a stub here.
	HTTPClient *http.Client

	// Now is a clock injection point for tests; nil uses time.Now.
	Now func() time.Time
}

// Dispatcher is the long-running coordinator. Construct with New
// and then call Run with a context. Run blocks until ctx is done or
// an unrecoverable error occurs.
type Dispatcher struct {
	cfg    Config
	logger *log.Logger
	hc     *http.Client
	now    func() time.Time

	// Resolved at startup:
	source   Source
	cluster  string
	nodeID   string
	sourceID string
	state    *stateStore
}

// New constructs a Dispatcher. It performs no I/O — Run does.
func New(cfg Config) *Dispatcher {
	d := &Dispatcher{
		cfg:    cfg,
		logger: cfg.Logger,
		hc:     cfg.HTTPClient,
		now:    cfg.Now,
	}
	if d.logger == nil {
		d.logger = log.Default()
	}
	if d.now == nil {
		d.now = time.Now
	}
	if d.cfg.AerospikeConf == "" {
		d.cfg.AerospikeConf = "/etc/aerospike/aerospike.conf"
	}
	if d.hc == nil {
		tr := &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: d.cfg.InsecureTLS},
			DisableKeepAlives: false,
			// We want the response body's chunks acknowledged
			// promptly; long idles are fine, and the connection
			// itself is held open by the chunked POST body.
			IdleConnTimeout: 0,
		}
		d.hc = &http.Client{
			Transport: tr,
			// No client-side overall timeout: the request body is
			// long-lived (this is a streaming POST). Per-write
			// deadlines are enforced via the underlying TCP socket.
			Timeout: 0,
		}
	}
	return d
}

// Run starts the dispatcher and blocks until ctx is cancelled or the
// dispatcher hits an unrecoverable startup error (e.g. the source
// path is invalid AND auto-discovery fails too).
//
// Recoverable transport errors (HTTP 5xx, network failures, server
// shutdowns mid-stream) trigger reconnect with exponential backoff
// 1s..30s; the file-tail goroutine continues to track the live tail
// during the backoff so no bytes are lost as long as the source
// file isn't rotated faster than the backoff completes.
func (d *Dispatcher) Run(ctx context.Context) error {
	if err := d.cfg.validate(); err != nil {
		return err
	}

	d.source = ResolveSource(d.cfg)
	d.logger.Printf("dispatcher: resolved source = %s", d.source.Label())

	if err := d.resolveIdentity(ctx); err != nil {
		return fmt.Errorf("resolve identity: %w", err)
	}
	d.logger.Printf("dispatcher: cluster=%s node=%s", d.cluster, d.nodeID)

	d.sourceID = computeSourceID(d.cluster, d.nodeID, d.source)
	st, err := newStateStore(d.cfg.StateFile, d.sourceID)
	if err != nil {
		return fmt.Errorf("state: %w", err)
	}
	d.state = st

	switch {
	case d.source.IsFile():
		return d.runFile(ctx)
	case d.source.IsJournal():
		return d.runJournal(ctx)
	default:
		return errors.New("dispatcher: no usable log source")
	}
}

// validate sanity-checks Config before Run does any I/O.
func (c *Config) validate() error {
	if c.Target == "" {
		return errors.New("Target is required")
	}
	if c.Token == "" {
		return errors.New("Token is required")
	}
	u, err := url.Parse(c.Target)
	if err != nil {
		return fmt.Errorf("parse Target: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("Target must be http or https, got %q", u.Scheme)
	}
	return nil
}

// resolveIdentity fills in d.cluster and d.nodeID using (in order):
// CLI overrides, asinfo, and finally log-scan. If all three fail
// it returns an error — without an identity we can't label rows.
func (d *Dispatcher) resolveIdentity(ctx context.Context) error {
	d.cluster = d.cfg.ClusterName
	d.nodeID = d.cfg.NodeID
	if d.cluster != "" && d.nodeID != "" {
		return nil
	}
	// Try asinfo. Fast path; deterministic when service is up.
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if dialableTCP(tctx, "127.0.0.1:3000", time.Second) {
		nid, cn, err := asinfoNodeAndCluster(tctx)
		if err == nil {
			if d.nodeID == "" {
				d.nodeID = nid
			}
			if d.cluster == "" && cn != "" {
				d.cluster = cn
			}
		}
	}
	if d.nodeID != "" && d.cluster != "" {
		return nil
	}
	// Log-scan fallback: tail the file (if file source) for up to
	// ~30s waiting for the NODE-ID/CLUSTER-NAME ticker line. Skipped
	// for journal sources since journalctl --output=cat is the same
	// stream the dispatcher will be reading anyway and we don't want
	// to consume it twice.
	if d.source.IsFile() && (d.nodeID == "" || d.cluster == "") {
		nid, cn, err := scanLogForIdentity(ctx, d.source.File, 30*time.Second)
		if err == nil {
			if d.nodeID == "" {
				d.nodeID = nid
			}
			if d.cluster == "" {
				d.cluster = cn
			}
		}
	}
	if d.nodeID == "" {
		return errors.New("could not auto-detect node-id; pass --node-id")
	}
	if d.cluster == "" {
		// Cluster name is technically optional in Aerospike; fall
		// back to a sentinel so the AGI listener still has a
		// non-empty label.
		d.cluster = "null"
	}
	return nil
}

func (d *Dispatcher) runFile(ctx context.Context) error {
	startOffset := int64(0)
	startInode := uint64(0)
	snap := d.state.Snapshot()
	if snap.FileOffset > 0 {
		startOffset = snap.FileOffset
		startInode = snap.FileInode
	} else if !d.cfg.BackfillFromStart {
		// First run, no state, no backfill: open at EOF so we don't
		// re-process whatever the file already has.
		if size, err := fileSize(d.source.File); err == nil {
			startOffset = size
		}
	}
	tail := newFileTail(d.source.File, startOffset, startInode)
	go func() {
		if err := tail.run(ctx); err != nil {
			d.logger.Printf("dispatcher: file tail exited: %s", err)
		}
	}()
	return d.streamLoop(ctx, tail.Lines(), func(after int64, inode uint64) {
		d.state.UpdateFile(after, inode)
	})
}

func (d *Dispatcher) runJournal(ctx context.Context) error {
	snap := d.state.Snapshot()
	tail := newJournalTail(d.source.Journal, snap.JournalCursor)
	go func() {
		if err := tail.run(ctx); err != nil {
			d.logger.Printf("dispatcher: journal tail exited: %s", err)
		}
	}()
	return d.streamLoop(ctx, tail.Lines(), func(_ int64, _ uint64) {
		// Journal cursor isn't surfaced by `journalctl --output=cat`
		// directly; for now we update only the wall-clock so a
		// restart re-attaches via -fn0.
		d.state.UpdateJournal("")
	})
}

// streamLoop consumes lines from in and POSTs them as a chunked HTTP
// stream to the AGI listener, reconnecting with exponential backoff
// on transport errors. On every successful row send it invokes
// onProgress so the caller can update its checkpoint state.
func (d *Dispatcher) streamLoop(ctx context.Context, in <-chan tailLine, onProgress func(after int64, inode uint64)) error {
	const (
		minBackoff = time.Second
		maxBackoff = 30 * time.Second
	)
	flushTicker := time.NewTicker(time.Second)
	defer flushTicker.Stop()
	go d.stateFlusher(ctx, flushTicker.C)

	backoff := minBackoff
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		err := d.streamOnce(ctx, in, onProgress)
		if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
			return nil
		}
		d.logger.Printf("dispatcher: stream error, retry in %s: %s", backoff, err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// streamOnce opens a single chunked POST to the AGI listener and
// streams lines from in into its body. It returns when:
//   - in is closed (caller should treat that as a normal shutdown), or
//   - the HTTP transaction fails (caller will retry with backoff).
func (d *Dispatcher) streamOnce(ctx context.Context, in <-chan tailLine, onProgress func(after int64, inode uint64)) error {
	target, err := d.buildURL()
	if err != nil {
		return err
	}
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Transfer-Encoding", "chunked")

	// Pump lines from in to the request body in a goroutine. Close
	// the pipe writer on EOF / cancel so the HTTP client returns.
	var (
		pumpErr error
		pumpWG  sync.WaitGroup
	)
	pumpWG.Add(1)
	go func() {
		defer pumpWG.Done()
		defer pw.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case ln, ok := <-in:
				if !ok {
					return
				}
				if _, err := pw.Write(ln.Line); err != nil {
					pumpErr = err
					return
				}
				if _, err := pw.Write([]byte{'\n'}); err != nil {
					pumpErr = err
					return
				}
				onProgress(ln.After, ln.Inode)
			}
		}
	}()

	resp, err := d.hc.Do(req)
	if err != nil {
		_ = pr.CloseWithError(err)
		pumpWG.Wait()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = pr.CloseWithError(fmt.Errorf("status %d", resp.StatusCode))
		pumpWG.Wait()
		return fmt.Errorf("AGI returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	// Drain the response body so the connection can be reused. The
	// listener returns 200 only when the request body is fully
	// consumed (i.e. when the dispatcher closes pw).
	_, _ = io.Copy(io.Discard, resp.Body)
	pumpWG.Wait()
	return pumpErr
}

// stateFlusher periodically writes the in-memory dispatcher state
// to disk, so a restart resumes from approximately the last second
// of progress (the AGI listener is responsible for de-duping at the
// row level via stable nodePrefix labels).
func (d *Dispatcher) stateFlusher(ctx context.Context, tk <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			_ = d.state.Flush()
			return
		case <-tk:
			if err := d.state.Flush(); err != nil {
				d.logger.Printf("dispatcher: state flush: %s", err)
			}
		}
	}
}

func (d *Dispatcher) buildURL() (string, error) {
	u, err := url.Parse(d.cfg.Target)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "/agi/ingest/stream")
	q := u.Query()
	q.Set("cluster", d.cluster)
	q.Set("node", d.nodeID)
	q.Set("source", strings.TrimPrefix(d.source.Label(), "file:"))
	if d.source.IsJournal() {
		q.Set("source", "journal:"+d.source.Journal)
	}
	q.Set("source-id", d.sourceID)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// computeSourceID hashes the (cluster,node,source) tuple to a stable
// short string. The AGI listener uses this to bind reconnects to the
// same per-stream goroutine so node-prefix allocation stays sticky.
func computeSourceID(cluster, node string, src Source) string {
	h := sha1.New()
	_, _ = io.WriteString(h, cluster)
	_, _ = io.WriteString(h, "|")
	_, _ = io.WriteString(h, node)
	_, _ = io.WriteString(h, "|")
	_, _ = io.WriteString(h, src.Label())
	return hex.EncodeToString(h.Sum(nil))
}

// fileSize returns the size of the file at path, or 0+err.
func fileSize(p string) (int64, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// AerospikeConfDefault is the standard install path on every Aerospike
// distribution. Exposed so the CLI default flag value can reference
// the same constant without re-typing the path.
const AerospikeConfDefault = "/etc/aerospike/aerospike.conf"

// EnsureStateDir creates the parent directory of the state file with
// 0750 perms. Used by the CLI before Run so the dispatcher itself
// doesn't have to worry about a nonexistent /var/lib/aerolab path.
func EnsureStateDir(stateFile string) error {
	if stateFile == "" {
		return nil
	}
	return os.MkdirAll(filepath.Dir(stateFile), 0o750)
}
