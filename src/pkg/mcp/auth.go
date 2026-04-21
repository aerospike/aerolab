package mcp

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// BearerMiddleware returns an HTTP middleware that rejects any request
// whose Authorization header does not carry the given bearer token. The
// comparison uses constant-time equality to avoid timing leaks.
//
// If token is empty, the returned middleware is a no-op pass-through.
func BearerMiddleware(token string) func(http.Handler) http.Handler {
	if token == "" {
		return func(h http.Handler) http.Handler { return h }
	}
	want := []byte(token)
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !authorizationHeaderMatches(r, want) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="aerolab-mcp"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			h.ServeHTTP(w, r)
		})
	}
}

func authorizationHeaderMatches(r *http.Request, want []byte) bool {
	h := r.Header.Get("Authorization")
	if h == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	got := []byte(strings.TrimSpace(h[len(prefix):]))
	if len(got) != len(want) {
		// Still consume the constant-time compare to avoid an early
		// length-based side channel.
		_ = subtle.ConstantTimeCompare(padTo(got, len(want)), want)
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func padTo(b []byte, n int) []byte {
	if len(b) >= n {
		return b[:n]
	}
	out := make([]byte, n)
	copy(out, b)
	return out
}
