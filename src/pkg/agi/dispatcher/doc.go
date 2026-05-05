// Package dispatcher implements the aerolab-side live log streaming
// dispatcher: it tails an Aerospike server's logs (file or journald)
// on a cluster node and forwards lines, line-delimited and unmodified,
// over a chunked HTTP POST to an AGI instance's
// /agi/ingest/stream endpoint.
//
// The dispatcher is fully self-contained and pulls in NO CLI / cobra
// deps so it can be unit-tested in isolation. It auto-discovers:
//
//   - The log destination (file path vs. journald unit) from
//     aerospike.conf, using the same `logging` stanza walk pattern
//     used elsewhere in aerolab. Manual overrides (--source-file,
//     --source-journal) are honoured.
//   - The cluster name and node-id, via asinfo over 127.0.0.1:3000
//     when reachable, falling back to a tail-and-wait scan for the
//     first NODE-ID/CLUSTER-NAME ticker line in the log itself, and
//     finally to explicit CLI flags.
//
// On the wire each request is a chunked HTTP/1.1 POST that streams
// raw log lines (NDJSON-compatible, but text/plain). The dispatcher
// reconnects with exponential backoff (1s..30s) on any transport
// error and persists the last successfully posted byte offset to
// the state file so it can resume after restarts.
//
// File tails handle log rotation by inode change: when the inode of
// the configured path changes, the dispatcher drains the old file
// descriptor to EOF, reopens the new file, and resets the offset.
//
// Journald tails are implemented by exec'ing
// `journalctl -fn0 -u <unit> --output=cat` and reading its stdout.
// (Pure-Go sdjournal would be cleaner but pulls a CGo dep we don't
// otherwise need; the journalctl path keeps the binary fully static.)
package dispatcher
