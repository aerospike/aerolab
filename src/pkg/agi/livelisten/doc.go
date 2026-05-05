// Package livelisten exposes the loopback HTTP listener that accepts live
// Aerospike log lines from aerolab agi exec dispatch and forwards them into
// the shared ingest putBatcher via pkg/agi/ingest.
package livelisten
