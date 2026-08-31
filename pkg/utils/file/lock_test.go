package file_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/citrusleaf/aerolab/pkg/utils/file"
	"github.com/stretchr/testify/require"
)

func TestAcquireLockCreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "test.lock")
	lock, err := file.AcquireLock(path)
	require.NoError(t, err)
	require.FileExists(t, path)
	require.NoError(t, lock.Release())
}

func TestAcquireLockExcludesSecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	held, err := file.AcquireLock(path)
	require.NoError(t, err)

	acquired := make(chan struct{})
	go func() {
		second, err := file.AcquireLock(path)
		if err == nil {
			second.Release() //nolint:errcheck
		}
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("second holder acquired the lock while it was still held")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, held.Release())
	select {
	case <-acquired:
	case <-time.After(10 * time.Second):
		t.Fatal("second holder did not acquire the lock after it was released")
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	lock, err := file.AcquireLock(filepath.Join(t.TempDir(), "test.lock"))
	require.NoError(t, err)
	require.NoError(t, lock.Release())
	require.NoError(t, lock.Release())
}
