package livelisten

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/ingest"
)

// Config configures the live HTTP listener.
type Config struct {
	ListenAddr  string
	OffsetsPath string
	TokensPath  string
	MaxStreams  int
	Workers     int
}

// Listener accepts chunked POST bodies on /agi/ingest/stream.
type Listener struct {
	ing *ingest.Ingest
	cfg Config

	shards        *ingest.MetaShards
	results       chan *ingest.ProcessResult
	offsets       *offsetStore
	srv           *http.Server
	poolDone      chan struct{}
	shutdownOnce  sync.Once
	pipelineReady bool
	active        atomic.Int32
}

// New constructs a listener. Call Serve to bind the HTTP server and worker pool.
func New(i *ingest.Ingest, cfg Config) *Listener {
	if cfg.MaxStreams <= 0 {
		cfg.MaxStreams = 256
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 16
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:18080"
	}
	if cfg.OffsetsPath == "" {
		cfg.OffsetsPath = "/opt/agi/live/offsets.json"
	}
	return &Listener{ing: i, cfg: cfg, offsets: newOffsetStore(cfg.OffsetsPath)}
}

// Handler returns the HTTP handler for POST /agi/ingest/stream.
func (l *Listener) Handler() http.HandlerFunc {
	return l.handleStream
}

// Serve runs a dedicated loopback http.Server until ctx is cancelled.
func (l *Listener) Serve(ctx context.Context) error {
	meta, err := l.ing.LoadMetaEntriesFromLabelsDB()
	if err != nil {
		log.Printf("livelisten: load label meta: %v", err)
	}
	l.shards = ingest.NewMetaShards(meta)

	l.results = make(chan *ingest.ProcessResult, 128)
	l.ing.RetainPutBatcherHold()
	l.pipelineReady = true

	l.poolDone = make(chan struct{})
	go func() {
		l.ing.RunWorkerPool(l.results, l.cfg.Workers)
		close(l.poolDone)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/agi/ingest/stream", l.handleStream)

	l.srv = &http.Server{Addr: l.cfg.ListenAddr, Handler: mux}

	go func() {
		<-ctx.Done()
		shctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = l.srv.Shutdown(shctx)
	}()

	var serveErr error
	if err := l.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		serveErr = err
	}
	l.shutdownPipeline()
	if serveErr != nil {
		return serveErr
	}
	return nil
}

// Shutdown stops the listener and releases putBatcher holds.
func (l *Listener) Shutdown(ctx context.Context) error {
	if l.srv != nil {
		_ = l.srv.Shutdown(ctx)
	}
	l.shutdownPipeline()
	return nil
}

func (l *Listener) shutdownPipeline() {
	l.shutdownOnce.Do(func() {
		if l.results != nil {
			close(l.results)
			<-l.poolDone
			l.results = nil
		}
		if l.offsets != nil {
			l.offsets.Stop()
			l.offsets = nil
		}
		if l.pipelineReady && l.ing != nil {
			l.ing.ReleasePutBatcherHold()
			l.pipelineReady = false
		}
	})
}
