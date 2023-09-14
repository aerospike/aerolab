package plugin

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bestmethod/inslice"
	"github.com/bestmethod/logger"
)

type variableQuery struct {
	Payload struct {
		Target string `json:"target"`
	} `json:"payload"`
	Range struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	} `json:"range"`
}

type variableResponse struct {
	Text  string `json:"__text"`
	Value string `json:"__value"`
}

func (p *Plugin) handleVariable(w http.ResponseWriter, r *http.Request) {
	logger.Info("QUERY START (type:variable) (remote:%s)", r.RemoteAddr)
	defer logger.Info("QUERY END (type:variable) (remote:%s)", r.RemoteAddr)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		responseError(w, http.StatusBadRequest, "Failed to read body (remote:%s) (error:%s)", r.RemoteAddr, err)
		return
	}
	logger.Detail("(remote:%s) (payload:%s)", r.RemoteAddr, string(body))
	query := new(variableQuery)
	err = json.Unmarshal(body, query)
	if err != nil {
		responseError(w, http.StatusBadRequest, "Failed to unmarshal json request (remote:%s) (error:%s)", r.RemoteAddr, err)
		return
	}
	if query.Payload.Target == "" {
		responseError(w, http.StatusBadRequest, "Query does not contain target variable name (remote:%s)", r.RemoteAddr)
		return
	}
	p.cache.lock.RLock()
	q := strings.Split(query.Payload.Target, "@")
	target := q[0]
	clusterNames := []string{}
	if len(q) > 1 {
		cn := strings.Join(q[1:], "@")
		err = json.Unmarshal([]byte(cn), &clusterNames)
		if err != nil {
			responseError(w, http.StatusBadRequest, "Failed to unmarshal json request cluster names (remote:%s) (error:%s)", r.RemoteAddr, err)
			return
		}
	}
	if _, ok := p.cache.metadata[target]; !ok {
		p.cache.lock.RUnlock()
		logger.Warn("Query target %s does not exist (remote:%s)", target, r.RemoteAddr)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
		return
	}
	response := []*variableResponse{}
	for _, item := range p.cache.metadata[target] {
		if len(clusterNames) == 0 || inslice.HasString(clusterNames, item.ClusterName) {
			for _, i := range item.Entries {
				found := false
				for _, l := range response {
					if l.Value == i {
						found = true
						break
					}
				}
				if !found {
					response = append(response, &variableResponse{
						Text:  i,
						Value: i,
					})
				}
			}
			for _, i := range item.StaticEntries {
				found := false
				for _, l := range response {
					if l.Value == i {
						found = true
						break
					}
				}
				if !found {
					response = append(response, &variableResponse{
						Text:  i,
						Value: i,
					})
				}
			}
		}
	}
	p.cache.lock.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(response)
}
