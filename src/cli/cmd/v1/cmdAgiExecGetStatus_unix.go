//go:build !noagi && (darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package cmd

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/aerospike/aerolab/pkg/agi/ingest"
	"golang.org/x/sys/unix"
)

// pidAlive reports whether pidFile names a PID file containing a
// numeric PID for a process the kernel still knows about. Returns
// false on any error (missing file, garbage contents, dead pid). The
// caller treats the boolean as "the daemon associated with this pid
// file is up" and never inspects the underlying error — matching the
// pre-migration behaviour where every per-daemon pid check did the
// same os.ReadFile / Atoi / FindProcess sequence inline.
//
// Note: os.FindProcess on Unix is documented to always succeed, so
// "still knows about" is enforced by sending signal 0; on platforms
// where FindProcess is the only check available the helper degrades
// to "pid file exists and is a number".
func pidAlive(pidFile string) bool {
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 doesn't deliver but does the kernel-level liveness
	// check; an ESRCH/EPERM error means "no such process" or "not
	// ours, but exists". The latter still counts as alive.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// If errno is EPERM the process exists under a different
		// uid; treat that as alive. Anything else (ESRCH, etc.)
		// means the pid is stale.
		if errors.Is(err, syscall.EPERM) {
			return true
		}
		return false
	}
	return true
}

// GetAgiStatus retrieves comprehensive system and ingest status from an AGI instance.
// This function is platform-specific and uses Unix syscalls for system information.
//
// Parameters:
//   - enabled: If false, returns an empty status struct
//   - ingestProgressPath: Path to the directory containing ingest progress files
//
// Returns:
//   - *ingest.IngestStatusStruct: Status information including system resources, process status, and ingest progress
//   - error: nil on success, or an error describing what failed
func GetAgiStatus(enabled bool, ingestProgressPath string) (*ingest.IngestStatusStruct, error) {
	status := new(ingest.IngestStatusStruct)

	if !enabled {
		return status, nil
	}

	// Get disk stats via unix.Statfs
	var stat unix.Statfs_t
	err := unix.Statfs("/opt/agi", &stat)
	if err != nil {
		return nil, err
	}
	status.System.DiskFreeBytes = stat.Bavail * uint64(stat.Bsize)
	status.System.DiskTotalBytes = stat.Blocks * uint64(stat.Bsize)

	// Get memory stats from /proc/meminfo
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

	// IngestStatusStruct.AerospikeRunning is a legacy field name —
	// after the embedded-db migration there is no `asd` process to
	// look for, but the wire format is preserved for backward
	// compatibility with monitor listeners and the web UI. Its
	// post-migration meaning is "the storage backend is up": the
	// merged service (cmdAgiExecService) writes
	// /opt/agi/service.pid, and either of the legacy two-process
	// pids is also a valid signal that the embedded db is open
	// somewhere. Setting AerospikeRunning in any of those cases
	// keeps !AerospikeRunning's "stack is stopped" semantics
	// intact for consumers that haven't been updated.
	if pidAlive("/opt/agi/service.pid") || pidAlive("/opt/agi/ingest.pid") || pidAlive("/opt/agi/plugin.pid") {
		status.AerospikeRunning = true
	}

	if pidAlive("/opt/agi/ingest.pid") || pidAlive("/opt/agi/service.pid") {
		status.Ingest.Running = true
	}

	if pidAlive("/opt/agi/plugin.pid") || pidAlive("/opt/agi/service.pid") {
		status.PluginRunning = true
	}

	if pidAlive("/opt/agi/grafanafix.pid") {
		status.GrafanaHelperRunning = true
	}

	// Read ingest steps from /opt/agi/ingest/steps.json
	steps := new(ingest.IngestSteps)
	f, err := os.ReadFile("/opt/agi/ingest/steps.json")
	if err == nil {
		err = json.Unmarshal(f, steps)
		if err != nil {
			return nil, err
		}
	}
	status.Ingest.CompleteSteps = steps

	// Parse downloader progress
	err = func() error {
		reader, err := agiGetReader(ingestProgressPath, "downloader.json")
		if err != nil {
			return err
		}
		defer reader.Close()
		p := new(ingest.ProgressDownloader)
		//nolint:errcheck
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

	// Parse unpacker progress
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
		//nolint:errcheck
		json.NewDecoder(reader).Decode(p)
		for fn, f := range p.Files {
			for _, nerr := range f.Errors {
				status.Ingest.Errors = append(status.Ingest.Errors, "Unpacker::"+fn+"::1::"+nerr)
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}

	// Parse pre-processor progress
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
		//nolint:errcheck
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

	// Parse log-processor progress
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
		//nolint:errcheck
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

	// Parse collectinfo processor progress
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
		//nolint:errcheck
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

// agiReadCloser is a helper struct that manages multiple closers for nested readers
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

// agiGetReader opens a progress file with automatic gzip support.
// If the plain file doesn't exist, it tries with .gz extension.
// If neither exists, returns a reader with empty JSON object "{}".
//
// Parameters:
//   - ingestProgressPath: Base directory for progress files
//   - fname: Filename to open
//
// Returns:
//   - io.ReadCloser: Reader for the file contents
//   - error: nil on success, or an error describing what failed
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

// cut is a helper function for parsing /proc/meminfo style lines.
// It splits a string by separator and returns the field at the specified index.
//
// Parameters:
//   - s: String to split
//   - field: 1-based index of field to return
//   - sep: Separator string
//
// Returns:
//   - string: The field at the specified index, or empty string if not found
func cut(s string, field int, sep string) string {
	parts := []string{}
	for _, p := range splitMultiple(s, sep) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if field > len(parts) || field < 1 {
		return ""
	}
	return parts[field-1]
}

// splitMultiple splits a string by a separator, handling multiple consecutive separators
func splitMultiple(s string, sep string) []string {
	result := []string{}
	current := ""
	for _, c := range s {
		if string(c) == sep {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
