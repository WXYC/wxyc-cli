package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// albumInfoBody mirrors GET /library/info?album_id= (Backend-Service
// libraryService.getAlbumFromDB): snake_case catalog columns, the camelCase
// discogs-unavailable trio (BS#1895), nested reconciled_identity, and urls.
const albumInfoBody = `{"id":45029,"artist_id":812,"genre_id":7,"format_id":2,` +
	`"code_letters":"ST","code_artist_number":42,"code_comp_letter":null,"code_number":3,"code_volume_letters":null,` +
	`"artist_name":"Stereolab","alphabetical_name":"Stereolab","album_title":"Dots and Loops",` +
	`"label":"Duophonic","record_label":"Duophonic","label_id":91,"alternate_artist_name":null,` +
	`"disc_quantity":2,"album_artist":null,"plays":0,` +
	`"add_date":"1997-10-01T00:00:00.000Z","last_modified":"2026-03-14T18:22:05.112Z",` +
	`"format_name":"cd","genre_name":"Rock","date_lost":null,"date_found":null,"on_streaming":true,` +
	`"legacy_release_id":65880,"urls":["https://stereolab.bandcamp.com/album/dots-and-loops"],` +
	`"reconciled_identity":{"discogs_artist_id":5577,"musicbrainz_artist_id":null,"wikidata_qid":null,"spotify_artist_id":null,"apple_music_artist_id":null,"bandcamp_id":null},` +
	`"discogsUnavailable":true,"discogsUnavailableNote":"wrong pressing","lastDiscogsRecheckAt":null}`

// playCountsBody mirrors GET /library/{id}/flowsheet-play-counts (BS#2592).
const playCountsBody = `{"direct":17,"rotation_linked":4,"legacy_linked":31}`

func TestAlbumInfo(t *testing.T) {
	var q string
	srv := serve(t, "/library/info", albumInfoBody, &q)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	a, raw, err := c.AlbumInfo(context.Background(), 45029)
	if err != nil {
		t.Fatal(err)
	}
	if q != "album_id=45029" {
		t.Errorf("query = %q, want album_id=45029", q)
	}
	if string(raw) != albumInfoBody {
		t.Errorf("raw = %s, want the body verbatim", raw)
	}
	if a.ID != 45029 || a.ArtistName != "Stereolab" || a.AlbumTitle != "Dots and Loops" ||
		a.Format != "cd" || a.GenreName != "Rock" || a.Label != "Duophonic" || a.ShelfCode() != "Rock ST 42/3" {
		t.Errorf("album = %+v", a.Album)
	}
	if a.LegacyReleaseID == nil || *a.LegacyReleaseID != 65880 {
		t.Errorf("legacy_release_id = %v, want 65880", a.LegacyReleaseID)
	}
	if a.DiscQuantity == nil || *a.DiscQuantity != 2 {
		t.Errorf("disc_quantity = %v, want 2", a.DiscQuantity)
	}
	if a.OnStreaming == nil || !*a.OnStreaming {
		t.Errorf("on_streaming = %v, want true", a.OnStreaming)
	}
	if a.AddDate != "1997-10-01T00:00:00.000Z" {
		t.Errorf("add_date = %q", a.AddDate)
	}
	if len(a.URLs) != 1 || a.URLs[0] != "https://stereolab.bandcamp.com/album/dots-and-loops" {
		t.Errorf("urls = %v", a.URLs)
	}
	if !a.DiscogsUnavailable || a.DiscogsUnavailableNote == nil || *a.DiscogsUnavailableNote != "wrong pressing" {
		t.Errorf("discogsUnavailable = %v, note = %v", a.DiscogsUnavailable, a.DiscogsUnavailableNote)
	}
}

func TestFlowsheetPlayCounts(t *testing.T) {
	srv := serve(t, "/library/45029/flowsheet-play-counts", playCountsBody, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	p, raw, err := c.FlowsheetPlayCounts(context.Background(), 45029)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != playCountsBody {
		t.Errorf("raw = %s, want the body verbatim", raw)
	}
	if p.Direct != 17 || p.RotationLinked != 4 || p.LegacyLinked != 31 {
		t.Errorf("counts = %+v", p)
	}
}

func TestAlbumInfo_NotFoundIsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"No catalog album for that album_id"}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	_, _, err := c.AlbumInfo(context.Background(), 1)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		t.Errorf("err = %v, want a 404 StatusError", err)
	}
}
