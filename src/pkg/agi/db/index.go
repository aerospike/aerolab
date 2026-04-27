package db

import (
	"errors"

	"github.com/cockroachdb/pebble/v2"
)

// indexedColumnSnapshot reads (colID, hasIndex) from the set's schema
// under s.mu. Callers that already hold s.mu should use s.indexedColumn()
// directly.
func (d *DB) indexedColumnSnapshot(s *setSchema) (colID uint32, hasIndex bool) {
	s.mu.RLock()
	colID, hasIndex = s.indexedColumn()
	s.mu.RUnlock()
	return colID, hasIndex
}

// readOldIndexedValue probes the existing data record at dkey to discover
// both whether the row exists and, if the set has an indexed column, what
// the old value of that column is. The caller supplies hasIndex/colID
// (already resolved under s.mu) so this function does not need to
// touch the schema.
//
// On v2 indexed sets, D/ stores an 8-byte forward pointer to the
// covering I/ entry; this function returns the unbiased int64 (which
// is the indexed column's value, by construction) without touching
// the row payload at all. On unindexed sets D/ still holds the row
// payload, but indexed-state lookups never go through this function
// for unindexed sets — the caller passes hasIndex=false and we just
// report rowExists.
//
// Returns:
//
//	oldVal    - the prior value of the indexed column; 0 when not present
//	rowExists - true when a record exists at dkey. This is reported
//	            honestly even when the pointer decode fails, so
//	            callers can still proceed to delete a corrupt row.
//	hasOldVal - true when the row existed and carried a decodable
//	            8-byte index pointer (always true on a non-corrupt
//	            indexed row in v2)
//	oldRow    - the copied on-disk payload (nil if rowExists=false).
//	            For unindexed sets this is the row bytes; for indexed
//	            sets it is the 8-byte pointer (callers don't currently
//	            use it on the indexed path).
//	err       - decode / read error. When err is errCorruptIndexedValue
//	            the row does exist on disk; the caller may ignore the
//	            index side and still delete the data record.
func (d *DB) readOldIndexedValue(dkey []byte, hasIndex bool, colID uint32) (oldVal int64, rowExists, hasOldVal bool, oldRow []byte, err error) {
	d.stats.pebbleGets.Add(1)
	raw, closer, gerr := d.p.Get(dkey)
	if gerr != nil {
		if isNotFound(gerr) {
			return 0, false, false, nil, nil
		}
		return 0, false, false, nil, gerr
	}
	buf := make([]byte, len(raw))
	copy(buf, raw)
	_ = closer.Close()
	if !hasIndex {
		return 0, true, false, buf, nil
	}
	// v2: D/ for an indexed set is an 8-byte biased-ts pointer. A
	// length other than 8 is on-disk corruption (or a leftover v1
	// row, which Open should have refused via the version check).
	biased, ok := decodePointer(buf)
	if !ok {
		return 0, true, false, buf, errCorruptIndexedValue
	}
	return unbiasUint64(biased), true, true, buf, nil
}

// stageIndexWrite places the index delete/insert operations for a single
// primary key into the batch. oldExists indicates whether an old index
// entry must be deleted. newHasIndexed indicates whether the new row has
// the indexed column and a new entry should be inserted; payload is
// the full row bytes that get clustered at the index key (covering
// index, see README.md storage layout). When newHasIndexed is true
// payload MUST be non-nil; the caller always has the encoded row
// available at index-write time.
func stageIndexWrite(batch *pebble.Batch, setID, colID uint32, pk string, oldExists bool, oldVal int64, newHasIndexed bool, newVal int64, payload []byte) error {
	if oldExists {
		if err := batch.Delete(indexKey(setID, colID, oldVal, pk), nil); err != nil {
			return err
		}
	}
	if newHasIndexed {
		if err := batch.Set(indexKey(setID, colID, newVal, pk), payload, nil); err != nil {
			return err
		}
	}
	return nil
}

// errCorruptIndexedValue is returned when a row's indexed column payload
// (on v2 indexed sets, the 8-byte D/ pointer) does not decode cleanly.
// On v2 indexed sets this fires when D/ is the wrong length; on
// unindexed sets it cannot fire (caller passes hasIndex=false).
var errCorruptIndexedValue = errors.New("db: indexed-row D/ pointer is malformed")

// isNotFound reports whether err indicates a missing key in Pebble.
func isNotFound(err error) bool {
	return errors.Is(err, pebble.ErrNotFound)
}
