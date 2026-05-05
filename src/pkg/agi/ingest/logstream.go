package ingest

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

var errNotMatched = errors.New("LINE NOT MATCHED")

type logStream struct {
	defId               int
	timestampDefId      int
	patterns            *patterns
	TimeRanges          *TimeRanges
	timestampDefIdx     int
	timestampNeedsRegex bool
	timestampStatName   string
	multilineItems      map[string]*multilineItem
	aggregateItems      map[string]*aggregator // where string is the unique string to aggregate against
	logFileStartTime    time.Time
	logFileEndTime      time.Time
}

type LogStreamOutput struct {
	Data     map[string]any
	Metadata map[string]any
	Line     string
	Error    error
	SetName  string
}

type multilineItem struct {
	line       string
	timestamp  time.Time
	nodePrefix int
}

type aggregator struct {
	stat      int              // the stat that increases
	startTime time.Time        // time first encountered, resets if we fell outside of the aggregation period
	endTime   time.Time        //  startTime+Every aggregation window
	out       *LogStreamOutput // this will be set as time goes by to allow for dumping of data, the stat will need to be overridden
}

func newLogStream(clusterName string, p *patterns, t *TimeRanges, timestampName string) *logStream {
	defId := 0
	for i := range p.Defs {
		if p.Defs[i].ClusterName == clusterName {
			defId = i
			break
		}
	}
	tsDefId := 0
	for i := range p.Timestamps {
		if p.Timestamps[i].ClusterName == clusterName {
			tsDefId = i
			break
		}
	}
	return &logStream{
		defId:             defId,
		timestampDefId:    tsDefId,
		patterns:          p,
		TimeRanges:        t,
		multilineItems:    make(map[string]*multilineItem),
		timestampDefIdx:   -1,
		timestampStatName: timestampName,
		aggregateItems:    make(map[string]*aggregator),
	}
}

func (s *logStream) Close() (outputs []*LogStreamOutput, logStartTime time.Time, logEndTime time.Time) {
	ret := []*LogStreamOutput{}
	for _, a := range s.aggregateItems {
		ret = append(ret, a.out)
	}
	s.aggregateItems = make(map[string]*aggregator)
	for _, mit := range s.multilineItems {
		ra, err := s.lineProcess(mit.line, mit.timestamp, mit.nodePrefix)
		if err == nil {
			ret = append(ret, ra...)
		}
	}
	s.multilineItems = make(map[string]*multilineItem)
	return ret, s.logFileStartTime, s.logFileEndTime
}

const parseTimeError = "TIME PARSE: %s"

func (s *logStream) Process(line string, nodePrefix int) ([]*LogStreamOutput, error) {
	timestamp, lineOffset, err := s.lineGetTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf(parseTimeError, err)
	}
	if s.TimeRanges != nil {
		if !s.TimeRanges.From.IsZero() && timestamp.Before(s.TimeRanges.From) {
			return nil, nil
		}
		if !s.TimeRanges.To.IsZero() && timestamp.After(s.TimeRanges.To) {
			return nil, nil
		}
	}
	if lineOffset > 0 {
		line = line[lineOffset:]
	}
	out := []*LogStreamOutput{}
	for _, m := range s.patterns.Multiline {
		// check and handle new multiline
		if strings.Contains(line, m.StartLineSearch) {
			if _, ok := s.multilineItems[m.StartLineSearch]; ok {
				outx, err := s.lineProcess(s.multilineItems[m.StartLineSearch].line, s.multilineItems[m.StartLineSearch].timestamp, s.multilineItems[m.StartLineSearch].nodePrefix)
				s.multilineItems[m.StartLineSearch] = &multilineItem{
					line:       line,
					timestamp:  timestamp,
					nodePrefix: nodePrefix,
				}
				if err != nil {
					return nil, err
				}
				if len(outx) > 0 {
					out = append(out, outx...)
				}
				return out, nil
			}
			s.multilineItems[m.StartLineSearch] = &multilineItem{
				line:       line,
				timestamp:  timestamp,
				nodePrefix: nodePrefix,
			}
			return out, nil
		}
		// check and handle existing multiline
		for mStart := range s.multilineItems {
			if m.StartLineSearch == mStart {
				results := m.reMatchLines.FindStringSubmatch(line)
				if len(results) > 0 {
					if timestamp.Before(s.multilineItems[mStart].timestamp) {
						//if s.multilineItems[mStart].timestamp != timestamp { // turns out this can happen
						return nil, errors.New("multiline statistic had timestamps move backwards in time")
					}
					// append multiline string
					for mpi, mp := range m.ReMatchJoin {
						results := mp.re.FindStringSubmatch(line)
						if len(results) > 1 {
							// we matched the line, now join results into multiline string
							for iresult, result := range results {
								if iresult != m.ReMatchJoin[mpi].MatchSeq {
									continue
								}
								s.multilineItems[mStart].line = s.multilineItems[mStart].line + result
							}
							return nil, nil
						}
					}
					return nil, nil
				}
				break
			}
		}
	}

	// non-multiline, just process
	outx, err := s.lineProcess(line, timestamp, nodePrefix)
	if len(outx) > 0 {
		out = append(out, outx...)
	}
	return out, err
}

