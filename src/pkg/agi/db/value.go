package db

import (
	"bytes"
	"fmt"
)

// ColumnType is the typed tag carried by each Value.
type ColumnType uint8

const (
	// TypeInvalid marks the zero Value.
	TypeInvalid ColumnType = 0
	// TypeInt64 is a signed 64-bit integer.
	TypeInt64 ColumnType = 1
	// TypeFloat64 is an IEEE-754 double.
	TypeFloat64 ColumnType = 2
	// TypeString is a UTF-8 string.
	TypeString ColumnType = 3
	// TypeBytes is an opaque byte slice.
	TypeBytes ColumnType = 4
	// TypeBool is a boolean.
	TypeBool ColumnType = 5
)

func (t ColumnType) String() string {
	switch t {
	case TypeInt64:
		return "int64"
	case TypeFloat64:
		return "float64"
	case TypeString:
		return "string"
	case TypeBytes:
		return "bytes"
	case TypeBool:
		return "bool"
	}
	return "invalid"
}

// IsNumeric reports whether a column of this type may be used as the indexed
// column of a set.
func (t ColumnType) IsNumeric() bool {
	return t == TypeInt64
}

// Value is a typed column value. The zero Value has type TypeInvalid.
type Value struct {
	t ColumnType
	i int64
	f float64
	b []byte
}

// Type returns the tag of this value.
func (v Value) Type() ColumnType { return v.t }

// IsZero reports whether v is the zero Value.
func (v Value) IsZero() bool { return v.t == TypeInvalid }

// AsInt returns the int64 and true iff the value is TypeInt64.
func (v Value) AsInt() (int64, bool) { return v.i, v.t == TypeInt64 }

// AsFloat returns the float64 and true iff the value is TypeFloat64.
func (v Value) AsFloat() (float64, bool) { return v.f, v.t == TypeFloat64 }

// AsString returns the string and true iff the value is TypeString.
func (v Value) AsString() (string, bool) {
	if v.t != TypeString {
		return "", false
	}
	return string(v.b), true
}

// AsBytes returns the raw byte slice and true iff the value is TypeBytes.
// The returned slice is owned by the caller after Get/Scan/Query decode.
func (v Value) AsBytes() ([]byte, bool) {
	if v.t != TypeBytes {
		return nil, false
	}
	return v.b, true
}

// AsBool returns the bool and true iff the value is TypeBool. For any
// non-bool value the returned bool is always false to avoid a convincing
// "true" slipping past a caller that forgets to check the ok flag.
func (v Value) AsBool() (bool, bool) {
	if v.t != TypeBool {
		return false, false
	}
	return v.i != 0, true
}

// String renders the value for debugging. It is deliberately not the same as
// AsString; it always succeeds and never signals a type mismatch. String
// satisfies fmt.Stringer so `%v` formats a Value usefully out of the box.
func (v Value) String() string {
	switch v.t {
	case TypeInt64:
		return fmt.Sprintf("Int(%d)", v.i)
	case TypeFloat64:
		return fmt.Sprintf("Float(%g)", v.f)
	case TypeString:
		return fmt.Sprintf("Str(%q)", string(v.b))
	case TypeBytes:
		return fmt.Sprintf("Bytes(%d bytes)", len(v.b))
	case TypeBool:
		return fmt.Sprintf("Bool(%t)", v.i != 0)
	}
	return "Invalid"
}

// Int returns an int64-typed Value.
func Int(v int64) Value { return Value{t: TypeInt64, i: v} }

// Float returns a float64-typed Value.
func Float(v float64) Value { return Value{t: TypeFloat64, f: v} }

// Str returns a string-typed Value. The underlying bytes are copied.
func Str(s string) Value { return Value{t: TypeString, b: []byte(s)} }

// BytesV returns a bytes-typed Value. The slice is NOT copied into the
// Value itself, but the encode path does copy the bytes into Pebble's
// write batch before Put / Update returns, so the caller is free to
// mutate or reuse the slice as soon as the store call returns.
//
// The caller MUST NOT mutate the slice concurrently with an in-flight
// Put / Update on the same Value — there is no synchronisation between
// the caller's goroutine and the encode goroutine.
func BytesV(b []byte) Value { return Value{t: TypeBytes, b: b} }

// BoolV returns a bool-typed Value.
func BoolV(b bool) Value {
	var i int64
	if b {
		i = 1
	}
	return Value{t: TypeBool, i: i}
}

// Row is a sparse map of column name to value. Row is not goroutine safe and
// callers should treat rows returned from the DB as owned by the caller.
type Row map[string]Value

// Clone returns a shallow copy of the row. Byte/string column contents are
// shared, which is safe because Value is immutable from the caller's
// perspective.
func (r Row) Clone() Row {
	if r == nil {
		return nil
	}
	out := make(Row, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

// cmpValues returns -1/0/1 when a<b/a==b/a>b and ok=true when the types are
// comparable. Types must match exactly; there is no implicit conversion.
func cmpValues(a, b Value) (int, bool) {
	if a.t != b.t {
		return 0, false
	}
	switch a.t {
	case TypeInt64, TypeBool:
		switch {
		case a.i < b.i:
			return -1, true
		case a.i > b.i:
			return 1, true
		}
		return 0, true
	case TypeFloat64:
		switch {
		case a.f < b.f:
			return -1, true
		case a.f > b.f:
			return 1, true
		}
		return 0, true
	case TypeString, TypeBytes:
		return bytes.Compare(a.b, b.b), true
	}
	return 0, false
}
