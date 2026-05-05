package livelisten

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/ingest"
)

// Config holds the listener's runtime knobs. Constructed by the
// merged service from ingest.Config.Live + a few fixed paths.
type Config struct {
	// ListenAddr is the loopback bind. Default 127.0.0.1:18080.
	// External traffic reaches it via the proxy's reverse proxy
	// (cmdAgiExecProxy.go); binding to anything other than
	// loopback would expose the unauthenticated endpoint.
	ListenAddr string

	// OffsetsPath is the on-disk checkpoint file for per-stream
	// last-acked byte offsets. Refreshed every ~1s while a
	// stream is active; consumed by dispatcher on reconnect to
	// resume tailing from the right offset after AGI restarts.
	OffsetsPath string

	// TokensPath is the directory of bearer tokens that this
	// listener will accept on the Authorization header.
	// Matches the cmdAgiExecProxy.go token-watch directory; the
	// dispatcher uses the same token (typically
	// /opt/agi/tokens/dispatcher).
	TokensPath string

	// MaxStreams caps in-flight streams. New requests beyond
	// the cap return HTTP 429.
	MaxStreams int

	// Workers is the live worker pool size. Drains the shared
	// live results channel through the same row-stamp / putBatch
	// code path as the batch pipeline.
	Workers int

	// IdleTimeout is the maximum time a stream may sit without
	// receiving a line before the listener tears it down. Set
	// to zero to disable. Default 5m.
	IdleTimeout time.Duration

	// SourceCount, when set, lets the listener publish the
	// current active-stream count for observability (e.g. a
	// "live N streams" indicator in the sources label). Called
	// from a low-frequency goroutine; cheap implementations
	// only.
	SourceCount func(int)
}

// Listener owns the HTTP server, the token loader, the offsets
// checkpoint writer, and the per-Ingest live worker pool. Construct
// via New and drive via Serve / Shutdown. Listener is safe for
// concurrent use; New / Serve / Shutdown each run at most once.
type Listener struct {
	cfg     Config
	ingest  *ingest.Ingest
	shards  *ingest.MetaShards
	tokens  *tokenStore
	offsets *offsetStore
	srv     *http.Server

	// active counts in-flight streams. Atomic so the handler
	// can probe and bump it without taking a mutex on every
	// request.
	active int64

	startOnce sync.Once
	stopOnce  sync.Once
	startErr  error

	// retainedBatcher reflects whether PutBatcherRetain
	// succeeded for this listener; if true Shutdown must
	// release the retain before the underlying batcher can
	// drain.
	retainedBatcher bool
}

// New builds a Listener. cfg defaults are applied here so callers
// can pass a sparse Config.
func New(i *ingest.Ingest, cfg Config) *Listener {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:18080"
	}
	if cfg.MaxStreams <= 0 {
		cfg.MaxStreams = 256
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 16
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}
	if cfg.OffsetsPath == "" {
		cfg.OffsetsPath = "/opt/agi/live/offsets.json"
	}
	if cfg.TokensPath == "" {
		cfg.TokensPath = "/opt/agi/tokens"
	}
	return &Listener{
		cfg:     cfg,
		ingest:  i,
		shards:  i.NewLiveMetaShards(),
		tokens:  newTokenStore(cfg.TokensPath),
		offsets: newOffsetStore(cfg.OffsetsPath),
	}
}

// Handler returns the http.Handler used both by Serve (loopback
// listener) and by callers that wish to mount the route into an
// existing mux (e.g. tests).
func (l *Listener) Handler() http.HandlerFunc { return l.handle }

