package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tokenServer stands in for GET {authBase}/token. It records the bearer it saw
// and how many times it was hit, and returns a JWT whose exp is `expUnix`.
func tokenServer(t *testing.T, expUnix int64, gotBearer *string, hits *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(hits, 1)
		if gotBearer != nil {
			*gotBearer = r.Header.Get("Authorization")
		}
		jwt := makeJWT(t, map[string]any{"sub": "u1", "role": "dj", "exp": expUnix})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"token": jwt})
	}))
}

func newProvider(base, session string, now func() time.Time) *TokenProvider {
	return &TokenProvider{
		AuthBase: base,
		HTTP:     http.DefaultClient,
		Session:  func() (string, error) { return session, nil },
		Skew:     30 * time.Second,
		Now:      now,
	}
}

func TestTokenProvider_ExchangesAndSendsSessionBearer(t *testing.T) {
	issued := makeJWT(t, map[string]any{"sub": "u1", "role": "dj", "exp": time.Now().Add(time.Hour).Unix()})
	var bearer, method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		bearer, method = r.Header.Get("Authorization"), r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"token": issued})
	}))
	defer srv.Close()

	p := newProvider(srv.URL, "sess-abc", time.Now)
	got, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got != issued {
		t.Errorf("Token() = %q, want the issued JWT %q", got, issued)
	}
	if method != http.MethodGet {
		t.Errorf("exchange method = %q, want GET", method)
	}
	if bearer != "Bearer sess-abc" {
		t.Errorf("exchange Authorization = %q, want %q", bearer, "Bearer sess-abc")
	}
}

func TestTokenProvider_CachesUntilExpiry(t *testing.T) {
	var hits int32
	srv := tokenServer(t, time.Now().Add(time.Hour).Unix(), nil, &hits)
	defer srv.Close()

	p := newProvider(srv.URL, "sess", time.Now)
	for i := 0; i < 3; i++ {
		if _, err := p.Token(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if hits != 1 {
		t.Errorf("server hit %d times, want 1 (cached)", hits)
	}
}

func TestTokenProvider_RefreshesWhenExpired(t *testing.T) {
	var hits int32
	base := time.Now()
	// JWT expires 10 minutes out.
	srv := tokenServer(t, base.Add(10*time.Minute).Unix(), nil, &hits)
	defer srv.Close()

	clock := base
	p := newProvider(srv.URL, "sess", func() time.Time { return clock })

	if _, err := p.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Advance past expiry; next call must re-fetch.
	clock = base.Add(20 * time.Minute)
	if _, err := p.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Errorf("server hit %d times, want 2 (refresh after expiry)", hits)
	}
}

// Any non-401 failure from the exchange is a server problem, not a dead
// session: it must not wrap ErrSessionExpired, or the CLI would tell the user
// to log in again (exit 3) for what is really a backend outage.
func TestTokenProvider_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newProvider(srv.URL, "sess", time.Now)
	_, err := p.Token(context.Background())
	if err == nil {
		t.Fatal("Token() = nil error on 500 exchange, want error")
	}
	if errors.Is(err, ErrSessionExpired) {
		t.Errorf("err = %v, must not wrap ErrSessionExpired on a 500", err)
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("err = %v, want it to name HTTP 500", err)
	}
}

// A 401 from the token exchange means the stored session token is dead. It must
// classify as an auth failure (ErrSessionExpired) so the CLI exits ExitAuth and
// agents know to re-login, rather than as an unclassified error.
func TestTokenProvider_RejectedSessionIsAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"session expired"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := newProvider(srv.URL, "stale-session", time.Now)
	_, err := p.Token(context.Background())
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("err = %v, want it to wrap ErrSessionExpired", err)
	}
}

// An empty stored session is the same "run login first" condition as a missing
// one, so it must carry ErrNoSession (→ ExitAuth), not a look-alike fmt error.
func TestTokenProvider_EmptySessionIsErrNoSession(t *testing.T) {
	p := &TokenProvider{
		AuthBase: "http://unused.invalid",
		HTTP:     http.DefaultClient,
		Session:  func() (string, error) { return "", nil },
		Skew:     30 * time.Second,
	}
	_, err := p.Token(context.Background())
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v, want it to wrap ErrNoSession", err)
	}
}

// A cached JWT inside the skew window counts as expired: with exp 10s out and
// a 30s skew, the next Token() must refetch rather than hand out a token that
// may lapse mid-request.
func TestTokenProvider_RefetchesInsideSkewWindow(t *testing.T) {
	var hits int32
	now := time.Now()
	srv := tokenServer(t, now.Add(10*time.Second).Unix(), nil, &hits)
	defer srv.Close()

	p := newProvider(srv.URL, "sess", func() time.Time { return now })
	for i := 0; i < 2; i++ {
		if _, err := p.Token(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if hits != 2 {
		t.Errorf("server hit %d times, want 2 (exp within skew must refetch)", hits)
	}
}

// Refresh is the transport's answer to a 401: the server already rejected the
// cached JWT, so it must refetch even though the cache looks fresh.
func TestTokenProvider_RefreshIgnoresFreshCache(t *testing.T) {
	var hits int32
	srv := tokenServer(t, time.Now().Add(time.Hour).Unix(), nil, &hits)
	defer srv.Close()

	p := newProvider(srv.URL, "sess", time.Now)
	if _, err := p.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if hits != 2 {
		t.Errorf("server hit %d times, want 2 (Refresh must bypass the cache)", hits)
	}
}

// End to end through the real provider: the API rejects the first JWT with a
// 401, the transport calls Refresh, and the retry carries the newly issued JWT.
func TestTokenProvider_TransportRetriesWithRefreshedJWT(t *testing.T) {
	var issued []string
	var mu sync.Mutex
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		jwt := makeJWT(t, map[string]any{"sub": "u1", "jti": len(issued), "exp": time.Now().Add(time.Hour).Unix()})
		issued = append(issued, jwt)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": jwt})
	}))
	defer authSrv.Close()

	var seen []string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if len(seen) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer apiSrv.Close()

	p := newProvider(authSrv.URL, "sess", time.Now)
	client := &http.Client{Transport: &Transport{Token: p.Token, Refresh: p.Refresh}}
	res, err := client.Get(apiSrv.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after refresh", res.StatusCode)
	}
	if len(issued) != 2 || len(seen) != 2 {
		t.Fatalf("issued %d JWTs, API saw %d requests; want 2 and 2", len(issued), len(seen))
	}
	if seen[0] != "Bearer "+issued[0] || seen[1] != "Bearer "+issued[1] {
		t.Errorf("API saw %q, want first then refreshed JWT %q", seen, issued)
	}
}
