package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WXYC/wxyc-cli/internal/auth"
)

// TestMain scrubs every ambient WXYC_* variable so a developer's exported
// WXYC_API_URL, WXYC_JWT, WXYC_ALLOW_WRITE, ... can never reach a test. Tests
// set only what they need with t.Setenv.
func TestMain(m *testing.M) {
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "WXYC_") {
			os.Unsetenv(k)
		}
	}
	os.Exit(m.Run())
}

// newTestApp returns an App wired for hermetic tests: env comes from the
// (scrubbed) process env, WXYC_API_URL defaults to a trap server that fails
// the test on any request, and the session store is a FileStore in a temp
// dir, so nothing touches the network or the OS keychain unless a test
// explicitly points it somewhere.
func newTestApp(t *testing.T) *App {
	t.Helper()
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to default API: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(trap.Close)
	return &App{
		getenv: func(k string) string {
			if v := os.Getenv(k); v != "" {
				return v
			}
			if k == "WXYC_API_URL" {
				return trap.URL
			}
			return ""
		},
		newStore: func() auth.Store { return auth.FileStore{Dir: t.TempDir()} },
	}
}

// runCLI executes the command tree with args on a fresh hermetic App and
// returns stdout and the error.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return runApp(t, newTestApp(t), args...)
}

// runApp executes the command tree against a caller-configured App.
func runApp(t *testing.T, app *App, args ...string) (string, error) {
	t.Helper()
	var out, errbuf bytes.Buffer
	app.stdout, app.stderr = &out, &errbuf
	root := newRoot(app)
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(&errbuf)
	err := root.Execute()
	return out.String(), err
}

// The command tree must resolve config through the App's injected getenv,
// never the process environment directly.
func TestApp_UsesInjectedEnvNotProcessEnv(t *testing.T) {
	ambient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request reached the process-env API: %s %s", r.Method, r.URL)
	}))
	defer ambient.Close()
	t.Setenv("WXYC_API_URL", ambient.URL)
	t.Setenv("WXYC_JWT", "ambient")

	var hits int32
	injected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(`[]`))
	}))
	defer injected.Close()
	env := map[string]string{"WXYC_API_URL": injected.URL, "WXYC_JWT": "injected"}
	app := newTestApp(t)
	app.getenv = func(k string) string { return env[k] }

	if _, err := runApp(t, app, "library", "search", "--artist", "x"); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("injected API hit %d times, want 1", hits)
	}
}

// With a static WXYC_JWT nothing needs the session store, so it must never
// be constructed (constructing the real one probes the OS keychain).
func TestApp_StaticJWTNeverConstructsStore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
	app := newTestApp(t)
	app.newStore = func() auth.Store {
		t.Error("session store constructed despite WXYC_JWT")
		return auth.FileStore{Dir: t.TempDir()}
	}

	if _, err := runApp(t, app, "library", "search", "--artist", "x"); err != nil {
		t.Fatal(err)
	}
}

// Without WXYC_JWT, the provider loads the session from the injected store;
// an empty one must surface as ErrNoSession (exit 3) before any request.
func TestApp_SessionComesFromInjectedStore(t *testing.T) {
	_, err := runCLI(t, "library", "search", "--artist", "x")
	if !errors.Is(err, auth.ErrNoSession) {
		t.Errorf("err = %v, want ErrNoSession from the empty temp store", err)
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

// A body shaped differently from the expected array must be an error in table
// mode, not a silent "no results".
func TestRotation_TableDecodeErrorIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"rotation":[{"artist_name":"X","rotation_bin":"H"}]}`))
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "library", "rotation")
	if err == nil || !strings.Contains(err.Error(), "/library/rotation: decoding response") {
		t.Fatalf("err = %v (out %q), want a /library/rotation decoding error", err, out)
	}
}

// hangingTokenApp returns an App whose stored session is exchanged against a
// /token endpoint that never responds.
func hangingTokenApp(t *testing.T) *App {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	t.Setenv("WXYC_AUTH_URL", srv.URL)

	store := auth.FileStore{Dir: t.TempDir()}
	if err := store.Save("default", "sess"); err != nil {
		t.Fatal(err)
	}
	app := newTestApp(t)
	app.newStore = func() auth.Store { return store }
	return app
}

// runBounded runs fn and fails the test if it hasn't returned within d.
func runBounded(t *testing.T, d time.Duration, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("still blocked after %v", d)
		return nil
	}
}

// Commands that fetch a token directly (whoami, resolveDJID) are outside the
// API client's timeout, so the token exchange needs its own.
func TestWhoami_TokenExchangeTimesOut(t *testing.T) {
	app := hangingTokenApp(t)
	app.authTimeout = 100 * time.Millisecond

	err := runBounded(t, 5*time.Second, func() error {
		_, err := runApp(t, app, "whoami")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "Client.Timeout exceeded") {
		t.Fatalf("err = %v, want a client timeout", err)
	}
}

func TestApp_TokenExchangeHasDefaultTimeout(t *testing.T) {
	app := newTestApp(t)
	app.build("", false, false)
	if got := app.provider.HTTP.Timeout; got != 30*time.Second {
		t.Errorf("token client timeout = %v, want 30s", got)
	}
}

// Cancelling the root context (Ctrl-C) aborts an in-flight token exchange.
func TestRun_ContextCancelAbortsTokenFetch(t *testing.T) {
	app := hangingTokenApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runBounded(t, 5*time.Second, func() error {
		app.stdout, app.stderr = io.Discard, io.Discard
		return run(ctx, app, []string{"whoami"})
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}
