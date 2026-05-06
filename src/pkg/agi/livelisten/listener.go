package livelisten

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/aerospike/aerolab/pkg/agi/ingest"
)

const (
	defaultListenAddr = "127.0.0.1:18080"
	defaultMaxStreams = 256
	defaultWorkers    = 16
)

// Config controls the live ingest listener.
type Config struct {
	ListenAddr  string
	OffsetsPath string
	TokensPath  string
	MaxStreams  int
	Workers     int
}

// Listener owns the live ingest HTTP server and the worker pool that drains
// parsed rows into ingest.
type Listener struct {
	i      *ingest.Ingest
	cfg    Config
	server *http.Server

	mu            sync.Mutex
	activeStreams map[string]struct{}
	workers       *ingest.LiveWorkers
	startErr      error
	startOnce     sync.Once
	shutdownOnce  sync.Once
	offsets       *offsetStore
}

// New creates a live ingest listener with safe defaults.
func New(i *ingest.Ingest, cfg Config) *Listener {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
	}
	if cfg.MaxStreams <= 0 {
		cfg.MaxStreams = defaultMaxStreams
	}
	if cfg.Workers <= 0 {
		cfg.Workers = defaultWorkers
	}
	l := &Listener{
		i:             i,
		cfg:           cfg,
		activeStreams: make(map[string]struct{}),
		offsets:       newOffsetStore(cfg.OffsetsPath),
	}
	l.server = &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: http.HandlerFunc(l.Handler()),
	}
	return l
}

// Handler returns the HTTP handler for POST /agi/ingest/stream.
func (l *Listener) Handler() http.HandlerFunc {
	return l.handleStream
}

// Serve starts the loopback HTTP listener and blocks until the server exits or
// the context is canceled.
func (l *Listener) Serve(ctx context.Context) error {
	if err := l.ensureStarted(); err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = l.Shutdown(context.Background())
	}()
	err := l.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops the HTTP server, closes live workers, and persists offsets.
func (l *Listener) Shutdown(ctx context.Context) error {
	var err error
	l.shutdownOnce.Do(func() {
		if l.server != nil {
			err = l.server.Shutdown(ctx)
		}
		if l.workers != nil {
			l.workers.Close()
		}
		if l.offsets != nil {
			if serr := l.offsets.Save(); err == nil {
				err = serr
			}
		}
	})
	return err
}

func (l *Listener) ensureStarted() error {
	l.startOnce.Do(func() {
		if l.i == nil {
			l.startErr = errors.New("livelisten: ingest is required")
			return
		}
		l.startErr = l.offsets.Load()
		if l.startErr != nil {
			return
		}
		l.workers, l.startErr = l.i.StartLiveWorkers(l.cfg.Workers)
	})
	return l.startErr
}
