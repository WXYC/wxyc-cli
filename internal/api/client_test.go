package api

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
	raw, err := c.do(context.Background(), http.MethodPost, "/flowsheet/", nil, map[string]any{"x": 1})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("do(POST) = %q, %v; want io.ErrUnexpectedEOF", raw, err)
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
	if _, err := c.do(context.Background(), http.MethodPost, "/flowsheet", nil, map[string]any{}); err == nil ||
		!strings.Contains(err.Error(), "/flowsheet: response is not valid JSON") {
		t.Errorf("do(POST) err = %v, want a '/flowsheet: response is not valid JSON' error", err)
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

// Every request path must send the JSON headers: Accept always, and
// Content-Type exactly when there is a body. Body-less calls (GET, the
// query-string DELETE) must send no body at all — not even a JSON "null" —
// and body-carrying calls must send one, with the method the endpoint expects.
func TestClient_SendsJSONHeaders(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		method string
		body   bool
		call   func(c *Client) error
	}{
		{"getRaw", http.MethodGet, false, func(c *Client) error { _, err := c.getRaw(ctx, "/x", map[string]string{"a": "1"}); return err }},
		{"deleteQueryRaw", http.MethodDelete, false, func(c *Client) error {
			_, err := c.deleteQueryRaw(ctx, "/x", map[string]string{"a": "1"})
			return err
		}},
		{"do POST", http.MethodPost, true, func(c *Client) error {
			_, err := c.do(ctx, http.MethodPost, "/x", nil, map[string]int{"a": 1})
			return err
		}},
		{"patchRaw", http.MethodPatch, true, func(c *Client) error { _, err := c.patchRaw(ctx, "/x", map[string]int{"a": 1}); return err }},
		{"do DELETE with body", http.MethodDelete, true, func(c *Client) error {
			_, err := c.do(ctx, http.MethodDelete, "/x", nil, map[string]int{"a": 1})
			return err
		}},
		{"BinAdd", http.MethodPost, true, func(c *Client) error { return c.BinAdd(ctx, 45029) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var method, accept, ctype string
			var bodyLen int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				method, accept, ctype, bodyLen = r.Method, r.Header.Get("Accept"), r.Header.Get("Content-Type"), len(b)
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			if err := tt.call(&Client{BaseURL: srv.URL, HTTP: srv.Client()}); err != nil {
				t.Fatal(err)
			}
			if method != tt.method {
				t.Errorf("method = %s, want %s", method, tt.method)
			}
			if accept != "application/json" {
				t.Errorf("Accept = %q, want application/json", accept)
			}
			if tt.body {
				if bodyLen == 0 || ctype != "application/json" {
					t.Errorf("body len %d, Content-Type %q; want a body with application/json", bodyLen, ctype)
				}
			} else if bodyLen != 0 || ctype != "" {
				t.Errorf("body len %d, Content-Type %q; want no body and no Content-Type", bodyLen, ctype)
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
			_, postErr := c.do(context.Background(), http.MethodPost, "/x", nil, map[string]int{})
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

// A refusal's {message, code} body can outrun the display cap; the command
// layer parses StatusError.Body to print a friendly line, so the stored body
// must stay whole (valid JSON) while only Error()'s text is capped.
func TestStatusError_LongJSONBodyStaysParseable(t *testing.T) {
	body := `{"code":"closed_show","message":"` + strings.Repeat("m", maxErrorBody+500) + `"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, err := c.patchRaw(context.Background(), "/flowsheet/shows/1", map[string]int{})
	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want *StatusError", err)
	}
	if se.Body != body {
		t.Errorf("Body len %d, want the whole %d-byte body", len(se.Body), len(body))
	}
	if want := "/flowsheet/shows/1: HTTP 403: " + body[:maxErrorBody]; se.Error() != want {
		t.Errorf("Error() len %d, want capped text of len %d", len(se.Error()), len(want))
	}
}

// An error response's unread tail must be drained so the keep-alive
// connection goes back to the pool rather than being torn down.
func TestClient_ErrorBodyIsDrainedForReuse(t *testing.T) {
	var conns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(strings.Repeat("x", maxErrorRead+64<<10)))
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	for i := 0; i < 2; i++ {
		if _, err := c.getRaw(context.Background(), "/x", nil); err == nil {
			t.Fatal("err = nil, want a StatusError")
		}
	}
	if n := conns.Load(); n != 1 {
		t.Errorf("opened %d connections for 2 requests, want 1 (reused)", n)
	}
}
