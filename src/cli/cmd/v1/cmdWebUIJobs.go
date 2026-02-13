package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lithammer/shortuuid"
)

// JobStatus represents the current state of a job
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusError     JobStatus = "error"
	JobStatusFailed    JobStatus = "failed"
)

// Job represents a submitted command job
type Job struct {
	ID               string                 `json:"id"`
	User             string                 `json:"user"`
	CommandPath      string                 `json:"commandPath"`
	Parameters       map[string]interface{} `json:"parameters"`
	CLICommand       string                 `json:"cliCommand"`
	Status           JobStatus              `json:"status"`
	CreatedAt        time.Time              `json:"createdAt"`
	StartedAt        *time.Time             `json:"startedAt,omitempty"`
	CompletedAt      *time.Time             `json:"completedAt,omitempty"`
	Error            string                 `json:"error,omitempty"`
	RefreshInventory bool                   `json:"refreshInventory,omitempty"`

	// Subprocess execution fields
	PID       int  `json:"pid,omitempty"`
	ExitCode  *int `json:"exitCode,omitempty"`
	Cancelled bool `json:"cancelled,omitempty"`
	TimedOut  bool `json:"timedOut,omitempty"`

	// ReloadRequired is set when config/backend completes - frontend should reload the page
	ReloadRequired bool `json:"reloadRequired,omitempty"`

	// TempDir is the temp directory for uploaded files (cleaned after completion)
	TempDir string `json:"tempDir,omitempty"`
}

