package plugin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/bestmethod/logger"
)

func (p *Plugin) Listen() error {
	logger.Debug("Listener: setup")
	p.srv = &http.Server{Addr: p.config.Service.ListenAddress + ":" + strconv.Itoa(p.config.Service.ListenPort)}
	http.HandleFunc("/shutdown", p.handleShutdown)
	logger.Debug("Listener: start")
	if err := p.srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (p *Plugin) handleShutdown(w http.ResponseWriter, r *http.Request) {
	logger.Debug("Listener: shutdown")
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
			logger.Debug("Graceful Server Shutdown Failed, Forcing shutdown: %s", err)
			p.srv.Close()
		}
	}()
}
