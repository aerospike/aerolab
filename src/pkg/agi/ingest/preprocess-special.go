package ingest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/rglonek/logger"
	"github.com/rglonek/sbs"
)

// {"JsonPayload":{"Log": "full line..."}}
// {"Log": "full line..."}
// {"TextPayload": "full line...","resource":{"labels":{"pod_name":""}}}
// {"timestamp":"2021-06-18T10:00:00Z" "jsonPayload":{"level":"INFO","module":"info","module_detail":"ticker.c:497","message":"{test} memory-usage: total-bytes 0 index-bytes 0 sindex-bytes 0 data-bytes 0 used-pct 0.00"}}
// tsv: random nodename log...
// tsv: random nodename clustername log...

func (i *Ingest) preProcessSpecial(fn string, mimeType *mimetype.MIME) (fnlist []string, err error) {
	fh, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	// read 16kb
	b := make([]byte, 16384)
	_, err = fh.Read(b)
	if err != nil {
		return nil, err
	}
	fh.Seek(0, 0)
	// test for tab formats
	line := strings.Split(sbs.ByteSliceToString(b), "\n")[0]
	tabsplit := strings.Split(line, "\t")
	if len(tabsplit) == 4 {
		logger.Debug("PreProcess-special: %s is tab-4", fn)
		s := bufio.NewScanner(fh)
		tracker := make(map[string]*os.File)
		for s.Scan() {
			if err = s.Err(); err != nil {
				return nil, err
			}
			line := strings.Split(s.Text(), "\t")
			if len(line) < 4 {
				logger.Detail("PreProcess-Special: %s is tab-4 format but found line without 4 tabs: %s", fn, line)
				continue
			}
			if line[1] == "pod_name" && strings.TrimSuffix(line[3], "\n") == "text_payload" { // timestamp       pod_name        text_payload
				logger.Detail("PreProcess-Special: %s tab-4 header found, ignoring", fn)
				continue
			}
			ident := strings.Trim(line[2], "\r\n\t ") + "-" + strings.Trim(line[1], "\r\t\n ")
			logline := strings.Trim(line[3], "\r\t\n ") + "\n"
			if _, ok := tracker[fn+"_special-split_"+ident]; !ok {
				out, err := os.Create(fn + "_special-split_" + ident)
				if err != nil {
					return fnlist, err
				}
				defer out.Close()
				tracker[fn+"_special-split_"+ident] = out
				fnlist = append(fnlist, fn+"_special-split_"+ident)
			}
			_, err = tracker[fn+"_special-split_"+ident].WriteString(logline)
			if err != nil {
				return fnlist, err
			}
		}
		logger.Debug("PreProcess-special: %s split into %v", fn, fnlist)
		return fnlist, nil
	}
	if len(tabsplit) == 3 {
		logger.Debug("PreProcess-special: %s is tab-3", fn)
		s := bufio.NewScanner(fh)
		tracker := make(map[string]*os.File)
		for s.Scan() {
			if err = s.Err(); err != nil {
				return nil, err
			}
			line := strings.Split(s.Text(), "\t")
			if len(line) < 3 {
				logger.Detail("PreProcess-Special: %s is tab-3 format but found line without 3 tabs: %s", fn, line)
				continue
			}
			if line[1] == "pod_name" && strings.TrimSuffix(line[2], "\n") == "text_payload" { // timestamp       pod_name        text_payload
				logger.Detail("PreProcess-Special: %s tab-3 header found, ignoring", fn)
				continue
			}
			ident := strings.Trim(line[1], "\r\n\t ")
			logline := strings.Trim(line[2], "\r\t\n ") + "\n"
			if _, ok := tracker[fn+"_special-split_"+ident]; !ok {
				out, err := os.Create(fn + "_special-split_" + ident)
				if err != nil {
					return fnlist, err
				}
				defer out.Close()
				tracker[fn+"_special-split_"+ident] = out
				fnlist = append(fnlist, fn+"_special-split_"+ident)
			}
			_, err = tracker[fn+"_special-split_"+ident].WriteString(logline)
			if err != nil {
				return fnlist, err
			}
		}
		logger.Debug("PreProcess-special: %s split into %v", fn, fnlist)
		return fnlist, nil
	}
	// test for json
	if !mimeType.Is("application/json") && !mimeType.Is("application/x-ndjson") {
		return nil, errPreProcessNotSpecial
	}

	// try one-line decoder
	logger.Debug("PreProcess-special: %s is json", fn)
	dec := json.NewDecoder(fh)
	v := new(jsonPayload)
	tracker := make(map[string]*os.File)
	errCount := 0
	for {
		err = dec.Decode(v)
		if err != nil && err == io.EOF {
			break
		} else if err != nil {
			logger.Detail("PreProcess-Special: json error in %s: %s", fn, err)
			errCount++
			if errCount > 1000 {
				return fnlist, fmt.Errorf("encountered >1000 errors processing json; last error: %s", err)
			}
			continue
		}
		line := ""
		ident := "noident"
		if v.Log != "" {
			line = v.Log
		} else if v.JsonPayload.Log != "" {
			line = v.JsonPayload.Log
		} else if v.TextPayload != "" {
			line = v.TextPayload
			if v.Resource.Labels.PodName != "" {
				ident = v.Resource.Labels.PodName
			}
		} else {
			ts, err := time.Parse("2006-01-02T15:04:05Z", v.Timestamp)
			if err != nil {
				logger.Detail("PreProcess-special: found json, but did not match 3 simple formats, and didn't find timestamp in complex format in %s: %s", fn, err)
				continue
			}
			line = fmt.Sprintf("%s: %s (%s): (%s) %s", ts.Format("Jan 02 2006 15:04:05 MST"), v.JsonPayload.Level, v.JsonPayload.Module, v.JsonPayload.ModuleDetail, v.JsonPayload.Message)
		}
		if _, ok := tracker[fn+"_special-split_"+ident]; !ok {
			out, err := os.Create(fn + "_special-split_" + ident)
			if err != nil {
				return fnlist, err
			}
			defer out.Close()
			tracker[fn+"_special-split_"+ident] = out
			fnlist = append(fnlist, fn+"_special-split_"+ident)
		}
		if !strings.HasSuffix(line, "\n") {
			line = line + "\n"
		}
		_, err = tracker[fn+"_special-split_"+ident].WriteString(line)
		if err != nil {
			return fnlist, err
		}
	}
	logger.Debug("PreProcess-special: %s split into %v", fn, fnlist)
	return fnlist, nil
}
