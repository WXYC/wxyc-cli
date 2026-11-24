package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// artistCardBody mirrors GET /library/artists/{id} (Backend-Service
// getArtistCard): the ArtistCardRow (artist_id, not id) spread with the
// BS#2597 dependent counts.
const artistCardBody = `{"artist_id":812,"artist_name":"Stereolab","alphabetical_name":"Stereolab",` +
	`"genre_id":7,"code_letters":"ST","code_artist_number":42,"code_comp_letter":null,` +
	`"release_count":14,"cross_reference_source_count":1,"cross_reference_target_count":0,` +
	`"library_cross_reference_count":0,"compilation_credit_count":3}`

// artistReleasesBody mirrors GET /library/artists/{id}/releases
// (getArtistReleases + artistReleasesQuery).
const artistReleasesBody = `{"artist_id":812,"releases":[` +
	`{"id":45029,"last_modified":"2026-03-14T18:22:05.112Z","format_name":"cd","genre_id":7,` +
	`"code_letters":"ST","code_artist_number":42,"code_comp_letter":null,"code_number":3,` +
	`"code_volume_letters":null,"album_title":"Dots and Loops","alternate_artist_name":null},` +
	`{"id":45030,"last_modified":"2026-03-14T18:22:05.112Z","format_name":"vinyl","genre_id":7,` +
	`"code_letters":"ST","code_artist_number":42,"code_comp_letter":null,"code_number":4,` +
	`"code_volume_letters":"B","album_title":"Aluminum Tunes","alternate_artist_name":"Groop"}` +
	`],"total":14,"page":1,"totalPages":7}`

func TestArtistCard(t *testing.T) {
	for _, tc := range []struct {
		genre int
		want  string
	}{{0, ""}, {7, "genre_id=7"}} {
		var q string
		srv := serve(t, "/library/artists/812", artistCardBody, &q)
		c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
		a, raw, err := c.ArtistCard(context.Background(), 812, tc.genre)
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
		if q != tc.want {
			t.Errorf("genre %d: query = %q, want %q", tc.genre, q, tc.want)
		}
		if string(raw) != artistCardBody {
			t.Errorf("raw = %s, want the body verbatim", raw)
		}
		if a.ArtistID != 812 || a.ArtistName != "Stereolab" || a.GenreID != 7 || a.CodeLetters != "ST" ||
			a.CodeArtistNumber != 42 || a.CodeCompLetter != nil || a.ReleaseCount != 14 ||
			a.CrossReferenceSourceCount != 1 || a.CompilationCreditCount != 3 {
			t.Errorf("card = %+v", a)
		}
	}
}

func TestArtistReleases_QueryString(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params map[string]string
		want   string
	}{
		{"none", nil, ""},
		{"all", map[string]string{"page": "1", "limit": "2", "genre_id": "7"}, "genre_id=7&limit=2&page=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var q string
			srv := serve(t, "/library/artists/812/releases", artistReleasesBody, &q)
			defer srv.Close()
			c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
			r, raw, err := c.ArtistReleases(context.Background(), 812, tc.params)
			if err != nil {
				t.Fatal(err)
			}
			if q != tc.want {
				t.Errorf("query = %q, want %q", q, tc.want)
			}
			if string(raw) != artistReleasesBody {
				t.Errorf("raw = %s, want the body verbatim", raw)
			}
			if r.ArtistID != 812 || r.Total != 14 || r.Page != 1 || r.TotalPages != 7 || len(r.Releases) != 2 {
				t.Fatalf("envelope = %+v", r)
			}
			x := r.Releases[1]
			if x.ID != 45030 || x.AlbumTitle != "Aluminum Tunes" || x.Format != "vinyl" || x.GenreID != 7 ||
				x.CodeLetters != "ST" || x.CodeArtistNumber != 42 || x.CodeNumber != 4 ||
				x.CodeVolumeLetters == nil || *x.CodeVolumeLetters != "B" ||
				x.AlternateArtistName == nil || *x.AlternateArtistName != "Groop" {
				t.Errorf("release = %+v", x)
			}
		})
	}
}

func TestArtistCard_NotFoundIsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Artist not found"}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, _, err := c.ArtistCard(context.Background(), 1, 0)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		t.Errorf("err = %v, want a 404 StatusError", err)
	}
}
