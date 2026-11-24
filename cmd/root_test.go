package cmd

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/safety"
)

// runCLI executes the command tree with args and returns stdout and the error.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out, errbuf bytes.Buffer
	app := &App{stdout: &out, stderr: &errbuf}
	root := newRoot(app)
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(&errbuf)
	err := root.Execute()
	return out.String(), err
}

func TestWriteGate_BlocksMutatingCommandByDefault(t *testing.T) {
	// A static JWT avoids the keyring/store and network for this path; the
	// gate must reject before RunE, so no request is ever attempted.
	t.Setenv("WXYC_JWT", "unused")
	t.Setenv("WXYC_ALLOW_WRITE", "")

	_, err := runCLI(t, "bin", "add", "45029")
	var blocked *safety.BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("err = %v, want *BlockedError", err)
	}
	if mapExit(err) != ExitBlocked {
		t.Errorf("exit = %d, want ExitBlocked(%d)", mapExit(err), ExitBlocked)
	}
}

func TestWriteGate_AllowsMutatingCommandWhenUnlocked(t *testing.T) {
	var posted int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/djs/bin" {
			atomic.AddInt32(&posted, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	_, err := runCLI(t, "bin", "add", "45029", "--write")
	if err != nil {
		t.Fatalf("unlocked write err = %v", err)
	}
	if posted != 1 {
		t.Errorf("POST /djs/bin hit %d times, want 1", posted)
	}
}

func TestWriteGate_EnvUnlockAlsoWorks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
	t.Setenv("WXYC_ALLOW_WRITE", "1")

	if _, err := runCLI(t, "bin", "add", "1"); err != nil {
		t.Errorf("WXYC_ALLOW_WRITE=1 should permit write, got %v", err)
	}
}

func TestRotation_JSONKeepsFullShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/rotation" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"artist_name":"X","rotation_bin":"Heavy","reconciled_identity":{"discogs_artist_id":42}}]`))
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "library", "rotation", "--json")
	if err != nil {
		t.Fatal(err)
	}
	// Nested identity the table never renders must survive in --json.
	if !strings.Contains(out, "reconciled_identity") || !strings.Contains(out, "42") {
		t.Errorf("--json dropped nested fields:\n%s", out)
	}
}

func TestLibrarySearch_TableShowsShelfCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"id":1,"code_letters":"DA","code_artist_number":11,"code_number":4,
		  "artist_name":"Dave","album_title":"Wax","format_name":"vinyl","genre_name":"Jazz"}]`))
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "library", "search", "--artist", "dave")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "SHELF") || !strings.Contains(out, "Jazz DA 11/4") {
		t.Errorf("table missing shelf code:\n%s", out)
	}
}

func TestReadCommand_NeedsNoUnlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"entries":[{"id":1,"entry_type":"track","artist_name":"A","track_title":"T","album_title":"Al"}]}`))
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "tail", "--json")
	if err != nil {
		t.Fatalf("read err = %v", err)
	}
	if !strings.Contains(out, `"artist_name": "A"`) {
		t.Errorf("json output missing entry:\n%s", out)
	}
}

// GET /library/ rejects n outside 1..100 with a 400 (Backend-Service
// parseCountQueryParam, SEARCH_COUNT_MAX). The CLI must refuse locally rather
// than send n=101.
func TestLibrarySearch_LimitAboveMaxRefusedBeforeRequest(t *testing.T) {
	for _, n := range []string{"101", "-1"} {
		t.Run(n, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				if r.URL.Query().Get("n") == n {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"message":"n must be an integer from 1 to 100"}`))
					return
				}
				w.Write([]byte(`[]`))
			}))
			defer srv.Close()
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", "unused")

			_, err := runCLI(t, "library", "search", "--artist", "dave", "-n", n)
			if err == nil || !strings.Contains(err.Error(), "--limit must be from 1 to 100") {
				t.Errorf("err = %v, want local --limit range error", err)
			}
			if hits != 0 {
				t.Errorf("server hit %d times, want 0", hits)
			}
		})
	}
}

func TestLibrarySearch_LimitInRangeSentAsIs(t *testing.T) {
	for _, tc := range []struct{ arg, want string }{{"100", "100"}, {"1", "1"}, {"0", ""}} {
		t.Run(tc.arg, func(t *testing.T) {
			var got string
			var present bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Query().Get("n")
				present = r.URL.Query().Has("n")
				w.Write([]byte(`[{"id":1,"artist_name":"Dave","album_title":"Wax","format_name":"vinyl","genre_name":"Jazz"}]`))
			}))
			defer srv.Close()
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", "unused")

			if _, err := runCLI(t, "library", "search", "--artist", "dave", "-n", tc.arg); err != nil {
				t.Fatal(err)
			}
			if tc.want == "" && present {
				t.Errorf("n=%q sent, want omitted", got)
			}
			if got != tc.want {
				t.Errorf("n = %q, want %q", got, tc.want)
			}
		})
	}
}

// GET /library/ takes on_streaming=true|false and accepts it alone, with no
// artist_name or album_title (Backend-Service searchForAlbum, #872). Unset
// must send nothing so the server applies no streaming filter.
func TestLibrarySearch_OnStreamingQueryString(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		want    string
		present bool
	}{
		{"true", []string{"--on-streaming"}, "true", true},
		{"explicit true", []string{"--on-streaming=true"}, "true", true},
		{"false", []string{"--on-streaming=false"}, "false", true},
		{"unset", []string{"--artist", "dave"}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var query url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query = r.URL.Query()
				w.Write([]byte(`[{"id":1,"artist_name":"Dave","album_title":"Wax","format_name":"vinyl","genre_name":"Jazz","on_streaming":false}]`))
			}))
			defer srv.Close()
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", "unused")

			if _, err := runCLI(t, append([]string{"library", "search"}, tc.args...)...); err != nil {
				t.Fatal(err)
			}
			if got := query.Has("on_streaming"); got != tc.present {
				t.Errorf("on_streaming present = %v, want %v (query %q)", got, tc.present, query.Encode())
			}
			if got := query.Get("on_streaming"); got != tc.want {
				t.Errorf("on_streaming = %q, want %q", got, tc.want)
			}
			if tc.present && (query.Has("artist_name") || query.Has("album_title")) {
				t.Errorf("standalone filter sent extra params: %q", query.Encode())
			}
		})
	}
}
