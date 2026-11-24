package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// GET /flowsheet fixtures for the three on_air states (Backend-Service
// app.yaml OnAirInfo). Entries come newest first. In the live one, the newest
// row is a spotlight, and the newest track is not the first track row in the
// body, so the command must pick it by show and play order.
const (
	nowLive = `{"entries":[` +
		`{"id":9003,"show_id":4812,"play_order":13,"entry_type":"spotlight","spotlight_role":"performer","spotlight_name":"The Weirs"},` +
		`{"id":8990,"show_id":4811,"play_order":40,"entry_type":"track","artist_name":"Old Show","album_title":"Old Album","track_title":"Old Track"},` +
		`{"id":9001,"show_id":4812,"play_order":12,"entry_type":"track","artist_name":"Stereolab","album_title":"Dots and Loops","track_title":"Miss Modular"},` +
		`{"id":9000,"show_id":4812,"play_order":11,"entry_type":"track","artist_name":"Broadcast","album_title":"Tender Buttons","track_title":"Corporeal"}],` +
		`"on_air":{"dj_name":"DJ Lark","show_id":4812,"show_name":"Lark Hours",` +
		`"specialty":{"id":3,"slug":"new-science-experience","name":"New Science Experience","short_name":"NSE","tagline":null,"logs_playcuts":true},` +
		`"request_line":{"mode":"closed","source":"spotlight"},` +
		`"spotlight":{"id":9003,"show_id":4812,"play_order":13,"add_time":"2026-11-24T22:10:00.000Z","entry_type":"spotlight",` +
		`"spotlight_role":"performer","spotlight_name":"The Weirs","message":"The Weirs (performer)"}}}`
	nowLiveDJs = `[{"id":"u_lark","dj_name":"DJ Lark"},{"id":"u_anon","dj_name":null}]`

	// A freeform, untitled show with no spotlight and no computed request line.
	nowLiveBare = `{"entries":[],` +
		`"on_air":{"dj_name":"DJ Wren","show_id":4900,"show_name":null,"specialty":null,"spotlight":null}}`

	nowAuto    = `{"entries":[{"id":8990,"show_id":4811,"play_order":40,"entry_type":"track","artist_name":"Old Show","album_title":"Old Album","track_title":"Old Track"}],"on_air":null}`
	nowUnknown = `{"entries":[{"id":8990,"show_id":4811,"play_order":40,"entry_type":"track","artist_name":"Old Show","album_title":"Old Album","track_title":"Old Track"}]}`
)

func nowServer(t *testing.T, flowsheet, djs string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/flowsheet":
			w.Write([]byte(flowsheet))
		case "/flowsheet/djs-on-air":
			w.Write([]byte(djs))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
}

// nowField returns the value of the table row whose first column is name.
func nowField(t *testing.T, out, name string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, name+" "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func TestFlowsheetNow_Live(t *testing.T) {
	nowServer(t, nowLive, nowLiveDJs)
	out, err := runCLI(t, "flowsheet", "now")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"status":       "live",
		"djs":          "DJ Lark, ?",
		"show":         "Lark Hours (show 4812)",
		"specialty":    "New Science Experience",
		"request line": "closed (source: spotlight)",
		"spotlight":    "The Weirs (performer)",
		"last track":   "Stereolab – Miss Modular (Dots and Loops)",
	} {
		if got := nowField(t, out, name); got != want {
			t.Errorf("%s = %q, want %q\n%s", name, got, want, out)
		}
	}
}

func TestFlowsheetNow_LiveBare(t *testing.T) {
	nowServer(t, nowLiveBare, `[]`)
	out, err := runCLI(t, "flowsheet", "now")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"status":       "live",
		"djs":          "DJ Wren", // djs-on-air empty: fall back to on_air.dj_name
		"show":         "untitled (show 4900)",
		"specialty":    "none (freeform)",
		"request line": "unknown",
		"spotlight":    "none",
		"last track":   "none",
	} {
		if got := nowField(t, out, name); got != want {
			t.Errorf("%s = %q, want %q\n%s", name, got, want, out)
		}
	}
}

func TestFlowsheetNow_Automation(t *testing.T) {
	nowServer(t, nowAuto, `[]`)
	out, err := runCLI(t, "flowsheet", "now")
	if err != nil {
		t.Fatal(err)
	}
	if got := nowField(t, out, "status"); got != "Auto DJ" {
		t.Errorf("status = %q, want Auto DJ\n%s", got, out)
	}
	if got := nowField(t, out, "last track"); got != "Old Show – Old Track (Old Album)" {
		t.Errorf("last track = %q\n%s", got, out)
	}
	for _, absent := range []string{"show", "specialty", "request line", "spotlight", "djs"} {
		if got := nowField(t, out, absent); got != "" {
			t.Errorf("automation shows %s = %q\n%s", absent, got, out)
		}
	}
}

func TestFlowsheetNow_Unknown(t *testing.T) {
	nowServer(t, nowUnknown, `[{"id":"u_lark","dj_name":"DJ Lark"}]`)
	out, err := runCLI(t, "flowsheet", "now")
	if err != nil {
		t.Fatal(err)
	}
	if got := nowField(t, out, "status"); got != "unknown" {
		t.Errorf("status = %q, want unknown\n%s", got, out)
	}
	// The DJ list is its own endpoint, so it still reports who is on.
	if got := nowField(t, out, "djs"); got != "DJ Lark" {
		t.Errorf("djs = %q, want DJ Lark\n%s", got, out)
	}
	if got := nowField(t, out, "show"); got != "" {
		t.Errorf("unknown shows show = %q\n%s", got, out)
	}
}

func TestFlowsheetNow_JSONIsBothServerBodies(t *testing.T) {
	for _, body := range []string{nowLive, nowAuto, nowUnknown} {
		nowServer(t, body, nowLiveDJs)
		out, err := runCLI(t, "--json", "flowsheet", "now")
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Flowsheet map[string]json.RawMessage `json:"flowsheet"`
			DJsOnAir  []map[string]any           `json:"djs_on_air"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, out)
		}
		var want map[string]json.RawMessage
		_ = json.Unmarshal([]byte(body), &want)
		onAir, present := got.Flowsheet["on_air"]
		wantOnAir, wantPresent := want["on_air"]
		if present != wantPresent || (present && compactJSON(t, onAir) != compactJSON(t, wantOnAir)) {
			t.Errorf("on_air = %s (present %v), want %s (present %v)", onAir, present, wantOnAir, wantPresent)
		}
		if len(got.DJsOnAir) != 2 || got.DJsOnAir[1]["dj_name"] != nil {
			t.Errorf("djs_on_air = %+v", got.DJsOnAir)
		}
	}
}

func compactJSON(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(v)
	return string(out)
}

func TestFlowsheetNow_JSONKeepsHTMLCharacters(t *testing.T) {
	nowServer(t, nowLive, strings.Replace(nowLiveDJs, "DJ Lark", htmlText, 1))
	out, err := runCLI(t, "--json", "flowsheet", "now")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, htmlText) {
		t.Errorf("--json altered %q:\n%s", htmlText, out)
	}
}
