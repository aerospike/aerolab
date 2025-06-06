package plugin

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerospike-client-go/v8"
	"github.com/bestmethod/inslice"
	"github.com/bestmethod/logger"
	"github.com/rglonek/sbs"
)

type timeseriesResponse struct {
	Target     string           `json:"target"`
	Datapoints []*responsePoint `json:"datapoints"` // list of int tuples, [][data,timestamp]
	groups     []string         // used for response grouping
	groupHash  []byte
	binIdx     int
}

type responsePoint struct {
	point      []float64
	isDataNull bool
}

func (p *responsePoint) MarshalJSON() ([]byte, error) {
	if p.isDataNull {
		return json.Marshal([]*float64{nil, &p.point[1]})
	}
	return json.Marshal(p.point)
}

func (p *Plugin) handleQueryTimeseries(req *queryRequest, i int, remote string, r *http.Request) ([]*timeseriesResponse, error) {
	logger.Detail("Build query (type:timeseries) (remote:%s)", remote)
	ntime := time.Now()
	// fill bin list for the statement
	binListA := []string{req.Targets[i].Payload.TimestampBinName}
	target := req.Targets[i]
	for _, bin := range target.Payload.Bins {
		if inslice.HasString(binListA, bin.Name) {
			continue
		}
		binListA = append(binListA, bin.Name)
	}
	for _, filter := range target.Payload.FilterVariables {
		if val, ok := req.selectedVars[filter.Name]; ok && len(val) == 1 && val[0] == "NONE" {
			emptyResponse := []*timeseriesResponse{}
			logger.Detail("Build query abort - NONE selected in filter (type:timeseries) (remote:%s)", remote)
			return emptyResponse, nil
		}
		if inslice.HasString(binListA, filter.Name) {
			continue
		}
		binListA = append(binListA, filter.Name)
	}
	for _, g := range target.Payload.GroupBy {
		if inslice.HasString(binListA, g.Name) {
			continue
		}
		binListA = append(binListA, g.Name)
	}
	// statement and timestamp filter
	stmt := aerospike.NewStatement(p.config.Aerospike.Namespace, target.Target, binListA...)
	aerr := stmt.SetFilter(aerospike.NewRangeFilter(target.Payload.TimestampBinName, req.Range.From.UnixMilli(), req.Range.To.UnixMilli()))
	if aerr != nil {
		return nil, fmt.Errorf("error creating aerospike filter: %s", aerr)
	}
	// build expression
	var exp []*aerospike.Expression
	var new *aerospike.Expression
	var vals []*aerospike.Expression
	var valsOr *aerospike.Expression
	// expression: filter variables
	for _, filter := range target.Payload.FilterVariables {
		if _, ok := req.selectedVars[filter.Name]; !ok {
			return nil, fmt.Errorf("variable %s does not exist", filter.Name)
		}
		new = nil
		vals = nil
		p.cache.lock.RLock()
		for _, v := range req.selectedVars[filter.Name] {
			if n, ok := p.cache.metadata[filter.Name]; !ok || n == nil {
				logger.Detail("Grafana requsted a filter which results in nil dereference, skipping (ok:%t filter.Name:%s v:%s)", ok, filter.Name, v)
				continue
			}
			idxval := inslice.StringMatch(p.cache.metadata[filter.Name].Entries, v)
			if idxval == -1 {
				continue
			}
			vals = append(vals, aerospike.ExpEq(aerospike.ExpIntBin(filter.Name), aerospike.ExpIntVal(int64(idxval))))
		}
		p.cache.lock.RUnlock()
		if len(vals) == 0 {
			continue
		}
		valsOr = nil
		valsOr = vals[0]
		if len(vals) > 1 {
			valsOr = aerospike.ExpOr(vals...)
		}
		if filter.MustExist {
			new = aerospike.ExpAnd(aerospike.ExpBinExists(filter.Name), valsOr)
		} else {
			new = aerospike.ExpOr(aerospike.ExpNot(aerospike.ExpBinExists(filter.Name)), valsOr)
		}
		exp = append(exp, new)
	}
	// data bin list and expression bin selection
	binList := []string{}
	for _, bin := range target.Payload.Bins {
		binList = append(binList, bin.Name)
		if !bin.Required {
			continue
		}
		// for bin selections, check if it exists, or otherwise error that stat is not found
		p.cache.lock.RLock()
		if !inslice.HasString(p.cache.binNames, bin.Name) {
			p.cache.lock.RUnlock()
			if bin.DisplayName == "" {
				bin.DisplayName = bin.Name
			}
			return nil, fmt.Errorf("statistic bin %s (%s) not found", bin.DisplayName, bin.Name)
		}
		p.cache.lock.RUnlock()
		// expression fill
		exp = append(exp, aerospike.ExpBinExists(bin.Name))
	}
	// group list and expression group by
	groupList := []string{}
	for _, bin := range target.Payload.GroupBy {
		groupList = append(groupList, bin.Name)
		if !bin.Required {
			continue
		}
		exp = append(exp, aerospike.ExpBinExists(bin.Name))
	}
	// query
	qp := p.queryPolicy()
	if len(exp) == 1 {
		qp.FilterExpression = exp[0]
	} else {
		qp.FilterExpression = aerospike.ExpAnd(exp...)
	}
	logger.Detail("Query start (type:timeseries) (remote:%s) (buildTime:%s)", remote, time.Since(ntime).String())
	ntime = time.Now()
	recset, aerr := p.db.Query(qp, stmt)
	if aerr != nil {
		return nil, fmt.Errorf("%s", aerr)
	}
	timedIsCancelled := make(chan bool, 1)
	timedIsEnd := make(chan bool, 1)
	defer func() {
		timedIsEnd <- true
	}()
	go p.timedCheckSocketTimeout(r.Context(), recset, timedIsCancelled, timedIsEnd)
	resp := []*timeseriesResponse{}
	logger.Detail("Enum results (type:timeseries) (remote:%s) (runQueryTime:%s)", remote, time.Since(ntime).String())
	ntime = time.Now()
	datapointCount := 0
	p.cache.lock.RLock()
	ptime1 := time.Duration(0)
	ptime2 := time.Duration(0)
	ptime3 := time.Duration(0)
	var ptime time.Time
	for rec := range recset.Results() {
		if len(timedIsCancelled) > 0 {
			p.cache.lock.RUnlock()
			return nil, errors.New("socket closed by client while enumerating")
		}
		dp := &datapoint{
			datapoints: make(map[string]point),
			groups:     make([]*timeseriesGroup, 0, len(groupList)),
		}
		if rec.Err != nil {
			p.cache.lock.RUnlock()
			return nil, fmt.Errorf("%s", rec.Err)
		}
		if p.config.LogLevel > 5 {
			ptime = time.Now()
		}
		for k, v := range rec.Record.Bins {
			if k == target.Payload.TimestampBinName {
				dp.timestampMs = v.(int)
				continue
			}
			if inslice.HasString(groupList, k) {
				if _, ok := p.cache.metadata[k]; !ok {
					p.cache.lock.RUnlock()
					return nil, fmt.Errorf("metadata for item %s not found, metadata corrupt or log ingestion in progress", k)
				}
				if len(p.cache.metadata[k].Entries) <= v.(int) {
					p.cache.lock.RUnlock()
					return nil, fmt.Errorf("metadata entry at index %v for item %s not found, metadata corrupt or log ingestion in progress", v, k)
				}
				dp.groups = append(dp.groups, &timeseriesGroup{
					name:  k,
					value: p.cache.metadata[k].Entries[v.(int)],
				})
				continue
			}
			if !inslice.HasString(binList, k) {
				continue
			}
			displayName := ""
			for _, bin := range target.Payload.Bins {
				if bin.Name == k {
					displayName = bin.DisplayName
					break
				}
			}
			switch vv := v.(type) {
			case int64:
				dp.datapoints[displayName] = point{
					value:   float64(vv),
					binName: k,
				}
			case int:
				dp.datapoints[displayName] = point{
					value:   float64(vv),
					binName: k,
				}
			case float64:
				dp.datapoints[displayName] = point{
					value:   vv,
					binName: k,
				}
			case string:
				vva, err := strconv.ParseFloat(vv, 64)
				if err == nil {
					dp.datapoints[displayName] = point{
						value:   vva,
						binName: k,
					}
				}
			}
			datapointCount++
			if datapointCount > p.config.MaxDataPointsReceived {
				p.cache.lock.RUnlock()
				return resp, errors.New("too many datapoints received, limit data by zooming in or selecting dropdown filters")
			}
		}
		if p.config.LogLevel > 5 {
			ptime1 += time.Since(ptime)
			ptime = time.Now()
		}
		// sort dp groups
		sort.Slice(dp.groups, func(i, j int) bool {
			idxI := -1
			idxJ := -1
			for gi, gg := range groupList {
				if gg == dp.groups[i].name {
					idxI = gi
				} else if gg == dp.groups[j].name {
					idxJ = gi
				}
				if idxI > -1 && idxJ > -1 {
					break
				}
			}
			return idxI < idxJ
		})
		if p.config.LogLevel > 5 {
			ptime2 += time.Since(ptime)
			ptime = time.Now()
		}
		// convert to resp response type
		for k, v := range dp.datapoints {
			groupHash := sha256.New()
			dpGroups := make([]string, 0, len(dp.groups)+1)
			if p.config.TimeseriesDisplayNameFirst && k != "" {
				dpGroups = append(dpGroups, k)
				groupHash.Write(sbs.StringToByteSlice(k))
			}
			for _, g := range dp.groups {
				if g.value != "" {
					dpGroups = append(dpGroups, g.value)
					groupHash.Write(sbs.StringToByteSlice(g.value))
				}
			}
			if !p.config.TimeseriesDisplayNameFirst && k != "" {
				dpGroups = append(dpGroups, k)
				groupHash.Write(sbs.StringToByteSlice(k))
			}
			grHash := groupHash.Sum(nil)
			found := -1
			for i := range resp {
				if !bytes.Equal(resp[i].groupHash, grHash) {
					continue
				}
				found = i
			}
			if found < 0 {
				found = len(resp)
				if len(resp) == p.config.MaxSeriesPerGraph {
					p.cache.lock.RUnlock()
					return resp, errors.New("too many series for graph, reduce series by selecting dropdown filters")
				}
				resp = append(resp, &timeseriesResponse{
					Datapoints: []*responsePoint{},
					groups:     dpGroups,
					groupHash:  grHash,
					Target:     strings.Join(dpGroups, p.config.TimeseriesLegendSeparator),
					binIdx:     inslice.StringMatch(binList, v.binName),
				})
			}
			val := float64(v.value)
			ts := float64(dp.timestampMs)
			resp[found].Datapoints = append(resp[found].Datapoints, &responsePoint{point: []float64{val, ts}})
		}
		if p.config.LogLevel > 5 {
			ptime3 += time.Since(ptime)
		}
	}
	p.cache.lock.RUnlock()

	logger.Detail("Sort by time (type:timeseries) (remote:%s) (datapoints:%d) (enumTime:%s) (binListTime:%s) (dpSortTime:%s) (dp2respTime:%s) (waitOnAerospikeTime:%s)", remote, datapointCount, time.Since(ntime).String(), ptime1.String(), ptime2.String(), ptime3.String(), time.Duration(time.Since(ntime)-ptime1-ptime2-ptime3).String())
	ntime = time.Now()
	for ri := range resp {
		sort.Slice(resp[ri].Datapoints, func(i, j int) bool {
			return resp[ri].Datapoints[i].point[1] < resp[ri].Datapoints[j].point[1]
		})
	}
	if len(timedIsCancelled) > 0 {
		return nil, errors.New("socket closed by client after data sort")
	}

	logger.Detail("Sort legend (type:timeseries) (remote:%s) (datapoints:%d) (timeSortTime:%s)", remote, datapointCount, time.Since(ntime).String())
	ntime = time.Now()
	sort.Slice(resp, func(i, j int) bool {
		return resp[i].Target < resp[j].Target
	})
	if len(timedIsCancelled) > 0 {
		return nil, errors.New("socket closed by client after legend sort")
	}

	logger.Detail("Post-processing (type:timeseries) (remote:%s) (datapoints:%d) (legendSortTime:%s)", remote, datapointCount, time.Since(ntime).String())
	ntime = time.Now()
	reduceIntervalWindow := req.Range.To.Sub(req.Range.From).Milliseconds() / int64(req.MaxDataPoints)
	if reduceIntervalWindow > int64(req.IntervalMs) {
		reduceIntervalWindow = int64(req.IntervalMs)
	}
	reduceIntervalWindow *= 2 // 2 real datapoints per window
	datapointCount = 0
	nullDatapoints := 0
	for ri := range resp {
		datapoints := []*responsePoint{}
		lastPointTime := float64(-1)
		lastValue := float64(0)
		isFirstValue := true
		windowStartTime := float64(0)
		windowMinPoint := []float64{}
		windowMaxPoint := []float64{}
		windowNullTs := []float64{}
		for _, point := range resp[ri].Datapoints {
			if len(timedIsCancelled) > 0 {
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
				logger.Detail("(type:timeseries) (remote:%s) duplicate datapoint detected, skipping: PointTime=%0.1f", remote, lastPointTime)
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
			sse := singularSeriesExtend(target.Payload.Bins[resp[ri].binIdx].SingularSeriesExtend, datapoints[0].point)
			if len(sse) > 0 {
				datapoints = []*responsePoint{sse[0], datapoints[0], sse[1]}
			}
		}
		// save
		resp[ri].Datapoints = datapoints
	}
	logger.Detail("Return values (type:timeseries) (remote:%s) (reduceWindowMs:%d) (datapoints:%d) (nullpoints:%d) (postProcessTime:%s)", remote, reduceIntervalWindow, datapointCount, nullDatapoints, time.Since(ntime).String())
	return resp, nil
}

func singularSeriesExtend(extender interface{}, point []float64) []*responsePoint {
	defaultPoints := []*responsePoint{
		{
			isDataNull: false,
			point:      []float64{float64(0), point[1] - 500},
		}, {
			isDataNull: false,
			point:      []float64{float64(0), point[1] + 500},
		},
	}
	switch sse := extender.(type) {
	case int:
		return []*responsePoint{
			{
				isDataNull: false,
				point:      []float64{float64(sse), point[1] - 500},
			}, {
				isDataNull: false,
				point:      []float64{float64(sse), point[1] + 500},
			},
		}
	case float64:
		return []*responsePoint{
			{
				isDataNull: false,
				point:      []float64{float64(sse), point[1] - 500},
			}, {
				isDataNull: false,
				point:      []float64{float64(sse), point[1] + 500},
			},
		}
	case string:
		if strings.ToUpper(sse) == "REPEAT" {
			return []*responsePoint{
				{
					isDataNull: false,
					point:      []float64{float64(point[0]), point[1] - 500},
				}, {
					isDataNull: false,
					point:      []float64{float64(point[0]), point[1] + 500},
				},
			}
		} else if strings.HasPrefix(strings.ToUpper(sse), "DISABLE") {
			return nil
		} else if sseNo, ok := stringToFloat(sse); ok {
			return []*responsePoint{
				{
					isDataNull: false,
					point:      []float64{float64(sseNo), point[1] - 500},
				}, {
					isDataNull: false,
					point:      []float64{float64(sseNo), point[1] + 500},
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

func getDatapoints(windowMinPoint []float64, windowMaxPoint []float64, windowNullTs []float64, extender interface{}) (datapoints []*responsePoint, dpCount int, nullCount int) {
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
		datapoints = append(datapoints, &responsePoint{point: []float64{0, nullTsBefore}, isDataNull: true})
		nullCount++
	}
	if windowMinPoint[1] < windowMaxPoint[1] {
		datapoints = append(datapoints, &responsePoint{point: []float64{windowMinPoint[0], windowMinPoint[1]}})
		dpCount++
	} else {
		datapoints = append(datapoints, &responsePoint{point: []float64{windowMaxPoint[0], windowMaxPoint[1]}})
		dpCount++
	}
	if nullTsMid > -1 {
		datapoints = append(datapoints, &responsePoint{point: []float64{0, nullTsMid}, isDataNull: true})
		nullCount++
	}
	if windowMinPoint[1] > windowMaxPoint[1] {
		datapoints = append(datapoints, &responsePoint{point: []float64{windowMinPoint[0], windowMinPoint[1]}})
		dpCount++
	} else if windowMinPoint[1] < windowMaxPoint[1] {
		datapoints = append(datapoints, &responsePoint{point: []float64{windowMaxPoint[0], windowMaxPoint[1]}})
		dpCount++
	}
	if nullTsAfter > -1 {
		datapoints = append(datapoints, &responsePoint{point: []float64{0, nullTsAfter}, isDataNull: true})
		nullCount++
	}
	if nullTsBefore > -1 && nullTsAfter > -1 && dpCount == 1 {
		// single datapoint between 2 nulls, add sse
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			datapoints = []*responsePoint{datapoints[0], sse[0], datapoints[1], sse[1], datapoints[2]}
		}
	} else if nullTsAfter > -1 && dpCount == 1 {
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			datapoints = []*responsePoint{datapoints[0], sse[1], datapoints[1]}
		}
	} else if nullTsBefore > -1 && dpCount == 1 {
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			datapoints = []*responsePoint{datapoints[0], sse[0], datapoints[1]}
		}
	}
	if nullTsBefore > -1 && nullTsMid > -1 {
		// add sse around the first datapoint[1] as [0] is null
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			dpTemp := datapoints[2:]
			datapoints = []*responsePoint{datapoints[0], sse[0], datapoints[1], sse[1]}
			datapoints = append(datapoints, dpTemp...)
		}
	} else if nullTsMid > -1 {
		// before is not null but mid is, add just to the right of data
		sse := singularSeriesExtend(extender, windowMinPoint)
		if len(sse) > 0 {
			dpTemp := datapoints[1:]
			datapoints = []*responsePoint{datapoints[0], sse[0]}
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
	groups      []*timeseriesGroup
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
