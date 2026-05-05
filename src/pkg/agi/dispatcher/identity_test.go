package dispatcher

import "testing"

// TestMatchIdentity_TypicalTickerLine exercises the standard
// NODE-ID/CLUSTER-NAME ticker pattern emitted ~every 10s by the
// server. This is the line the dispatcher's log-scan fallback path
// (scanLogForIdentity) waits for when asinfo is unavailable.
func TestMatchIdentity_TypicalTickerLine(t *testing.T) {
	line := "Aug 20 2024 12:34:56 GMT: INFO (info): (ticker.c:184)  NODE-ID bb9a1c2d3 CLUSTER-SIZE 3 CLUSTER-NAME bob"
	nid, cn, ok := matchIdentity(line)
	if !ok {
		t.Fatal("expected match")
	}
	if nid != "bb9a1c2d3" {
		t.Errorf("node-id: want bb9a1c2d3, got %q", nid)
	}
	if cn != "bob" {
		t.Errorf("cluster-name: want bob, got %q", cn)
	}
}

// TestMatchIdentity_NoClusterName covers the (legacy / community
// build) variant where CLUSTER-NAME is omitted from the ticker.
// matchIdentity must still surface the node-id and return ok=true so
// the dispatcher can use the CLI-supplied or "null" cluster default.
func TestMatchIdentity_NoClusterName(t *testing.T) {
	line := "Aug 20 2024 12:34:56 GMT: INFO (info): (ticker.c:184)  NODE-ID bb9a1c2d3 CLUSTER-SIZE 3"
	nid, cn, ok := matchIdentity(line)
	if !ok {
		t.Fatal("expected match")
	}
	if nid != "bb9a1c2d3" {
		t.Errorf("node-id: want bb9a1c2d3, got %q", nid)
	}
	if cn != "" {
		t.Errorf("cluster-name: want empty, got %q", cn)
	}
}

// TestMatchIdentity_NoMatch ensures unrelated lines are rejected (no
// false positives that would let the dispatcher attach a wrong
// identity to all subsequent rows).
func TestMatchIdentity_NoMatch(t *testing.T) {
	line := "Aug 20 2024 12:34:56 GMT: INFO (drv_ssd): (drv_ssd.c:42) some other line"
	if _, _, ok := matchIdentity(line); ok {
		t.Fatal("expected no match")
	}
}
