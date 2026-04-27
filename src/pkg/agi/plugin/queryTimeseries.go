package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/db"
	"github.com/cespare/xxhash/v2"
	"log"
)

type timeseriesResponse struct {
	Target string `json:"target"`
	// Datapoints is a slice of values (not pointers). Each
	// responsePoint is 24 bytes (a [2]float64 plus a bool with
	// padding); storing them inline keeps the per-row append cost
	// at exactly one slice grow rather than two heap allocations
	// (struct + 2-element backing array) per emitted point. JSON
	// output is byte-identical: encoding/json takes the address of
	// each element when calling the pointer-receiver MarshalJSON.
	Datapoints []responsePoint `json:"datapoints"` // list of int tuples, [][data,timestamp]
	groups     []string        // used for response grouping
	// groupHash is an xxhash/64 of the assembled group strings; the
	// per-row hot loop in handleQueryTimeseries compares this as a
	// single uint64 (an order of magnitude faster than the previous
	// SHA-256 + bytes.Equal pair, which dominated enumTime in
	// dashboards with millions of rows). Cryptographic strength is
	// not required here — the input is built from
	// dictionary-controlled metadata strings and a sub-million
	// series cap, well below xxhash's collision floor.
	groupHash uint64
	binIdx    int
}

type responsePoint struct {
	// point is a fixed [value, timestamp] tuple. Switching from
	// []float64 to a [2]float64 array drops the per-point backing
	// slice allocation (the prior shape paid one make() per
	// emitted point on top of the &responsePoint{} struct alloc).
	// Index access semantics (point[0], point[1]) are unchanged,
	// and JSON marshalling of a [2]float64 produces the same
	// "[v,t]" output as []float64{v,t}.
	point      [2]float64
	isDataNull bool
}

func (p *responsePoint) MarshalJSON() ([]byte, error) {
	if p.isDataNull {
		// p is *responsePoint, so p.point[1] is addressable; the
		// [2]float64 element address has the same lifetime as the
		// containing struct (which the caller owns), so the
		// pointer is safe to hand to json.Marshal.
		return json.Marshal([]*float64{nil, &p.point[1]})
	}
	return json.Marshal(p.point)
}