// Serve runs the loopback HTTP server and blocks until ctx is
// cancelled or the underlying http.Server returns. Returns
// http.ErrServerClosed on clean shutdown — callers commonly
// errors.Is-check that and treat it as "fine".
func (l *Listener) Serve(ctx context.Context) error {
	l.startOnce.Do(func() {
		// Pin the putBatcher across the listener lifetime so
		// the batch path's deferred Close cannot tear down
		// the batcher while we still have streams writing
		// rows.
		if !l.ingest.PutBatcherRetain() {
			l.startErr = errors.New("livelisten: putBatcher already torn down; cannot start")
			return
		}
		l.retainedBatcher = true
		// Refuse to start without WAL: the dirty-marker
		// mechanism in pkg/agi/ingest/init.go wipes the DB
		// on next start when WAL=off, which is correct for
		// batch ingest (the source files repopulate it) but
		// not for live (the source lines are gone). The
		// service caller already log-warns on this; we
		// double-check here so the listener cannot be wired
		// into a non-WAL run by an over-eager test.
		if !l.ingest.EnableWAL() {
			l.startErr = errors.New("livelisten: ingest WAL is off; live mode requires EnableWAL=true")
			l.ingest.PutBatcherRelease()
			l.retainedBatcher = false
			return
		}
		if err := l.ingest.StartLiveWorkers(ctx, l.cfg.Workers); err != nil {
			l.startErr = err
			l.ingest.PutBatcherRelease()
			l.retainedBatcher = false
			return
		}
		// Start the offsets checkpoint flusher and the token
		// directory watcher. Both stop on ctx.Done().
		l.tokens.start(ctx)
		l.offsets.start(ctx)
		mux := http.NewServeMux()
		mux.HandleFunc("/agi/ingest/stream", l.handle)
		mux.HandleFunc("/agi/ingest/health", l.health)
		l.srv = &http.Server{
			Addr:    l.cfg.ListenAddr,
			Handler: mux,
			// Long-lived chunked POSTs: don't cap with
			// the default ReadTimeout. The handler
			// reads line-by-line and returns when the
			// client closes, ctx is cancelled, or the
			// per-stream idle timeout trips.
		}
	})
	if l.startErr != nil {
		return l.startErr
	}
	ln, err := net.Listen("tcp", l.cfg.ListenAddr)
	if err != nil {
		return err
	}
	// When ctx cancels, gracefully shut down the server. Use a
	// background ctx with a tight deadline for Shutdown so we
	// don't hang behind a stuck client.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = l.srv.Shutdown(shutdownCtx)
	}()
	err = l.srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown asks the listener to stop. Safe to call from any
// goroutine; releases the putBatcher refcount, closes the live
// worker pool, and gracefully drains any in-flight HTTP
// connections. Returns nil on clean shutdown.
func (l *Listener) Shutdown(ctx context.Context) error {
	var firstErr error
	l.stopOnce.Do(func() {
		if l.srv != nil {
			if err := l.srv.Shutdown(ctx); err != nil {
				firstErr = err
			}
		}
		// StopLiveWorkers closes the live results channel
		// and waits for the worker goroutines; this happens
		// AFTER the HTTP server has stopped accepting so no
		// new rows enter the channel after the close.
		l.ingest.StopLiveWorkers()
		if l.retainedBatcher {
			l.ingest.PutBatcherRelease()
			l.retainedBatcher = false
		}
		// Final offsets flush so dispatcher resume sees the
		// latest committed state.
		if err := l.offsets.flushNow(); err != nil && firstErr == nil {
			firstErr = err
		}
		log.Printf("INFO: livelisten: shutdown complete")
	})
	return firstErr
}

// activeCount is the listener's view of in-flight streams. Used by
// the SourceCount callback (if any) and by the MaxStreams gate.
func (l *Listener) activeCount() int { return int(atomic.LoadInt64(&l.active)) }

// publishCount notifies the SourceCount callback of the current
// active count. Best-effort; fire and forget.
func (l *Listener) publishCount() {
	if l.cfg.SourceCount == nil {
		return
	}
	go l.cfg.SourceCount(l.activeCount())
}

// SafeOffsetsPath returns the absolute, cleaned offsets file path.
// Pure utility for external callers (e.g. tests) that want to read
// the checkpoint.
func (l *Listener) SafeOffsetsPath() string {
	return filepath.Clean(l.cfg.OffsetsPath)
}
