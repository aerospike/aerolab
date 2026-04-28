package ingest

import (
	"encoding/binary"
	"encoding/hex"

	"github.com/zeebo/xxh3"
)

// MetricsRowPKSeparator is the legacy three-part-key delimiter
// used inside the hash input. It is intentionally identical to
// the unhashed v2 separator so re-ingest of the same source data
// produces the same hash and therefore the same idempotent
// overwrite semantics ingest has always relied on.
const MetricsRowPKSeparator = "::/::"

// MetricsRowKey returns the deterministic XXH3-128 hash of the
// (cluster, node, log-line) tuple used as the metrics-set
// primary key. The components are joined by MetricsRowPKSeparator
// before hashing.
//
// XXH3-128 is non-cryptographic but provides a 128-bit output;
// the birthday-bound collision probability for 10^10 distinct
// keys (~1 TiB of typical Aerospike server logs at ~150 B/line)
// is ≈1.5e-19, several orders of magnitude below the host's
// other failure modes (uncorrected disk read errors, ECC RAM
// faults, etc.). 64-bit hashes are *not* sufficient at this
// scale; 128 bits gives roughly 2^64 birthday-safety headroom.
//
// Output is 32 lowercase hex characters (16 raw bytes encoded).
// We encode rather than using the raw bytes as the DB key so the
// key remains a printable Go string and so debug tools (logs,
// /debug/db/sample, --get-key) can echo it without escaping.
func MetricsRowKey(clusterName, nodeIdent, logLine string) string {
	var h xxh3.Hasher
	h.WriteString(clusterName) //nolint:errcheck // xxh3.Hasher.WriteString never fails
	h.WriteString(MetricsRowPKSeparator)
	h.WriteString(nodeIdent)
	h.WriteString(MetricsRowPKSeparator)
	h.WriteString(logLine)
	return encodeUint128Hex(h.Sum128())
}

// MetricsRowKeyFromCombined is the same hash computed from a
// pre-joined "<cluster><sep><node>" identifier and a log line.
// Equivalent to MetricsRowKey when the join uses
// MetricsRowPKSeparator; ingest's hot path takes this form
// because the combined string is already constructed once per
// log file.
func MetricsRowKeyFromCombined(nodeIdentifier, logLine string) string {
	var h xxh3.Hasher
	h.WriteString(nodeIdentifier) //nolint:errcheck
	h.WriteString(MetricsRowPKSeparator)
	h.WriteString(logLine)
	return encodeUint128Hex(h.Sum128())
}

// MetricsRowKeyFromString hashes an arbitrary pre-joined string
// in legacy form ("cluster::/::node::/::line"). Exposed for the
// `aerolab agi query --hash-key` debugging helper so operators
// can compute the same key ingest would have produced for a
// given combined input.
func MetricsRowKeyFromString(joined string) string {
	sum := xxh3.HashString128(joined)
	return encodeUint128Hex(sum)
}

// encodeUint128Hex renders an xxh3.Uint128 as a fixed-width 32-
// character lowercase hex string with the high half first.
// Big-endian byte order is chosen for legibility and so trailing
// bytes don't dominate visual diff in iterated debug output.
func encodeUint128Hex(u xxh3.Uint128) string {
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[0:8], u.Hi)
	binary.BigEndian.PutUint64(raw[8:16], u.Lo)
	return hex.EncodeToString(raw[:])
}
