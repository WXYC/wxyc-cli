package auth

import (
	"context"
	"io"
	"net/http"
	"time"
)

// Transport is an http.RoundTripper that authenticates every request with a
// bearer JWT from Token, and transparently refreshes once on a 401. Installing
// it on the http.Client used by the API layer means no individual command can
// forget to authenticate or mishandle an expired token.
//
// Token and Refresh match TokenProvider.Token / TokenProvider.Refresh, so a
// provider wires straight in.
//
// AttemptTimeout, when non-zero, bounds each HTTP attempt — from sending the
// request to closing the response body — but not the token fetch before it,
// which the provider bounds itself. The original request and the 401 retry
// each get the full budget.
type Transport struct {
	Base           http.RoundTripper
	Token          func(ctx context.Context) (string, error)
	Refresh        func(ctx context.Context) (string, error)
	AttemptTimeout time.Duration
}

func (t *Transport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := t.Token(req.Context())
	if err != nil {
		return nil, err
	}

	res, err := t.attempt(cloneWithAuth(req, tok))
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		return res, err
	}

	// The JWT was rejected — likely expired between our expiry check and the
	// request landing. Refresh once and retry. Bodies are replayed via GetBody.
	res.Body.Close()
	tok, err = t.Refresh(req.Context())
	if err != nil {
		return nil, err
	}
	retry, err := rewind(req)
	if err != nil {
		return nil, err
	}
	return t.attempt(cloneWithAuth(retry, tok))
}

// attempt sends one request under AttemptTimeout. The deadline stays in force
// while the caller reads the body and is released when the body is closed.
func (t *Transport) attempt(req *http.Request) (*http.Response, error) {
	if t.AttemptTimeout <= 0 {
		return t.base().RoundTrip(req)
	}
	ctx, cancel := context.WithTimeout(req.Context(), t.AttemptTimeout)
	res, err := t.base().RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	res.Body = &cancelOnClose{ReadCloser: res.Body, cancel: cancel}
	return res, nil
}

// cancelOnClose releases an attempt's deadline when its body is closed.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnClose) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func cloneWithAuth(req *http.Request, token string) *http.Request {
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

// rewind returns a request whose body is reset to the start, using GetBody when
// the original carried a replayable body.
func rewind(req *http.Request) (*http.Request, error) {
	if req.Body == nil || req.GetBody == nil {
		return req, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	r := req.Clone(req.Context())
	r.Body = body
	return r, nil
}
