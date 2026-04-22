# AGI Plugin — Query Design Document

Source: `src/pkg/agi/plugin/`

This document describes how the AGI Grafana datasource plugin builds and
executes queries against Aerospike, and—more importantly—the post-processing
algorithms that turn raw record streams into renderable series. It focuses on
behaviour that is not obvious from the code alone: the downsampling window,
null-gap injection, delta handling, singular-series extension, and all of the
per-bin options that influence the final output.

The plugin is a JSON-over-HTTP datasource (Grafana SimpleJson-style). The
relevant HTTP endpoints live in `frontend.go`:

| Path                       | Handler                         | Purpose                                    |
| -------------------------- | ------------------------------- | ------------------------------------------ |
| `/metrics`                 | `handleMetrics`                 | List set names (the "metric" list)         |
| `/metric-payload-options`  | `handleMetricPayloadOptions`    | Not implemented                            |
| `/query`                   | `handleQuery`                   | Primary query dispatch (timeseries/table/static) |
| `/variable`                | `handleVariable`                | Grafana variable/dropdown population       |
| `/tag-keys`, `/tag-values` | `handleTagKeys`, `handleTagValues` | Not used by the AGI dashboards            |
| `/histogram`               | `handleHistogram`               | HDR-style bucket aggregation               |
| `/shutdown`                | `handleShutdown`                | Graceful shutdown                          |

All query handlers share a few pieces of infrastructure:

- A background cache refresher (`queryAndCache` in `backendQueryAndCache.go`)
  that every `CacheRefreshInterval` (default `30s`) reloads:
  - the namespace's set names (via `sets/<ns>` info call);
  - the bin list (first via `bins/<ns>` info, then a fallback to a
    `BINLIST` record stored in the `labels` set);
  - per-label metadata records from the `labels` set (`metaEntries` struct with
    `Entries`, `ByCluster`, `StaticEntriesIdx`).
- Two bounded concurrency gates (`requests` and `jobs` buffered channels)
  sized by `MaxConcurrentRequests` and `MaxConcurrentJobs`. `/query` and
  `/histogram` acquire a request slot immediately and a job slot just before
  doing database work.
- A `timedCheckSocketTimeout` goroutine that polls the HTTP request context
  once per second while the Aerospike recordset is being drained. If the
  client disconnects it closes the recordset and issues
  `jobs:module=query;cmd=kill-job;trid=<taskId>` to every node so the scan
  stops server-side.

---

## 1. The Label Dictionary (string → int)

Most filterable columns (cluster name, node ident, namespace, histogram-name,
etc.) are **not** stored as strings in the time-series sets. Instead, a record
in the `labels` set keeps an ordered list of distinct values:

```go
type metaEntries struct {
    Entries          []string         // the canonical dictionary
    ByCluster        map[string][]int // entry indices visible per cluster
    StaticEntriesIdx []int            // static entries (e.g. histogram bucket names)
}
```

Time-series records then store the **index** of the string in an integer bin.
This has two big consequences for query design:

1. Every filter-variable comparison is rewritten from a string equality into
   `ExpEq(ExpIntBin(name), ExpIntVal(idx))`. If the value is not in the
   dictionary the query simply skips that clause (`idxval == -1 → continue`).
2. Dictionaries are looked up under the cache read lock. If a client requests
   a filter that does not exist in the cache (`p.cache.metadata[filter.Name]`
   is nil) the plugin logs a warning and drops that clause rather than
   failing. This keeps Grafana variable panels resilient while the log
   ingester is still populating metadata.
3. On read, an integer in a group-by column is translated back to a string
   via `p.cache.metadata[k].Entries[v.(int)]`. A corrupt or mid-ingestion
   dictionary where the index is out of range is reported as
   `metadata entry at index %v for item %s not found`.

Filter variables and group-by columns both participate in expression
construction, but filters use dictionary-mapped integer equality while
group-by columns are used to bucket results after the query.

---

## 2. `/query` Dispatch (`frontendQuery.go`)

`handleQuery` is a thin wrapper. It:

1. Acquires a request slot.
2. Parses the body into `queryRequest`.
3. Flattens `ScopedVars` into `selectedVars map[string][]string`. Values can
   arrive as a single string or a JSON array of strings; Grafana's reserved
   `__` variables are dropped.
4. Acquires a job slot.
5. For each `Target`, dispatches based on `Payload.Type`:
   - empty / `timeseries` → `handleQueryTimeseries`
   - `table` → `handleQueryTable`
   - `static` → `handleQueryStatic`
