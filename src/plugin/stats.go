package plugin

import (
	"fmt"
	"runtime"
	"time"

	"github.com/bestmethod/logger"
)

func (p *Plugin) stats() {
	if p.config.LogLevel < 5 {
		return
	}
	oldjobs := -1
	jobs := -1
	requests := -1
	oldrequests := -1
	for {
		requests = len(p.requests)
		jobs = len(p.jobs)
		if oldjobs != jobs || oldrequests != requests {
			oldrequests = requests
			oldjobs = jobs
			p.printStats()
		}
		time.Sleep(time.Second)
	}
}

func (p *Plugin) printStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	logger.Debug("STAT: HEAP: Alloc=%s TotalAlloc=%s Sys=%s NumGC=%d HeapObjects=%d InUse=%s Idle=%s", convSize(m.Alloc), convSize(m.TotalAlloc), convSize(m.Sys), m.NumGC, m.HeapObjects, convSize(m.HeapInuse), convSize(m.HeapIdle))
	logger.Debug("STAT: REQUESTS=%d JOBS=%d", len(p.requests), len(p.jobs))
}

func convSize(size uint64) string {
	var sizeString string
	if size > 1023 && size < 1024*1024 {
		sizeString = fmt.Sprintf("%.2f KB", float64(size)/1024)
	} else if size < 1024 {
		sizeString = fmt.Sprintf("%v B", size)
	} else if size >= 1024*1024 && size < 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f MB", float64(size)/1024/1024)
	} else if size >= 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f GB", float64(size)/1024/1024/1024)
	}
	return sizeString
}
