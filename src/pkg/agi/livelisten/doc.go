// Package livelisten implements the in-process HTTP listener for AGI's
// live log streaming path. It accepts chunked POSTs from the
// aerolab agi-dispatch sidecar (see pkg/agi/dispatcher) and streams
// each log line through pkg/agi/ingest's live API into the same
// putBatcher / Pebble pipeline the batch ingest path uses.
//
// Wire shape:
//
//   POST /agi/ingest/stream
//     ?cluster=<name>&node=<id>&source=<file>&source-id=<sha1>
//     Authorization: Bearer <token>
//     X-Resume-Offset: <byte>     (optional, advisory)
//     Content-Type: text/plain    (NDJSON or newline-delimited)
//     Body: log lines, one per record
//     Trailer: X-Last-Offset      (final, written by dispatcher)
//
// Per request the handler:
//   - Validates the bearer token against TokensPath (matches the
//     proxy's existing token-watch loop).
//   - Looks up / allocates a stable nodePrefix via
//     ingest.AllocLiveStream.
//   - Spawns a per-connection ingest.LiveStream and feeds the body
//     through it line-by-line.
//   - Periodically flushes a per-stream offset checkpoint to
//     OffsetsPath so dispatcher resume works across AGI restarts.
//   - On EOF / context cancel calls LiveStream.Close to drain any
//     parser tail state and writes the final X-Last-Offset trailer.
//
// The listener and the proxy reverse-proxy that fronts it use a
// loopback HTTP server on Config.ListenAddr (default
// 127.0.0.1:18080). External traffic is gated by the proxy's
// TLS + token layer (see cmdAgiExecProxy.go).
package livelisten
