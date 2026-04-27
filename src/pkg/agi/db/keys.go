package db

import (
	"encoding/binary"
)

// Key-space prefixes. All DB keys are prefixed with exactly one of these
// single bytes so scans can use tight bounds and there is no risk of
// collision between data, index, and metadata keys.
const (
	prefixMeta  byte = 'M'
	prefixData  byte = 'D'
	prefixIndex byte = 'I'
)

// Metadata sub-namespaces (appear after prefixMeta).
const (
	metaSchemaPrefix = "schema/"  // M/schema/<setID-be4> => JSON
	metaSetNamePref  = "setname/" // legacy; written by old builds, purged on Open, never read
	metaNextSetIDKey = "nextSetID"
	metaVersionKeyS  = "version"
)

// biasInt64 maps signed int64 to unsigned u64 such that the lexicographic
// order of the resulting big-endian bytes matches the numeric order of the
// original int64. Used for index keys so Pebble's byte-ordered iterator
// streams records in timestamp order.
func biasInt64(v int64) uint64 {
	return uint64(v) ^ (1 << 63)
}

func unbiasUint64(u uint64) int64 {
	return int64(u ^ (1 << 63))
}

// dataKey encodes the primary data key for (setID, pk).
// Layout: 'D' | be4(setID) | pk-bytes
func dataKey(setID uint32, pk string) []byte {
	out := make([]byte, 5+len(pk))
	out[0] = prefixData
	binary.BigEndian.PutUint32(out[1:5], setID)
	copy(out[5:], pk)
	return out
}

// dataLowerBound returns the smallest possible data key for the set.
func dataLowerBound(setID uint32) []byte {
	out := make([]byte, 5)
	out[0] = prefixData
	binary.BigEndian.PutUint32(out[1:5], setID)
	return out
}

// dataUpperBound returns the first data key outside the set. Pebble's
// IterOptions.UpperBound is exclusive.
func dataUpperBound(setID uint32) []byte {
	out := make([]byte, 5)
	out[0] = prefixData
	binary.BigEndian.PutUint32(out[1:5], setID+1)
	// If setID+1 wraps to 0 we bump the prefix byte so the upper bound
	// remains strictly larger than any key in setID=math.MaxUint32.
	if setID == ^uint32(0) {
		out[0] = prefixData + 1
		binary.BigEndian.PutUint32(out[1:5], 0)
	}
	return out
}

// parseDataKey extracts setID and pk from a data key. Returns ok=false if
// the key is not a data key.
func parseDataKey(key []byte) (setID uint32, pk string, ok bool) {
	if len(key) < 5 || key[0] != prefixData {
		return 0, "", false
	}
	setID = binary.BigEndian.Uint32(key[1:5])
	pk = string(key[5:])
	return setID, pk, true
}

// parseDataKeyBytes is the zero-copy variant of parseDataKey. pkBytes is
// a slice INTO the input key; it must not be retained past the next
// Pebble iterator advance / Close. Callers that need a stable value
// must copy it with string(pkBytes) at yield time.
func parseDataKeyBytes(key []byte) (setID uint32, pkBytes []byte, ok bool) {
	if len(key) < 5 || key[0] != prefixData {
		return 0, nil, false
	}
	setID = binary.BigEndian.Uint32(key[1:5])
	pkBytes = key[5:]
	return setID, pkBytes, true
}

// indexKey encodes a numeric range-index entry.
// Layout: 'I' | be4(setID) | be4(colID) | be8(biased-int64) | pk-bytes
func indexKey(setID, colID uint32, v int64, pk string) []byte {
	out := make([]byte, 17+len(pk))
	out[0] = prefixIndex
	binary.BigEndian.PutUint32(out[1:5], setID)
	binary.BigEndian.PutUint32(out[5:9], colID)
	binary.BigEndian.PutUint64(out[9:17], biasInt64(v))
	copy(out[17:], pk)
	return out
}

