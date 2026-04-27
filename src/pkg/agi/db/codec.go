package db

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
)

// Wire format of an encoded row:
//
//   [uvarint numCols]
//   repeated numCols times, sorted ascending by colID:
//     [uvarint colID]
//     [uint8   type]
//     [uvarint payloadLen]
//     [payloadLen bytes of payload]
//
// Payload by type:
//
//   TypeInt64   => varint-encoded (zigzag) signed 64-bit integer
//   TypeFloat64 => 8 little-endian bytes of math.Float64bits
//   TypeString  => raw UTF-8 bytes
//   TypeBytes   => raw bytes
//   TypeBool    => exactly one byte, 0x00 or 0x01
//
// Sorting by colID keeps decode deterministic and makes it easy to jump over
// unwanted columns without decoding them: readers only need to read colID,
// type, payloadLen and advance past payloadLen bytes.

// codecEntry is a minimal internal representation used when encoding.
type codecEntry struct {
	ColID uint32
	Typ   ColumnType
	Val   Value
}

// encodeRow serializes the given columns into the on-disk row format.
// The entries slice is treated as owned by this function and will be
// sorted in place by colID; callers that want to keep their slice in
// a stable order should pass a copy.
//
// Payload emission is inlined per type: the old encodePayload helper
// produced intermediate byte slices for int64/float64/bool payloads
// which encodeRow then copied into `out`, paying one extra allocation
// per fixed-size column on the Put hot path. The switch below appends
// directly into `out` for those cases; string/bytes still alias the
// caller's buffer (payload data is written straight into out).
func encodeRow(entries []codecEntry) ([]byte, error) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].ColID < entries[j].ColID })
	// Cheap upper-bound sizing.
	est := binary.MaxVarintLen64
	for _, e := range entries {
		est += binary.MaxVarintLen64 + 1 + binary.MaxVarintLen64
		switch e.Typ {
		case TypeInt64:
			est += binary.MaxVarintLen64
		case TypeFloat64:
			est += 8
		case TypeString, TypeBytes:
			est += len(e.Val.b)
		case TypeBool:
			est++
		}
	}
	out := make([]byte, 0, est)
	out = binary.AppendUvarint(out, uint64(len(entries)))
	for _, e := range entries {
		if e.Val.t != e.Typ {
			return nil, fmt.Errorf("db: column type %s does not match value type %s", e.Typ, e.Val.t)
		}
		out = binary.AppendUvarint(out, uint64(e.ColID))
		out = append(out, byte(e.Typ))
		switch e.Typ {
		case TypeInt64:
			// Encode the varint into a stack-allocated scratch so we
			// can write the length prefix before the payload without
			// heap-allocating an intermediate slice.
			var scratch [binary.MaxVarintLen64]byte
			nPay := binary.PutVarint(scratch[:], e.Val.i)
			out = binary.AppendUvarint(out, uint64(nPay))
			out = append(out, scratch[:nPay]...)
		case TypeFloat64:
			out = binary.AppendUvarint(out, 8)
			out = binary.LittleEndian.AppendUint64(out, math.Float64bits(e.Val.f))
		case TypeString, TypeBytes:
			out = binary.AppendUvarint(out, uint64(len(e.Val.b)))
			out = append(out, e.Val.b...)
		case TypeBool:
			out = binary.AppendUvarint(out, 1)
			if e.Val.i != 0 {
				out = append(out, 1)
			} else {
				out = append(out, 0)
			}
		default:
			return nil, fmt.Errorf("db: unknown column type %d", e.Typ)
		}
	}
	return out, nil
}

// decodedRow is the per-iterator scratch buffer used by the scan/query
// hot path. It replaces the older map[uint32]Value with a flat
// []Value indexed by colID and a parallel presence bitmap. Two wins:
//
//  1. Reads are an array index + bit test; writes are an array store + a
//     bit set. Both eliminate the ~20 ns/op map hash that used to
//     dominate per-row work for the AGI 2.27 M-row dashboard scan.
//  2. "Presence only" columns can be marked via markPresent without
//     paying decodePayload — exactly what existsExpr needs.
//
// The buffer is allocated once at iterator init from a snapshot of
// s.NextColID and reset between rows; values are left in place and
// simply re-overwritten or hidden by the cleared presence bitmap.
type decodedRow struct {
	vals    []Value
	present []uint64
}

