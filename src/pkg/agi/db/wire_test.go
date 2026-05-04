package db

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWireValueRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		json string // expected canonical encoding
	}{
		{"int", Int(42), `{"int":42}`},
		{"int_neg", Int(-1), `{"int":-1}`},
		{"int_max_ms", Int(1730000000123), `{"int":1730000000123}`},
		{"float", Float(3.5), `{"float":3.5}`},
		{"str", Str("hello"), `{"str":"hello"}`},
		{"bool_true", BoolV(true), `{"bool":true}`},
		{"bool_false", BoolV(false), `{"bool":false}`},
		{"bytes", BytesV([]byte{0x01, 0x02, 0x03}), `{"bytes":"AQID"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wv := FromValue(tc.v)
			b, err := json.Marshal(wv)
			if err != nil {
				t.Fatalf("marshal: %s", err)
			}
			if !strings.Contains(string(b), strings.TrimPrefix(strings.TrimSuffix(tc.json, "}"), "{")) {
				t.Fatalf("marshal: got %s want %s", string(b), tc.json)
			}
			var back WireValue
			if err := json.Unmarshal(b, &back); err != nil {
				t.Fatalf("unmarshal: %s", err)
			}
			got, err := back.ToValue()
			if err != nil {
				t.Fatalf("ToValue: %s", err)
			}
			if c, ok := cmpValues(got, tc.v); !ok || c != 0 {
				t.Fatalf("round-trip: got %v want %v", got, tc.v)
			}
		})
	}
}

func TestWireValueRejectsAmbiguous(t *testing.T) {
	body := []byte(`{"int":"1","str":"hello"}`)
	var wv WireValue
	if err := json.Unmarshal(body, &wv); err != nil {
		t.Fatalf("unmarshal: %s", err)
	}
	if _, err := wv.ToValue(); err == nil {
		t.Fatal("expected ambiguous-value error, got nil")
	}
}

func TestWireValueRejectsEmpty(t *testing.T) {
	body := []byte(`{}`)
	var wv WireValue
	if err := json.Unmarshal(body, &wv); err != nil {
		t.Fatalf("unmarshal: %s", err)
	}
	if _, err := wv.ToValue(); err == nil {
		t.Fatal("expected empty-value error, got nil")
	}
}

func TestWireExprRoundTripBuildsTreeThatEvaluates(t *testing.T) {
	// Build a tree directly, marshal, unmarshal, recompile, and run
	// it against an in-memory row to assert the recompiled Expr
	// behaves identically to the original.
	want := And(
		Eq("ClusterName", Int(3)),
		Or(
			Exists("latency_p99"),
			Not(Eq("dropped", BoolV(true))),
		),
		Between("ts", Int(100), Int(200)),
	)
	// The wire form is constructed by hand because there's no
	// FromExpr helper — debug callers always start from JSON; the
	// server only ever serialises rows, not exprs.
	wireBody := []byte(`{
	  "and": [
	    {"eq": {"col":"ClusterName","value":{"int":"3"}}},
	    {"or": [
	      {"exists":{"col":"latency_p99"}},
	      {"not":{"eq":{"col":"dropped","value":{"bool":true}}}}
	    ]},
	    {"between":{"col":"ts","lo":{"int":"100"},"hi":{"int":"200"}}}
	  ]
	}`)
	var we WireExpr
	if err := json.Unmarshal(wireBody, &we); err != nil {
		t.Fatalf("unmarshal: %s", err)
	}
	got, err := we.ToExpr()
	if err != nil {
		t.Fatalf("ToExpr: %s", err)
	}

	row := Row{
		"ClusterName": Int(3),
		"latency_p99": Int(99),
		"dropped":     BoolV(false),
		"ts":          Int(150),
	}
	getter := func(name string) (Value, bool) {
		v, ok := row[name]
		return v, ok
	}
	if !want.eval(getter) {
		t.Fatal("hand-built tree did not match the row; test bug")
	}
	if !got.eval(getter) {
		t.Fatal("recompiled wire tree did not match the row")
	}

	// Flip ClusterName to mismatch and verify both reject.
	row["ClusterName"] = Int(4)
	if want.eval(getter) || got.eval(getter) {
		t.Fatal("expected both trees to reject ClusterName=4")
	}
}

func TestWireExprRejectsAmbiguous(t *testing.T) {
	body := []byte(`{"and":[],"or":[]}`)
	var we WireExpr
	if err := json.Unmarshal(body, &we); err != nil {
		t.Fatalf("unmarshal: %s", err)
	}
	if _, err := we.ToExpr(); err == nil {
		t.Fatal("expected ambiguous-expr error, got nil")
	}
}

func TestWireExprRejectsEmpty(t *testing.T) {
	body := []byte(`{}`)
	var we WireExpr
	if err := json.Unmarshal(body, &we); err != nil {
		t.Fatalf("unmarshal: %s", err)
	}
	if _, err := we.ToExpr(); err == nil {
		t.Fatal("expected empty-expr error, got nil")
	}
}

func TestWireRowRoundTrip(t *testing.T) {
	r := Row{
		"a": Int(1),
		"b": Str("hello"),
		"c": Float(1.5),
		"d": BoolV(true),
		"e": BytesV([]byte{0xff, 0x00}),
	}
	wr := FromRow(r)
	b, err := json.Marshal(wr)
	if err != nil {
		t.Fatalf("marshal: %s", err)
	}
	var back WireRow
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %s", err)
	}
	got, err := back.ToRow()
	if err != nil {
		t.Fatalf("ToRow: %s", err)
	}
	if len(got) != len(r) {
		t.Fatalf("got %d cols, want %d", len(got), len(r))
	}
	for k, want := range r {
		gv, ok := got[k]
		if !ok {
			t.Fatalf("missing column %q", k)
		}
		if c, ok := cmpValues(gv, want); !ok || c != 0 {
			t.Fatalf("col %q: got %v want %v", k, gv, want)
		}
	}
}

func TestCurrentStorageVersionExposed(t *testing.T) {
	if got := CurrentStorageVersion(); got != currentStorageVersion {
		t.Fatalf("CurrentStorageVersion=%d, want %d", got, currentStorageVersion)
	}
}
