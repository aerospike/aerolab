package ingest

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bestmethod/inslice"
)

var errNotMatched = errors.New("LINE NOT MATCHED")

type logStream struct {
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

type logStreamOutput struct {
	Data     map[string]interface{}
	Metadata map[string]interface{}
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
	out       *logStreamOutput // this will be set as time goes by to allow for dumping of data, the stat will need to be overridden
}

func newLogStream(p *patterns, t *TimeRanges, timestampName string) *logStream {
	return &logStream{
		patterns:          p,
		TimeRanges:        t,
		multilineItems:    make(map[string]*multilineItem),
		timestampDefIdx:   -1,
		timestampStatName: timestampName,
		aggregateItems:    make(map[string]*aggregator),
	}
}

func (s *logStream) Close() (outputs []*logStreamOutput, logStartTime time.Time, logEndTime time.Time) {
	ret := []*logStreamOutput{}
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

func (s *logStream) Process(line string, nodePrefix int) ([]*logStreamOutput, error) {
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
	out := []*logStreamOutput{}
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
			sloc := s.patterns.Timestamps[s.timestampDefIdx].regex.FindStringIndex(line)
			if sloc == nil {
				err = errors.New("regex not matched")
				return
			}
			tsString = line[sloc[0]:sloc[1]]
			lineOffset = sloc[0]
		} else {
			if len(line) < len(s.patterns.Timestamps[s.timestampDefIdx].Definition) {
				err = errors.New("line too short")
				return
			}
			tsString = line[0:len(s.patterns.Timestamps[s.timestampDefIdx].Definition)]
		}
		timestamp, err = time.Parse(s.patterns.Timestamps[s.timestampDefIdx].Definition, tsString)
		return
	}

	for j, def := range s.patterns.Timestamps {
		if len(line) < len(def.Definition) {
			continue
		}
		tsString := line[0:len(def.Definition)]
		timestamp, err = time.Parse(def.Definition, tsString)
		if err == nil {
			s.timestampDefIdx = j
			return
		}
	}

	for j, def := range s.patterns.Timestamps {
		sloc := def.regex.FindStringIndex(line)
		if sloc == nil {
			continue
		}
		tsString := line[sloc[0]:sloc[1]]
		lineOffset = sloc[0]
		timestamp, err = time.Parse(def.Definition, tsString)
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

func (s *logStream) lineProcess(line string, timestamp time.Time, nodePrefix int) ([]*logStreamOutput, error) {
	for _, p := range s.patterns.Patterns {
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
			nRes := make(map[string]interface{})
			if p.StoreNodePrefix != "" {
				nRes[p.StoreNodePrefix] = nodePrefix
			}
			nMeta := make(map[string]interface{})
			for rIndex, result := range results {
				if rIndex == 0 {
					continue
				}
				nKey := resultNames[rIndex]
				if inslice.HasString(s.patterns.GlobalLabels, nKey) || inslice.HasString(p.Labels, nKey) {
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
					if inslice.HasString(p.Histogram.Buckets, newHist[0]) {
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
			ret := []*logStreamOutput{}
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
				uniq := nMeta[p.Aggregate.On].(string)
				if _, ok := s.aggregateItems[uniq]; !ok {
					s.aggregateItems[uniq] = &aggregator{
						stat:      newVal,
						startTime: timestamp,
						endTime:   timestamp.Add(p.Aggregate.Every),
						out: &logStreamOutput{
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
			ret = append(ret, &logStreamOutput{
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
