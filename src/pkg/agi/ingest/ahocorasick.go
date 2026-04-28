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
type acMatcher struct {
	gotoFn []map[byte]int // children per state
	fail   []int          // failure links
	output [][]int        // pattern indices ending at each state
	minIdx []int          // memoised min pattern index reachable via output chain
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
	return m, nil
}

// FirstIndex returns the smallest pattern index whose needle appears
// in line, or -1 if none. The smallest-index recovery is the semantic
// preserved from the prior linear `for _, p := range Patterns { if
// !strings.Contains(...) continue; ... return on first hit }` loop.
//
// Walks line in O(len(line)). Short-circuits when index 0 has been
// observed since no smaller index exists.
func (m *acMatcher) FirstIndex(line string) int {
	s := 0
	best := -1
	for j := 0; j < len(line); j++ {
		c := line[j]
		for {
			if next, ok := m.gotoFn[s][c]; ok {
				s = next
				break
			}
			if s == 0 {
				break
			}
			s = m.fail[s]
		}
		if cand := m.minIdx[s]; cand != -1 {
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
	s := 0
	seen := map[int]struct{}{}
	for j := 0; j < len(line); j++ {
		c := line[j]
		for {
			if next, ok := m.gotoFn[s][c]; ok {
				s = next
				break
			}
			if s == 0 {
				break
			}
			s = m.fail[s]
		}
		// Walk the dictionary-suffix chain to collect every
		// pattern that ends at any suffix-state reachable from s.
		for t := s; t != 0; t = m.fail[t] {
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
