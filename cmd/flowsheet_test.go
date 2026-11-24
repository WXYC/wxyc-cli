package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// fakeJWT builds an unsigned token whose payload carries the given sub claim.
// ParseClaims decodes the payload segment without verifying, so this is enough
// for the dj_id-from-token path (resolveDJID).
func fakeJWT(sub string) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + sub + `"}`))
	return "h." + payload + ".s"
}

// captureFlowsheetServer records the last mutating request and replies with resp.
func captureFlowsheetServer(t *testing.T, resp string) (*httptest.Server, *struct {
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
		if r.Method != http.MethodGet {
			got.Method, got.Path = r.Method, r.URL.Path
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &got.Body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestFlowsheetStart_PostsJoinWithTokenDJID(t *testing.T) {
	srv, got := captureFlowsheetServer(t, `{"id":77,"show_name":"Freeform","primary_dj_id":"dj-42","end_time":null}`)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	out, err := runCLI(t, "flowsheet", "start", "--name", "Freeform", "--write")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/flowsheet/join" {
		t.Errorf("path = %q, want /flowsheet/join", got.Path)
	}
	if got.Body["dj_id"] != "dj-42" {
		t.Errorf("dj_id = %v, want dj-42 (from token)", got.Body["dj_id"])
	}
	if !strings.Contains(out, "77") || !strings.Contains(out, "started") {
		t.Errorf("confirmation missing show/status:\n%s", out)
	}
}

func TestFlowsheetAdd_RequiresArtistAlbumWithoutAlbumID(t *testing.T) {
	// No server should be hit — validation fails before the request.
	t.Setenv("WXYC_JWT", "unused")

	_, err := runCLI(t, "flowsheet", "add", "--track", "T", "--write")
	if err == nil || !strings.Contains(err.Error(), "--artist and --album are required") {
		t.Fatalf("err = %v, want artist/album validation error", err)
	}
}

func TestFlowsheetAdd_PostsTrack(t *testing.T) {
	srv, got := captureFlowsheetServer(t, `{"id":501,"entry_type":"track","artist_name":"Boards","track_title":"Roygbiv"}`)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "add",
		"--track", "Roygbiv", "--artist", "Boards", "--album", "Music Has...", "--segue", "--write")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/flowsheet" {
		t.Errorf("path = %q, want /flowsheet", got.Path)
	}
	if got.Body["track_title"] != "Roygbiv" || got.Body["segue"] != true {
		t.Errorf("body = %v, want track + segue", got.Body)
	}
	if !strings.Contains(out, "501") || !strings.Contains(out, "Roygbiv") {
		t.Errorf("confirmation missing entry:\n%s", out)
	}
}

func TestFlowsheetTalkset_PostsMarker(t *testing.T) {
	srv, got := captureFlowsheetServer(t, `{"id":9,"entry_type":"talkset","message":"Talkset"}`)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	if _, err := runCLI(t, "flowsheet", "talkset", "--write"); err != nil {
		t.Fatal(err)
	}
	if got.Path != "/flowsheet" {
		t.Errorf("path = %q, want /flowsheet", got.Path)
	}
	if got.Body["entry_type"] != "talkset" || got.Body["message"] != "Talkset" {
		t.Errorf("body = %v, want talkset marker", got.Body)
	}
}

func TestFlowsheetMove_PatchesPlayOrder(t *testing.T) {
	srv, got := captureFlowsheetServer(t, `{"id":501,"entry_type":"track","play_order":2,"artist_name":"Boards"}`)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "move", "501", "2", "--write")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodPatch || got.Path != "/flowsheet/play-order" {
		t.Errorf("request = %s %s, want PATCH /flowsheet/play-order", got.Method, got.Path)
	}
	if got.Body["entry_id"] != float64(501) || got.Body["new_position"] != float64(2) {
		t.Errorf("body = %v, want entry_id 501 / new_position 2", got.Body)
	}
	if !strings.Contains(out, "501") {
		t.Errorf("confirmation missing entry:\n%s", out)
	}
}

func TestFlowsheetMove_RejectsNonIntAndBadPosition(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")

	if _, err := runCLI(t, "flowsheet", "move", "abc", "2", "--write"); err == nil ||
		!strings.Contains(err.Error(), "entry_id must be an integer") {
		t.Errorf("err = %v, want entry_id int error", err)
	}
	if _, err := runCLI(t, "flowsheet", "move", "501", "0", "--write"); err == nil ||
		!strings.Contains(err.Error(), "new_position must be >= 1") {
		t.Errorf("err = %v, want new_position >= 1 error", err)
	}
}

