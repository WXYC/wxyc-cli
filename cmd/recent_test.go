package cmd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// recentFixture mirrors GET /flowsheet/shows/recent's real response
// (Backend-Service flowsheet.service.ts getRecentShows, BS#2435): newest
// first; show 202 is still on the air with a DJ whose handle was filtered to
// null; show 201 is a legacy show whose one DJ has id: null; show 200 has no
// resolvable DJ at all.
const recentFixture = `{"shows":[` +
	`{"id":202,"show_name":"Night Moves","start_time":"2026-11-24T22:00:00.000Z","end_time":null,` +
	`"djs":[{"id":"u_ryan","dj_name":"DJ Ryan"},{"id":"u_anon","dj_name":null}]},` +
	`{"id":201,"show_name":null,"start_time":"2026-11-24T20:00:00.000Z","end_time":"2026-11-24T22:00:00.000Z",` +
	`"djs":[{"id":null,"dj_name":"Legacy Larry"}]},` +
	`{"id":200,"show_name":"Ghost Hour","start_time":"2026-11-24T19:00:00.000Z","end_time":"2026-11-24T20:00:00.000Z","djs":[]}]}`

func recentServer(t *testing.T, query *url.Values, hits *int32) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		if r.URL.Path != "/flowsheet/shows/recent" {
			t.Errorf("path = %q, want /flowsheet/shows/recent", r.URL.Path)
		}
		if query != nil {
			*query = r.URL.Query()
		}
		w.Write([]byte(recentFixture))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
}

func TestFlowsheetRecent_SendsWindowHours(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "window_hours=24"},
		{[]string{"--hours", "1"}, "window_hours=1"},
		{[]string{"--hours", "168"}, "window_hours=168"},
	} {
		var q url.Values
		var hits int32
		recentServer(t, &q, &hits)
		if _, err := runCLI(t, append([]string{"flowsheet", "recent"}, tc.args...)...); err != nil {
			t.Fatal(err)
		}
		if got := q.Encode(); got != tc.want {
			t.Errorf("%v: query = %s, want %s", tc.args, got, tc.want)
		}
	}
}

func TestFlowsheetRecent_OutOfRangeSendsNothing(t *testing.T) {
	for _, h := range []string{"0", "169", "-1"} {
		var hits int32
		recentServer(t, nil, &hits)
		_, err := runCLI(t, "flowsheet", "recent", "--hours", h)
		if err == nil || !strings.Contains(err.Error(), "1 to 168") {
			t.Errorf("--hours %s: err = %v, want a 1 to 168 error", h, err)
		}
		if hits != 0 {
			t.Errorf("--hours %s: %d requests sent, want 0", h, hits)
		}
	}
}

func TestFlowsheetRecent_Table(t *testing.T) {
	app := zonedApp(t, mustZone(t, "America/New_York"), time.Date(2026, 11, 24, 23, 0, 0, 0, time.UTC))
	var hits int32
	recentServer(t, nil, &hits)
	out, err := runApp(t, app, "flowsheet", "recent")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("want header + 3 rows:\n%s", out)
	}
	for _, want := range []string{"ID", "START", "END", "SHOW", "DJS"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("header missing %s: %q", want, lines[0])
		}
	}
	for i, want := range [][]string{
		{"202", "2026-11-24 17:00", "on air", "Night Moves", "DJ Ryan, ?"},
		{"201", "2026-11-24 15:00", "2026-11-24 17:00", "Legacy Larry"},
		{"200", "2026-11-24 14:00", "2026-11-24 15:00", "Ghost Hour"},
	} {
		for _, w := range want {
			if !strings.Contains(lines[i+1], w) {
				t.Errorf("row %d missing %q: %q", i+1, w, lines[i+1])
			}
		}
	}
}

func TestFlowsheetRecent_JSONIsServerBody(t *testing.T) {
	var hits int32
	recentServer(t, nil, &hits)
	out, err := runCLI(t, "--json", "flowsheet", "recent")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"shows"`, `"djs"`, `"id": null`, `"dj_name": null`, `"u_anon"`} {
		if !strings.Contains(out, want) {
			t.Errorf("--json dropped %s:\n%s", want, out)
		}
	}
}
