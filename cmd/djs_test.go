package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// jwtWithSub builds an unsigned JWT whose payload carries sub, enough for the
// CLI's claim-reading path (it never verifies the signature).
func jwtWithSub(sub string) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + sub + `"}`))
	return "x." + payload + ".y"
}

func TestPlaylists_DefaultsToOwnDJID(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", jwtWithSub("me123"))

	if _, err := runCLI(t, "djs", "playlists"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "dj_id=me123") {
		t.Errorf("query = %q, want dj_id=me123 from token", gotQuery)
	}
}

func TestPlaylists_TableProjection(t *testing.T) {
	body := `[{"show":42,"date":"2026-05-15T16:03:07.479Z",` +
		`"djs":[{"dj_id":"abc","dj_name":"Ryan Shaw"}],` +
		`"preview":[` +
		`{"entry_type":"show_start","artist_name":"marker"},` +
		`{"entry_type":"track","artist_name":"3BallMTY","track_title":"Beso Al Aire"}` +
		`]}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "djs", "playlists", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2026-05-15") || !strings.Contains(out, "Ryan Shaw") {
		t.Errorf("table missing date/dj:\n%s", out)
	}
	// Only the track row appears in the preview; the marker row is dropped.
	if !strings.Contains(out, "3BallMTY – Beso Al Aire") {
		t.Errorf("preview missing track:\n%s", out)
	}
	if strings.Contains(out, "marker") {
		t.Errorf("preview should skip non-track rows:\n%s", out)
	}
}

// showServer serves /djs/playlists with the given body and /flowsheet/playlist
// with one full show per show_id from shows, counting show fetches.
func showServer(t *testing.T, playlists string, shows map[string]string) (*httptest.Server, *int) {
	t.Helper()
	fetches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/djs/playlists":
			w.Write([]byte(playlists))
		case "/flowsheet/playlist":
			fetches++
			body, ok := shows[r.URL.Query().Get("show_id")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"message":"Show not found"}`))
				return
			}
			w.Write([]byte(body))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	return srv, &fetches
}

const showFull42 = `{"id":42,"start_time":"2026-10-16T15:59:19.052Z","show_djs":[{"id":"abc","dj_name":"Ryan Shaw"}],` +
	`"entries":[` +
	`{"entry_type":"show_start","dj_name":"Ryan Shaw"},` +
	`{"entry_type":"track","artist_name":"Little Mazarn","track_title":"White Fang","album_title":"Little Mazarn"},` +
	`{"entry_type":"track","artist_name":"Yasmin Williams","track_title":"Jarabi","album_title":"Urban Driftwood"},` +
	`{"entry_type":"talkset"},` +
	`{"entry_type":"track","artist_name":"Duke Pearson","track_title":"The Fakir","album_title":"Prairie Dog"}` +
	`]}`

func TestPlaylistsFull_FetchesEveryShow(t *testing.T) {
	playlists := `[` +
		`{"show":42,"date":"2026-10-16T15:59:19.052Z","preview":[{"entry_type":"track","artist_name":"Little Mazarn","track_title":"White Fang"}]},` +
		`{"show":43,"date":"2026-10-30T16:00:00.000Z","preview":[]}` +
		`]`
	show43 := `{"id":43,"start_time":"2026-10-30T16:00:00.000Z","entries":[` +
		`{"entry_type":"track","artist_name":"Japan","track_title":"Talking Drum","album_title":"Tin Drum"}]}`
	srv, fetches := showServer(t, playlists, map[string]string{"42": showFull42, "43": show43})
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "djs", "playlists", "abc", "--full")
	if err != nil {
		t.Fatal(err)
	}
	if *fetches != 2 {
		t.Errorf("fetched %d shows, want 2", *fetches)
	}
	// One row per track, tagged with its show's date; markers are dropped.
	for _, want := range []string{"2026-10-16", "The Fakir", "2026-10-30", "Talking Drum"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "talkset") {
		t.Errorf("--full table should list tracks only:\n%s", out)
	}
}

func TestPlaylistsFull_JSONIsArrayOfShowBodies(t *testing.T) {
	playlists := `[{"show":42,"date":"2026-10-16T15:59:19.052Z","preview":[]}]`
	srv, _ := showServer(t, playlists, map[string]string{"42": showFull42})
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "--json", "djs", "playlists", "abc", "--full")
	if err != nil {
		t.Fatal(err)
	}
	var shows []map[string]any
	if err := json.Unmarshal([]byte(out), &shows); err != nil {
		t.Fatalf("--json --full is not a JSON array: %v\n%s", err, out)
	}
	if len(shows) != 1 || shows[0]["show_djs"] == nil {
		t.Errorf("want one full show body, got %s", out)
	}
	if entries, _ := shows[0]["entries"].([]any); len(entries) != 5 {
		t.Errorf("entries = %d, want all 5", len(entries))
	}
}

func TestPlaylistsFull_MissingShowFails(t *testing.T) {
	playlists := `[{"show":99,"date":"2026-10-16T15:59:19.052Z","preview":[]}]`
	srv, _ := showServer(t, playlists, map[string]string{})
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	if _, err := runCLI(t, "djs", "playlists", "abc", "--full"); err == nil {
		t.Error("want an error when a listed show 404s")
	}
}
