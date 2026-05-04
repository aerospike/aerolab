# AGI Database Query Tool

`aerolab agi query` is a read-only debugging tool for inspecting the AGI instance's embedded log database (the LSM-backed store at `/opt/agi/db`) while the AGI service is running.

It exists because the database is held under an exclusive file lock by the live `agi-plugin` service: stopping the service to inspect data with another tool would defeat the purpose of having a running AGI in the first place. Instead, the plugin process exposes a small, localhost-only HTTP debug API and `aerolab agi query` knows how to talk to it.

## Table of Contents

- [What you can do](#what-you-can-do)
- [How it works](#how-it-works)
- [Transports: laptop vs. AGI shell](#transports-laptop-vs-agi-shell)
- [Quick reference](#quick-reference)
- [Modes](#modes)
  - [`--info`](#--info)
  - [`--list-sets`](#--list-sets)
  - [`--describe SET`](#--describe-set)
  - [`--sample SET`](#--sample-set)
  - [`--get-set / --get-key`](#--get-set----get-key)
  - [`--hash-key STRING`](#--hash-key-string)
  - [`--plan FILE`](#--plan-file)
- [Query plan reference](#query-plan-reference)
  - [Values](#values)
  - [Expressions](#expressions)
  - [Top-level fields](#top-level-fields)
- [Output formats](#output-formats)
- [Real-life recipes](#real-life-recipes)
- [Safety rails](#safety-rails)
- [Troubleshooting](#troubleshooting)

---

## What you can do

| Goal                                                   | Mode flag            |
|--------------------------------------------------------|----------------------|
| Sanity-check that the DB is open and how big it is     | `--info`             |
| List every logical "set" (table) ingest has populated  | `--list-sets`        |
| See the full sparse schema of one set                  | `--describe SET`     |
| Eyeball the first N rows of a set                      | `--sample SET`       |
| Point-read one row by its primary key                  | `--get-set / --get-key` |
| Run an arbitrary indexed-range + filter + project plan | `--plan FILE`        |
| Compute the metrics-set PK for a known triple, locally | `--hash-key STRING`  |

All operations are **read-only**. `Put`, `Delete`, `DropSet`, and friends are deliberately not exposed — debugging by mutating live ingest state is rarely what the operator wanted, and there is no rollback.

## How it works

```
┌──────────────────┐                ┌─────────────────────────────────────┐
│   your machine   │                │           AGI instance              │
│                  │                │                                     │
│  aerolab agi     │  ssh+curl OR   │  127.0.0.1:8851  ◄──── plugin mux   │
│      query       │  direct HTTP   │  /debug/db/info                     │
│                  │ ─────────────► │  /debug/db/sets                     │
│                  │                │  /debug/db/sets/{name}              │
│                  │                │  /debug/db/sample                   │
│                  │                │  /debug/db/get                      │
│                  │                │  /debug/db/query   ◄─── shared      │
│                  │                │      ▲                  Pebble      │
│                  │                │      └── ingest writes ─┘           │
└──────────────────┘                └─────────────────────────────────────┘
```

The plugin process owns the only handle to the embedded database. `aerolab agi query` never touches the DB files directly — it always goes through the plugin's debug HTTP API, which shares the same handle as ingest and survives concurrent reads cleanly.

## Transports: laptop vs. AGI shell

The same command works in two environments:

### Auto-detected (default)

```bash
# From your laptop or a CI runner
aerolab agi query -n myagi --info
```

This SSH'es into the AGI box (using your existing aerolab inventory + credentials) and invokes `curl` against the local debug listener. Auth, inventory, and rate-limiting are inherited from the rest of the aerolab agi family.

```bash
# From an SSH session on the AGI box itself
aerolab agi query --info
```

When `aerolab agi query` detects it is running on the AGI host (the `/opt/aerolab-agi-exec` marker file is present, the same sentinel `aerolab` already uses to skip auto-downgrades on AGI hosts), it switches to a direct localhost HTTP transport. **No SSH hop, no backend credentials, no cloud inventory lookup.**

### Forced

```bash
aerolab agi query --transport=local --info   # Force direct HTTP
aerolab agi query --transport=ssh   --info   # Force SSH-and-curl
aerolab agi query --transport=auto  --info   # Default (auto-detect)
```

Forcing `local` is useful when you have a port-forwarded tunnel to the plugin (e.g. `ssh -L 8851:127.0.0.1:8851 myagi`) but no aerolab inventory configured. Forcing `ssh` is useful for testing the SSH path from the AGI host itself.

## Quick reference

```bash
# health check
aerolab agi query -n myagi --info

# discover what's in the DB
aerolab agi query -n myagi --list-sets
aerolab agi query -n myagi --describe metrics

# eyeball some rows
aerolab agi query -n myagi --sample metrics --limit 5

# point read by primary key (copy a real key from --sample first)
aerolab agi query -n myagi --get-set metrics --get-key "$(aerolab agi query -n myagi --sample metrics --limit 1 -o ndjson | jq -r 'select(._meta == null) | .key')"

# compute the metrics PK locally without contacting the box
aerolab agi query --hash-key 'cluster-a::/::1_bb978a3b3565000::/::Apr 22 2026 00:00:25 GMT+0700: INFO ...'

# arbitrary indexed-range query
cat <<'EOF' | aerolab agi query -n myagi --plan -
{
  "set": "metrics",
  "between": {"col":"timestamp","lo":{"int":1730000000000},"hi":{"int":1730003600000}},
  "where":   {"and":[
    {"eq":{"col":"ClusterName","value":{"int":0}}},
    {"exists":{"col":"latency_p99"}}
  ]},
  "project": ["timestamp","latency_p99"],
  "limit":   100
}
EOF
```

## Modes

Every command takes the AGI instance name (`-n / --name`, default `agi`) and an output format (`-o / --output`, default `table`). Modes are mutually exclusive — pass exactly one. `--hash-key` is the one exception that doesn't talk to the AGI instance at all and works without `-n`.

### `--info`

Returns the database's path, on-disk storage version, the list of registered set names, and a snapshot of the live `db.Stats` block (puts, deletes, gets, scans, queries, open iterators, Pebble metrics like cache size / compaction debt / disk usage).

```bash
aerolab agi query -n myagi --info
```

Use it as a fast "is the DB even open" probe. If this comes back, the plugin is up, the Pebble lock is held, and ingest can proceed.

### `--list-sets`

Lists every set that has been registered. Each entry shows the set name, the column count, and the indexed column (if any).

```bash
aerolab agi query -n myagi --list-sets
```

```
┌─────────┬─────────┬─────────────────┐
│ NAME    │ COLUMNS │ INDEXED COLUMN  │
├─────────┼─────────┼─────────────────┤
│ labels  │       1 │ (none)          │
│ metrics │      12 │ timestamp       │
└─────────┴─────────┴─────────────────┘
```

A set with no indexed column can only be scanned end-to-end (no `Between` push-down). The `metrics` set is always indexed on `timestamp` — it's the AGI plugin's hot path.

### `--describe SET`

Prints the full sparse schema for one set: every column's name, value type (`int64`, `float64`, `string`, `bytes`, `bool`), and indexed flag.

```bash
aerolab agi query -n myagi --describe metrics
```

```
Set: metrics
Indexed column: timestamp

┌─────────────┬──────┬─────────┐
│ NAME        │ TYPE │ INDEXED │
├─────────────┼──────┼─────────┤
│ timestamp   │ int64│ yes     │
│ ClusterName │ int64│         │
│ NodeIdent   │ int64│         │
│ latency_p99 │ int64│         │
│ op_count    │ int64│         │
│ ...                          │
└─────────────┴──────┴─────────┘
```

The schema is sparse — any given row may omit any column. `--describe` shows you the union of every column ingest has registered.

### `--sample SET`

Streams the first N rows of a set via a forward `Scan`, then prints a summary line. Use this when you need to see the actual data shape in a hurry.

```bash
aerolab agi query -n myagi --sample metrics --limit 5
```

`--limit` defaults to 100 and is capped server-side at 10 000.

### `--get-set / --get-key`

Point-reads exactly one row by primary key. Returns `{"found": false}` when the key isn't present.

```bash
aerolab agi query -n myagi --get-set metrics --get-key e3b0c44298fc1c149afbf4c8996fb924
```

The primary key for the **metrics set** is a 32-character lowercase hex string: the XXH3-128 of `<ClusterName>::/::<NodePrefix>_<NodeID>::/::<rawLogLine>`. Practical consequences:

- You can't construct a key by typing — copy one from `--sample` (the simplest path), or compute one locally from the three components with [`--hash-key`](#--hash-key-string) (handy when you have the (cluster, node, line) triple in front of you and want to point-read it without a round-trip).
- It is exact-match only.
- Because the line is part of the hashed input, two identical log lines from the same node at the same time hash to the **same** key and overwrite each other. That's the intended idempotency contract — re-running ingest on the same source data does not double-count.
- Metrics-set keys are **opaque on purpose**: nothing on the live read path (Grafana datasource, plugin queries) decodes the key back into (cluster, node, line). Cluster and node identity travel as int columns inside the row payload (`ClusterName`, `NodeIdent`), indexed by the labels metadata. The raw line itself is intentionally not stored — if you want it, the original log files are still on disk under `/opt/agi/files/input`.

The **`labels` set** is different: its keys are short, meaningful strings (`ClusterName`, `NodeIdent`, `BINLIST`, `sources`, `timerange`, …) that ingest writes by hand. Those you can pass to `--get-key` directly — see the [recipes](#real-life-recipes) below.

> **Why hashed?** Pre-v3 builds used the raw `cluster::/::node::/::logLine` string as the key. That cost ~150 bytes per row in the LSM (twice — once in the `D/` forward pointer, once as the suffix of the `I/` index entry) and bought no functionality the live read path uses. XXH3-128 gives ~10× the storage saving with a birthday-collision probability of ≈1.5 × 10⁻¹⁹ at 1 TiB of typical Aerospike server logs — orders of magnitude below the box's other failure modes (uncorrected disk errors, ECC RAM faults). The DB layer is unchanged; only the `pkg/agi/ingest` PK construction differs. Old volumes opened by a v3 build will refuse with `ErrStorageVersionMismatch` and trigger a wipe + re-ingest, which is the standard recovery path AGI was designed around.

### `--hash-key STRING`

Compute the metrics-set primary key for a known `cluster::/::node::/::line` triple. Pure local computation — no SSH, no HTTP, no backend, no aerolab inventory needed. Works on a bare laptop.

```bash
aerolab agi query --hash-key 'aero-tbsprod::/::1_bb97829b3565000::/::Apr 22 2026 00:00:25 GMT+0700: INFO (nsup): (nsup.c:419) {vdsp} ...'
# → e3b0c44298fc1c149afbf4c8996fb924
```

The output is the same 32-char hex string ingest would have written for that input. Pipe it straight into `--get-key`:

```bash
KEY=$(aerolab agi query --hash-key 'aero-tbsprod::/::1_bb97829b3565000::/::'"$(head -n 1 /path/to/aerospike.log)")
aerolab agi query -n myagi --get-set metrics --get-key "$KEY"
```

`--hash-key` is mutually exclusive with all other modes; combining it with `--info`, `--sample`, etc. is a usage error. Its only "side effect" is printing one line to stdout (or to `--stdout FILE`).

### `--plan FILE`

Posts an arbitrary query plan as JSON. The plan body can be in a file (`--plan path/to/plan.json`) or piped on stdin (`--plan -`). Results stream back as newline-delimited JSON terminated by a `_meta` summary record:

```
{"key":"...","row":{...}}
{"key":"...","row":{...}}
...
{"_meta":{"rowsReturned":42,"truncated":false,"durationMs":"7"}}
```

`durationMs` is purely server-side; it doesn't include SSH time on the SSH transport.

## Query plan reference

The wire format is a tagged-union JSON object that maps one-to-one onto the Go `QueryBuilder` API the plugin uses internally. There is no SQL parser involved — the format is small and explicit by design.

### Values

A typed value is a JSON object with **exactly one** of:

```json
{"int":   42}                    // int64
{"float": 1.25}                  // float64
{"str":   "hello"}               // string
{"bool":  true}                  // bool
{"bytes": "aGVsbG8="}            // base64 std-encoded bytes
```

`int` accepts both bare numbers (`{"int":42}`) and quoted strings (`{"int":"42"}`); the latter is preferred for timestamps in milliseconds because it can't be silently truncated to float64 by an intermediate JSON parser.

### Expressions

A predicate is a JSON object with **exactly one** of these keys:

| Shape                                                          | Meaning                            |
|----------------------------------------------------------------|------------------------------------|
| `{"and": [expr, …]}`                                           | logical AND of all sub-expressions |
| `{"or":  [expr, …]}`                                           | logical OR of all sub-expressions  |
| `{"not": expr}`                                                | logical NOT                        |
| `{"eq":      {"col":"X", "value": VALUE}}`                     | column X equals VALUE              |
| `{"in":      {"col":"X", "values":[VALUE, …]}}`                | column X is one of VALUES          |
| `{"between": {"col":"X", "lo": VALUE, "hi": VALUE}}`           | column X within [lo, hi]           |
| `{"exists":  {"col":"X"}}`                                     | column X is present in the row    |

`exists` matters because the AGI schema is sparse: a row may simply not carry a column at all, which is different from carrying a zero/empty value.

### Top-level fields

```json
{
  "set":     "<set name>",
  "between": {"col":"<indexed column>","lo":VALUE,"hi":VALUE},
  "where":   <expression>,
  "project": ["col1","col2",...],
  "limit":   1000
}
```

| Field     | Required | Notes                                                                                                     |
|-----------|----------|-----------------------------------------------------------------------------------------------------------|
| `set`     | yes      | Must already exist; check via `--list-sets`.                                                              |
| `between` | optional | Must reference the indexed column to push down to a fast indexed scan; otherwise the planner falls back to a full scan. |
| `where`   | optional | Any expression tree. Filters are evaluated server-side.                                                   |
| `project` | optional | Restricts the columns decoded into each row. Use it: every column you don't ask for saves CPU + bandwidth.|
| `limit`   | optional | Caps result rows. Defaults to 1 000, max 100 000.                                                         |

If `between` is omitted on an indexed set, the query degrades to a full set scan. That is allowed but rarely what you want for `metrics` — always supply a time range.

## Output formats

`-o / --output` accepts:

| Format        | Best for                                                       |
|---------------|----------------------------------------------------------------|
| `table`       | (default) human-readable table per mode                        |
| `json`        | single-line JSON; rows for `--sample` / `--plan` are wrapped as `{"rows":[...], "meta":{...}}` |
| `json-indent` | as `json` but pretty-printed                                   |
| `ndjson`      | raw newline-delimited records, with the trailing `_meta` line — pipe-friendly |

For machine consumption (`jq`, automation), prefer `ndjson`:

```bash
aerolab agi query -n myagi --sample metrics --limit 1000 -o ndjson \
  | jq -r 'select(._meta == null) | "\(.row.timestamp.int)\t\(.row.latency_p99.int)"'
```

## Real-life recipes

### "Is ingest actually writing to the DB?"

```bash
aerolab agi query -n myagi --info | jq '.stats | {Puts, Scans, Queries, OpenIterators}'
```

Repeat the call a few seconds apart. If `Puts` isn't climbing while `agi details` says ingest is running, ingest is bottlenecked somewhere upstream of the DB.

### "What clusters and nodes did I actually ingest?"

The `labels` set contains the discovery metadata. The `ClusterName` row carries the JSON-encoded list of clusters seen so far:

```bash
aerolab agi query -n myagi --get-set labels --get-key ClusterName -o json-indent
```

```json
{
  "set": "labels",
  "key": "ClusterName",
  "found": true,
  "row": {
    "json": {"str":"{\"Entries\":[\"cluster-a\",\"cluster-b\"], ...}"}
  }
}
```

For nodes:

```bash
aerolab agi query -n myagi --get-set labels --get-key NodeIdent -o json-indent
```

### "Show me the last 5 minutes of latency for cluster A"

`ClusterName` in the metrics set is encoded as the index of the cluster in the labels metadata. For the first cluster (`Entries[0]`) that's `0`. Pick a `[lo, hi]` range covering 5 minutes:

```bash
NOW=$(date +%s%3N)            # ms since epoch
FIVEMIN_AGO=$((NOW - 300000))

cat <<EOF | aerolab agi query -n myagi --plan - -o ndjson
{
  "set": "metrics",
  "between": {"col":"timestamp","lo":{"int":"$FIVEMIN_AGO"},"hi":{"int":"$NOW"}},
  "where":   {"and":[
    {"eq":{"col":"ClusterName","value":{"int":0}}},
    {"exists":{"col":"latency_p99"}}
  ]},
  "project": ["timestamp","NodeIdent","latency_p99"],
  "limit":   10000
}
EOF
```

The `exists` clause is important: rows in the metrics set are sparse, and not every timestamp carries every metric. Filtering rows that actually have `latency_p99` removes the ones that only carry `op_count` or other metrics.

### "Find rows where any of these node identifiers appear"

```bash
cat <<'EOF' | aerolab agi query -n myagi --plan -
{
  "set": "metrics",
  "between": {"col":"timestamp","lo":{"int":1730000000000},"hi":{"int":1730003600000}},
  "where": {"in":{"col":"NodeIdent","values":[{"int":0},{"int":2},{"int":5}]}},
  "project": ["timestamp","NodeIdent","latency_p99","op_count"],
  "limit": 5000
}
EOF
```

### "Spot-check that ingest is filling cluster-only metric `dropped`"

```bash
cat <<'EOF' | aerolab agi query -n myagi --plan - -o json-indent
{
  "set": "metrics",
  "between": {"col":"timestamp","lo":{"int":0},"hi":{"int":99999999999999}},
  "where":   {"and":[
    {"exists":{"col":"dropped"}},
    {"not":{"eq":{"col":"dropped","value":{"bool":false}}}}
  ]},
  "project": ["timestamp","ClusterName","dropped"],
  "limit":   50
}
EOF
```

Useful when a Grafana panel that depends on a sparse boolean column comes up empty and you want to know whether the column is missing from the data or just not being queried correctly.

### "Run the same query straight from the AGI shell"

Once you've SSH'd into the AGI box (`aerolab agi attach -n myagi` or directly via your inventory), the same command Just Works without `-n`:

```bash
aerolab agi query --info
aerolab agi query --list-sets
aerolab agi query --sample metrics --limit 5
```

This is the recommended workflow when something is wrong on the box and you want to avoid round-tripping back to your laptop's inventory. It also means scripts you write inside the ttyd shell are portable: they don't carry the AGI cluster name and don't need backend credentials.

### "Pipe results into another tool"

NDJSON is line-oriented and trivially tools-friendly:

```bash
# count rows that have latency_p99 in the last hour
NOW=$(date +%s%3N); HR_AGO=$((NOW - 3600000))
cat <<EOF | aerolab agi query -n myagi --plan - -o ndjson \
  | jq -s 'map(select(._meta == null)) | length'
{
  "set":"metrics",
  "between":{"col":"timestamp","lo":{"int":"$HR_AGO"},"hi":{"int":"$NOW"}},
  "where":{"exists":{"col":"latency_p99"}},
  "project":["timestamp"],
  "limit": 100000
}
EOF
```

```bash
# dump the latest 1000 rows to a file for offline analysis
aerolab agi query -n myagi --sample metrics --limit 1000 -o ndjson \
  --stdout /tmp/metrics-sample.ndjson
```

### "Save and replay a query"

```bash
cat > /tmp/last5min.json <<'EOF'
{
  "set": "metrics",
  "between": {"col":"timestamp","lo":{"int":1730000000000},"hi":{"int":1730003600000}},
  "where": {"and":[
    {"eq":{"col":"ClusterName","value":{"int":0}}},
    {"exists":{"col":"latency_p99"}}
  ]},
  "project": ["timestamp","NodeIdent","latency_p99"],
  "limit": 1000
}
EOF

aerolab agi query -n myagi --plan /tmp/last5min.json -o table
aerolab agi query -n myagi --plan /tmp/last5min.json -o ndjson > /tmp/last5min.ndjson
```

## Safety rails

The debug API is conservative on purpose:

- **Read-only.** No put/delete/drop endpoints exist.
- **Localhost-only.** The plugin's listener is `127.0.0.1:8851`. The AGI proxy never forwards `/debug/*` paths, so this is not reachable from the public web URL.
- **Default `LIMIT=1000` on `--plan`**, capped at 100 000. `--sample` defaults to 100, capped at 10 000.
- **60 s server-side per-request timeout** on `/debug/db/query`, 30 s on `/debug/db/sample`. A runaway curl can't pin a Pebble snapshot indefinitely.
- **`DisallowUnknownFields`** on the query plan decoder. Typoed field names fail loud (`HTTP 400`) instead of being silently dropped.
- **Trailing `_meta` record** on every NDJSON stream. Use it to distinguish "iterator drained cleanly" from "we hit the limit and there would have been more" (`truncated: true`) or "iterator errored mid-stream" (`error: ...`).

## Troubleshooting

### `no running AGI instance named "agi" (node 1)`

The SSH transport couldn't find a matching instance in your aerolab inventory. Check `aerolab agi list` and pass the right `-n / --name`. Note that AGI replicas / templates on node numbers > 1 don't run the plugin — only node 1 does.

### `ssh exec to AGI "myagi": ... (stderr=curl: (7) Failed to connect to 127.0.0.1 port 8851: Connection refused)`

The plugin isn't running. Check `aerolab agi status -n myagi` — the `agi-plugin` service should be active. If it isn't, look at `/var/log/agi-plugin.log` on the box.

### `http 400 ... server error: query must specify 'set'`

Your query plan was decoded but failed validation. The error envelope (`{"error":"..."}`) is surfaced verbatim from the server.

### `http 400 ... json: unknown field "wat"`

You misspelled a top-level field in the plan. The decoder is strict on purpose — the alternative is a silently-ignored field that produces wrong-but-plausible results. Compare against the [Top-level fields](#top-level-fields) reference.

### `http 400 ... wire: ambiguous value (set exactly one of int|float|str|bytes|bool)`

Your value object set more than one type tag. A value is a tagged union: pick exactly one (e.g. `{"int":42}`, not `{"int":42,"str":"42"}`).

### `--transport=local` fails with `connection refused`

You're on a machine that isn't the AGI box. The local transport only works when there's actually a plugin on `127.0.0.1:8851` — either run on the AGI host itself, or set up an SSH tunnel:

```bash
ssh -L 8851:127.0.0.1:8851 root@my-agi-box &
aerolab agi query --transport=local --info
```

### Output is truncated / `_meta.truncated == true`

You hit the row cap. Bump `--limit`, but keep in mind the server's hard ceiling (100 000 for `--plan`, 10 000 for `--sample`). For larger pulls, narrow the `between` range and run multiple queries.

### `--info` reports `storageVersion: 2` (or you see `ErrStorageVersionMismatch` on plugin start)

You're looking at a pre-v3 AGI volume. v3 changed the metrics-set PK from the raw `cluster::/::node::/::logLine` string to its 32-char XXH3-128 hex; old rows are unreachable from new ingest and the storage version constant was bumped so existing volumes refuse to open until they're wiped and re-ingested. The fix is the standard AGI recovery flow:

```bash
aerolab agi destroy   -n myagi    # keeps the volume
aerolab agi delete    -n myagi    # or this, to also drop the volume
aerolab agi create    -n myagi --source-... ...
```

The original log files on disk are the source of truth, so re-ingest is cheap and lossless.

### `--get-key` with a hand-crafted metrics key always returns `{"found": false}`

The metrics PK is a hashed value, not a meaningful string. Either copy a real key from `--sample <set>` output, or compute one with `--hash-key 'cluster::/::node::/::line'`. See the [`--get-set / --get-key`](#--get-set----get-key) section for context.

### "I want to write SQL, not JSON"

By design, no. The wire format is a thin shell over the Go QueryBuilder — server-side parsing surface is the smallest it can possibly be. If the JSON gets repetitive, write a tiny client-side wrapper that emits it; everything in this document is just JSON over HTTP. See the recipes above for the typical pattern (heredoc + `--plan -`).
