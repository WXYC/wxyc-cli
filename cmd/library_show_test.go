package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// Fixtures with the server's real key names: GET /library/info?album_id=
// (libraryService.getAlbumFromDB) and GET /library/{id}/flowsheet-play-counts
// (BS#2592).
const (
	showAlbumInfo = `{"id":45029,"artist_id":812,"genre_id":7,"format_id":2,` +
		`"code_letters":"ST","code_artist_number":42,"code_comp_letter":null,"code_number":3,"code_volume_letters":null,` +
		`"artist_name":"Stereolab","alphabetical_name":"Stereolab","album_title":"Dots and Loops",` +
		`"label":"Duophonic","record_label":"Duophonic","label_id":91,"alternate_artist_name":null,` +
		`"disc_quantity":2,"album_artist":null,"plays":0,` +
		`"add_date":"1997-10-01T00:00:00.000Z","last_modified":"2026-03-14T18:22:05.112Z",` +
		`"format_name":"cd","genre_name":"Rock","date_lost":null,"date_found":null,"on_streaming":true,` +
		`"legacy_release_id":65880,"urls":["https://stereolab.bandcamp.com/"],` +
		`"reconciled_identity":null,` +
		`"discogsUnavailable":false,"discogsUnavailableNote":null,"lastDiscogsRecheckAt":null}`
	showPlayCounts = `{"direct":17,"rotation_linked":4,"legacy_linked":31}`
)

func libraryShowServer(t *testing.T, info, counts string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
			w.Write([]byte(`{"message":"No catalog album for that album_id"}`))
			return
		}
		switch r.URL.Path {
		case "/library/info":
			if got := r.URL.Query().Get("album_id"); got != "45029" {
				t.Errorf("album_id = %q, want 45029", got)
			}
			w.Write([]byte(info))
		case "/library/45029/flowsheet-play-counts":
			w.Write([]byte(counts))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
}

func TestLibraryShow_Table(t *testing.T) {
	libraryShowServer(t, showAlbumInfo, showPlayCounts, 0)
	out, err := runCLI(t, "library", "show", "45029")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"id":               "45029",
		"legacy id":        "65880",
		"artist":           "Stereolab",
		"album":            "Dots and Loops",
		"label":            "Duophonic",
		"format":           "cd",
		"genre":            "Rock",
		"shelf":            "Rock ST 42/3",
		"discs":            "2",
		"on streaming":     "true",
		"added":            "1997-10-01T00:00:00.000Z",
		"url":              "https://stereolab.bandcamp.com/",
		"plays (direct)":   "17",
		"plays (rotation)": "4",
		"plays (legacy)":   "31",
	} {
		if got := nowField(t, out, name); got != want {
			t.Errorf("%s = %q, want %q\n%s", name, got, want, out)
		}
	}
	// The dead legacy `plays` counter is not shown, and the three arms are
	// never summed (BS#2592).
	if deadOrSummed := regexp.MustCompile(`(?m)^plays( \(total\))?\s+\d`); deadOrSummed.MatchString(out) {
		t.Errorf("unexpected legacy or summed plays row:\n%s", out)
	}
	// Optional fields that are null on the server get no row.
	for _, name := range []string{"album artist", "alternate artist", "lost", "found", "discogs unavailable"} {
		if got := nowField(t, out, name); got != "" {
			t.Errorf("unexpected %s row = %q", name, got)
		}
	}
}

func TestLibraryShow_JSONWrapsBothRawBodies(t *testing.T) {
	libraryShowServer(t, showAlbumInfo, showPlayCounts, 0)
	out, err := runCLI(t, "library", "show", "45029", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Errorf("keys = %d, want album and play_counts only:\n%s", len(got), out)
	}
	assertSameJSON(t, "album", got["album"], showAlbumInfo)
	assertSameJSON(t, "play_counts", got["play_counts"], showPlayCounts)
}

func assertSameJSON(t *testing.T, name string, got json.RawMessage, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	json.Unmarshal([]byte(want), &w)
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Errorf("%s = %s, want %s", name, gb, wb)
	}
}

func TestLibraryShow_NotFoundIsExitNotFound(t *testing.T) {
	libraryShowServer(t, "", "", http.StatusNotFound)
	_, err := runCLI(t, "library", "show", "45029")
	if err == nil {
		t.Fatal("a missing album succeeded silently")
	}
	if got := mapExit(err); got != ExitNotFound {
		t.Errorf("exit = %d, want ExitNotFound(%d)", got, ExitNotFound)
	}
}

func TestLibraryShow_RejectsNonIntegerID(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	_, err := runCLI(t, "library", "show", "abc")
	if err == nil || !strings.Contains(err.Error(), "album_id must be an integer") {
		t.Errorf("err = %v, want album_id integer error", err)
	}
}

func TestLibraryShow_JSONKeepsHTMLCharacters(t *testing.T) {
	libraryShowServer(t, strings.Replace(showAlbumInfo, `"album_title":"Dots and Loops"`, `"album_title":"`+htmlText+`"`, 1), showPlayCounts, 0)
	out, err := runCLI(t, "library", "show", "45029", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, htmlText) {
		t.Errorf("--json altered %q:\n%s", htmlText, out)
	}
}
