package cmd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	_ "time/tzdata" // real zones (with DST) for the parsing table, on any host
)

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestRangeWindow_ParsesTimes(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	tokyo := mustZone(t, "Asia/Tokyo")
	// "now" is 2026-11-24 15:30 in New York (20:30Z).
	now := time.Date(2026, 11, 24, 15, 30, 0, 0, ny)
	for _, tc := range []struct {
		name         string
		since, until string
		loc          *time.Location
		start, end   int64 // epoch ms sent to the server
	}{
		{"rfc3339 utc", "2026-11-20T22:00:00Z", "2026-11-21T00:00:00Z", ny, 1795212000000, 1795219200000},
		{"rfc3339 offsets", "2026-11-20T17:00:00-05:00", "2026-11-21T09:00:00+09:00", ny, 1795212000000, 1795219200000},
		{"rfc3339 fractional", "2026-11-20T22:00:00.25Z", "2026-11-21T00:00:00Z", ny, 1795212000250, 1795219200000},
		{"bare date alone is that local day", "2026-11-20", "", ny, 1795150800000, 1795237200000},
		{"bare until is the end of that day", "2026-11-20", "2026-11-22", ny, 1795150800000, 1795410000000},
		{"same bare date twice is one day", "2026-11-20", "2026-11-20", ny, 1795150800000, 1795237200000},
		{"bare date in another zone", "2026-11-20", "", tokyo, 1795132800000 - 9*3600000, 1795132800000 + 15*3600000},
		{"bare date in utc", "2026-11-20", "", time.UTC, 1795132800000, 1795132800000 + 24*3600000},
		{"dst fall-back day is 25 hours", "2026-11-01", "", ny, 1793505600000, 1793595600000},
		{"local time without zone", "2026-11-20T09:30", "2026-11-20 10:30:15", ny, 1795185000000, 1795185000000 + 3615000},
		{"timestamp alone runs to now", "2026-11-24T00:00:00-05:00", "", ny, 1795496400000, 1795552200000},
		{"today", "today", "", ny, 1795496400000, 1795582800000},
		{"yesterday", "yesterday", "", ny, 1795410000000, 1795496400000},
		{"yesterday until now", "yesterday", "now", ny, 1795410000000, 1795552200000},
		{"exactly 8 days is allowed", "2026-11-17", "2026-11-24", ny, 1795496400000 - 7*86400000, 1795582800000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end, err := rangeWindow(tc.since, tc.until, tc.loc, now)
			if err != nil {
				t.Fatal(err)
			}
			if start != tc.start || end != tc.end {
				t.Errorf("window = [%d, %d), want [%d, %d)", start, end, tc.start, tc.end)
			}
		})
	}
}

