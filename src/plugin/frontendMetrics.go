package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bestmethod/logger"
	"github.com/rglonek/sbs"
)

type metricQuery struct {
	Metric  string                 `json:"metric"`
	Payload map[string]interface{} `json:"payload"`
}

type metricResponse struct {
	Label string `json:"label"`
	Value string `json:"value"`
	// builder payloads not supported at this time
}

func (p *Plugin) handleMetrics(w http.ResponseWriter, r *http.Request) {
	logger.Info("QUERY START (type:metrics) (remote:%s)", r.RemoteAddr)
	defer logger.Info("QUERY END (type:metrics) (remote:%s)", r.RemoteAddr)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		responseError(w, http.StatusBadRequest, "Failed to read body (remote:%s) (error:%s)", r.RemoteAddr, err)
		return
	}
	logger.Detail("(remote:%s) (payload:%s)", r.RemoteAddr, sbs.ByteSliceToString(body))
	query := new(metricQuery)
	err = json.Unmarshal(body, query)
	if err != nil {
		responseError(w, http.StatusBadRequest, "Failed to unmarshal json request (remote:%s) (error:%s)", r.RemoteAddr, err)
		return
	}
	p.cache.lock.RLock()
	response := make([]*metricResponse, len(p.cache.setNames))
	for i, setName := range p.cache.setNames {
		response[i] = &metricResponse{
			Label: setName,
			Value: setName,
		}
	}
	p.cache.lock.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(response)
}

func responseError(w http.ResponseWriter, httpStatus int, message string, tail ...interface{}) {
	logger.Warn(message, tail...)
	w.WriteHeader(httpStatus)
	w.Write(sbs.StringToByteSlice(fmt.Sprintf(message, tail...)))
}