// JobSubmitResponse is returned when a job is submitted
type JobSubmitResponse struct {
	JobID         string    `json:"jobId"`
	User          string    `json:"user"`
	CommandPath   string    `json:"commandPath"`
	CLICommand    string    `json:"cliCommand"`
	Status        JobStatus `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
	StatusURL     string    `json:"statusUrl"`
	LogsURL       string    `json:"logsUrl"`
	LogsStreamURL string    `json:"logsStreamUrl"`
}

// JobListResponse is returned when listing jobs
type JobListResponse struct {
	Jobs  []*Job `json:"jobs"`
	Count int    `json:"count"`
}

// JobManager handles job lifecycle and storage
type JobManager struct {
	basePath string
	jobs     map[string]*Job
	mu       sync.RWMutex
}

// NewJobManager creates a new JobManager
func NewJobManager() (*JobManager, error) {
	rootDir, err := AerolabRootDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get aerolab root dir: %w", err)
	}

	basePath := filepath.Join(rootDir, "restapi", "commands")
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create job storage directory: %w", err)
	}

	jm := &JobManager{
		basePath: basePath,
		jobs:     make(map[string]*Job),
	}

	// Load existing jobs from disk
	if err := jm.loadExistingJobs(); err != nil {
		// Log warning but don't fail - we can start fresh
		fmt.Printf("Warning: failed to load existing jobs: %s\n", err)
	}

	return jm, nil
}

// GenerateJobID creates a new job ID using shortuuid and timestamp
func GenerateJobID() string {
	return fmt.Sprintf("%s-%d", shortuuid.New(), time.Now().Unix())
}

// CreateJob creates a new job and saves it to disk.
// If cliCommand is non-empty it is used as the display command; otherwise one
// is generated from the params map (which includes all values, even defaults).
func (jm *JobManager) CreateJob(user, commandPath string, params map[string]interface{}, refreshInventory bool, cliCommand string) (*Job, error) {
	jobID := GenerateJobID()

	if cliCommand == "" {
		cliCommand = generateCLICommand(commandPath, params)
	}

	job := &Job{
		ID:               jobID,
		User:             user,
		CommandPath:      commandPath,
		Parameters:       params,
		CLICommand:       cliCommand,
		Status:           JobStatusPending,
		CreatedAt:        time.Now(),
		RefreshInventory: refreshInventory,
	}

	jm.mu.Lock()
	jm.jobs[jobID] = job
	jm.mu.Unlock()

	if err := jm.SaveJob(job); err != nil {
		return nil, err
	}

	return job, nil
}

// GetJob retrieves a job by ID
func (jm *JobManager) GetJob(jobID string) (*Job, error) {
	jm.mu.RLock()
	job, exists := jm.jobs[jobID]
	jm.mu.RUnlock()

	if exists {
		return job, nil
	}

	// Try to load from disk
	job, err := jm.loadJobFromDisk(jobID)
	if err != nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	jm.mu.Lock()
	jm.jobs[jobID] = job
	jm.mu.Unlock()

	return job, nil
}

// GetJobForUser retrieves a job by ID, ensuring it belongs to the user (or user is admin)
func (jm *JobManager) GetJobForUser(jobID, user string, isAdmin bool) (*Job, error) {
	job, err := jm.GetJob(jobID)
	if err != nil {
		return nil, err
	}

	if !isAdmin && job.User != user {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	return job, nil
}

// SaveJob saves a job to disk atomically (write to temp file, then rename)
func (jm *JobManager) SaveJob(job *Job) error {
	jobDir := jm.getJobDir(job.User, job.ID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return fmt.Errorf("failed to create job directory: %w", err)
	}

	jsonPath := filepath.Join(jobDir, "command.json")
	tempPath := filepath.Join(jobDir, "command.json.tmp")

	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	// Write to temp file first
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp job file: %w", err)
	}

	// Atomically rename temp file to final path
	if err := os.Rename(tempPath, jsonPath); err != nil {
		// Clean up temp file on failure
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename job file: %w", err)
	}

	// Update in-memory cache
	jm.mu.Lock()
	jm.jobs[job.ID] = job
	jm.mu.Unlock()

	return nil
}

// UpdateJobStatus updates the job status and saves it
func (jm *JobManager) UpdateJobStatus(jobID string, status JobStatus, errorMsg string) error {
	job, err := jm.GetJob(jobID)
	if err != nil {
		return err
	}

	job.Status = status
	if errorMsg != "" {
		job.Error = errorMsg
	}

	now := time.Now()
	switch status {
	case JobStatusRunning:
		job.StartedAt = &now
	case JobStatusCompleted, JobStatusFailed, JobStatusError:
		job.CompletedAt = &now
	}

	return jm.SaveJob(job)
}

// UpdateJobStatusWithMeta updates the job status with additional metadata (PID, exit code, etc.)
func (jm *JobManager) UpdateJobStatusWithMeta(jobID string, status JobStatus, errorMsg string, meta *Job) error {
	job, err := jm.GetJob(jobID)
	if err != nil {
		return err
	}

	job.Status = status
	if errorMsg != "" {
		job.Error = errorMsg
	}

	// Copy metadata from the provided job
	if meta != nil {
		job.PID = meta.PID
		job.ExitCode = meta.ExitCode
		job.Cancelled = meta.Cancelled
		job.TimedOut = meta.TimedOut
		job.ReloadRequired = meta.ReloadRequired
	}

	now := time.Now()
	switch status {
	case JobStatusRunning:
		job.StartedAt = &now
	case JobStatusCompleted, JobStatusFailed, JobStatusError:
		job.CompletedAt = &now
	}

	return jm.SaveJob(job)
}

// ListJobs lists jobs for a user with optional status filter.
// showMaxHistory limits the number of completed/failed jobs returned (0 = no limit).
// Active (running/pending) jobs are always returned in full.
func (jm *JobManager) ListJobs(user string, statusFilter string, allUsers bool, showMaxHistory int) ([]*Job, error) {
	var jobs []*Job

	// Walk the base directory
	err := filepath.Walk(jm.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			return nil
		}

		if info.Name() != "command.json" {
			return nil
		}

		// Load job
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var job Job
		if err := json.Unmarshal(data, &job); err != nil {
			return nil
		}

		// Filter by user
		if !allUsers && job.User != user {
			return nil
		}

		// Filter by status
		if statusFilter != "" && string(job.Status) != statusFilter {
			return nil
		}

		jobs = append(jobs, &job)
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Separate active (running/pending) from completed (completed/failed/error)
	var active, completed []*Job
	for _, j := range jobs {
		switch j.Status {
		case JobStatusRunning, JobStatusPending:
			active = append(active, j)
		default:
			completed = append(completed, j)
		}
	}

	// Sort completed by CreatedAt descending (newest first)
	sort.Slice(completed, func(i, j int) bool {
		return completed[i].CreatedAt.After(completed[j].CreatedAt)
	})

	// Truncate completed if showMaxHistory is set
	if showMaxHistory > 0 && len(completed) > showMaxHistory {
		completed = completed[:showMaxHistory]
	}

	// Merge: active first (by CreatedAt desc), then truncated completed
	sort.Slice(active, func(i, j int) bool {
		return active[i].CreatedAt.After(active[j].CreatedAt)
	})
	jobs = append(active, completed...)

	return jobs, nil
}

// GetLogPath returns the path to the job's log file
func (jm *JobManager) GetLogPath(job *Job) string {
	return filepath.Join(jm.getJobDir(job.User, job.ID), "command.log")
}

// OpenLogFile opens/creates the log file for writing
func (jm *JobManager) OpenLogFile(job *Job) (*os.File, error) {
	jobDir := jm.getJobDir(job.User, job.ID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create job directory: %w", err)
	}

	logPath := filepath.Join(jobDir, "command.log")
	return os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
}

// ReadLogs reads the current log content
func (jm *JobManager) ReadLogs(job *Job) (string, error) {
	logPath := jm.GetLogPath(job)
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// ReadLogsFromOffset reads log content from a specific offset
func (jm *JobManager) ReadLogsFromOffset(job *Job, offset int64) (string, int64, error) {
	logPath := jm.GetLogPath(job)

	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", 0, nil
		}
		return "", offset, err
	}
	defer file.Close()

	// Seek to offset
	newOffset, err := file.Seek(offset, io.SeekStart)
	if err != nil {
		return "", offset, err
	}

	// Read remaining content
	data, err := io.ReadAll(file)
	if err != nil {
		return "", offset, err
	}

	return string(data), newOffset + int64(len(data)), nil
}

// getJobDir returns the directory path for a job
func (jm *JobManager) getJobDir(user, jobID string) string {
	return filepath.Join(jm.basePath, sanitizePathComponent(user), jobID)
}

// loadJobFromDisk loads a single job from disk by searching all user directories
func (jm *JobManager) loadJobFromDisk(jobID string) (*Job, error) {
	var foundJob *Job

	err := filepath.Walk(jm.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || foundJob != nil {
			return nil
		}

		if !info.IsDir() {
			return nil
		}

		// Check if this directory contains our job
		if info.Name() == jobID {
			jsonPath := filepath.Join(path, "command.json")
			data, err := os.ReadFile(jsonPath)
			if err != nil {
				return nil
			}

			var job Job
			if err := json.Unmarshal(data, &job); err != nil {
				return nil
			}

			foundJob = &job
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if foundJob == nil {
		return nil, fmt.Errorf("job not found")
	}

	return foundJob, nil
}

// loadExistingJobs loads all jobs from disk into memory cache
func (jm *JobManager) loadExistingJobs() error {
	return filepath.Walk(jm.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() || info.Name() != "command.json" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var job Job
		if err := json.Unmarshal(data, &job); err != nil {
			return nil
		}

		jm.mu.Lock()
		jm.jobs[job.ID] = &job
		jm.mu.Unlock()

		return nil
	})
}

// CleanupOldJobs removes completed/failed jobs older than maxAge
func (jm *JobManager) CleanupOldJobs(maxAge time.Duration) (int, error) {
	if maxAge == 0 {
		return 0, nil // Cleanup disabled
	}

	cutoff := time.Now().Add(-maxAge)
	count := 0

	err := filepath.Walk(jm.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}

		if info.IsDir() || info.Name() != "command.json" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var job Job
		if err := json.Unmarshal(data, &job); err != nil {
			return nil
		}

		// Skip running/pending jobs
		if job.Status == JobStatusPending || job.Status == JobStatusRunning {
			return nil
		}

		// Determine the time to check against
		var checkTime time.Time
		if job.CompletedAt != nil {
			checkTime = *job.CompletedAt
		} else if job.StartedAt != nil {
			checkTime = *job.StartedAt
		} else {
			checkTime = job.CreatedAt
		}

		// Delete if older than cutoff
		if checkTime.Before(cutoff) {
			jobDir := filepath.Dir(path)
			if err := os.RemoveAll(jobDir); err != nil {
				// Log but continue
				return nil
			}

			// Remove from memory cache
			jm.mu.Lock()
			delete(jm.jobs, job.ID)
			jm.mu.Unlock()

			count++
		}

		return nil
	})

	return count, err
}

// CountRunningJobs returns the number of jobs with status "running"
func (jm *JobManager) CountRunningJobs() int {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	count := 0
	for _, job := range jm.jobs {
		if job.Status == JobStatusRunning {
			count++
		}
	}
	return count
}

// KillAllRunningJobs sends SIGKILL to all running jobs that have a valid PID
func (jm *JobManager) KillAllRunningJobs() {
	jm.mu.RLock()
	jobs := make([]*Job, 0, len(jm.jobs))
	for _, job := range jm.jobs {
		if job.Status == JobStatusRunning && job.PID > 0 {
			jobs = append(jobs, job)
		}
	}
	jm.mu.RUnlock()

	for _, job := range jobs {
		proc, err := os.FindProcess(job.PID)
		if err != nil {
			continue
		}
		_ = proc.Signal(syscall.SIGKILL)
	}
}

// generateCLICommand builds the equivalent aerolab CLI command using map-based parameters.
// This is a simplified version that works with JSON parameters directly.
// For more accurate reconstruction with defaults handling, use generateCLICommandFromStruct.
func generateCLICommand(cmdPath string, params map[string]interface{}) string {
	parts := []string{"aerolab"}

	// Add command path
	parts = append(parts, strings.Split(cmdPath, "/")...)

	// Sort parameter keys for deterministic output order
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Add parameters as flags
	for _, key := range keys {
		value := params[key]
		switch v := value.(type) {
		case bool:
			if v {
				parts = append(parts, fmt.Sprintf("--%s", key))
			}
		case []interface{}:
			for _, elem := range v {
				parts = append(parts, fmt.Sprintf("--%s=%s", key, shellEscape(fmt.Sprintf("%v", elem))))
			}
		case []string:
			for _, elem := range v {
				parts = append(parts, fmt.Sprintf("--%s=%s", key, shellEscape(elem)))
			}
		case string:
			parts = append(parts, fmt.Sprintf("--%s=%s", key, shellEscape(v)))
		default:
			parts = append(parts, fmt.Sprintf("--%s=%s", key, shellEscape(fmt.Sprintf("%v", v))))
		}
	}

	return strings.Join(parts, " ")
}

// sanitizePathComponent sanitizes a string for use as a path component
func sanitizePathComponent(s string) string {
	// Replace or remove characters that are problematic in paths
	result := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r
		}
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, s)

	if result == "" {
		return "anonymous"
	}

	return strings.ToLower(result)
}

// LogWriter is an io.Writer that writes to both a file and captures for the job
type LogWriter struct {
	file *os.File
	mu   sync.Mutex
}

// NewLogWriter creates a new LogWriter
func NewLogWriter(file *os.File) *LogWriter {
	return &LogWriter{file: file}
}

// Write implements io.Writer
func (lw *LogWriter) Write(p []byte) (n int, err error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	if lw.file != nil {
		return lw.file.Write(p)
	}
	return len(p), nil
}

// Close closes the underlying file
func (lw *LogWriter) Close() error {
	if lw.file != nil {
		return lw.file.Close()
	}
	return nil
}
