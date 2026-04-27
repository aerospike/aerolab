package db

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

// ColumnSpec describes a column in a set. A spec supplies the column's name,
// its value type, and whether it is the indexed column for the set.
//
// Indexing model (design trade-off, not a TODO):
//
//	Only one indexed column per set is allowed and it must be numeric
//	(TypeInt64). This is deliberate and encoded end-to-end:
//	  - indexKey() bakes a single colID into the key layout.
//	  - setSchema.IndexedCol is a string, not a set.
//	  - DropSet / DropColumn delete by the single-column index range.
//	Supporting N indexed columns would require a new index key layout,
//	migration code, a per-column range iterator in Query.Between, and
//	cross-column plan selection in QueryBuilder.Run. None of that is
//	"cheap" — the single-index constraint matches the AGI plugin's
//	actual usage (a single timestamp column) and is left as-is.
type ColumnSpec struct {
	Name    string
	Type    ColumnType
	Indexed bool
}

// columnInfo is the runtime entry for a registered column.
type columnInfo struct {
	ID   uint32
	Type ColumnType
}

// setSchema is the per-set metadata cached in memory and persisted to the
// M/schema/ sub-namespace.
//
// Concurrency: the immutable fields (ID, Name) may be read lock-free by
// any holder of the pointer. The mutable fields (NextColID, Columns,
// ByID, IndexedCol) are guarded by mu, so implicit column registration
// on one set does not serialize writes on every other set. The dropped
// flag is atomic and is only ever flipped from false to true by
// DropSet; it is checked under a row lock by every mutator so that a
// concurrent drop cannot leave orphan data under a retired setID.
type setSchema struct {
	mu         sync.RWMutex
	ID         uint32
	Name       string
	NextColID  uint32
	Columns    map[string]columnInfo // by name
	ByID       map[uint32]string     // reverse lookup
	IndexedCol string                // empty if none
	dropped    atomic.Bool
}

// persistedSchema is the JSON form of setSchema.
type persistedSchema struct {
	ID         uint32              `json:"id"`
	Name       string              `json:"name"`
	NextColID  uint32              `json:"nextColID"`
	Cols       []persistedColumn   `json:"cols"`
	IndexedCol string              `json:"indexedCol,omitempty"`
}

type persistedColumn struct {
	ID   uint32     `json:"id"`
	Name string     `json:"name"`
	Type ColumnType `json:"type"`
}

func (s *setSchema) toPersisted() *persistedSchema {
	p := &persistedSchema{
		ID:         s.ID,
		Name:       s.Name,
		NextColID:  s.NextColID,
		IndexedCol: s.IndexedCol,
	}
	for name, info := range s.Columns {
		p.Cols = append(p.Cols, persistedColumn{ID: info.ID, Name: name, Type: info.Type})
	}
	// Sort by colID so the serialized JSON is deterministic. Without
	// this, every implicit column registration rewrites the schema
	// record with a different byte sequence (map iteration order) and
	// bloats the LSM.
	sort.Slice(p.Cols, func(i, j int) bool { return p.Cols[i].ID < p.Cols[j].ID })
	return p
}

// fromPersisted reconstitutes an in-memory setSchema from its JSON form.
// If the persisted IndexedCol refers to a column that is not in the
// serialized column list (corruption / partial restore), the returned
// warnings slice describes the repair(s) performed and IndexedCol is
// silently cleared so callers cannot observe an indexed-but-absent
// column. An empty warnings slice means the record decoded cleanly.
func fromPersisted(p *persistedSchema) (*setSchema, []string) {
	s := &setSchema{
		ID:         p.ID,
		Name:       p.Name,
		NextColID:  p.NextColID,
		Columns:    make(map[string]columnInfo, len(p.Cols)),
		ByID:       make(map[uint32]string, len(p.Cols)),
		IndexedCol: p.IndexedCol,
	}
	for _, c := range p.Cols {
		s.Columns[c.Name] = columnInfo{ID: c.ID, Type: c.Type}
		s.ByID[c.ID] = c.Name
	}
	var warnings []string
	if s.IndexedCol != "" {
		if _, present := s.Columns[s.IndexedCol]; !present {
			warnings = append(warnings, fmt.Sprintf("set %q (id=%d): persisted IndexedCol=%q is not in column list; clearing", s.Name, s.ID, s.IndexedCol))
			s.IndexedCol = ""
		}
	}
	return s, warnings
}

