package plugin

import "time"

type queryRequest struct {
	RequestId string `json:"requestId"`
	Range     struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	} `json:"range"`
	IntervalMs    int                        `json:"intervalMs"`
	Targets       []*queryTarget             `json:"targets"`
	MaxDataPoints int                        `json:"maxDataPoints"`
	ScopedVars    map[string]*queryScopedVar `json:"scopedVars"`
	selectedVars  map[string][]string        // extracted from ScopedVars
	AdHocFilters  []interface{}              `json:"adHocFilters"` // not implemented
}

type queryTarget struct {
	RefId   string        `json:"refId"`
	Target  string        `json:"target"` // set name
	Payload *queryPayload `json:"payload"`
}

type queryScopedVar struct {
	Value interface{} `json:"value"`
}

type queryPayload struct {
	Type   string   `json:"type"` // timeseries|table|static(serve json)
	Static struct { // file destination for static json serve
		File  string   `json:"file"`
		Name  string   `json:"name"`
		Names []string `json:"names"`
	} `json:"static"`
	FilterVariables  []*requestFilter  `json:"filterBy"`         // all: which grafana filters to filter by, e.g. ClusterName,NodeIdent
	Bins             []*bin            `json:"bins"`             // all: which bins to plot
	SortOrder        []int             `json:"sortOrder"`        // table: by which grouping to sort first, and then second, etc
	GroupBy          []*requestGroupBy `json:"groupBy"`          // timeseries: which bin values to group by, e.g. ClusterName,NodeIdent
	TimestampBinName string            `json:"timestampBinName"` // timeseries: name of timestamp bin
}

type requestGroupBy struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

type requestFilter struct {
	Name      string `json:"name"`
	MustExist bool   `json:"mustExist"`
}

type bin struct {
	Name                 string          `json:"name"`                // all: bin name
	DisplayName          string          `json:"displayName"`         // all: display name for legend
	Type                 string          `json:"type"`                // all: string/number
	Reverse              bool            `json:"reverse"`             // timeseries: reverse/mirror values (*-1 final results)
	Required             bool            `json:"required"`            // timeseries: fail if bin not found
	ProduceDelta         bool            `json:"produceDelta"`        // timeseries: for translating cumulative values to per/ticker
	DeltaToPerSecond     bool            `json:"convertToPerSecond"`  // timeseries: divide stat by timeX-timeY to get per-second values
	MaxIntervalSeconds   int             `json:"maxIntervalSeconds"`  // timeseries: if breached, will insert 'null', value=0 disables
	Limits               *responseLimits `json:"limits"`              // timeseries: floor/ceil at limit
	SingularSeriesExtend interface{}     `json:"singlarSeriesExtend"` // timeseries: if series has 1 datapoint only, should we extend by adding data to left and right; either an int value, or "REPEAT" (repeat datapoint), or "DISABLE", enabled as 0 by default
}

type responseLimits struct {
	MinValue            *int `json:"minValue"`            // if below, will apply minValue
	MaxValue            *int `json:"maxValue"`            // if above, will apply maxValue
	ReplaceWithOriginal bool `json:"replaceWithOriginal"` // if set to true, replace with original value, if set to false, replace with min/max value itself
}
