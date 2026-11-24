package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransport_InjectsBearer(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := &Transport{
		Token:   func(context.Context) (string, error) { return "jwt-1", nil },
		Refresh: func(context.Context) (string, error) { return "jwt-1", nil },
	}
	client := &http.Client{Transport: tr}
	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if seen != "Bearer jwt-1" {
		t.Errorf("Authorization = %q, want %q", seen, "Bearer jwt-1")
	}
}

func TestTransport_RefreshesOn401AndRetries(t *testing.T) {
	var refreshes int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only the refreshed token is accepted.
		if r.Header.Get("Authorization") == "Bearer jwt-new" {
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "ok")
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := &Transport{
		Token: func(context.Context) (string, error) { return "jwt-stale", nil },
		Refresh: func(context.Context) (string, error) {
			atomic.AddInt32(&refreshes, 1)
			return "jwt-new", nil
		},
	}
	client := &http.Client{Transport: tr}
	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("final status = %d, want 200", res.StatusCode)
	}
	if refreshes != 1 {
		t.Errorf("refreshes = %d, want 1", refreshes)
	}
}

func TestTransport_GivesUpAfterOneRetry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusUnauthorized) // always reject
	}))
	defer srv.Close()

	tr := &Transport{
		Token:   func(context.Context) (string, error) { return "a", nil },
		Refresh: func(context.Context) (string, error) { return "b", nil },
	}
	client := &http.Client{Transport: tr}
	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
	if hits != 2 { // original + one retry, then give up
		t.Errorf("upstream hits = %d, want 2", hits)
	}
}

// Body must be replayable on retry.
func TestTransport_ReplaysBodyOnRetry(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if r.Header.Get("Authorization") == "Bearer new" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := &Transport{
		Token:   func(context.Context) (string, error) { return "old", nil },
		Refresh: func(context.Context) (string, error) { return "new", nil },
	}
	client := &http.Client{Transport: tr}
	res, err := client.Post(srv.URL, "text/plain", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(bodies) != 2 || bodies[0] != "payload" || bodies[1] != "payload" {
		t.Errorf("bodies = %v, want both %q", bodies, "payload")
	}
}

// AttemptTimeout bounds each request to the API, not the token fetch before
// it: a slow token exchange must not eat into the request's budget.
func TestTransport_AttemptTimeoutExcludesTokenFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	tr := &Transport{
		Token: func(context.Context) (string, error) {
			time.Sleep(300 * time.Millisecond)
			return "jwt", nil
		},
		Refresh:        func(context.Context) (string, error) { return "jwt", nil },
		AttemptTimeout: 250 * time.Millisecond,
	}
	res, err := (&http.Client{Transport: tr}).Get(srv.URL)
	if err != nil {
		t.Fatalf("Get() err = %v, want success", err)
	}
	defer res.Body.Close()
	if b, err := io.ReadAll(res.Body); err != nil || string(b) != "ok" {
		t.Errorf("body = %q, %v; want %q", b, err, "ok")
	}
}

// A request that hangs is cut off after AttemptTimeout, including while the
// body is still being read.
func TestTransport_AttemptTimeoutBoundsHangingRequest(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	tr := &Transport{
		Token:          func(context.Context) (string, error) { return "jwt", nil },
		Refresh:        func(context.Context) (string, error) { return "jwt", nil },
		AttemptTimeout: 100 * time.Millisecond,
	}
	done := make(chan error, 1)
	go func() {
		res, err := (&http.Client{Transport: tr}).Get(srv.URL)
		if err == nil {
			_, err = io.ReadAll(res.Body)
			res.Body.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hanging request not cut off after AttemptTimeout")
	}
}

// A failure to obtain a token (e.g. a dead session) must surface from the
// request, still classifiable via errors.Is through the *url.Error wrapper.
func TestTransport_TokenErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request sent despite Token error")
	}))
	defer srv.Close()

	tr := &Transport{
		Token:   func(context.Context) (string, error) { return "", ErrSessionExpired },
		Refresh: func(context.Context) (string, error) { return "unused", nil },
	}
	_, err := (&http.Client{Transport: tr}).Get(srv.URL)
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("err = %v, want it to wrap ErrSessionExpired", err)
	}
}

// A failed refresh after a 401 must surface as an error, not as the 401 or a
// retry with no token.
func TestTransport_RefreshErrorPropagates(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := &Transport{
		Token:   func(context.Context) (string, error) { return "stale", nil },
		Refresh: func(context.Context) (string, error) { return "", ErrSessionExpired },
	}
	_, err := (&http.Client{Transport: tr}).Get(srv.URL)
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("err = %v, want it to wrap ErrSessionExpired", err)
	}
	if hits != 1 {
		t.Errorf("upstream hits = %d, want 1 (no retry after failed refresh)", hits)
	}
}

// A body that can't be replayed (no GetBody) was consumed by the first
// attempt, so retrying would send an empty or truncated body. The transport
// must return the 401 instead of retrying.
func TestTransport_NoRetryWithoutReplayableBody(t *testing.T) {
	var hits, refreshes int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := &Transport{
		Token: func(context.Context) (string, error) { return "old", nil },
		Refresh: func(context.Context) (string, error) {
			atomic.AddInt32(&refreshes, 1)
			return "new", nil
		},
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL, io.NopCloser(strings.NewReader("payload")))
	if err != nil {
		t.Fatal(err)
	}
	if req.GetBody != nil {
		t.Fatal("test setup: GetBody should be nil for an opaque body")
	}
	res, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
	if hits != 1 || refreshes != 0 {
		t.Errorf("hits = %d, refreshes = %d; want 1 and 0 (no retry)", hits, refreshes)
	}
}

// With no Refresh (a static token), retrying a 401 would resend the identical
// token: the transport must return the 401 after one attempt.
func TestTransport_NilRefreshDoesNotRetry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := &Transport{Token: func(context.Context) (string, error) { return "static", nil }}
	res, err := (&http.Client{Transport: tr}).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
	if hits != 1 {
		t.Errorf("upstream hits = %d, want 1 (no retry without Refresh)", hits)
	}
}