6. Serialises all responses as a single JSON array.

Between each target dispatch the context is checked; if the Grafana client
disappears the handler bails out early.

---

## 3. Timeseries Query (`queryTimeseries.go`)

This is the most complex path and the source of every algorithmic choice the
user asked us to document.

### 3.1 Bin list and statement construction

The plugin assembles the **bins to fetch** by unioning:

- the timestamp bin name (`TimestampBinName`),
- every data `bin` listed in the target payload,
- every `FilterVariable` bin,
- every `GroupBy` bin.

A `Statement` is created over `namespace=<Aerospike.Namespace>, set=<Target>`
with a **range secondary-index filter** on the timestamp bin:

```go
stmt.SetFilter(aerospike.NewRangeFilter(
    TimestampBinName,
    req.Range.From.UnixMilli(),
    req.Range.To.UnixMilli(),
))
```

This is the only scan narrowing provided by the server. Everything else
(filters, group-by, required bins) is implemented as a filter expression on
the query policy — i.e. predicate pushdown evaluated per-record by the
Aerospike node.

### 3.2 Filter expression

The filter expression is built from three groups of clauses, all combined
with `ExpAnd`:

1. **Filter variables.** For each named filter the plugin collects one
   `ExpEq(ExpIntBin(name), ExpIntVal(dictIdx))` per selected value, OR's them
   together, and then:
   - if `MustExist=true` → `AND(ExpBinExists(name), OR-of-equalities)`;
   - otherwise → `OR(NOT(ExpBinExists(name)), OR-of-equalities)`.
     This means absence of the bin is treated as "does not match" only when
     `MustExist` is set; the looser form is what lets AGI render partial
     series while ingestion is still running.
   - A client-selected value of `"NONE"` short-circuits the entire query with
     an empty response. This is used by dashboard dropdowns that include a
     `NONE` sentinel (controlled by `Config.AddNoneToLabels`).

2. **Required bins.** For every data bin with `Required=true` a
   `ExpBinExists(bin.Name)` is AND'ed into the expression. Additionally, the
   plugin pre-flights the bin name against the cached bin list; if a
   `Required` bin is not currently present anywhere in the set the handler
   fails fast with `"statistic bin <name> (<display>) not found"` rather
   than returning an empty graph.

3. **Required group-by bins.** Same `ExpBinExists` trick, AND'ed in.

Query-policy timeouts are taken from `Aerospike.Timeouts.QuerySocket` /
`QueryTotal`.

### 3.3 Safety gates

Two dashboard-level toggles (shipped as Grafana variables) are read once per
query:

- `DisableSeriesSafety` disables the `MaxSeriesPerGraph` check.
- `DisableDataSizeSafety` disables the `MaxDataPointsReceived` check.

During result enumeration:

- Every non-timestamp, non-groupBy, non-filter bin increments
  `datapointCount`. If the total crosses `MaxDataPointsReceived` the query
  returns the partial response plus the error
  `"too many datapoints received, limit data by zooming in or selecting
  dropdown filters"`. The default (34,560,000) is sized so that four
  concurrent graphs rendering 1,000 series for a day fit in roughly 4 GiB.
- Whenever a new series (new group hash) is about to be created, if
  `len(resp) == MaxSeriesPerGraph` the query returns the partial response
  plus `"too many series for graph, reduce series by selecting dropdown
  filters"`.

Both errors surface as HTTP 400 in `handleQuery`, which is what causes the
red Grafana error banner advising the user to narrow their filters.

### 3.4 Record decoding

For each record the plugin builds a local `datapoint`:

```go
type datapoint struct {
    groups      []*timeseriesGroup // { name, value } one per GroupBy bin
    datapoints  map[string]point   // displayName → { value, binName }
    timestampMs int
}
```

- Integer group-by bins are decoded through
  `p.cache.metadata[k].Entries[v.(int)]` (string-dictionary lookup).
- Data-bin values are coerced to `float64`. Integers (`int`, `int64`),
  `float64`, and numeric strings are accepted; anything else is silently
  dropped.
- Group entries are then sorted to the canonical order listed in
  `Payload.GroupBy` so that two records with the same group values always
  produce the same hash regardless of Aerospike's bin-map iteration order.

### 3.5 Series bucketing by group hash

Each data bin on a single record is split into its own series. The series
identity (the Grafana legend) is computed from:

