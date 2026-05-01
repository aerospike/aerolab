package ingest

import (
	"errors"
	"fmt"
	"sort"
)

// acMatcher is an Aho-Corasick goto/failure trie that finds, for any
// input string, the lowest-index needle whose substring appears in the
// input. Construction is O(sum-of-needle-lengths); matching is
// O(len(input)).
//
// Built once per cluster Defs entry at compile time (see
// patterns.compile). Empty needles are rejected at Build time so the
// trie never has a degenerate state that matches every input.
//
// The minIdx[s] precomputation per state is what makes FirstIndex true
// O(len(line)): instead of walking the dictionary-suffix chain at every
// accepting state, every state caches the smallest pattern index
// reachable via output(s) ∪ output(fail*(s)). FirstIndex can also
// short-circuit once it has observed pattern index 0 since no smaller
// index can exist.
//
// dense is the deterministic transition table. Entry dense[s*256+c]
// holds the state to jump to from state s on input byte c, with the
// fail-link chain already collapsed at build time. This turns the
// query-side inner loop (the original `for { goto[s][c] || s = fail[s] }`
// dance) into a single indexed load per input byte. The pre-collapse
// form's `gotoFn[s][c]` map lookup was 201s cum (~9.5% of total CPU)
// in production AGI ingest pprofs; the dense table eliminates the
// map hash per byte plus the inner failure-chain walk on every miss.
//
// Memory: numStates * 256 * 4 bytes. AGI's compiled trie has on the
// order of 10^3-10^4 states; the dense table is therefore ~1-10 MiB,
// trivially small next to the rest of the ingest working set.
//
// gotoFn is preserved alongside dense because it carries the sparse
// build-time edge structure (and is expected by tests / future build
// logic). It is not consulted by FirstIndex / AnyIndices on the hot
// path.
type acMatcher struct {
	gotoFn []map[byte]int // children per state (sparse, build-time)
	fail   []int          // failure links
	output [][]int        // pattern indices ending at each state
	minIdx []int          // memoised min pattern index reachable via output chain
	dense  []int32        // dense[s*256+c] = next state (fail-chain collapsed); query-side hot path
}

