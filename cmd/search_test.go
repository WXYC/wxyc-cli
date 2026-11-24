package cmd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// searchFixture mirrors GET /flowsheet/search's real response shape
// (Backend-Service search.controller.ts / search.service.ts).
const searchFixture = `{"results":[` +
	`{"id":9912,"play_date":"2026-11-20T22:14:05.123Z","artist_name":"Stereolab",` +
	`"track_title":"French Disko","album_title":"Jenny Ondioline","record_label":"Duophonic",` +
	`"show_id":1950333,"dj_name":"DJ Ryan","rotation_bin":"H","request_flag":false,"on_streaming":null}` +
	`],"total":120,"page":0,"totalPages":3,"nextCursor":"MjAyNi0xMS0yMFQyMjoxNDowNS4xMjMxMjNa.9912.word"}`

func searchServer(t *testing.T, query *url.Values, hits *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		if r.URL.Path != "/flowsheet/search" {
			t.Errorf("path = %q, want /flowsheet/search", r.URL.Path)
		}
		if query != nil {
			*query = r.URL.Query()
		}
		w.Write([]byte(searchFixture))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
	return srv
}

func TestFlowsheetSearch_FlagsMapToQueryString(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string // url.Values.Encode() of the expected query
	}{
		{"no query sends nothing", nil, ""},
		{"query only", []string{"stereolab"}, "q=stereolab"},
		{"multi-word query joined", []string{"french", "disko"}, "q=french+disko"},
		{"all flags", []string{"stereolab", "--limit", "100", "--page", "2", "--sort", "artist", "--order", "asc"},
			"limit=100&order=asc&page=2&q=stereolab&sort=artist"},
		{"short limit", []string{"-n", "1"}, "limit=1"},
		{"page zero explicit", []string{"--page", "0"}, "page=0"},
		{"cursor", []string{"x", "--cursor", "abc.1.word"}, "cursor=abc.1.word&q=x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var q url.Values
			searchServer(t, &q, nil)
			if _, err := runCLI(t, append([]string{"flowsheet", "search"}, tc.args...)...); err != nil {
				t.Fatal(err)
			}
			if got := q.Encode(); got != tc.want {
				t.Errorf("query = %q, want %q", got, tc.want)
			}
		})
	}
}

// The server silently falls back to date/desc for an unknown sort/order and
// 400s on limit outside 1..100 or a negative page; the CLI refuses all of
// these locally, before any request.
func TestFlowsheetSearch_BadFlagsRefusedBeforeRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"sort", []string{"--sort", "album"}, "--sort must be one of date, artist, song, dj"},
		{"order", []string{"--order", "up"}, "--order must be asc or desc"},
		{"limit high", []string{"--limit", "101"}, "--limit must be from 1 to 100"},
		{"limit zero", []string{"--limit", "0"}, "--limit must be from 1 to 100"},
		{"page negative", []string{"--page", "-1"}, "--page must be 0 or more"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits int32
			searchServer(t, nil, &hits)
			_, err := runCLI(t, append([]string{"flowsheet", "search"}, tc.args...)...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
			if hits != 0 {
				t.Errorf("server hit %d times, want 0", hits)
			}
		})
	}
}

func TestFlowsheetSearch_Table(t *testing.T) {
	searchServer(t, nil, nil)
	out, err := runCLI(t, "flowsheet", "search", "stereolab")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"DATE", "DJ", "ARTIST", "TRACK", "ALBUM",
		"2026-11-20T22:14:05.123Z", "DJ Ryan", "Stereolab", "French Disko", "Jenny Ondioline",
		"page 1 of 3 (120 results)", "--cursor MjAyNi0xMS0yMFQyMjoxNDowNS4xMjMxMjNa.9912.word"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
}

func TestFlowsheetSearch_JSONIsRawPassthrough(t *testing.T) {
	searchServer(t, nil, nil)
	out, err := runCLI(t, "--json", "flowsheet", "search", "stereolab")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"record_label"`, `"rotation_bin"`, `"on_streaming"`, `"totalPages"`, `"nextCursor"`} {
		if !strings.Contains(out, want) {
			t.Errorf("--json missing %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "page 1 of") {
		t.Errorf("--json carries the table footer:\n%s", out)
	}
}

// total is capped at COUNT_CAP+1 (10001) as a "10000+" sentinel (BS#1681),
// and in cursor mode the server's page is always 0, so the footer must not
// claim a page number there.
func TestFlowsheetSearch_Footer(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		page, totalPages, total int
		cursorMode              bool
		next, want, notWant     string
	}{
		{"offset", 1, 3, 120, false, "", "page 2 of 3 (120 results)", "--cursor"},
		{"capped total", 0, 201, 10001, false, "c", "page 1 of 201 (10000+ results)\nnext page: --cursor c", "10001"},
		{"cursor mode", 0, 201, 10001, true, "c", "10000+ results\nnext page: --cursor c", "page 1"},
		{"empty", 0, 0, 0, false, "", "page 1 of 1 (0 results)", "--cursor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := searchFooter(tc.page, tc.totalPages, tc.total, tc.cursorMode, tc.next)
			if !strings.Contains(got, tc.want) {
				t.Errorf("footer = %q, want %q", got, tc.want)
			}
			if strings.Contains(got, tc.notWant) {
				t.Errorf("footer = %q, must not contain %q", got, tc.notWant)
			}
		})
	}
}