func (p *Plugin) handleQueryTimeseries(req *queryRequest, i int, remote string, r *http.Request) ([]*timeseriesResponse, error) {
	disableSeriesSafety := false
	if v, ok := req.selectedVars["DisableSeriesSafety"]; ok && v[0] == "true" {
		disableSeriesSafety = true
	}
	disableDPSafety := false
	if v, ok := req.selectedVars["DisableDataSizeSafety"]; ok && v[0] == "true" {
		disableDPSafety = true
	}
	log.Printf("DETAIL: DisableSeriesSafety:%t DisableDataSizeSafety:%t (type:timeseries) (remote:%s)", disableSeriesSafety, disableDPSafety, remote)
	ntime := time.Now()
	target := req.Targets[i]
	// Pick a single timestamp column name and use it for both
	// Between() (the indexed predicate) and Project() (so the row
	// the iterator hands back actually contains the timestamp the
	// loop expects). Honour an explicit Grafana payload override
	// if set; otherwise fall back to the plugin config. Splitting
	// the two — config for Between, payload for Project — was the
	// pre-fix bug: when the names diverged, Between filtered the
	// right column but Project asked for one that didn't exist,
	// so every dp.timestampMs collapsed to 0 silently.
	tsCol := target.Payload.TimestampBinName
	if tsCol == "" {
		tsCol = p.config.TimestampBinName
	}
	// fill bin list for the statement
	binListA := []string{tsCol}
	for _, bin := range target.Payload.Bins {
		if slices.Contains(binListA, bin.Name) {
			continue
		}
		binListA = append(binListA, bin.Name)
	}
	for _, filter := range target.Payload.FilterVariables {
		if val, ok := req.selectedVars[filter.Name]; ok && len(val) == 1 && val[0] == "NONE" {
			emptyResponse := []*timeseriesResponse{}
			log.Printf("DETAIL: Build query abort - NONE selected in filter (type:timeseries) (remote:%s)", remote)
			return emptyResponse, nil
		}
		if slices.Contains(binListA, filter.Name) {
			continue
		}
		binListA = append(binListA, filter.Name)
	}
	for _, g := range target.Payload.GroupBy {
		if slices.Contains(binListA, g.Name) {
			continue
		}
		binListA = append(binListA, g.Name)
	}

	// build filter expressions
	var exps []db.Expr
	// filter variables
	for _, filter := range target.Payload.FilterVariables {
		if _, ok := req.selectedVars[filter.Name]; !ok {
			return nil, fmt.Errorf("variable %s does not exist", filter.Name)
		}
		var vals []db.Value
		p.cache.lock.RLock()
		for _, v := range req.selectedVars[filter.Name] {
			if n, ok := p.cache.metadata[filter.Name]; !ok || n == nil {
				log.Printf("DETAIL: Grafana requsted a filter which results in nil dereference, skipping (ok:%t filter.Name:%s v:%s)", ok, filter.Name, v)
				continue
			}
			idxval := slices.Index(p.cache.metadata[filter.Name].Entries, v)
			if idxval == -1 {
				continue
			}
			vals = append(vals, db.Int(int64(idxval)))
		}
		p.cache.lock.RUnlock()
		if len(vals) == 0 {
			continue
		}
		valsExpr := db.In(filter.Name, vals...)
		var filterExp db.Expr
		if filter.MustExist {
			filterExp = db.And(db.Exists(filter.Name), valsExpr)
		} else {
			filterExp = db.Or(db.Not(db.Exists(filter.Name)), valsExpr)
		}
		exps = append(exps, filterExp)
	}
	// bin required-exists and bin-existence cache check
	binList := []string{}
	for _, bin := range target.Payload.Bins {
		binList = append(binList, bin.Name)
		if !bin.Required {
			continue
		}
		p.cache.lock.RLock()
		if !slices.Contains(p.cache.binNames, bin.Name) {
			p.cache.lock.RUnlock()
			if bin.DisplayName == "" {
				bin.DisplayName = bin.Name
			}
			return nil, fmt.Errorf("statistic bin %s (%s) not found", bin.DisplayName, bin.Name)
		}
		p.cache.lock.RUnlock()
		exps = append(exps, db.Exists(bin.Name))
	}
	// group list required-exists
	groupList := []string{}
	for _, bin := range target.Payload.GroupBy {
		groupList = append(groupList, bin.Name)
		if !bin.Required {
			continue
		}
		exps = append(exps, db.Exists(bin.Name))
	}

	// query
	q := p.db.Query(target.Target).
		Between(tsCol,
			db.Int(req.Range.From.UnixMilli()),
			db.Int(req.Range.To.UnixMilli())).
		Project(binListA...)
	if len(exps) == 1 {
		q = q.Where(exps[0])
	} else if len(exps) > 1 {
		q = q.Where(db.And(exps...))
	}
	log.Printf("DETAIL: Query start (type:timeseries) (remote:%s) (buildTime:%s)", remote, time.Since(ntime).String())
	ntime = time.Now()
	it := q.Run(r.Context())
	defer it.Close()
	resp := []*timeseriesResponse{}
	log.Printf("DETAIL: Enum results (type:timeseries) (remote:%s) (runQueryTime:%s)", remote, time.Since(ntime).String())
	ntime = time.Now()
	datapointCount := 0
	// Snapshot the metadata entries we will look up inside the
	// iterator loop before starting it. Holding p.cache.lock.RLock()
	// across the entire iterator (the previous behaviour) blocks
	// cacheMetadataList's writer-priority WLock; once one waiter
	// arrives, every subsequent reader queues behind it for the
	// full duration of this query. The histogram handler already
	// uses this copy-then-release pattern; do the same here.
	groupMeta := make(map[string][]string, len(groupList))
	p.cache.lock.RLock()
	for _, name := range groupList {
		entry, ok := p.cache.metadata[name]
		if !ok || entry == nil {
			continue
		}
		entries := make([]string, len(entry.Entries))
		copy(entries, entry.Entries)
		groupMeta[name] = entries
	}
	p.cache.lock.RUnlock()
	// Per-row lookups ahead of the hot loop. The previous shape used
	// slices.Contains(groupList, k) and slices.Contains(binList, k)
	// for every projected column on every row. With ~6 projected
	// cols × 2.27M rows × 2 linear scans, the constant factor here
	// was a meaningful fraction of enumTime; collapse it to one map
	// lookup per column. Roles are exclusive and ordered by the
	// original switch's priority (timestamp > group > bin), so
	// duplicates between groupBy/Bins resolve the same way they did
	// before.
	const (
		roleNone uint8 = iota
		roleTS
		roleGroup
		roleBin
	)
	colRole := make(map[string]uint8, len(binListA))
	for _, b := range binList {
		colRole[b] = roleBin
	}
	for _, g := range groupList {
		colRole[g] = roleGroup
	}
	colRole[tsCol] = roleTS
	// displayName lookup that used to scan target.Payload.Bins per
	// matching column.
	binDisplayName := make(map[string]string, len(target.Payload.Bins))
	for _, bin := range target.Payload.Bins {
		binDisplayName[bin.Name] = bin.DisplayName
	}
	// groupSlot lets the row loop write each group entry directly
	// into its groupList-ordered slot, removing the per-row
	// sort.Slice closure that used to follow the row decode. The
	// closure was doing two linear scans over groupList per pair
	// being compared, was the second-largest contributor to
	// dpSortTime in profiles, and emitted a fresh
	// func(i,j int) bool capture per row.
	groupSlot := make(map[string]int, len(groupList))
	for i, g := range groupList {
		groupSlot[g] = i
	}
	// Reusable per-row group buffer. Each iteration zeroes it
	// (clear() — Go 1.21+) so absent slots have value:"" and the
	// existing "skip empty value" filter at the hash/Target
	// assembly sites keeps the same semantics as the previous
	// append-only []*timeseriesGroup. Sized once outside the loop;
	// the in-place write at slot index avoids the
	// make([]*timeseriesGroup, 0, len(groupList)) + per-group
	// &timeseriesGroup{} heap allocations the old shape did per
	// row.
	dpGroupsBuf := make([]timeseriesGroup, len(groupList))
	// dst is the reusable Row passed to ReadInto; the iterator
	// clears and refills it in place each call, avoiding the
	// per-row map allocation that Record() was doing inside
	// shapeRowFromPlan.
	dst := make(db.Row, len(binListA))
	// xxhash digest for the per-row group key. Stack-allocated and
	// Reset() each iteration; replaces sha256.New() (which heap
	// allocates a *Digest plus a backing block buffer per row) and
	// the bytes.Equal compare on the resulting []byte.
	var groupHasher xxhash.Digest
	ptime1 := time.Duration(0)
	ptime2 := time.Duration(0)
	ptime3 := time.Duration(0)
	var ptime time.Time
	for it.Next() {
		_, row := it.ReadInto(dst)
		clear(dpGroupsBuf)
		dp := &datapoint{
			datapoints: make(map[string]point, len(target.Payload.Bins)),
			groups:     dpGroupsBuf,
		}
		if p.config.LogLevel > 5 {
			ptime = time.Now()
		}
		for k, v := range row {
			switch colRole[k] {
			case roleTS:
				ts, _ := v.AsInt()
				dp.timestampMs = int(ts)
				continue
			case roleGroup:
				idx, ok := v.AsInt()
				if !ok {
					return nil, fmt.Errorf("group column %s is not an int64", k)
				}
				entries, ok := groupMeta[k]
				if !ok {
					return nil, fmt.Errorf("metadata for item %s not found, metadata corrupt or log ingestion in progress", k)
				}
				// Defensive: a negative idx would pass the upper
				// bound check (len(...) is always >= 0) and then
				// panic on the entries[idx] access below. Corrupt
				// ingest data or a stale write from a previous
				// schema could produce one; fail the query with a
				// useful message instead.
				if idx < 0 || int64(len(entries)) <= idx {
					return nil, fmt.Errorf("metadata entry at index %d for item %s not found, metadata corrupt or log ingestion in progress", idx, k)
				}
				// Write directly to the groupList-ordered slot.
				// groupSlot is populated for every name in
				// groupList; colRole only routes a column here if
				// it is in groupList, so the lookup is guaranteed
				// to hit. No append, no per-group heap alloc, no
				// per-row sort.
				dp.groups[groupSlot[k]] = timeseriesGroup{
					name:  k,
					value: entries[idx],
				}
				continue
			case roleBin:
				// fall through to value extraction below
			default:
				continue
			}
			displayName := binDisplayName[k]
			switch v.Type() {
			case db.TypeInt64:
				iv, _ := v.AsInt()
				dp.datapoints[displayName] = point{
					value:   float64(iv),
					binName: k,
				}
			case db.TypeFloat64:
				fv, _ := v.AsFloat()
				dp.datapoints[displayName] = point{
					value:   fv,
					binName: k,
				}
			case db.TypeString:
				sv, _ := v.AsString()
				vva, err := strconv.ParseFloat(sv, 64)
				if err == nil {
					dp.datapoints[displayName] = point{
						value:   vva,
						binName: k,
					}
				}
			}
		}
		// Count one datapoint per row, not per column. The previous
		// "datapointCount++ inside the inner column loop" inflated
		// the count by len(row), so a wide multi-bin pattern hit
		// MaxDataPointsReceived an order of magnitude early and
		// returned the "too many datapoints" error spuriously.
		datapointCount++
		if !disableDPSafety {
			if datapointCount > p.config.MaxDataPointsReceived {
				return resp, errors.New("too many datapoints received, limit data by zooming in or selecting dropdown filters")
			}
		}
		if p.config.LogLevel > 5 {
			ptime1 += time.Since(ptime)
			// dpSortTime is reported below for backward
			// compatibility with log parsers; the per-row sort it
			// used to time has been removed (groups are now
			// written in groupList order directly), so it is
			// always zero.
			ptime = time.Now()
		}
		for k, v := range dp.datapoints {
			// Hash first; defer the dpGroups []string + Target join
			// allocations until we know the series is new. With a
			// few hundred unique series and millions of rows, this
			// turns the common path into "hash, lookup, append"
			// with no per-row slice allocation.
			groupHasher.Reset()
			if p.config.TimeseriesDisplayNameFirst && k != "" {
				groupHasher.WriteString(k) //nolint:errcheck // xxhash.Digest.WriteString never fails
			}
			for _, g := range dp.groups {
				if g.value != "" {
					groupHasher.WriteString(g.value) //nolint:errcheck // xxhash.Digest.WriteString never fails
				}
			}
			if !p.config.TimeseriesDisplayNameFirst && k != "" {
				groupHasher.WriteString(k) //nolint:errcheck // xxhash.Digest.WriteString never fails
			}
			grHash := groupHasher.Sum64()
			found := -1
			for i := range resp {
				if resp[i].groupHash != grHash {
					continue
				}
				found = i
				break
			}
			if found < 0 {
				dpGroups := make([]string, 0, len(dp.groups)+1)
				if p.config.TimeseriesDisplayNameFirst && k != "" {
					dpGroups = append(dpGroups, k)
				}
				for _, g := range dp.groups {
					if g.value != "" {
						dpGroups = append(dpGroups, g.value)
					}
				}
				if !p.config.TimeseriesDisplayNameFirst && k != "" {
					dpGroups = append(dpGroups, k)
				}
				found = len(resp)
				if !disableSeriesSafety {
					if len(resp) == p.config.MaxSeriesPerGraph {
						return resp, errors.New("too many series for graph, reduce series by selecting dropdown filters")
					}
				}
				resp = append(resp, &timeseriesResponse{
					Datapoints: []responsePoint{},
					groups:     dpGroups,
					groupHash:  grHash,
					Target:     strings.Join(dpGroups, p.config.TimeseriesLegendSeparator),
					binIdx:     slices.Index(binList, v.binName),
				})
			}
			val := float64(v.value)
			ts := float64(dp.timestampMs)
			resp[found].Datapoints = append(resp[found].Datapoints, responsePoint{point: [2]float64{val, ts}})
		}
		if p.config.LogLevel > 5 {
			ptime3 += time.Since(ptime)
		}
	}
	if err := it.Err(); err != nil {
		if errors.Is(err, r.Context().Err()) || isSocketTimeout(r.Context()) {
			return nil, errors.New("socket closed by client while enumerating")
		}
		return nil, fmt.Errorf("query iterator: %s", err)
	}

	log.Printf("DETAIL: Sort by time (type:timeseries) (remote:%s) (datapoints:%d) (enumTime:%s) (binListTime:%s) (dpSortTime:%s) (dp2respTime:%s) (waitOnDBTime:%s)", remote, datapointCount, time.Since(ntime).String(), ptime1.String(), ptime2.String(), ptime3.String(), (time.Since(ntime)-ptime1-ptime2-ptime3).String())
	ntime = time.Now()
	for ri := range resp {
		sort.Slice(resp[ri].Datapoints, func(i, j int) bool {
			return resp[ri].Datapoints[i].point[1] < resp[ri].Datapoints[j].point[1]
		})
	}
	if isSocketTimeout(r.Context()) {
		return nil, errors.New("socket closed by client after data sort")
	}

	log.Printf("DETAIL: Sort legend (type:timeseries) (remote:%s) (datapoints:%d) (timeSortTime:%s)", remote, datapointCount, time.Since(ntime).String())
	ntime = time.Now()
	sort.Slice(resp, func(i, j int) bool {
		return resp[i].Target < resp[j].Target
	})
	if isSocketTimeout(r.Context()) {
		return nil, errors.New("socket closed by client after legend sort")
	}

	log.Printf("DETAIL: Post-processing (type:timeseries) (remote:%s) (datapoints:%d) (legendSortTime:%s)", remote, datapointCount, time.Since(ntime).String())
	ntime = time.Now()
	reduceIntervalWindow := min(req.Range.To.Sub(req.Range.From).Milliseconds()/int64(req.MaxDataPoints), int64(req.IntervalMs))
	reduceIntervalWindow *= 2 // 2 real datapoints per window
	datapointCount = 0
	nullDatapoints := 0
	for ri := range resp {
		datapoints := []responsePoint{}
		lastPointTime := float64(-1)
		lastValue := float64(0)
		isFirstValue := true
		windowStartTime := float64(0)
		windowMinPoint := []float64{}
		windowMaxPoint := []float64{}
		windowNullTs := []float64{}
		for _, point := range resp[ri].Datapoints {
			if isSocketTimeout(r.Context()) {
				return nil, errors.New("socket closed by client during post-processing")
			}
			bin := target.Payload.Bins[resp[ri].binIdx]
			// maxIntervalSeconds breached
			if lastPointTime != -1 && bin.MaxIntervalSeconds != 0 && float64(point.point[1])-lastPointTime > float64(bin.MaxIntervalSeconds*1000) {
				ts := float64(point.point[1] - 1)
				windowNullTs = append(windowNullTs, ts)
			}
			prevPointTime := lastPointTime
			lastPointTime = float64(point.point[1])
			if prevPointTime == lastPointTime {
				log.Printf("DETAIL: (type:timeseries) (remote:%s) duplicate datapoint detected, skipping: PointTime=%0.1f", remote, lastPointTime)
				continue
			}
			val := float64(point.point[0])
			// produce delta
			if bin.ProduceDelta {
				if isFirstValue {
					isFirstValue = false
					lastValue = float64(point.point[0])
					continue
				}
				val -= lastValue
				lastValue = float64(point.point[0])
			}
			// apply reverse
			if bin.Reverse {
				val *= -1
			}
			// apply limits
			if bin.Limits != nil {
				if bin.Limits.MinValue != nil && val < float64(*bin.Limits.MinValue) {
					if bin.Limits.ReplaceWithOriginal {
						val = float64(point.point[0])
					} else {
						val = float64(*bin.Limits.MinValue)
					}
				}
				if bin.Limits.MaxValue != nil && val > float64(*bin.Limits.MaxValue) {
					if bin.Limits.ReplaceWithOriginal {
						val = float64(point.point[0])
					} else {
						val = float64(*bin.Limits.MaxValue)
					}
				}
			}
			// convert delta values from per-ticker-interval to per-second
			if bin.DeltaToPerSecond {
				tr := (float64(lastPointTime) - prevPointTime) / 1000
				if tr > 0 {
					val = val / tr
				}
			}
			// reduce and store datapoint
			ts := float64(point.point[1])
			if windowStartTime == 0 {
				windowStartTime = ts
			}
			if ts-windowStartTime > float64(reduceIntervalWindow) {
				dps, dpCount, nullCount := getDatapoints(windowMinPoint, windowMaxPoint, windowNullTs, bin.SingularSeriesExtend)
				datapoints = append(datapoints, dps...)
				nullDatapoints += nullCount
				datapointCount += dpCount
				windowStartTime = ts
				windowNullTs = []float64{}
				windowMinPoint = []float64{}
				windowMaxPoint = []float64{}
			}
			if len(windowMinPoint) == 0 || val < windowMinPoint[0] {
				windowMinPoint = []float64{val, ts}
			}
			if len(windowMaxPoint) == 0 || val > windowMaxPoint[0] {
				windowMaxPoint = []float64{val, ts}
			}
		}
		// store last unstored datapoint
		if len(windowMinPoint) > 0 {
			dps, dpCount, nullCount := getDatapoints(windowMinPoint, windowMaxPoint, windowNullTs, target.Payload.Bins[resp[ri].binIdx].SingularSeriesExtend)
			datapoints = append(datapoints, dps...)
			nullDatapoints += nullCount
			datapointCount += dpCount
		}
		// SingularSeriesExtend feature
		if len(datapoints) == 1 {
			// datapoints[0].point is now a [2]float64 array;
			// take a slice to keep singularSeriesExtend's
			// []float64 signature unchanged. Indexing through
			// datapoints[0] is addressable (it lives in a
			// locally-allocated slice), so the [:]-slice is
			// safe and shares the same backing storage.
			sse := singularSeriesExtend(target.Payload.Bins[resp[ri].binIdx].SingularSeriesExtend, datapoints[0].point[:])
			if len(sse) > 0 {
				datapoints = []responsePoint{sse[0], datapoints[0], sse[1]}
			}
		}
		resp[ri].Datapoints = datapoints
	}
	log.Printf("DETAIL: Return values (type:timeseries) (remote:%s) (reduceWindowMs:%d) (datapoints:%d) (nullpoints:%d) (postProcessTime:%s)", remote, reduceIntervalWindow, datapointCount, nullDatapoints, time.Since(ntime).String())
	return resp, nil
}

