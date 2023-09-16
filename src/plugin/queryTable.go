package plugin

import (
	"fmt"
	"sort"

	"github.com/aerospike/aerospike-client-go/v6"
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
	binList := []string{}
	target := req.Targets[i]
	for _, bin := range target.Payload.Bins {
		binList = append(binList, bin.Name)
	}
	for _, filter := range target.Payload.FilterVariables {
		binList = append(binList, filter.Name)
	}
	stmt := aerospike.NewStatement(p.config.Aerospike.Namespace, target.Target, binList...)
	exp := aerospike.ExpBinExists("")
	for _, filter := range target.Payload.FilterVariables {
		if _, ok := req.selectedVars[filter.Name]; !ok {
			return nil, fmt.Errorf("variable %s does not exist", filter.Name)
		}
		// TODO: fill exp with filter vars, ensure if var value is '[]', we should just set mustExist accordingly, and not worry about the filter matching value
		// TODO: for filter lists, check if exists is MustExist is set; for binList, check if exists if Required is true
	}
	qp := p.queryPolicy()
	qp.FilterExpression = exp
	recset, aerr := p.db.Query(qp, stmt)
	if aerr != nil {
		return nil, fmt.Errorf("%s", aerr)
	}
	resp := &tableResponse{
		Type: "table",
	}
	for rec := range recset.Results() {
		if rec.Err != nil {
			return nil, fmt.Errorf("%s", rec.Err)
		}
		bins := rec.Record.Bins
		for k, v := range bins {
			found := false
			for _, c := range resp.Columns {
				if c.binName == k {
					found = true
					break
				}
			}
			if !found {
				nType := "string"
				switch v.(type) {
				case int, int32, int64, float32, float64:
					nType = "number"
				}
				name := k
				for _, bin := range target.Payload.Bins {
					if bin.Name == k {
						if bin.DisplayName != "" {
							name = bin.DisplayName
						}
						break
					}
				}
				resp.Columns = append(resp.Columns, &tableColumn{
					Text:    name,
					binName: k,
					Type:    nType,
				})
			}
		}
		row := []interface{}{}
		for _, c := range resp.Columns {
			if v, ok := bins[c.binName]; ok {
				row = append(row, v)
			} else {
				if c.Type == "string" {
					row = append(row, "")
				} else {
					row = append(row, 0)
				}
			}
		}
		resp.Rows = append(resp.Rows, row)
	}
	// sort
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
	return resp, nil
}
