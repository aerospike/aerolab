package dispatcher

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	aeroconf "github.com/rglonek/aerospike-config-file-parser"
)

// Config configures the log dispatcher client.
type Config struct {
	Target              string
	Token               string
	ClusterName         string
	NodeID              string
	AerospikeConf       string
	SourceFile          string
	SourceJournal       string
	StateFile           string
	InsecureTLS         bool
	BackfillFromStart   bool
	HTTPIdleConnTimeout time.Duration
}

// Dispatcher streams logs to AGI.
type Dispatcher struct {
	cfg Config
}

// New creates a dispatcher.
func New(cfg Config) *Dispatcher {
	if cfg.HTTPIdleConnTimeout <= 0 {
		cfg.HTTPIdleConnTimeout = 90 * time.Second
	}
	if cfg.AerospikeConf == "" {
		cfg.AerospikeConf = "/etc/aerospike/aerospike.conf"
	}
	return &Dispatcher{cfg: cfg}
}

type stateFile struct {
	Offset int64  `json:"offset"`
	Cursor string `json:"cursor,omitempty"`
}

func (d *Dispatcher) loadState() stateFile {
	b, err := os.ReadFile(d.cfg.StateFile)
	if err != nil {
		return stateFile{}
	}
	var s stateFile
	_ = json.Unmarshal(b, &s)
	return s
}

func (d *Dispatcher) saveState(s stateFile) {
	_ = os.MkdirAll(filepath.Dir(d.cfg.StateFile), 0755)
	b, _ := json.Marshal(s)
	tmp := d.cfg.StateFile + ".tmp"
	_ = os.WriteFile(tmp, b, 0644)
	_ = os.Rename(tmp, d.cfg.StateFile)
}

// Run blocks until ctx is cancelled, tailing the configured source.
func (d *Dispatcher) Run(ctx context.Context) error {
	if d.cfg.Target == "" {
		return errors.New("dispatcher: Target is required")
	}
	if d.cfg.Token == "" {
		return errors.New("dispatcher: Token is required")
	}

	cluster := d.cfg.ClusterName
	node := d.cfg.NodeID
	if cluster == "" || node == "" {
		cn, nd, err := queryAsinfoNodeCluster("127.0.0.1:3000")
		if err == nil {
			if cluster == "" {
				cluster = cn
			}
			if node == "" {
				node = nd
			}
		}
	}
	if cluster == "" || node == "" {
		return fmt.Errorf("dispatcher: could not determine cluster/node (asinfo failed); set ClusterName and NodeID")
	}

	sourceID := hashString(d.cfg.SourceFile + "|" + d.cfg.SourceJournal + "|" + cluster + "|" + node)

	var (
		filePath  string
		journalU  string
	)
	if d.cfg.SourceFile != "" {
		filePath = d.cfg.SourceFile
	} else if d.cfg.SourceJournal != "" {
		journalU = d.cfg.SourceJournal
	} else {
		kind, path, err := discoverLogDestination(d.cfg.AerospikeConf)
		if err != nil {
			return err
		}
		if kind == "file" {
			filePath = path
		} else {
			journalU = path
		}
	}

	st := d.loadState()
	if d.cfg.BackfillFromStart {
		st.Offset = 0
	}

	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: d.cfg.InsecureTLS}}
	cli := &http.Client{Transport: tr, Timeout: 0}

	if filePath != "" {
		return d.runFile(ctx, cli, cluster, node, sourceID, filePath, st)
	}
	return d.runJournal(ctx, cli, cluster, node, sourceID, journalU, st)
}

func (d *Dispatcher) runFile(ctx context.Context, cli *http.Client, cluster, node, sourceID, path string, st stateFile) error {
	offset := st.Offset
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		cur := offset
		err := d.postStream(ctx, cli, cluster, node, sourceID, filepath.Base(path), cur, func(w io.Writer) error {
			return tailFileToWriter(ctx, path, cur, w, &offset)
		})
		if errors.Is(err, context.Canceled) {
			return err
		}
		if err != nil {
			time.Sleep(backoffSleep(attempt))
			attempt++
			continue
		}
		attempt = 0
		st.Offset = offset
		d.saveState(st)
	}
}

