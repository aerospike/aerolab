package scriptlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lithammer/shortuuid"
)

const (
	// BaseDir is the base directory for storing script failure logs
	BaseDir = "/tmp/aerolab"
)

// ScriptFailure contains information about a failed script execution
type ScriptFailure struct {
	// ID is the unique identifier for this failure (shortuuid)
	ID string `json:"id"`
	// Timestamp is the unix timestamp of when the failure occurred
	Timestamp int64 `json:"timestamp"`
	// ClusterName is the name of the cluster where the failure occurred
	ClusterName string `json:"clusterName"`
	// NodeNo is the node number where the failure occurred
	NodeNo int `json:"nodeNo"`
	// InstanceID is the instance ID where the failure occurred (optional)
	InstanceID string `json:"instanceId,omitempty"`
	// Command is the command that was executed (os.Args or command info)
	Command []string `json:"command"`
	// ScriptPath is the path where the script was uploaded on the remote machine
	ScriptPath string `json:"scriptPath"`
	// Script is the script content that failed
	Script []byte `json:"-"`
	// Stdout is the standard output from the failed script
	Stdout []byte `json:"-"`
	// Stderr is the standard error from the failed script
	Stderr []byte `json:"-"`
	// ErrorMessage is the error message from the failure
	ErrorMessage string `json:"errorMessage"`
}

// Detail contains the JSON-serializable detail information
type Detail struct {
	ID           string   `json:"id"`
	Timestamp    int64    `json:"timestamp"`
	ClusterName  string   `json:"clusterName"`
	NodeNo       int      `json:"nodeNo"`
	InstanceID   string   `json:"instanceId,omitempty"`
	Command      []string `json:"command"`
	ScriptPath   string   `json:"scriptPath"`
	ErrorMessage string   `json:"errorMessage"`
}

// SaveFailure saves the script failure information to the local filesystem
// Returns the path where the failure was saved
func SaveFailure(f *ScriptFailure) (string, error) {
	if f.ID == "" {
		f.ID = shortuuid.New()
	}
	if f.Timestamp == 0 {
		f.Timestamp = time.Now().Unix()
	}

	// Create the directory path: /tmp/aerolab/{shortuuid}-{unixTimestamp}/
	dirName := fmt.Sprintf("%s-%d", f.ID, f.Timestamp)
	dirPath := filepath.Join(BaseDir, dirName)

	// Create the directory
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dirPath, err)
	}

	// Save the script
	scriptPath := filepath.Join(dirPath, "script")
	if len(f.Script) > 0 {
		if err := os.WriteFile(scriptPath, f.Script, 0644); err != nil {
			return "", fmt.Errorf("failed to write script file: %w", err)
		}
	}

	// Save the combined log (stdout + stderr)
	logPath := filepath.Join(dirPath, "log")
	var logContent []byte
	if len(f.Stdout) > 0 || len(f.Stderr) > 0 {
		logContent = append(logContent, []byte("=== STDOUT ===\n")...)
		logContent = append(logContent, f.Stdout...)
		logContent = append(logContent, []byte("\n=== STDERR ===\n")...)
		logContent = append(logContent, f.Stderr...)
	} else {
		logContent = []byte("(no output captured - the command may have failed before producing any output)\n")
	}
	if err := os.WriteFile(logPath, logContent, 0644); err != nil {
		return "", fmt.Errorf("failed to write log file: %w", err)
	}

	// Save the detail JSON
	detailPath := filepath.Join(dirPath, "detail")
	detail := Detail{
		ID:           f.ID,
		Timestamp:    f.Timestamp,
		ClusterName:  f.ClusterName,
		NodeNo:       f.NodeNo,
		InstanceID:   f.InstanceID,
		Command:      f.Command,
		ScriptPath:   f.ScriptPath,
		ErrorMessage: f.ErrorMessage,
	}
	detailJSON, err := json.MarshalIndent(detail, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal detail: %w", err)
	}
	if err := os.WriteFile(detailPath, detailJSON, 0644); err != nil {
		return "", fmt.Errorf("failed to write detail file: %w", err)
	}

	return dirPath, nil
}

// FormatError returns a user-friendly error message including the path to saved logs
func FormatError(path string, clusterName string, nodeNo int, originalErr error) string {
	return fmt.Sprintf(
		"script execution failed on %s node %d: %v\nScript output saved to: %s\n  - script: %s/script\n  - log: %s/log\n  - detail: %s/detail",
		clusterName, nodeNo, originalErr, path, path, path, path,
	)
}

// FormatErrorSimple returns a simpler error message with just the path
func FormatErrorSimple(path string, originalErr error) string {
	return fmt.Sprintf("script execution failed: %v\nOutput saved to: %s", originalErr, path)
}

// SaveAndFormatError is a convenience function that saves the failure and returns a formatted error
func SaveAndFormatError(f *ScriptFailure) (string, error) {
	path, err := SaveFailure(f)
	if err != nil {
		// If we can't save, return the original error with note about save failure
		return "", fmt.Errorf("script failed on %s node %d: %s (additionally, failed to save logs: %v)",
			f.ClusterName, f.NodeNo, f.ErrorMessage, err)
	}
	return path, fmt.Errorf("%s", FormatError(path, f.ClusterName, f.NodeNo, fmt.Errorf("%s", f.ErrorMessage)))
}

// NewScriptFailure creates a new ScriptFailure with basic fields populated
func NewScriptFailure(clusterName string, nodeNo int, script []byte, stdout []byte, stderr []byte, err error) *ScriptFailure {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	return &ScriptFailure{
		ID:           shortuuid.New(),
		Timestamp:    time.Now().Unix(),
		ClusterName:  clusterName,
		NodeNo:       nodeNo,
		Script:       script,
		Stdout:       stdout,
		Stderr:       stderr,
		ErrorMessage: errMsg,
		Command:      os.Args,
	}
}

// NewScriptFailureWithPath creates a new ScriptFailure with script path information
func NewScriptFailureWithPath(clusterName string, nodeNo int, scriptPath string, script []byte, stdout []byte, stderr []byte, err error) *ScriptFailure {
	f := NewScriptFailure(clusterName, nodeNo, script, stdout, stderr, err)
	f.ScriptPath = scriptPath
	return f
}

// CleanupOldFailures removes failure directories older than the specified duration
func CleanupOldFailures(maxAge time.Duration) error {
	entries, err := os.ReadDir(BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read directory %s: %w", BaseDir, err)
	}

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(BaseDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				// Log but don't fail on cleanup errors
				continue
			}
		}
	}

	return nil
}
