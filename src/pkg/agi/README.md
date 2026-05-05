# AGI Package

The AGI (Aerospike Grafana Integration) package provides comprehensive log ingestion, processing, and visualization capabilities for Aerospike server logs. It consists of several subpackages that work together to collect, process, and visualize Aerospike logs and metrics, backed by an embedded `pkg/agi/db` (Pebble-based) store.

## Subpackages

### ingest
Handles log ingestion from various sources (S3, SFTP, local files) and processes them for storage in the embedded `pkg/agi/db` store.

**Key Features:**
- Multi-source log downloading (S3, SFTP, local)
- Automatic log unpacking and decompression
- Log preprocessing and pattern matching
- Embedded Pebble-backed storage via `pkg/agi/db`
- Progress tracking and monitoring

**Main Functions:**
- `Run(yamlFile string) error` - Main entry point for log ingestion
- `Init(config *Config) (*Ingest, error)` - Initialize ingest system
- `MakeConfig(setDefaults bool, configFile string, parseEnv bool) (*Config, error)` - Create configuration

### plugin
Provides a Grafana plugin backend for querying processed log data from the embedded `pkg/agi/db` store.

**Key Features:**
- Grafana datasource plugin backend
- Query processing and caching
- Metrics and timeseries data handling
- Concurrent request management

**Main Functions:**
- `Init(config *Config) (*Plugin, error)` - Initialize plugin system
- `MakeConfig(setDefaults bool, configFile string, parseEnv bool) (*Config, error)` - Create configuration

### grafanafix
Handles Grafana setup, dashboard management, and configuration fixes for optimal Aerospike server log visualization.

**Key Features:**
- Automatic dashboard importing
- Timezone configuration
- Annotation management
- Custom labeling and branding

**Main Functions:**
- `Run(g *GrafanaFix)` - Main entry point for Grafana setup
- `MakeConfig(setDefaults bool, configYaml io.Reader, parseEnv bool) (*GrafanaFix, error)` - Create configuration
- `EarlySetup(iniPath string, provisioningDir string, pluginsDir string, pluginUrl string, grafanaPort int) error` - Initial Grafana setup

### notifier
Provides notification capabilities for AGI monitoring and alerting.

**Key Features:**
- Authentication encoding/decoding
- Monitoring integration

**Main Functions:**
- `EncodeAuthJson() (string, error)` - Encode authentication data
- `DecodeAuthJson(val string) (*AgiMonitorAuth, error)` - Decode authentication data

### livelisten
In-process HTTP listener that accepts live log streams from a remote
dispatcher and feeds them through the same `metaShards` /
`putBatcher` / Pebble pipeline that batch ingest uses. Intended to be
embedded inside `cmdAgiExecService` (the merged AGI service); it is
NOT a standalone binary.

**Key Features:**
- Bearer-token auth via `/opt/agi/tokens/`
- Per-request stable nodePrefix allocation so reconnects keep the
  same labels for the same `(cluster,node)` pair
- Per-stream byte-offset checkpointing to `/opt/agi/live/offsets.json`
  so dispatchers can resume across AGI restarts
- Hard cap on concurrent streams (HTTP 429 when full)
- Refuses to start when `db.EnableWAL=false` (a wipe-on-restart with
  live ingest would lose data the dispatcher cannot replay)

**Main Functions:**
- `New(i *ingest.Ingest, cfg Config) *Listener`
- `(l *Listener) Serve(ctx context.Context) error`
- `(l *Listener) Shutdown(ctx context.Context) error`

### dispatcher
Aerolab-side library that tails a running Aerospike node's logs and
POSTs them, line-delimited and chunked, to an AGI listener via
`/agi/ingest/stream`. Pulls in NO CLI deps so it can be unit-tested
in isolation; the corresponding CLI lives at
`cli/cmd/v1/cmdAgiExecDispatch.go`.

**Key Features:**
- Auto-discovery of log destination (file / journald) from
  `/etc/aerospike/aerospike.conf`
- Auto-discovery of cluster name and node-id via `asinfo`, with a
  log-scan fallback for early-boot scenarios
- File-tail with inode-aware rotation handling
- Journald source via `journalctl -fn0 -u <unit> --output=cat`
- Exponential backoff (1s..30s) reconnect on transport errors
- Atomic state-file write so restarts resume from approximately the
  last successfully posted offset