// indexRangeBounds returns [lower, upper) suitable for Pebble IterOptions.
// The range covers all index entries for (setID, colID) whose value is in
// [lo, hi]. Lo and hi are inclusive.
func indexRangeBounds(setID, colID uint32, lo, hi int64) (lower, upper []byte) {
	lower = make([]byte, 17)
	lower[0] = prefixIndex
	binary.BigEndian.PutUint32(lower[1:5], setID)
	binary.BigEndian.PutUint32(lower[5:9], colID)
	binary.BigEndian.PutUint64(lower[9:17], biasInt64(lo))

	upper = make([]byte, 17)
	upper[0] = prefixIndex
	binary.BigEndian.PutUint32(upper[1:5], setID)
	binary.BigEndian.PutUint32(upper[5:9], colID)
	// Upper bound is exclusive. hi is inclusive. Adding 1 to the biased
	// uint64 value works for every hi except math.MaxInt64, where we bump
	// the colID / setID / prefix byte (in that order) so the bound still
	// falls strictly after every matching key even at the edges of the
	// uint32 / byte domains.
	biasedHi := biasInt64(hi)
	if biasedHi != ^uint64(0) {
		binary.BigEndian.PutUint64(upper[9:17], biasedHi+1)
		return lower, upper
	}
	// biasedHi is max: need to step the colID byte(s) instead.
	binary.BigEndian.PutUint64(upper[9:17], 0)
	if colID != ^uint32(0) {
		binary.BigEndian.PutUint32(upper[5:9], colID+1)
		return lower, upper
	}
	// colID is also max: step the setID.
	binary.BigEndian.PutUint32(upper[5:9], 0)
	if setID != ^uint32(0) {
		binary.BigEndian.PutUint32(upper[1:5], setID+1)
		return lower, upper
	}
	// setID is also max: step the prefix byte and zero the rest.
	binary.BigEndian.PutUint32(upper[1:5], 0)
	upper[0] = prefixIndex + 1
	return lower, upper
}

// parseIndexKey extracts the components of an index key.
func parseIndexKey(key []byte) (setID, colID uint32, val int64, pk string, ok bool) {
	if len(key) < 17 || key[0] != prefixIndex {
		return 0, 0, 0, "", false
	}
	setID = binary.BigEndian.Uint32(key[1:5])
	colID = binary.BigEndian.Uint32(key[5:9])
	val = unbiasUint64(binary.BigEndian.Uint64(key[9:17]))
	pk = string(key[17:])
	return setID, colID, val, pk, true
}

// parseIndexKeyBytes is the zero-copy variant of parseIndexKey. pkBytes
// is a slice INTO the input key; do not retain past the next iterator
// advance / Close.
func parseIndexKeyBytes(key []byte) (setID, colID uint32, val int64, pkBytes []byte, ok bool) {
	if len(key) < 17 || key[0] != prefixIndex {
		return 0, 0, 0, nil, false
	}
	setID = binary.BigEndian.Uint32(key[1:5])
	colID = binary.BigEndian.Uint32(key[5:9])
	val = unbiasUint64(binary.BigEndian.Uint64(key[9:17]))
	pkBytes = key[17:]
	return setID, colID, val, pkBytes, true
}

// encodePointer encodes a biased timestamp (the result of biasInt64 on
// the indexed column's int64 value) as the 8-byte forward-pointer
// payload stored at D/ for indexed sets in storage version 2+. The
// caller passes the already-biased uint64 so the same byte layout
// flows through indexKey() and the D/ pointer payload without a second
// bias.
func encodePointer(biasedTs uint64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, biasedTs)
	return out
}

// decodePointer extracts the biased uint64 timestamp from an 8-byte D/
// pointer payload. Returns ok=false on a malformed (wrong-length)
// payload; callers treat that as a corrupt-row signal symmetric with
// errCorruptIndexedValue.
func decodePointer(p []byte) (biasedTs uint64, ok bool) {
	if len(p) != 8 {
		return 0, false
	}
	return binary.BigEndian.Uint64(p), true
}

// indexKeyBiasedRaw extracts the 8-byte biased-ts uint64 from an index
// key without unbias-ing it. The orphan-skip guard in indexScanIter
// uses this to compare against the D/ pointer's biased ts byte-for-
// byte (no signed-int round-trip).
func indexKeyBiasedRaw(idxKey []byte) uint64 {
	if len(idxKey) < 17 {
		return 0
	}
	return binary.BigEndian.Uint64(idxKey[9:17])
}

// dataKeyBytes is dataKey but takes the pk as []byte (avoiding the
// string copy when the caller already has a byte slice from a Pebble
// iterator). The returned slice is freshly allocated and safe to
// retain.
func dataKeyBytes(setID uint32, pk []byte) []byte {
	out := make([]byte, 5+len(pk))
	out[0] = prefixData
	binary.BigEndian.PutUint32(out[1:5], setID)
	copy(out[5:], pk)
	return out
}

// indexKeyFromBiased is indexKey but takes the already-biased uint64
// timestamp. Used on the read path when we have the bias bytes from
// a D/ pointer and want to avoid an unbias / re-bias round trip.
func indexKeyFromBiased(setID, colID uint32, biasedTs uint64, pk string) []byte {
	out := make([]byte, 17+len(pk))
	out[0] = prefixIndex
	binary.BigEndian.PutUint32(out[1:5], setID)
	binary.BigEndian.PutUint32(out[5:9], colID)
	binary.BigEndian.PutUint64(out[9:17], biasedTs)
	copy(out[17:], pk)
	return out
}

