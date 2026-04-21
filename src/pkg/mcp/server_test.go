package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBearerMiddlewareAllowsNoAuthWhenTokenEmpty(t *testing.T) {
	mw := BearerMiddleware("")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestBearerMiddlewareRejectsMissingOrWrongToken(t *testing.T) {
	mw := BearerMiddleware("sekret")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"wrong", "Bearer nope", http.StatusUnauthorized},
		{"right", "Bearer sekret", http.StatusOK},
		{"with spaces", "Bearer  sekret", http.StatusOK},
		{"wrong scheme", "Basic sekret", http.StatusUnauthorized},
		{"too long", "Bearer sekretX", http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("got %d, want %d", rec.Code, c.want)
			}
		})
	}
}

func TestNewServerRegistersToolsAndInstructions(t *testing.T) {
	reg := newTestRegistry(t)
	server, err := NewServer(Config{Registry: reg})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	sess := connectInMemory(t, server)

	// Verify initialize instructions reach the client.
	info := sess.InitializeResult()
	if !strings.Contains(info.Instructions, "aerolab_list_commands") {
		t.Errorf("expected instructions to mention aerolab_list_commands, got %q", info.Instructions)
	}

	// Verify both generic and auto tools are present.
	var count int
	for tool, err := range sess.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Tools iter: %v", err)
		}
		_ = tool
		count++
	}
	if count < 4 {
		t.Errorf("expected >=4 tools (3 generic + auto), got %d", count)
	}
}

func TestServeHTTPSmokeAndAuth(t *testing.T) {
	reg := newTestRegistry(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	cfg := Config{
		Registry:       reg,
		Transport:      TransportHTTP,
		Addr:           "127.0.0.1:0",
		AuthToken:      "hunter2",
		SessionTimeout: 5 * time.Second,
	}
	// Start on a listener we control so we know the real port.
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	handler := sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return server },
		&sdkmcp.StreamableHTTPOptions{SessionTimeout: cfg.SessionTimeout},
	)
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	httpSrv := httptest.NewServer(BearerMiddleware(cfg.AuthToken)(mux))
	t.Cleanup(httpSrv.Close)

	// Suppress unused var warnings for done/ctx in this simpler variant.
	_ = done
	_ = ctx

	// /healthz requires auth.
	unauth, err := http.Get(httpSrv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	unauth.Body.Close()
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without bearer token, got %d", unauth.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, httpSrv.URL+"/healthz", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authed GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 with bearer token, got %d", resp.StatusCode)
	}
}
