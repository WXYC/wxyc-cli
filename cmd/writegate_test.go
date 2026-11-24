package cmd

import (
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/safety"
	"github.com/WXYC/wxyc-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// gatedCommands is every command that must be blocked by the write gate,
// keyed by command path (minus "wxyc "), with valid args that would
// otherwise send a request, the op the BlockedError must name, and the write
// request ("METHOD /path") the unlocked command sends. Adding or removing a
// mutating command is a deliberate change to this table.
var gatedCommands = map[string]struct {
	args  []string
	op    string
	write string
}{
	"bin add":                {[]string{"45029"}, "bin:add", "POST /djs/bin"},
	"bin rm":                 {[]string{"45029"}, "bin:rm", "DELETE /djs/bin"},
	"flowsheet start":        {nil, "flowsheet:start", "POST /flowsheet/join"},
	"flowsheet add":          {[]string{"--track", "T", "--artist", "A", "--album", "B"}, "flowsheet:add", "POST /flowsheet"},
	"flowsheet talkset":      {nil, "flowsheet:talkset", "POST /flowsheet"},
	"flowsheet breakpoint":   {nil, "flowsheet:breakpoint", "POST /flowsheet"},
	"flowsheet spotlight":    {[]string{"--role", "performer", "The", "Weirs"}, "flowsheet:spotlight", "POST /flowsheet"},
	"flowsheet request-line": {[]string{"closed"}, "flowsheet:request-line", "PATCH /flowsheet/shows/4812"},
	"flowsheet move":         {[]string{"501", "2"}, "flowsheet:move", "PATCH /flowsheet/play-order"},
	"flowsheet edit":         {[]string{"501", "--artist", "Boards"}, "flowsheet:edit", "PATCH /flowsheet"},
	"flowsheet rm":           {[]string{"501"}, "flowsheet:rm", "DELETE /flowsheet"},
	"flowsheet end":          {nil, "flowsheet:end", "POST /flowsheet/end"},
}

// The set of commands carrying the mutates annotation must be exactly
// gatedCommands, so an annotated write can't ship without a blocked-by-default
// test. A command that names an op but forgets annMutates would slip past the
// gate entirely, so naming an op without it fails too. A write with neither
// annotation is invisible here; that one is on review.
func TestEveryMutatingCommandIsGated(t *testing.T) {
	got := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		path := strings.TrimPrefix(c.CommandPath(), "wxyc ")
		if c.Annotations[annMutates] == "true" {
			got[path] = true
		} else if op, ok := c.Annotations[annOp]; ok {
			t.Errorf("%q names op %q but lacks %s=true, so the gate lets it through", path, op, annMutates)
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
			t.Setenv("WXYC_JWT", testutil.FakeJWT("dj-42"))

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
			if n := atomic.LoadInt32(&hits); n != 0 {
				t.Errorf("server hit %d times, want 0", n)
			}
		})
	}
}

// The table's args must be valid: with --write each command gets past arg
// validation and sends its write, so the blocked test above is proving the
// gate stopped it, not a usage error. Only the write counts: a read sent
// before a failed validation (e.g. a bad --specialty slug) would not.
func TestWriteGate_GatedCommandArgsAreValid(t *testing.T) {
	for path, tc := range gatedCommands {
		t.Run(path, func(t *testing.T) {
			var mu sync.Mutex
			var writes []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					// request-line reads the on-air show before its PATCH.
					if r.URL.Path == "/flowsheet" {
						_, _ = w.Write([]byte(`{"entries":[],"on_air":` + openShow + `}`))
						return
					}
					w.WriteHeader(http.StatusTeapot)
					return
				}
				mu.Lock()
				writes = append(writes, r.Method+" "+r.URL.Path)
				mu.Unlock()
				w.WriteHeader(http.StatusTeapot)
			}))
			defer srv.Close()
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", testutil.FakeJWT("dj-42"))

			args := append(append(strings.Fields(path), tc.args...), "--write")
			runCLI(t, args...) // the teapot response errors; only sending the write matters
			mu.Lock()
			defer mu.Unlock()
			if !slices.Contains(writes, tc.write) {
				t.Errorf("unlocked %q sent writes %v, want %q; its table args are invalid", path, writes, tc.write)
			}
		})
	}
}
