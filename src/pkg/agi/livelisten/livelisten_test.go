package livelisten

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestReadTokenDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one"), []byte("alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "two"), []byte(" beta "), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readTokenDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	want := []string{"alpha", "beta"}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("tokens: got %v want %v", got, want)
	}
}
