package plugin

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bestmethod/inslice"
	"github.com/bestmethod/logger"
)

func (p *Plugin) queryAndCache() {
	for {
		logger.Debug("Starting cache refresh")
		logger.Detail("Cache: set list")
		err := p.cacheSetList()
		if err != nil {
			logger.Warn("Could not get set list: %s", err)
		}
		logger.Detail("Cache: bin list")
		err = p.cacheBinList()
		if err != nil {
			logger.Warn("Could not get bin list: %s", err)
		}
		logger.Detail("Cache: metadata list")
		err = p.cacheMetadataList()
		if err != nil {
			logger.Warn("Could not get metadata list: %s", err)
		}
		logger.Debug("Finished cache refresh, sleeping %v", p.config.CacheRefreshInterval)
		time.Sleep(p.config.CacheRefreshInterval)
	}
}

func (p *Plugin) cacheSetList() error {
	sets := []string{}
	for _, node := range p.db.GetNodes() {
		setList, err := node.RequestInfo(p.ip, fmt.Sprintf("sets/%s", p.config.Aerospike.Namespace))
		if err != nil {
			return fmt.Errorf("aerospike.RequestInfo: %s", err)
		}
		nsets := setList[fmt.Sprintf("sets/%s", p.config.Aerospike.Namespace)]
		for _, nset := range strings.Split(nsets, ":") {
			if strings.HasPrefix(nset, "set=") {
				nsetname := strings.Split(nset, "=")[1]
				if len(inslice.String(sets, nsetname, 1)) == 0 {
					sets = append(sets, nsetname)
				}
			}
		}
	}
	p.cache.lock.Lock()
	p.cache.setNames = sets
	p.cache.lock.Unlock()
	return nil
}

func (p *Plugin) cacheBinList() error {
	bins := []string{}
	for _, node := range p.db.GetNodes() {
		binList, err := node.RequestInfo(p.ip, fmt.Sprintf("bins/%s", p.config.Aerospike.Namespace))
		if err != nil {
			return fmt.Errorf("aerospike.RequestInfo: %s", err)
		}
		nbins := binList[fmt.Sprintf("bins/%s", p.config.Aerospike.Namespace)]
		for _, nbin := range strings.Split(nbins, ",") {
			if !strings.Contains(nbin, "=") {
				nbin := strings.Trim(nbin, ";\n")
				if len(inslice.String(bins, nbin, 1)) == 0 {
					bins = append(bins, nbin)
				}
			}
		}
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

func (p *Plugin) cacheMetadataList() error {
	meta := make(map[string]*metaEntries)
	recset, err := p.db.ScanAll(p.scanPolicy(), p.config.Aerospike.Namespace, p.config.LabelsSetName)
	if err != nil {
		return fmt.Errorf("could not read existing labels: %s", err)
	}
	for rec := range recset.Results() {
		if err := rec.Err; err != nil {
			return fmt.Errorf("error iterating through existing labels: %s", err)
		}
		for k, v := range rec.Record.Bins {
			metaItem := &metaEntries{}
			nerr := json.Unmarshal([]byte(v.(string)), &metaItem)
			if nerr != nil {
				logger.Warn("Failed to unmarshal existing label data for %s: %s", k, nerr)
			}
			meta[k] = metaItem
		}
	}
	p.cache.lock.Lock()
	p.cache.metadata = meta
	p.cache.lock.Unlock()
	return nil
}
