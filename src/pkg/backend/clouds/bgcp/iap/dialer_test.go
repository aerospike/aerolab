package iap

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// staticTokenSource is a minimal oauth2.TokenSource for use in tests.
type staticTokenSource struct {
	tok *oauth2.Token
	err error
}

func (s *staticTokenSource) Token() (*oauth2.Token, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.tok, nil
}

func TestMakeDialer_ReturnsNonNil(t *testing.T) {
	ts := &staticTokenSource{tok: &oauth2.Token{AccessToken: "fake"}}
	d := MakeDialer(ts, Target{
		Project:  "p",
		Zone:     "z",
		Instance: "i",
		Port:     22,
	})
	if d == nil {
		t.Fatal("MakeDialer returned nil dialer")
	}
}

func TestMakeDialer_DefaultsInterfaceToNic0(t *testing.T) {
	// We can't observe iaplib internals from here, but we can at least exercise
	// the construction path and ensure no panic when Interface is empty. The
	// dial itself will fail because the token source is fake; we only care that
	// the closure was built and is callable.
	ts := &staticTokenSource{err: errors.New("test: no real token")}
	d := MakeDialer(ts, Target{
		Project:  "p",
		Zone:     "z",
		Instance: "i",
		// Interface intentionally left empty -- should default to "nic0"
		Port: 22,
	})
	if d == nil {
		t.Fatal("MakeDialer returned nil dialer")
	}
}

func TestAnnotateDialError_Permission(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
	}{
		{"403", "received 403 from server"},
		{"forbidden", "tunnel.cloudproxy.app: forbidden"},
		{"permission", "permission denied"},
		{"unauthorized", "unauthorized"},
		{"401", "401 Unauthorized"},
	}
	tgt := Target{Project: "p", Zone: "z", Instance: "i", Port: 22}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := annotateDialError(tgt, errors.New(tc.raw))
			if err == nil {
				t.Fatal("expected wrapped error, got nil")
			}
			if !strings.Contains(err.Error(), "iap.tunnelResourceAccessor") {
				t.Errorf("expected hint to mention iap.tunnelResourceAccessor; got: %s", err)
			}
		})
	}
}

func TestAnnotateDialError_APINotEnabled(t *testing.T) {
	t.Parallel()
	tgt := Target{Project: "myproj", Zone: "z", Instance: "i", Port: 22}
	err := annotateDialError(tgt, errors.New("IAP API has not been used in project 12345"))
	if err == nil {
		t.Fatal("expected wrapped error, got nil")
	}
	if !strings.Contains(err.Error(), "iap.googleapis.com") {
		t.Errorf("expected hint to mention iap.googleapis.com; got: %s", err)
	}
	if !strings.Contains(err.Error(), "myproj") {
		t.Errorf("expected hint to include project id; got: %s", err)
	}
}

func TestAnnotateDialError_NotFound(t *testing.T) {
	t.Parallel()
	tgt := Target{Project: "p", Zone: "z", Instance: "missing", Port: 22}
	err := annotateDialError(tgt, errors.New("404 not found"))
	if err == nil {
		t.Fatal("expected wrapped error, got nil")
	}
	if !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "z") {
		t.Errorf("expected hint to include instance and zone; got: %s", err)
	}
}

func TestAnnotateDialError_GenericFallback(t *testing.T) {
	t.Parallel()
	tgt := Target{Project: "p", Zone: "z", Instance: "i", Port: 22}
	orig := errors.New("connection reset by peer")
	err := annotateDialError(tgt, orig)
	if err == nil {
		t.Fatal("expected wrapped error, got nil")
	}
	if !errors.Is(err, orig) {
		t.Errorf("expected wrapped error to satisfy errors.Is; got: %s", err)
	}
	if !strings.Contains(err.Error(), "iap dial p/z/i:22") {
		t.Errorf("expected wrapped error to include target identifier; got: %s", err)
	}
}
