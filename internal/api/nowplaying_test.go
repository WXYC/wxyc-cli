package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func nowServer(t *testing.T, flowsheet string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/flowsheet":
			if got := r.URL.Query().Get("limit"); got != "20" {
				t.Errorf("limit = %q, want 20", got)
			}
			w.Write([]byte(flowsheet))
		case "/flowsheet/djs-on-air":
			w.Write([]byte(`[{"id":"dj-42","dj_name":"DJ Current"}]`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
}

func TestNowPlaying_LiveOnAir(t *testing.T) {
	srv := nowServer(t, `{"entries":[
		{"id":1,"entry_type":"track","artist_name":"A","track_title":"One","album_title":"First"},
		{"id":2,"entry_type":"talkset","message":"talk"},
		{"id":3,"entry_type":"track","artist_name":"B","track_title":"Two","album_title":"Second"}
	],"on_air":{"dj_name":"DJ Current","show_id":9,"show_name":"Freeform",
		"specialty":{"id":1,"slug":"new-science","name":"New Science","short_name":"NS"},
		"request_line":{"mode":"requests","source":"dj"},
		"spotlight":{"id":5,"show_id":9,"play_order":4,"spotlight_name":"Spot Artist","message":"Spot"}}}`)
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	now, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if now.State != OnAirLive {
		t.Errorf("state = %q, want live", now.State)
	}
	if now.OnAir == nil || now.OnAir.ShowID != 9 || *now.OnAir.ShowName != "Freeform" {
		t.Fatalf("on_air = %+v", now.OnAir)
	}
	if now.OnAir.RequestLine.Mode != "requests" || now.OnAir.RequestLine.Source != "dj" {
		t.Errorf("request_line = %+v", now.OnAir.RequestLine)
	}
	if now.OnAir.Specialty.Name != "New Science" || now.OnAir.Spotlight.SpotlightName != "Spot Artist" {
		t.Errorf("specialty/spotlight = %+v / %+v", now.OnAir.Specialty, now.OnAir.Spotlight)
	}
	if now.LastTrack == nil || now.LastTrack.TrackTitle != "Two" {
		t.Errorf("last track = %+v, want the newest track entry", now.LastTrack)
	}
	if len(now.DJs) != 1 || *now.DJs[0].DJName != "DJ Current" {
		t.Errorf("djs = %+v", now.DJs)
	}
	if !strings.Contains(string(now.Raw), `"flowsheet"`) || !strings.Contains(string(now.Raw), `"djs_on_air"`) {
		t.Errorf("raw = %s, want both bodies", now.Raw)
	}
}

func TestNowPlaying_NullOnAirIsAutomation(t *testing.T) {
	srv := nowServer(t, `{"entries":[],"on_air":null}`)
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	now, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if now.State != OnAirAuto || now.OnAir != nil {
		t.Errorf("state = %q on_air = %+v, want auto with no object", now.State, now.OnAir)
	}
}

func TestNowPlaying_AbsentOnAirIsUnknown(t *testing.T) {
	srv := nowServer(t, `{"entries":[]}`)
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	now, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if now.State != OnAirUnknown || now.OnAir != nil {
		t.Errorf("state = %q on_air = %+v, want unknown with no object", now.State, now.OnAir)
	}
}
