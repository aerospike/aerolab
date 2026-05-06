package dispatcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestTailFileReadsRotation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "aerospike.log")
	if err := os.WriteFile(logPath, []byte("first\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lines := make(chan string, 4)
	errCh := make(chan error, 1)
	go func() {
		errCh <- tailFile(ctx, logPath, 0, func(line string, offset int64) error {
			lines <- line
			if line == "first" {
				if err := os.Rename(logPath, filepath.Join(dir, "aerospike.log.1")); err != nil {
					return err
				}
				if err := os.WriteFile(logPath, []byte("second\n"), 0644); err != nil {
					return err
				}
			}
			if line == "second" {
				cancel()
			}
			return nil
		})
	}()
	got := []string{}
	for len(got) < 2 {
		select {
		case line := <-lines:
			got = append(got, line)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for rotated lines; got %v", got)
		}
	}
	err := <-errCh
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("tailFile returned %v", err)
	}
	if !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("unexpected lines: %v", got)
	}
}
