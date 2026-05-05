package livelisten

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/pkg/agi/ingest"
)

func (l *Listener) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if l.results == nil {
		http.Error(w, "live listener not initialised", http.StatusServiceUnavailable)
		return
	}
	if !l.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if int(l.active.Load()) >= l.cfg.MaxStreams {
		http.Error(w, "too many streams", http.StatusTooManyRequests)
		return
	}
	cluster := r.URL.Query().Get("cluster")
	node := r.URL.Query().Get("node")
	source := r.URL.Query().Get("source")
	sourceID := r.URL.Query().Get("source-id")
	if cluster == "" || node == "" || sourceID == "" {
		http.Error(w, "missing query: cluster, node, source-id (source optional)", http.StatusBadRequest)
		return
	}
	if source == "" {
		source = "live.log"
	}

	l.active.Add(1)
	defer l.active.Add(-1)

	flusher, _ := w.(http.Flusher)
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}

	prefix := l.ing.AllocNodePrefixForLive(cluster, node)
	labels := map[string]any{
		"ClusterName": cluster,
		"NodeIdent":   fmt.Sprintf("%d_%s", prefix, node),
	}
	uniq := cluster + "::/::" + strconv.Itoa(prefix) + "_" + node
	synthetic := "live/" + sourceID + "/" + source

	stream := l.ing.NewLiveLogStream(cluster)
	cache := make(ingest.ResolveCache)
	resolved := l.ing.ResolveLabelsForLive(labels, cache, l.shards, cluster, synthetic)

	resume := int64(0)
	if v := r.Header.Get("X-Resume-Offset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			resume = n
		}
	}
	if l.offsets != nil {
		if persisted := l.offsets.get(sourceID); persisted > resume {
			resume = persisted
		}
	}

	var byteOffset int64
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.Copy(pw, r.Body)
		_ = pw.Close()
	}()
	sc := bufio.NewScanner(pr)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, ingestMaxScan())

	for sc.Scan() {
		line := sc.Text()
		nl := int64(len(line) + 1)
		if byteOffset+nl <= resume {
			byteOffset += nl
			continue
		}
		byteOffset += nl

		outs, err := stream.Process(line, prefix)
		if err != nil {
			msg := err.Error()
			if !strings.Contains(msg, "LINE NOT MATCHED") && !strings.Contains(msg, "TIME PARSE:") {
				log.Printf("livelisten: parse: %v", err)
			}
		}
		for _, o := range outs {
			l.results <- &ingest.ProcessResult{
				FileName:       synthetic,
				Data:           o.Data,
				Resolved:       l.ing.MergeResolvedForLive(resolved, o.Metadata, cache, l.shards, cluster, synthetic),
				Error:          o.Error,
				SetName:        o.SetName,
				LogLine:        o.Line,
				UniqNodeString: uniq,
			}
		}
		if l.offsets != nil {
			l.offsets.set(sourceID, byteOffset)
		}
	}
	if err := sc.Err(); err != nil {
		log.Printf("livelisten: scanner: %v", err)
	}
	outs, _, _ := stream.Close()
	for _, o := range outs {
		l.results <- &ingest.ProcessResult{
			FileName:       synthetic,
			Data:           o.Data,
			Resolved:       l.ing.MergeResolvedForLive(resolved, o.Metadata, cache, l.shards, cluster, synthetic),
			Error:          o.Error,
			SetName:        o.SetName,
			LogLine:        o.Line,
			UniqNodeString: uniq,
		}
	}
	if l.offsets != nil {
		l.offsets.set(sourceID, byteOffset)
	}
	w.Header().Set("X-Last-Offset", strconv.FormatInt(byteOffset, 10))
	if flusher != nil {
		flusher.Flush()
	}
}

func ingestMaxScan() int {
	// align with ingest default log buffer (~1 MiB line cap)
	return 1024 * 1024
}

func (l *Listener) authorize(r *http.Request) bool {
	if l.cfg.TokensPath == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	tok := strings.TrimPrefix(auth, "Bearer ")
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return false
	}
	tokens, err := readTokenDir(l.cfg.TokensPath)
	if err != nil {
		log.Printf("livelisten: tokens: %v", err)
		return false
	}
	for _, t := range tokens {
		if t == tok {
			return true
		}
	}
	return false
}

func readTokenDir(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		t := strings.TrimSpace(string(b))
		if t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}
