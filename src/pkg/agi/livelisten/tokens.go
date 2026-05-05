package livelisten

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// tokenStore is a goroutine-safe set of bearer tokens loaded from
// every regular file under TokensPath. The proxy already watches
// the same directory with fsnotify; here we use a simpler 30s poll
// because the listener is in-process and reload latency is not
// safety-critical (existing connections keep working through a
// rotation; a freshly-rotated dispatcher just retries until its
// new token reaches us).
type tokenStore struct {
	dir string

	mu    sync.RWMutex
	set   map[string]struct{}
	loadT time.Time
}

func newTokenStore(dir string) *tokenStore {
	return &tokenStore{dir: dir, set: map[string]struct{}{}}
}

func (t *tokenStore) start(ctx context.Context) {
	t.reload()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.reload()
			}
		}
	}()
}

// reload walks t.dir and rebuilds the in-memory set. Files shorter
// than 32 chars are skipped (matches the proxy's 64-char minimum
// loosely; we're more permissive here so test fixtures with shorter
// tokens still work).
func (t *tokenStore) reload() {
	if t.dir == "" {
		return
	}
	entries, err := os.ReadDir(t.dir)
	if err != nil {
		// Missing dir is normal on first start before the
		// AGI bootstrap creates /opt/agi/tokens.
		if !os.IsNotExist(err) {
			log.Printf("WARN: livelisten: read token dir %s: %s", t.dir, err)
		}
		return
	}
	next := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(t.dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			log.Printf("WARN: livelisten: read token %s: %s", p, err)
			continue
		}
		s := trimSpaceASCII(string(b))
		if len(s) < 32 {
			continue
		}
		next[s] = struct{}{}
	}
	t.mu.Lock()
	t.set = next
	t.loadT = time.Now()
	t.mu.Unlock()
}

func (t *tokenStore) has(tok string) bool {
	if tok == "" {
		return false
	}
	t.mu.RLock()
	_, ok := t.set[tok]
	t.mu.RUnlock()
	return ok
}

// add lets tests / SetTokensForTest inject tokens without touching
// the filesystem.
func (t *tokenStore) add(tok string) {
	t.mu.Lock()
	if t.set == nil {
		t.set = map[string]struct{}{}
	}
	t.set[tok] = struct{}{}
	t.mu.Unlock()
}

// trimSpaceASCII strips ASCII whitespace from both ends; sufficient
// for token files written via os.WriteFile which sometimes carry a
// trailing newline. Inlined to avoid pulling the stdlib `strings`
// pkg into the hot listener path; tokens are ASCII by construction.
func trimSpaceASCII(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

// SetTokensForTest replaces the in-memory token set with the
// supplied set. Test-only entry-point used by package tests where
// spinning up a tokens directory is overkill.
func (l *Listener) SetTokensForTest(tokens ...string) {
	l.tokens.mu.Lock()
	l.tokens.set = make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		l.tokens.set[t] = struct{}{}
	}
	l.tokens.mu.Unlock()
}