- the sorted `GroupBy` values that are non-empty, and
- the bin's display name.

The display name can be placed before or after the group-by values by setting
`Config.TimeseriesDisplayNameFirst`. The joined legend uses
`Config.TimeseriesLegendSeparator` (default `" : "`). A SHA-256 of the
assembled parts is used as the `groupHash`; lookup into the existing `resp`
slice is linear. `binIdx` is stored on the response so that post-processing
can recover the per-bin options later.

### 3.6 Per-series post-processing (the interesting part)

After the record stream is drained, each series' `Datapoints` slice is:

1. Sorted ascending by timestamp.
2. Walked in a single pass that simultaneously performs, **in this exact
   order per sample**:
   1. gap detection → null injection (`MaxIntervalSeconds`),
   2. `lastPointTime` update + duplicate-timestamp drop,
   3. raw-value capture (`val = point.point[0]`),
   4. delta conversion (`ProduceDelta`; first sample consumed),
   5. sign reversal (`Reverse`),
   6. floor/ceil limiting (`Limits` / `ReplaceWithOriginal`),
   7. delta-to-per-second conversion (`DeltaToPerSecond`),
   8. window-boundary check → flush previous window via `getDatapoints`,
   9. window-start init (first sample only),
   10. min/max window accumulation.
3. Flushed one more time after the loop, to emit the tail window.
4. If the final series has exactly one datapoint, wrapped with singular
   series extension points.

The order is important because the timestamp used for gap detection is the
**raw, un-transformed** sample timestamp, the value used for
`ReplaceWithOriginal` is the **raw** sample value (undoes both `ProduceDelta`
and `Reverse`), and `DeltaToPerSecond` uses the **elapsed ms between this
and the previous raw timestamp**, not the window-relative time.

#### 3.6.1 The downsampling window

```go
reduceIntervalWindow := min(
    (req.Range.To - req.Range.From) / req.MaxDataPoints,
    int64(req.IntervalMs),
)
reduceIntervalWindow *= 2 // 2 real datapoints per window
```

The intent is to honour Grafana's per-query rendering budget
(`maxDataPoints`) while never producing windows *finer* than Grafana's own
interval hint. The doubling is deliberate: each window will emit **both** the
min and max observed value (see `getDatapoints`), so we size the window to
hold two real points. This preserves spikes and dips that a naive mean would
erase.

Edge case: if the range is very short relative to `MaxDataPoints`, integer
division can yield `reduceIntervalWindow == 0`. Because the boundary check
is `ts - windowStartTime > reduceIntervalWindow`, every distinct timestamp
then triggers a flush and each window holds exactly one sample — the
algorithm degenerates gracefully to "no downsampling".

The boundary comparison is **strict `>`**, which means the sample that
crosses the boundary is **not** added to the flushed window; it becomes the
first sample of the new window (and its timestamp is what resets
`windowStartTime`). So each window is half-open `[startTs, startTs +
reduceIntervalWindow]` on the left and open-right where the next sample
lands.

#### 3.6.2 Null-gap injection (`MaxIntervalSeconds`)

For each bin the user may declare `MaxIntervalSeconds`. Inside the walk:

```go
if lastPointTime != -1 &&
   bin.MaxIntervalSeconds != 0 &&
   point.t - lastPointTime > bin.MaxIntervalSeconds*1000 {
    windowNullTs = append(windowNullTs, point.t - 1)
}
```

The intuition: if two adjacent raw samples are separated by more than the
declared ticker interval, we consider the series broken (process crash,
ingestion gap, etc.) and insert a synthetic null point one millisecond before
the new sample. The null is accumulated in `windowNullTs` and then emitted by
`getDatapoints` in the appropriate position inside the current downsample
window. `MaxIntervalSeconds=0` disables the feature entirely.

Notes:

- The null timestamp is literally `currentPoint.ts - 1` (integer ms).
- The check runs **before** the duplicate-timestamp drop and **before**
  `ProduceDelta`'s first-value consumption. This means that even when
  `ProduceDelta` silently discards the first sample, a gap detected on the
  second sample is still recorded correctly.
- `lastPointTime` is also updated on duplicates (right before `continue`),
  so a run of duplicates doesn't shift the reference point for gap
  detection.

Marshalling of nulls is special-cased:

```go
func (p *responsePoint) MarshalJSON() ([]byte, error) {
    if p.isDataNull {
        return json.Marshal([]*float64{nil, &p.point[1]}) // [null, ts]
    }
    return json.Marshal(p.point)
}
```

