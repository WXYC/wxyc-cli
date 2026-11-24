package cmd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func artistServer(t *testing.T, got map[string]url.Values, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/library/artists/431":
			if status != http.StatusOK {
				http.Error(w, `{"message":"Artist not found"}`, status)
				return
			}
			got["artist"] = r.URL.Query()
			w.Write([]byte(`{"artist_id":431,"artist_name":"Isis","alphabetical_name":"Isis",` +
				`"genre_id":7,"code_letters":"IS","code_artist_number":1,` +
				`"code_comp_letter":null,"release_count":2}`))
		case "/library/artists/431/releases":
			got["releases"] = r.URL.Query()
			w.Write([]byte(`{"artist_id":431,"releases":[` +
				`{"id":10,"format_name":"vinyl","genre_id":7,"code_letters":"IS",` +
				`"code_artist_number":1,"code_number":3,"album_title":"Oceanic"},` +
				`{"id":11,"format_name":"cd","genre_id":7,"code_letters":"IS",` +
				`"code_artist_number":1,"code_number":4,"code_volume_letters":"B",` +
				`"album_title":"Panopticon"}],"total":2,"page":0,"totalPages":1}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
}

func TestLibraryArtist_PrintsCardAndReleases(t *testing.T) {
	got := map[string]url.Values{}
	srv := artistServer(t, got, http.StatusOK)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "library", "artist", "431", "--genre", "7", "--page", "2", "--limit", "10")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Artist: Isis", "Filing: IS 1", "Oceanic", "Panopticon", "IS 1/4 B"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got["artist"].Get("genre_id") != "7" {
		t.Errorf("artist genre_id = %q", got["artist"].Get("genre_id"))
	}
	if got["releases"].Get("genre_id") != "7" || got["releases"].Get("page") != "2" || got["releases"].Get("limit") != "10" {
		t.Errorf("releases query = %v", got["releases"])
	}
}

func TestLibraryArtist_JSONWrapsBothBodies(t *testing.T) {
	got := map[string]url.Values{}
	srv := artistServer(t, got, http.StatusOK)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "--json", "library", "artist", "431")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"artist"`, `"releases"`, `"release_count"`, `"totalPages"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json missing %q:\n%s", want, out)
		}
	}
}

func TestLibraryArtist_RejectsBadPagingAndNotFound(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	for _, args := range [][]string{
		{"library", "artist", "431", "--limit", "0"},
		{"library", "artist", "431", "--page", "-1"},
	} {
		if _, err := runCLI(t, args...); err == nil {
			t.Errorf("%v: want a local error", args)
		}
	}

	got := map[string]url.Values{}
	srv := artistServer(t, got, http.StatusNotFound)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	_, err := runCLI(t, "library", "artist", "431")
	if err == nil || mapExit(err) != ExitNotFound {
		t.Errorf("404 err = %v, want ExitNotFound", err)
	}
}