func TestFlowsheetEdit_PatchesChangedFieldsOnly(t *testing.T) {
	srv, got := captureFlowsheetServer(t, `{"id":501,"entry_type":"track","artist_name":"Boards"}`)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "edit", "501", "--artist", "Boards", "--segue", "--write")
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
	// Untouched flags must not be sent.
	for _, k := range []string{"track_title", "album_title", "request_flag", "message"} {
		if _, ok := data[k]; ok {
			t.Errorf("untouched field %q leaked: %v", k, data)
		}
	}
	if !strings.Contains(out, "501") {
		t.Errorf("confirmation missing entry:\n%s", out)
	}
}

func TestFlowsheetEdit_FailsFastWithNoFields(t *testing.T) {
	// No server should be hit — validation fails before the request.
	t.Setenv("WXYC_JWT", "unused")

	_, err := runCLI(t, "flowsheet", "edit", "501", "--write")
	if err == nil || !strings.Contains(err.Error(), "no fields to edit") {
		t.Fatalf("err = %v, want no-fields validation error", err)
	}
}

func TestFlowsheetRm_DeletesEntry(t *testing.T) {
	srv, got := captureFlowsheetServer(t, `{"id":501,"entry_type":"track","artist_name":"Boards","track_title":"Roygbiv"}`)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "rm", "501", "--write")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodDelete || got.Path != "/flowsheet" {
		t.Errorf("request = %s %s, want DELETE /flowsheet", got.Method, got.Path)
	}
	if got.Body["entry_id"] != float64(501) {
		t.Errorf("body = %v, want entry_id 501", got.Body)
	}
	if !strings.Contains(out, "501") || !strings.Contains(out, "Roygbiv") {
		t.Errorf("confirmation missing removed entry:\n%s", out)
	}
}

func TestFlowsheetEnd_PostsEndWithTokenDJID(t *testing.T) {
	srv, got := captureFlowsheetServer(t, `{"id":77,"end_time":"2026-07-10T05:00:00Z"}`)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	out, err := runCLI(t, "flowsheet", "end", "--write")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/flowsheet/end" || got.Body["dj_id"] != "dj-42" {
		t.Errorf("request = %s dj_id=%v, want /flowsheet/end dj-42", got.Path, got.Body["dj_id"])
	}
	if !strings.Contains(out, "ended") {
		t.Errorf("confirmation missing status:\n%s", out)
	}
}

// specialtyServer serves the catalog on GET and records the join like
// captureFlowsheetServer.
func specialtyServer(t *testing.T) (*httptest.Server, *struct {
	Joined bool
	Body   map[string]any
}) {
	t.Helper()
	got := &struct {
		Joined bool
		Body   map[string]any
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/specialty-shows":
			w.Write([]byte(`[{"id":3,"slug":"new-science-experience","active":true},{"id":5,"slug":"retired","active":false},{"id":8,"slug":"orange-county-special","active":true}]`))
		case r.URL.Path == "/flowsheet/join":
			got.Joined = true
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &got.Body)
			w.Write([]byte(`{"id":77,"show_name":"S","primary_dj_id":"dj-42","end_time":null}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestFlowsheetStart_Specialty(t *testing.T) {
	for _, tc := range []struct {
		flag    string
		wantID  float64
		wantErr string
	}{
		{"new-science-experience", 3, ""},
		{"7", 7, ""},
		{"nope", 0, "new-science-experience, orange-county-special"},
		{"retired", 0, "unknown specialty"},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			srv, got := specialtyServer(t)
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", fakeJWT("dj-42"))
			_, err := runCLI(t, "flowsheet", "start", "--specialty", tc.flag, "--write")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				if got.Joined {
					t.Error("join sent despite unknown slug")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Body["specialty_id"] != tc.wantID {
				t.Errorf("specialty_id = %v, want %v", got.Body["specialty_id"], tc.wantID)
			}
		})
	}
}

func TestFlowsheetShow_ListsEveryEntry(t *testing.T) {
	srv, _ := showServer(t, `[]`, map[string]string{"42": showFull42})
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "show", "42")
	if err != nil {
		t.Fatal(err)
	}
	// All three tracks, not just a preview's worth.
	for _, want := range []string{"White Fang", "Jarabi", "The Fakir", "talkset"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
}

func TestFlowsheetShow_RejectsNonIntID(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	_, err := runCLI(t, "flowsheet", "show", "abc")
	if err == nil || !strings.Contains(err.Error(), "show_id must be an integer") {
		t.Errorf("err = %v, want show_id integer error", err)
	}
}

func TestFlowsheetShow_JSONIsServerBody(t *testing.T) {
	srv, _ := showServer(t, `[]`, map[string]string{"42": showFull42})
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "--json", "flowsheet", "show", "42")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"show_djs"`) || !strings.Contains(out, `"The Fakir"`) {
		t.Errorf("--json should pass the full show through:\n%s", out)
	}
}

func TestFlowsheetShow_TableIsInPlayOrder(t *testing.T) {
	// The server returns a show's entries newest-first (play_order descending).
	body := `{"id":42,"entries":[` +
		`{"play_order":3,"entry_type":"show_end","dj_name":"Ryan Shaw"},` +
		`{"play_order":2,"entry_type":"track","artist_name":"Little Mazarn","track_title":"White Fang"},` +
		`{"play_order":1,"entry_type":"show_start","dj_name":"Ryan Shaw"}` +
		`]}`
	srv, _ := showServer(t, `[]`, map[string]string{"42": body})
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "show", "42")
	if err != nil {
		t.Fatal(err)
	}
	start, track, end := strings.Index(out, "show_start"), strings.Index(out, "White Fang"), strings.Index(out, "show_end")
	if !(start < track && track < end) {
		t.Errorf("table not in play order (start=%d track=%d end=%d):\n%s", start, track, end, out)
	}
}

