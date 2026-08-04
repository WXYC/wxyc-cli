package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/auth"
)

// captureServer records the method, path, and decoded body of the single
// request it serves, then replies with resp.
func captureServer(t *testing.T, resp string) (*httptest.Server, *struct {
	Method string
	Path   string
	Body   map[string]any
}) {
	t.Helper()
	got := &struct {
		Method string
		Path   string
		Body   map[string]any
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Method, got.Path = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func newTestClient(url string) *Client {
	return &Client{BaseURL: url, HTTP: http.DefaultClient}
}

func TestFlowsheetStart_PostsJoinWithDJID(t *testing.T) {
	srv, got := captureServer(t, `{"id":77,"show_name":"Freeform","primary_dj_id":"dj-42","end_time":null}`)
	c := newTestClient(srv.URL)

	s, _, err := c.FlowsheetStart(context.Background(), StartShowRequest{DJID: "dj-42", ShowName: "Freeform"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodPost || got.Path != "/flowsheet/join" {
		t.Errorf("request = %s %s, want POST /flowsheet/join", got.Method, got.Path)
	}
	if got.Body["dj_id"] != "dj-42" || got.Body["show_name"] != "Freeform" {
		t.Errorf("body = %v, want dj_id/show_name set", got.Body)
	}
	if s.EffectiveShowID() != 77 {
		t.Errorf("show id = %d, want 77", s.EffectiveShowID())
	}
}

func TestFlowsheetStart_OmitsEmptyOptionals(t *testing.T) {
	srv, got := captureServer(t, `{"id":1}`)
	c := newTestClient(srv.URL)

	if _, _, err := c.FlowsheetStart(context.Background(), StartShowRequest{DJID: "dj-42"}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"show_name", "specialty_id", "dj_name_override"} {
		if _, ok := got.Body[k]; ok {
			t.Errorf("body carried empty optional %q: %v", k, got.Body)
		}
	}
}

func TestFlowsheetStart_CohostUsesShowID(t *testing.T) {
	// Joining an active show echoes a show_djs row (show_id, no id).
	srv, _ := captureServer(t, `{"show_id":88,"dj_id":"dj-9"}`)
	c := newTestClient(srv.URL)

	s, _, err := c.FlowsheetStart(context.Background(), StartShowRequest{DJID: "dj-9"})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != 0 || s.EffectiveShowID() != 88 {
		t.Errorf("cohost session = %+v, want EffectiveShowID 88 with ID 0", s)
	}
}

func TestFlowsheetEnd_PostsEndWithDJID(t *testing.T) {
	srv, got := captureServer(t, `{"id":77,"end_time":"2026-07-10T05:00:00Z"}`)
	c := newTestClient(srv.URL)

	if _, _, err := c.FlowsheetEnd(context.Background(), "dj-42"); err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodPost || got.Path != "/flowsheet/end" {
		t.Errorf("request = %s %s, want POST /flowsheet/end", got.Method, got.Path)
	}
	if got.Body["dj_id"] != "dj-42" {
		t.Errorf("body = %v, want dj_id dj-42", got.Body)
	}
}

func TestFlowsheetAddTrack_PostsTrackFields(t *testing.T) {
	srv, got := captureServer(t, `{"id":501,"entry_type":"track","artist_name":"Boards","track_title":"Roygbiv"}`)
	c := newTestClient(srv.URL)

	e, _, err := c.FlowsheetAddTrack(context.Background(), FlowsheetTrack{
		TrackTitle: "Roygbiv", ArtistName: "Boards", AlbumTitle: "Music Has...", Segue: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/flowsheet" {
		t.Errorf("path = %q, want /flowsheet", got.Path)
	}
	if got.Body["track_title"] != "Roygbiv" || got.Body["artist_name"] != "Boards" || got.Body["segue"] != true {
		t.Errorf("body = %v, want track fields + segue", got.Body)
	}
	// A message key must NOT be present — this is a track, not a marker.
	if _, ok := got.Body["message"]; ok {
		t.Errorf("track body leaked a message key: %v", got.Body)
	}
	if e.ID != 501 || e.EntryType != "track" {
		t.Errorf("result = %+v, want id 501 track", e)
	}
}

func TestFlowsheetAddTrack_WithAlbumIDOmitsBlankFields(t *testing.T) {
	srv, got := captureServer(t, `{"id":1,"entry_type":"track"}`)
	c := newTestClient(srv.URL)

	albumID := 45029
	if _, _, err := c.FlowsheetAddTrack(context.Background(), FlowsheetTrack{
		TrackTitle: "T", AlbumID: &albumID,
	}); err != nil {
		t.Fatal(err)
	}
	if got.Body["album_id"] != float64(45029) {
		t.Errorf("album_id = %v, want 45029", got.Body["album_id"])
	}
	// Blank artist/album must be omitted so the server takes the backfill path.
	for _, k := range []string{"artist_name", "album_title", "record_label"} {
		if _, ok := got.Body[k]; ok {
			t.Errorf("blank %q not omitted: %v", k, got.Body)
		}
	}
}

func TestFlowsheetAddMarker_PostsMessageAndType(t *testing.T) {
	srv, got := captureServer(t, `{"id":9,"entry_type":"talkset","message":"Talkset"}`)
	c := newTestClient(srv.URL)

	e, _, err := c.FlowsheetAddMarker(context.Background(), "Talkset", "talkset")
	if err != nil {
		t.Fatal(err)
	}
	if got.Body["message"] != "Talkset" || got.Body["entry_type"] != "talkset" {
		t.Errorf("body = %v, want message+entry_type", got.Body)
	}
	if e.EntryType != "talkset" {
		t.Errorf("result entry_type = %q, want talkset", e.EntryType)
	}
}

func TestFlowsheetMove_PatchesPlayOrder(t *testing.T) {
	srv, got := captureServer(t, `{"id":501,"entry_type":"track","play_order":2}`)
	c := newTestClient(srv.URL)

	e, _, err := c.FlowsheetMove(context.Background(), 501, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodPatch || got.Path != "/flowsheet/play-order" {
		t.Errorf("request = %s %s, want PATCH /flowsheet/play-order", got.Method, got.Path)
	}
	// iOS is the first real consumer of this endpoint, so assert the exact keys.
	if got.Body["entry_id"] != float64(501) || got.Body["new_position"] != float64(2) {
		t.Errorf("body = %v, want entry_id 501 / new_position 2", got.Body)
	}
	if e.ID != 501 {
		t.Errorf("result id = %d, want 501", e.ID)
	}
}

func TestFlowsheetUpdate_PatchesChangedFields(t *testing.T) {
	srv, got := captureServer(t, `{"id":501,"entry_type":"track","artist_name":"Boards"}`)
	c := newTestClient(srv.URL)

	artist := "Boards"
	segue := true
	e, _, err := c.FlowsheetUpdate(context.Background(), 501, FlowsheetUpdateFields{
		ArtistName: &artist, Segue: &segue,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodPatch || got.Path != "/flowsheet" {
		t.Errorf("request = %s %s, want PATCH /flowsheet", got.Method, got.Path)
	}
	if got.Body["entry_id"] != float64(501) {
		t.Errorf("entry_id = %v, want 501", got.Body["entry_id"])
	}
	data, ok := got.Body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %v, want object", got.Body["data"])
	}
	if data["artist_name"] != "Boards" || data["segue"] != true {
		t.Errorf("data = %v, want artist_name/segue set", data)
	}
	// Unset fields must be omitted so the server leaves them alone.
	for _, k := range []string{"album_title", "track_title", "request_flag", "message"} {
		if _, ok := data[k]; ok {
			t.Errorf("unset field %q leaked into data: %v", k, data)
		}
	}
	if e.ArtistName != "Boards" {
		t.Errorf("result artist = %q, want Boards", e.ArtistName)
	}
}

func TestFlowsheetUpdate_SendsEmptyStringToClear(t *testing.T) {
	srv, got := captureServer(t, `{"id":501}`)
	c := newTestClient(srv.URL)

	empty := ""
	if _, _, err := c.FlowsheetUpdate(context.Background(), 501, FlowsheetUpdateFields{
		RecordLabel: &empty,
	}); err != nil {
		t.Fatal(err)
	}
	data := got.Body["data"].(map[string]any)
	if v, ok := data["record_label"]; !ok || v != "" {
		t.Errorf("record_label = %v (present=%v), want cleared to empty string", v, ok)
	}
}

func TestFlowsheetDelete_DeletesWithBody(t *testing.T) {
	srv, got := captureServer(t, `{"id":501,"entry_type":"track","artist_name":"Boards"}`)
	c := newTestClient(srv.URL)

	e, _, err := c.FlowsheetDelete(context.Background(), 501)
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodDelete || got.Path != "/flowsheet" {
		t.Errorf("request = %s %s, want DELETE /flowsheet", got.Method, got.Path)
	}
	if got.Body["entry_id"] != float64(501) {
		t.Errorf("body = %v, want entry_id 501", got.Body)
	}
	if e.ID != 501 {
		t.Errorf("result id = %d, want 501", e.ID)
	}
}

// TestFlowsheetDelete_RewindsBodyOn401 exercises the unusual DELETE-with-body
// path through the auth Transport: a 401 on the first attempt must be retried
// with the body replayed via GetBody, not sent empty.
func TestFlowsheetDelete_RewindsBodyOn401(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":501}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: &http.Client{Transport: &auth.Transport{
		Token:   func(context.Context) (string, error) { return "t", nil },
		Refresh: func(context.Context) (string, error) { return "t2", nil },
	}}}

	if _, _, err := c.FlowsheetDelete(context.Background(), 501); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 {
		t.Fatalf("got %d requests, want 2 (401 then retry)", len(bodies))
	}
	if bodies[0] == "" || bodies[0] != bodies[1] {
		t.Errorf("retry body = %q, want replay of %q", bodies[1], bodies[0])
	}
}

func TestFlowsheetWrite_404Surfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)

	_, _, err := c.FlowsheetDelete(context.Background(), 999)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		t.Fatalf("err = %v, want StatusError 404", err)
	}
}

func TestFlowsheetWrite_StatusErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Bad Request, There are no active shows"))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)

	_, _, err := c.FlowsheetAddTrack(context.Background(), FlowsheetTrack{TrackTitle: "T"})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusBadRequest {
		t.Fatalf("err = %v, want StatusError 400", err)
	}
}
