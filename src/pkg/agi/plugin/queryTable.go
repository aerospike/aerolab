package plugin

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"log"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

type tableResponse struct {
	Type    string         `json:"type"` // must be "table"
	Columns []*tableColumn `json:"columns"`
	Rows    [][]any        `json:"rows"` // a list of column data
}

type tableColumn struct {
	Text    string `json:"text"`
	binName string
	Type    string `json:"type"` // time, string, number
}

func (p *Plugin) handleQueryTable(ctx context.Context, req *queryRequest, i int, remote string) (*tableResponse, error) {
	log.Printf("DETAIL: Build query (type:table) (remote:%s)", remote)
	binList := []string{}
	target := req.Targets[i]
	for _, bin := range target.Payload.Bins {
		binList = append(binList, bin.Name)
	}
	for _, filter := range target.Payload.FilterVariables {
		binList = append(binList, filter.Name)
	}
	// Decide how to materialize filter values based on the target
	// set's schema. Ingest stores categorical labels as integer
	// indices into metadata[filter.Name].Entries (see
	// processlogs.go buildDataRow / data[k]=idx). Non-metric sets
	// — notably collectinfo — store raw strings. Comparing a
	// Str against an int64 column (or vice versa) matches nothing
	// and silently returns empty results; we must pick the right
	// materialization per filter column.
	//
	// Strategy: ask the db for the column type. If it reports
	// TypeInt64 we translate via the cache's Entries slice exactly
	// like queryTimeseries does; if it reports TypeString (or the
	// schema has no such column yet, e.g. the column only exists
	// for a minority of rows) we fall back to Str. Unknown
	// columns default to Str since that's the historical behavior
	// for collectinfo-style sets.
	colTypes := map[string]db.ColumnType{}
	if schema, ok := p.db.SchemaOf(target.Target); ok {
		for _, c := range schema {
			colTypes[c.Name] = c.Type
		}
	}
	var exps []db.Expr
	for _, filter := range target.Payload.FilterVariables {
		if _, ok := req.selectedVars[filter.Name]; !ok {
			return nil, fmt.Errorf("variable %s does not exist", filter.Name)
		}
		var vals []db.Value
		if colTypes[filter.Name] == db.TypeInt64 {
			p.cache.lock.RLock()
			meta := p.cache.metadata[filter.Name]
			for _, v := range req.selectedVars[filter.Name] {
				if meta == nil {
					log.Printf("DETAIL: table filter %s has int schema but no metadata cache, skipping value %q", filter.Name, v)
					continue
				}
				idx := slices.Index(meta.Entries, v)
				if idx == -1 {
					continue
				}
				vals = append(vals, db.Int(int64(idx)))
			}
			p.cache.lock.RUnlock()
		} else {
			for _, v := range req.selectedVars[filter.Name] {
				vals = append(vals, db.Str(v))
			}
		}
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
	for _, bin := range target.Payload.Bins {
		if !bin.Required {
			continue
		}
		exps = append(exps, db.Exists(bin.Name))
	}
	q := p.db.Query(target.Target).Project(binList...)
	if len(exps) == 1 {
		q = q.Where(exps[0])
	} else if len(exps) > 1 {
		q = q.Where(db.And(exps...))
	}
	log.Printf("DETAIL: Run query (type:table) (remote:%s)", remote)
	it := q.Run(ctx)
	defer it.Close()
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
	log.Printf("DETAIL: Enum results (type:table) (remote:%s)", remote)
	// Reusable Row passed to ReadInto; the iterator clears and
	// refills it in place each call instead of allocating a fresh
	// map[string]Value per row inside Record().
	dst := make(db.Row, len(binList))
	for it.Next() {
		_, bins := it.ReadInto(dst)
		row := make([]any, 0, len(resp.Columns))
		for _, col := range resp.Columns {
			if v, ok := bins[col.binName]; ok {
				row = append(row, valueToAny(v))
				continue
			}
			row = append(row, "")
		}
		resp.Rows = append(resp.Rows, row)
	}
	if err := it.Err(); err != nil {
		return nil, fmt.Errorf("query iterator: %s", err)
	}
	// sort
	log.Printf("DETAIL: Sort data (type:table) (remote:%s)", remote)
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
				case int64:
					switch vj := nj[so].(type) {
					case int64:
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
		resp.Rows = [][]any{}
	}
	log.Printf("DETAIL: Return data (type:table) (remote:%s)", remote)
	return resp, nil
}

// valueToAny coerces a db.Value back to the bare Go type Grafana's table
// response expects.
func valueToAny(v db.Value) any {
	switch v.Type() {
	case db.TypeInt64:
		iv, _ := v.AsInt()
		return iv
	case db.TypeFloat64:
		fv, _ := v.AsFloat()
		return fv
	case db.TypeString:
		sv, _ := v.AsString()
		return sv
	case db.TypeBytes:
		bv, _ := v.AsBytes()
		return bv
	case db.TypeBool:
		bv, _ := v.AsBool()
		return bv
	}
	return ""
}
