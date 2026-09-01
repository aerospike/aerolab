package cmd

import (
	"errors"

	"github.com/citrusleaf/aerolab/pkg/backend/backends"
)

// Exit codes aerolab terminates with. Commands that simply fail keep using
// ExitCodeError; the more specific codes exist so that automation can tell
// retryable infrastructure problems apart from real failures without parsing
// log output.
//
// Signal-driven shutdown does not go through here: pkg/utils/shutdown exits
// with the conventional 128+signal values (130 for SIGINT, 143 for SIGTERM).
const (
	// ExitCodeOK is returned when the command succeeded.
	ExitCodeOK = 0
	// ExitCodeError is the generic failure code.
	ExitCodeError = 1
	// ExitCodeCapacity is returned when AWS or GCP could not supply the
	// requested capacity (insufficient instance capacity, zone resource pool
	// exhausted, quota exceeded, and similar). Retrying later, in another
	// zone, or with a different instance type is usually what is needed.
	ExitCodeCapacity = 10
)

// ExitCoder is implemented by errors that want the process to terminate with a
// specific exit code. Returning a plain error from a command is still fine and
// keeps the generic ExitCodeError; implementing this interface is how a
// command opts into a more specific one.
type ExitCoder interface {
	error
	ExitCode() int
}

// exitCodeError attaches an exit code to an error.
type exitCodeError struct {
	err  error
	code int
}

func (e *exitCodeError) Error() string { return e.err.Error() }
func (e *exitCodeError) Unwrap() error { return e.err }
func (e *exitCodeError) ExitCode() int { return e.code }

// WithExitCode tags an error so that aerolab terminates with the given exit
// code. It is a no-op on a nil error.
func WithExitCode(err error, code int) error {
	if err == nil {
		return nil
	}
	return &exitCodeError{err: err, code: code}
}

// ExitCodeFor resolves the process exit code for an error returned by a
// command. Errors that carry their own code win; otherwise cloud capacity
// failures are recognised so they can be retried by the caller.
func ExitCodeFor(err error) int {
	if err == nil {
		return ExitCodeOK
	}
	var coder ExitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	// IsCapacityError also matches on the provider's error text, so this still
	// works for errors that were reformatted rather than wrapped on the way up.
	if backends.IsCapacityError(err) {
		return ExitCodeCapacity
	}
	return ExitCodeError
}
