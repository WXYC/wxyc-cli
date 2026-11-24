package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// execute is the whole of Execute except wiring to the real process: the
// "error: …" line goes to the App's stderr, nothing to stdout, and the
// returned code is the CLI's machine-readable contract.
func TestExecute_ErrorLineAndExitCode(t *testing.T) {
	cases := []struct {
		name    string
		status  int // server reply; 0 = no server needed
		jwt     string
		args    []string
		code    int
		wantErr string
	}{
		{"not found", http.StatusNotFound, "x", []string{"schedule"}, ExitNotFound,
			"error: /schedule: HTTP 404: nope\n"},
		{"forbidden", http.StatusForbidden, "x", []string{"schedule"}, ExitForbidden,
			"error: /schedule: HTTP 403: nope\n"},
		{"unauthenticated", http.StatusUnauthorized, "x", []string{"schedule"}, ExitAuth,
			"error: /schedule: HTTP 401: nope\n"},
		{"write blocked", 0, "x", []string{"bin", "add", "1"}, ExitBlocked,
			"error: \"bin:add\" mutates state and the CLI is read-only; re-run with --write (interactive) or WXYC_ALLOW_WRITE=1 (scripted) to permit\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(t)
			env := map[string]string{"WXYC_JWT": tc.jwt}
			if tc.status != 0 {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.status)
					w.Write([]byte("nope"))
				}))
				t.Cleanup(srv.Close)
				env["WXYC_API_URL"] = srv.URL
			}
			trap := app.getenv
			app.getenv = func(k string) string {
				if v, ok := env[k]; ok {
					return v
				}
				return trap(k)
			}
			var out, errb bytes.Buffer
			app.stdout, app.stderr = &out, &errb

			code := execute(context.Background(), app, tc.args)

			if code != tc.code {
				t.Errorf("code = %d, want %d", code, tc.code)
			}
			if errb.String() != tc.wantErr {
				t.Errorf("stderr = %q, want %q", errb.String(), tc.wantErr)
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want empty", out.String())
			}
		})
	}
}

func TestExecute_SuccessIsZeroAndQuiet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "x")
	app := newTestApp(t)
	var out, errb bytes.Buffer
	app.stdout, app.stderr = &out, &errb

	if code := execute(context.Background(), app, []string{"schedule", "--json"}); code != ExitOK {
		t.Errorf("code = %d, want 0", code)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errb.String())
	}
}
