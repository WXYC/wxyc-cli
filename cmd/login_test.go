package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/auth"
)

// signInServer stands in for better-auth: it records the sign-in path and
// body and answers with a session token in the set-auth-token header.
func signInServer(t *testing.T, token string) (*httptest.Server, *string, *map[string]string) {
	t.Helper()
	var path string
	body := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding sign-in body: %v", err)
		}
		w.Header().Set("set-auth-token", token)
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &path, &body
}

// loginApp is a hermetic App whose auth server, stdin and password reader
// are the given fakes, and whose session store is the returned temp dir.
func loginApp(t *testing.T, authURL, stdin, password string) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	app := newTestApp(t)
	app.getenv = func(k string) string {
		if k == "WXYC_AUTH_URL" {
			return authURL
		}
		return ""
	}
	app.newStore = func() auth.Store { return auth.FileStore{Dir: dir} }
	app.stdin = strings.NewReader(stdin)
	app.readPassword = func() ([]byte, error) { return []byte(password), nil }
	return app, dir
}

func TestLogin_SavesSessionTokenNotPassword(t *testing.T) {
	srv, path, body := signInServer(t, "sess-tok-123")
	app, dir := loginApp(t, srv.URL, "dj@wxyc.org\n", "hunter2")

	out, err := runApp(t, app, "login", "--profile", "night")
	if err != nil {
		t.Fatalf("login err = %v", err)
	}
	if *path != "/sign-in/email" || (*body)["email"] != "dj@wxyc.org" || (*body)["password"] != "hunter2" {
		t.Errorf("sign-in = %s %v, want /sign-in/email with the typed credentials", *path, *body)
	}
	if want := "signed in as dj@wxyc.org (profile \"night\")\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}

	got, err := auth.FileStore{Dir: dir}.Load("night")
	if err != nil || got != "sess-tok-123" {
		t.Errorf("stored session for night = %q, %v; want sess-tok-123", got, err)
	}
	// Nothing on disk may contain the password.
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if b, _ := os.ReadFile(p); strings.Contains(string(b), "hunter2") {
			t.Errorf("%s contains the password", p)
		}
		return nil
	})
}

// Prompts belong on stderr so stdout stays clean for scripts.
func TestLogin_PromptsGoToStderr(t *testing.T) {
	srv, _, _ := signInServer(t, "tok")
	app, _ := loginApp(t, srv.URL, "djname\n", "pw")

	out, err := runApp(t, app, "login")
	if err != nil {
		t.Fatal(err)
	}
	stderr := app.stderr.(interface{ String() string }).String()
	if !strings.Contains(stderr, "WXYC email or username: ") || !strings.Contains(stderr, "Password: ") {
		t.Errorf("stderr = %q, want both prompts", stderr)
	}
	if strings.Contains(out, "Password") {
		t.Errorf("stdout = %q, prompt leaked onto stdout", out)
	}
}

// fmt.Scanln stopped at the first space; a username is the whole line.
func TestLogin_IdentifierMayContainSpaces(t *testing.T) {
	srv, path, body := signInServer(t, "tok")
	app, _ := loginApp(t, srv.URL, "  DJ Night Owl \r\n", "pw")

	if _, err := runApp(t, app, "login"); err != nil {
		t.Fatal(err)
	}
	if *path != "/sign-in/username" || (*body)["username"] != "DJ Night Owl" {
		t.Errorf("sign-in = %s %v, want username %q", *path, *body, "DJ Night Owl")
	}
}

func TestLogin_EmptyIdentifierIsRejected(t *testing.T) {
	app, _ := loginApp(t, "http://unused.invalid", "\n", "pw")
	app.readPassword = func() ([]byte, error) {
		t.Error("asked for a password after an empty username")
		return nil, nil
	}

	_, err := runApp(t, app, "login")
	if err == nil || err.Error() != "an email or username is required" {
		t.Errorf("err = %v, want %q", err, "an email or username is required")
	}
}

func TestLogin_PasswordReadErrorIsReported(t *testing.T) {
	app, _ := loginApp(t, "http://unused.invalid", "dj\n", "")
	app.readPassword = func() ([]byte, error) { return nil, errors.New("not a terminal") }

	_, err := runApp(t, app, "login")
	if err == nil || err.Error() != "reading password: not a terminal" {
		t.Errorf("err = %v, want %q", err, "reading password: not a terminal")
	}
}

// The username line must be read without buffering past its newline, so
// nothing after it is swallowed from the shared stdin.
func TestReadLine_StopsAtNewline(t *testing.T) {
	r := strings.NewReader("first\nsecond\n")
	got, err := readLine(r)
	if err != nil || got != "first" {
		t.Fatalf("readLine = %q, %v; want first", got, err)
	}
	rest, _ := readLine(r)
	if rest != "second" {
		t.Errorf("second readLine = %q, want second", rest)
	}
}
