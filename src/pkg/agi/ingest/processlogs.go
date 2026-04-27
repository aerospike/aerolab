package ingest

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"log"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

type MetaEntries map[string]*metaEntries

type metaEntries struct {
	Entries          []string
	ByCluster        map[string][]int
	StaticEntriesIdx []int
}

func (i *Ingest) ProcessLogsPrep() (foundLogs map[string]*LogFile, meta map[string]*metaEntries, err error) {
	i.progress.Lock()
	i.progress.LogProcessor.Finished = false
	i.progress.LogProcessor.running = true
	i.progress.LogProcessor.wasRunning = true
	i.progress.LogProcessor.StartTime = time.Now()
	i.progress.Unlock()
	// find node prefix->nodeID from log files
	log.Printf("DEBUG: Process Logs: enumerating log files")
	foundLogs = make(map[string]*LogFile) //cluster,nodeid,prefix
	err = filepath.Walk(i.config.Directories.Logs, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		fn := strings.Split(info.Name(), "_")
		if len(fn) != 3 {
			return nil
		}
		fdir, _ := path.Split(filePath)
		_, fcluster := path.Split(strings.TrimSuffix(fdir, "/"))
		foundLogs[filePath] = &LogFile{
			ClusterName: fcluster,
			NodePrefix:  fn[0],
			NodeID:      fn[1],
			NodeSuffix:  fn[2],
			Size:        info.Size(),
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("listing logs: %s", err)
	}
	// merge list
	log.Printf("DEBUG: ProcessLogs: merging lists")
	i.progress.Lock()
	maps.Copy(foundLogs, i.progress.LogProcessor.Files)
	i.progress.LogProcessor.Files = make(map[string]*LogFile)
	maps.Copy(i.progress.LogProcessor.Files, foundLogs)
	i.progress.LogProcessor.changed = true
	meta = make(map[string]*metaEntries)
	// The labels set uses <row-key = label-name, column-name = "json">
	// (see init.go's labelsValueCol). Keys like "BINLIST" / "cfName" /
	// "sources" / "timerange" are not metaEntries; skip them. Every
	// other row is a meta entry for the label named by its primary
	// key.
	skipKeys := map[string]struct{}{
		"BINLIST":   {},
		"cfName":    {},
		"sources":   {},
		"timerange": {},
	}
	it := i.db.Scan(i.patterns.LabelsSetName, labelsValueCol)
	for it.Next() {
		key, row := it.Record()
		if _, skip := skipKeys[key]; skip {
			continue
		}
		v, ok := row[labelsValueCol]
		if !ok {
			continue
		}
		s, ok := v.AsString()
		if !ok {
			continue
		}
		metaItem := &metaEntries{}
		if uerr := json.Unmarshal([]byte(s), metaItem); uerr != nil {
			log.Printf("WARN: Failed to unmarshal existing label data for %s: %s", key, uerr)
			continue
		}
		meta[key] = metaItem
	}
	if serr := it.Err(); serr != nil {
		log.Printf("WARN: Could not read existing labels: %s", serr)
	}
	_ = it.Close()
	i.progress.Unlock()
	err = i.saveProgress()
	return foundLogs, meta, err
}

func (i *Ingest) ProcessLogs(foundLogs map[string]*LogFile, meta map[string]*metaEntries) error {
	if foundLogs == nil || meta == nil {
		var err error
		foundLogs, meta, err = i.ProcessLogsPrep()
		if err != nil {
			i.progress.Lock()
			i.progress.LogProcessor.running = false
			i.progress.Unlock()
			return err
		}
	}
	defer func() {
		i.progress.Lock()
		i.progress.LogProcessor.running = false
		i.progress.Unlock()
	}()
	metaLock := new(sync.Mutex)
	// process
	resultsChan := make(chan *processResult)
	go i.processLogsFeed(foundLogs, resultsChan)

	// feed results to backend DB
	wg := new(sync.WaitGroup)
	threads := make(chan bool, i.config.MaxPutThreads)
	for data := range resultsChan {
		if data.Error != nil {
			log.Printf("ERROR: Log Processor: error encountered processing %s: %s", data.FileName, data.Error)
		}
		if data.Data != nil && data.SetName != "" && data.LogLine != "" {
			wg.Add(1)
			threads <- true
			go func(metadata map[string]any, data map[string]any, fn string, logLine string, setName string, nodeIdentifier string) {
				defer func() {
					wg.Done()
					<-threads
				}()
				metaLock.Lock()
				// Walk every meta key. Previously a marshal or db.Put
				// failure for one key returned immediately, dropping
				// every remaining label (including ClusterName) on
				// the floor and writing the metric row with a
				// partial label-set. Keep going on per-key errors
				// and only skip translation (data[k]=idx) for keys
				// we couldn't persist.
				for k, v := range metadata {
					sv, ok := v.(string)
					if !ok {
						log.Printf("ERROR: Log Processor: metadata %q has non-string value %T; skipping", k, v)
						continue
					}
					if _, ok := meta[k]; !ok {
						meta[k] = &metaEntries{
							ByCluster: make(map[string][]int),
						}
					}
					clusterName, _ := metadata["ClusterName"].(string)
					// Track exactly which mutation we made so we can
					// roll it back if the labels-set Put fails.
					// Without rollback the in-memory meta drifts
					// ahead of the on-disk snapshot: subsequent
					// metric rows in this run encode an idx that
					// the plugin can't resolve (entries[idx]
					// missing), and after a restart the plugin
					// surfaces "metadata corrupt or log ingestion
					// in progress" errors for those rows.
					const (
						mutNone        = 0
						mutAppendEntry = 1 // appended to Entries (and possibly ByCluster)
						mutAppendCl    = 2 // only appended to ByCluster[cluster]
					)
					mutKind := mutNone
					var mutCluster string
					saveMeta := false
					idx := slices.Index(meta[k].Entries, sv)
					if idx == -1 {
						idx = len(meta[k].Entries)
						saveMeta = true
						meta[k].Entries = append(meta[k].Entries, sv)
						if meta[k].ByCluster == nil {
							meta[k].ByCluster = make(map[string][]int)
						}
						if clusterName != "" {
							meta[k].ByCluster[clusterName] = append(meta[k].ByCluster[clusterName], len(meta[k].Entries)-1)
							mutCluster = clusterName
						}
						mutKind = mutAppendEntry
					} else if clusterName != "" && !slices.Contains(meta[k].ByCluster[clusterName], idx) {
						meta[k].ByCluster[clusterName] = append(meta[k].ByCluster[clusterName], idx)
						saveMeta = true
						mutKind = mutAppendCl
						mutCluster = clusterName
					}
					rollback := func() {
						switch mutKind {
						case mutAppendEntry:
							if n := len(meta[k].Entries); n > 0 {
								meta[k].Entries = meta[k].Entries[:n-1]
							}
							if mutCluster != "" {
								if cl := meta[k].ByCluster[mutCluster]; len(cl) > 0 {
									meta[k].ByCluster[mutCluster] = cl[:len(cl)-1]
									if len(meta[k].ByCluster[mutCluster]) == 0 {
										delete(meta[k].ByCluster, mutCluster)
									}
								}
							}
						case mutAppendCl:
							if cl := meta[k].ByCluster[mutCluster]; len(cl) > 0 {
								meta[k].ByCluster[mutCluster] = cl[:len(cl)-1]
								if len(meta[k].ByCluster[mutCluster]) == 0 {
									delete(meta[k].ByCluster, mutCluster)
								}
							}
						}
					}
					if saveMeta {
						metajson, merr := json.Marshal(meta[k])
						if merr != nil {
							log.Printf("ERROR: Log Processor: could not jsonify metadata for %s key %s: %s; rolling back in-memory meta", fn, k, merr)
							rollback()
							// Skip this key: the metric row will
							// not carry a translated idx for it,
							// but no other label is corrupted.
							continue
						}
						i.binList.lock.Lock()
						if !slices.Contains(i.binList.BinNames, k) {
							i.binList.BinNames = append(i.binList.BinNames, k)
							i.binList.changed = true
						}
						i.binList.lock.Unlock()
						if perr := i.db.Put(i.patterns.LabelsSetName, k, db.Row{labelsValueCol: db.Str(string(metajson))}); perr != nil {
							log.Printf("ERROR: Log Processor: could not store metadata for %s key %s: %s; rolling back in-memory meta", fn, k, perr)
							rollback()
							continue
						}
					}
					data[k] = idx
				}
				metaLock.Unlock()
				i.binList.lock.Lock()
				for k := range data {
					if !slices.Contains(i.binList.BinNames, k) {
						i.binList.BinNames = append(i.binList.BinNames, k)
						i.binList.changed = true
					}
				}
				i.binList.lock.Unlock()
				row, rerr := buildDataRow(data, i.config.TimestampColumnName)
				if rerr != nil {
					log.Printf("ERROR: Log Processor: %s: %s", fn, rerr)
					return
				}
				if perr := i.putData(setName, nodeIdentifier+"::/::"+logLine, row); perr != nil {
					log.Printf("ERROR: Log Processor: could not insert data for %s: %s", fn, perr)
				}
				serr := i.storeBinList()
				if serr != nil {
					log.Printf("ERROR: Log Processor: could not store bin list: %s", serr)
				}
			}(data.Metadata, data.Data, data.FileName, data.LogLine, data.SetName, data.UniqNodeString)
		}
	}
	wg.Wait()

	// Final flush of the bin list, in case the last Put inside the
	// worker-goroutine loop saw changed=false but a later goroutine
	// re-set it. If this fails the next ingest cycle's storeBinList
	// will retry, so we log and continue rather than aborting the
	// whole pipeline.
	if serr := i.storeBinList(); serr != nil {
		log.Printf("ERROR: Log Processor: could not store bin list: %s", serr)
	}

	// done
	i.progress.Lock()
	i.progress.LogProcessor.Finished = true
	i.progress.Unlock()
	return nil
}

func (i *Ingest) storeBinList() error {
	i.binList.lock.Lock()
	defer i.binList.lock.Unlock()
	if i.binList.changed {
		binListJson, err := json.Marshal(i.binList.BinNames)
		if err != nil {
			return err
		}
		if err := i.db.Put(i.patterns.LabelsSetName, "BINLIST", db.Row{labelsValueCol: db.Str(string(binListJson))}); err != nil {
			return err
		}
		i.binList.changed = false
	}
	return nil
}

// buildDataRow translates the dynamic map[string]any shape produced by the
// log stream into a typed db.Row. The timestamp column is forced to int64
// because that set's indexed column requires it. Every other Go scalar
// type is mapped to its native db.Value so that floats keep their
// precision and bools/bytes round-trip without going through fmt.Sprint.
//
// Anything truly unrecognised (struct, slice, map) falls through to
// db.Str(fmt.Sprint(v)) — that's a last-resort coercion, not a
// normal path.
//
// Returns an error when the timestamp column is missing or carries a
// type we cannot sensibly cast to int64. Silently writing a row with
// timestamp=0 (the old behaviour) created a phantom point at the unix
// epoch that Grafana rendered on every timeseries panel; the log
// pipeline already treats errNoTimestamp rows as non-stat, so this
// only ever triggers on malformed pattern output and the line should
// be dropped rather than injected.
func buildDataRow(data map[string]any, timestampCol string) (db.Row, error) {
	row := make(db.Row, len(data))
	tsSeen := false
	for k, v := range data {
		if k == timestampCol {
			tsSeen = true
			switch vt := v.(type) {
			case int64:
				row[k] = db.Int(vt)
			case int:
				row[k] = db.Int(int64(vt))
			case int32:
				row[k] = db.Int(int64(vt))
			case uint64:
				row[k] = db.Int(int64(vt))
			case uint32:
				row[k] = db.Int(int64(vt))
			default:
				return nil, fmt.Errorf("timestamp column %q has non-integer type %T (value=%v); dropping row", timestampCol, v, v)
			}
			continue
		}
		switch vt := v.(type) {
		case int:
			row[k] = db.Int(int64(vt))
		case int64:
			row[k] = db.Int(vt)
		case int32:
			row[k] = db.Int(int64(vt))
		case uint64:
			row[k] = db.Int(int64(vt))
		case uint32:
			row[k] = db.Int(int64(vt))
		case float64:
			row[k] = db.Float(vt)
		case float32:
			row[k] = db.Float(float64(vt))
		case bool:
			row[k] = db.BoolV(vt)
		case []byte:
			row[k] = db.BytesV(vt)
		case string:
			row[k] = db.Str(vt)
		default:
			row[k] = db.Str(fmt.Sprint(v))
		}
	}
	if !tsSeen {
		return nil, fmt.Errorf("timestamp column %q missing from row", timestampCol)
	}
	return row, nil
}

// putData writes a data row, retrying on a type-conflict error after
// coercing every offending column to the type the schema already has
// recorded. The embedded db enforces per-column types; this helper
// reconciles the case where a pattern legitimately emits the same
// column with two compatible representations (e.g. int once and a
// string-of-int later) without failing the whole row.
//
// Coercion rules (incoming → existing column):
//
//   - any → existing string : format via strconv (int / float / bool)
//   - string → existing int : strconv.ParseInt; drop column on failure
//   - float → existing int  : refuse (float→int is lossy and signals a
//     pattern bug); drop column instead of silently truncating
//   - any → existing float  : strconv.ParseFloat for strings, widen for
//     ints; drop column otherwise
//
// The timestamp column is never coerced; its schema type is pinned to
// int64 at RegisterSet time. Error classification uses the
// db.ErrColumnTypeConflict sentinel via errors.Is so we don't rely on
// the exact wording of the db layer's error messages.
//
// Dropped columns mean the row goes through with one less field,
// which is preferable to losing every other bin in the row.
//
// Retry policy: on a conflict we re-read SchemaOf and coerce. Two
// goroutines writing the same previously-unseen column concurrently
// can race — worker B's Put can miss A's column-addition and trip
// its own ErrColumnTypeConflict on the retry. We bound the retry
// count (putDataMaxRetries) so the helper always terminates; each
// iteration re-fetches the schema so we eventually observe the
// stable resolved type.
//
// 5 retries (was 3) absorbs first-batch storms where MaxPutThreads
// goroutines all race to introduce the same brand-new columns: each
// retry re-reads SchemaOf, so once any one writer commits the type
// the rest converge in a single pass. Beyond ~5 the pathology is no
// longer "concurrent first-write" but a real bug, and looping
// forever would hide it.
const putDataMaxRetries = 5

// putData hands the row to the per-set putBatcher (AssumeNew=true) on
// the ingest hot path. The batcher accumulates rows and flushes them
// via db.PutBatch in PutBatchSize chunks; on a column-type conflict
// the batcher falls back to putDataSingle on each affected row.
//
// Returns nil eagerly: the batcher commits asynchronously and surfaces
// its own errors via log.Printf. The pre-batching contract (return nil
// on success, error on type-conflict-after-retries) is preserved for
// callers that expect the function to return — but with batching the
// error visibility moved into the flusher's log output. ProcessLogs's
// callers only used the return value to log; they did not gate
// downstream work on it.
func (i *Ingest) putData(set, key string, row db.Row) error {
	if i.putBatcher == nil {
		// Tests construct Ingest without finalizeInit and call
		// putData directly; fall back to the synchronous path.
		return i.putDataSingle(set, key, row)
	}
	i.putBatcher.submit(set, key, row)
	return nil
}

// putDataSingle is the legacy synchronous path: one row, with retry on
// db.ErrColumnTypeConflict. The batcher uses it as the per-row
// fallback when a PutBatch fails atomically due to a type conflict.
// Tests that construct an Ingest without finalizeInit also reach this
// path via putData.
func (i *Ingest) putDataSingle(set, key string, row db.Row) error {
	current := row
	var lastErr error
	for attempt := 0; attempt < putDataMaxRetries; attempt++ {
		err := i.db.Put(set, key, current)
		if err == nil {
			return nil
		}
		if !errors.Is(err, db.ErrColumnTypeConflict) {
			return err
		}
		lastErr = err
		schema, ok := i.db.SchemaOf(set)
		if !ok {
			// Set may have been dropped mid-flight, or the db was
			// closed. Either way we can't coerce without schema
			// knowledge; surface the original error.
			return err
		}
		current = i.coerceRow(current, schema)
	}
	log.Printf("WARN: putData: %q key %q: type conflict persisted after %d retries: %s", set, key, putDataMaxRetries, lastErr)
	return lastErr
}

// coerceRow applies the per-column coercion rules documented on
// putData. It is a pure function of (row, schema) so repeated calls
// on a row whose schema has stabilised are idempotent.
func (i *Ingest) coerceRow(row db.Row, schema []db.ColumnSpec) db.Row {
	types := make(map[string]db.ColumnType, len(schema))
	for _, col := range schema {
		types[col.Name] = col.Type
	}
	ts := i.config.TimestampColumnName
	coerced := make(db.Row, len(row))
	for k, v := range row {
		if k == ts {
			coerced[k] = v
			continue
		}
		want, known := types[k]
		if !known {
			coerced[k] = v
			continue
		}
		if v.Type() == want {
			coerced[k] = v
			continue
		}
		switch want {
		case db.TypeString:
			if iv, ok := v.AsInt(); ok {
				coerced[k] = db.Str(strconv.FormatInt(iv, 10))
				continue
			}
			if fv, ok := v.AsFloat(); ok {
				coerced[k] = db.Str(strconv.FormatFloat(fv, 'f', -1, 64))
				continue
			}
			if bv, ok := v.AsBool(); ok {
				coerced[k] = db.Str(strconv.FormatBool(bv))
				continue
			}
			if sv, ok := v.AsString(); ok {
				coerced[k] = db.Str(sv)
				continue
			}
		case db.TypeInt64:
			// Refuse float→int: silently truncating real-valued
			// metrics into an int column is the kind of corruption
			// that takes weeks to notice. Drop instead.
			if v.Type() == db.TypeFloat64 {
				log.Printf("WARN: putData: column %q is int but pattern emitted float; dropping", k)
				continue
			}
			if sv, ok := v.AsString(); ok {
				if iv, perr := strconv.ParseInt(sv, 10, 64); perr == nil {
					coerced[k] = db.Int(iv)
					continue
				}
			}
		case db.TypeFloat64:
			if iv, ok := v.AsInt(); ok {
				coerced[k] = db.Float(float64(iv))
				continue
			}
			if sv, ok := v.AsString(); ok {
				if fv, perr := strconv.ParseFloat(sv, 64); perr == nil {
					coerced[k] = db.Float(fv)
					continue
				}
			}
		}
	}
	return coerced
}

func (i *Ingest) processLogsFeed(foundLogs map[string]*LogFile, resultsChan chan *processResult) {
	// resultsChan MUST be closed exactly once, even if a per-file
	// goroutine or this dispatcher itself panics. Otherwise the
	// consumer in ProcessLogs (`for data := range resultsChan`)
	// blocks forever, the WaitGroup never returns, and Ingest.Close
	// is never reached. The outer recover catches a panic in the
	// dispatch loop or the wg.Wait path; the per-goroutine recover
	// catches a panic in processLogFile so the WaitGroup count still
	// drains. Both are logged so the failure remains visible.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR: Log Processor: processLogsFeed panic: %v", r)
		}
		close(resultsChan)
	}()
	wg := new(sync.WaitGroup)
	threads := make(chan bool, i.config.Processor.MaxConcurrentLogFiles)
	for n, f := range foundLogs {
		if f.Finished {
			continue
		}
		threads <- true
		wg.Add(1)
		go func(n string, f *LogFile) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("ERROR: Log Processor: processLogFile panic on %s: %v", n, r)
				}
				<-threads
				wg.Done()
			}()
			labels := map[string]any{
				"ClusterName": f.ClusterName,
				"NodeIdent":   f.NodePrefix + "_" + f.NodeID,
			}
			fd, err := os.Open(n)
			if err != nil {
				resultsChan <- &processResult{
					FileName: n,
					Error:    err,
				}
				return
			}
			defer fd.Close()
			if f.Processed > 0 && f.Processed < f.Size {
				move := f.Processed - int64(i.config.Processor.LogReadBufferSizeKb*1024*2)
				if move > 0 {
					//nolint:errcheck
					fd.Seek(move, 0)
				}
			}
			nprefix, _ := strconv.Atoi(f.NodePrefix)
			i.processLogFile(n, fd, resultsChan, labels, nprefix, f.ClusterName+"::/::"+f.NodePrefix+"_"+f.NodeID)
		}(n, f)
	}
	wg.Wait()
}

