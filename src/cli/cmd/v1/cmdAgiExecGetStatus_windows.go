//go:build windows
// +build windows

package cmd

import (
	"github.com/aerospike/aerolab/pkg/agi/ingest"
)

// GetAgiStatus is a stub implementation for Windows.
// AGI functionality is not supported on Windows, so this returns an empty status.
//
// Parameters:
//   - enabled: Ignored on Windows
//   - ingestProgressPath: Ignored on Windows
//
// Returns:
//   - *ingest.IngestStatusStruct: Empty status struct
//   - error: Always nil
func GetAgiStatus(enabled bool, ingestProgressPath string) (*ingest.IngestStatusStruct, error) {
	return new(ingest.IngestStatusStruct), nil
}