func TestFlowsheetSpotlight_PostsEachRole(t *testing.T) {
	for _, role := range []string{"performer", "dj", "reader", "guest", "host"} {
		t.Run(role, func(t *testing.T) {
			srv, got := captureFlowsheetServer(t, `{"id":12,"entry_type":"spotlight","message":"In the studio: Weirs"}`)
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", "unused")

			out, err := runCLI(t, "flowsheet", "spotlight", "--role", role, "  The", "Weirs  ", "--write")
			if err != nil {
				t.Fatal(err)
			}
			if got.Method != http.MethodPost || got.Path != "/flowsheet" {
				t.Errorf("request = %s %s, want POST /flowsheet", got.Method, got.Path)
			}
			want := map[string]any{"entry_type": "spotlight", "spotlight_role": role, "spotlight_name": "The Weirs"}
			if len(got.Body) != len(want) {
				t.Errorf("body = %v, want exactly %v", got.Body, want)
			}
			for k, v := range want {
				if got.Body[k] != v {
					t.Errorf("body[%q] = %v, want %v", k, got.Body[k], v)
				}
			}
			if !strings.Contains(out, "12") || !strings.Contains(out, "In the studio: Weirs") {
				t.Errorf("confirmation missing entry:\n%s", out)
			}
		})
	}
}

func TestFlowsheetSpotlight_RefusesBadInputBeforeRequest(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"unknown role", []string{"--role", "drummer", "Weirs"}, "performer, dj, reader, guest, host"},
		{"missing role", []string{"Weirs"}, "performer, dj, reader, guest, host"},
		{"empty name", []string{"--role", "guest", "   "}, "1 to 128 characters"},
		{"no name", []string{"--role", "guest"}, "1 to 128 characters"},
		{"129-character name", []string{"--role", "guest", strings.Repeat("é", 129)}, "1 to 128 characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
			t.Cleanup(srv.Close)
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", "unused")

			_, err := runCLI(t, append([]string{"flowsheet", "spotlight", "--write"}, tc.args...)...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want mention of %q", err, tc.want)
			}
			if hits != 0 {
				t.Errorf("server hit %d times, want 0", hits)
			}
		})
	}
}

func TestFlowsheetSpotlight_AcceptsA128CharacterName(t *testing.T) {
	srv, got := captureFlowsheetServer(t, `{"id":1,"entry_type":"spotlight"}`)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	name := strings.Repeat("é", 128) // 128 runes, 256 bytes: the server counts code points
	if _, err := runCLI(t, "flowsheet", "spotlight", "--role", "guest", name, "--write"); err != nil {
		t.Fatal(err)
	}
	if got.Body["spotlight_name"] != name {
		t.Errorf("spotlight_name = %v, want the 128-character name", got.Body["spotlight_name"])
	}
}

