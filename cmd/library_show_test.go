package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func albumShowServer(t *testing.T, infoStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/library/info":
			if infoStatus != http.StatusOK {
				http.Error(w, `{"message":"No catalog album for that album_id"}`, infoStatus)
				return
			}
			w.Write([]byte(`{"id":53951,"legacy_release_id":1234,"code_letters":"AP",
				"code_artist_number":2,"code_number":5,"artist_name":"Aphex Twin",
				"album_title":"Classics","label":"R&S","format_name":"vinyl",
				"genre_name":"Electronic","plays":17,"on_streaming":true}`))
		case "/library/53951/flowsheet-play-counts":
			w.Write([]byte(`{"direct":4,"rotation_linked":2,"legacy_linked":1}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
}

func TestLibraryShow_PrintsAlbumAndCounts(t *testing.T) {
	srv := albumShowServer(t, http.StatusOK)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	out, err := runCLI(t, "library", "show", "53951")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Aphex Twin", "Classics", "vinyl", "Electronic", "17", "4"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
}

func TestLibraryShow_JSONWrapsBothBodies(t *testing.T) {
	srv := albumShowServer(t, http.StatusOK)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	out, err := runCLI(t, "library", "show", "53951", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"album"`, `"plays"`, `"legacy_release_id"`, `"rotation_linked"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json %q missing %q", out, want)
		}
	}
}

func TestLibraryShow_NotFoundExits5(t *testing.T) {
	srv := albumShowServer(t, http.StatusNotFound)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	_, err := runCLI(t, "library", "show", "53951")
	if err == nil {
		t.Fatal("expected an error")
	}
	if mapExit(err) != ExitNotFound {
		t.Errorf("exit = %d, want ExitNotFound(%d)", mapExit(err), ExitNotFound)
	}
}
