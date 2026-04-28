package ingest

import (
	"strings"
	"testing"
)

// TestMetricsRowKeyShape locks in the expected output format
// (32-char lowercase hex) and validates that all three entry
// points produce mutually consistent hashes for equivalent
// inputs. The exact hash values are not asserted; that's the
// xxh3 library's contract, and locking specific bytes here
// would just couple our tests to an upstream choice.
func TestMetricsRowKeyShape(t *testing.T) {
	const (
		cluster = "aero-tbsprod"
		node    = "1_bb97829b3565000"
		line    = "Apr 22 2026 00:00:25 GMT+0700: INFO (nsup): (nsup.c:419) {vdsp} hi"
	)

	combined := cluster + MetricsRowPKSeparator + node
	joined := combined + MetricsRowPKSeparator + line

	a := MetricsRowKey(cluster, node, line)
	b := MetricsRowKeyFromCombined(combined, line)
	c := MetricsRowKeyFromString(joined)

	if a != b || b != c {
		t.Fatalf("hash mismatch across entry points:\n  RowKey            = %s\n  FromCombined      = %s\n  FromString        = %s", a, b, c)
	}

	if got, want := len(a), 32; got != want {
		t.Fatalf("hash length: got %d, want %d (%q)", got, want, a)
	}

	for i, r := range a {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			t.Fatalf("non-lowercase-hex byte %q at index %d in %q", r, i, a)
		}
	}
}

// TestMetricsRowKeyDeterministic asserts the hash is a pure
// function of its inputs across many calls — the idempotency
// contract that lets ingest re-run safely on the same source
// data without producing duplicate rows.
func TestMetricsRowKeyDeterministic(t *testing.T) {
	cases := []struct {
		cluster, node, line string
	}{
		{"", "", ""},
		{"a", "b", "c"},
		{"cluster-a", "1_xxx", "INFO line"},
		{"with::/::sep", "0_y", "log with ::/:: inside"},
		{strings.Repeat("x", 100), strings.Repeat("y", 100), strings.Repeat("z", 1000)},
	}
	for _, tc := range cases {
		first := MetricsRowKey(tc.cluster, tc.node, tc.line)
		for i := 0; i < 16; i++ {
			again := MetricsRowKey(tc.cluster, tc.node, tc.line)
			if again != first {
				t.Fatalf("non-deterministic hash for (%q,%q,%q): %s vs %s",
					tc.cluster, tc.node, tc.line, first, again)
			}
		}
	}
}

// TestMetricsRowKeyDistinguishes confirms the hash actually
// distinguishes between the three positional fields. Even
// though XXH3-128 is non-cryptographic, the input mixing must
// ensure that swapping a separator or moving a fragment from
// "cluster" into "node" produces a different output, otherwise
// re-ingest from differently-shaped sources could collapse.
func TestMetricsRowKeyDistinguishes(t *testing.T) {
	mustDiffer := func(label, a, b string) {
		t.Helper()
		if a == b {
			t.Fatalf("%s: hashes unexpectedly equal: %s", label, a)
		}
	}

	base := MetricsRowKey("alpha", "beta", "gamma")
	mustDiffer("change cluster", base, MetricsRowKey("alpha2", "beta", "gamma"))
	mustDiffer("change node", base, MetricsRowKey("alpha", "beta2", "gamma"))
	mustDiffer("change line", base, MetricsRowKey("alpha", "beta", "gamma2"))
	// Field-shifting attacks: "ab" / "" / "c" should hash
	// differently from "a" / "b" / "c" because the separator
	// is part of the input. This is what makes re-keying
	// safe across operator-driven schema renames.
	mustDiffer("field shift L", MetricsRowKey("ab", "", "c"), MetricsRowKey("a", "b", "c"))
	mustDiffer("field shift R", MetricsRowKey("a", "", "bc"), MetricsRowKey("a", "b", "c"))
}
