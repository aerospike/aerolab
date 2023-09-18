package plugin

import (
	"fmt"

	"github.com/aerospike/aerospike-client-go/v6"
)

type timeseriesResponse struct {
	Target     string   `json:"target"`
	Datapoints [][]int  `json:"datapoints"` // list of int tuples, [][data,timestamp]
	groups     []string // used for response grouping
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
	binList := []string{req.Targets[i].Payload.TimestampBinName}
	target := req.Targets[i]
	for _, bin := range target.Payload.Bins {
		binList = append(binList, bin.Name)
	}
	for _, filter := range target.Payload.FilterVariables {
		binList = append(binList, filter.Name)
	}
	stmt := aerospike.NewStatement(p.config.Aerospike.Namespace, target.Target, binList...)
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
		for _, v := range req.selectedVars[filter.Name] {
			vals = append(vals, aerospike.ExpEq(aerospike.ExpStringBin(filter.Name), aerospike.ExpStringVal(v)))
		}
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
	for _, bin := range target.Payload.Bins {
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
	recset, aerr := p.db.Query(qp, stmt)
	if aerr != nil {
		return nil, fmt.Errorf("%s", aerr)
	}
	resp := []*timeseriesResponse{}
	for rec := range recset.Results() {
		if rec.Err != nil {
			return nil, fmt.Errorf("%s", rec.Err)
		}
		bins := rec.Record.Bins
		_ = bins
		// TODO
	}
	_ = resp // TODO remove me
	return nil, nil
}
