// Package progress provides thread-safe progress tracking for file transfers
// with support for multi-node parallel operations and TUI display.
package progress

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// FileProgress tracks progress for a single file transfer
type FileProgress struct {
	NodeNo    int
	FileName  string
	Current   int64
	Total     int64
	StartTime time.Time
	Complete  bool
	Error     error
}

// Speed returns the transfer speed in bytes per second
func (fp *FileProgress) Speed() float64 {
	elapsed := time.Since(fp.StartTime).Seconds()
	if elapsed < 0.1 {
		return 0
	}
	return float64(atomic.LoadInt64(&fp.Current)) / elapsed
}

// Percent returns the completion percentage (0-100)
func (fp *FileProgress) Percent() float64 {
	if fp.Total == 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&fp.Current)) / float64(fp.Total) * 100
}

// Tracker manages progress for multiple concurrent transfers
type Tracker struct {
	mu         sync.RWMutex
	files      map[string]*FileProgress // key: "nodeNo:filename"
	totalBytes int64
	done       atomic.Bool
	cancelled  atomic.Bool
	errors     []error
}

// NewTracker creates a new progress tracker
func NewTracker() *Tracker {
	return &Tracker{
		files: make(map[string]*FileProgress),
	}
}

// SetTotalBytes sets the total bytes to be transferred (for aggregate progress)
func (t *Tracker) SetTotalBytes(total int64) {
	atomic.StoreInt64(&t.totalBytes, total)
}

// StartFile registers a new file transfer
func (t *Tracker) StartFile(nodeNo int, fileName string, size int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := fileKey(nodeNo, fileName)
	t.files[key] = &FileProgress{
		NodeNo:    nodeNo,
		FileName:  fileName,
		Total:     size,
		StartTime: time.Now(),
	}
}

// UpdateProgress updates the progress for a file
func (t *Tracker) UpdateProgress(nodeNo int, fileName string, current int64) {
	t.mu.RLock()
	key := fileKey(nodeNo, fileName)
	fp, ok := t.files[key]
	t.mu.RUnlock()

	if ok {
		atomic.StoreInt64(&fp.Current, current)
	}
}

// CompleteFile marks a file as complete
func (t *Tracker) CompleteFile(nodeNo int, fileName string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := fileKey(nodeNo, fileName)
	if fp, ok := t.files[key]; ok {
		fp.Complete = true
		fp.Error = err
		if err != nil {
			t.errors = append(t.errors, err)
		}
	}
}

// SetError records an error for a node
func (t *Tracker) SetError(nodeNo int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errors = append(t.errors, err)
}

// GetFileProgress returns a snapshot of all file progress
func (t *Tracker) GetFileProgress() []*FileProgress {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*FileProgress, 0, len(t.files))
	for _, fp := range t.files {
		// Create a copy to avoid race conditions
		copy := &FileProgress{
			NodeNo:    fp.NodeNo,
			FileName:  fp.FileName,
			Current:   atomic.LoadInt64(&fp.Current),
			Total:     fp.Total,
			StartTime: fp.StartTime,
			Complete:  fp.Complete,
			Error:     fp.Error,
		}
		result = append(result, copy)
	}
	return result
}

// GetTotalProgress returns total bytes and bytes done across all files
func (t *Tracker) GetTotalProgress() (total, done int64) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	total = atomic.LoadInt64(&t.totalBytes)
	for _, fp := range t.files {
		done += atomic.LoadInt64(&fp.Current)
	}
	return total, done
}

// IsComplete returns true if all transfers are complete
func (t *Tracker) IsComplete() bool {
	return t.done.Load()
}

// SetComplete marks the tracker as complete
func (t *Tracker) SetComplete() {
	t.done.Store(true)
}

// IsCancelled returns true if the operation was cancelled
func (t *Tracker) IsCancelled() bool {
	return t.cancelled.Load()
}

// Cancel marks the tracker as cancelled
func (t *Tracker) Cancel() {
	t.cancelled.Store(true)
}

// GetErrors returns all errors that occurred
func (t *Tracker) GetErrors() []error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return append([]error{}, t.errors...)
}

// HasErrors returns true if any errors occurred
func (t *Tracker) HasErrors() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.errors) > 0
}

func fileKey(nodeNo int, fileName string) string {
	return string(rune(nodeNo)) + ":" + fileName
}

// ProgressReader wraps an io.Reader to track bytes read
type ProgressReader struct {
	reader   io.Reader
	tracker  *Tracker
	nodeNo   int
	fileName string
	current  int64
}

// NewProgressReader creates a new progress-tracking reader
func (t *Tracker) NewProgressReader(r io.Reader, nodeNo int, fileName string, total int64) *ProgressReader {
	t.StartFile(nodeNo, fileName, total)
	return &ProgressReader{
		reader:   r,
		tracker:  t,
		nodeNo:   nodeNo,
		fileName: fileName,
	}
}

// Read implements io.Reader
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		current := atomic.AddInt64(&pr.current, int64(n))
		pr.tracker.UpdateProgress(pr.nodeNo, pr.fileName, current)
	}
	return n, err
}

// Complete marks the file as complete
func (pr *ProgressReader) Complete(err error) {
	pr.tracker.CompleteFile(pr.nodeNo, pr.fileName, err)
}

// ProgressWriter wraps an io.Writer to track bytes written
type ProgressWriter struct {
	writer   io.Writer
	tracker  *Tracker
	nodeNo   int
	fileName string
	current  int64
}

// NewProgressWriter creates a new progress-tracking writer
func (t *Tracker) NewProgressWriter(w io.Writer, nodeNo int, fileName string, total int64) *ProgressWriter {
	t.StartFile(nodeNo, fileName, total)
	return &ProgressWriter{
		writer:   w,
		tracker:  t,
		nodeNo:   nodeNo,
		fileName: fileName,
	}
}

// Write implements io.Writer
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if n > 0 {
		current := atomic.AddInt64(&pw.current, int64(n))
		pw.tracker.UpdateProgress(pw.nodeNo, pw.fileName, current)
	}
	return n, err
}

// Complete marks the file as complete
func (pw *ProgressWriter) Complete(err error) {
	pw.tracker.CompleteFile(pw.nodeNo, pw.fileName, err)
}

// CalculateSize calculates the total size of a file or directory
func CalculateSize(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}
