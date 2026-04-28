package db

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// CurrentStorageVersion exposes the on-disk storage version this build
// understands. The /debug/db/info handler in the plugin surfaces it so
// operators can quickly tell whether a directory at ${path} would be
// upgraded-or-wiped on next Open. The unexported sibling
// currentStorageVersion is the source of truth.
func CurrentStorageVersion() uint32 { return currentStorageVersion }

// --- Value ---

// WireValue is the JSON shape of a typed Value. Exactly one field must
// be set per value; the rest are omitted. The encoding is tagged-union
// rather than a single "v" + "type" pair so that hand-written JSON is
// hard to misuse:
//
//	{"int": 5}
//	{"float": 3.14}
//	{"str": "hello"}
//	{"bytes": "aGVsbG8="}   // base64
//	{"bool": true}
//
// JSON numbers are decoded via json.Number so int64 round-trips through
// arbitrary-precision and we don't silently truncate ts ms values via
// float64.
type WireValue struct {
	Int   *json.Number `json:"int,omitempty"`
	Float *float64     `json:"float,omitempty"`
	Str   *string      `json:"str,omitempty"`
	Bytes *string      `json:"bytes,omitempty"` // base64 std-encoded
	Bool  *bool        `json:"bool,omitempty"`
}

// ToValue converts a WireValue to a typed Value. Returns an error if
// zero or more than one variant is set, or if the int/bytes payload
// cannot be decoded.
func (w WireValue) ToValue() (Value, error) {
	set := 0
	if w.Int != nil {
		set++
	}
	if w.Float != nil {
		set++
	}
	if w.Str != nil {
		set++
	}
	if w.Bytes != nil {
		set++
	}
	if w.Bool != nil {
		set++
	}
	if set == 0 {
		return Value{}, errors.New("wire: empty value (need exactly one of int|float|str|bytes|bool)")
	}
	if set > 1 {
		return Value{}, errors.New("wire: ambiguous value (set exactly one of int|float|str|bytes|bool)")
	}
	switch {
	case w.Int != nil:
		i, err := w.Int.Int64()
		if err != nil {
			return Value{}, fmt.Errorf("wire: int64 parse: %w", err)
		}
		return Int(i), nil
	case w.Float != nil:
		return Float(*w.Float), nil
	case w.Str != nil:
		return Str(*w.Str), nil
	case w.Bytes != nil:
		raw, err := base64.StdEncoding.DecodeString(*w.Bytes)
		if err != nil {
			return Value{}, fmt.Errorf("wire: bytes base64 decode: %w", err)
		}
		return BytesV(raw), nil
	case w.Bool != nil:
		return BoolV(*w.Bool), nil
	}
	return Value{}, errors.New("wire: unreachable")
}

// FromValue produces the JSON-shape of a Value. The zero Value renders
// as a JSON object with no fields set, which round-trips back to the
// zero Value via ToValue (which then errors). Callers that may emit a
// zero Value should test v.IsZero() first and skip the field entirely.
func FromValue(v Value) WireValue {
	switch v.t {
	case TypeInt64:
		n := json.Number(fmt.Sprintf("%d", v.i))
		return WireValue{Int: &n}
	case TypeFloat64:
		f := v.f
		return WireValue{Float: &f}
	case TypeString:
		s := string(v.b)
		return WireValue{Str: &s}
	case TypeBytes:
		s := base64.StdEncoding.EncodeToString(v.b)
		return WireValue{Bytes: &s}
	case TypeBool:
		b := v.i != 0
		return WireValue{Bool: &b}
	}
	return WireValue{}
}

// --- Row ---

// WireRow is the JSON shape of a Row. Keys are the column names. The
// custom marshaller keeps the on-the-wire payload compact: int64 / bool
// values are JSON numbers/booleans rather than the tagged-union form.
// Bytes are base64 strings prefixed with "b64:" so the reader can tell
// them apart from plain strings; callers reading the Row back into Go
// should use ParseWireRow which restores types from the schema.
type WireRow map[string]WireValue

// FromRow converts a Row to its JSON shape.
func FromRow(r Row) WireRow {
	if r == nil {
		return nil
	}
	out := make(WireRow, len(r))
	for k, v := range r {
		out[k] = FromValue(v)
	}
	return out
}

// ToRow converts a WireRow back to a typed Row. Errors if any value is
// malformed.
func (w WireRow) ToRow() (Row, error) {
	if w == nil {
		return nil, nil
	}
	out := make(Row, len(w))
	for k, v := range w {
		val, err := v.ToValue()
		if err != nil {
			return nil, fmt.Errorf("wire: row column %q: %w", k, err)
		}
		out[k] = val
	}
	return out, nil
}

// --- Expr ---

