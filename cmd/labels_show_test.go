package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Fixtures with the server's real key names: GET /labels/{id}
// (labelsService.getLabelWithReferences).
const (
	showLabel = `{"id":91,"label_name":"Duophonic","parent_label_id":12,` +
		`"references":{"library":38,"rotation":2,"flowsheet":1204,"intake_items":0},"child_labels":3}`
	showLabelNoParent = `{"id":12,"label_name":"Too Pure","parent_label_id":null,` +
		`"references":{"library":5,"rotation":0,"flowsheet":77,"intake_items":1},"child_labels":0}`
)

func labelShowServer(t *testing.T, wantPath, body string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if status != 0 {
			w.WriteHeader(status)
			w.Write([]byte(`{"message":"Label not found"}`))
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
}

func TestLabelsShow_Table(t *testing.T) {
	labelShowServer(t, "/labels/91", showLabel, 0)
	out, err := runCLI(t, "labels", "show", "91")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"id":                     "91",
		"label":                  "Duophonic",
		"parent id":              "12",
		"child labels":           "3",
		"references (library)":   "38",
		"references (rotation)":  "2",
		"references (flowsheet)": "1204",
		"references (intake)":    "0",
	} {
		if got := nowField(t, out, name); got != want {
			t.Errorf("%s = %q, want %q\n%s", name, got, want, out)
		}
	}
}

func TestLabelsShow_NullParentGetsNoRow(t *testing.T) {
	labelShowServer(t, "/labels/12", showLabelNoParent, 0)
	out, err := runCLI(t, "labels", "show", "12")
	if err != nil {
		t.Fatal(err)
	}
	if got := nowField(t, out, "parent id"); got != "" {
		t.Errorf("unexpected parent id row = %q\n%s", got, out)
	}
	if got := nowField(t, out, "references (intake)"); got != "1" {
		t.Errorf("references (intake) = %q, want 1\n%s", got, out)
	}
}

func TestLabelsShow_JSONIsRawBody(t *testing.T) {
	labelShowServer(t, "/labels/91", showLabel, 0)
	out, err := runCLI(t, "labels", "show", "91", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertSameJSON(t, "label", json.RawMessage(out), showLabel)
}

func TestLabelsShow_NotFoundIsExitNotFound(t *testing.T) {
	labelShowServer(t, "/labels/91", "", http.StatusNotFound)
	_, err := runCLI(t, "labels", "show", "91")
	if err == nil {
		t.Fatal("a missing label succeeded silently")
	}
	if got := mapExit(err); got != ExitNotFound {
		t.Errorf("exit = %d, want ExitNotFound(%d)", got, ExitNotFound)
	}
}

func TestLabelsShow_RejectsNonIntegerID(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	if _, err := runCLI(t, "labels", "show", "abc"); err == nil {
		t.Error("non-integer label id accepted")
	}
}