func TestFlowsheetSpotlight_ServerRefusals(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantMsg  string
		wantExit int
	}{
		{"flag off", http.StatusForbidden,
			`{"message":"Spotlight entries are not enabled","code":"spotlight_disabled"}`,
			"who's on air is not turned on yet", ExitForbidden},
		{"validation", http.StatusBadRequest,
			`{"message":"Bad Request, a spotlight entry has no track fields"}`,
			"Bad Request, a spotlight entry has no track fields", ExitError},
		{"not on the show", http.StatusBadRequest,
			`{"message":"Bad Request: DJ not a member of show"}`,
			"you are not on the open show", ExitError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", "unused")

			_, err := runCLI(t, "flowsheet", "spotlight", "--role", "guest", "Weirs", "--write")
			if err == nil {
				t.Fatal("err = nil, want a refusal")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.wantMsg) || strings.Contains(msg, "\n") || strings.Contains(msg, "{") {
				t.Errorf("err = %q, want one line containing %q and no raw JSON", msg, tc.wantMsg)
			}
			if got := mapExit(err); got != tc.wantExit {
				t.Errorf("exit = %d, want %d", got, tc.wantExit)
			}
		})
	}
}

func TestFlowsheetSpotlight_HelpSaysNameIsPublic(t *testing.T) {
	out, err := runCLI(t, "flowsheet", "spotlight", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"public", "Lock Screen", "archive", "on air"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q:\n%s", want, out)
		}
	}
}

// spotlightEntry is a spotlight row as GET /flowsheet and /flowsheet/playlist
// send it (FlowsheetV2SpotlightEntry): no dj_name and no track fields.
func spotlightEntry(playOrder int, role, name string) string {
	return fmt.Sprintf(`{"id":%d,"show_id":42,"play_order":%d,"add_time":"2026-11-20T16:05:00.000Z",`+
		`"entry_type":"spotlight","spotlight_role":%q,"spotlight_name":%q,"message":"In the studio: %s"}`,
		900+playOrder, playOrder, role, name, name)
}

