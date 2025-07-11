package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	//"github.com/HdrHistogram/hdrhistogram-go"
	"github.com/aerospike/aerospike-client-go/v8"
	"github.com/bestmethod/inslice"
	"github.com/bestmethod/logger"
	"github.com/rglonek/sbs"
)

type HistogramRequest struct {
	Range struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	} `json:"range"`
	Cluster string `json:"cluster"`
	Metric  struct {
		Target string `json:"target"`
		Set    string `json:"set"`
		Name   string `json:"name"`
	} `json:"metric"`
}

func (p *Plugin) handleHistogram(w http.ResponseWriter, r *http.Request) {
	logger.Info("QUERY INCOMING (type:histogram) (remote:%s)", r.RemoteAddr)
	qtime := time.Now()
	p.requests <- true
	defer func() {
		<-p.requests
		logger.Info("QUERY END (type:histogram) (runningRequests:%d) (runningJobs:%d) (remote:%s) (totalTime:%s)", len(p.requests), len(p.jobs), r.RemoteAddr, time.Since(qtime).String())
	}()
	logger.Info("QUERY START (type:histogram) (runningRequests:%d) (runningJobs:%d) (remote:%s) (waitTime:%s)", len(p.requests), len(p.jobs), r.RemoteAddr, time.Since(qtime).String())
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		responseError(w, http.StatusBadRequest, "Failed to read body (remote:%s) (error:%s)", r.RemoteAddr, err)
		return
	}
	logger.Detail("(remote:%s) (payload:%s)", r.RemoteAddr, sbs.ByteSliceToString(body))
	req := new(HistogramRequest)
	err = json.Unmarshal(body, req)
	if err != nil {
		responseError(w, http.StatusBadRequest, "Failed to unmarshal body json (remote:%s) (error:%s)", r.RemoteAddr, err)
		return
	}

	p.cache.lock.RLock()
	defer p.cache.lock.RUnlock()
	if _, ok := p.cache.metadata[req.Metric.Target]; !ok {
		logger.Warn("Query target %s does not exist (remote:%s)", req.Metric.Target, r.RemoteAddr)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
		return
	}

	// Find the index of the metric name in the target
	idxval := inslice.StringMatch(p.cache.metadata[req.Metric.Target].Entries, req.Metric.Name)
	if idxval == -1 {
		responseError(w, http.StatusBadRequest, "Metric %s does not exist in target %s (remote:%s)", req.Metric.Name, req.Metric.Target, r.RemoteAddr)
		return
	}

	// Build the filter expression for the metric
	filter := aerospike.ExpEq(aerospike.ExpIntBin(req.Metric.Target), aerospike.ExpIntVal(int64(idxval)))

	// Filter by cluster
	idxval = inslice.StringMatch(p.cache.metadata["ClusterName"].Entries, req.Cluster)
	if idxval == -1 {
		responseError(w, http.StatusBadRequest, "Cluster %s does not exist (remote:%s)", req.Cluster, r.RemoteAddr)
		return
	}
	clusterfilter := aerospike.ExpAnd(filter, aerospike.ExpEq(aerospike.ExpIntBin("ClusterName"), aerospike.ExpIntVal(int64(idxval))))

	// Aerospike query setup
	binList := []string{"00", "01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "21", "22", "23", "24"}
	stmt := aerospike.NewStatement(p.config.Aerospike.Namespace, req.Metric.Set, binList...)
	dateFilter := aerospike.NewRangeFilter(p.config.Aerospike.TimestampBinName, req.Range.From.UnixMilli(), req.Range.To.UnixMilli())
	aerr := stmt.SetFilter(dateFilter)
	if aerr != nil {
		responseError(w, http.StatusInternalServerError, "Failed to set Aerospike filter: %s", aerr)
		return
	}
	qp := p.queryPolicy()
	qp.FilterExpression = aerospike.ExpAnd(filter, clusterfilter)
	//qp.FilterExpression = clusterfilter
	recset, aerr := p.db.Query(qp, stmt)
	if aerr != nil {
		responseError(w, http.StatusInternalServerError, "Aerospike query error: %s", aerr)
		return
	}

	response := make(map[int64]int64)

	// TODO: group by node identifier
	for rec := range recset.Results() {
		if rec.Err != nil {
			logger.Error("Aerospike record error: %s", rec.Err)
			continue
		}
		key := int64(0)
		for k, v := range rec.Record.Bins {
			if k == "00" {
				key = 0
			} else if k == "01" {
				key = 1
			} else if k == "02" {
				key = 2
			} else if k == "03" {
				key = 4
			} else if k == "04" {
				key = 8
			} else if k == "05" {
				key = 16
			} else if k == "06" {
				key = 32
			} else if k == "07" {
				key = 64
			} else if k == "08" {
				key = 128
			} else if k == "09" {
				key = 256
			} else if k == "10" {
				key = 512
			} else if k == "11" {
				key = 1024
			} else if k == "12" {
				key = 2048
			} else if k == "13" {
				key = 4096
			} else if k == "14" {
				key = 8192
			} else if k == "15" {
				key = 16384
			} else if k == "16" {
				key = 32768
			} else if k == "17" {
				key = 65536
			} else if k == "18" {
				key = 131072
			} else if k == "19" {
				key = 262144
			} else if k == "20" {
				key = 524288
			} else if k == "21" {
				key = 1048576
			} else if k == "22" {
				key = 2097152
			} else if k == "23" {
				key = 4194304
			} else if k == "tail" {
				key = 8388608
			}

			val, err := toInt64(v)
			if err != nil {
				responseError(w, http.StatusInternalServerError, "Invalid value for key %s: %s", k, err)
				return
			}
			if _, ok := response[key]; !ok {
				response[key] = 0
			}
			response[key] += val
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// toInt64 converts various numeric types to int64
func toInt64(v interface{}) (int64, error) {
	switch t := v.(type) {
	case int:
		return int64(t), nil
	case int64:
		return t, nil
	case float64:
		return int64(t), nil
	case float32:
		return int64(t), nil
	case uint64:
		return int64(t), nil
	case uint32:
		return int64(t), nil
	case uint:
		return int64(t), nil
	case string:
		return strconv.ParseInt(t, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported type: %T", v)
	}
}
