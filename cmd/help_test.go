package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
)

// helpOf returns the --help output of the command at path.
func helpOf(t *testing.T, path ...string) string {
	t.Helper()
	out, err := runCLI(t, append(path, "--help")...)
	if err != nil {
		t.Fatalf("%v --help: %v", path, err)
	}
	return out
}

// The root help is the only page an agent is sure to read, so it must carry
// the scripting contract the README documents: the environment variables and
// every exit code mapExit can return.
func TestRootHelp_NamesEnvVarsAndExitCodes(t *testing.T) {
	out := helpOf(t)
	for _, want := range []string{
		"WXYC_JWT", "WXYC_API_URL", "WXYC_AUTH_URL", "WXYC_PROFILE", "WXYC_ALLOW_WRITE",
		"  0    success\n", "  1    unclassified error\n", "  2    write blocked",
		"  3    not authenticated", "  4    forbidden", "  5    not found\n", "  130  interrupted",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("root help missing %q:\n%s", want, out)
		}
	}
}

// --profile names its env var the way --write does, and doesn't claim a
// hard-coded default that WXYC_PROFILE silently overrides.
func TestProfileFlag_NamesEnvVar(t *testing.T) {
	out := helpOf(t)
	const want = `--profile string   credential profile (also WXYC_PROFILE; default "default")`
	if !strings.Contains(out, want) {
		t.Errorf("root help missing %q:\n%s", want, out)
	}
}

// walk visits every command under c.
func walk(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		walk(sub, fn)
	}
}

// Every mutating command, and only those, is tagged [write] in its summary,
// so a parent's command list shows which subcommands need --write.
func TestWriteTag_MarksExactlyTheMutatingCommands(t *testing.T) {
	root := newRoot(newTestApp(t))
	var writes int
	walk(root, func(c *cobra.Command) {
		mutates := c.Annotations[annMutates] == "true"
		tagged := strings.HasPrefix(c.Short, writeTag)
		switch {
		case mutates && !tagged:
			t.Errorf("%q mutates but Short %q lacks %q", c.CommandPath(), c.Short, writeTag)
		case !mutates && strings.Contains(c.Short, "[write]"):
			t.Errorf("%q is read-only but Short %q is tagged", c.CommandPath(), c.Short)
		case strings.Count(c.Short, "[write]") > 1:
			t.Errorf("%q Short %q tagged twice", c.CommandPath(), c.Short)
		}
		if mutates {
			writes++
		}
	})
	if writes != 12 {
		t.Errorf("found %d mutating commands, want 12 (bin add/rm + 10 flowsheet)", writes)
	}

	out := helpOf(t, "flowsheet")
	for _, want := range []string{
		"start        [write] Start your show",
		"now          Show what's on air right now",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("flowsheet help missing %q:\n%s", want, out)
		}
	}
}

// Requirements the commands enforce must be stated in their help.
func TestHelp_StatesEnforcedRequirements(t *testing.T) {
	cases := []struct {
		path []string
		want []string
	}{
		{[]string{"flowsheet", "add"}, []string{
			"--artist string     artist name (required unless --album-id)",
			"--album string      album title (required unless --album-id)",
		}},
		{[]string{"library", "search"}, []string{
			"Give at least one of --artist, --album or --on-streaming.",
		}},
		{[]string{"flowsheet", "tail"}, []string{
			"number of entries, 1 to 200",
		}},
		{[]string{"bin", "rm"}, []string{
			"Exits 5 if the album is not in your bin.",
		}},
		{[]string{"djs", "playlists"}, []string{
			`the "sub" that "wxyc whoami" shows`,
		}},
		{[]string{"flowsheet", "show"}, []string{
			"flowsheet recent", "flowsheet range", "djs playlists",
		}},
	}
	for _, tc := range cases {
		out := helpOf(t, tc.path...)
		for _, want := range tc.want {
			if !strings.Contains(out, want) {
				t.Errorf("%v help missing %q:\n%s", tc.path, want, out)
			}
		}
	}
}

// The server 400s a library search with no filter; refuse it locally,
// sending nothing.
func TestLibrarySearch_NoFilterRefusedBeforeRequest(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"Missing query parameter. Query must include: artist_name, album_title, or on_streaming"}`))
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	_, err := runCLI(t, "library", "search")
	if want := "give at least one of --artist, --album or --on-streaming"; err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
	if hits != 0 {
		t.Errorf("server hit %d times, want 0", hits)
	}
}

// The server refuses more than 200 entries (MAX_ITEMS); refuse it locally.
func TestFlowsheetTail_RejectsLimitAboveMax(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	_, err := runCLI(t, "flowsheet", "tail", "-n", "201")
	if want := "--limit must be at most 200, got 201"; err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}