func (d *Dispatcher) runJournal(ctx context.Context, cli *http.Client, cluster, node, sourceID, unit string, st stateFile) error {
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := d.postStream(ctx, cli, cluster, node, sourceID, "journal", 0, func(w io.Writer) error {
			return journalFollow(ctx, unit, st.Cursor, w, &st.Cursor)
		})
		if errors.Is(err, context.Canceled) {
			return err
		}
		if err != nil {
			time.Sleep(backoffSleep(attempt))
			attempt++
			continue
		}
		attempt = 0
		d.saveState(st)
	}
}

func backoffSleep(attempt int) time.Duration {
	d := time.Second * time.Duration(1<<min(attempt, 5))
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (d *Dispatcher) postStream(ctx context.Context, cli *http.Client, cluster, node, sourceID, source string, resume int64, body func(io.Writer) error) error {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_ = body(pw)
	}()

	u, err := url.Parse(strings.TrimSuffix(d.cfg.Target, "/") + "/agi/ingest/stream")
	if err != nil {
		return err
	}
	q := url.Values{}
	q.Set("cluster", cluster)
	q.Set("node", node)
	q.Set("source", source)
	q.Set("source-id", sourceID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), pr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	if resume > 0 {
		req.Header.Set("X-Resume-Offset", strconv.FormatInt(resume, 10))
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("dispatcher: HTTP %d: %s", resp.StatusCode, string(bytes.TrimSpace(slurp)))
	}
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

func discoverLogDestination(confPath string) (kind string, path string, err error) {
	b, err := os.ReadFile(confPath)
	if err != nil {
		return "", "", fmt.Errorf("read aerospike conf: %w", err)
	}
	cfg, err := aeroconf.Parse(bytes.NewReader(b))
	if err != nil {
		return "", "", fmt.Errorf("parse aerospike conf: %w", err)
	}
	if cfg.Type("logging") != aeroconf.ValueStanza {
		return "file", "/var/log/aerospike/aerospike.log", nil
	}
	keys := cfg.Stanza("logging").ListKeys()
	var filePath string
	var console bool
	for _, k := range keys {
		if strings.HasPrefix(k, "file ") {
			filePath = strings.TrimSpace(strings.TrimPrefix(k, "file "))
		}
		if strings.HasPrefix(k, "console") {
			console = true
		}
	}
	if filePath != "" {
		return "file", filePath, nil
	}
	if console {
		return "journal", "aerospike.service", nil
	}
	return "file", "/var/log/aerospike/aerospike.log", nil
}

func tailFileToWriter(ctx context.Context, path string, start int64, w io.Writer, offsetOut *int64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if start > 0 {
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return err
		}
	}
	buf := make([]byte, 32*1024)
	pos := start
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			pos += int64(n)
			if offsetOut != nil {
				*offsetOut = pos
			}
		}
		if err == io.EOF {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
	}
}

func journalFollow(ctx context.Context, unit, cursor string, w io.Writer, cursorOut *string) error {
	args := []string{"journalctl", "-fn0", "-u", unit, "--output=cat"}
	if cursor != "" {
		args = append(args, "--cursor="+cursor)
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = w
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func hashString(s string) string {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return fmt.Sprintf("%016x", h)
}

func queryAsinfoNodeCluster(hostport string) (cluster, node string, err error) {
	host := "127.0.0.1"
	port := "3000"
	if parts := strings.Split(hostport, ":"); len(parts) >= 1 && parts[0] != "" {
		host = parts[0]
	}
	if parts := strings.Split(hostport, ":"); len(parts) >= 2 && parts[1] != "" {
		port = parts[1]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	nodeOut, err := exec.CommandContext(ctx, "asinfo", "-h", host, "-p", port, "-v", "node").Output()
	if err != nil {
		return "", "", err
	}
	clusterOut, err := exec.CommandContext(ctx, "asinfo", "-h", host, "-p", port, "-v", "cluster-name").Output()
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(clusterOut)), strings.TrimSpace(string(nodeOut)), nil
}