// encodeSchema returns the byte payload stored at metaSchemaKey(setID).
func encodeSchema(s *setSchema) ([]byte, error) {
	return json.Marshal(s.toPersisted())
}

// decodeSchema parses a persisted schema record. The warnings slice, if
// non-empty, describes repairs applied to internally-inconsistent fields
// (e.g. an IndexedCol referring to a column that is not in the Cols
// list); callers are expected to log them.
func decodeSchema(raw []byte) (*setSchema, []string, error) {
	var p persistedSchema
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, nil, fmt.Errorf("db: decode schema: %w", err)
	}
	s, warnings := fromPersisted(&p)
	return s, warnings, nil
}

// schemaCheckpoint is a cheap snapshot of setSchema's mutable fields.
// RegisterSet and prepareEntries take one before mutating the schema so
// that a subsequent disk-persist failure can be rolled back, keeping the
// in-memory caches and the on-disk record in sync.
type schemaCheckpoint struct {
	columns    map[string]columnInfo
	byID       map[uint32]string
	nextColID  uint32
	indexedCol string
}

func (s *setSchema) checkpoint() schemaCheckpoint {
	cp := schemaCheckpoint{
		nextColID:  s.NextColID,
		indexedCol: s.IndexedCol,
		columns:    make(map[string]columnInfo, len(s.Columns)),
		byID:       make(map[uint32]string, len(s.ByID)),
	}
	for k, v := range s.Columns {
		cp.columns[k] = v
	}
	for k, v := range s.ByID {
		cp.byID[k] = v
	}
	return cp
}

func (s *setSchema) restore(cp schemaCheckpoint) {
	s.Columns = cp.columns
	s.ByID = cp.byID
	s.NextColID = cp.nextColID
	s.IndexedCol = cp.indexedCol
}

// addColumn adds (or asserts) a column on the schema. If the column already
// exists with a different type, addColumn returns an error. If it is new,
// the next colID is assigned and changed=true is returned to signal that the
// schema needs to be persisted.
func (s *setSchema) addColumn(name string, typ ColumnType, indexed bool) (colID uint32, changed bool, err error) {
	if info, ok := s.Columns[name]; ok {
		if info.Type != typ {
			return 0, false, fmt.Errorf("%w: column %q in set %q has type %s, cannot store %s", ErrColumnTypeConflict, name, s.Name, info.Type, typ)
		}
		if indexed {
			switch s.IndexedCol {
			case name:
				// already the indexed column; nothing to do.
			case "":
				// Promoting an existing non-indexed column to indexed
				// would require back-filling every previously-written
				// row's index entry, which this package does not
				// support. Reject explicitly rather than produce the
				// misleading "already indexed on \"\"" message.
				return 0, false, fmt.Errorf("db: column %q in set %q already exists as non-indexed; cannot promote to indexed (no back-fill)", name, s.Name)
			default:
				return 0, false, fmt.Errorf("db: set %q already indexed on %q, cannot re-index on %q", s.Name, s.IndexedCol, name)
			}
		}
		return info.ID, false, nil
	}
	if indexed {
		if s.IndexedCol != "" {
			return 0, false, fmt.Errorf("db: set %q already indexed on %q", s.Name, s.IndexedCol)
		}
		if !typ.IsNumeric() {
			return 0, false, fmt.Errorf("db: indexed column %q must be numeric (int64), got %s", name, typ)
		}
		s.IndexedCol = name
	}
	id := s.NextColID
	s.NextColID++
	s.Columns[name] = columnInfo{ID: id, Type: typ}
	s.ByID[id] = name
	return id, true, nil
}

// indexedColumn returns the indexed colID (or 0, false) for the set.
func (s *setSchema) indexedColumn() (uint32, bool) {
	if s.IndexedCol == "" {
		return 0, false
	}
	info, ok := s.Columns[s.IndexedCol]
	if !ok {
		return 0, false
	}
	return info.ID, true
}