Grafana reads `[null, ts]` as "connect-break here", which is exactly the
visual behaviour we want for a crashed process.

#### 3.6.3 Duplicate-timestamp drop

`if prevPointTime == lastPointTime { continue }` — two samples sharing an
exact millisecond are assumed to be a log re-ingest and the second is
dropped. A `DETAIL` log line is emitted.

#### 3.6.4 Delta and per-second conversion (`ProduceDelta`, `DeltaToPerSecond`)

Many Aerospike statistics are monotonically increasing counters. When
`ProduceDelta` is true:

- The first sample of the series is consumed to seed `lastValue = rawValue`
  and emits no output point (hard `continue`, so it also skips window
  accumulation).
- Every subsequent sample emits `rawNow - lastValue`, then updates
  `lastValue = rawNow`. `lastValue` always holds the **raw** previous sample,
  never a delta or reversed value.

Then, if `DeltaToPerSecond` is true, the value is divided by
`(lastPointTime - prevPointTime) / 1000` (elapsed seconds since the previous
sample), but **only if the divisor is `> 0`**. The code does not require
`ProduceDelta` to be set for this branch to run — if a dashboard author
enables `DeltaToPerSecond` alone, the raw counter value is divided by
elapsed time. This is almost never what you want; the feature is named for
its intended pairing with `ProduceDelta`.

These options are designed to be composed: `ProduceDelta=true +
DeltaToPerSecond=true` gives a per-second rate from a cumulative counter, and
`ProduceDelta=true` alone gives per-ticker deltas.

#### 3.6.5 Sign reversal (`Reverse`)

A trivial `val *= -1`. Used to mirror a series below zero so two related
metrics (e.g., incoming vs outgoing) can be visualised symmetrically on one
panel.

#### 3.6.6 Limits (`Limits.MinValue`, `Limits.MaxValue`, `ReplaceWithOriginal`)

Floor/ceil clamping with one twist: if `ReplaceWithOriginal=true`, a value
that violates the limit is replaced with **the pre-processing raw value**
(`point.point[0]` straight from the Aerospike record). This is used when a
delta transform would otherwise produce a nonsensical negative (e.g., a
counter reset) — in that case we fall back to displaying the raw counter,
which is less misleading than clipping to zero.

Clamping happens *after* reverse but *before* delta-to-per-second. A
consequence worth calling out: `ReplaceWithOriginal` uses the **raw** sample,
so it undoes both `ProduceDelta` **and** `Reverse`, not just the delta. If a
series is defined with `Reverse=true` and the rendered (reversed) value
crosses a limit, enabling `ReplaceWithOriginal` will emit the positive raw
value, not the mirrored one.

Both `MinValue` and `MaxValue` are checked in the same sample iteration;
they can therefore trigger back-to-back on a value that is first clamped up
(min) and then the clamp would exceed max — but since both clamp toward the
valid range, the end state is the nearer bound.

#### 3.6.7 Min/Max window downsampling (`getDatapoints`)

The window boundary is crossed when `ts - windowStartTime >
reduceIntervalWindow`. When that happens the plugin flushes the current
window via `getDatapoints(windowMinPoint, windowMaxPoint, windowNullTs,
extender)`, then the triggering sample becomes the first sample of the new
window and `windowStartTime` is reset to its timestamp.

Inside the window, `windowMinPoint` and `windowMaxPoint` each carry `[value,
ts]`. Both use **strict** comparisons when accumulating:

```go
if len(windowMinPoint) == 0 || val < windowMinPoint[0] { windowMinPoint = [val, ts] }
if len(windowMaxPoint) == 0 || val > windowMaxPoint[0] { windowMaxPoint = [val, ts] }
```

The strictness matters: when multiple samples share the same extreme value,
the **earliest** one wins and its timestamp is retained. Combined with the
chronological-first emit rule, this keeps the leading edge of a plateau
visible.

The output order is always chronological: whichever of min/max has the
earlier timestamp is emitted first. If `minTs == maxTs` (a window of one
sample, or an early-exit window where only one update happened) only
`windowMaxPoint` is emitted — the second extremum slot stays empty because
neither `<` nor `>` fires on the tail branch at lines 575-581.

`windowNullTs` (from §3.6.2) is then spliced into the output so that:

- a null before both min/max is emitted first (`nullTsBefore`),
- a null *between* min and max is emitted in the middle (`nullTsMid`),
- a null after both is emitted last (`nullTsAfter`).