func (r *decodedRow) resize(bufCap uint32) {
	if uint32(len(r.vals)) < bufCap {
		r.vals = make([]Value, bufCap)
	}
	n := int((bufCap + 63) >> 6)
	if len(r.present) < n {
		r.present = make([]uint64, n)
	}
}

// Reset clears the presence bitmap. Values are left in place; callers
// must not consult vals[id] without first checking the bit (which is
// what Get/Has do).
func (r *decodedRow) Reset() {
	for i := range r.present {
		r.present[i] = 0
	}
}

// Set marks colID present and stores its decoded Value. Caller must
// have sized the buffer (resize) so that id is in range.
func (r *decodedRow) Set(id uint32, v Value) {
	r.vals[id] = v
	r.present[id>>6] |= 1 << (id & 63)
}

// markPresent records that colID exists on the row without populating a
// Value. Used by the codec for presence-only filter columns. A
// subsequent Get(id) will return (zero Value, true), which is exactly
// what existsExpr.eval consumes.
func (r *decodedRow) markPresent(id uint32) {
	r.present[id>>6] |= 1 << (id & 63)
}

// Get reads the Value at id, returning ok=false when the column was
// not present on the row. ok=true with a zero Value means the column
// was marked presence-only by the plan; callers must not interpret the
// zero Value as a real payload.
func (r *decodedRow) Get(id uint32) (Value, bool) {
	w := int(id >> 6)
	if w >= len(r.present) {
		return Value{}, false
	}
	if r.present[w]&(1<<(id&63)) == 0 {
		return Value{}, false
	}
	return r.vals[id], true
}

// Has reports whether colID is present on the row.
func (r *decodedRow) Has(id uint32) bool {
	w := int(id >> 6)
	if w >= len(r.present) {
		return false
	}
	return r.present[w]&(1<<(id&63)) != 0
}

// decodeMask drives decodeRow's inner loop. It is built once per plan
// and shared across every row decoded by that iterator. Two parallel
// bitmaps separate "decode the value" columns from "mark presence
// only" columns; decodeAll bypasses both bitmaps and decodes every
// column on the row up to bufCap.
type decodeMask struct {
	decodeAll      bool
	wantValueBM    []uint64
	wantPresenceBM []uint64
	maxWantID      uint32
	bufCap         uint32
	// numWant is the total number of distinct colIDs in the union of
	// the value and presence sets. Used to short-circuit decode once
	// every wanted column has been seen on the row.
	numWant int
}

func bitmapSet(bm []uint64, id uint32) {
	bm[id>>6] |= 1 << (id & 63)
}

func bitmapTest(bm []uint64, id uint32) bool {
	w := int(id >> 6)
	if w >= len(bm) {
		return false
	}
	return bm[w]&(1<<(id&63)) != 0
}

// buildDecodeMask constructs a mask from sorted valueIDs and
// presenceIDs slices. bufCap is the iterator's NextColID snapshot.
// Callers must dedupe presenceIDs against valueIDs before calling.
func buildDecodeMask(valueIDs, presenceIDs []uint32, bufCap uint32) decodeMask {
	m := decodeMask{bufCap: bufCap}
	if len(valueIDs) == 0 && len(presenceIDs) == 0 {
		return m
	}
	cap := bufCap
	for _, id := range valueIDs {
		if id+1 > cap {
			cap = id + 1
		}
	}
	for _, id := range presenceIDs {
		if id+1 > cap {
			cap = id + 1
		}
	}
	words := int((cap + 63) >> 6)
	if len(valueIDs) > 0 {
		m.wantValueBM = make([]uint64, words)
		for _, id := range valueIDs {
			bitmapSet(m.wantValueBM, id)
			if id > m.maxWantID {
				m.maxWantID = id
			}
		}
	}
	if len(presenceIDs) > 0 {
		m.wantPresenceBM = make([]uint64, words)
		for _, id := range presenceIDs {
			bitmapSet(m.wantPresenceBM, id)
			if id > m.maxWantID {
				m.maxWantID = id
			}
		}
	}
	m.numWant = len(valueIDs) + len(presenceIDs)
	return m
}

