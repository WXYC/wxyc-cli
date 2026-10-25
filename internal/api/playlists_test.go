package api

import (
	"context"
	"strings"
	"testing"
)

func TestPlaylists(t *testing.T) {
	// dj_name on a track preview row is null; show_start carries the dj_name and
	// an unmodeled field (metadata_status) that must still survive in raw.
	body := `[{"show":1947075,"show_name":"","date":"2026-05-15T16:03:07.479Z",` +
		`"djs":[{"dj_id":"abc","dj_name":"Ryan Shaw"}],"specialty_show":"",` +
		`"preview":[` +
		`{"id":1,"entry_type":"show_start","artist_name":"El Vaquero","dj_name":"El Vaquero","metadata_status":"pending"},` +
		`{"id":2,"entry_type":"track","artist_name":"3BallMTY","track_title":"Beso Al Aire","record_label":"Latin Power"}` +
		`]}]`

	var q string
	srv := serve(t, "/djs/playlists", body, &q)
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	p, raw, err := c.Playlists(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "dj_id=abc") {
		t.Errorf("query = %q, want dj_id=abc", q)
	}
	if len(p) != 1 || p[0].Show != 1947075 {
		t.Fatalf("decoded %+v", p)
	}
	if len(p[0].DJs) != 1 || p[0].DJs[0].DJName != "Ryan Shaw" {
		t.Errorf("djs = %+v", p[0].DJs)
	}
	if len(p[0].Preview) != 2 || p[0].Preview[1].TrackTitle != "Beso Al Aire" {
		t.Errorf("preview = %+v", p[0].Preview)
	}
	// Fidelity guarantee: unmodeled preview field survives for --json.
	if !strings.Contains(string(raw), "metadata_status") {
		t.Errorf("raw dropped unmodeled field: %s", raw)
	}
}

func TestShow(t *testing.T) {
	// Mirrors GET /flowsheet/playlist: show metadata spread at the top level,
	// adjacent-show ids, and the full entry list. legacy_dj_name and
	// metadata_status are unmodeled and must still survive in raw.
	body := `{"id":1950333,"primary_dj_id":null,"legacy_dj_name":"Ryan Shaw",` +
		`"start_time":"2026-07-17T17:59:38.121Z","end_time":"2026-07-17T20:59:04.215Z",` +
		`"show_name":null,"specialty_show_name":"","show_djs":[],` +
		`"previous_show_id":1950329,"next_show_id":1950339,` +
		`"entries":[` +
		`{"id":1,"show_id":1950333,"play_order":1,"entry_type":"show_start"},` +
		`{"id":2,"show_id":1950333,"play_order":2,"entry_type":"track","artist_name":"The Moles",` +
		`"track_title":"Cars For Kings Cross","album_title":"Untune the Sky","metadata_status":"done"}` +
		`]}`

	var q string
	srv := serve(t, "/flowsheet/playlist", body, &q)
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	s, raw, err := c.Show(context.Background(), 1950333)
	if err != nil {
		t.Fatal(err)
	}
	if q != "show_id=1950333" {
		t.Errorf("query = %q, want show_id=1950333", q)
	}
	if s.ID != 1950333 || s.StartTime != "2026-07-17T17:59:38.121Z" {
		t.Errorf("decoded %+v", s)
	}
	if len(s.Entries) != 2 || s.Entries[1].TrackTitle != "Cars For Kings Cross" {
		t.Errorf("entries = %+v", s.Entries)
	}
	for _, want := range []string{"legacy_dj_name", "metadata_status", "previous_show_id"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("raw dropped unmodeled field %s: %s", want, raw)
		}
	}
}
