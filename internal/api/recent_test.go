package api

import (
	"context"
	"testing"
)

// recentBody mirrors GET /flowsheet/shows/recent (Backend-Service
// flowsheet.service.ts getRecentShows, BS#2435): shows newest first, each with
// its DJs' on-air handles. A legacy show's single DJ has id: null; an account
// DJ whose handle was filtered away has dj_name: null.
const recentBody = `{"shows":[` +
	`{"id":202,"show_name":"Night Moves","start_time":"2026-11-24T22:00:00.000Z","end_time":null,` +
	`"djs":[{"id":"u_ryan","dj_name":"DJ Ryan"},{"id":"u_anon","dj_name":null}]},` +
	`{"id":201,"show_name":null,"start_time":"2026-11-24T20:00:00.000Z","end_time":"2026-11-24T22:00:00.000Z",` +
	`"djs":[{"id":null,"dj_name":"Legacy Larry"}]}]}`

func TestFlowsheetRecent(t *testing.T) {
	var q string
	srv := serve(t, "/flowsheet/shows/recent", recentBody, &q)
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	res, raw, err := c.FlowsheetRecent(context.Background(), 6)
	if err != nil {
		t.Fatal(err)
	}
	if q != "window_hours=6" {
		t.Errorf("query = %q, want window_hours=6", q)
	}
	if len(res.Shows) != 2 {
		t.Fatalf("decoded %+v", res)
	}
	s := res.Shows[0]
	if s.ID != 202 || s.ShowName == nil || *s.ShowName != "Night Moves" || s.StartTime != "2026-11-24T22:00:00.000Z" ||
		s.EndTime != nil || len(s.DJs) != 2 || s.DJs[0].ID == nil || *s.DJs[0].ID != "u_ryan" || s.DJs[1].DJName != nil {
		t.Errorf("show 0 = %+v", s)
	}
	l := res.Shows[1]
	if l.ShowName != nil || l.EndTime == nil || len(l.DJs) != 1 || l.DJs[0].ID != nil ||
		l.DJs[0].DJName == nil || *l.DJs[0].DJName != "Legacy Larry" {
		t.Errorf("show 1 = %+v", l)
	}
	if string(raw) != recentBody {
		t.Errorf("raw = %s", raw)
	}
}
