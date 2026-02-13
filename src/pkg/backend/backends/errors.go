package backends

import (
	"errors"
	"strings"
	"time"
)

// CapacityError represents a cloud provider capacity-related error
type CapacityError struct {
	Provider    BackendType
	ErrorCode   string
	Message     string
	OriginalErr error
}

func (e *CapacityError) Error() string {
	return e.Message
}

func (e *CapacityError) Unwrap() error {
	return e.OriginalErr
}

// TransientError represents a temporary/transient error that may succeed on retry
type TransientError struct {
	Operation   string
	Message     string
	OriginalErr error
}

func (e *TransientError) Error() string {
	return e.Message
}

func (e *TransientError) Unwrap() error {
	return e.OriginalErr
}

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	// MaxRetries is the maximum number of retries for transient failures (default: 0, no retries)
	MaxRetries int
	// RetrySleep is the sleep duration between retries (default: 30s)
	RetrySleep time.Duration
	// CapacityRetries is the maximum number of retries specifically for capacity errors (default: 0, no retries)
	CapacityRetries int
	// CapacityRetrySleep is the sleep duration between capacity retries (default: 60s)
	CapacityRetrySleep time.Duration
}

// DefaultRetryConfig returns a RetryConfig with default values
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:         0,
		RetrySleep:         30 * time.Second,
		CapacityRetries:    0,
		CapacityRetrySleep: 60 * time.Second,
	}
}

// AWS capacity-related error codes
var awsCapacityErrorCodes = []string{
	"InsufficientInstanceCapacity",
	"InsufficientCapacity",
	"InsufficientHostCapacity",
	"InsufficientReservedInstanceCapacity",
	"InsufficientAddressCapacity",
	"SpotMaxPriceTooLow",
	"MaxSpotInstanceCountExceeded",
	"InstanceLimitExceeded",
	"VcpuLimitExceeded",
}

// AWS capacity-related error messages (partial matches)
var awsCapacityErrorMessages = []string{
	"insufficient capacity",
	"capacity not available",
	"exceeded your spot instance limit",
	"vcpu limit",
	"instance limit",
	"not have sufficient capacity",
}

// GCP capacity-related error codes/messages
var gcpCapacityErrorCodes = []string{
	"ZONE_RESOURCE_POOL_EXHAUSTED",
	"ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS",
	"QUOTA_EXCEEDED",
	"RESOURCE_NOT_AVAILABLE",
	"RESOURCE_POOL_EXHAUSTED",
	"STOCKOUT",
}

var gcpCapacityErrorMessages = []string{
	"resource pool exhausted",
	"quota exceeded",
	"does not have enough resources available",
	"insufficient resources",
	"stockout",
}

// AWS transient error codes that may succeed on retry
var awsTransientErrorCodes = []string{
	"RequestLimitExceeded",
	"Throttling",
	"ThrottlingException",
	"ProvisionedThroughputExceededException",
	"ServiceUnavailable",
	"InternalError",
	"RequestTimeout",
}

// GCP transient error codes that may succeed on retry
var gcpTransientErrorCodes = []string{
	"RESOURCE_NOT_READY",
	"SERVICE_UNAVAILABLE",
	"INTERNAL_ERROR",
	"DEADLINE_EXCEEDED",
}

// SSH/SFTP transient error messages
var sshTransientErrorMessages = []string{
	"connection refused",
	"connection reset",
	"connection timed out",
	"i/o timeout",
	"no route to host",
	"network is unreachable",
	"broken pipe",
	"handshake failed",
	"ssh: handshake failed",
	"failed to dial",
}

// IsCapacityError checks if the error is a capacity-related error for any cloud provider
func IsCapacityError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's already a CapacityError
	var capErr *CapacityError
	if errors.As(err, &capErr) {
		return true
	}

	errStr := strings.ToLower(err.Error())

	// Check AWS capacity error codes and messages
	for _, code := range awsCapacityErrorCodes {
		if strings.Contains(errStr, strings.ToLower(code)) {
			return true
		}
	}
	for _, msg := range awsCapacityErrorMessages {
		if strings.Contains(errStr, msg) {
			return true
		}
	}

	// Check GCP capacity error codes and messages
	for _, code := range gcpCapacityErrorCodes {
		if strings.Contains(errStr, strings.ToLower(code)) {
			return true
		}
	}
	for _, msg := range gcpCapacityErrorMessages {
		if strings.Contains(errStr, msg) {
			return true
		}
	}

	return false
}

// IsTransientError checks if the error is a transient error that may succeed on retry
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's already a TransientError
	var transErr *TransientError
	if errors.As(err, &transErr) {
		return true
	}

	errStr := strings.ToLower(err.Error())

	// Check AWS transient error codes
	for _, code := range awsTransientErrorCodes {
		if strings.Contains(errStr, strings.ToLower(code)) {
			return true
		}
	}

	// Check GCP transient error codes
	for _, code := range gcpTransientErrorCodes {
		if strings.Contains(errStr, strings.ToLower(code)) {
			return true
		}
	}

	// Check SSH/SFTP transient error messages
	for _, msg := range sshTransientErrorMessages {
		if strings.Contains(errStr, msg) {
			return true
		}
	}

	return false
}

// IsRetryableError checks if the error is either a capacity error or a transient error
func IsRetryableError(err error) bool {
	return IsCapacityError(err) || IsTransientError(err)
}

// NewCapacityError creates a new CapacityError
func NewCapacityError(provider BackendType, errorCode, message string, originalErr error) *CapacityError {
	return &CapacityError{
		Provider:    provider,
		ErrorCode:   errorCode,
		Message:     message,
		OriginalErr: originalErr,
	}
}

// NewTransientError creates a new TransientError
func NewTransientError(operation, message string, originalErr error) *TransientError {
	return &TransientError{
		Operation:   operation,
		Message:     message,
		OriginalErr: originalErr,
	}
}

// WrapAsCapacityError wraps an error as a CapacityError if it matches capacity error patterns
func WrapAsCapacityError(provider BackendType, err error) error {
	if err == nil {
		return nil
	}
	if IsCapacityError(err) {
		return NewCapacityError(provider, "", err.Error(), err)
	}
	return err
}

// WrapAsTransientError wraps an error as a TransientError if it matches transient error patterns
func WrapAsTransientError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if IsTransientError(err) {
		return NewTransientError(operation, err.Error(), err)
	}
	return err
}
