package plugin

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bestmethod/logger"
)

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
	FilterVariables  []string // which grafana filters to filter by, e.g. ClusterName,NodeIdent
	GroupBy          []string // which bin values to group by, e.g. ClusterName,NodeIdent
	SortOrder        []int    // by which grouping to sort first, and then second, etc
	TimestampBinName string   // name of timestamp bin
	Bins             []*bin   // which bins to plot
}

type bin struct {
	Name        string
	DisplayName string
	Reverse     bool // reverse/mirror values
	Required    bool
}

func (p *Plugin) handleQuery(w http.ResponseWriter, r *http.Request) {
	logger.Info("QUERY START (type:query) (remote:%s)", r.RemoteAddr)
	defer logger.Info("QUERY END (type:query) (remote:%s)", r.RemoteAddr)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		responseError(w, http.StatusBadRequest, "Failed to read body (remote:%s) (error:%s)", r.RemoteAddr, err)
		return
	}
	logger.Detail("(remote:%s) (payload:%s)", r.RemoteAddr, string(body))
	req := new(queryRequest)
	err = json.Unmarshal(body, req)
	if err != nil {
		responseError(w, http.StatusBadRequest, "Failed to unmarshal body json (remote:%s) (error:%s)", r.RemoteAddr, err)
		return
	}
	req.selectedVars = make(map[string][]string)
	for n, v := range req.ScopedVars {
		if strings.HasPrefix(n, "__") {
			continue
		}
		switch vv := v.Value.(type) {
		case string:
			if vv == "[]" {
				req.selectedVars[n] = []string{}
			} else {
				req.selectedVars[n] = []string{vv}
			}
		case []interface{}:
			for _, item := range vv {
				switch vva := item.(type) {
				case string:
					if _, ok := req.selectedVars[n]; !ok {
						req.selectedVars[n] = []string{}
					}
					req.selectedVars[n] = append(req.selectedVars[n], vva)
				default:
					responseError(w, http.StatusBadRequest, "Failed to unmarshal Scoped Vars json (remote:%s) (error:incorrect list value type:%T)", r.RemoteAddr, v.Value)
					return
				}
			}
		default:
			responseError(w, http.StatusBadRequest, "Failed to unmarshal Scoped Vars json (remote:%s) (error:incorrect type:%T)", r.RemoteAddr, v.Value)
			return
		}
	}
	if p.config.LogLevel > 5 {
		body, err = json.Marshal(req)
		if err != nil {
			responseError(w, http.StatusBadRequest, "Failed to marshal body json for detail logging (remote:%s) (error:%s)", r.RemoteAddr, err)
			return
		}
		bodyx, err := json.Marshal(req.selectedVars)
		if err != nil {
			responseError(w, http.StatusBadRequest, "Failed to marshal body json of scoped vars for detail logging (remote:%s) (error:%s)", r.RemoteAddr, err)
			return
		}
		logger.Detail("(remote:%s) (parsed-payload:%s) (selected-vars:%s)", r.RemoteAddr, string(body), string(bodyx))
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("[]"))
}
