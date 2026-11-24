package cmd

import (
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/safety"
	"github.com/spf13/cobra"
)

// gatedCommands is every command that must be blocked by the write gate,
// keyed by command path (minus "wxyc "), with valid args that would
// otherwise send a request, and the op the BlockedError must name. Adding
// or removing a mutating command is a deliberate change to this table.
var gatedCommands = map[string]struct {
	args []string
	op   string
}{
	"bin add":                {[]string{"45029"}, "bin:add"},
	"bin rm":                 {[]string{"45029"}, "bin:rm"},
	"flowsheet start":        {nil, "flowsheet:start"},
	"flowsheet add":          {[]string{"--track", "T", "--artist", "A", "--album", "B"}, "flowsheet:add"},
	"flowsheet talkset":      {nil, "flowsheet:talkset"},
	"flowsheet breakpoint":   {nil, "flowsheet:breakpoint"},
	"flowsheet spotlight":    {[]string{"--role", "performer", "The", "Weirs"}, "flowsheet:spotlight"},
	"flowsheet request-line": {[]string{"closed"}, "flowsheet:request-line"},
	"flowsheet move":         {[]string{"501", "2"}, "flowsheet:move"},
	"flowsheet edit":         {[]string{"501", "--artist", "Boards"}, "flowsheet:edit"},
	"flowsheet rm":           {[]string{"501"}, "flowsheet:rm"},
	"flowsheet end":          {nil, "flowsheet:end"},
}

// The set of commands carrying the mutates annotation must be exactly
// gatedCommands, so a new write can't ship without a blocked-by-default test.
func TestEveryMutatingCommandIsGated(t *testing.T) {
	got := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Annotations[annMutates] == "true" {
			got[strings.TrimPrefix(c.CommandPath(), "wxyc ")] = true
		}
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk(newRoot(&App{}))

	gotKeys := slices.Sorted(maps.Keys(got))
	wantKeys := slices.Sorted(maps.Keys(gatedCommands))
	if !slices.Equal(gotKeys, wantKeys) {
		t.Errorf("gated commands = %v, want %v", gotKeys, wantKeys)
	}
}

// Every mutating command, given valid args but no --write, must be refused
// by the gate with exit 2 before any request is sent.
func TestWriteGate_BlocksEveryMutatingCommand(t *testing.T) {
	for path, tc := range gatedCommands {
		t.Run(path, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				t.Errorf("request sent despite locked gate: %s %s", r.Method, r.URL)
				w.WriteHeader(http.StatusTeapot)
			}))
			defer srv.Close()
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

			args := append(strings.Fields(path), tc.args...)
			_, err := runCLI(t, args...)
			var blocked *safety.BlockedError
			if !errors.As(err, &blocked) {
				t.Fatalf("err = %v, want *BlockedError", err)
			}
			if blocked.Op != tc.op {
				t.Errorf("op = %q, want %q", blocked.Op, tc.op)
			}
			if mapExit(err) != ExitBlocked {
				t.Errorf("exit = %d, want ExitBlocked(%d)", mapExit(err), ExitBlocked)
			}
			if hits != 0 {
				t.Errorf("server hit %d times, want 0", hits)
			}
		})
	}
}

// The table's args must be valid: with --write each command gets past arg
// validation and reaches the server, so the blocked test above is proving
// the gate stopped it, not a usage error.
func TestWriteGate_GatedCommandArgsAreValid(t *testing.T) {
	for path, tc := range gatedCommands {
		t.Run(path, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				w.WriteHeader(http.StatusTeapot)
			}))
			defer srv.Close()
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

			args := append(append(strings.Fields(path), tc.args...), "--write")
			runCLI(t, args...) // the teapot response errors; only reaching the server matters
			if hits == 0 {
				t.Errorf("unlocked %q sent no request; its table args are invalid", path)
			}
		})
	}
}
