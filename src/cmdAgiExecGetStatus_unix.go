//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris
// +build darwin dragonfly freebsd linux netbsd openbsd solaris

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/ingest"
	ps "github.com/mitchellh/go-ps"
	"golang.org/x/sys/unix"
)

func getAgiStatus(enabled bool, ingestProgressPath string) (*ingest.IngestStatusStruct, error) {
	status := new(ingest.IngestStatusStruct)

	if !enabled {
		return status, nil
	}

	var stat unix.Statfs_t
	err := unix.Statfs("/opt/agi", &stat)
	if err != nil {
		return nil, err
	}
	status.System.DiskFreeBytes = stat.Bavail * uint64(stat.Bsize)
	status.System.DiskTotalBytes = stat.Blocks * uint64(stat.Bsize)

	ram, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	rams := bufio.NewScanner(bytes.NewReader(ram))
	for rams.Scan() {
		line := strings.TrimRight(rams.Text(), "\r\n\t ")
		if !strings.HasSuffix(line, "kB") {
			continue
		}
		if strings.HasPrefix(line, "MemTotal:") {
			status.System.MemoryTotalBytes, _ = strconv.Atoi(cut(line, 2, " "))
			status.System.MemoryTotalBytes = status.System.MemoryTotalBytes * 1024
		} else if strings.HasPrefix(line, "MemAvailable:") {
			status.System.MemoryFreeBytes, _ = strconv.Atoi(cut(line, 2, " "))
			status.System.MemoryFreeBytes = status.System.MemoryFreeBytes * 1024
		}
		if status.System.MemoryTotalBytes > 0 && status.System.MemoryFreeBytes > 0 {
			break
		}
	}

	plist, err := ps.Processes()
	if err != nil {
		return nil, err
	}
	for _, p := range plist {
		if strings.HasSuffix(p.Executable(), "asd") {
			status.AerospikeRunning = true
			break
		}
	}
	pidf, err := os.ReadFile("/opt/agi/ingest.pid")
	if err == nil {
		pid, err := strconv.Atoi(string(pidf))
		if err == nil {
			_, err := os.FindProcess(pid)
			if err == nil {
				status.Ingest.Running = true
			}
		}
	}
	pidf, err = os.ReadFile("/opt/agi/plugin.pid")
	if err == nil {
		pid, err := strconv.Atoi(string(pidf))
		if err == nil {
			_, err := os.FindProcess(pid)
			if err == nil {
				status.PluginRunning = true
			}
		}
	}
	pidf, err = os.ReadFile("/opt/agi/grafanafix.pid")
	if err == nil {
		pid, err := strconv.Atoi(string(pidf))
		if err == nil {
			_, err := os.FindProcess(pid)
			if err == nil {
				status.GrafanaHelperRunning = true
			}
		}
	}
	steps := new(ingest.IngestSteps)
	f, err := os.ReadFile("/opt/agi/ingest/steps.json")
	if err == nil {
		json.Unmarshal(f, steps)
	}
	status.Ingest.CompleteSteps = steps

	err = func() error {
		reader, err := agiGetReader(ingestProgressPath, "downloader.json")
		if err != nil {
			return err
		}
		defer reader.Close()
		p := new(ingest.ProgressDownloader)
		json.NewDecoder(reader).Decode(p)
		totalSize := int64(0)
		dlSize := int64(0)
		for fn, f := range p.S3Files {
			if f.Error != "" {
				status.Ingest.Errors = append(status.Ingest.Errors, "Downloader::"+fn+"::1::"+f.Error)
			}
			totalSize += f.Size
			if f.IsDownloaded {
				dlSize += f.Size
			} else {
				if nstat, err := os.Stat(path.Join("/opt/agi/files/input/s3source", fn)); err == nil {
					dlSize += nstat.Size()
				}
			}
		}
		for fn, f := range p.SftpFiles {
			if f.Error != "" {
				status.Ingest.Errors = append(status.Ingest.Errors, "Downloader::"+fn+"::1::"+f.Error)
			}
			totalSize += f.Size
			if f.IsDownloaded {
				dlSize += f.Size
			} else {
				if nstat, err := os.Stat(path.Join("/opt/agi/files/input/sftpsource", fn)); err == nil {
					dlSize += nstat.Size()
				}
			}
		}
		status.Ingest.DownloaderTotalSize = totalSize
		status.Ingest.DownloaderCompleteSize = dlSize
		if totalSize > 0 {
			status.Ingest.DownloaderCompletePct = int((100 * dlSize) / totalSize)
		}

		return nil
	}()
	if err != nil {
		return nil, err
	}
	err = func() error {
		if steps.Download && !steps.Unpack {
			status.Ingest.DownloaderCompletePct = 100
		}
		reader, err := agiGetReader(ingestProgressPath, "unpacker.json")
		if err != nil {
			return err
		}
		defer reader.Close()
		p := new(ingest.ProgressUnpacker)
		json.NewDecoder(reader).Decode(p)
		for fn, f := range p.Files {
			for _, nerr := range f.Errors {
				status.Ingest.Errors = append(status.Ingest.Errors, "Unpaker::"+fn+"::1::"+nerr)
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	err = func() error {
		if steps.Unpack && !steps.PreProcess {
			status.Ingest.DownloaderCompletePct = 100
		}
		reader, err := agiGetReader(ingestProgressPath, "pre-processor.json")
		if err != nil {
			return err
		}
		defer reader.Close()
		p := new(ingest.ProgressPreProcessor)
		json.NewDecoder(reader).Decode(p)
		for fn, f := range p.Files {
			for _, nerr := range f.Errors {
				status.Ingest.Errors = append(status.Ingest.Errors, "Pre-Processor::"+fn+"::1::"+nerr)
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	err = func() error {
		if steps.PreProcess {
			status.Ingest.DownloaderCompletePct = 100
		}
		reader, err := agiGetReader(ingestProgressPath, "log-processor.json")
		if err != nil {
			return err
		}
		defer reader.Close()
		p := new(ingest.ProgressLogProcessor)
		json.NewDecoder(reader).Decode(p)
		totalSize := int64(0)
		dlSize := int64(0)
		for _, f := range p.Files {
			totalSize += f.Size
			if f.Finished {
				dlSize += f.Size
			} else {
				dlSize += f.Processed
			}
		}
		status.Ingest.LogProcessorTotalSize = totalSize
		status.Ingest.LogProcessorCompleteSize = dlSize
		if totalSize > 0 {
			status.Ingest.LogProcessorCompletePct = int((100 * dlSize) / totalSize)
		}
		if p.LineErrors != nil {
			for fn := range p.Files {
				nodePrefix, err := strconv.Atoi(p.Files[fn].NodePrefix)
				if err == nil {
					errs := p.LineErrors.Get(nodePrefix)
					for nerr, nerrc := range errs {
						status.Ingest.Errors = append(status.Ingest.Errors, "Log Processor::"+fn+"::"+strconv.Itoa(nerrc)+"::"+nerr)
					}
				}
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	err = func() error {
		if steps.Unpack && !steps.PreProcess {
			status.Ingest.DownloaderCompletePct = 100
		}
		reader, err := agiGetReader(ingestProgressPath, "cf-processor.json")
		if err != nil {
			return err
		}
		defer reader.Close()
		p := new(ingest.ProgressCollectProcessor)
		json.NewDecoder(reader).Decode(p)
		for fn, f := range p.Files {
			for _, nerr := range f.Errors {
				status.Ingest.Errors = append(status.Ingest.Errors, "CollectInfo Processor::"+fn+"::1::"+nerr)
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	status.Ingest.ErrorCount = len(status.Ingest.Errors)
	return status, nil
}

type agiReadCloser struct {
	closers []io.ReadCloser
	reader  io.Reader
}

func (a *agiReadCloser) Read(p []byte) (int, error) {
	return a.reader.Read(p)
}

func (a *agiReadCloser) Close() error {
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i].Close()
	}
	return nil
}

func agiGetReader(ingestProgressPath string, fname string) (r io.ReadCloser, err error) {
	ret := &agiReadCloser{}
	npath := path.Join(ingestProgressPath, fname)
	gz := false
	isEmptyResponse := false
	if _, err := os.Stat(npath); err != nil {
		npath = npath + ".gz"
		if _, err := os.Stat(npath); err != nil {
			isEmptyResponse = true
		}
		gz = true
	}
	if !isEmptyResponse {
		fa, err := os.Open(npath)
		if err != nil {
			return nil, fmt.Errorf("file open error: %s: %s", npath, err)
		}
		ret.closers = []io.ReadCloser{fa}
		ret.reader = fa
		if gz {
			fx, err := gzip.NewReader(fa)
			if err != nil {
				fa.Close()
				return nil, fmt.Errorf("could not open gz for reading: %s: %s", npath, err)
			}
			ret.closers = append(ret.closers, fx)
			ret.reader = fx
		}
	} else {
		ret.reader = strings.NewReader("{}")
	}
	return ret, nil
}
