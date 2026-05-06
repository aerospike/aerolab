package dispatcher

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config controls live log dispatch from an Aerospike node to AGI.
type Config struct {
	Target            string
	Token             string
	ClusterName       string
	NodeID            string
	AerospikeConf     string
	SourceFile        string
	SourceJournal     string
	StateFile         string
	InsecureTLS       bool
	BackfillFromStart bool
}

// Dispatcher tails one discovered log source and posts it to AGI.
type Dispatcher struct {
	cfg Config
}

// New creates a Dispatcher.
func New(cfg Config) *Dispatcher {
	if cfg.AerospikeConf == "" {
		cfg.AerospikeConf = "/etc/aerospike/aerospike.conf"
	}
	if cfg.StateFile == "" {
		cfg.StateFile = "/var/lib/aerolab/agi-dispatch.state"
	}
	return &Dispatcher{cfg: cfg}
}

// Run discovers node metadata and streams log lines until ctx is canceled.
func (d *Dispatcher) Run(ctx context.Context) error {
	if d.cfg.Target == "" {
		return fmt.Errorf("target is required")
	}
	if d.cfg.Token == "" {
		return fmt.Errorf("token is required")
	}
	source, err := d.resolveSource()
	if err != nil {
		return err
	}
	cluster, node, err := d.resolveIdentity(source)
	if err != nil {
		return err
	}
	state, err := loadState(d.cfg.StateFile)
	if err != nil {
		return err
	}
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		err = d.streamOnce(ctx, source, cluster, node, state)
		if err == nil {
			backoff = time.Second
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("WARN: agi dispatch stream failed: %s; reconnecting in %s", err, backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

func (d *Dispatcher) streamOnce(ctx context.Context, source Source, cluster, node string, state *State) error {
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	tailErr := make(chan error, 1)
	go func() {
		tailErr <- d.tailToWriter(ctx, source, state, pw)
	}()

	reqURL, err := d.streamURL(source, cluster, node)
	if err != nil {
		_ = pw.CloseWithError(err)
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, pr)
	if err != nil {
		_ = pw.CloseWithError(err)
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	req.Header.Set("Content-Type", "text/plain")
	if state.Offset > 0 {
		req.Header.Set("X-Resume-Offset", fmt.Sprintf("%d", state.Offset))
	}
	client, err := d.httpClient()
	if err != nil {
		_ = pw.CloseWithError(err)
		return err
	}
	resp, err := client.Do(req)
	cancel()
	if err != nil {
		_ = pw.CloseWithError(err)
		<-tailErr
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	terr := <-tailErr
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("live ingest returned %s", resp.Status)
	}
	return terr
}

func (d *Dispatcher) tailToWriter(ctx context.Context, source Source, state *State, pw *io.PipeWriter) error {
	defer pw.Close()
	writeLine := func(line string, offset int64) error {
		if _, err := io.WriteString(pw, line+"\n"); err != nil {
			return err
		}
		state.Offset = offset
		return saveState(d.cfg.StateFile, state)
	}
	switch source.Kind {
	case SourceKindFile:
		offset := state.Offset
		if d.cfg.BackfillFromStart {
			offset = 0
		} else if offset == 0 {
			offset = -1
		}
		return tailFile(ctx, source.Path, offset, writeLine)
	case SourceKindJournal:
		return tailJournal(ctx, source.Unit, writeLine)
	default:
		return fmt.Errorf("unknown source kind %q", source.Kind)
	}
}

func (d *Dispatcher) streamURL(source Source, cluster, node string) (string, error) {
	u, err := url.Parse(d.cfg.Target)
	if err != nil {
		return "", err
	}
	u.Path = "/agi/ingest/stream"
	q := u.Query()
	q.Set("cluster", cluster)
	q.Set("node", node)
	q.Set("source", source.Name())
	q.Set("source-id", stableSourceID(cluster, node, source.Name()))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (d *Dispatcher) httpClient() (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if d.cfg.InsecureTLS {
		tlsCfg.InsecureSkipVerify = true
	} else if ca, err := os.ReadFile("/opt/aerolab/agi-ca.pem"); err == nil {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		pool.AppendCertsFromPEM(ca)
		tlsCfg.RootCAs = pool
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   0,
	}, nil
}

func stableSourceID(cluster, node, source string) string {
	sum := sha1.Sum([]byte(strings.Join([]string{cluster, node, source}, "\x00")))
	return hex.EncodeToString(sum[:])
}
