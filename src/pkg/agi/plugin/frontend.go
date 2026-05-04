package plugin

import (
	"context"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"log"
)

func (p *Plugin) Listen() error {
	log.Printf("DEBUG: Listener: setup")
	// Use a per-plugin mux instead of http.DefaultServeMux so that
	// (a) two plugins in the same process don't panic on duplicate
	// path registrations and (b) tests that wrap the mux in httptest
	// don't pollute the global mux for the rest of the process.
	p.mux = http.NewServeMux()
	// Every handler is wrapped in trackHandler so Close() can wait
	// for in-flight requests to finish before closing the db. The
	// shutdown/ping handlers don't touch the db but are wrapped too
	// for uniformity — their overhead is a single WaitGroup Add/Done.
	p.mux.HandleFunc("/shutdown", p.trackHandler(p.handleShutdown))
	p.mux.HandleFunc("/metrics", p.trackHandler(p.handleMetrics))
	p.mux.HandleFunc("/metric-payload-options", p.trackHandler(p.handleMetricPayloadOptions))
	p.mux.HandleFunc("/query", p.trackHandler(p.handleQuery))
	p.mux.HandleFunc("/variable", p.trackHandler(p.handleVariable))
	p.mux.HandleFunc("/tag-keys", p.trackHandler(p.handleTagKeys))
	p.mux.HandleFunc("/tag-values", p.trackHandler(p.handleTagValues))
	p.mux.HandleFunc("/histogram", p.trackHandler(p.handleHistogram))
	// Debug/inspection routes for operators (read-only, localhost-only
	// by virtue of the configured listen address). See frontend_debug.go.
	p.registerDebugHandlers()
	p.mux.HandleFunc("/", p.trackHandler(p.handlePing))
	p.srv = &http.Server{
		Addr:    p.config.Service.ListenAddress + ":" + strconv.Itoa(p.config.Service.ListenPort),
		Handler: p.mux,
	}
	log.Printf("INFO: Listener: start")
	if err := p.srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// trackHandler wraps an http.HandlerFunc so every in-flight request is
// counted on p.handlers. Close() waits on p.handlers AFTER srv.Shutdown
// returns — Shutdown stops accepting new connections but lets the
// handlers that were already dispatched run to completion, which is
// precisely the window where a handler can be inside p.db.Query(...).
// Without this, Close() can call p.db.Close() while a query iterator
// is still open and get ErrIteratorsOpen.
//
// trackHandler also installs a panic recover. Go's net/http already
// recovers panics inside handlers, but it does so AFTER the handler
// returns and only logs to stderr without request context. Recovering
// here means:
//   - p.handlers.Done() always fires (already true via defer, but the
//     intent is now explicit and the wrapper documents why).
//   - The panic is logged with method+URL+remote so we can correlate it
//     to a Grafana request.
//   - A 500 is written if no response has been sent yet (when
//     WriteHeader has been called this is a no-op, so it can't smash
//     valid responses).
//   - Most importantly, deferred iterator Close()s inside the handler
//     have already unwound by the time the panic reaches us, so the db
//     handle is back to a quiescent state and Plugin.Close() will not
//     hit ErrIteratorsOpen on a subsequent shutdown.
func (p *Plugin) trackHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p.handlers.Add(1)
		defer p.handlers.Done()
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("ERROR: handler panic (method:%s url:%s remote:%s): %v\n%s",
					r.Method, r.URL.Path, r.RemoteAddr, rec, debug.Stack())
				// http.Error is a no-op once WriteHeader has run; safe
				// to call unconditionally.
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		h(w, r)
	}
}

func (p *Plugin) handlePing(w http.ResponseWriter, r *http.Request) {
	log.Printf("INFO: Listener: received ping from %s", r.RemoteAddr)
	w.WriteHeader(http.StatusOK)
	//nolint:errcheck
	w.Write([]byte("OK"))
}

func (p *Plugin) handleShutdown(w http.ResponseWriter, r *http.Request) {
	log.Printf("INFO: Listener: shutdown request from %s", r.RemoteAddr)
	w.WriteHeader(http.StatusOK)
	//nolint:errcheck
	w.Write([]byte("Shutting down..."))
	go p.Shutdown()
}

// Shutdown triggers a graceful shutdown of the plugin's HTTP server.
// It is safe to call multiple times and from any goroutine (e.g. from
// a SIGTERM handler in cmdAgiExecService). Shutdown waits for in-flight
// requests up to Config.DB.ShutdownTimeout before force-closing.
// Shutdown does NOT close the underlying db handle; call Plugin.Close
// for that.
func (p *Plugin) Shutdown() {
	if p.srv == nil {
		return
	}
	timeout := p.config.DB.ShutdownTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := p.srv.Shutdown(ctx); err != nil {
		log.Printf("DEBUG: Graceful Server Shutdown Failed, Forcing shutdown: %s", err)
		p.srv.Close()
	}
}
