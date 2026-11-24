package cmd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseWindowTime(t *testing.T) {
	rfc, err := parseWindowTime("2026-11-24T01:00:00Z", false)
	if err != nil || !rfc.Equal(time.Date(2026, 11, 24, 1, 0, 0, 0, time.UTC)) {
		t.Errorf("RFC3339 = %v, %v", rfc, err)
	}

	start, err := parseWindowTime("2026-11-24", false)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 24, 0, 0, 0, 0, time.Local); !start.Equal(want) {
		t.Errorf("start = %v, want %v", start, want)
	}

	end, err := parseWindowTime("2026-11-24", true)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 25, 0, 0, 0, 0, time.Local); !end.Equal(want) {
		t.Errorf("end = %v, want %v", end, want)
	}

	if _, err := parseWindowTime("yesterday", false); err == nil {
		t.Error("want an error for an unparseable time")
	}
}

func rangeServer(t *testing.T, got *url.Values) *httptest.Server {
	t.Helper()
	body := `{"shows":[{"id":42,"show_name":"Freeform"}],` +
		`"entries":[{"id":1,"show_id":42,"play_order":1,"entry_type":"track",` +
		`"artist_name":"Autechre","track_title":"Amber","album_title":"Amber"}]}`
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/flowsheet/range" {
			t.Errorf("path = %q, want /flowsheet/range", r.URL.Path)
		}
		*got = r.URL.Query()
		w.Write([]byte(body))
	}))
}

func TestFlowsheetRange_MapsWindowToEpochMillis(t *testing.T) {
	var got url.Values
	srv := rangeServer(t, &got)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "range", "--since", "2026-11-24", "--until", "2026-11-25")
	if err != nil {
		t.Fatal(err)
	}
	start, _ := parseWindowTime("2026-11-24", false)
	end, _ := parseWindowTime("2026-11-25", true)
	if got.Get("start") != strconv.FormatInt(start.UnixMilli(), 10) {
		t.Errorf("start = %q, want %d", got.Get("start"), start.UnixMilli())
	}
	if got.Get("end") != strconv.FormatInt(end.UnixMilli(), 10) {
		t.Errorf("end = %q, want %d", got.Get("end"), end.UnixMilli())
	}
	if !strings.Contains(out, "Autechre") || !strings.Contains(out, "Amber") {
		t.Errorf("table missing entries:\n%s", out)
	}
}

func TestFlowsheetRange_RejectsBadWindowsLocally(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	for _, tc := range []struct {
		since, until, want string
	}{
		{"2026-11-25", "2026-11-24", "after"},
		{"2026-11-01", "2026-11-20", "8 days"},
	} {
		_, err := runCLI(t, "flowsheet", "range", "--since", tc.since, "--until", tc.until)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s..%s err = %v, want containing %q", tc.since, tc.until, err, tc.want)
		}
	}
}

func TestFlowsheetRange_JSONPassesEnvelope(t *testing.T) {
	var got url.Values
	srv := rangeServer(t, &got)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "--json", "flowsheet", "range",
		"--since", "2026-11-24T00:00:00Z", "--until", "2026-11-25T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"shows"`) || !strings.Contains(out, `"entries"`) {
		t.Errorf("--json should pass the envelope:\n%s", out)
	}
}
