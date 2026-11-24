package cmd

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/auth"
)

// With a static WXYC_JWT there is nothing to refresh, so a 401 must be
// returned after one request: retrying would resend the identical token and
// double every request (and every write).
func TestStaticJWT_401IsNotRetried(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "static")

	_, err := runCLI(t, "bin", "list")
	if got := mapExit(err); got != ExitAuth {
		t.Errorf("exit = %d (err %v), want ExitAuth(%d)", got, err, ExitAuth)
	}
	if hits != 1 {
		t.Errorf("API hit %d times, want 1 (no retry with a static JWT)", hits)
	}
}

// providerApp returns an App that exchanges a stored session token against a
// token endpoint answering with tokenStatus. The API itself must never be hit.
func providerApp(t *testing.T, tokenStatus int) *App {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/token" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		http.Error(w, `{"message":"nope"}`, tokenStatus)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)

	app := newTestApp(t)
	store := auth.FileStore{Dir: t.TempDir()}
	if err := store.Save("default", "sess"); err != nil {
		t.Fatal(err)
	}
	app.newStore = func() auth.Store { return store }
	return app
}

// A rejected session reaches mapExit wrapped in *url.Error by the HTTP client;
// it must still classify as ExitAuth so the user is told to log in again.
func TestProvider_RejectedSessionExitsAuth(t *testing.T) {
	_, err := runApp(t, providerApp(t, http.StatusUnauthorized), "bin", "list")
	if got := mapExit(err); got != ExitAuth {
		t.Errorf("exit = %d (err %v), want ExitAuth(%d)", got, err, ExitAuth)
	}
}

// A token-endpoint outage is not a dead session: it must not tell the user to
// log in again.
func TestProvider_TokenServerErrorExitsError(t *testing.T) {
	_, err := runApp(t, providerApp(t, http.StatusInternalServerError), "bin", "list")
	if got := mapExit(err); got != ExitError {
		t.Errorf("exit = %d (err %v), want ExitError(%d)", got, err, ExitError)
	}
}