// newACMatcher builds an Aho-Corasick automaton for the given needles.
// The pattern index returned by FirstIndex is the index into this
// slice. Empty needles are rejected because they would otherwise match
// at every input position and break the first-match-wins semantic.
func newACMatcher(needles []string) (*acMatcher, error) {
	if len(needles) == 0 {
		return nil, errors.New("acMatcher: needles list is empty")
	}
	for i, n := range needles {
		if n == "" {
			return nil, fmt.Errorf("acMatcher: needles[%d] is empty", i)
		}
	}
	m := &acMatcher{
		gotoFn: []map[byte]int{{}},
		fail:   []int{0},
		output: [][]int{nil},
	}
	// 1) Build the trie. Each needle is inserted as a path from the
	// root; the terminal state records the pattern index in output.
	for pi, needle := range needles {
		s := 0
		for j := 0; j < len(needle); j++ {
			c := needle[j]
			next, ok := m.gotoFn[s][c]
			if !ok {
				next = len(m.gotoFn)
				m.gotoFn = append(m.gotoFn, map[byte]int{})
				m.fail = append(m.fail, 0)
				m.output = append(m.output, nil)
				m.gotoFn[s][c] = next
			}
			s = next
		}
		m.output[s] = append(m.output[s], pi)
	}
	// 2) BFS to compute failure links. The root's direct children
	// fail to the root; deeper states fail to the longest proper
	// suffix that is also a prefix of some needle, found by walking
	// the failure chain of the parent on the same input byte.
	queue := make([]int, 0, len(m.gotoFn))
	for _, child := range m.gotoFn[0] {
		m.fail[child] = 0
		queue = append(queue, child)
	}
	for len(queue) > 0 {
		r := queue[0]
		queue = queue[1:]
		for c, s := range m.gotoFn[r] {
			queue = append(queue, s)
			f := m.fail[r]
			for {
				if next, ok := m.gotoFn[f][c]; ok && next != s {
					m.fail[s] = next
					break
				}
				if f == 0 {
					m.fail[s] = 0
					break
				}
				f = m.fail[f]
			}
		}
	}
	// 3) Compute minIdx[s] = min(output[s] ∪ minIdx[fail[s]]) in BFS
	// order so fail[s] is always resolved before s. The BFS order
	// from 2) is consumed by `queue`; rebuild it locally so this
	// pass is independent of the fail-link construction.
	m.minIdx = make([]int, len(m.gotoFn))
	for i := range m.minIdx {
		m.minIdx[i] = -1
	}
	bfs := []int{0}
	for i := 0; i < len(bfs); i++ {
		r := bfs[i]
		for _, child := range m.gotoFn[r] {
			bfs = append(bfs, child)
		}
	}
	for _, s := range bfs {
		best := -1
		for _, pi := range m.output[s] {
			if best == -1 || pi < best {
				best = pi
			}
		}
		if s != 0 {
			f := m.fail[s]
			if m.minIdx[f] != -1 && (best == -1 || m.minIdx[f] < best) {
				best = m.minIdx[f]
			}
		}
		m.minIdx[s] = best
	}
	// 4) Collapse goto+fail into a dense transition table. For the
	// root, missing edges self-loop at the root (matching the
	// original FirstIndex's `if s == 0 { break }` after fail). For
	// non-root states, missing edges resolve to dense[fail[s]][c]
	// because fail[s] < s in BFS order, so dense is fully resolved
	// for every parent of s by the time we fill row s. This is the
	// textbook "deterministic AC" transformation.
	numStates := len(m.gotoFn)
	m.dense = make([]int32, numStates*256)
	// Root row first, with self-loop on missing.
	for c, next := range m.gotoFn[0] {
		m.dense[int(c)] = int32(next)
	}
	// Walk states in BFS order so dense[fail[s]] is filled first.
	for i := 1; i < len(bfs); i++ {
		s := bfs[i]
		f := m.fail[s]
		base := s * 256
		fbase := f * 256
		// Default to the fail-link's row...
		copy(m.dense[base:base+256], m.dense[fbase:fbase+256])
		// ...then overlay this state's direct edges, which take
		// precedence by definition (goto wins over fail).
		for c, next := range m.gotoFn[s] {
			m.dense[base+int(c)] = int32(next)
		}
	}
	return m, nil
}

// FirstIndex returns the smallest pattern index whose needle appears
// in line, or -1 if none. The smallest-index recovery is the semantic
// preserved from the prior linear `for _, p := range Patterns { if
// !strings.Contains(...) continue; ... return on first hit }` loop.
//
// Walks line in O(len(line)). Short-circuits when index 0 has been
// observed since no smaller index exists.
//
// Implementation: one indexed load per input byte against the dense
// transition table. The pre-collapse form did a `gotoFn[s][c]` map
// lookup plus a fail-chain walk on every miss, which dominated the
// parser CPU profile. The dense form has identical externally
// observable behaviour because the table simply caches the same goto
// + fail walk that the original loop would have performed.
func (m *acMatcher) FirstIndex(line string) int {
	dense := m.dense
	minIdx := m.minIdx
	s := int32(0)
	best := -1
	for j := 0; j < len(line); j++ {
		s = dense[int(s)<<8|int(line[j])]
		if cand := minIdx[s]; cand != -1 {
			if best == -1 || cand < best {
				best = cand
				if best == 0 {
					return 0
				}
			}
		}
	}
	return best
}

// AnyIndices returns the (sorted, deduped) list of pattern indices
// whose needles appear in line. Used as a fallback when the caller
// needs to consider every candidate (currently unused; reserved for
// future "try every match in order" semantics).
func (m *acMatcher) AnyIndices(line string) []int {
	dense := m.dense
	s := int32(0)
	seen := map[int]struct{}{}
	for j := 0; j < len(line); j++ {
		s = dense[int(s)<<8|int(line[j])]
		// Walk the dictionary-suffix chain to collect every
		// pattern that ends at any suffix-state reachable from s.
		// The dense table only encodes single-byte transitions;
		// the suffix chain still lives on m.fail, which is the
		// authoritative source for the dictionary closure.
		for t := int(s); t != 0; t = m.fail[t] {
			for _, pi := range m.output[t] {
				seen[pi] = struct{}{}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]int, 0, len(seen))
	for pi := range seen {
		out = append(out, pi)
	}
	sort.Ints(out)
	return out
}
