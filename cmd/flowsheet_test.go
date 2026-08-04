package cmd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/safety"
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

func TestFlowsheetStart_BlockedWithoutWrite(t *testing.T) {
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))
	t.Setenv("WXYC_ALLOW_WRITE", "")

	_, err := runCLI(t, "flowsheet", "start")
	var blocked *safety.BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("err = %v, want *BlockedError", err)
	}
	if mapExit(err) != ExitBlocked {
		t.Errorf("exit = %d, want ExitBlocked(%d)", mapExit(err), ExitBlocked)
	}
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

func TestFlowsheetMove_BlockedWithoutWrite(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	t.Setenv("WXYC_ALLOW_WRITE", "")

	_, err := runCLI(t, "flowsheet", "move", "501", "2")
	if mapExit(err) != ExitBlocked {
		t.Errorf("exit = %d, want ExitBlocked(%d)", mapExit(err), ExitBlocked)
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

func TestFlowsheetRm_BlockedWithoutWrite(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	t.Setenv("WXYC_ALLOW_WRITE", "")

	_, err := runCLI(t, "flowsheet", "rm", "501")
	if mapExit(err) != ExitBlocked {
		t.Errorf("exit = %d, want ExitBlocked(%d)", mapExit(err), ExitBlocked)
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
