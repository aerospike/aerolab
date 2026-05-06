// Package livelisten exposes the loopback HTTP endpoint used by AGI live log
// streaming. The listener accepts chunked newline-delimited log lines, creates
// one ingest.LiveStream per connection, and submits parsed rows into ingest's
// shared putBatcher pipeline.
package livelisten
