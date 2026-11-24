package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLabel_Show_DecodesReferences(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/labels/42" {
			t.Errorf("got %s %s, want GET /labels/42", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"id":42,"label_name":"Sub Pop","parent_label_id":7,
			"references":{"library":12,"rotation":3,"flowsheet":5,"intake_items":1},
			"child_labels":2}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	label, raw, err := c.Label(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if label.LabelName != "Sub Pop" || label.ParentLabelID == nil || *label.ParentLabelID != 7 {
		t.Errorf("unexpected label %+v", label)
	}
	if label.References.Library != 12 || label.References.Rotation != 3 ||
		label.References.Flowsheet != 5 || label.References.IntakeItems != 1 {
		t.Errorf("references = %+v", label.References)
	}
	if label.ChildLabels != 2 {
		t.Errorf("child_labels = %d, want 2", label.ChildLabels)
	}
	if len(raw) == 0 {
		t.Error("expected raw JSON for --json passthrough")
	}
}

func TestLabel_Show_PropagatesNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Label not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, _, err := c.Label(context.Background(), 999)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		t.Fatalf("err = %v, want StatusError 404", err)
	}
}