func singularSeriesExtend(extender any, point []float64) []responsePoint {
	defaultPoints := []responsePoint{
		{
			isDataNull: false,
			point:      [2]float64{float64(0), point[1] - 500},
		}, {
			isDataNull: false,
			point:      [2]float64{float64(0), point[1] + 500},
		},
	}
	switch sse := extender.(type) {
	case int:
		return []responsePoint{
			{
				isDataNull: false,
				point:      [2]float64{float64(sse), point[1] - 500},
			}, {
				isDataNull: false,
				point:      [2]float64{float64(sse), point[1] + 500},
			},
		}
	case float64:
		return []responsePoint{
			{
				isDataNull: false,
				point:      [2]float64{float64(sse), point[1] - 500},
			}, {
				isDataNull: false,
				point:      [2]float64{float64(sse), point[1] + 500},
			},
		}
	case string:
		if strings.ToUpper(sse) == "REPEAT" {
			return []responsePoint{
				{
					isDataNull: false,
					point:      [2]float64{float64(point[0]), point[1] - 500},
				}, {
					isDataNull: false,
					point:      [2]float64{float64(point[0]), point[1] + 500},
				},
			}
		} else if strings.HasPrefix(strings.ToUpper(sse), "DISABLE") {
			return nil
		} else if sseNo, ok := stringToFloat(sse); ok {
			return []responsePoint{
				{
					isDataNull: false,
					point:      [2]float64{float64(sseNo), point[1] - 500},
				}, {
					isDataNull: false,
					point:      [2]float64{float64(sseNo), point[1] + 500},
				},
			}
		} else {
			return defaultPoints
		}
	}
	return defaultPoints
}

