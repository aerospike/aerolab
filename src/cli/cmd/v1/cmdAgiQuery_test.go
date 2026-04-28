//go:build !noagi

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestLocalTransportRoundTripsAgainstHTTPTestServer wires a tiny
// httptest.Server in front of a fake responder and confirms the
// localTransport's GET / POST round-trip the body and headers
// correctly. This is the primary regression guard for the
// "I'm running on the AGI box itself" code path — every other
// transport detail (auth, SSH, etc.) is bypassed so a failure here
// is unambiguous.
func TestLocalTransportRoundTripsAgainstHTTPTestServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/db/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("info: got method %s want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		//nolint:errcheck
		w.Write([]byte(`{"path":"/tmp/db","setCount":1}`))
	})
	mux.HandleFunc("/debug/db/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("query: got method %s want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("query: content-type %q want application/json", got)
		}
		// Echo the body back so the test can verify it survived
		// the round trip without mangling.
		w.Header().Set("Content-Type", "application/x-ndjson")
		buf := new(bytes.Buffer)
		//nolint:errcheck
		buf.ReadFrom(r.Body)
		fmt.Fprintf(w, `{"key":"echo","row":%s}`+"\n", buf.String())
		fmt.Fprint(w, `{"_meta":{"rowsReturned":1,"truncated":false,"durationMs":"0"}}`+"\n")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	tr := &localTransport{}

	// GET round-trip.
	body, err := tr.Get("http://" + u.Host + "/debug/db/info")
	if err != nil {
		t.Fatalf("Get: %s", err)
	}
	var info struct {
		Path     string `json:"path"`
		SetCount int    `json:"setCount"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatalf("decode info: %s", err)
	}
	if info.Path != "/tmp/db" || info.SetCount != 1 {
		t.Fatalf("info: got %+v", info)
	}

	// POST round-trip: the server echoes the body back as the row
	// payload, so a complete client→server→client trip is observed.
	plan := `{"set":"x","limit":1}`
	body, err = tr.Post("http://"+u.Host+"/debug/db/query", "application/json", []byte(plan))
	if err != nil {
		t.Fatalf("Post: %s", err)
	}
	if !strings.Contains(string(body), plan) {
		t.Fatalf("Post: response did not echo plan; got %s", string(body))
	}
	if !strings.Contains(string(body), `"_meta"`) {
		t.Fatalf("Post: missing trailing _meta record in body: %s", string(body))
	}
}

// TestLocalTransportSurfacesServerErrorEnvelope confirms that a
// 4xx response with a {"error":"..."} body surfaces verbatim in
// the returned error. The renderer's tryDecodeError relies on
// this contract.
func TestLocalTransportSurfacesServerErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		//nolint:errcheck
		w.Write([]byte(`{"error":"set \"missing\" not found"}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	tr := &localTransport{}
	_, err := tr.Get("http://" + u.Host + "/anything")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `set "missing" not found`) {
		t.Fatalf("error message did not surface server body: %s", err)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error message did not include status code: %s", err)
	}
}

// TestResolveTransportRespectsExplicitChoice confirms that the
// transport selector honours the --transport flag without consulting
// the AGI marker.
func TestResolveTransportRespectsExplicitChoice(t *testing.T) {
	cases := []struct {
		in   string
		want queryTransport
		err  bool
	}{
		{"local", transportLocal, false},
		{"ssh", transportSSH, false},
		{"bogus", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			c := &AgiQueryCmd{Transport: tc.in}
			got, err := c.resolveTransport()
			if tc.err {
				if err == nil {
					t.Fatalf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