func (s *logStream) lineGetTimestamp(line string) (timestamp time.Time, lineOffset int, err error) {
	if s.timestampDefIdx >= 0 {
		var tsString string
		if s.timestampNeedsRegex {
			sloc := s.patterns.Timestamps[s.timestampDefId].Defs[s.timestampDefIdx].regex.FindStringIndex(line)
			if sloc == nil {
				err = errors.New("regex not matched")
				return
			}
			tsString = line[sloc[0]:sloc[1]]
			lineOffset = sloc[0]
		} else {
			if len(line) < len(s.patterns.Timestamps[s.timestampDefId].Defs[s.timestampDefIdx].Definition) {
				err = errors.New("line too short")
				return
			}
			tsString = line[0:len(s.patterns.Timestamps[s.timestampDefId].Defs[s.timestampDefIdx].Definition)]
		}
		timestamp, err = time.Parse(s.patterns.Timestamps[s.timestampDefId].Defs[s.timestampDefIdx].Definition, tsString)
		if err == nil && timestamp.Year() == 0 {
			timestamp = timestamp.AddDate(time.Now().Year(), 0, 0)
			if timestamp.After(time.Now().Add(24 * time.Hour)) {
				timestamp = timestamp.AddDate(-1, 0, 0)
			}
		}
		return
	}

	for j, def := range s.patterns.Timestamps[s.timestampDefId].Defs {
		if len(line) < len(def.Definition) {
			continue
		}
		tsString := line[0:len(def.Definition)]
		timestamp, err = time.Parse(def.Definition, tsString)
		if err == nil && timestamp.Year() == 0 {
			timestamp = timestamp.AddDate(time.Now().Year(), 0, 0)
			if timestamp.After(time.Now().Add(24 * time.Hour)) {
				timestamp = timestamp.AddDate(-1, 0, 0)
			}
		}
		if err == nil {
			s.timestampDefIdx = j
			return
		}
	}

	for j, def := range s.patterns.Timestamps[s.timestampDefId].Defs {
		sloc := def.regex.FindStringIndex(line)
		if sloc == nil {
			continue
		}
		tsString := line[sloc[0]:sloc[1]]
		lineOffset = sloc[0]
		timestamp, err = time.Parse(def.Definition, tsString)
		if err == nil && timestamp.Year() == 0 {
			timestamp = timestamp.AddDate(time.Now().Year(), 0, 0)
			if timestamp.After(time.Now().Add(24 * time.Hour)) {
				timestamp = timestamp.AddDate(-1, 0, 0)
			}
		}
		if err == nil {
			s.timestampDefIdx = j
			s.timestampNeedsRegex = true
			return
		}
	}
	err = errNoTimestamp
	return
}

var errNoTimestamp = errors.New("timestamp not found")

