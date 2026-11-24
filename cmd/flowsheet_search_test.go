package cmd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const searchBody = `{"results":[{"id":1,"play_date":"2026-11-24T01:00:00Z",` +
	`"artist_name":"Autechre","track_title":"Amber","album_title":"Amber",` +
	`"record_label":"Warp","show_id":42,"dj_name":"Ada","rotation_bin":null,` +
	`"request_flag":false,"on_streaming":true}],"total":1,"page":0,"totalPages":1,"nextCursor":"abc"}`

func searchServer(t *testing.T, got *url.Values) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/flowsheet/search" {
			t.Errorf("path = %q, want /flowsheet/search", r.URL.Path)
		}
		*got = r.URL.Query()
		w.Write([]byte(searchBody))
	}))
}

func TestFlowsheetSearch_MapsFlags(t *testing.T) {
	var got url.Values
	srv := searchServer(t, &got)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "search", "autechre",
		"--limit", "25", "--page", "2", "--sort", "artist", "--order", "asc", "--cursor", "tok")
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"q": "autechre", "limit": "25", "page": "2", "sort": "artist", "order": "asc", "cursor": "tok",
	} {
		if v := got.Get(key); v != want {
			t.Errorf("%s = %q, want %q", key, v, want)
		}
	}
	for _, want := range []string{"Autechre", "Amber", "Ada"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
}

func TestFlowsheetSearch_NoQueryOmitsQ(t *testing.T) {
	var got url.Values
	srv := searchServer(t, &got)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	if _, err := runCLI(t, "flowsheet", "search"); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["q"]; ok {
		t.Errorf("q = %q, want the parameter absent", got.Get("q"))
	}
	if got.Get("limit") != "50" || got.Get("sort") != "date" || got.Get("order") != "desc" {
		t.Errorf("defaults = %v", got)
	}
}

func TestFlowsheetSearch_RejectsOutOfRangeLimit(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	for _, limit := range []string{"0", "101"} {
		if _, err := runCLI(t, "flowsheet", "search", "--limit", limit); err == nil {
			t.Errorf("--limit %s: want a local error", limit)
		}
	}
}

func TestFlowsheetSearch_JSONPassesTheEnvelope(t *testing.T) {
	var got url.Values
	srv := searchServer(t, &got)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "--json", "flowsheet", "search", "amber")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"nextCursor"`) || !strings.Contains(out, `"on_streaming"`) {
		t.Errorf("--json should pass the full envelope:\n%s", out)
	}
}
