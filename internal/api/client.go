// Package api is a thin typed client for the WXYC backend REST API. It maps
// HTTP status codes to a StatusError that the command layer translates into
// distinct process exit codes.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client talks to the WXYC backend. HTTP is expected to carry the auth
// Transport, so the client itself never touches tokens.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// maxErrorBody caps how much of a StatusError's body Error() prints, so a
// proxy's HTML error page isn't dumped whole to stderr.
const maxErrorBody = 4096

// maxErrorRead caps how much of a non-2xx body a StatusError stores. It is far
// above maxErrorBody so a long JSON refusal stays whole and parseable by the
// command layer; it only guards memory against a runaway error page.
const maxErrorRead = 1 << 20

// StatusError is returned for any non-2xx response. Code is the HTTP status;
// the command layer maps it to an exit code (401→auth, 403→forbidden, …).
type StatusError struct {
	Code int
	Path string
	Body string
}

func (e *StatusError) Error() string {
	msg := e.Body
	if len(msg) > maxErrorBody {
		// Cutting by bytes can split a multi-byte rune; drop the fragment.
		msg = strings.ToValidUTF8(msg[:maxErrorBody], "")
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = http.StatusText(e.Code)
	}
	return fmt.Sprintf("%s: HTTP %d: %s", e.Path, e.Code, msg)
}

// getInto fetches path, returns the response body verbatim, and (when out is
// non-nil) decodes a copy into out. Read commands use the raw return for
// --json passthrough and the decoded copy to build the human table, so the two
// views never drift and no server field is silently dropped from --json.
func (c *Client) getInto(ctx context.Context, path string, query map[string]string, out any) ([]byte, error) {
	raw, err := c.getRaw(ctx, path, query)
	if err != nil {
		return nil, err
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return raw, fmt.Errorf("%s: decoding response: %w", path, err)
		}
	}
	return raw, nil
}

// getRaw fetches path and returns the response body verbatim, for endpoints
// whose full shape the CLI passes through to --json without modeling it.
func (c *Client) getRaw(ctx context.Context, path string, query map[string]string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, query, nil)
}

// deleteQueryRaw sends a body-less DELETE with its parameters in the query
// string, for endpoints that read req.query (e.g. DELETE /djs/bin), unlike
// DELETE /flowsheet, which reads a JSON body (see FlowsheetDelete).
func (c *Client) deleteQueryRaw(ctx context.Context, path string, query map[string]string) ([]byte, error) {
	return c.do(ctx, http.MethodDelete, path, query, nil)
}

// patchRaw sends a JSON body to path via PATCH and returns the response body
// verbatim, for a write whose echo the CLI passes through without modeling
// (e.g. PATCH /flowsheet/shows/{id}).
func (c *Client) patchRaw(ctx context.Context, path string, in any) ([]byte, error) {
	return c.do(ctx, http.MethodPatch, path, nil, in)
}

// do is the request path behind every method above that wants the response
// body. The body is returned verbatim; on a 2xx, a failed read (e.g. the
// connection dropped mid-body) is an error rather than a truncated success,
// as is a body that isn't JSON (e.g. a proxy's HTML error page), named by
// path. An empty body (204 No Content) is returned as is.
func (c *Client) do(ctx context.Context, method, path string, query map[string]string, in any) ([]byte, error) {
	res, err := c.send(ctx, method, path, query, in)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: reading response: %w", path, err)
	}
	if len(bytes.TrimSpace(body)) > 0 && !json.Valid(body) {
		return nil, fmt.Errorf("%s: response is not valid JSON", path)
	}
	return body, nil
}

// doDiscard is do for writes that report nothing from the reply: once a 2xx
// says the write landed, the body is drained and dropped, so a reply that
// breaks mid-read or isn't JSON can't turn a completed write into a failure
// (and a script into retrying it).
func (c *Client) doDiscard(ctx context.Context, method, path string, in any) error {
	res, err := c.send(ctx, method, path, nil, in)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return res.Body.Close()
}

// send is the single request path under do and doDiscard. It encodes query
// into the URL and, when in is non-nil, sends it as a JSON body; every request
// asks for JSON back. A non-2xx becomes a StatusError carrying up to
// maxErrorRead bytes of the body, with the rest drained so the connection can
// be reused. On a 2xx the caller owns (and must close) res.Body.
func (c *Client) send(ctx context.Context, method, path string, query map[string]string, in any) (*http.Response, error) {
	u := c.BaseURL + path
	if len(query) > 0 {
		vals := url.Values{}
		for k, v := range query {
			vals.Set(k, v)
		}
		u += "?" + vals.Encode()
	}

	var reqBody io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}
	// NewRequestWithContext derives GetBody from the *bytes.Reader, so the auth
	// Transport's 401-retry can rewind the body — including on DELETE /flowsheet,
	// which unusually reads its entry_id from a DELETE body.
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorRead))
		_, _ = io.Copy(io.Discard, res.Body)
		return nil, &StatusError{Code: res.StatusCode, Path: path, Body: string(b)}
	}
	return res, nil
}

// writeJSON is the body of a typed write method: it sends in to path and
// decodes the echoed entity into a T, returning it alongside the raw bytes for
// --json. A decode failure names path, like getInto's.
func writeJSON[T any](ctx context.Context, c *Client, method, path string, in any) (T, []byte, error) {
	var v T
	raw, err := c.do(ctx, method, path, nil, in)
	if err != nil {
		return v, nil, err
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, raw, fmt.Errorf("%s: decoding response: %w", path, err)
	}
	return v, raw, nil
}
