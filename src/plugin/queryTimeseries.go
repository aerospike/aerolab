package plugin

type timeseriesResponse struct {
	Target     string  `json:"target"`
	Datapoints [][]int `json:"datapoints"` // list of int tuples, [][data,timestamp]
}

func (p *Plugin) handleQueryTimeseries(req *queryRequest, i int, remote string) ([]*timeseriesResponse, error) {
	return nil, nil
}
