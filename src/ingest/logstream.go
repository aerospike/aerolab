package ingest

import (
	"errors"
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
	multilineItems      map[string]*multilineItem
}

type logStreamOutput struct {
	Data     map[string]interface{}
	Metadata map[string]interface{}
	Line     string
	Error    error
	SetName  string
}

type multilineItem struct {
	line      string
	timestamp time.Time
}

func newLogStream(p *patterns, t *TimeRanges) *logStream {
	return &logStream{
		patterns:        p,
		TimeRanges:      t,
		multilineItems:  make(map[string]*multilineItem),
		timestampDefIdx: -1,
	}
}

func (s *logStream) Close() []*logStreamOutput {
	return nil
}

func (s *logStream) Process(line string) ([]*logStreamOutput, error) {
	timestamp, lineOffset, err := s.lineGetTimestamp(line)
	if err != nil {
		return nil, err
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
				outx, err := s.lineProcess(s.multilineItems[m.StartLineSearch].line, s.multilineItems[m.StartLineSearch].timestamp)
				s.multilineItems[m.StartLineSearch] = &multilineItem{
					line:      line,
					timestamp: timestamp,
				}
				if err != nil {
					return nil, err
				}
				if outx != nil {
					out = append(out, outx)
				}
				return out, nil
			}
			s.multilineItems[m.StartLineSearch] = &multilineItem{
				line:      line,
				timestamp: timestamp,
			}
			return out, nil
		}
		// check and handle existing multiline
		for mStart := range s.multilineItems {
			if m.StartLineSearch == mStart {
				results := m.reMatchLines.FindStringSubmatch(line)
				if len(results) > 0 {
					if s.multilineItems[mStart].timestamp != timestamp {
						return nil, errors.New("timestamp mismatch between multiple lines of multiline statistic")
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
	outx, err := s.lineProcess(line, timestamp)
	if outx != nil {
		out = append(out, outx)
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
	err = errors.New("timestamp not found")
	return
}

func (s *logStream) lineProcess(line string, timestamp time.Time) (*logStreamOutput, error) {
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
			results := r.FindStringSubmatch(line)
			if len(results) == 0 {
				continue
			}
			resultNames := r.SubexpNames()
			nRes := make(map[string]interface{})
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
					if hInd < 0 {
						break
					}
					newHist := strings.Split(histograms[hInd+1:hIndE], ": ")
					if len(newHist) != 2 {
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
			// TODO apply special aggregation (like for warning messages)
			return &logStreamOutput{
				Data:     nRes,
				Metadata: nMeta,
				Line:     line,
				Error:    nil,
				SetName:  p.Name,
			}, nil
		}
	}
	return nil, errNotMatched
}