**Main Functions:**
- `New(cfg Config) *Dispatcher`
- `(d *Dispatcher) Run(ctx context.Context) error`
- `ResolveSource(cfg Config) Source` - public helper for source
  auto-discovery; useful in tests

## Usage

The AGI package is typically used as part of the Aerolab ecosystem to provide comprehensive log analysis and visualization for Aerospike server logs. Each subpackage can be used independently or together for a complete monitoring solution.

### Example: Basic Log Ingestion
```go
import "github.com/aerospike/aerolab/pkg/agi/ingest"

// Run log ingestion with configuration file
err := ingest.Run("config.yaml")
if err != nil {
    log.Fatal(err)
}
```

### Example: Grafana Setup
```go
import "github.com/aerospike/aerolab/pkg/agi/grafanafix"

// Setup Grafana with default configuration
if err := grafanafix.Run(nil); err != nil {
    log.Fatal(err)
}
```

## Configuration

Each subpackage supports configuration through YAML files and environment variables. See individual subpackage documentation for specific configuration options.

## Live ingest

Live ingest is an additive mode on top of the existing batch
pipeline. Both modes share one Pebble DB and the same
`metaShards`/`putBatcher`/`logStream` parser; the only thing that
changes is the source of bytes (a live HTTP POST instead of a file
on disk).

### Architecture

```
[Aerospike node]                                [AGI instance]
  aerospike.service                                cmdAgiExecProxy
    └─> aerospike.log ─┐                              :443 TLS + token
                       │                                 │
  aerolab agi exec dispatch                              ▼
    (systemd unit installed by                       /agi/ingest/stream
     `aerolab cluster add agi-client`)                   │ reverse-proxy
                       │                                 ▼
                       └── chunked HTTPS POST ──> livelisten Listener
                            /agi/ingest/stream         │ (127.0.0.1:18080)
                                                       ▼
                                                   ingest.Ingest
                                                   (shared with batch)
                                                       │
                                                       ▼
                                                   putBatcher → Pebble
```

Both batch ingest and live ingest hold a refcount on the shared
`putBatcher`; the merged service only drains the batcher when both
holders have released, so live mode survives the end of batch
ingest without losing rows.

### Operator playbook

```bash
# 1. Create an AGI with the live listener enabled (forces WAL=on,
#    PostIngestCompact=off, generates a dispatcher token).
aerolab agi create -n bobagi --enable-live-ingest

# 2. Spin up your Aerospike cluster.
aerolab cluster create -n bob

# 3. Install the dispatcher onto every node of the cluster, pointing
#    at the AGI created above. Re-run any time the AGI's IP/token
#    changes; the unit is rewritten and `systemctl restart`ed.
aerolab cluster add agi-client -n bob --send-logs-to bobagi

# 4. Run a workload, watch the AGI Grafana dashboard update live.
aerolab open -n bobagi
```

The dispatcher writes its state to
`/var/lib/aerolab/agi-dispatch.state` and its bearer token to
`/etc/aerolab/agi-dispatch.token`. Logs land in the journal under
`aerolab-agi-dispatch.service`.

### Edge cases

- **WAL is mandatory.** Live mode REQUIRES `db.enableWAL: true` in
  both `ingest.yaml` and `plugin.yaml`. The dirty-marker
  crash-safety mechanism in `pkg/agi/ingest/init.go` wipes the DB
  on the next start when WAL=off, which is correct for batch
  ingest (the source files re-populate it) but disastrous for
  live ingest (lines have already been consumed; no replay
  source). `cmdAgiCreate --enable-live-ingest` flips both files
  to WAL=on, and `cmdAgiExecService` refuses to start the live
  listener at runtime when WAL=false.
- **Compaction.** Live mode disables `PostIngestCompact` because a
  synchronous end-of-batch compaction would stall the live stream.
  For long-running live AGIs, run a daily manual compaction via
  `aerolab agi attach -- pebble compact` or equivalent.
- **Stream limits.** The listener rejects new streams with HTTP
  429 once `len(activeStreams) >= Config.Live.MaxStreams`
  (default 256).
- **Token rotation.** Tokens live in `/opt/agi/tokens/`; the proxy
  watches the directory with fsnotify. Replace the file to rotate;
  active dispatcher connections keep working until they reconnect,
  at which point they pick up the new token from
  `/etc/aerolab/agi-dispatch.token`.
