package plugin

import (
	"fmt"
	"sort"

	"github.com/aerospike/aerospike-client-go/v6"
	"github.com/bestmethod/logger"
)

type tableResponse struct {
	Type    string          `json:"type"` // must be "table"
	Columns []*tableColumn  `json:"columns"`
	Rows    [][]interface{} `json:"rows"` // a list of column data
}

type tableColumn struct {
	Text    string `json:"text"`
	binName string
	Type    string `json:"type"` // time, string, number
}

func (p *Plugin) handleQueryTable(req *queryRequest, i int, remote string) (*tableResponse, error) {
	logger.Detail("Build query (type:table) (remote:%s)", remote)
	binList := []string{}
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
	logger.Detail("Run query (type:table) (remote:%s)", remote)
	recset, aerr := p.db.Query(qp, stmt)
	if aerr != nil {
		return nil, fmt.Errorf("%s", aerr)
	}
	resp := &tableResponse{
		Type: "table",
	}
	for _, sel := range req.Targets[i].Payload.Bins {
		name := sel.Name
		if sel.DisplayName != "" {
			name = sel.DisplayName
		}
		resp.Columns = append(resp.Columns, &tableColumn{
			Text:    name,
			binName: sel.Name,
			Type:    sel.Type,
		})
	}
	logger.Detail("Enum results (type:table) (remote:%s)", remote)
	for rec := range recset.Results() {
		if rec.Err != nil {
			return nil, fmt.Errorf("%s", rec.Err)
		}
		bins := rec.Record.Bins
		row := []interface{}{}
		for _, col := range resp.Columns {
			if v, ok := bins[col.binName]; ok {
				row = append(row, v)
				continue
			}
			row = append(row, "")
		}
		resp.Rows = append(resp.Rows, row)
	}
	// sort
	logger.Detail("Sort data (type:table) (remote:%s)", remote)
	if len(target.Payload.SortOrder) > 0 {
		sort.Slice(resp.Rows, func(i, j int) bool {
			for _, so := range target.Payload.SortOrder {
				rev := false
				if so < 0 {
					so = so * -1
					rev = true
				}
				so--
				ni := resp.Rows[i]
				nj := resp.Rows[j]
				switch vi := ni[so].(type) {
				case int:
					switch vj := nj[so].(type) {
					case int:
						if vi < vj {
							return !rev
						} else if vi > vj {
							return rev
						}
					}
				case float64:
					switch vj := nj[so].(type) {
					case float64:
						if vi < vj {
							return !rev
						} else if vi > vj {
							return rev
						}
					}
				case string:
					switch vj := nj[so].(type) {
					case string:
						if vi < vj {
							return !rev
						} else if vi > vj {
							return rev
						}
					}
				}
			}
			return false
		})
	}
	if resp.Rows == nil {
		resp.Rows = [][]interface{}{}
	}
	logger.Detail("Return data (type:table) (remote:%s)", remote)
	return resp, nil
}
