package api

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

// searchBody mirrors GET /flowsheet/search (Backend-Service
// search.controller.ts + search.service.ts transformRow): results with the
// server's real key names, the offset-mode total/page/totalPages, and
// nextCursor (present only on a full date-sorted page).
const searchBody = `{"results":[` +
	`{"id":9912,"play_date":"2026-11-20T22:14:05.123Z","artist_name":"Stereolab",` +
	`"track_title":"French Disko","album_title":"Jenny Ondioline","record_label":"Duophonic",` +
	`"show_id":1950333,"dj_name":"DJ Ryan","rotation_bin":"H","request_flag":false,"on_streaming":null}` +
	`],"total":1,"page":0,"totalPages":1,"nextCursor":"MjAyNi0xMS0yMFQyMjoxNDowNS4xMjMxMjNa.9912.word"}`

func TestFlowsheetSearch(t *testing.T) {
	var q string
	srv := serve(t, "/flowsheet/search", searchBody, &q)
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	res, raw, err := c.FlowsheetSearch(context.Background(), map[string]string{"q": "stereolab", "limit": "10"})
	if err != nil {
		t.Fatal(err)
	}
	vals, _ := url.ParseQuery(q)
	if vals.Get("q") != "stereolab" || vals.Get("limit") != "10" || len(vals) != 2 {
		t.Errorf("query = %q, want q=stereolab&limit=10", q)
	}
	if len(res.Results) != 1 {
		t.Fatalf("decoded %+v", res)
	}
	r := res.Results[0]
	if r.PlayDate != "2026-11-20T22:14:05.123Z" || r.DJName != "DJ Ryan" || r.ArtistName != "Stereolab" ||
		r.TrackTitle != "French Disko" || r.AlbumTitle != "Jenny Ondioline" {
		t.Errorf("result = %+v", r)
	}
	if res.Total != 1 || res.Page != 0 || res.TotalPages != 1 {
		t.Errorf("paging = %+v", res)
	}
	if res.NextCursor != "MjAyNi0xMS0yMFQyMjoxNDowNS4xMjMxMjNa.9912.word" {
		t.Errorf("nextCursor = %q", res.NextCursor)
	}
	// Fidelity: unmodeled fields survive for --json.
	for _, k := range []string{"record_label", "rotation_bin", "on_streaming", "show_id"} {
		if !strings.Contains(string(raw), k) {
			t.Errorf("raw dropped %s: %s", k, raw)
		}
	}
}