func stringToFloat(s string) (f float64, ok bool) {
	for _, r := range s {
		if r != 46 && r != 45 && (r < 48 || r > 57) {
			return 0, false
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return f, false
	}
	return f, true
}

func getDatapoints(windowMinPoint []float64, windowMaxPoint []float64, windowNullTs []float64, extender any) (datapoints []responsePoint, dpCount int, nullCount int) {
	nullTsBefore := float64(-1)
	nullTsAfter := float64(-1)
	nullTsMid := float64(-1)
	for _, null := range windowNullTs {
		if null < windowMinPoint[1] && null < windowMaxPoint[1] {
			nullTsBefore = null
		}
		if null > windowMinPoint[1] && null > windowMaxPoint[1] {
			nullTsAfter = null
		}
		if (null > windowMinPoint[1] && null < windowMaxPoint[1]) || (null < windowMinPoint[1] && null > windowMaxPoint[1]) {
			nullTsMid = null
		}
		if nullTsBefore >= 0 && nullTsAfter >= 0 && nullTsMid >= 0 {
			break
		}
	}
	if nullTsBefore > -1 {
		datapoints = append(datapoints, responsePoint{point: [2]float64{0, nullTsBefore}, isDataNull: true})
		nullCount++
	}
	if windowMinPoint[1] < windowMaxPoint[1] {
		datapoints = append(datapoints, responsePoint{point: [2]float64{windowMinPoint[0], windowMinPoint[1]}})
		dpCount++
	} else {
		datapoints = append(datapoints, responsePoint{point: [2]float64{windowMaxPoint[0], windowMaxPoint[1]}})
		dpCount++
	}
	if nullTsMid > -1 {
		datapoints = append(datapoints, responsePoint{point: [2]float64{0, nullTsMid}, isDataNull: true})
		nullCount++
	}
	if windowMinPoint[1] > windowMaxPoint[1] {
		datapoints = append(datapoints, responsePoint{point: [2]float64{windowMinPoint[0], windowMinPoint[1]}})
		dpCount++
	} else if windowMinPoint[1] < windowMaxPoint[1] {
		datapoints = append(datapoints, responsePoint{point: [2]float64{windowMaxPoint[0], windowMaxPoint[1]}})
		dpCount++
	}
	if nullTsAfter > -1 {
		datapoints = append(datapoints, responsePoint{point: [2]float64{0, nullTsAfter}, isDataNull: true})
		nullCount++
	}
	if nullTsBefore > -1 && nullTsAfter > -1 && dpCount == 1 {
		// single datapoint between 2 nulls, add sse
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			datapoints = []responsePoint{datapoints[0], sse[0], datapoints[1], sse[1], datapoints[2]}
		}
	} else if nullTsAfter > -1 && dpCount == 1 {
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			datapoints = []responsePoint{datapoints[0], sse[1], datapoints[1]}
		}
	} else if nullTsBefore > -1 && dpCount == 1 {
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			datapoints = []responsePoint{datapoints[0], sse[0], datapoints[1]}
		}
	}
	if nullTsBefore > -1 && nullTsMid > -1 {
		// add sse around the first datapoint[1] as [0] is null
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			dpTemp := datapoints[2:]
			datapoints = []responsePoint{datapoints[0], sse[0], datapoints[1], sse[1]}
			datapoints = append(datapoints, dpTemp...)
		}
	} else if nullTsMid > -1 {
		// before is not null but mid is, add just to the right of data
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			dpTemp := datapoints[1:]
			datapoints = []responsePoint{datapoints[0], sse[0]}
			datapoints = append(datapoints, dpTemp...)
		}
	}
	if nullTsAfter > -1 && nullTsMid > -1 {
		// add sse around the second-to-last datapoint, as last is null
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			lastNul := datapoints[len(datapoints)-1]
			lastPoint := datapoints[len(datapoints)-2]
			datapoints = datapoints[:len(datapoints)-2]
			datapoints = append(datapoints, sse[0], lastPoint, sse[1], lastNul)
		}
	} else if nullTsMid > -1 {
		// add just to the left of data, as only mid point is null
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			lastPoint := datapoints[len(datapoints)-1]
			datapoints = datapoints[:len(datapoints)-1]
			datapoints = append(datapoints, sse[1], lastPoint)
		}
	}
	return
}

type datapoint struct {
	// groups is now indexed by groupList position (slot) and
	// reused across rows. Empty value (zero-value
	// timeseriesGroup{}) means "this slot is absent for this
	// row"; the hash/Target assembly already skipped
	// g.value == "" and that filter still does the right thing.
	groups      []timeseriesGroup
	datapoints  map[string]point
	timestampMs int
}

type point struct {
	value   float64
	binName string
}

type timeseriesGroup struct {
	name  string
	value string
}
