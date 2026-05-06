package livelisten

import (
	"bufio"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const streamPath = "/agi/ingest/stream"

func (l *Listener) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != streamPath {
		http.NotFound(w, r)
		return
	}
	if err := l.ensureStarted(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !l.checkBearer(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cluster := strings.TrimSpace(r.URL.Query().Get("cluster"))
	node := strings.TrimSpace(r.URL.Query().Get("node"))
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	if cluster == "" || node == "" {
		http.Error(w, "cluster and node query parameters are required", http.StatusBadRequest)
		return
	}
	if source == "" {
		source = "live"
	}
	sourceID := strings.TrimSpace(r.URL.Query().Get("source-id"))
	if sourceID == "" {
		sourceID = stableSourceID(cluster, node, source)
	}
	if !l.registerStream(sourceID) {
		http.Error(w, "too many active live streams", http.StatusTooManyRequests)
		return
	}
	defer l.unregisterStream(sourceID)

	offset := l.offsets.Get(sourceID)
	if headerOffset := strings.TrimSpace(r.Header.Get("X-Resume-Offset")); headerOffset != "" {
		if parsed, err := strconv.ParseInt(headerOffset, 10, 64); err == nil && parsed > offset {
			offset = parsed
		}
	}

	w.Header().Set("Trailer", "X-Last-Offset")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	stream := l.workers.NewLiveStream(cluster, node, source)
	defer stream.Close()

	scanner := bufio.NewScanner(r.Body)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)
	lastSave := time.Now()
	for scanner.Scan() {
		select {
		case <-r.Context().Done():
			w.Header().Set("X-Last-Offset", strconv.FormatInt(offset, 10))
			return
		default:
		}
		line := scanner.Text()
		if err := stream.ProcessLine(line); err != nil {
			log.Printf("ERROR: live ingest stream %s: %s", sourceID, err)
		}
		offset += int64(len(line) + 1)
		l.offsets.Set(sourceID, offset)
		if time.Since(lastSave) >= time.Second {
			if err := l.offsets.Save(); err != nil {
				log.Printf("WARN: live ingest offsets save: %s", err)
			}
			lastSave = time.Now()
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("ERROR: live ingest scanner %s: %s", sourceID, err)
	}
	l.offsets.Set(sourceID, offset)
	if err := l.offsets.Save(); err != nil {
		log.Printf("WARN: live ingest final offsets save: %s", err)
	}
	w.Header().Set("X-Last-Offset", strconv.FormatInt(offset, 10))
}

func (l *Listener) registerStream(sourceID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.activeStreams[sourceID]; !exists && len(l.activeStreams) >= l.cfg.MaxStreams {
		return false
	}
	l.activeStreams[sourceID] = struct{}{}
	return true
}

func (l *Listener) unregisterStream(sourceID string) {
	l.mu.Lock()
	delete(l.activeStreams, sourceID)
	l.mu.Unlock()
}

func (l *Listener) checkBearer(r *http.Request) bool {
	if l.cfg.TokensPath == "" {
		return true
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" {
		return false
	}
	ok := false
	err := filepath.WalkDir(l.cfg.TokensPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || ok {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		stored := strings.TrimSpace(string(b))
		if subtle.ConstantTimeCompare([]byte(token), []byte(stored)) == 1 {
			ok = true
		}
		return nil
	})
	if err != nil {
		log.Printf("WARN: live ingest token walk failed: %s", err)
	}
	return ok
}

func stableSourceID(cluster, node, source string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s\x00%s\x00%s", cluster, node, source)))
	return hex.EncodeToString(sum[:])
}