type processResult struct {
	FileName       string
	Data           map[string]any
	Metadata       map[string]any
	Error          error
	SetName        string
	LogLine        string
	UniqNodeString string
}

func (i *Ingest) processLogFile(fileName string, r *os.File, resultsChan chan *processResult, labels map[string]any, nodePrefix int, uniqNodeString string) {
	i.progress.Lock()
	i.progress.LogProcessor.Files[fileName].StartTime = time.Now().UTC().Format("2006-01-02 15:04:05") + " UTC"
	i.progress.LogProcessor.changed = true
	i.progress.Unlock()

	_, fn := path.Split(fileName)
	var unmatched *os.File
	var err error
	s := bufio.NewScanner(r)
	buffer := make([]byte, i.config.Processor.LogReadBufferSizeKb*1024)
	s.Buffer(buffer, i.config.Processor.LogReadBufferSizeKb*1024)
	loc := int64(0)
	timer := time.Now()
	stepper := i.config.ProgressPrint.UpdateInterval / 2
	logDir, fileNameOnly := path.Split(fileName)
	_, clusterName := path.Split(strings.Trim(logDir, "/"))
	stream := newLogStream(clusterName, i.patterns, &i.config.IngestTimeRanges, i.config.TimestampColumnName)
	for s.Scan() {
		if err = s.Err(); err != nil {
			resultsChan <- &processResult{
				Error: fmt.Errorf("could not read input file: %s", err),
			}
			return
		}
		line := s.Text()
		out, err := stream.Process(line, nodePrefix)
		if err != nil && err != errNotMatched && err != errNoTimestamp && !strings.HasPrefix(err.Error(), "TIME PARSE:") {
			log.Printf("ERROR: Stream Processor for line: %s", err)
			i.progress.LogProcessor.LineErrors.add(nodePrefix, err.Error())
			continue
		}
		if len(out) == 0 && err != nil && (err == errNotMatched || err == errNoTimestamp || strings.HasPrefix(err.Error(), "TIME PARSE:")) {
			if unmatched == nil {
				noStatDir := path.Join(i.config.Directories.NoStatLogs, labels["ClusterName"].(string))
				if mkErr := os.MkdirAll(noStatDir, 0755); mkErr != nil {
					log.Printf("ERROR: Could not create no-stat directory %s: %s", noStatDir, mkErr)
				}
				unmatched, err = os.Create(path.Join(noStatDir, fn))
				if err != nil {
					log.Printf("ERROR: Could not create file for non-stat: %s", err)
				} else {
					defer unmatched.Close()
				}
			}
			if unmatched != nil {
				_, err = unmatched.WriteString(line + "\n")
				if err != nil {
					log.Printf("ERROR: Could not write no-stat: %s", err)
				}
			}
			continue
		}
		for _, d := range out {
			results := make(map[string]any)
			for k, v := range d.Data {
				switch vt := v.(type) {
				case string:
					vint, err := strconv.Atoi(vt)
					if err != nil {
						results[k] = v
					} else {
						results[k] = vint
					}
				default:
					results[k] = v
				}
			}
			meta := d.Metadata
			maps.Copy(meta, labels)
			resultsChan <- &processResult{
				FileName:       fileName,
				Data:           results,
				Error:          d.Error,
				SetName:        d.SetName,
				LogLine:        d.Line,
				Metadata:       meta,
				UniqNodeString: uniqNodeString,
			}
		}
		// tracker of how many lines we processed already
		if time.Since(timer) > stepper {
			newloc, _ := r.Seek(0, 1)
			if newloc > 0 && newloc != loc {
				loc = newloc
				i.progress.Lock()
				i.progress.LogProcessor.Files[fileName].Processed = loc
				i.progress.LogProcessor.changed = true
				i.progress.Unlock()
			}
			timer = time.Now()
		}
	}
	out, startTime, endTime := stream.Close()
	for _, d := range out {
		meta := d.Metadata
		maps.Copy(meta, labels)
		resultsChan <- &processResult{
			FileName:       fileName,
			Data:           d.Data,
			Error:          d.Error,
			SetName:        d.SetName,
			LogLine:        d.Line,
			Metadata:       meta,
			UniqNodeString: uniqNodeString,
		}
	}
	// store startTime and endTime of logs
	for _, point := range []time.Time{startTime, endTime} {
		if point.IsZero() {
			continue
		}
		nodePrefix, err := strconv.Atoi(strings.Split(fileNameOnly, "_")[0])
		if err != nil {
			continue
		}
		meta := make(map[string]any)
		maps.Copy(meta, labels)
		meta["fileName"] = clusterName + "/" + fileNameOnly
		resultsChan <- &processResult{
			FileName: fileName,
			Data: map[string]any{
				"nodePrefix":                 nodePrefix,
				i.config.TimestampColumnName: point.UnixMilli(),
			},
			Error:          nil,
			SetName:        i.config.LogFileRangesSetName,
			LogLine:        fmt.Sprintf("%s:%d", fileName, point.UnixMilli()),
			Metadata:       meta,
			UniqNodeString: uniqNodeString,
		}
	}

	// done
	i.progress.Lock()
	i.progress.LogProcessor.Files[fileName].Processed = i.progress.LogProcessor.Files[fileName].Size
	i.progress.LogProcessor.Files[fileName].Finished = true
	i.progress.LogProcessor.Files[fileName].FinishTime = time.Now().UTC().Format("2006-01-02 15:04:05") + " UTC"
	i.progress.LogProcessor.changed = true
	i.progress.Unlock()
}
