package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderer_EmitRaw_JSONPassesThroughAllFields(t *testing.T) {
	var buf bytes.Buffer
	r := Renderer{JSON: true, Out: &buf}
	raw := []byte(`[{"id":1,"artist":"A","nested":{"deep":true},"extra":"kept"}]`)
	if err := r.EmitRaw(raw, []string{"ID"}, [][]string{{"1"}}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	// The full server payload survives, including fields the table never shows.
	for _, want := range []string{`"nested"`, `"deep"`, `"extra"`, `"kept"`} {
		if !strings.Contains(got, want) {
			t.Errorf("raw JSON output dropped %s:\n%s", want, got)
		}
	}
}

func TestRenderer_EmitRaw_TableUsesProjection(t *testing.T) {
	var buf bytes.Buffer
	r := Renderer{JSON: false, Out: &buf}
	raw := []byte(`[{"id":1,"artist":"A"}]`)
	if err := r.EmitRaw(raw, []string{"ARTIST"}, [][]string{{"Aphex Twin"}}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "{") {
		t.Errorf("table mode should not emit JSON:\n%s", got)
	}
	if !strings.Contains(got, "Aphex Twin") {
		t.Errorf("table missing projected row:\n%s", got)
	}
}

// In JSON mode a body that isn't valid JSON (e.g. truncated) must be an error,
// not passed through with exit 0.
func TestRenderer_EmitRaw_JSONRejectsInvalidJSON(t *testing.T) {
	var buf bytes.Buffer
	r := Renderer{JSON: true, Out: &buf}
	err := r.EmitRaw([]byte(`[{"artist_name":"X","rot`), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "response is not valid JSON") {
		t.Fatalf("EmitRaw() err = %v, want a 'response is not valid JSON' error", err)
	}
	if buf.Len() != 0 {
		t.Errorf("wrote partial output %q", buf.String())
	}
}
