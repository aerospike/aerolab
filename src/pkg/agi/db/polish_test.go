package db

import (
	"slices"
	"testing"
)

func TestPebbleFatalsCounterExposed(t *testing.T) {
	d := openTestDB(t)
	var _ uint64 = d.Stats().PebbleFatals
	if got := d.Stats().PebbleFatals; got != 0 {
		t.Errorf("PebbleFatals initial: got %d want 0", got)
	}
}

func TestSetsSkipsDropped(t *testing.T) {
	d := openTestDB(t)
	spec := []ColumnSpec{{Name: "k", Type: TypeInt64, Indexed: true}}
	if err := d.RegisterSet("a", spec); err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterSet("b", spec); err != nil {
		t.Fatal(err)
	}
	if err := d.DropSet("a"); err != nil {
		t.Fatal(err)
	}
	names := d.Sets()
	if slices.Contains(names, "a") {
		t.Errorf("Sets() should not include dropped set %q, got %v", "a", names)
	}
	if !slices.Contains(names, "b") {
		t.Errorf("Sets() should include %q, got %v", "b", names)
	}
}
