package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/safety"
)

func TestBinRm_BlockedWithoutWrite(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
	t.Setenv("WXYC_ALLOW_WRITE", "")

	_, err := runCLI(t, "bin", "rm", "45029")
	var blocked *safety.BlockedError
	if !errors.As(err, &blocked) || blocked.Op != "bin:rm" {
		t.Fatalf("err = %v, want *BlockedError for bin:rm", err)
	}
	if mapExit(err) != ExitBlocked {
		t.Errorf("exit = %d, want ExitBlocked(%d)", mapExit(err), ExitBlocked)
	}
	if hit {
		t.Error("blocked write still reached the server")
	}
}

func binRmServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/djs/bin" || r.URL.Query().Get("album_id") != "45029" {
			t.Errorf("got %s %s, want DELETE /djs/bin?album_id=45029", r.Method, r.URL.String())
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
	return srv
}

func TestBinRm_RemovesWithWrite(t *testing.T) {
	binRmServer(t, `{"id":812,"dj_id":"u_abc123","album_id":45029,"track_title":null}`)

	out, err := runCLI(t, "bin", "rm", "45029", "--write")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "45029") {
		t.Errorf("output %q doesn't name the removed album", out)
	}
}

func TestBinRm_JSONPassesThroughServerRow(t *testing.T) {
	binRmServer(t, `{"id":812,"dj_id":"u_abc123","album_id":45029,"track_title":null}`)

	out, err := runCLI(t, "bin", "rm", "45029", "--write", "--json")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	for _, k := range []string{`"id": 812`, `"dj_id": "u_abc123"`, `"track_title": null`} {
		if !strings.Contains(out, k) {
			t.Errorf("--json output missing %s:\n%s", k, out)
		}
	}
}

func TestBinRm_NotInBinIsExitNotFound(t *testing.T) {
	binRmServer(t, "")

	_, err := runCLI(t, "bin", "rm", "45029", "--write")
	if err == nil {
		t.Fatal("removing an album not in the bin succeeded silently")
	}
	if got := mapExit(err); got != ExitNotFound {
		t.Errorf("exit = %d, want ExitNotFound(%d)", got, ExitNotFound)
	}
}

func TestBinRm_RejectsNonIntegerID(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	if _, err := runCLI(t, "bin", "rm", "abc", "--write"); err == nil {
		t.Error("non-integer album_id accepted")
	}
}
