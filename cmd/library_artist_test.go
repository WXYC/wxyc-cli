package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Fixtures with the server's real key names: GET /library/artists/{id}
// (getArtistCard: ArtistCardRow + BS#2597 dependent counts), GET
// /library/artists/{id}/releases (getArtistReleases), GET /library/genres.
const (
	artistCard = `{"artist_id":812,"artist_name":"Stereolab","alphabetical_name":"Stereolab",` +
		`"genre_id":7,"code_letters":"ST","code_artist_number":42,"code_comp_letter":null,` +
		`"release_count":14,"cross_reference_source_count":1,"cross_reference_target_count":0,` +
		`"library_cross_reference_count":0,"compilation_credit_count":3}`
	artistReleases = `{"artist_id":812,"releases":[` +
		`{"id":45029,"last_modified":"2026-03-14T18:22:05.112Z","format_name":"cd","genre_id":7,` +
		`"code_letters":"ST","code_artist_number":42,"code_comp_letter":null,"code_number":3,` +
		`"code_volume_letters":null,"album_title":"Dots and Loops","alternate_artist_name":null},` +
		`{"id":45030,"last_modified":"2026-03-14T18:22:05.112Z","format_name":"vinyl","genre_id":7,` +
		`"code_letters":"ST","code_artist_number":42,"code_comp_letter":null,"code_number":4,` +
		`"code_volume_letters":"B","album_title":"Aluminum Tunes","alternate_artist_name":null}` +
		`],"total":14,"page":1,"totalPages":7}`
	artistGenres = `[{"id":3,"genre_name":"Jazz","plays":0},{"id":7,"genre_name":"Rock","plays":0}]`
)

// artistServer serves the three endpoints and records each one's raw query.
func artistServer(t *testing.T, releases string, status int) map[string]string {
	t.Helper()
	queries := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries[r.URL.Path] = r.URL.RawQuery
		if status != 0 {
			w.WriteHeader(status)
			w.Write([]byte(`{"message":"Artist not found"}`))
			return
		}
		switch r.URL.Path {
		case "/library/artists/812":
			w.Write([]byte(artistCard))
		case "/library/artists/812/releases":
			w.Write([]byte(releases))
		case "/library/genres":
			w.Write([]byte(artistGenres))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
	return queries
}

func TestLibraryArtist_CardThenReleases(t *testing.T) {
	artistServer(t, artistReleases, 0)
	out, err := runCLI(t, "library", "artist", "812")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"id":                  "812",
		"artist":              "Stereolab",
		"genre":               "Rock",
		"shelf":               "Rock ST 42",
		"releases":            "14",
		"compilation credits": "3",
		"page":                "1 of 0-6",
	} {
		if got := nowField(t, out, name); got != want {
			t.Errorf("%s = %q, want %q\n%s", name, got, want, out)
		}
	}
	card, list, ok := strings.Cut(out, "\n\n")
	if !ok {
		t.Fatalf("want the card and the releases as two tables:\n%s", out)
	}
	if strings.Contains(card, "Dots and Loops") {
		t.Errorf("release in the card table:\n%s", card)
	}
	lines := strings.Split(strings.TrimRight(list, "\n"), "\n")
	if len(lines) != 3 || strings.Join(strings.Fields(lines[0]), " ") != "ID ALBUM FORMAT SHELF" {
		t.Fatalf("releases table:\n%s", list)
	}
	if got := strings.Join(strings.Fields(lines[1]), " "); got != "45029 Dots and Loops cd Rock ST 42/3" {
		t.Errorf("row 1 = %q", got)
	}
	if got := strings.Join(strings.Fields(lines[2]), " "); got != "45030 Aluminum Tunes vinyl Rock ST 42/4 B" {
		t.Errorf("row 2 = %q", got)
	}
}

func TestLibraryArtist_FlagsMapToExactQueryStrings(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		card, rels string
	}{
		{"none", nil, "", ""},
		{"genre", []string{"--genre", "7"}, "genre_id=7", "genre_id=7"},
		{"paging", []string{"--page", "0", "--limit", "25"}, "", "limit=25&page=0"},
		{"all", []string{"--genre", "7", "--page", "2", "-n", "5"}, "genre_id=7", "genre_id=7&limit=5&page=2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := artistServer(t, artistReleases, 0)
			if _, err := runCLI(t, append([]string{"library", "artist", "812"}, tc.args...)...); err != nil {
				t.Fatal(err)
			}
			if got := q["/library/artists/812"]; got != tc.card {
				t.Errorf("card query = %q, want %q", got, tc.card)
			}
			if got := q["/library/artists/812/releases"]; got != tc.rels {
				t.Errorf("releases query = %q, want %q", got, tc.rels)
			}
		})
	}
}

func TestLibraryArtist_BadFlagsRefusedBeforeRequest(t *testing.T) {
	for _, args := range [][]string{
		{"abc"},
		{"812", "--limit", "101"},
		{"812", "--limit", "-1"},
		{"812", "--page", "-1"},
		{"812", "--genre", "-1"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("%v: request sent to %s", args, r.URL)
		}))
		t.Setenv("WXYC_API_URL", srv.URL)
		t.Setenv("WXYC_JWT", "unused")
		if _, err := runCLI(t, append([]string{"library", "artist"}, args...)...); err == nil {
			t.Errorf("%v accepted", args)
		}
		srv.Close()
	}
}

func TestLibraryArtist_NoReleases(t *testing.T) {
	artistServer(t, `{"artist_id":812,"releases":[],"total":0,"page":0,"totalPages":0}`, 0)
	out, err := runCLI(t, "library", "artist", "812")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out, "\n\nno releases\n") {
		t.Errorf("out:\n%s", out)
	}
}

func TestLibraryArtist_JSONWrapsBothRawBodies(t *testing.T) {
	artistServer(t, artistReleases, 0)
	out, err := runCLI(t, "library", "artist", "812", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Errorf("keys = %d, want artist and releases only:\n%s", len(got), out)
	}
	assertSameJSON(t, "artist", got["artist"], artistCard)
	assertSameJSON(t, "releases", got["releases"], artistReleases)
}

func TestLibraryArtist_NotFoundIsExitNotFound(t *testing.T) {
	artistServer(t, "", http.StatusNotFound)
	_, err := runCLI(t, "library", "artist", "812")
	if got := mapExit(err); got != ExitNotFound {
		t.Errorf("exit = %d (err %v), want ExitNotFound(%d)", got, err, ExitNotFound)
	}
}

func TestLibrarySearch_ShowsArtistID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":45029,"artist_id":812,"genre_id":7,"code_letters":"ST","code_artist_number":42,` +
			`"code_number":3,"artist_name":"Stereolab","album_title":"Dots and Loops","format_name":"cd","genre_name":"Rock"}]`))
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "library", "search", "--artist", "stereolab")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 || strings.Join(strings.Fields(lines[0]), " ") != "ID ARTIST_ID ARTIST ALBUM FORMAT SHELF" ||
		!strings.HasPrefix(strings.Join(strings.Fields(lines[1]), " "), "45029 812 Stereolab") {
		t.Errorf("search table:\n%s", out)
	}
}