The three slots are mutually independent; any subset may be populated. Only
one null of each type per window survives — later ones overwrite earlier
ones (the classification loop only exits early once all three slots are
filled). This is a deliberate information loss to keep the rendered series
readable when a dense zoom-out covers many outages.

On top of the null placement, `getDatapoints` also splices `SSE` padding
(see §3.6.8) when a window emits exactly one real point surrounded by
nulls, or when `nullTsMid` is present — padding the adjacent real extremum
with a constant on each side so the extremum is visible as a draw-able
segment between the nulls. In every case the SSE value is derived from
`windowMinPoint` (the min-value extremum), not the chronologically-emitted
point; for `REPEAT` mode this means the padding repeats the window's
minimum value, which is usually fine because a single-point window has
`min == max`.

`getDatapoints` returns the flushed `[]*responsePoint` plus two counters
(`dpCount`, `nullCount`) purely for debug logging.

After the main loop the final partially-filled window is flushed with the
same routine (guarded by `len(windowMinPoint) > 0`, so a series of zero
accepted samples yields no output).

#### 3.6.8 Singular series extension (`SingularSeriesExtend`)

A series that contains **exactly one** real data point after downsampling is
invisible on a Grafana time-series panel (there is nothing to draw a line
between). To avoid silently disappearing metrics, a single datapoint is
expanded into three: the original, plus a synthetic point 500 ms to each
side. The value of the synthetic points is controlled by the
`singlarSeriesExtend` field on the bin and can be:

- an integer or float constant (e.g. `0`),
- the literal string `"REPEAT"` → repeat the real value on both sides
  (renders as a horizontal segment),
- any string beginning with `"DISABLE"` (case-insensitive) → opt out, no
  padding,
- a numeric string → parsed as a float constant,
- anything else / unset → default to `0` on both sides.

The same routine is also used *inside* `getDatapoints` to pad the real data
point when a window contains one real sample surrounded by nulls
(`nullTsBefore`, `nullTsMid`, `nullTsAfter`). The padding reasoning is the
same: ensure the one real value actually renders between the nulls that flag
a gap.

The post-loop `len(datapoints) == 1` wrap counts every entry in the emitted
slice, including null markers. In practice `getDatapoints` always emits at
least one real sample when called, so the wrap only fires when the final
series collapses to a single real point — the intended scenario.

#### 3.6.9 End-to-end pseudocode

The following captures the complete post-processing pipeline for **one
series** (one entry in `resp`). `resp[ri].Datapoints` at this point is the
raw, unsorted list of `(value, tsMs)` pairs collected in §3.4.

