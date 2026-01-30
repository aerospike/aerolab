package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

// templateCreationRaceHandler handles race conditions when multiple users try to create
// the same template simultaneously.
//
// For Docker: Simply vacuum any existing dangling instances (always our own work).
// For AWS/GCP: Use heartbeat + session ID mechanism to distinguish between:
//   - Our own abandoned work (vacuum and proceed)
//   - Someone else's dead work (vacuum and proceed)
//   - Someone else's active work (wait for them to finish)
type templateCreationRaceHandler struct {
	// Configuration
	backendType        string
	templateVersionTag string // e.g., "agi-amd64-5" or "8.0.0-enterprise"
	sessionFile        string // local file storing our session ID

	// Tags used for coordination
	sessionTag   string // tag name for session ID
	heartbeatTag string // tag name for heartbeat timestamp

	// Timeouts
	heartbeatStaleTimeout time.Duration // how long before a heartbeat is considered stale
	waitTimeout           time.Duration // max time to wait for another process
	waitPollInterval      time.Duration // how often to check while waiting

	// Logger
	logger *logger.Logger
}

// templateCreationRaceResult is the result of checking for race conditions.
type templateCreationRaceResult struct {
	ShouldVacuum   bool                  // should we vacuum existing instances
	ShouldWait     bool                  // should we wait for another process
	ShouldProceed  bool                  // should we proceed with creation
	Instances      backends.InstanceList // instances to vacuum or wait for
	TemplateExists bool                  // template was created while we were waiting
	TemplateName   string                // name of the existing template (if any)
}

// newTemplateCreationRaceHandler creates a new race handler.
//
// Parameters:
//   - backendType: "docker", "aws", or "gcp"
//   - templateVersionTag: unique identifier for this template version (used in tags)
//   - logger: logger for output
func newTemplateCreationRaceHandler(backendType, templateVersionTag string, logger *logger.Logger) *templateCreationRaceHandler {
	// Create a unique session file path based on template version
	homeDir, _ := os.UserHomeDir()
	sessionDir := filepath.Join(homeDir, ".aerolab", "template-sessions")
	os.MkdirAll(sessionDir, 0700)

	return &templateCreationRaceHandler{
		backendType:           backendType,
		templateVersionTag:    templateVersionTag,
		sessionFile:           filepath.Join(sessionDir, templateVersionTag+".session"),
		sessionTag:            "aerolab.tmpl.session",
		heartbeatTag:          "aerolab.tmpl.heartbeat",
		heartbeatStaleTimeout: 5 * time.Minute,
		waitTimeout:           30 * time.Minute,
		waitPollInterval:      30 * time.Second,
		logger:                logger,
	}
}

// getOrCreateSessionID returns the session ID for this process, creating one if needed.
func (h *templateCreationRaceHandler) getOrCreateSessionID() (string, error) {
	// Try to read existing session ID
	if data, err := os.ReadFile(h.sessionFile); err == nil && len(data) > 0 {
		return string(data), nil
	}

	// Generate new session ID
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	sessionID := hex.EncodeToString(b)

	// Write to file
	if err := os.WriteFile(h.sessionFile, []byte(sessionID), 0600); err != nil {
		return "", fmt.Errorf("failed to write session file: %w", err)
	}

	return sessionID, nil
}

// clearSessionID removes the session ID file after successful completion.
func (h *templateCreationRaceHandler) clearSessionID() {
	os.Remove(h.sessionFile)
}

