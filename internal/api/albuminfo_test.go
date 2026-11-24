package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAlbumInfo_Show_DecodesDetailFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/library/info" {
			t.Errorf("got %s %s, want GET /library/info", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("album_id"); got != "53951" {
			t.Errorf("album_id = %q, want 53951", got)
		}
		w.Write([]byte(`{"id":53951,"legacy_release_id":1234,"code_letters":"AP",
			"code_artist_number":2,"code_number":5,"artist_name":"Aphex Twin",
			"album_title":"Classics","label":"R&S","format_name":"vinyl",
			"genre_name":"Electronic","plays":17,"on_streaming":true,
			"urls":["https://example.test/classics"]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	album, raw, err := c.AlbumInfo(context.Background(), 53951)
	if err != nil {
		t.Fatal(err)
	}
	if album.AlbumTitle != "Classics" || album.ArtistName != "Aphex Twin" || album.Format != "vinyl" {
		t.Errorf("unexpected album %+v", album)
	}
	if album.GenreName != "Electronic" || album.Label != "R&S" || album.Plays != 17 {
		t.Errorf("detail fields = %+v", album)
	}
	if album.LegacyReleaseID != 1234 || album.OnStreaming == nil || !*album.OnStreaming {
		t.Errorf("legacy/streaming fields = %+v", album)
	}
	if album.ShelfCode() == "" {
		t.Error("expected a shelf code from the code fields")
	}
	if len(raw) == 0 {
		t.Error("expected raw JSON")
	}
}

func TestAlbumInfo_Show_PropagatesNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"No catalog album for that album_id"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, _, err := c.AlbumInfo(context.Background(), 999)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		t.Fatalf("err = %v, want StatusError 404", err)
	}
}

func TestAlbumPlayCounts_Show_DecodesArms(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/53951/flowsheet-play-counts" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"direct":4,"rotation_linked":2,"legacy_linked":1}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	plays, _, err := c.AlbumPlayCounts(context.Background(), 53951)
	if err != nil {
		t.Fatal(err)
	}
	if plays.Direct != 4 || plays.RotationLinked != 2 || plays.LegacyLinked != 1 {
		t.Errorf("plays = %+v", plays)
	}
}
