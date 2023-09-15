package plugin

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bestmethod/logger"
)

func (p *Plugin) handleQuery(w http.ResponseWriter, r *http.Request) {
	logger.Info("QUERY INCOMING (type:query) (remote:%s)", r.RemoteAddr)
	p.requests <- true
	defer func() {
		<-p.requests
		logger.Info("QUERY END (type:query) (runningRequests:%d) (runningJobs:%d) (remote:%s)", len(p.requests), len(p.jobs), r.RemoteAddr)
	}()
	logger.Info("QUERY START (type:query) (runningRequests:%d) (runningJobs:%d) (remote:%s)", len(p.requests), len(p.jobs), r.RemoteAddr)
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
	logger.Info("QUERY ALLOCATE_JOB (type:query) (runningJobs:%d) (remote:%s)", len(p.jobs), r.RemoteAddr)
	p.jobs <- true
	defer func() {
		<-p.jobs
	}()
	logger.Info("QUERY DO_JOB (type:query) (runningJobs:%d) (remote:%s)", len(p.jobs), r.RemoteAddr)
	responses := []interface{}{}
	for i := range req.Targets {
		switch req.Targets[i].Payload.Type {
		case "":
			fallthrough
		case "timeseries":
			resp, err := p.handleQueryTimeseries(req, i, r.RemoteAddr)
			if err != nil {
				responseError(w, http.StatusBadRequest, "Request target timeseries %d (%s:%s) (remote:%s) (error:%s)", i, req.Targets[i].RefId, req.Targets[i].Target, r.RemoteAddr, err)
				return
			}
			responses = append(responses, resp)
		case "table":
			resp, err := p.handleQueryTable(req, i, r.RemoteAddr)
			if err != nil {
				responseError(w, http.StatusBadRequest, "Request target table %d (%s:%s) (remote:%s) (error:%s)", i, req.Targets[i].RefId, req.Targets[i].Target, r.RemoteAddr, err)
				return
			}
			responses = append(responses, resp)
		case "static":
			resp, err := p.handleQueryStatic(req, i, r.RemoteAddr)
			if err != nil {
				responseError(w, http.StatusBadRequest, "Request target static %d (%s:%s) (remote:%s) (error:%s)", i, req.Targets[i].RefId, req.Targets[i].Target, r.RemoteAddr, err)
				return
			}
			responses = append(responses, resp)
		default:
			responseError(w, http.StatusBadRequest, "Request payload type %s not supported (remote:%s)", req.Targets[i].Payload.Type, r.RemoteAddr)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(responses)
}