```text
function postProcessSeries(series, bin, rangeMs, maxDataPoints, intervalMs):

    # ---- 3.6.1: compute the downsample window size ----
    reduceWindow = min(rangeMs / maxDataPoints, intervalMs) * 2
    # reduceWindow may be 0 → degenerate, 1 sample per window

    # ---- step 0: sort the raw series chronologically ----
    sort(series, by: ts)

    out             = []           # final emitted datapoints
    lastPointTime   = -1           # last accepted raw ts
    lastValue       = 0            # last accepted raw value (for ProduceDelta)
    isFirstValue    = true         # ProduceDelta seed flag
    windowStartTime = 0            # current window's start ts (0 = not yet set)
    windowMin       = []           # [minValue, tsOfMin] inside current window
    windowMax       = []           # [maxValue, tsOfMax] inside current window
    windowNullTs    = []           # gap-null timestamps inside current window

    # ---------- SINGLE-PASS WALK ----------
    for sample in series:
        rawVal = sample.value
        ts     = sample.ts

        # 1) 3.6.2 GAP DETECTION (uses raw ts, may emit before duplicate drop)
        if lastPointTime != -1 and
           bin.MaxIntervalSeconds != 0 and
           ts - lastPointTime > bin.MaxIntervalSeconds * 1000:
                windowNullTs.append(ts - 1)

        # 2) 3.6.3 DUPLICATE DROP (note: lastPointTime is updated *before* skip)
        prevPointTime = lastPointTime
        lastPointTime = ts
        if prevPointTime == lastPointTime:
            log("duplicate")
            continue

        val = rawVal

        # 3) 3.6.4a PRODUCEDELTA (first sample is consumed, never emitted)
        if bin.ProduceDelta:
            if isFirstValue:
                isFirstValue = false
                lastValue    = rawVal        # always raw
                continue                     # skips *everything* below for this sample
            val       = rawVal - lastValue
            lastValue = rawVal               # always raw, never transformed

        # 4) 3.6.5 REVERSE (applies to the possibly-delta'd value)
        if bin.Reverse:
            val = -val

        # 5) 3.6.6 LIMITS (applied to post-reverse value)
        if bin.Limits != nil:
            if bin.Limits.MinValue != nil and val < bin.Limits.MinValue:
                val = bin.Limits.ReplaceWithOriginal ? rawVal : bin.Limits.MinValue
            if bin.Limits.MaxValue != nil and val > bin.Limits.MaxValue:
                val = bin.Limits.ReplaceWithOriginal ? rawVal : bin.Limits.MaxValue

        # 6) 3.6.4b DELTATOPERSECOND (applied unconditionally if flag set)
        if bin.DeltaToPerSecond:
            tr = (lastPointTime - prevPointTime) / 1000    # elapsed seconds
            if tr > 0:
                val = val / tr

        # 7) 3.6.7 WINDOW-BOUNDARY FLUSH (strict >, current sample starts new window)
        if windowStartTime == 0:
            windowStartTime = ts              # bootstrap on first accepted sample
        if ts - windowStartTime > reduceWindow:
            out.extend( getDatapoints(windowMin, windowMax, windowNullTs, bin.SSE) )
            windowStartTime = ts
            windowNullTs    = []
            windowMin       = []
            windowMax       = []

        # 8) 3.6.7 MIN/MAX ACCUMULATION (strict comparisons; earliest extremum wins)
        if windowMin is empty or val < windowMin.value:
            windowMin = [val, ts]
        if windowMax is empty or val > windowMax.value:
            windowMax = [val, ts]

    # ---------- TAIL FLUSH ----------
    if windowMin is not empty:
        out.extend( getDatapoints(windowMin, windowMax, windowNullTs, bin.SSE) )

    # ---------- 3.6.8 POST-DOWNSAMPLE SINGULAR SERIES EXTEND ----------
    if len(out) == 1:
        sse = singularSeriesExtend(bin.SSE, out[0])     # nil if "DISABLE"
        if sse != nil:
            out = [sse[0], out[0], sse[1]]

    return out


# ---- 3.6.7/3.6.8: emit one downsampled window ----
function getDatapoints(windowMin, windowMax, nullTs, sseExtender):
    # classify nulls: only the LAST of each class survives (no early exit until all three set)
    before = -1; mid = -1; after = -1
    for n in nullTs:
        if n < windowMin.ts and n < windowMax.ts:        before = n
        if n > windowMin.ts and n > windowMax.ts:        after  = n
        if (windowMin.ts < n < windowMax.ts) or
           (windowMax.ts < n < windowMin.ts):            mid    = n
        if before >= 0 and after >= 0 and mid >= 0: break

    dps = []
    if before > -1:
        dps.append( NULL_POINT(before) )

    # chronologically-earlier extremum
    if windowMin.ts < windowMax.ts:
        dps.append( windowMin ); dpCount = 1
    else:
        dps.append( windowMax ); dpCount = 1   # covers minTs == maxTs (single-sample window)

    if mid > -1:
        dps.append( NULL_POINT(mid) )

    # chronologically-later extremum (only emitted if the two extrema have
    # distinct timestamps — otherwise the window effectively had one sample)
    if windowMin.ts > windowMax.ts:
        dps.append( windowMin ); dpCount = 2
    elif windowMin.ts < windowMax.ts:
        dps.append( windowMax ); dpCount = 2

    if after > -1:
        dps.append( NULL_POINT(after) )

    # ---- SSE padding for single real point surrounded by nulls ----
    # SSE is always computed from windowMin (value used for REPEAT mode)
    if dpCount == 1:
        sse = singularSeriesExtend(sseExtender, windowMin)
        if sse != nil:
            if before > -1 and after > -1:        dps = [ null_before, sse[0], point, sse[1], null_after ]
            elif after > -1:                      dps = [ point, sse[1], null_after ]
            elif before > -1:                     dps = [ null_before, sse[0], point ]

    # ---- additional SSE padding when mid-null is present ----
    # (these re-splice whatever dps currently holds)
    if before > -1 and mid > -1:
        dps = [ null_before, sse[0], point1, sse[1], ...rest ]       # pad around point1 (before null was index 0)
    elif mid > -1:
        dps = [ earlier_extremum, sse[0], ...rest ]                  # pad to the right of the earlier extremum

    if after > -1 and mid > -1:
        dps = [ ...prefix, sse[0], penultimate_point, sse[1], null_after ]  # pad around the point before the after-null
    elif mid > -1:
        dps = [ ...all_but_last, sse[1], last_point ]                 # pad to the left of the last real point

    return dps


# ---- 3.6.8: compute ±500 ms padding based on the bin's SSE setting ----
function singularSeriesExtend(extender, referencePoint):
    ts = referencePoint.ts
    switch type(extender):
      case int, float:                return [ (extender, ts-500), (extender, ts+500) ]
      case string "REPEAT":           return [ (referencePoint.value, ts-500),
                                                (referencePoint.value, ts+500) ]
      case string starting "DISABLE": return nil
      case numeric string:            return [ (parsed, ts-500), (parsed, ts+500) ]
      default (unset / unknown):      return [ (0, ts-500), (0, ts+500) ]
```

