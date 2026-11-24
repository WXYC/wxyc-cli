package cmd

import (
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
			w.Write([]byte(flowsheet))
		case "/flowsheet/djs-on-air":
			w.Write([]byte(`[{"id":"dj-42","dj_name":"DJ Current"}]`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
}

func TestFlowsheetNow_Live(t *testing.T) {
	srv := nowServer(t, `{"entries":[
		{"id":3,"entry_type":"track","artist_name":"Boards","track_title":"Roygbiv","album_title":"Music Has..."}
	],"on_air":{"dj_name":"DJ Current","show_id":9,"show_name":"Freeform",
		"specialty":{"id":1,"slug":"new-science","name":"New Science","short_name":"NS"},
		"request_line":{"mode":"requests","source":"dj"},
		"spotlight":{"id":5,"show_id":9,"play_order":4,"spotlight_name":"Spot Artist","message":"Spot"}}}`)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	out, err := runCLI(t, "flowsheet", "now")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"live", "DJ Current", "Freeform", "New Science",
		"requests (dj)", "Spot Artist", "Boards", "Roygbiv"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
}

func TestFlowsheetNow_NullIsAutoDJ(t *testing.T) {
	srv := nowServer(t, `{"entries":[],"on_air":null}`)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	out, err := runCLI(t, "flowsheet", "now")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Auto DJ") {
		t.Errorf("output %q missing Auto DJ", out)
	}
}

func TestFlowsheetNow_AbsentIsUnknown(t *testing.T) {
	srv := nowServer(t, `{"entries":[]}`)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	out, err := runCLI(t, "flowsheet", "now")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "unknown") {
		t.Errorf("output %q missing unknown", out)
	}
	if strings.Contains(out, "Auto DJ") {
		t.Errorf("output %q should not say Auto DJ when the field is absent", out)
	}
}

func TestFlowsheetNow_JSONWrapsBothBodies(t *testing.T) {
	srv := nowServer(t, `{"entries":[],"on_air":null}`)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	out, err := runCLI(t, "flowsheet", "now", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"flowsheet"`, `"djs_on_air"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json %q missing %q", out, want)
		}
	}
}