func TestRangeWindow_RefusesBadWindows(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	now := time.Date(2026, 11, 24, 15, 30, 0, 0, ny)
	for _, tc := range []struct{ name, since, until, want string }{
		{"no since", "", "", "--since is required"},
		{"garbage since", "last tuesday", "", `--since: can't parse "last tuesday"`},
		{"garbage until", "2026-11-20", "2026-13-01", `--until: can't parse "2026-13-01"`},
		{"end before start", "2026-11-21T00:00:00Z", "2026-11-20T00:00:00Z", "--until must be after --since"},
		{"empty window", "2026-11-20T00:00:00Z", "2026-11-20T00:00:00Z", "--until must be after --since"},
		{"bare until before since", "2026-11-21", "2026-11-20", "--until must be after --since"},
		{"nine days", "2026-11-10", "2026-11-18", "window is 9 days; the most is 8"},
		// 8 calendar days, but the fall-back hour makes it 8d1h: the server
		// counts milliseconds, so this is too long.
		{"8 days across dst", "2026-10-26", "2026-11-02", "the most is 8"},
		{"since in the future runs to an earlier now", "2026-11-25T00:00:00-05:00", "", "--until must be after --since"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := rangeWindow(tc.since, tc.until, ny, now)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// rangeFixture mirrors GET /flowsheet/range's real response shape
// (Backend-Service flowsheet.controller.ts getEntriesInRange): entries in
// add_time order across two shows; show 101's entries are out of play order
// (entry 3 was moved up after it was logged).
const rangeFixture = `{"shows":[` +
	`{"id":100,"show_name":null,"dj_name":"DJ Early","specialty_id":null,"specialty":null,` +
	`"start_time":"2026-11-20T20:00:00.000Z","end_time":"2026-11-20T22:00:00.000Z"},` +
	`{"id":101,"show_name":"Night Moves","dj_name":"DJ Ryan","specialty_id":null,"specialty":null,` +
	`"start_time":"2026-11-20T22:00:00.000Z","end_time":null}],` +
	`"entries":[` +
	`{"id":1,"show_id":100,"play_order":7,"add_time":"2026-11-20T21:58:00.000Z","entry_type":"track",` +
	`"artist_name":"Cat Power","track_title":"Cross Bones Style","album_title":"Moon Pix","record_label":"Matador"},` +
	`{"id":2,"show_id":101,"play_order":1,"add_time":"2026-11-20T22:00:01.000Z","entry_type":"show_start","dj_name":"DJ Ryan","message":"Start of Show"},` +
	`{"id":3,"show_id":101,"play_order":3,"add_time":"2026-11-20T22:04:00.000Z","entry_type":"track",` +
	`"artist_name":"Stereolab","track_title":"French Disko","album_title":"Jenny Ondioline","record_label":"Duophonic"},` +
	`{"id":4,"show_id":101,"play_order":2,"add_time":"2026-11-20T22:08:00.000Z","entry_type":"track",` +
	`"artist_name":"Broadcast","track_title":"Pendulum","album_title":"Haha Sound","record_label":"Warp"}]}`

func rangeServer(t *testing.T, query *url.Values, hits *int32) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		if r.URL.Path != "/flowsheet/range" {
			t.Errorf("path = %q, want /flowsheet/range", r.URL.Path)
		}
		if query != nil {
			*query = r.URL.Query()
		}
		w.Write([]byte(rangeFixture))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
}

// zonedApp is a test App whose local zone and clock are pinned. They live on
// the App, not in package state, so tests that pin them can run in parallel.
func zonedApp(t *testing.T, loc *time.Location, now time.Time) *App {
	t.Helper()
	app := newTestApp(t)
	app.loc, app.now = loc, func() time.Time { return now }
	return app
}

func TestFlowsheetRange_SendsEpochMillis(t *testing.T) {
	app := zonedApp(t, mustZone(t, "America/New_York"), time.Date(2026, 11, 24, 15, 30, 0, 0, time.UTC))
	var q url.Values
	var hits int32
	rangeServer(t, &q, &hits)
	if _, err := runApp(t, app, "flowsheet", "range", "--since", "2026-11-20"); err != nil {
		t.Fatal(err)
	}
	if got := q.Encode(); got != "end=1795237200000&start=1795150800000" {
		t.Errorf("query = %s", got)
	}
}

func TestFlowsheetRange_BadWindowSendsNothing(t *testing.T) {
	app := zonedApp(t, time.UTC, time.Date(2026, 11, 24, 15, 30, 0, 0, time.UTC))
	for _, args := range [][]string{
		{"--since", "2026-11-01", "--until", "2026-11-20"},
		{"--since", "2026-11-20", "--until", "2026-11-19"},
		{"--since", "soon"},
		{},
	} {
		var hits int32
		rangeServer(t, nil, &hits)
		if _, err := runApp(t, app, append([]string{"flowsheet", "range"}, args...)...); err == nil {
			t.Errorf("%v: want error", args)
		}
		if hits != 0 {
			t.Errorf("%v: %d requests sent, want 0", args, hits)
		}
	}
}

func TestFlowsheetRange_TableLabelsShowsInPlayOrder(t *testing.T) {
	app := zonedApp(t, mustZone(t, "America/New_York"), time.Date(2026, 11, 24, 15, 30, 0, 0, time.UTC))
	var hits int32
	rangeServer(t, nil, &hits)
	out, err := runApp(t, app, "flowsheet", "range", "--since", "2026-11-20")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 5 {
		t.Fatalf("want header + 4 rows:\n%s", out)
	}
	for _, want := range []string{"TIME", "SHOW", "TYPE", "ARTIST", "TRACK", "ALBUM"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("header missing %s: %q", want, lines[0])
		}
	}
	for i, want := range [][]string{
		{"2026-11-20 16:58", "100 DJ Early", "track", "Cat Power", "Cross Bones Style"},
		{"2026-11-20 17:00", "101 DJ Ryan: Night Moves", "show_start", "DJ Ryan"},
		// Within show 101, play order wins over logging time.
		{"2026-11-20 17:08", "101 DJ Ryan: Night Moves", "track", "Broadcast", "Pendulum"},
		{"2026-11-20 17:04", "101 DJ Ryan: Night Moves", "track", "Stereolab", "French Disko"},
	} {
		for _, w := range want {
			if !strings.Contains(lines[i+1], w) {
				t.Errorf("row %d missing %q: %q", i+1, w, lines[i+1])
			}
		}
	}
}

func TestFlowsheetRange_JSONIsServerBody(t *testing.T) {
	app := zonedApp(t, time.UTC, time.Date(2026, 11, 24, 15, 30, 0, 0, time.UTC))
	var hits int32
	rangeServer(t, nil, &hits)
	out, err := runApp(t, app, "--json", "flowsheet", "range", "--since", "2026-11-20")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"shows"`, `"entries"`, `"specialty_id"`, `"record_label"`, `"message"`} {
		if !strings.Contains(out, want) {
			t.Errorf("--json dropped %s:\n%s", want, out)
		}
	}
}
