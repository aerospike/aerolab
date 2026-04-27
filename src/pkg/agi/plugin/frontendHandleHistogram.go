package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"log"

	"github.com/aerospike/aerolab/pkg/agi/db"
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
	log.Printf("INFO: QUERY INCOMING (type:histogram) (remote:%s)", r.RemoteAddr)
	qtime := time.Now()
	p.requests <- true
	defer func() {
		<-p.requests
		log.Printf("INFO: QUERY END (type:histogram) (runningRequests:%d) (runningJobs:%d) (remote:%s) (totalTime:%s)", len(p.requests), len(p.jobs), r.RemoteAddr, time.Since(qtime).String())
	}()
	log.Printf("INFO: QUERY START (type:histogram) (runningRequests:%d) (runningJobs:%d) (remote:%s) (waitTime:%s)", len(p.requests), len(p.jobs), r.RemoteAddr, time.Since(qtime).String())
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		responseError(w, http.StatusBadRequest, "Failed to read body (remote:%s) (error:%s)", r.RemoteAddr, err)
		return
	}
	log.Printf("DETAIL: (remote:%s) (payload:%s)", r.RemoteAddr, sbs.ByteSliceToString(body))
	req := new(HistogramRequest)
	err = json.Unmarshal(body, req)
	if err != nil {
		responseError(w, http.StatusBadRequest, "Failed to unmarshal body json (remote:%s) (error:%s)", r.RemoteAddr, err)
		return
	}
	// Histogram queries do real db.Query work just like the timeseries
	// path; gate them on the jobs semaphore (in addition to requests)
	// so a burst of histogram requests cannot saturate p.requests and
	// starve handleQuery callers. Acquire jobs AFTER body parsing so
	// malformed requests don't waste a job slot.
	log.Printf("INFO: QUERY ALLOCATE_JOB (type:histogram) (runningJobs:%d) (remote:%s)", len(p.jobs), r.RemoteAddr)
	jtime := time.Now()
	p.jobs <- true
	defer func() {
		<-p.jobs
	}()
	log.Printf("INFO: QUERY DO_JOB (type:histogram) (runningJobs:%d) (remote:%s) (waitTime:%s)", len(p.jobs), r.RemoteAddr, time.Since(jtime).String())

	// Resolve metric-name and cluster-name indices under the cache
	// RLock, then release immediately. Holding RLock across the
	// whole db.Query iteration below would make a concurrent
	// cacheMetadataList WLock wait for the iterator to drain on
	// slow scans, serializing histogram responses against every
	// cache refresh. Copying the two slices we need (the metric
	// target's Entries and ClusterName's Entries) is O(entries)
	// but each is at most a few hundred strings and we only do it
	// once per request.
	p.cache.lock.RLock()
	targetMeta := p.cache.metadata[req.Metric.Target]
	clusterMeta := p.cache.metadata[ClusterNameLabel]
	p.cache.lock.RUnlock()
	if targetMeta == nil {
		log.Printf("WARN: Query target %s does not exist (remote:%s)", req.Metric.Target, r.RemoteAddr)
		w.WriteHeader(http.StatusOK)
		//nolint:errcheck
		w.Write([]byte("[]"))
		return
	}
	idxval := slices.Index(targetMeta.Entries, req.Metric.Name)
	if idxval == -1 {
		responseError(w, http.StatusBadRequest, "Metric %s does not exist in target %s (remote:%s)", req.Metric.Name, req.Metric.Target, r.RemoteAddr)
		return
	}
	if clusterMeta == nil {
		responseError(w, http.StatusInternalServerError, "ClusterName metadata missing (remote:%s)", r.RemoteAddr)
		return
	}
	clusterIdx := slices.Index(clusterMeta.Entries, req.Cluster)
	if clusterIdx == -1 {
		responseError(w, http.StatusBadRequest, "Cluster %s does not exist (remote:%s)", req.Cluster, r.RemoteAddr)
		return
	}

	// Buckets "00".."24" map to powers-of-two latency boundaries;
	// "tail" is the >=8388608 bucket. Every column we want to read
	// MUST be in the projection, including "tail" — otherwise the
	// db won't return it. The order doesn't matter to the iterator,
	// but the switch below MUST handle every column the projection
	// asks for, and the default case MUST skip unknown columns
	// rather than aliasing them onto whatever bucket happened to be
	// last in map iteration order.
	binList := []string{"00", "01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "21", "22", "23", "24", "tail"}
	filterExpr := db.And(
		db.Eq(req.Metric.Target, db.Int(int64(idxval))),
		db.Eq(ClusterNameLabel, db.Int(int64(clusterIdx))),
	)
	it := p.db.Query(req.Metric.Set).
		Between(p.config.TimestampBinName,
			db.Int(req.Range.From.UnixMilli()),
			db.Int(req.Range.To.UnixMilli())).
		Where(filterExpr).
		Project(binList...).
		Run(r.Context())
	defer it.Close()

	response := make(map[int64]int64)

	// TODO: group by node identifier
	for it.Next() {
		_, row := it.Record()
		// `key` is declared per-column on purpose. The original
		// Aerospike-era code hoisted `key` outside this loop, and
		// when the switch fell through (unknown bucket name) it
		// silently re-used the previous iteration's `key`,
		// double-counting one bucket and dropping another. Because
		// Go map iteration is randomized, the corruption was
		// non-deterministic and never surfaced consistently in
		// tests. This rewrite scopes `key` per-iteration AND adds
		// the explicit `default: continue` below, which together
		// fix the bug. Do not move `var key int64` outside the
		// inner loop.
		for k, v := range row {
			var key int64
			switch k {
			case "00":
				key = 0
			case "01":
				key = 1
			case "02":
				key = 2
			case "03":
				key = 4
			case "04":
				key = 8
			case "05":
				key = 16
			case "06":
				key = 32
			case "07":
				key = 64
			case "08":
				key = 128
			case "09":
				key = 256
			case "10":
				key = 512
			case "11":
				key = 1024
			case "12":
				key = 2048
			case "13":
				key = 4096
			case "14":
				key = 8192
			case "15":
				key = 16384
			case "16":
				key = 32768
			case "17":
				key = 65536
			case "18":
				key = 131072
			case "19":
				key = 262144
			case "20":
				key = 524288
			case "21":
				key = 1048576
			case "22":
				key = 2097152
			case "23":
				key = 4194304
			case "24":
				key = 8388608
			case "tail":
				key = 16777216
			default:
				// Unknown column (shouldn't happen given the
				// projection above, but a pattern emitting an
				// extra bin would otherwise corrupt the
				// histogram). Drop it rather than aliasing.
				continue
			}

			val, err := toInt64(v)
			if err != nil {
				responseError(w, http.StatusInternalServerError, "Invalid value for key %s: %s", k, err)
				return
			}
			response[key] += val
		}
	}
	if err := it.Err(); err != nil {
		responseError(w, http.StatusInternalServerError, "query iterator: %s", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	//nolint:errcheck
	json.NewEncoder(w).Encode(response)
}

// toInt64 coerces a db.Value to an int64. TypeInt64 passes through,
// TypeFloat64 is truncated, TypeString is parsed, and any other type is
// an error.
func toInt64(v db.Value) (int64, error) {
	switch v.Type() {
	case db.TypeInt64:
		iv, _ := v.AsInt()
		return iv, nil
	case db.TypeFloat64:
		fv, _ := v.AsFloat()
		return int64(fv), nil
	case db.TypeString:
		sv, _ := v.AsString()
		return strconv.ParseInt(sv, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported type: %s", v.Type())
	}
}
