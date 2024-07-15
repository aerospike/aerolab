//go:build windows
// +build windows

package main

import (
	"github.com/aerospike/aerolab/ingest"
)

func getAgiStatus(bool, string) (*ingest.IngestStatusStruct, error) {
	return nil, nil
}
