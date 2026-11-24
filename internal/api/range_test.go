package api

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

// rangeBody mirrors GET /flowsheet/range (Backend-Service
// flowsheet.controller.ts getEntriesInRange): {shows, entries}, shows in the
// FlowsheetRangeShow projection and entries in the V2 wire shape.
const rangeBody = `{"shows":[{"id":1950333,"show_name":"Night Moves","dj_name":"DJ Ryan",` +
	`"specialty_id":7,"specialty":{"id":7,"slug":"new-science-experience","name":"New Science Experience","short_name":"NSE"},` +
	`"start_time":"2026-11-20T22:00:00.000Z","end_time":"2026-11-21T00:00:00.000Z"}],` +
	`"entries":[{"id":501,"show_id":1950333,"play_order":1,"add_time":"2026-11-20T22:00:01.000Z",` +
	`"entry_type":"show_start","dj_name":"DJ Ryan","message":"Start of Show"},` +
	`{"id":502,"show_id":1950333,"play_order":2,"add_time":"2026-11-20T22:04:05.123Z","entry_type":"track",` +
	`"artist_name":"Stereolab","album_title":"Jenny Ondioline","track_title":"French Disko",` +
	`"record_label":"Duophonic","request_flag":false,"segue":false,"rotation_id":null}]}`

func TestFlowsheetRange(t *testing.T) {
	var q string
	srv := serve(t, "/flowsheet/range", rangeBody, &q)
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	res, raw, err := c.FlowsheetRange(context.Background(), 1763676000000, 1763762400000)
	if err != nil {
		t.Fatal(err)
	}
	vals, _ := url.ParseQuery(q)
	if vals.Get("start") != "1763676000000" || vals.Get("end") != "1763762400000" || len(vals) != 2 {
		t.Errorf("query = %q, want start=1763676000000&end=1763762400000", q)
	}
	if len(res.Shows) != 1 || len(res.Entries) != 2 {
		t.Fatalf("decoded %+v", res)
	}
	s := res.Shows[0]
	if s.ID != 1950333 || s.ShowName == nil || *s.ShowName != "Night Moves" || s.DJName == nil || *s.DJName != "DJ Ryan" ||
		s.Specialty == nil || s.Specialty.Slug != "new-science-experience" ||
		s.StartTime != "2026-11-20T22:00:00.000Z" || s.EndTime == nil || *s.EndTime != "2026-11-21T00:00:00.000Z" {
		t.Errorf("show = %+v", s)
	}
	e := res.Entries[1]
	if e.ID != 502 || e.ShowID != 1950333 || e.AddTime != "2026-11-20T22:04:05.123Z" || e.ArtistName != "Stereolab" {
		t.Errorf("entry = %+v", e)
	}
	// Fidelity: unmodeled fields survive for --json.
	for _, k := range []string{"specialty_id", "short_name", "record_label", "message"} {
		if !strings.Contains(string(raw), k) {
			t.Errorf("raw dropped %s: %s", k, raw)
		}
	}
}
