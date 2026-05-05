package livelisten

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTokens_LoadFromDir verifies the basic case: a token file in the
// directory becomes a valid bearer token after reload().
func TestTokens_LoadFromDir(t *testing.T) {
	dir := t.TempDir()
	const validTok = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEF" // 42 chars > 32 min
	if err := os.WriteFile(filepath.Join(dir, "dispatcher"), []byte(validTok+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ts := newTokenStore(dir)
	ts.reload()
	if !ts.has(validTok) {
		t.Fatal("expected token to be loaded")
	}
}

// TestTokens_RejectShort checks the 32-char minimum length filter.
// Files shorter than 32 chars must be ignored — protects against an
// accidental empty/typoed file becoming a no-auth bypass.
func TestTokens_RejectShort(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "short"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	ts := newTokenStore(dir)
	ts.reload()
	if ts.has("hi") {
		t.Fatal("short token should have been rejected")
	}
}

// TestTokens_TrimTrailingNewline verifies that os.WriteFile-style
// trailing newlines and CRLF on Windows do not stop the token from
// matching the wire-side bearer header.
func TestTokens_TrimTrailingNewline(t *testing.T) {
	got := trimSpaceASCII(" \t" + "tok-value\r\n")
	want := "tok-value"
	if got != want {
		t.Fatalf("trimSpaceASCII: want %q, got %q", want, got)
	}
}

// TestTokens_AddProgrammatic exercises the SetTokensForTest /
// (*tokenStore).add escape hatch used by dispatcher_test.go to
// avoid spinning up a tokens directory.
func TestTokens_AddProgrammatic(t *testing.T) {
	ts := newTokenStore("")
	const tok = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	ts.add(tok)
	if !ts.has(tok) {
		t.Fatal("expected programmatic token to be present")
	}
}