// CheckForRaceCondition checks if there are existing template creation instances
// and determines what action to take.
//
// Parameters:
//   - instances: existing dangling template creation instances
//   - checkTemplateExists: function to check if template now exists (returns name, exists)
//
// Returns:
//   - templateCreationRaceResult with the recommended action
func (h *templateCreationRaceHandler) CheckForRaceCondition(
	instances backends.InstanceList,
	checkTemplateExists func() (string, bool),
) (*templateCreationRaceResult, error) {
	result := &templateCreationRaceResult{}

	// No existing instances - proceed normally
	if instances.Count() == 0 {
		result.ShouldProceed = true
		return result, nil
	}

	result.Instances = instances

	// For Docker: always vacuum (it's always our own work)
	if h.backendType == "docker" {
		h.logger.Info("Docker backend: vacuuming existing template creation instance (local work)")
		result.ShouldVacuum = true
		result.ShouldProceed = true
		return result, nil
	}

	// For AWS/GCP: check session and heartbeat
	sessionID, err := h.getOrCreateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to get session ID: %w", err)
	}

	inst := instances.Describe()[0]
	remoteSession := inst.Tags[h.sessionTag]
	remoteHeartbeatStr := inst.Tags[h.heartbeatTag]

	// Check if it's our own abandoned work
	if remoteSession == sessionID {
		h.logger.Info("Found our own abandoned template creation instance, vacuuming")
		result.ShouldVacuum = true
		result.ShouldProceed = true
		return result, nil
	}

	// Check if the remote heartbeat is stale
	if remoteHeartbeatStr != "" {
		remoteHeartbeat, err := strconv.ParseInt(remoteHeartbeatStr, 10, 64)
		if err == nil {
			heartbeatAge := time.Since(time.Unix(remoteHeartbeat, 0))
			if heartbeatAge > h.heartbeatStaleTimeout {
				h.logger.Info("Found stale template creation instance (no heartbeat for %v), vacuuming", heartbeatAge)
				result.ShouldVacuum = true
				result.ShouldProceed = true
				return result, nil
			}
			h.logger.Info("Found active template creation by another process (heartbeat %v ago)", heartbeatAge)
		}
	} else {
		// No heartbeat tag - could be old instance, check instance age
		instanceAge := time.Since(inst.CreationTime)
		if instanceAge > h.heartbeatStaleTimeout {
			h.logger.Info("Found old template creation instance without heartbeat (age %v), vacuuming", instanceAge)
			result.ShouldVacuum = true
			result.ShouldProceed = true
			return result, nil
		}
	}

	// Active work by another process - wait for them
	result.ShouldWait = true
	h.logger.Info("Template creation in progress by another process, waiting (up to %v)...", h.waitTimeout)

	startTime := time.Now()
	for time.Since(startTime) < h.waitTimeout {
		time.Sleep(h.waitPollInterval)

		// Check if template now exists
		if name, exists := checkTemplateExists(); exists {
			h.logger.Info("Template created by another process: %s", name)
			result.ShouldWait = false
			result.ShouldProceed = false
			result.TemplateExists = true
			result.TemplateName = name
			return result, nil
		}

		// Check if the other process is still running
		// Note: caller should refresh inventory before this check
		h.logger.Debug("Still waiting for template creation by another process...")
	}

	// Timeout - the other process may have failed, vacuum and proceed
	h.logger.Warn("Timeout waiting for template creation by another process, vacuuming and proceeding")
	result.ShouldWait = false
	result.ShouldVacuum = true
	result.ShouldProceed = true
	return result, nil
}

// GetInstanceTags returns the tags to add to a template creation instance.
func (h *templateCreationRaceHandler) GetInstanceTags() (map[string]string, error) {
	// For Docker, no special tags needed
	if h.backendType == "docker" {
		return nil, nil
	}

	sessionID, err := h.getOrCreateSessionID()
	if err != nil {
		return nil, err
	}

	return map[string]string{
		h.sessionTag:   sessionID,
		h.heartbeatTag: fmt.Sprintf("%d", time.Now().Unix()),
	}, nil
}

// StartHeartbeat starts a goroutine that periodically updates the heartbeat tag.
// Returns a stop function that should be called when template creation is complete.
//
// For Docker, this is a no-op and returns a no-op stop function.
func (h *templateCreationRaceHandler) StartHeartbeat(instances backends.InstanceList) (stop func()) {
	// For Docker, no heartbeat needed
	if h.backendType == "docker" {
		return func() {}
	}

	stopChan := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				err := instances.AddTags(map[string]string{
					h.heartbeatTag: fmt.Sprintf("%d", time.Now().Unix()),
				})
				if err != nil {
					h.logger.Debug("Failed to update heartbeat tag: %s", err)
				}
			}
		}
	}()

	return func() {
		close(stopChan)
		<-stopped
	}
}

// OnSuccess should be called after successful template creation.
// It clears the session file.
func (h *templateCreationRaceHandler) OnSuccess() {
	h.clearSessionID()
}

// CleanupDuplicateTemplates removes duplicate templates, keeping only the oldest one.
// This handles the race condition where two users simultaneously create the same template.
//
// Parameters:
//   - images: list of images that match the template criteria
//   - logger: logger for output
//
// Returns:
//   - *backends.Image: the template to use (oldest one)
//   - error: nil on success, or an error
func CleanupDuplicateTemplates(images backends.ImageList, logger *logger.Logger) (*backends.Image, error) {
	if images.Count() == 0 {
		return nil, fmt.Errorf("no templates found")
	}

	if images.Count() == 1 {
		return images.Describe()[0], nil
	}

	// Find the oldest template
	imageList := images.Describe()
	oldest := imageList[0]
	for _, img := range imageList[1:] {
		if img.CreationTime.Before(oldest.CreationTime) {
			oldest = img
		}
	}

	// Delete all others
	for _, img := range imageList {
		if img.ImageId == oldest.ImageId {
			continue
		}
		logger.Info("Cleaning up duplicate template: %s (created %s)", img.Name, img.CreationTime.Format(time.RFC3339))
	}

	// Create list of duplicates to delete
	var duplicates []*backends.Image
	for _, img := range imageList {
		if img.ImageId != oldest.ImageId {
			duplicates = append(duplicates, img)
		}
	}

	if len(duplicates) > 0 {
		// Delete the duplicates
		err := backends.ImageList(duplicates).DeleteImages(5 * time.Minute)
		if err != nil {
			logger.Warn("Failed to delete some duplicate templates: %s", err)
			// Don't return error - we can still use the oldest template
		}
	}

	logger.Info("Using template: %s (created %s)", oldest.Name, oldest.CreationTime.Format(time.RFC3339))
	return oldest, nil
}

