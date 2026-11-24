package cmd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func captureLibraryServer(t *testing.T) (*httptest.Server, *url.Values) {
	t.Helper()
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/library/" {
			t.Errorf("got %s %s, want GET /library/", r.Method, r.URL.Path)
		}
		got = r.URL.Query()
		w.Write([]byte(`[]`))
	}))
	return srv, &got
}

func TestLibrarySearch_OnStreamingSetsTrue(t *testing.T) {
	srv, got := captureLibraryServer(t)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	if _, err := runCLI(t, "library", "search", "--on-streaming"); err != nil {
		t.Fatal(err)
	}
	if v := got.Get("on_streaming"); v != "true" {
		t.Errorf("on_streaming = %q, want true", v)
	}
}

func TestLibrarySearch_NotOnStreamingSetsFalse(t *testing.T) {
	srv, got := captureLibraryServer(t)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	if _, err := runCLI(t, "library", "search", "--not-on-streaming"); err != nil {
		t.Fatal(err)
	}
	if v := got.Get("on_streaming"); v != "false" {
		t.Errorf("on_streaming = %q, want false", v)
	}
}

func TestLibrarySearch_StreamingUnsetSendsNothing(t *testing.T) {
	srv, got := captureLibraryServer(t)
	defer srv.Close()
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	if _, err := runCLI(t, "library", "search", "--artist", "aphex"); err != nil {
		t.Fatal(err)
	}
	if _, ok := (*got)["on_streaming"]; ok {
		t.Errorf("on_streaming = %q, want the key absent", got.Get("on_streaming"))
	}
}

func TestLibrarySearch_StreamingFlagsAreMutuallyExclusive(t *testing.T) {
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	_, err := runCLI(t, "library", "search", "--on-streaming", "--not-on-streaming")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("err = %v, want a mutual-exclusion error", err)
	}
}
