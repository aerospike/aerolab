package plugin

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aerospike/aerospike-client-go/v6"
	"github.com/bestmethod/inslice"
	"github.com/bestmethod/logger"
)

type timeseriesResponse struct {
	Target     string   `json:"target"`
	Datapoints [][]*int `json:"datapoints"` // list of int tuples, [][data,timestamp]
	groups     []string // used for response grouping
	binIdx     int
}

// TODO group tracking to make iteration faster (group2timeserieResponseIndex) using some sort of map
// TODO if len([]*timesriesResponse) goes over X max series (configurable, default=1000), bail
// TODO remember that for metadata bins, only their label index is in the record, the actual label value is in the metadata - look that up
// TODO since storage is on file, and file-based storage uses page caching, do we need data-in-memory? how much (if) is it slower if we are processing data that will fit in RAM? note: keep indexes in RAM for sanity sake
/*
optimisation 1 - avoid sending too many nulls and perform reduction by interval window instead of reducing using datapoint counts:
for interval:
  window=interval*3
  in window:
    find min datapoint
    find max datapoint
    find median datapoint
    if data missing (maxIntervalSeconds):
      insert 'null' value in the right place (before/between/after min/max) where missing datapoint is discovered
        (if missing in multiple places, just insert between min and max?)
      -> ship min, max and null datapoints
    if no data is missing (no missed tickers):
      -> ship min, max, median datapoints
*/
func (p *Plugin) handleQueryTimeseries(req *queryRequest, i int, remote string) ([]*timeseriesResponse, error) {
	logger.Detail("Build query (type:timeseries) (remote:%s)", remote)
	ntime := time.Now()
	binListA := []string{req.Targets[i].Payload.TimestampBinName}
	target := req.Targets[i]
	for _, bin := range target.Payload.Bins {
		if inslice.HasString(binListA, bin.Name) {
			continue
		}
		binListA = append(binListA, bin.Name)
	}
	for _, filter := range target.Payload.FilterVariables {
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
	stmt := aerospike.NewStatement(p.config.Aerospike.Namespace, target.Target, binListA...)
	aerr := stmt.SetFilter(aerospike.NewRangeFilter(target.Payload.TimestampBinName, req.Range.From.UnixMilli(), req.Range.To.UnixMilli()))
	if aerr != nil {
		return nil, fmt.Errorf("error creating aerospike filter: %s", aerr)
	}
	var exp *aerospike.Expression
	var new *aerospike.Expression
	var vals []*aerospike.Expression
	var valsOr *aerospike.Expression
	for _, filter := range target.Payload.FilterVariables {
		if _, ok := req.selectedVars[filter.Name]; !ok {
			return nil, fmt.Errorf("variable %s does not exist", filter.Name)
		}
		new = nil
		vals = nil
		p.cache.lock.RLock()
		for _, v := range req.selectedVars[filter.Name] {
			idxval := inslice.StringMatch(p.cache.metadata[filter.Name].Entries, v)
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
		if exp == nil {
			exp = new
		} else {
			exp = aerospike.ExpAnd(exp, new)
		}
	}
	binList := []string{}
	for _, bin := range target.Payload.Bins {
		binList = append(binList, bin.Name)
		if !bin.Required {
			continue
		}
		if exp == nil {
			exp = aerospike.ExpBinExists(bin.Name)
		} else {
			exp = aerospike.ExpAnd(exp, aerospike.ExpBinExists(bin.Name))
		}
	}
	groupList := []string{}
	for _, bin := range target.Payload.GroupBy {
		groupList = append(groupList, bin.Name)
		if !bin.Required {
			continue
		}
		if exp == nil {
			exp = aerospike.ExpBinExists(bin.Name)
		} else {
			exp = aerospike.ExpAnd(exp, aerospike.ExpBinExists(bin.Name))
		}
	}
	qp := p.queryPolicy()
	qp.FilterExpression = exp
	logger.Detail("Query start (type:timeseries) (remote:%s) (buildTime:%s)", remote, time.Since(ntime).String())
	ntime = time.Now()
	recset, aerr := p.db.Query(qp, stmt)
	if aerr != nil {
		return nil, fmt.Errorf("%s", aerr)
	}
	resp := []*timeseriesResponse{}
	logger.Detail("Enum results (type:timeseries) (remote:%s) (runQueryTime:%s)", remote, time.Since(ntime).String())
	ntime = time.Now()
	datapointCount := 0
	p.cache.lock.RLock()
	for rec := range recset.Results() {
		dp := &datapoint{
			datapoints: make(map[string]point),
			groups:     make([]*timeseriesGroup, 0, len(groupList)),
		}
		if rec.Err != nil {
			p.cache.lock.RUnlock()
			return nil, fmt.Errorf("%s", rec.Err)
		}
		for k, v := range rec.Record.Bins {
			if k == target.Payload.TimestampBinName {
				dp.timestampMs = v.(int)
				continue
			}
			if inslice.HasString(groupList, k) {
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
			dp.datapoints[displayName] = point{
				value:   v.(int),
				binName: k,
			}
			datapointCount++
		}
		// add dp to resp
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
		// convert to resp response type
		for k, v := range dp.datapoints {
			dpGroups := []string{}
			for _, g := range dp.groups {
				if g.value != "" {
					dpGroups = append(dpGroups, g.value)
				}
			}
			if k != "" {
				dpGroups = append(dpGroups, k)
			}
			found := -1
			for i := range resp {
				if !stringSlicesEqual(resp[i].groups, dpGroups) {
					continue
				}
				found = i
			}
			if found < 0 {
				found = len(resp)
				resp = append(resp, &timeseriesResponse{
					Datapoints: [][]*int{},
					groups:     dpGroups,
					Target:     strings.Join(dpGroups, "::"),
					binIdx:     inslice.StringMatch(binList, v.binName),
				})
			}
			val := v.value
			ts := dp.timestampMs
			resp[found].Datapoints = append(resp[found].Datapoints, []*int{&val, &ts})
		}
	}
	p.cache.lock.RUnlock()

	logger.Detail("Sort by time (type:timeseries) (remote:%s) (datapoints:%d) (enumTime:%s)", remote, datapointCount, time.Since(ntime).String())
	for ri := range resp {
		sort.Slice(resp[ri].Datapoints, func(i, j int) bool {
			return *resp[ri].Datapoints[i][1] < *resp[ri].Datapoints[j][1]
		})
	}

	logger.Detail("Sort legend (type:timeseries) (remote:%s) (datapoints:%d) (timeSortTime:%s)", remote, datapointCount, time.Since(ntime).String())
	ntime = time.Now()
	sort.Slice(resp, func(i, j int) bool {
		return resp[i].Target < resp[j].Target
	})
	logger.Detail("Post-processing (type:timeseries) (remote:%s) (datapoints:%d) (legendSortTime:%s)", remote, datapointCount, time.Since(ntime).String())
	ntime = time.Now()
	reduceIntervalWindow := req.Range.To.Sub(req.Range.From).Milliseconds() / int64(req.MaxDataPoints)
	if reduceIntervalWindow > int64(req.IntervalMs) {
		reduceIntervalWindow = int64(req.IntervalMs)
	}
	datapointCount = 0
	nullDatapoints := 0
	for ri := range resp {
		datapoints := [][]*int{}
		lastPointTime := -1
		lastValue := 0
		isFirstValue := true
		for _, point := range resp[ri].Datapoints {
			bin := target.Payload.Bins[resp[ri].binIdx]
			// maxIntervalSeconds breached
			if lastPointTime != -1 && *point[1]-lastPointTime > bin.MaxIntervalSeconds*1000 {
				ts := *point[1]
				datapoints = append(datapoints, []*int{nil, &ts})
				nullDatapoints++
			}
			lastPointTime = *point[1]
			val := *point[0]
			// produce delta
			if bin.ProduceDelta {
				if isFirstValue {
					isFirstValue = false
					lastValue = *point[0]
					continue
				}
				val -= lastValue
				lastValue = *point[0]
			}
			// apply reverse
			if bin.Reverse {
				val *= -1
			}
			// apply limits
			if bin.Limits != nil {
				if bin.Limits.MinValue != nil && val < *bin.Limits.MinValue {
					val = *bin.Limits.MinValue
				}
				if bin.Limits.MaxValue != nil && val < *bin.Limits.MaxValue {
					val = *bin.Limits.MaxValue
				}
			}
			// divide by ticker interval (for per/second values)
			if bin.TickerIntervalSeconds != 0 {
				val = val / bin.TickerIntervalSeconds
			}
			// store datapoint
			ts := *point[1]
			datapoints = append(datapoints, []*int{&val, &ts})
			datapointCount++
			// TODO: reduce to reduceIntervalWindow, while ensuring that we do not remove null points
		}
		resp[ri].Datapoints = datapoints
	}
	logger.Detail("Return values (type:timeseries) (remote:%s) (reduceWindowMs:%d) (datapoints:%d) (nullpoints:%d) (postProcessTime:%s)", remote, reduceIntervalWindow, datapointCount, nullDatapoints, time.Since(ntime).String())
	return resp, nil
}

type datapoint struct {
	groups      []*timeseriesGroup
	datapoints  map[string]point
	timestampMs int
}

type point struct {
	value   int
	binName string
}

type timeseriesGroup struct {
	name  string
	value string
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
