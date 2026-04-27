package db

import (
	"bytes"
	"math"
	"testing"
)

func TestCodecRoundTripAllTypes(t *testing.T) {
	big := bytes.Repeat([]byte{'x'}, 2<<20) // > 2 MiB payload
	entries := []codecEntry{
		{ColID: 1, Typ: TypeInt64, Val: Int(-42)},
		{ColID: 2, Typ: TypeFloat64, Val: Float(3.14159)},
		{ColID: 3, Typ: TypeString, Val: Str("hello world")},
		{ColID: 4, Typ: TypeBytes, Val: BytesV([]byte{1, 2, 3, 4})},
		{ColID: 5, Typ: TypeBool, Val: BoolV(true)},
		{ColID: 6, Typ: TypeBytes, Val: BytesV(big)},
	}
	raw, err := encodeRow(entries)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	all, err := decodeRowAll(raw)
	if err != nil {
		t.Fatalf("decodeRowAll: %v", err)
	}
	if len(all) != len(entries) {
		t.Fatalf("got %d cols, want %d", len(all), len(entries))
	}
	if v, _ := all[1].AsInt(); v != -42 {
		t.Errorf("int: got %d", v)
	}
	if v, _ := all[2].AsFloat(); v != 3.14159 {
		t.Errorf("float: got %v", v)
	}
	if v, _ := all[3].AsString(); v != "hello world" {
		t.Errorf("string: got %q", v)
	}
	if v, _ := all[4].AsBytes(); !bytes.Equal(v, []byte{1, 2, 3, 4}) {
		t.Errorf("bytes: got %v", v)
	}
	if v, _ := all[5].AsBool(); !v {
		t.Errorf("bool: got %v", v)
	}
	if v, _ := all[6].AsBytes(); !bytes.Equal(v, big) {
		t.Errorf("big bytes: length mismatch got %d", len(v))
	}
}

func TestCodecProjectionOnlyDecodesWanted(t *testing.T) {
	entries := []codecEntry{
		{ColID: 10, Typ: TypeInt64, Val: Int(1)},
		{ColID: 20, Typ: TypeString, Val: Str("keep me")},
		{ColID: 30, Typ: TypeFloat64, Val: Float(2.5)},
		{ColID: 40, Typ: TypeBytes, Val: BytesV([]byte{9})},
	}
	raw, err := encodeRow(entries)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := map[uint32]struct{}{20: {}, 30: {}}
	out := make(map[uint32]Value)
	if err := decodeWanted(raw, want, out); err != nil {
		t.Fatalf("decodeWanted: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 decoded, got %d", len(out))
	}
	if _, ok := out[10]; ok {
		t.Errorf("col 10 should not have been decoded")
	}
	if _, ok := out[40]; ok {
		t.Errorf("col 40 should not have been decoded")
	}
	if v, ok := out[20].AsString(); !ok || v != "keep me" {
		t.Errorf("col 20 mismatch: %q ok=%v", v, ok)
	}
	if v, ok := out[30].AsFloat(); !ok || v != 2.5 {
		t.Errorf("col 30 mismatch: %v ok=%v", v, ok)
	}
}

func TestCodecDecodeSingleShortCircuits(t *testing.T) {
	entries := []codecEntry{
		{ColID: 1, Typ: TypeInt64, Val: Int(111)},
		{ColID: 2, Typ: TypeInt64, Val: Int(222)},
		{ColID: 3, Typ: TypeInt64, Val: Int(333)},
	}
	raw, err := encodeRow(entries)
	if err != nil {
		t.Fatal(err)
	}
	v, ok, err := decodeSingle(raw, 2)
	if err != nil || !ok {
		t.Fatalf("decodeSingle(2): ok=%v err=%v", ok, err)
	}
	if iv, _ := v.AsInt(); iv != 222 {
		t.Errorf("got %d", iv)
	}
	_, ok, err = decodeSingle(raw, 99)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("col 99 unexpectedly present")
	}
}

func TestCodecInt64Edges(t *testing.T) {
	for _, v := range []int64{math.MinInt64, -1, 0, 1, math.MaxInt64} {
		entries := []codecEntry{{ColID: 1, Typ: TypeInt64, Val: Int(v)}}
		raw, err := encodeRow(entries)
		if err != nil {
			t.Fatalf("%d: encode %v", v, err)
		}
		got, err := decodeRowAll(raw)
		if err != nil {
			t.Fatalf("%d: decode %v", v, err)
		}
		iv, ok := got[1].AsInt()
		if !ok || iv != v {
			t.Errorf("%d round-trip got %d ok=%v", v, iv, ok)
		}
	}
}

func TestBiasInt64MonotonicOrdering(t *testing.T) {
	cases := []int64{math.MinInt64, -1000, -1, 0, 1, 1000, math.MaxInt64}
	for i := 0; i < len(cases)-1; i++ {
		if biasInt64(cases[i]) >= biasInt64(cases[i+1]) {
			t.Errorf("bias ordering broken: %d -> %x vs %d -> %x", cases[i], biasInt64(cases[i]), cases[i+1], biasInt64(cases[i+1]))
		}
		if unbiasUint64(biasInt64(cases[i])) != cases[i] {
			t.Errorf("roundtrip %d failed", cases[i])
		}
	}
}

// TestIndexRangeBoundsEdgeCases covers the overflow branches in
// indexRangeBounds where hi=MaxInt64 intersects a maxed colID / setID.
// The invariant we check is that upper > lower under the bytewise
// comparator (Pebble's iter order); if upper ever wraps below lower
// the iterator would silently return nothing.
func TestIndexRangeBoundsEdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		setID uint32
		colID uint32
		lo    int64
		hi    int64
	}{
		{"normal", 0, 0, 0, 100},
		{"hi_max", 0, 0, 0, math.MaxInt64},
		{"hi_max_col_max", 0, math.MaxUint32, 0, math.MaxInt64},
		{"hi_max_col_max_set_max", math.MaxUint32, math.MaxUint32, 0, math.MaxInt64},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lo, up := indexRangeBounds(c.setID, c.colID, c.lo, c.hi)
			if bytes.Compare(lo, up) >= 0 {
				t.Errorf("upper (%x) <= lower (%x)", up, lo)
			}
		})
	}
}

// TestMetaSchemaBoundsOrdered asserts metaSchemaLower() sorts strictly
// below metaSchemaUpper() byte-wise. If this invariant ever fails,
// loadSchemas would silently observe an empty schema namespace.
func TestMetaSchemaBoundsOrdered(t *testing.T) {
	lo := metaSchemaLower()
	up := metaSchemaUpper()
	if bytes.Compare(lo, up) >= 0 {
		t.Fatalf("metaSchemaUpper (%x) must sort strictly above metaSchemaLower (%x)", up, lo)
	}
}
