package ingest

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerospike-client-go/v6"
	"github.com/bestmethod/logger"
)

func (i *Ingest) ProcessLogs() error {
	i.progress.Lock()
	i.progress.LogProcessor.Finished = false
	i.progress.LogProcessor.running = true
	i.progress.LogProcessor.wasRunning = true
	i.progress.LogProcessor.StartTime = time.Now()
	i.progress.Unlock()
	defer func() {
		i.progress.Lock()
		i.progress.LogProcessor.running = false
		i.progress.Unlock()
	}()
	// find node prefix->nodeID from log files
	logger.Debug("Process Logs: enumerating log files")
	foundLogs := make(map[string]*logFile) //cluster,nodeid,prefix
	err := filepath.Walk(i.config.Directories.Logs, func(filePath string, info os.FileInfo, err error) error {
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
		foundLogs[filePath] = &logFile{
			ClusterName: fcluster,
			NodePrefix:  fn[0],
			NodeID:      fn[1],
			NodeSuffix:  fn[2],
			Size:        info.Size(),
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("listing collectinfos: %s", err)
	}
	// merge list
	logger.Debug("ProcessCollectInfo: merging lists")
	i.progress.Lock()
	for n, f := range i.progress.LogProcessor.Files {
		foundLogs[n] = f
	}
	i.progress.LogProcessor.Files = make(map[string]*logFile)
	for n, f := range foundLogs {
		i.progress.LogProcessor.Files[n] = f
	}
	i.progress.LogProcessor.changed = true
	i.progress.Unlock()

	// process
	resultsChan := make(chan *processResult)
	go i.processLogsFeed(foundLogs, resultsChan)

	// feed results to backend DB
	wg := new(sync.WaitGroup)
	threads := make(chan bool, i.config.Aerospike.MaxPutThreads)
	for data := range resultsChan {
		if data.Error != nil {
			logger.Error("Log Processor: error encountered processing %s: %s", data.FileName, data.Error)
		}
		if data.Data != nil && data.SetName != "" && data.LogLine != "" {
			wg.Add(1)
			threads <- true
			go func(data map[string]interface{}, fn string, logLine string, setName string) {
				key, err := aerospike.NewKey(i.config.Aerospike.Namespace, setName, fn+"::"+logLine)
				if err != nil {
					logger.Error("Log Processor: could not create key for %s: %s", fn, err)
					wg.Done()
					<-threads
					return
				}
				err = i.db.Put(i.wp, key, data)
				if err != nil {
					logger.Error("Log Processor: could not insert data for %s: %s", fn, err)
				}
				wg.Done()
				<-threads
			}(data.Data, data.FileName, data.LogLine, data.SetName)
		}
	}
	wg.Wait()

	// done
	i.progress.Lock()
	i.progress.LogProcessor.Finished = true
	i.progress.Unlock()
	return nil
}

func (i *Ingest) processLogsFeed(foundLogs map[string]*logFile, resultsChan chan *processResult) {
	wg := new(sync.WaitGroup)
	threads := make(chan bool, i.config.Processor.MaxConcurrentLogFiles)
	for n, f := range foundLogs {
		if f.Finished {
			continue
		}
		threads <- true
		wg.Add(1)
		go func(n string, f *logFile) {
			defer func() {
				<-threads
				defer wg.Done()
			}()
			labels := map[string]interface{}{
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
			i.processLogFile(n, fd, resultsChan, labels)
		}(n, f)
	}
	wg.Wait()
	close(resultsChan)
}

type processResult struct {
	FileName string
	Data     map[string]interface{}
	Error    error
	SetName  string
	LogLine  string
}

func (i *Ingest) processLogFile(fileName string, r *os.File, resultsChan chan *processResult, labels map[string]interface{}) {
	_, fn := path.Split(fileName)
	results := make(map[string]interface{})
	for k, v := range labels {
		results[k] = v
	}
	var unmatched *os.File
	var err error
	s := bufio.NewScanner(r)
	buffer := make([]byte, i.config.Processor.LogReadBufferSizeKb*1024)
	s.Buffer(buffer, i.config.Processor.LogReadBufferSizeKb*1024)
	loc := int64(0)
	timer := time.Now()
	stepper := i.config.ProgressPrint.UpdateInterval / 2
	stream := newLogStream()
	for s.Scan() {
		if err = s.Err(); err != nil {
			resultsChan <- &processResult{
				Error: fmt.Errorf("could not read input file: %s", err),
			}
			return
		}
		line := s.Text()
		out, err := stream.Process(line)
		if err != nil {
			logger.Error("Stream Processor for line: %s", err)
			continue
		}
		if len(out) == 0 {
			if unmatched == nil {
				os.MkdirAll(path.Join(i.config.Directories.NoStatLogs, labels["ClusterName"].(string)), 0755)
				unmatched, err = os.Create(path.Join(i.config.Directories.NoStatLogs, labels["ClusterName"].(string), fn))
				if err != nil {
					logger.Error("Could not create file for non-stat: %s", err)
				} else {
					defer unmatched.Close()
				}
			}
			if unmatched != nil {
				_, err = unmatched.WriteString(line + "\n")
				if err != nil {
					logger.Error("Could not write no-stat: %s", err)
				}
			}
			continue
		}
		for _, d := range out {
			resultsChan <- &processResult{
				FileName: fileName,
				Data:     d.Data,
				Error:    d.Error,
				SetName:  d.SetName,
				LogLine:  d.Line,
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
	out := stream.Close()
	for _, d := range out {
		resultsChan <- &processResult{
			FileName: fileName,
			Data:     d.Data,
			Error:    d.Error,
			SetName:  d.SetName,
			LogLine:  d.Line,
		}
	}
	i.progress.Lock()
	i.progress.LogProcessor.Files[fileName].Processed = i.progress.LogProcessor.Files[fileName].Size
	i.progress.LogProcessor.Files[fileName].Finished = true
	i.progress.LogProcessor.changed = true
	i.progress.Unlock()
}
