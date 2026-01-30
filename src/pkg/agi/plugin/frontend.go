package plugin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"log"
)

func (p *Plugin) Listen() error {
	log.Printf("DEBUG: Listener: setup")
	p.srv = &http.Server{Addr: p.config.Service.ListenAddress + ":" + strconv.Itoa(p.config.Service.ListenPort)}
	http.HandleFunc("/shutdown", p.handleShutdown)
	http.HandleFunc("/metrics", p.handleMetrics)
	http.HandleFunc("/metric-payload-options", p.handleMetricPayloadOptions)
	http.HandleFunc("/query", p.handleQuery)
	http.HandleFunc("/variable", p.handleVariable)
	http.HandleFunc("/tag-keys", p.handleTagKeys)
	http.HandleFunc("/tag-values", p.handleTagValues)
	http.HandleFunc("/histogram", p.handleHistogram)
	http.HandleFunc("/", p.handlePing)
	log.Printf("INFO: Listener: start")
	if err := p.srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (p *Plugin) handlePing(w http.ResponseWriter, r *http.Request) {
	log.Printf("INFO: Listener: received ping from %s", r.RemoteAddr)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (p *Plugin) handleShutdown(w http.ResponseWriter, r *http.Request) {
	log.Printf("INFO: Listener: shutdown request from %s", r.RemoteAddr)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Shutting down..."))
	go func() {
		timeout := p.config.Aerospike.Timeouts.QueryTotal
		if timeout == 0 {
			timeout = 60 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := p.srv.Shutdown(ctx); err != nil {
			log.Printf("DEBUG: Graceful Server Shutdown Failed, Forcing shutdown: %s", err)
			p.srv.Close()
		}
	}()
}
