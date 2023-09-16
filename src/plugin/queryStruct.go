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
	FilterVariables  []*requestFilter `json:"filterBy"`         // all: which grafana filters to filter by, e.g. ClusterName,NodeIdent
	SortOrder        []int            `json:"sortOrder"`        // all: by which grouping to sort first, and then second, etc
	Bins             []*bin           `json:"bins"`             // all: which bins to plot
	GroupBy          []string         `json:"groupBy"`          // timeseries: which bin values to group by, e.g. ClusterName,NodeIdent
	TimestampBinName string           `json:"timestampBinName"` // timeseries: name of timestamp bin
}

type requestFilter struct {
	Name      string `json:"name"`
	MustExist bool   `json:"mustExist"`
}

type bin struct {
	Name                  string          `json:"name"`                  // all: bin name
	DisplayName           string          `json:"displayName"`           // all: display name for legend
	Reverse               bool            `json:"reverse"`               // timeseries: reverse/mirror values (*-1 final results)
	Required              bool            `json:"required"`              // timeseries: fail if bin not found
	ProduceDelta          bool            `json:"produceDelta"`          // timeseries: for translating cumulative values to per/ticker
	TickerIntervalSeconds int             `json:"tickerIntervalSeconds"` // timeseries: set to translate per/ticker to per/second, value x=0 disables
	MaxIntervalSeconds    int             `json:"maxIntervalSeconds"`    // timeseries: if breached, will insert 'null', value=0 disables
	Limits                *responseLimits `json:"limits"`                // timeseries: floor/ceil at limit
}

type responseLimits struct {
	MinValue *int `json:"minValue"` // if below, will apply minValue
	MaxValue *int `json:"maxValue"` // if above, will apply maxValue
}