// WireExpr is the JSON shape of a filter Expr. Exactly one of the
// dispatch fields must be set per node:
//
//	{"and":  [expr,...]}
//	{"or":   [expr,...]}
//	{"not":   expr}
//	{"eq":      {"col":"X","value":VAL}}
//	{"in":      {"col":"X","values":[VAL,...]}}
//	{"between": {"col":"X","lo":VAL,"hi":VAL}}
//	{"exists":  {"col":"X"}}
//
// The single-key dispatch (rather than an "op" discriminator) is
// chosen because:
//   - It makes hand-written JSON read like a Lisp s-expression and
//     therefore matches the existing Eq/In/Between/Exists/Not/And/Or
//     builder names one-to-one.
//   - encoding/json's standard struct tagging gives us validation for
//     free: a node with two dispatch keys set is caught by the
//     "exactly one" check below before we ever look at the payload.
type WireExpr struct {
	And     []WireExpr      `json:"and,omitempty"`
	Or      []WireExpr      `json:"or,omitempty"`
	Not     *WireExpr       `json:"not,omitempty"`
	Eq      *WireEqExpr     `json:"eq,omitempty"`
	In      *WireInExpr     `json:"in,omitempty"`
	Between *WireBetween    `json:"between,omitempty"`
	Exists  *WireExistsExpr `json:"exists,omitempty"`
}

type WireEqExpr struct {
	Col   string    `json:"col"`
	Value WireValue `json:"value"`
}

type WireInExpr struct {
	Col    string      `json:"col"`
	Values []WireValue `json:"values"`
}

type WireBetween struct {
	Col string    `json:"col"`
	Lo  WireValue `json:"lo"`
	Hi  WireValue `json:"hi"`
}

type WireExistsExpr struct {
	Col string `json:"col"`
}

// ToExpr compiles a WireExpr to the Expr tree the QueryBuilder accepts.
func (w *WireExpr) ToExpr() (Expr, error) {
	if w == nil {
		return nil, nil
	}
	set := 0
	if w.And != nil {
		set++
	}
	if w.Or != nil {
		set++
	}
	if w.Not != nil {
		set++
	}
	if w.Eq != nil {
		set++
	}
	if w.In != nil {
		set++
	}
	if w.Between != nil {
		set++
	}
	if w.Exists != nil {
		set++
	}
	if set == 0 {
		return nil, errors.New("wire: empty expression (need one of and|or|not|eq|in|between|exists)")
	}
	if set > 1 {
		return nil, errors.New("wire: ambiguous expression (set exactly one of and|or|not|eq|in|between|exists)")
	}
	switch {
	case w.And != nil:
		es := make([]Expr, 0, len(w.And))
		for i := range w.And {
			e, err := w.And[i].ToExpr()
			if err != nil {
				return nil, fmt.Errorf("wire: and[%d]: %w", i, err)
			}
			es = append(es, e)
		}
		return And(es...), nil
	case w.Or != nil:
		es := make([]Expr, 0, len(w.Or))
		for i := range w.Or {
			e, err := w.Or[i].ToExpr()
			if err != nil {
				return nil, fmt.Errorf("wire: or[%d]: %w", i, err)
			}
			es = append(es, e)
		}
		return Or(es...), nil
	case w.Not != nil:
		e, err := w.Not.ToExpr()
		if err != nil {
			return nil, fmt.Errorf("wire: not: %w", err)
		}
		return Not(e), nil
	case w.Eq != nil:
		v, err := w.Eq.Value.ToValue()
		if err != nil {
			return nil, fmt.Errorf("wire: eq.value: %w", err)
		}
		return Eq(w.Eq.Col, v), nil
	case w.In != nil:
		vs := make([]Value, 0, len(w.In.Values))
		for i, wv := range w.In.Values {
			v, err := wv.ToValue()
			if err != nil {
				return nil, fmt.Errorf("wire: in.values[%d]: %w", i, err)
			}
			vs = append(vs, v)
		}
		return In(w.In.Col, vs...), nil
	case w.Between != nil:
		lo, err := w.Between.Lo.ToValue()
		if err != nil {
			return nil, fmt.Errorf("wire: between.lo: %w", err)
		}
		hi, err := w.Between.Hi.ToValue()
		if err != nil {
			return nil, fmt.Errorf("wire: between.hi: %w", err)
		}
		return Between(w.Between.Col, lo, hi), nil
	case w.Exists != nil:
		return Exists(w.Exists.Col), nil
	}
	return nil, errors.New("wire: unreachable")
}

// --- Query ---

// WireQuery is the JSON shape of a debug/query request. The fields map
// directly to QueryBuilder.* so the only thing the server has to do is
// translate values + compile the Expr tree.
//
//	{
//	  "set": "metrics",
//	  "between": {"col":"timestamp","lo":{"int":1},"hi":{"int":2}},
//	  "where":   {"and":[{"eq":{"col":"X","value":{"int":0}}}]},
//	  "project": ["timestamp","latency_p99"],
//	  "limit":   1000
//	}
type WireQuery struct {
	Set     string       `json:"set"`
	Between *WireBetween `json:"between,omitempty"`
	Where   *WireExpr    `json:"where,omitempty"`
	Project []string     `json:"project,omitempty"`
	// Limit caps the number of rows returned. <=0 means no cap. The
	// server applies its own ceiling on top of this; see the debug
	// handler.
	Limit int `json:"limit,omitempty"`
}