// dataKeyFromIndexKey produces the corresponding data key ('D' | be4(setID)
// | pk) directly from an index key ('I' | be4(setID) | be4(colID) |
// be8(biased-val) | pk), avoiding the intermediate string copy that
// parseIndexKey + dataKey would otherwise incur. The returned byte slice
// is freshly allocated and safe to retain.
func dataKeyFromIndexKey(idxKey []byte) []byte {
	// idxKey layout: I|setID(4)|colID(4)|val(8)|pk
	// data layout:  D|setID(4)|pk
	pkLen := len(idxKey) - 17
	out := make([]byte, 5+pkLen)
	out[0] = prefixData
	copy(out[1:5], idxKey[1:5]) // setID
	copy(out[5:], idxKey[17:])  // pk
	return out
}

// indexPrefix returns the smallest index-key prefix shared by all entries
// for (setID, colID). Useful for wiping a set's index.
func indexPrefix(setID, colID uint32) []byte {
	out := make([]byte, 9)
	out[0] = prefixIndex
	binary.BigEndian.PutUint32(out[1:5], setID)
	binary.BigEndian.PutUint32(out[5:9], colID)
	return out
}

// indexSetLower / indexSetUpper bound every index entry for a set regardless
// of colID.
func indexSetLower(setID uint32) []byte {
	out := make([]byte, 5)
	out[0] = prefixIndex
	binary.BigEndian.PutUint32(out[1:5], setID)
	return out
}

func indexSetUpper(setID uint32) []byte {
	out := make([]byte, 5)
	out[0] = prefixIndex
	binary.BigEndian.PutUint32(out[1:5], setID+1)
	if setID == ^uint32(0) {
		out[0] = prefixIndex + 1
		binary.BigEndian.PutUint32(out[1:5], 0)
	}
	return out
}

// metaSchemaKey returns the storage key for a set's schema JSON.
func metaSchemaKey(setID uint32) []byte {
	out := make([]byte, 1+len(metaSchemaPrefix)+4)
	out[0] = prefixMeta
	copy(out[1:], metaSchemaPrefix)
	binary.BigEndian.PutUint32(out[1+len(metaSchemaPrefix):], setID)
	return out
}

func metaSchemaLower() []byte {
	out := make([]byte, 1+len(metaSchemaPrefix))
	out[0] = prefixMeta
	copy(out[1:], metaSchemaPrefix)
	return out
}

func metaSchemaUpper() []byte {
	b := metaSchemaLower()
	// increment last byte
	i := len(b) - 1
	for i >= 0 {
		if b[i] < 0xff {
			b[i]++
			return b
		}
		b[i] = 0
		i--
	}
	// Unreachable with the current metaSchemaPrefix / prefixMeta byte
	// values: metaSchemaLower() is 'M' + "schema/", none of whose bytes
	// are 0xff, so the increment loop always returns above. Any edit to
	// those constants that lands here would produce an upper bound that
	// sorts strictly below the lower bound (byte-lexicographic: zeros +
	// one more zero is less than the non-zero original), which would
	// silently cause loadSchemas to observe an empty namespace. Loud
	// failure is correct.
	panic("db: metaSchemaPrefix is all 0xff; metaSchemaUpper needs updating to match new prefix constants")
}

// metaNextSetIDKeyBytes is the key that stores the monotonic next-set-id.
func metaNextSetIDKeyBytes() []byte {
	out := make([]byte, 1+len(metaNextSetIDKey))
	out[0] = prefixMeta
	copy(out[1:], metaNextSetIDKey)
	return out
}

// metaVersionKey is the key that stores the storage format version as a
// big-endian uint32. See currentStorageVersion.
func metaVersionKey() []byte {
	out := make([]byte, 1+len(metaVersionKeyS))
	out[0] = prefixMeta
	copy(out[1:], metaVersionKeyS)
	return out
}

// metaSetNameLower / metaSetNameUpper bound the legacy M/setname/ namespace.
// These keys were written by old builds but never read; they are range-
// deleted on Open as a one-shot migration.
func metaSetNameLower() []byte {
	out := make([]byte, 1+len(metaSetNamePref))
	out[0] = prefixMeta
	copy(out[1:], metaSetNamePref)
	return out
}

func metaSetNameUpper() []byte {
	b := metaSetNameLower()
	i := len(b) - 1
	for i >= 0 {
		if b[i] < 0xff {
			b[i]++
			return b
		}
		b[i] = 0
		i--
	}
	panic("db: metaSetNamePref is all 0xff; metaSetNameUpper needs updating to match new prefix constants")
}
