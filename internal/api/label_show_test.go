package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// labelWithReferencesBody mirrors GET /labels/{id} (Backend-Service
// labelsService.getLabelWithReferences): the labels row plus references
// counted per LABEL_REFERENCES source and child_labels.
const labelWithReferencesBody = `{"id":91,"label_name":"Duophonic","parent_label_id":12,` +
	`"references":{"library":38,"rotation":2,"flowsheet":1204,"intake_items":0},"child_labels":3}`

func TestLabel(t *testing.T) {
	srv := serve(t, "/labels/91", labelWithReferencesBody, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	l, raw, err := c.Label(context.Background(), 91)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != labelWithReferencesBody {
		t.Errorf("raw = %s, want the body verbatim", raw)
	}
	if l.ID != 91 || l.LabelName != "Duophonic" || l.ParentLabelID == nil || *l.ParentLabelID != 12 {
		t.Errorf("label = %+v", l.Label)
	}
	want := LabelReferences{Library: 38, Rotation: 2, Flowsheet: 1204, IntakeItems: 0}
	if l.References != want {
		t.Errorf("references = %+v, want %+v", l.References, want)
	}
	if l.ChildLabels != 3 {
		t.Errorf("child_labels = %d, want 3", l.ChildLabels)
	}
}

func TestLabel_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Label not found"}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, _, err := c.Label(context.Background(), 91)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		t.Errorf("err = %v, want 404 StatusError", err)
	}
}
