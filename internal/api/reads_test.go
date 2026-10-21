package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, wantPath string, body string, capture *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantPath != "" && r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if capture != nil {
			*capture = r.URL.RawQuery
		}
		w.Write([]byte(body))
	}))
}

func TestGenres(t *testing.T) {
	// last_modified is not modeled on Genre; it must still survive in raw.
	body := `[{"id":1,"genre_name":"Africa","plays":3,"last_modified":"2026-01-20T00:34:25.969Z"}]`
	srv := serve(t, "/library/genres", body, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	g, raw, err := c.Genres(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(g) != 1 || g[0].GenreName != "Africa" || g[0].Plays != 3 {
		t.Errorf("decoded %+v", g)
	}
	// Fidelity guarantee: raw carries the unmodeled field for --json.
	if !strings.Contains(string(raw), "last_modified") {
		t.Errorf("raw dropped unmodeled field: %s", raw)
	}
}

func TestFormats(t *testing.T) {
	srv := serve(t, "/library/formats", `[{"id":3,"format_name":"vinyl"}]`, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	f, _, err := c.Formats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(f) != 1 || f[0].FormatName != "vinyl" {
		t.Errorf("got %+v", f)
	}
}

func TestLabelsAndSearch(t *testing.T) {
	srv := serve(t, "/labels", `[{"id":7,"label_name":"June Appal"}]`, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	l, _, err := c.Labels(context.Background())
	if err != nil || len(l) != 1 || l[0].LabelName != "June Appal" {
		t.Fatalf("Labels got %+v err %v", l, err)
	}

	var q string
	ssrv := serve(t, "/labels/search", `[{"id":7,"label_name":"June Appal"}]`, &q)
	defer ssrv.Close()
	sc := &Client{BaseURL: ssrv.URL, HTTP: ssrv.Client()}
	if _, _, err := sc.LabelSearch(context.Background(), "appal", 5); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "q=appal") || !strings.Contains(q, "limit=5") {
		t.Errorf("query = %q, want q=appal&limit=5", q)
	}
}

func TestSchedule(t *testing.T) {
	srv := serve(t, "/schedule", `[{"id":1,"day":1,"start_time":"08:00","show_duration":120}]`, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	s, _, err := c.Schedule(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0].Day != 1 || s[0].StartTime != "08:00" {
		t.Errorf("got %+v", s)
	}
}

func TestRotation_RawPassthrough(t *testing.T) {
	body := `[{"id":9,"artist_name":"X","reconciled_identity":{"discogs_artist_id":42}}]`
	srv := serve(t, "/library/rotation", body, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	raw, err := c.Rotation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The full nested identity must survive to the raw bytes.
	if !strings.Contains(string(raw), "reconciled_identity") || !strings.Contains(string(raw), "42") {
		t.Errorf("raw rotation dropped nested fields: %s", raw)
	}
}

func TestGetRaw_MapsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, err := c.getRaw(context.Background(), "/library/rotation", nil)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusForbidden {
		t.Fatalf("getRaw err = %v, want StatusError 403", err)
	}
}

func TestSpecialtyShows(t *testing.T) {
	body := `[{"id":3,"slug":"new-science-experience","name":"New Science Experience","active":true,"tagline":"x"}]`
	srv := serve(t, "/specialty-shows", body, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	s, raw, err := c.SpecialtyShows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0].ID != 3 || s[0].Slug != "new-science-experience" || !s[0].Active {
		t.Errorf("decoded %+v", s)
	}
	if !strings.Contains(string(raw), "tagline") {
		t.Errorf("raw dropped unmodeled field: %s", raw)
	}
}
