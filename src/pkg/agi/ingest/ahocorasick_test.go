package ingest

import (
	"reflect"
	"strings"
	"testing"
)

func TestACMatcher_EmptyNeedle(t *testing.T) {
	if _, err := newACMatcher([]string{"foo", "", "bar"}); err == nil {
		t.Fatal("expected error for empty needle, got nil")
	}
}

func TestACMatcher_NoNeedles(t *testing.T) {
	if _, err := newACMatcher(nil); err == nil {
		t.Fatal("expected error for nil needles, got nil")
	}
	if _, err := newACMatcher([]string{}); err == nil {
		t.Fatal("expected error for empty needles, got nil")
	}
}

func TestACMatcher_FirstIndex_NoMatch(t *testing.T) {
	m, err := newACMatcher([]string{"foo", "bar"})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.FirstIndex("hello world"); got != -1 {
		t.Fatalf("FirstIndex(\"hello world\")=%d; want -1", got)
	}
}

func TestACMatcher_FirstIndex_SingleMatch(t *testing.T) {
	m, err := newACMatcher([]string{"foo", "bar", "baz"})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.FirstIndex("contains bar in middle"); got != 1 {
		t.Fatalf("FirstIndex=%d; want 1", got)
	}
}

func TestACMatcher_FirstIndex_FirstMatchWins(t *testing.T) {
	// Both "needle1" (idx 0) and "needle2" (idx 1) appear in the
	// haystack; FirstIndex must return the smallest pattern index
	// regardless of byte position in the line. This preserves the
	// `for _, p := range Patterns { if strings.Contains(...) }`
	// semantic of the pre-fix code.
	m, err := newACMatcher([]string{"needle1", "needle2"})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.FirstIndex("xxx needle2 yyy needle1 zzz"); got != 0 {
		t.Fatalf("FirstIndex=%d; want 0 (first-match-wins)", got)
	}
	if got := m.FirstIndex("xxx needle1 yyy needle2 zzz"); got != 0 {
		t.Fatalf("FirstIndex=%d; want 0", got)
	}
}

func TestACMatcher_FirstIndex_OverlappingNeedles(t *testing.T) {
	// "her" overlaps with "she" / "his" via shared suffixes; the
	// classic AC test case.
	m, err := newACMatcher([]string{"he", "she", "his", "hers"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		input string
		want  int
	}{
		{"he", 0},
		{"she", 0},  // contains "he" at index 1
		{"his", 2},
		{"ushers", 0},  // contains "she" (idx 1), "he" (idx 0), "hers" (idx 3); 0 wins
		{"missing", -1},
	}
	for _, tc := range tests {
		if got := m.FirstIndex(tc.input); got != tc.want {
			t.Errorf("FirstIndex(%q)=%d; want %d", tc.input, got, tc.want)
		}
	}
}

func TestACMatcher_FirstIndex_SingleByteNeedle(t *testing.T) {
	m, err := newACMatcher([]string{"x"})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.FirstIndex("a x b"); got != 0 {
		t.Fatalf("FirstIndex=%d; want 0", got)
	}
	if got := m.FirstIndex("abc"); got != -1 {
		t.Fatalf("FirstIndex=%d; want -1", got)
	}
}

func TestACMatcher_AnyIndices(t *testing.T) {
	m, err := newACMatcher([]string{"he", "she", "his", "hers"})
	if err != nil {
		t.Fatal(err)
	}
	got := m.AnyIndices("ushers")
	want := []int{0, 1, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AnyIndices(ushers)=%v; want %v", got, want)
	}
	if g := m.AnyIndices("nothing here"); !reflect.DeepEqual(g, []int{0}) {
		t.Fatalf("AnyIndices(\"nothing here\")=%v; want [0]", g)
	}
	if g := m.AnyIndices("xxxxxx"); g != nil {
		t.Fatalf("AnyIndices(xxxxxx)=%v; want nil", g)
	}
}

func TestACMatcher_AgreesWithStringsContains(t *testing.T) {
	// Brute-force cross-check: for every (needles, line) pair below,
	// AC's FirstIndex must equal the smallest pattern index `i` for
	// which strings.Contains(line, needles[i]) is true. This
	// captures any semantic divergence between the AC walker and the
	// pre-fix linear loop.
	cases := [][]string{
		{"foo", "bar", "baz"},
		{"a", "ab", "abc"},
		{"xx", "x", "xy"},
		{"the", "quick", "brown", "fox"},
		{"INFO", "WARN", "ERROR", "DEBUG"},
	}
	lines := []string{
		"",
		"the quick brown fox",
		"abc",
		"INFO ticker WARN",
		"foobarbaz",
		"xxxxx",
		"nothing matches here",
	}
	for ci, needles := range cases {
		m, err := newACMatcher(needles)
		if err != nil {
			t.Fatalf("case %d: build: %s", ci, err)
		}
		for li, line := range lines {
			want := -1
			for i, n := range needles {
				if strings.Contains(line, n) {
					want = i
					break
				}
			}
			if got := m.FirstIndex(line); got != want {
				t.Errorf("case %d line %d FirstIndex(%q, %v)=%d; want %d",
					ci, li, line, needles, got, want)
			}
		}
	}
}
