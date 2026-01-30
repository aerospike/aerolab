package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aerospike/aerospike-client-go/v8"
	"log"
	"github.com/rglonek/sbs"
)

func isSocketTimeout(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func (p *Plugin) timedCheckSocketTimeout(ctx context.Context, resultSet *aerospike.Recordset, isCancelled chan bool, end chan bool) {
	for {
		if len(end) > 0 {
			<-end
			return
		}
		if isSocketTimeout(ctx) {
			isCancelled <- true
			if resultSet.IsActive() {
				resultSet.Close()
			}
			for _, node := range p.db.GetNodes() {
				node.RequestInfo(p.ip, fmt.Sprintf("jobs:module=query;cmd=kill-job;trid=%d", resultSet.TaskId()))
			}
			return
		}
		time.Sleep(time.Second)
	}
}

func (p *Plugin) handleQuery(w http.ResponseWriter, r *http.Request) {
	log.Printf("INFO: QUERY INCOMING (type:query) (remote:%s)", r.RemoteAddr)
	qtime := time.Now()
	p.requests <- true
	defer func() {
		<-p.requests
		log.Printf("INFO: QUERY END (type:query) (runningRequests:%d) (runningJobs:%d) (remote:%s) (totalTime:%s)", len(p.requests), len(p.jobs), r.RemoteAddr, time.Since(qtime).String())
	}()
	log.Printf("INFO: QUERY START (type:query) (runningRequests:%d) (runningJobs:%d) (remote:%s) (waitTime:%s)", len(p.requests), len(p.jobs), r.RemoteAddr, time.Since(qtime).String())
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		responseError(w, http.StatusBadRequest, "Failed to read body (remote:%s) (error:%s)", r.RemoteAddr, err)
		return
	}
	log.Printf("DETAIL: (remote:%s) (payload:%s)", r.RemoteAddr, sbs.ByteSliceToString(body))
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
		log.Printf("DETAIL: (remote:%s) (parsed-payload:%s) (selected-vars:%s)", r.RemoteAddr, sbs.ByteSliceToString(body), sbs.ByteSliceToString(bodyx))
	}
	log.Printf("INFO: QUERY ALLOCATE_JOB (type:query) (runningJobs:%d) (remote:%s)", len(p.jobs), r.RemoteAddr)
	jtime := time.Now()
	p.jobs <- true
	defer func() {
		<-p.jobs
	}()
	log.Printf("INFO: QUERY DO_JOB (type:query) (runningJobs:%d) (remote:%s) (waitTime:%s)", len(p.jobs), r.RemoteAddr, time.Since(jtime).String())
	dtime := time.Now()
	responses := []interface{}{}
	for i := range req.Targets {
		if isSocketTimeout(r.Context()) {
			err = r.Context().Err()
			errString := "success"
			if err != nil {
				errString = err.Error()
			}
			log.Printf("WARN: QUERY GRAFANA SOCKET TIMEOUT (type:query) (remote:%s): client terminated connection: %s", r.RemoteAddr, errString)
			return
		}
		switch req.Targets[i].Payload.Type {
		case "":
			fallthrough
		case "timeseries":
			resp, err := p.handleQueryTimeseries(req, i, r.RemoteAddr, r)
			if err != nil {
				responseError(w, http.StatusBadRequest, "Request target timeseries %d (%s:%s) (remote:%s) (error:%s)", i, req.Targets[i].RefId, req.Targets[i].Target, r.RemoteAddr, err)
				return
			}
			for _, ri := range resp {
				responses = append(responses, ri)
			}
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
	log.Printf("INFO: QUERY SEND_DATA (type:query) (remote:%s) (db.Get.Time:%s)", r.RemoteAddr, time.Since(dtime).String())
	stime := time.Now()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(responses)
	log.Printf("INFO: QUERY SENT (type:query) (remote:%s) (sendTime:%s)", r.RemoteAddr, time.Since(stime).String())
}
