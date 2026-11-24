package api

import (
	"context"
	"errors"
	"testing"
)

// The three on_air states of GET /flowsheet (Backend-Service app.yaml
// OnAirInfo): an object while a DJ is live, JSON null on automation, and
// absent when the server does not report it.
const (
	nowLiveBody = `{"entries":[` +
		`{"id":9002,"show_id":4812,"play_order":12,"entry_type":"spotlight","spotlight_role":"performer","spotlight_name":"The Weirs"},` +
		`{"id":9001,"show_id":4812,"play_order":11,"entry_type":"track","artist_name":"Stereolab","album_title":"Dots and Loops","track_title":"Miss Modular"}],` +
		`"on_air":{"dj_name":"DJ Lark","show_id":4812,"show_name":"Lark Hours",` +
		`"specialty":{"id":3,"slug":"new-science-experience","name":"New Science Experience","short_name":"NSE","tagline":null,"logs_playcuts":true},` +
		`"request_line":{"mode":"closed","source":"spotlight"},` +
		`"spotlight":{"id":9002,"show_id":4812,"play_order":12,"add_time":"2026-11-24T22:10:00.000Z","entry_type":"spotlight",` +
		`"spotlight_role":"performer","spotlight_name":"The Weirs","message":"The Weirs (performer)"}}}`
	nowAutoBody    = `{"entries":[],"on_air":null}`
	nowUnknownBody = `{"entries":[]}`
)

func TestFlowsheetPage_OnAirStates(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantErr    error
	}{
		{"live", nowLiveBody, nil},
		{"automation", nowAutoBody, ErrNoOpenShow},
		{"unknown", nowUnknownBody, ErrOnAirUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var q string
			srv := serve(t, "/flowsheet", tc.body, &q)
			defer srv.Close()
			c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

			p, raw, err := c.FlowsheetPage(context.Background(), 20)
			if err != nil {
				t.Fatal(err)
			}
			if q != "limit=20" {
				t.Errorf("query = %q, want limit=20", q)
			}
			if string(raw) != tc.body {
				t.Errorf("raw = %s, want the body verbatim", raw)
			}
			o, err := p.OnAir()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("OnAir err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			if o.DJName != "DJ Lark" || o.ShowID != 4812 || o.ShowName == nil || *o.ShowName != "Lark Hours" {
				t.Errorf("on_air = %+v", o)
			}
			if o.Specialty == nil || o.Specialty.Name != "New Science Experience" || o.Specialty.Slug != "new-science-experience" {
				t.Errorf("specialty = %+v", o.Specialty)
			}
			if o.RequestLine == nil || o.RequestLine.Mode != "closed" || o.RequestLine.Source != "spotlight" {
				t.Errorf("request_line = %+v", o.RequestLine)
			}
			if o.Spotlight == nil || o.Spotlight.SpotlightName != "The Weirs" || o.Spotlight.SpotlightRole != "performer" {
				t.Errorf("spotlight = %+v", o.Spotlight)
			}
			if len(p.Entries) != 2 || p.Entries[1].TrackTitle != "Miss Modular" {
				t.Errorf("entries = %+v", p.Entries)
			}
		})
	}
}

func TestDJsOnAir(t *testing.T) {
	body := `[{"id":"u_lark","dj_name":"DJ Lark"},{"id":"u_anon","dj_name":null}]`
	srv := serve(t, "/flowsheet/djs-on-air", body, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	djs, raw, err := c.DJsOnAir(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(djs) != 2 || djs[0].DJName == nil || *djs[0].DJName != "DJ Lark" || djs[1].DJName != nil {
		t.Errorf("djs = %+v", djs)
	}
	if string(raw) != body {
		t.Errorf("raw = %s", raw)
	}
}
