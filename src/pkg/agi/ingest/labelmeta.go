package ingest

import (
	"encoding/json"
	"log"
)

// LoadMetaEntriesFromLabelsDB loads existing per-label meta rows from
// the labels set into a map suitable for NewMetaShards. Safe to call
// while batch ingest is idle; used by the live HTTP listener.
func (i *Ingest) LoadMetaEntriesFromLabelsDB() (MetaEntries, error) {
	meta := make(MetaEntries)
	if i.patterns == nil || i.patterns.LabelsSetName == "" {
		return meta, nil
	}
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
		metaItem.ensureIdx()
		meta[key] = metaItem
	}
	err := it.Err()
	_ = it.Close()
	return meta, err
}

// NewMetaShards wraps a label meta map for concurrent live ingestion.
func NewMetaShards(meta MetaEntries) *MetaShards {
	if meta == nil {
		meta = make(MetaEntries)
	}
	return &MetaShards{meta: meta}
}
