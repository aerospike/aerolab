package plugin

type tableResponse struct {
	Type    string          `json:"type"` // must be "table"
	Columns []*tableColumn  `json:"columns"`
	Rows    [][]interface{} `json:"rows"` // a list of column data
}

type tableColumn struct {
	Text string `json:"text"`
	Type string `json:"type"` // time, string, number
}

func (p *Plugin) handleQueryTable(req *queryRequest, i int, remote string) ([]*tableResponse, error) {
	return nil, nil
}
