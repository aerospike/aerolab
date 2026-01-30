// Package agi provides shared constants and utilities for the Aerospike Grafana Integration.
package agi

import (
	_ "embed"
	"strconv"
	"strings"
)

// agiVersionStr is the embedded version string from agi.version.txt
//
//go:embed agi.version.txt
var agiVersionStr string

// AGIVersion returns the current version of the AGI implementation.
// This version number is the ONLY trigger for automatic new template creation.
// When AGI implementation changes require a new template (new software versions,
// directory structure changes, service configuration changes, etc.), increment this value
// using scripts/new-agi-version.sh.
//
// Note: Changes to aerospike version, grafana version, or aerolab version do NOT
// trigger automatic template recreation. Only changes to this AGIVersion will.
//
// Version History:
//   - 1: Initial AGI template implementation with template-based approach
var AGIVersion = parseAGIVersion()

func parseAGIVersion() int {
	v, err := strconv.Atoi(strings.TrimSpace(agiVersionStr))
	if err != nil {
		return 1 // fallback to version 1
	}
	return v
}

// Ingest Event Constants - preserved for compatibility with existing implementations
const (
	AgiEventInitComplete       = "INGEST_STEP_INIT_COMPLETE"
	AgiEventDownloadComplete   = "INGEST_STEP_DOWNLOAD_COMPLETE"
	AgiEventUnpackComplete     = "INGEST_STEP_UNPACK_COMPLETE"
	AgiEventPreProcessComplete = "INGEST_STEP_PREPROCESS_COMPLETE"
	AgiEventProcessComplete    = "INGEST_STEP_PROCESS_COMPLETE"
	AgiEventIngestFinish       = "INGEST_FINISHED"
	AgiEventServiceDown        = "SERVICE_DOWN"
	AgiEventServiceUp          = "SERVICE_UP"
	AgiEventMaxAge             = "MAX_AGE_REACHED"
	AgiEventMaxInactive        = "MAX_INACTIVITY_REACHED"
	AgiEventSpotNoCapacity     = "SPOT_INSTANCE_CAPACITY_SHUTDOWN"
	AgiEventResourceMonitor    = "SYS_RESOURCE_USAGE_MONITOR"
)
