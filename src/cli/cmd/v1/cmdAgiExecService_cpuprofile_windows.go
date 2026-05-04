//go:build !noagi && windows

package cmd

import (
	"github.com/aerospike/aerolab/pkg/agi/plugin"
)

// installCPUProfileRotateHandler is a no-op on Windows because
// SIGUSR1 (the trigger used on unix to rotate the plugin CPU
// profile in-place) does not exist there. Operators on Windows
// must restart the service to obtain a fully-formed pprof file.
//
// The returned cleanup function is intentionally empty so the
// caller's `defer` site is identical across platforms.
func installCPUProfileRotateHandler(_ *plugin.Plugin, _ *System) func() {
	return func() {}
}