func TestFlowsheetTail_ShowsSpotlightNameAndRole(t *testing.T) {
	body := `{"entries":[` + spotlightEntry(2, "guest", "Jane Doe") + `,` +
		`{"id":1,"play_order":1,"entry_type":"track","artist_name":"A","track_title":"T","album_title":"Al"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "tail")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Jane Doe (guest)") {
		t.Errorf("table missing spotlight name and role:\n%s", out)
	}
}

func TestFlowsheetShow_ShowsSpotlightNameAndRole(t *testing.T) {
	for _, tc := range []struct{ role, want string }{
		{"performer", "The Weirs (performer)"},
		// The role list is open-ended: an unknown role is shown as-is.
		{"drummer", "The Weirs (drummer)"},
	} {
		t.Run(tc.role, func(t *testing.T) {
			body := `{"id":42,"entries":[` + spotlightEntry(2, tc.role, "The Weirs") + `,` +
				`{"play_order":1,"entry_type":"show_start","dj_name":"Ryan Shaw"}]}`
			srv, _ := showServer(t, `[]`, map[string]string{"42": body})
			t.Cleanup(srv.Close)
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", "unused")

			out, err := runCLI(t, "flowsheet", "show", "42")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("table missing %q:\n%s", tc.want, out)
			}
		})
	}
}

func TestFlowsheetEdit_SpotlightFieldsBuildPatchData(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want map[string]any
	}{
		{"role", []string{"--spotlight-role", "performer"}, map[string]any{"spotlight_role": "performer"}},
		{"name", []string{"--spotlight-name", "  The Weirs "}, map[string]any{"spotlight_name": "The Weirs"}},
		{"both", []string{"--spotlight-role", "reader", "--spotlight-name", "Jane Doe"},
			map[string]any{"spotlight_role": "reader", "spotlight_name": "Jane Doe"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := captureFlowsheetServer(t, spotlightEntry(3, "performer", "The Weirs"))
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", "unused")

			out, err := runCLI(t, append([]string{"flowsheet", "edit", "903", "--write"}, tc.args...)...)
			if err != nil {
				t.Fatal(err)
			}
			if got.Method != http.MethodPatch || got.Path != "/flowsheet" || got.Body["entry_id"] != float64(903) {
				t.Errorf("request = %s %s %v, want PATCH /flowsheet entry_id 903", got.Method, got.Path, got.Body)
			}
			if data := got.Body["data"]; !reflect.DeepEqual(data, tc.want) {
				t.Errorf("data = %v, want exactly %v", data, tc.want)
			}
			if !strings.Contains(out, "The Weirs (performer)") {
				t.Errorf("confirmation missing spotlight name and role:\n%s", out)
			}
		})
	}
}

func TestFlowsheetEdit_RefusesBadSpotlightInputBeforeRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown role", []string{"--spotlight-role", "drummer"}, "performer, dj, reader, guest, host"},
		{"empty role", []string{"--spotlight-role", ""}, "performer, dj, reader, guest, host"},
		{"empty name", []string{"--spotlight-name", "  "}, "1 to 128 characters"},
		{"129-character name", []string{"--spotlight-name", strings.Repeat("é", 129)}, "1 to 128 characters"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
			t.Cleanup(srv.Close)
			t.Setenv("WXYC_API_URL", srv.URL)
			t.Setenv("WXYC_JWT", "unused")

			_, err := runCLI(t, append([]string{"flowsheet", "edit", "903", "--write"}, tc.args...)...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want mention of %q", err, tc.want)
			}
			if hits != 0 {
				t.Errorf("server hit %d times, want 0", hits)
			}
		})
	}
}

// showWithHeader mirrors GET /flowsheet/playlist's top level as getShowInfo
// builds it: the shows row spread, specialty (SpecialtyShowRef), show_djs with
// active, and the adjacent-show ids — here the oldest show, so no previous.
const showWithHeader = `{"id":42,"show_name":"Late Night Static","primary_dj_id":"u1",` +
	`"legacy_dj_name":null,"specialty_id":7,` +
	`"start_time":"2026-07-17T17:59:38.121Z","end_time":"2026-07-17T20:59:04.215Z",` +
	`"specialty_show_name":"The New Science Experience",` +
	`"specialty":{"id":7,"slug":"new-science-experience","name":"The New Science Experience",` +
	`"short_name":"NSE","tagline":null,"logs_playcuts":true},` +
	`"show_djs":[{"id":"u1","dj_name":"DJ Alpha","active":true},` +
	`{"id":"u2","dj_name":"DJ Beta","active":false}],` +
	`"previous_show_id":null,"next_show_id":43,` +
	`"entries":[{"play_order":1,"entry_type":"track","artist_name":"Little Mazarn","track_title":"White Fang"}]}`

func TestFlowsheetShow_TablePrintsShowHeader(t *testing.T) {
	srv, _ := showServer(t, `[]`, map[string]string{"42": showWithHeader})
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "show", "42")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"show 42: Late Night Static",
		"program: The New Science Experience",
		"time:    2026-07-17T17:59:38.121Z – 2026-07-17T20:59:04.215Z",
		"djs:     DJ Alpha, DJ Beta (left)",
		"prev:    none",
		"next:    43",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("header missing %q:\n%s", want, out)
		}
	}
	// The header sits above the entries table.
	if h, e := strings.Index(out, "show 42"), strings.Index(out, "White Fang"); h < 0 || h > e {
		t.Errorf("header not above entries:\n%s", out)
	}
}

func TestFlowsheetShow_HeaderOmitsMissingProgramAndOpenEnd(t *testing.T) {
	// A regular show (specialty null) still on the air (end_time null), with a
	// legacy DJ who has no account and so no show_djs row.
	body := `{"id":42,"show_name":null,"specialty":null,"legacy_dj_name":"Ryan Shaw",` +
		`"start_time":"2026-07-17T17:59:38.121Z","end_time":null,"show_djs":[],` +
		`"previous_show_id":41,"next_show_id":null,"entries":[]}`
	srv, _ := showServer(t, `[]`, map[string]string{"42": body})
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "flowsheet", "show", "42")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "program:") {
		t.Errorf("regular show should have no program line:\n%s", out)
	}
	for _, want := range []string{"show 42\n", "time:    2026-07-17T17:59:38.121Z – (no end)", "djs:     Ryan Shaw", "prev:    41", "next:    none"} {
		if !strings.Contains(out, want) {
			t.Errorf("header missing %q:\n%s", want, out)
		}
	}
}

func TestFlowsheetShow_JSONHasNoHeader(t *testing.T) {
	srv, _ := showServer(t, `[]`, map[string]string{"42": showWithHeader})
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")

	out, err := runCLI(t, "--json", "flowsheet", "show", "42")
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("--json output is not pure JSON: %v\n%s", err, out)
	}
}
