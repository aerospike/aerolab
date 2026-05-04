//go:build !noagi && !windows

package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/aerospike/aerolab/pkg/agi/plugin"
)

// installCPUProfileRotateHandler wires SIGUSR1 to a goroutine that
// rotates the plugin's CPU profile on demand. Go's runtime/pprof
// buffers the entire profile in memory until StopCPUProfile is
// called, so the only way for an operator to obtain a populated
// cpu.plugin.pprof short of a full service stop is to ask the
// running process to rotate. Each rotation produces a timestamped,
// fully-formed pprof file next to the configured output path; the
// configured path itself always points at the live (in-memory,
// 0-byte on disk) profile. Repeatable: the goroutine drains the
// channel in a loop and only exits when the returned cleanup
// function is invoked. SIGUSR2 is intentionally left unbound so
// future "reload config" or similar can claim it without disturbing
// the rotate handler.
//
// Returns a cleanup function that the caller must defer.
func installCPUProfileRotateHandler(p *plugin.Plugin, system *System) func() {
	usrCh := make(chan os.Signal, 1)
	signal.Notify(usrCh, syscall.SIGUSR1)
	go func() {
		for range usrCh {
			if p == nil {
				continue
			}
			rotated, err := p.RotateCPUProfile()
			if err != nil {
				system.Logger.Warn("plugin CPU profile rotate failed: %s", err)
				continue
			}
			if rotated != "" {
				system.Logger.Info("plugin CPU profile rotated to %s", rotated)
			} else {
				system.Logger.Info("plugin CPU profile started (no prior profile to flush)")
			}
		}
	}()
	return func() {
		signal.Stop(usrCh)
		close(usrCh)
	}
}
