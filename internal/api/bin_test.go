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

func TestBin_Remove_DeletesWithAlbumIDQuery(t *testing.T) {
	var method, path, albumID string
	var bodyLen int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		albumID = r.URL.Query().Get("album_id")
		bodyLen, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	if err := c.BinRemove(context.Background(), 45029); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/djs/bin" {
		t.Errorf("got %s %s, want DELETE /djs/bin", method, path)
	}
	if albumID != "45029" {
		t.Errorf("album_id = %q, want 45029 in the query string", albumID)
	}
	if bodyLen != 0 {
		t.Errorf("body length = %d, want no body", bodyLen)
	}
}
