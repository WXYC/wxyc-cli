package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBin_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/djs/bin" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`[{"album_id":45029,"album_title":"Identical Sunsets","artist_name":"Paul Dunmall"}]`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	items, _, err := c.Bin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AlbumTitle != "Identical Sunsets" || items[0].AlbumID != 45029 {
		t.Errorf("unexpected bin %+v", items)
	}
}

func TestBin_Add_PostsAlbumID(t *testing.T) {
	var method, path string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	if err := c.BinAdd(context.Background(), 45029); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/djs/bin" {
		t.Errorf("got %s %s, want POST /djs/bin", method, path)
	}
	if body["album_id"].(float64) != 45029 {
		t.Errorf("body = %v, want album_id 45029", body)
	}
}

func TestBin_Add_PropagatesForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	err := c.BinAdd(context.Background(), 1)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusForbidden {
		t.Fatalf("BinAdd err = %v, want StatusError 403", err)
	}
}

func TestBin_Remove_SendsAlbumIDInQuery(t *testing.T) {
	var method, path, query string
	var bodyLen int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, query = r.Method, r.URL.Path, r.URL.Query().Get("album_id")
		raw, _ := io.ReadAll(r.Body)
		bodyLen = len(raw)
		// The server echoes the deleted bins row (drizzle .returning()).
		w.Write([]byte(`{"id":812,"dj_id":"u_abc123","album_id":45029,"track_title":null}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	e, raw, err := c.BinRemove(context.Background(), 45029)
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/djs/bin" {
		t.Errorf("got %s %s, want DELETE /djs/bin", method, path)
	}
	// deleteFromBin reads req.query.album_id, not the body.
	if query != "45029" {
		t.Errorf("album_id query = %q, want 45029", query)
	}
	if bodyLen != 0 {
		t.Errorf("sent a %d-byte body; the server ignores it", bodyLen)
	}
	if e.ID != 812 || e.AlbumID != 45029 || e.DJID != "u_abc123" || e.TrackTitle != nil {
		t.Errorf("decoded %+v", e)
	}
	if len(raw) == 0 {
		t.Error("raw bytes empty, want server response for --json")
	}
}

func TestBin_Remove_NotInBin(t *testing.T) {
	// removeFromBin returns rows[0] of an empty .returning(), so Express sends
	// 200 with an empty body when the album wasn't in the bin.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, _, err := c.BinRemove(context.Background(), 1)
	if !errors.Is(err, ErrNotInBin) {
		t.Fatalf("err = %v, want ErrNotInBin", err)
	}
}

func TestBin_Remove_PropagatesForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, _, err := c.BinRemove(context.Background(), 1)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusForbidden {
		t.Fatalf("BinRemove err = %v, want StatusError 403", err)
	}
}

// POST /djs/bin replies 201 with the inserted row, but BinAdd reports nothing
// from it. Once the status says the write landed, a reply body that breaks
// mid-read or isn't JSON must not turn that success into a failure — a script
// would retry a write that already happened.
func TestBin_Add_SuccessIgnoresReplyBody(t *testing.T) {
	tests := map[string]http.HandlerFunc{
		"truncated": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":812,"album`))
		},
		"not json": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`<html>proxy page</html>`))
		},
	}
	for name, h := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
			if err := c.BinAdd(context.Background(), 45029); err != nil {
				t.Errorf("BinAdd() = %v, want nil after a 201", err)
			}
		})
	}
}
