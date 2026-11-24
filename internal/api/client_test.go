package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/testutil"
)

func TestClient_GetInto_DecodesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":1,"artist_name":"Aphex Twin"}]`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	var out []struct {
		ID     int    `json:"id"`
		Artist string `json:"artist_name"`
	}
	if _, err := c.getInto(context.Background(), "/library/", nil, &out); err != nil {
		t.Fatalf("getInto() error = %v", err)
	}
	if len(out) != 1 || out[0].Artist != "Aphex Twin" {
		t.Fatalf("decoded %+v, want one Aphex Twin row", out)
	}
}

func TestClient_GetInto_SendsQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	q := map[string]string{"artist_name": "boards of canada"}
	var out []any
	if _, err := c.getInto(context.Background(), "/library/", q, &out); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "artist_name=boards+of+canada" {
		t.Errorf("query = %q, want artist_name=boards+of+canada", gotQuery)
	}
}

func TestClient_GetInto_MapsStatusToError(t *testing.T) {
	tests := []struct {
		code int
	}{{http.StatusUnauthorized}, {http.StatusForbidden}, {http.StatusNotFound}, {http.StatusInternalServerError}}
	for _, tt := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":"nope"}`, tt.code)
		}))
		c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
		var out any
		_, err := c.getInto(context.Background(), "/x", nil, &out)
		var se *StatusError
		if !errors.As(err, &se) {
			srv.Close()
			t.Fatalf("code %d: got %v, want *StatusError", tt.code, err)
		}
		if se.Code != tt.code {
			t.Errorf("StatusError.Code = %d, want %d", se.Code, tt.code)
		}
		srv.Close()
	}
}

// truncatingServer declares a 100-byte body, sends fewer bytes, then hangs up,
// mimicking a connection dropped mid-response on a 2xx.
func truncatingServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":45029,"artist`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClient_QueryRaw_TruncatedBodyIsError(t *testing.T) {
	srv := truncatingServer(t)
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	raw, err := c.deleteQueryRaw(context.Background(), "/djs/bin", map[string]string{"album_id": "45029"})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("deleteQueryRaw() = %q, %v; want io.ErrUnexpectedEOF", raw, err)
	}
}

func TestClient_BodyRaw_TruncatedBodyIsError(t *testing.T) {
	srv := truncatingServer(t)
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	raw, err := c.postRaw(context.Background(), "/flowsheet/", map[string]any{"x": 1})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("postRaw() = %q, %v; want io.ErrUnexpectedEOF", raw, err)
	}
}

// A 2xx body that isn't JSON must fail naming the endpoint, so the error
// says where the bad response came from in both --json and table mode.
func TestClient_Raw_InvalidJSONNamesPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>502 Bad Gateway</html>`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	if _, err := c.getRaw(context.Background(), "/library/rotation", nil); err == nil ||
		!strings.Contains(err.Error(), "/library/rotation: response is not valid JSON") {
		t.Errorf("getRaw() err = %v, want a '/library/rotation: response is not valid JSON' error", err)
	}
	if _, err := c.postRaw(context.Background(), "/flowsheet", map[string]any{}); err == nil ||
		!strings.Contains(err.Error(), "/flowsheet: response is not valid JSON") {
		t.Errorf("postRaw() err = %v, want a '/flowsheet: response is not valid JSON' error", err)
	}
}

// The backend answers some endpoints with 204 No Content (e.g. GET
// /flowsheet/latest on an empty flowsheet); an empty body is not an error.
func TestClient_Raw_EmptyBodyIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	raw, err := c.getRaw(context.Background(), "/flowsheet/latest", nil)
	if err != nil || len(raw) != 0 {
		t.Errorf("getRaw() = %q, %v; want empty, nil", raw, err)
	}
}

// Every request path must send the JSON headers; testutil.CaptureServer fails
// the test if Accept (always) or Content-Type (with a body) is missing.
func TestClient_SendsJSONHeaders(t *testing.T) {
	ctx := context.Background()
	calls := map[string]func(c *Client) error{
		"getRaw": func(c *Client) error { _, err := c.getRaw(ctx, "/x", map[string]string{"a": "1"}); return err },
		"deleteQueryRaw": func(c *Client) error {
			_, err := c.deleteQueryRaw(ctx, "/x", map[string]string{"a": "1"})
			return err
		},
		"postRaw":   func(c *Client) error { _, err := c.postRaw(ctx, "/x", map[string]int{"a": 1}); return err },
		"patchRaw":  func(c *Client) error { _, err := c.patchRaw(ctx, "/x", map[string]int{"a": 1}); return err },
		"deleteRaw": func(c *Client) error { _, err := c.deleteRaw(ctx, "/x", map[string]int{"a": 1}); return err },
		"BinAdd":    func(c *Client) error { return c.BinAdd(ctx, 45029) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			srv, _ := testutil.CaptureServer(t, `{}`)
			if err := call(&Client{BaseURL: srv.URL, HTTP: srv.Client()}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// StatusError.Error() is what a user sees on stderr for every non-2xx, so its
// exact text is pinned: the server's message, a status-text fallback for an
// empty body, and a cap so a proxy's HTML error page can't flood the terminal.
func TestStatusError_ErrorText(t *testing.T) {
	huge := strings.Repeat("x", maxErrorBody+1000)
	tests := []struct {
		name string
		code int
		body string
		want string
	}{
		{"json body", http.StatusBadRequest, `{"message":"Missing album identifier"}` + "\n", `/x: HTTP 400: {"message":"Missing album identifier"}`},
		{"empty body", http.StatusServiceUnavailable, "", "/x: HTTP 503: Service Unavailable"},
		{"oversized body", http.StatusBadGateway, huge, "/x: HTTP 502: " + huge[:maxErrorBody]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.code)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
			_, getErr := c.getRaw(context.Background(), "/x", nil)
			_, postErr := c.postRaw(context.Background(), "/x", map[string]int{})
			for method, err := range map[string]error{"GET": getErr, "POST": postErr} {
				var se *StatusError
				if !errors.As(err, &se) {
					t.Fatalf("%s err = %v, want *StatusError", method, err)
				}
				if got := se.Error(); got != tt.want {
					t.Errorf("%s Error() = %.80q (len %d), want %.80q (len %d)", method, got, len(got), tt.want, len(tt.want))
				}
			}
		})
	}
}
