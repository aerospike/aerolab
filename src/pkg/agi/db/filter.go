package db

// Expr is a boolean predicate evaluated against a row. Expressions are
// built by the helpers Eq, In, Between, Exists, Not, And, Or. An Expr is
// safe to use concurrently; it must be constructed from immutable inputs.
type Expr interface {
	// eval is invoked during scans. get must return the column by name,
	// reporting present=false if the column is absent from the row.
	eval(get colGetter) bool
	// columns appends all column names referenced by the expression to
	// dst. Duplicates are acceptable.
	columns(dst []string) []string
}

// colGetter is a lazy accessor that a scan wires up to the decoded (or
// on-demand decoded) row.
type colGetter func(name string) (Value, bool)

// Eq returns a predicate that is true when the named column exists and
// equals v (using the value's native type comparator).
func Eq(col string, v Value) Expr { return &eqExpr{col: col, v: v} }

// In returns a predicate that is true when the column exists and equals any
// of the supplied values. An empty vals slice produces a predicate that is
// always false.
func In(col string, vals ...Value) Expr { return &inExpr{col: col, vals: vals} }

// Between returns a predicate that is true when the column exists and
// lo <= col <= hi. lo and hi must be the same type as one another. If the
// row's column is a different type the predicate is false.
func Between(col string, lo, hi Value) Expr { return &betweenExpr{col: col, lo: lo, hi: hi} }

// Exists is true when the column is present on the row (regardless of
// value).
func Exists(col string) Expr { return &existsExpr{col: col} }

// Not inverts an inner predicate.
func Not(e Expr) Expr { return &notExpr{e: e} }

// And is the conjunction of its operands. An empty And is true.
func And(es ...Expr) Expr { return &andExpr{es: es} }

// Or is the disjunction of its operands. An empty Or is false.
func Or(es ...Expr) Expr { return &orExpr{es: es} }

// ---- implementations ----

type eqExpr struct {
	col string
	v   Value
}

func (e *eqExpr) eval(get colGetter) bool {
	v, ok := get(e.col)
	if !ok {
		return false
	}
	c, ok := cmpValues(v, e.v)
	return ok && c == 0
}
func (e *eqExpr) columns(dst []string) []string { return append(dst, e.col) }

type inExpr struct {
	col  string
	vals []Value
}

func (e *inExpr) eval(get colGetter) bool {
	if len(e.vals) == 0 {
		return false
	}
	v, ok := get(e.col)
	if !ok {
		return false
	}
	for _, cand := range e.vals {
		if c, ok := cmpValues(v, cand); ok && c == 0 {
			return true
		}
	}
	return false
}
func (e *inExpr) columns(dst []string) []string { return append(dst, e.col) }

type betweenExpr struct {
	col    string
	lo, hi Value
}

func (e *betweenExpr) eval(get colGetter) bool {
	if e.lo.t != e.hi.t {
		return false
	}
	v, ok := get(e.col)
	if !ok {
		return false
	}
	cLo, okLo := cmpValues(v, e.lo)
	if !okLo {
		return false
	}
	cHi, okHi := cmpValues(v, e.hi)
	if !okHi {
		return false
	}
	return cLo >= 0 && cHi <= 0
}
func (e *betweenExpr) columns(dst []string) []string { return append(dst, e.col) }

type existsExpr struct {
	col string
}

// existsExpr is the canonical "presence-only" predicate: it asks the
// getter whether the column appears on the row and does not consult
// the Value at all. The query planner relies on this contract to
// classify the column into queryPlan.mask.wantPresenceBM, which lets
// the codec set a presence bit and skip decodePayload entirely. If
// you ever change this eval to also inspect v, also revisit
// classifyFilterCols in query.go so the planner promotes the column
// into the value-decode set.
func (e *existsExpr) eval(get colGetter) bool {
	_, ok := get(e.col)
	return ok
}
func (e *existsExpr) columns(dst []string) []string { return append(dst, e.col) }

// classifyFilterCols walks an Expr tree and partitions its column
// references into "presence-only" (existsExpr, including under
// notExpr) and "value-needed" (eqExpr/inExpr/betweenExpr) sets.
// Conjunction / disjunction / negation propagate through to the leaf
// classification; the planner subsequently dedupes presence against
// value (value supersedes).
//
// An unrecognised Expr type is conservatively treated as value-needed
// for every column it touches; that keeps the decoder correct for
// any future predicate that does inspect Value.
func classifyFilterCols(e Expr, presence, value map[string]struct{}) {
	switch ex := e.(type) {
	case *existsExpr:
		presence[ex.col] = struct{}{}
	case *notExpr:
		if ex.e != nil {
			classifyFilterCols(ex.e, presence, value)
		}
	case *andExpr:
		for _, sub := range ex.es {
			if sub == nil {
				continue
			}
			classifyFilterCols(sub, presence, value)
		}
	case *orExpr:
		for _, sub := range ex.es {
			if sub == nil {
				continue
			}
			classifyFilterCols(sub, presence, value)
		}
	case *eqExpr:
		value[ex.col] = struct{}{}
	case *inExpr:
		value[ex.col] = struct{}{}
	case *betweenExpr:
		value[ex.col] = struct{}{}
	default:
		for _, n := range e.columns(nil) {
			value[n] = struct{}{}
		}
	}
}

type notExpr struct {
	e Expr
}

func (n *notExpr) eval(get colGetter) bool {
	if n.e == nil {
		return true
	}
	return !n.e.eval(get)
}
func (n *notExpr) columns(dst []string) []string {
	if n.e == nil {
		return dst
	}
	return n.e.columns(dst)
}

type andExpr struct {
	es []Expr
}

func (a *andExpr) eval(get colGetter) bool {
	for _, e := range a.es {
		if e == nil {
			continue
		}
		if !e.eval(get) {
			return false
		}
	}
	return true
}
func (a *andExpr) columns(dst []string) []string {
	for _, e := range a.es {
		if e == nil {
			continue
		}
		dst = e.columns(dst)
	}
	return dst
}

type orExpr struct {
	es []Expr
}

func (o *orExpr) eval(get colGetter) bool {
	for _, e := range o.es {
		if e == nil {
			continue
		}
		if e.eval(get) {
			return true
		}
	}
	return false
}
func (o *orExpr) columns(dst []string) []string {
	for _, e := range o.es {
		if e == nil {
			continue
		}
		dst = e.columns(dst)
	}
	return dst
}