**Where each feature is calculated:**

| Feature                         | Computed where                      | Uses                               |
| ------------------------------- | ----------------------------------- | ---------------------------------- |
| `reduceIntervalWindow`          | Once per series, before the walk    | `range`, `maxDataPoints`, `intervalMs` |
| Gap-null timestamp              | Step 1 of the walk                  | raw `ts`, `lastPointTime`, `MaxIntervalSeconds` |
| Duplicate drop                  | Step 2                              | `prevPointTime`, raw `ts`          |
| `ProduceDelta` seeding + subtraction | Step 3                         | raw value, raw `lastValue`         |
| `Reverse`                       | Step 4                              | post-delta `val`                   |
| `Limits` + `ReplaceWithOriginal`| Step 5                              | post-reverse `val`, raw value for fallback |
| `DeltaToPerSecond` division     | Step 6                              | `val`, `(lastPointTime - prevPointTime)` |
| Window flush decision           | Step 7                              | raw `ts`, `windowStartTime`, `reduceWindow` |
| Min/max accumulation            | Step 8                              | transformed `val`, raw `ts`        |
| Null classification (3-slot)    | `getDatapoints`                     | `windowNullTs`, `windowMin.ts`, `windowMax.ts` |
| Chronological emit order        | `getDatapoints`                     | `windowMin.ts` vs `windowMax.ts`   |
| In-window SSE padding           | `getDatapoints` (after base emit)   | `windowMin` (value source), `extender` |
| Tail-window flush               | After the walk, before post-SSE     | final state of `windowMin/Max/NullTs` |
| Post-downsample SSE wrap        | After tail flush                    | `len(out) == 1`, `extender`        |

### 3.7 Legend sort

Finally the response slice is sorted alphabetically by `Target` (the joined
legend string) so that Grafana's colour assignment is deterministic between
reloads.

---

## 4. Table Query (`queryTable.go`)

Much simpler than the timeseries path:

- Bin list = data bins + filter variables.
- Filter expressions are built with **string** equality
  (`ExpEq(ExpStringBin, ExpStringVal)`)—tables do not use the label
  dictionary.
- `Required` bins add `ExpBinExists` AND-clauses.
- Columns are defined up-front from `Payload.Bins` with `Type` (`time`,
  `string`, `number`) passed through to Grafana.
- Rows are assembled in bin-list order; missing bins become `""`.
- `SortOrder` is a list of 1-indexed column numbers; a negative entry means
  descending. Sorting is multi-key (stable within groups), with per-type
  comparators for `int`, `float64`, and `string`.

There is no downsampling, no filter expression caching, and no safety gate;
table payloads are assumed small.

---

## 5. Static Query (`queryStatic.go`)

`Payload.Static.File` is read from disk and parsed as a flat JSON object.
The handler then builds a one-row table response: columns = the names
requested in `Payload.Static.Names` (plus legacy single `Name`), values =
`data[name]` (or empty string if absent). Type inference picks `number` for
numeric Go kinds and `string` otherwise. This is how AGI exposes static
metadata (node counts, ingestion progress, etc.) into table panels.

---

## 6. Histogram Query (`/histogram`, `frontendHandleHistogram.go`)

A specialised endpoint for HDR-style bucket aggregation. The request
supplies a `Cluster`, a `Metric.Target` (the dictionary column, e.g.
`HistogramName`), a `Metric.Name` (the specific histogram), and a `Metric.Set`
(which Aerospike set to scan).

