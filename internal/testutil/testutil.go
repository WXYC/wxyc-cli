// Package testutil holds test helpers shared by more than one package's
// tests. Only _test.go files import it, so it never ships in the binary.
package testutil

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// FakeJWT builds an unsigned token whose payload carries the given sub claim.
// The CLI decodes the payload without verifying the signature, so this is
// enough for any path that reads claims (e.g. resolving dj_id from the token).
func FakeJWT(sub string) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + sub + `"}`))
	return "h." + payload + ".s"
}

// Captured is the last mutating (non-GET) request a CaptureServer served.
type Captured struct {
	Method string
	Path   string
	Body   map[string]any
}

// CaptureServer replies to every request with a 200 and resp, records the
// last non-GET request (GETs are lookups a command may make before its write),
// and fails the test unless every request carries the JSON headers the
// backend needs: Accept: application/json always, and Content-Type:
// application/json whenever there is a body — Express's json() middleware
// ignores a body without it, so the write would 400 in production.
func CaptureServer(t *testing.T, resp string) (*httptest.Server, *Captured) {
	t.Helper()
	got := &Captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if a := r.Header.Get("Accept"); a != "application/json" {
			t.Errorf("%s %s: Accept = %q, want application/json", r.Method, r.URL.Path, a)
		}
		if ct := r.Header.Get("Content-Type"); len(b) > 0 && ct != "application/json" {
			t.Errorf("%s %s: Content-Type = %q, want application/json", r.Method, r.URL.Path, ct)
		}
		if r.Method != http.MethodGet {
			got.Method, got.Path = r.Method, r.URL.Path
			got.Body = nil
			_ = json.Unmarshal(b, &got.Body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}