// decodeAllMask returns a mask that decodes every column on the row up
// to bufCap. Columns whose IDs landed past bufCap (theoretically
// possible if a writer raced our schema-snapshot under the s.mu
// release / pebble snapshot pin window) are skipped silently — they
// cannot appear in any plan's outNames and the iterator's caller will
// see the same shape it would have without the race.
func decodeAllMask(bufCap uint32) decodeMask {
	return decodeMask{decodeAll: true, bufCap: bufCap}
}

// decodeRow walks raw, populating out for every colID in mask.
// valueIDs are decoded via decodePayload; presenceIDs only get their
// presence bit set. The maxWantID short-circuit (rows are encoded in
// ascending colID order) is preserved against the union of both sets.
//
// out must be sized via decodedRow.resize(mask.bufCap) before the
// first call; callers Reset() between rows to clear the presence
// bitmap. Values left over from prior rows are inert because Get/Has
// gate every read on the bitmap.
func decodeRow(raw []byte, mask *decodeMask, out *decodedRow) error {
	p := raw
	numCols, n := binary.Uvarint(p)
	if n <= 0 {
		return errors.New("db: bad row header")
	}
	p = p[n:]
	if !mask.decodeAll && mask.numWant == 0 {
		return nil
	}
	decoded := 0
	for i := uint64(0); i < numCols; i++ {
		colIDu, n := binary.Uvarint(p)
		if n <= 0 {
			return errors.New("db: bad colID")
		}
		p = p[n:]
		if len(p) < 1 {
			return errors.New("db: truncated after colID")
		}
		typ := ColumnType(p[0])
		p = p[1:]
		sz, n := binary.Uvarint(p)
		if n <= 0 {
			return errors.New("db: bad payload len")
		}
		p = p[n:]
		if uint64(len(p)) < sz {
			return errors.New("db: truncated payload")
		}
		payload := p[:sz]
		p = p[sz:]
		colID := uint32(colIDu)
		if mask.decodeAll {
			if colID >= mask.bufCap {
				continue
			}
			v, err := decodePayload(typ, payload)
			if err != nil {
				return err
			}
			out.Set(colID, v)
			continue
		}
		if colID > mask.maxWantID {
			return nil
		}
		if bitmapTest(mask.wantValueBM, colID) {
			v, err := decodePayload(typ, payload)
			if err != nil {
				return err
			}
			out.Set(colID, v)
			decoded++
			if decoded == mask.numWant {
				return nil
			}
			continue
		}
		if bitmapTest(mask.wantPresenceBM, colID) {
			out.markPresent(colID)
			decoded++
			if decoded == mask.numWant {
				return nil
			}
		}
	}
	return nil
}

// decodeWanted walks the row, decoding only columns whose IDs appear in
// wantIDs. If wantIDs is nil, every column is decoded. Results are written
// into out keyed by colID. Callers are expected to clear out before reuse.
//
// Rows are serialized in ascending colID order (see encodeRow), so once we
// have decoded every wanted column we can stop without touching the rest
// of the payload. This is a measurable win on narrow projections over
// wide rows.
//
// Used by the Get / RMW paths in crud.go which are not on the iterator
// hot path; iterators use decodeRow + decodedRow instead.
func decodeWanted(raw []byte, wantIDs map[uint32]struct{}, out map[uint32]Value) error {
	// Explicit "no columns wanted" guard: avoids relying on the implicit
	// maxWantID==0 short-circuit below, which only happens to be correct
	// because valid colIDs start at 0. A non-nil, empty wantIDs means the
	// caller asked for nothing; we have nothing to do.
	if wantIDs != nil && len(wantIDs) == 0 {
		return nil
	}
	p := raw
	numCols, n := binary.Uvarint(p)
	if n <= 0 {
		return errors.New("db: bad row header")
	}
	p = p[n:]
	want := len(wantIDs)
	decoded := 0
	// maxWantID is the highest colID in wantIDs; once the row cursor has
	// advanced past it we know no further wanted column can appear.
	var maxWantID uint32
	for id := range wantIDs {
		if id > maxWantID {
			maxWantID = id
		}
	}
	for i := uint64(0); i < numCols; i++ {
		colIDu, n := binary.Uvarint(p)
		if n <= 0 {
			return errors.New("db: bad colID")
		}
		p = p[n:]
		if len(p) < 1 {
			return errors.New("db: truncated after colID")
		}
		typ := ColumnType(p[0])
		p = p[1:]
		sz, n := binary.Uvarint(p)
		if n <= 0 {
			return errors.New("db: bad payload len")
		}
		p = p[n:]
		if uint64(len(p)) < sz {
			return errors.New("db: truncated payload")
		}
		payload := p[:sz]
		p = p[sz:]
		colID := uint32(colIDu)
		if wantIDs != nil {
			if _, ok := wantIDs[colID]; !ok {
				if colID > maxWantID {
					// Past every wanted ID; no point scanning further.
					return nil
				}
				continue
			}
		}
		val, err := decodePayload(typ, payload)
		if err != nil {
			return err
		}
		out[colID] = val
		if wantIDs != nil {
			decoded++
			if decoded == want {
				return nil
			}
		}
	}
	return nil
}