Query construction:

- The scan is restricted to 25 fixed bins named `"00".."24"`, which are the
  HDR bucket columns.
- Timestamp range filter as usual.
- Filter expression is `ExpAnd(filter, clusterfilter)` where both clauses
  are integer equality against dictionary indices for metric name and
  cluster.

Aggregation:

- Each bin name maps to a power-of-two key:
  - `00`→0, `01`→1, `02`→2, `03`→4, `04`→8, …, `23`→4,194,304, `tail`→8,388,608.
  - Bucket `03` and up double each step — this matches the HDR bucket layout.
- All records contribute to a single `map[int64]int64` (bucket → count);
  counts are summed across nodes and time.

The result is serialized as a flat `{ "bucket": count }` JSON object.

Known limitation called out in the code: there is currently no grouping by
node identifier; all nodes in the cluster are merged.

---

## 7. Variable Query (`/variable`, `frontendVariable.go`)

Populates Grafana dropdowns.

- Payload is of the form `"<target>"` or `"<target>@<json-array-of-clusters>"`
  or `"file::<path>"`.
- `file::` mode reads the file and returns its contents as a single option
  (used for embedding things like help URLs into dropdowns).
- Normal mode reads `p.cache.metadata[target]`:
  - With no cluster filter → every entry in `Entries`, de-duplicated.
  - With a cluster filter → only entries whose indices appear in
    `ByCluster[cluster]` for one of the requested clusters.
- If `target` is listed in `Config.AddNoneToLabels`, a sentinel `NONE` option
  is prepended. Selecting `NONE` later causes the timeseries query to
  short-circuit (see §3.2).
- If the target does not exist in the cache, the handler returns `[]` with
  a warning — this keeps dashboards renderable while ingestion is still
  populating metadata.

---

## 8. Cache Refresh Cadence

`queryAndCache` runs forever in a goroutine started by `Init`:

1. `cacheSetList` — `sets/<ns>` info call per node; the plugin walks
   `set=<name>` tokens from each `set=...:...` group.
2. `cacheBinListOld` — legacy `bins/<ns>` info call per node, then falls
   back to `cacheBinList` (reading the `BINLIST` record in the labels set).
3. `cacheMetadataList` — a `ScanAll` over the `labels` set (`BINLIST` bin
   skipped). Each bin's string value is unmarshalled into `*metaEntries`.
4. Sleep `CacheRefreshInterval` (default 30s) and repeat.

All mutations take `cache.lock.Lock()`; all reads (including inside
`handleQueryTimeseries`) take `cache.lock.RLock()` for the duration of the
record-enumeration loop so dictionary indices do not shift mid-query.

---

## 9. Summary of Per-Bin Controls

The `bin` struct in `queryStruct.go` is the primary surface a dashboard
author uses to shape a series. Quick reference:

| Field                | Applies to  | Effect                                                                 |
| -------------------- | ----------- | ---------------------------------------------------------------------- |
| `Name`               | all         | Aerospike bin to read.                                                 |
| `DisplayName`        | all         | Overrides `Name` in the legend / table header.                         |
| `Type`               | table       | `time`, `string`, or `number`; drives Grafana column typing.           |
| `Required`           | all         | Adds `ExpBinExists`. For timeseries, also fails the query up-front if the bin is missing from the cached bin list. |
| `Reverse`            | timeseries  | Multiplies the value by −1 after delta processing.                     |
| `ProduceDelta`       | timeseries  | Converts a cumulative counter to per-sample delta. First sample is consumed. |
| `DeltaToPerSecond`   | timeseries  | Divides the delta by the elapsed seconds between samples.              |
| `MaxIntervalSeconds` | timeseries  | Gap size above which a null is injected just before the next sample. `0` disables. |
| `Limits.MinValue`    | timeseries  | Clamp values below this (or substitute raw — see `ReplaceWithOriginal`). |
| `Limits.MaxValue`    | timeseries  | Clamp values above this.                                               |
| `Limits.ReplaceWithOriginal` | timeseries | When clamped, emit the pre-transform raw value instead of the limit. |
| `SingularSeriesExtend` | timeseries | Padding value/behaviour for one-point series or one-point-between-nulls windows. `"DISABLE"` opts out. |

The rest of the query behaviour (filter-variable intersection, group-by
bucketing, safety gates, downsample window sizing, null placement inside a
window) is global and is driven by `Config` values and the Grafana request
itself.
