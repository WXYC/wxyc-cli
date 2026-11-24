package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLabelsShow_PrintsCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/labels/42" {
			t.Errorf("path = %q, want /labels/42", r.URL.Path)
		}
		w.Write([]byte(`{"id":42,"label_name":"Sub Pop","parent_label_id":7,
			"references":{"library":12,"rotation":3,"flowsheet":5,"intake_items":1},
			"child_labels":2}`))
	}))
	defer srv.Close()

	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	out, err := runCLI(t, "labels", "show", "42")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Sub Pop", "12", "7", "2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
}

func TestLabelsShow_NotFoundExits5(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Label not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	_, err := runCLI(t, "labels", "show", "999")
	if err == nil {
		t.Fatal("expected an error")
	}
	if mapExit(err) != ExitNotFound {
		t.Errorf("exit = %d, want ExitNotFound(%d)", mapExit(err), ExitNotFound)
	}
}
