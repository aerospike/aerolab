package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/citrusleaf/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

func TestExitCodeFor(t *testing.T) {
	capacity := backends.NewCapacityError(backends.BackendTypeAWS, "InsufficientInstanceCapacity", "no capacity in us-east-1a", errors.New("InsufficientInstanceCapacity"))
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, ExitCodeOK},
		{"generic", errors.New("something broke"), ExitCodeError},
		{"capacity error", capacity, ExitCodeCapacity},
		{"wrapped capacity error", fmt.Errorf("cluster create failed: %w", capacity), ExitCodeCapacity},
		{
			"capacity error reported by an execute error",
			&ExecuteError{Err: capacity, Logger: logger.NewLogger()},
			ExitCodeCapacity,
		},
		{
			"gcp capacity error recognised from the provider text",
			errors.New("operation failed: ZONE_RESOURCE_POOL_EXHAUSTED"),
			ExitCodeCapacity,
		},
		{"explicit exit code wins", WithExitCode(capacity, 7), 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCodeFor(tc.err); got != tc.want {
				t.Errorf("ExitCodeFor(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// The generic failure path must keep reporting 1, and ErrExecuteError must
// still identify an already-logged error now that ExecuteError unwraps to the
// error it carries.
func TestExecuteErrorStillMarksLoggedErrors(t *testing.T) {
	wrapped := &ExecuteError{Err: errors.New("boom"), Logger: logger.NewLogger()}
	if !errors.Is(wrapped, ErrExecuteError) {
		t.Error("ExecuteError should match ErrExecuteError")
	}
	if got := ExitCodeFor(wrapped); got != ExitCodeError {
		t.Errorf("ExitCodeFor = %d, want %d", got, ExitCodeError)
	}
	if !errors.Is(fmt.Errorf("outer: %w", wrapped), ErrExecuteError) {
		t.Error("wrapping should preserve the ErrExecuteError marker")
	}
}