// decodeSingle fetches the value of a single column. It stops scanning as
// soon as the target colID is observed. Returns (zero, false, nil) if the
// column is absent.
func decodeSingle(raw []byte, colID uint32) (Value, bool, error) {
	p := raw
	numCols, n := binary.Uvarint(p)
	if n <= 0 {
		return Value{}, false, errors.New("db: bad row header")
	}
	p = p[n:]
	for i := uint64(0); i < numCols; i++ {
		cidU, n := binary.Uvarint(p)
		if n <= 0 {
			return Value{}, false, errors.New("db: bad colID")
		}
		p = p[n:]
		if len(p) < 1 {
			return Value{}, false, errors.New("db: truncated after colID")
		}
		typ := ColumnType(p[0])
		p = p[1:]
		sz, n := binary.Uvarint(p)
		if n <= 0 {
			return Value{}, false, errors.New("db: bad payload len")
		}
		p = p[n:]
		if uint64(len(p)) < sz {
			return Value{}, false, errors.New("db: truncated payload")
		}
		payload := p[:sz]
		p = p[sz:]
		cid := uint32(cidU)
		if cid == colID {
			v, err := decodePayload(typ, payload)
			if err != nil {
				return Value{}, false, err
			}
			return v, true, nil
		}
		// Rows are sorted ascending by colID; once we pass it we can bail.
		if cid > colID {
			return Value{}, false, nil
		}
	}
	return Value{}, false, nil
}

// decodeRowAll decodes every column and returns the raw colID-keyed map.
// Used by internal paths that need to inspect the full row.
func decodeRowAll(raw []byte) (map[uint32]Value, error) {
	out := make(map[uint32]Value)
	if err := decodeWanted(raw, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

func decodePayload(typ ColumnType, payload []byte) (Value, error) {
	switch typ {
	case TypeInt64:
		iv, n := binary.Varint(payload)
		if n <= 0 {
			return Value{}, errors.New("db: bad int payload")
		}
		return Int(iv), nil
	case TypeFloat64:
		if len(payload) < 8 {
			return Value{}, errors.New("db: short float payload")
		}
		return Float(math.Float64frombits(binary.LittleEndian.Uint64(payload))), nil
	case TypeString:
		// Copy so the caller's Value is not aliased to the Pebble value
		// buffer (which is only valid until the iterator advances).
		b := make([]byte, len(payload))
		copy(b, payload)
		return Value{t: TypeString, b: b}, nil
	case TypeBytes:
		b := make([]byte, len(payload))
		copy(b, payload)
		return Value{t: TypeBytes, b: b}, nil
	case TypeBool:
		if len(payload) < 1 {
			return Value{}, errors.New("db: short bool payload")
		}
		// Accept only the two valid encodings emitted by encodePayload.
		// A byte other than 0x00 / 0x01 means the wire bytes are bogus
		// (or produced by an older / buggy writer) and we'd rather
		// surface that than silently coerce to true.
		switch payload[0] {
		case 0:
			return BoolV(false), nil
		case 1:
			return BoolV(true), nil
		default:
			return Value{}, fmt.Errorf("db: bad bool payload byte 0x%02x", payload[0])
		}
	}
	return Value{}, fmt.Errorf("db: unknown column type %d", typ)
}
