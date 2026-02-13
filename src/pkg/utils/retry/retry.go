package retry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rglonek/logger"
)

// Config holds configuration for retry operations
type Config struct {
	// MaxRetries is the maximum number of retries (0 means no retries, just one attempt)
	MaxRetries int
	// RetrySleep is the duration to sleep between retries
	RetrySleep time.Duration
	// Logger for retry attempt logging (optional)
	Logger *logger.Logger
	// OnRetry is called before each retry attempt (optional)
	OnRetry func(attempt int, err error)
	// ShouldRetry determines if the error should trigger a retry (optional, defaults to always retry on error)
	ShouldRetry func(err error) bool
	// Context for cancellation (optional)
	Context context.Context
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		MaxRetries: 3,
		RetrySleep: 5 * time.Second,
	}
}

// WithRetry executes a function with retries, returning the result and any error
func WithRetry[T any](cfg *Config, fn func() (T, error)) (T, error) {
	var zero T
	if cfg == nil {
		cfg = DefaultConfig()
	}

	ctx := cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if we should retry
		if cfg.ShouldRetry != nil && !cfg.ShouldRetry(err) {
			return zero, err
		}

		// Don't sleep or log on the last attempt
		if attempt < cfg.MaxRetries {
			if cfg.OnRetry != nil {
				cfg.OnRetry(attempt+1, err)
			}
			if cfg.Logger != nil {
				cfg.Logger.Detail("Retry attempt %d/%d after error: %v", attempt+1, cfg.MaxRetries, err)
			}

			// Sleep with context awareness
			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(cfg.RetrySleep):
			}
		}
	}

	return zero, fmt.Errorf("failed after %d attempts: %w", cfg.MaxRetries+1, lastErr)
}

// WithRetryNoResult executes a function with retries that doesn't return a value
func WithRetryNoResult(cfg *Config, fn func() error) error {
	_, err := WithRetry(cfg, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// WithRetrySimple is a simpler version that just takes max retries and sleep duration
func WithRetrySimple[T any](maxRetries int, retrySleep time.Duration, fn func() (T, error)) (T, error) {
	return WithRetry(&Config{
		MaxRetries: maxRetries,
		RetrySleep: retrySleep,
	}, fn)
}

// WithRetrySimpleNoResult is a simpler version for functions that don't return a value
func WithRetrySimpleNoResult(maxRetries int, retrySleep time.Duration, fn func() error) error {
	return WithRetryNoResult(&Config{
		MaxRetries: maxRetries,
		RetrySleep: retrySleep,
	}, fn)
}

// RetryableFunc represents a function that can be retried
type RetryableFunc[T any] func() (T, error)

// RetryableFuncNoResult represents a function without return value that can be retried
type RetryableFuncNoResult func() error

// Executor manages retry execution with configured settings
type Executor struct {
	config *Config
}

// NewExecutor creates a new retry executor with the given configuration
func NewExecutor(cfg *Config) *Executor {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Executor{config: cfg}
}

// Execute runs a function with retries using the executor's configuration
func (e *Executor) Execute(fn func() error) error {
	return WithRetryNoResult(e.config, fn)
}

// ExecuteWithResult runs a function with retries that returns a value
func ExecuteWithResult[T any](e *Executor, fn func() (T, error)) (T, error) {
	return WithRetry(e.config, fn)
}

// CombineErrors combines multiple errors into one
func CombineErrors(errs ...error) error {
	var combined error
	for _, err := range errs {
		if err != nil {
			combined = errors.Join(combined, err)
		}
	}
	return combined
}

// RetryState tracks the state of retries for coordination between different retry levels
type RetryState struct {
	// CapacityRetriesUsed tracks how many capacity retries have been used
	CapacityRetriesUsed int
	// TransientRetriesUsed tracks how many transient retries have been used
	TransientRetriesUsed int
	// LastError holds the last error encountered
	LastError error
}

// NewRetryState creates a new retry state tracker
func NewRetryState() *RetryState {
	return &RetryState{}
}

// ShouldRetryCapacity returns true if more capacity retries are available
func (s *RetryState) ShouldRetryCapacity(maxCapacityRetries int) bool {
	return s.CapacityRetriesUsed < maxCapacityRetries
}

// ShouldRetryTransient returns true if more transient retries are available
func (s *RetryState) ShouldRetryTransient(maxTransientRetries int) bool {
	return s.TransientRetriesUsed < maxTransientRetries
}

// IncrementCapacityRetry increments the capacity retry counter
func (s *RetryState) IncrementCapacityRetry() {
	s.CapacityRetriesUsed++
}

// IncrementTransientRetry increments the transient retry counter
func (s *RetryState) IncrementTransientRetry() {
	s.TransientRetriesUsed++
}

// Reset resets the retry state
func (s *RetryState) Reset() {
	s.CapacityRetriesUsed = 0
	s.TransientRetriesUsed = 0
	s.LastError = nil
}