// coerceStringInts mutates m in place, converting any string value
// that parses as a base-10 integer into an int. Called by lineProcess
// at emission time so the downstream worker (ProcessLogs) and the
// aggregator pipeline both observe already-typed scalars and the
// per-row "results := make(map[string]any)" copy that used to live in
// processLogFile is no longer needed.
//
// Only the non-aggregator emission and the new-aggregator storage call
// this; the aggregator-update branch writes int directly and the
// aggregator-flushed branch reuses an already-coerced map. Histogram
// padding is applied before this call so the synthetic "tail" /
// "<bucket>plus" int values remain ints (they were never strings).
func coerceStringInts(m map[string]any) {
	for k, v := range m {
		if vt, ok := v.(string); ok {
			if vint, err := strconv.Atoi(vt); err == nil {
				m[k] = vint
			}
		}
	}
}

func (s *logStream) lineProcess(line string, timestamp time.Time, nodePrefix int) ([]*LogStreamOutput, error) {
	def := s.patterns.Defs[s.defId]
	// Replace the linear `for _, p := range Patterns { if
	// !strings.Contains(line, p.Search) continue }` scan with one
	// Aho-Corasick lookup that returns the smallest pattern index
	// whose `search` substring appears in the line. First-match-
	// wins semantic is preserved exactly: FirstIndex returns the
	// smallest pattern index, just like the linear loop did. The
	// inner regex / RegexAdvanced extraction below is untouched —
	// AC only narrows the candidate from N patterns to 1.
	//
	// Tests / patterns built without compile() (matcher == nil)
	// fall through to the original linear behavior so this file
	// remains usable in isolation.
	patterns := def.Patterns
	if def.matcher != nil {
		idx := def.matcher.FirstIndex(line)
		if idx < 0 {
			return nil, errNotMatched
		}
		patterns = def.Patterns[idx : idx+1]
	}
	for _, p := range patterns {
		if !strings.Contains(line, p.Search) {
			continue
		}
		// replace
		for _, r := range p.Replace {
			line = r.regex.ReplaceAllString(line, r.Sub)
		}
		// find string submatch
		for _, r := range p.regex {
			setName := p.Name
			results := []string{}
			if p.Regex[0] != "DROP TO ADVANCED EXPORT" {
				results = r.FindStringSubmatch(line)
			}
			if len(results) == 0 {
				for _, rr := range p.RegexAdvanced {
					setName = rr.SetName
					results = rr.regex.FindStringSubmatch(line)
					if len(results) > 0 {
						r = rr.regex
						break
					}
				}
			}
			if len(results) == 0 {
				continue
			}
			resultNames := r.SubexpNames()
			nRes := make(map[string]any)
			if p.StoreNodePrefix != "" {
				nRes[p.StoreNodePrefix] = nodePrefix
			}
			nMeta := make(map[string]any)
			for rIndex, result := range results {
				if rIndex == 0 {
					continue
				}
				nKey := resultNames[rIndex]
				if slices.Contains(s.patterns.GlobalLabels, nKey) || slices.Contains(p.Labels, nKey) {
					nMeta[nKey] = result
				} else {
					nRes[nKey] = result
				}
			}
			// handle histogram
			if p.Histogram != nil && len(p.Histogram.Buckets) > 0 {
				histograms := nRes["histogram"].(string) // eg: (00: 29931197672) (01: 0005096257) (02: 0001437128) (03: 0000148677) (04: 0000008291) (05: 0000002562) (06: 0000002307) (07: 0000001008) ...
				delete(nRes, "histogram")
				// store histogram in nRes["00"] etc
				hInd := 0
				hIndE := 0
				for {
					hInd = strings.Index(histograms, "(")
					if hInd < 0 {
						break
					}
					hIndE = strings.Index(histograms, ")")
					if hIndE < 0 {
						break
					}
					newHist := strings.Split(histograms[hInd+1:hIndE], ": ")
					if len(newHist) != 2 {
						histograms = histograms[hIndE+1:]
						continue
					}
					if slices.Contains(p.Histogram.Buckets, newHist[0]) {
						nRes[newHist[0]] = newHist[1]
					}
					histograms = histograms[hIndE+1:]
				}
			}
			// default value padding
			for padKey, padVal := range p.DefaultValuePadding {
				if _, ok := nRes[padKey]; !ok {
					nRes[padKey] = padVal
				}
			}
			if p.Histogram != nil && p.Histogram.GenCumulative && len(p.Histogram.Buckets) > 0 {
				buckets := make(map[string]int)
				total, _ := strconv.Atoi(nRes["total"].(string))
				for _, bucket := range p.Histogram.Buckets {
					buckets[bucket], _ = strconv.Atoi(nRes[bucket].(string))
				}
				tail := total
				for _, v := range buckets {
					tail -= v
				}
				nRes["tail"] = tail
				for vi, v := range p.Histogram.Buckets {
					n := v + "plus"
					val := tail
					for _, vv := range p.Histogram.Buckets[vi:] {
						val += buckets[vv]
					}
					nRes[n] = val
				}
			}
			nRes[s.timestampStatName] = timestamp.UnixMilli()
			//aggregate
			ret := []*LogStreamOutput{}
			aggrToDelete := []string{}
			for aguniq, ag := range s.aggregateItems {
				if ag.endTime.After(timestamp) {
					continue
				}
				aggrToDelete = append(aggrToDelete, aguniq)
				ret = append(ret, ag.out)
				if s.logFileStartTime.IsZero() || ag.startTime.Before(s.logFileStartTime) {
					s.logFileStartTime = ag.startTime
				}
				if s.logFileEndTime.IsZero() || ag.endTime.After(s.logFileEndTime) {
					s.logFileEndTime = ag.endTime
				}
			}
			for _, ag := range aggrToDelete {
				delete(s.aggregateItems, ag)
			}
			if p.Aggregate != nil && p.Aggregate.Field != "" {
				newVal, _ := strconv.Atoi(nRes[p.Aggregate.Field].(string))
				if p.Aggregate.Increment {
					newVal++
					nRes[p.Aggregate.Field] = strconv.Itoa(newVal)
				}
				if nMeta[p.Aggregate.On] == nil {
					return nil, fmt.Errorf("AGGREGATION FAILURE: aggregation item %s is not in the patterns `labels:` section", p.Aggregate.On)
				}
				uniq := nMeta[p.Aggregate.On].(string)
				if _, ok := s.aggregateItems[uniq]; !ok {
					// Coerce string-int values to int now,
					// once, so the aggregator stores the
					// already-typed shape and processLogFile
					// no longer has to allocate a per-row map
					// and copy fields just to perform the same
					// coercion. Subsequent updates of this
					// aggregator's Field are written as int
					// directly (see else branch), preserving
					// the column type.
					coerceStringInts(nRes)
					s.aggregateItems[uniq] = &aggregator{
						stat:      newVal,
						startTime: timestamp,
						endTime:   timestamp.Add(p.Aggregate.Every),
						out: &LogStreamOutput{
							Data:     nRes,
							Metadata: nMeta,
							Line:     line,
							Error:    nil,
							SetName:  setName,
						},
					}
				} else {
					s.aggregateItems[uniq].stat += newVal
					s.aggregateItems[uniq].out.Data[p.Aggregate.Field] = s.aggregateItems[uniq].stat
				}
				return ret, nil
			}
			// Coerce string-int values once at emission time
			// (bonus: the per-row map allocation in
			// processLogFile is gone now that nRes carries the
			// final types).
			coerceStringInts(nRes)
			ret = append(ret, &LogStreamOutput{
				Data:     nRes,
				Metadata: nMeta,
				Line:     line,
				Error:    nil,
				SetName:  setName,
			})
			if s.logFileStartTime.IsZero() || timestamp.Before(s.logFileStartTime) {
				s.logFileStartTime = timestamp
			}
			if s.logFileEndTime.IsZero() || timestamp.After(s.logFileEndTime) {
				s.logFileEndTime = timestamp
			}
			return ret, nil
		}
	}
	return nil, errNotMatched
}
