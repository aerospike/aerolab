package plugin

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rglonek/sbs"
	"log"
)

func (p *Plugin) queryAndCache() {
	for {
		log.Printf("DEBUG: Starting cache refresh")
		log.Printf("DETAIL: Cache: set list")
		if err := p.cacheSetList(); err != nil {
			log.Printf("WARN: Could not get set list: %s", err)
		}
		log.Printf("DETAIL: Cache: bin list")
		if err := p.cacheBinList(); err != nil {
			log.Printf("WARN: Could not get bin list: %s", err)
		}
		log.Printf("DETAIL: Cache: metadata list")
		if err := p.cacheMetadataList(); err != nil {
			log.Printf("WARN: Could not get metadata list: %s", err)
		}
		log.Printf("DEBUG: Finished cache refresh, sleeping %v", p.config.CacheRefreshInterval)
		// Sleep with cancellation: Close() closes p.done so we exit
		// promptly instead of waking up after the db has already
		// been closed.
		select {
		case <-p.done:
			log.Printf("DEBUG: cache refresh: shutdown signalled, exiting")
			return
		case <-time.After(p.config.CacheRefreshInterval):
		}
	}
}

func (p *Plugin) cacheSetList() error {
	sets := p.db.Sets()
	p.cache.lock.Lock()
	p.cache.setNames = sets
	p.cache.lock.Unlock()
	// Warn once per set that exists but has no indexed timestamp
	// column. A Query(set).Between(TimestampBinName, ...) on such a
	// set silently degrades to a full-set scan with the Between
	// predicate evaluated in memory — correct but O(N) in the whole
	// set's row count. Almost always this means ingest's
	// registerSets missed the set, e.g. a user-added pattern in
	// PatternsFile that predates this build; the fix is to
	// RegisterSet it at ingest time.
	ts := p.config.TimestampBinName
	for _, name := range sets {
		if name == p.config.LabelsSetName {
			continue // labels set is intentionally non-indexed
		}
		schema, ok := p.db.SchemaOf(name)
		if !ok {
			continue
		}
		foundIndexedTs := false
		hasTs := false
		for _, col := range schema {
			if col.Name == ts {
				hasTs = true
				if col.Indexed {
					foundIndexedTs = true
				}
				break
			}
		}
		if hasTs && !foundIndexedTs {
			p.cache.lock.Lock()
			if p.cache.warnedNonIndexed == nil {
				p.cache.warnedNonIndexed = make(map[string]bool)
			}
			warned := p.cache.warnedNonIndexed[name]
			p.cache.warnedNonIndexed[name] = true
			p.cache.lock.Unlock()
			if !warned {
				log.Printf("WARN: set %q has column %q but it is not indexed; timeseries Between queries will fall back to full scan", name, ts)
			}
		}
	}
	return nil
}

func (p *Plugin) cacheBinList() error {
	row, err := p.db.Get(p.config.LabelsSetName, "BINLIST", labelsValueCol)
	if err != nil {
		return fmt.Errorf("db.Get BINLIST: %s", err)
	}
	if row == nil {
		// BINLIST has not been written yet (fresh AGI before its
		// first ingest cycle). Don't treat this as an error or it
		// becomes a noisy WARN every CacheRefreshInterval. Leave
		// the existing bin cache untouched; the next refresh will
		// pick up the row once ingest writes it.
		log.Printf("DEBUG: cacheBinList: BINLIST not yet present; skipping refresh")
		return nil
	}
	v, ok := row[labelsValueCol]
	if !ok {
		return fmt.Errorf("db.Get BINLIST: %s column not present", labelsValueCol)
	}
	raw, ok := v.AsString()
	if !ok {
		return fmt.Errorf("db.Get BINLIST: %s column is not a string", labelsValueCol)
	}
	bins := []string{}
	if err := json.Unmarshal(sbs.StringToByteSlice(raw), &bins); err != nil {
		return fmt.Errorf("could not unmarshal bin list: %s", err)
	}
	p.cache.lock.Lock()
	p.cache.binNames = bins
	p.cache.lock.Unlock()
	return nil
}

type metaEntries struct {
	Entries          []string
	ByCluster        map[string][]int
	StaticEntriesIdx []int
}

// labelsControlKeys is the set of rows in the labels set that are not
// "real" categorical metadata (they are engine/operational records
// written by ingest: BINLIST, cfName, sources, timerange). Ingest has
// a symmetric skip list in ProcessLogsPrep. If the plugin puts these
// into p.cache.metadata, they leak into Grafana's variable lists and
// pollute label-driven lookups in handleHistogram and elsewhere.
var labelsControlKeys = map[string]struct{}{
	"BINLIST":   {},
	"cfName":    {},
	"sources":   {},
	"timerange": {},
}

func (p *Plugin) cacheMetadataList() error {
	meta := make(map[string]*metaEntries)
	it := p.db.Scan(p.config.LabelsSetName, labelsValueCol)
	defer it.Close()
	for it.Next() {
		key, row := it.Record()
		if _, skip := labelsControlKeys[key]; skip {
			continue
		}
		v, ok := row[labelsValueCol]
		if !ok {
			continue
		}
		raw, ok := v.AsString()
		if !ok {
			continue
		}
		metaItem := &metaEntries{}
		if err := json.Unmarshal(sbs.StringToByteSlice(raw), &metaItem); err != nil {
			log.Printf("WARN: Failed to unmarshal existing label data for %s: %s", key, err)
			continue
		}
		meta[key] = metaItem
	}
	if err := it.Err(); err != nil {
		return fmt.Errorf("could not read existing labels: %s", err)
	}
	p.cache.lock.Lock()
	p.cache.metadata = meta
	p.cache.lock.Unlock()
	return nil
}
