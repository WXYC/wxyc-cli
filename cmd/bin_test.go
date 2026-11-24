package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/safety"
)

func TestBinRemove_BlockedWithoutWrite(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	t.Setenv("WXYC_ALLOW_WRITE", "")

	_, err := runCLI(t, "bin", "rm", "45029")
	var blocked *safety.BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("err = %v, want *BlockedError", err)
	}
	if mapExit(err) != ExitBlocked {
		t.Errorf("exit = %d, want ExitBlocked(%d)", mapExit(err), ExitBlocked)
	}
}

func TestBinRemove_DeletesAlbumFromBin(t *testing.T) {
	var method, albumID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		albumID = r.URL.Query().Get("album_id")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", fakeJWT("dj-42"))

	out, err := runCLI(t, "bin", "rm", "45029", "--write")
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || albumID != "45029" {
		t.Errorf("got %s with album_id %q, want DELETE with 45029", method, albumID)
	}
	if out == "" {
		t.Error("expected a confirmation message")
	}
}
