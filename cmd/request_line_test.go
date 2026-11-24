package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/safety"
)

// requestLineServer serves GET /flowsheet with onAir as the on_air value (a
// JSON literal; "" omits the key), records any PATCH, and after a PATCH serves
// onAirAfter instead, as the server would once the mode changed.
func requestLineServer(t *testing.T, onAir, onAirAfter string) (*httptest.Server, *struct {
	Patches int
	Path    string
	Body    map[string]any
}) {
	t.Helper()
	got := &struct {
		Patches int
		Path    string
		Body    map[string]any
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/flowsheet":
			v := onAir
			if got.Patches > 0 {
				v = onAirAfter
			}
			if v == "" {
				_, _ = w.Write([]byte(`{"entries":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"entries":[],"on_air":` + v + `}`))
		case r.Method == http.MethodPatch:
			got.Patches++
			got.Path = r.URL.Path
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &got.Body)
			_, _ = w.Write([]byte(`{"id":4812,"show_name":null}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
	return srv, got
}

const openShow = `{"dj_name":"DJ Lark","show_id":4812,"request_line":{"mode":"requests","source":"default"}}`

func TestFlowsheetRequestLine_PatchesEachMode(t *testing.T) {
	cases := []struct {
		arg      string
		wantBody any // the request_mode value; nil means JSON null
		after    string
	}{
		{"requests", "requests", `{"show_id":4812,"request_line":{"mode":"requests","source":"dj"}}`},
		{"feedback", "feedback", `{"show_id":4812,"request_line":{"mode":"feedback","source":"dj"}}`},
		{"closed", "closed", `{"show_id":4812,"request_line":{"mode":"closed","source":"dj"}}`},
		{"default", nil, `{"show_id":4812,"request_line":{"mode":"feedback","source":"program"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.arg, func(t *testing.T) {
			_, got := requestLineServer(t, openShow, tc.after)

			out, err := runCLI(t, "flowsheet", "request-line", tc.arg, "--write")
			if err != nil {
				t.Fatal(err)
			}
			if got.Patches != 1 || got.Path != "/flowsheet/shows/4812" {
				t.Errorf("patches = %d to %q, want 1 to /flowsheet/shows/4812", got.Patches, got.Path)
			}
			v, ok := got.Body["request_mode"]
			if len(got.Body) != 1 || !ok || v != tc.wantBody {
				t.Errorf("body = %v, want exactly {request_mode: %v}", got.Body, tc.wantBody)
			}
			var want struct {
				RequestLine struct{ Mode, Source string } `json:"request_line"`
			}
			_ = json.Unmarshal([]byte(tc.after), &want)
			if !strings.Contains(out, want.RequestLine.Mode) || !strings.Contains(out, want.RequestLine.Source) {
				t.Errorf("output missing the effective mode %+v:\n%s", want.RequestLine, out)
			}
		})
	}
}

func TestFlowsheetRequestLine_JSONIsTheReReadOnAir(t *testing.T) {
	after := `{"dj_name":"DJ Lark","show_id":4812,"request_line":{"mode":"closed","source":"dj"},"spotlight":null}`
	requestLineServer(t, openShow, after)

	out, err := runCLI(t, "flowsheet", "request-line", "closed", "--write", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if _, ok := got["spotlight"]; !ok || got["dj_name"] != "DJ Lark" {
		t.Errorf("--json = %v, want the on_air object verbatim", got)
	}
}

func TestFlowsheetRequestLine_RefusesBadArgumentBeforeRequest(t *testing.T) {
	for _, args := range [][]string{{"messages"}, {"Requests"}, {}, {"closed", "feedback"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var hits int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
			t.Cleanup(srv.Close)
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", "unused")

			_, err := runCLI(t, append([]string{"flowsheet", "request-line", "--write"}, args...)...)
			if err == nil || !strings.Contains(err.Error(), "requests, feedback, closed, default") {
				t.Errorf("err = %v, want it to name the valid modes", err)
			}
			if hits != 0 {
				t.Errorf("server hit %d times, want 0", hits)
			}
		})
	}
}

func TestFlowsheetRequestLine_NoOpenShow(t *testing.T) {
	for name, onAir := range map[string]string{"automation": "null", "unknown": ""} {
		t.Run(name, func(t *testing.T) {
			_, got := requestLineServer(t, onAir, onAir)

			_, err := runCLI(t, "flowsheet", "request-line", "closed", "--write")
			if err == nil {
				t.Fatal("err = nil, want a refusal")
			}
			if msg := err.Error(); strings.Contains(msg, "\n") || !strings.HasSuffix(msg, ".") {
				t.Errorf("err = %q, want one sentence", msg)
			}
			if mapExit(err) == ExitOK {
				t.Error("exit = 0, want non-zero")
			}
			if got.Patches != 0 {
				t.Errorf("patches = %d, want 0", got.Patches)
			}
		})
	}
}

func TestFlowsheetRequestLine_BlockedWithoutWrite(t *testing.T) {
	_, got := requestLineServer(t, openShow, openShow)
	t.Setenv("WXYC_ALLOW_WRITE", "")

	_, err := runCLI(t, "flowsheet", "request-line", "closed")
	var blocked *safety.BlockedError
	if !errors.As(err, &blocked) || mapExit(err) != ExitBlocked {
		t.Errorf("err = %v (exit %d), want *BlockedError / ExitBlocked", err, mapExit(err))
	}
	if got.Patches != 0 {
		t.Errorf("patches = %d, want 0", got.Patches)
	}
}

func TestFlowsheetRequestLine_ServerRefusals(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantMsg  string
		wantExit int
	}{
		{"ended show", http.StatusForbidden,
			`{"message":"Forbidden: this show has ended; only a music director or station manager may edit it","code":"closed_show"}`,
			"this show has ended", ExitForbidden},
		{"not on the show", http.StatusBadRequest,
			`{"message":"Bad Request: DJ not a member of the show this entry belongs to"}`,
			"you are not on the open show", ExitError},
		{"validation", http.StatusBadRequest,
			`{"message":"Bad Request: request_mode must be one of requests, feedback, closed"}`,
			"request_mode must be one of", ExitError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte(`{"entries":[],"on_air":` + openShow + `}`))
					return
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", "unused")

			_, err := runCLI(t, "flowsheet", "request-line", "closed", "--write")
			if err == nil {
				t.Fatal("err = nil, want a refusal")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.wantMsg) || strings.Contains(msg, "\n") || strings.Contains(msg, "{") {
				t.Errorf("err = %q, want one line containing %q and no raw JSON", msg, tc.wantMsg)
			}
			if got := mapExit(err); got != tc.wantExit {
				t.Errorf("exit = %d, want %d", got, tc.wantExit)
			}
		})
	}
}

func TestFlowsheetRequestLine_HelpNamesMessagesOnly(t *testing.T) {
	out, err := runCLI(t, "flowsheet", "request-line", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Messages only", "requests", "feedback", "closed", "default"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q:\n%s", want, out)
		}
	}
}
