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

// maxErrorBody caps how much of a non-2xx response body a StatusError keeps,
// so a proxy's HTML error page isn't dumped whole to stderr.
const maxErrorBody = 4096

// StatusError is returned for any non-2xx response. Code is the HTTP status;
// the command layer maps it to an exit code (401→auth, 403→forbidden, …).
type StatusError struct {
	Code int
	Path string
	Body string
}

func (e *StatusError) Error() string {
	msg := strings.TrimSpace(e.Body)
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
// deleteRaw which sends a JSON body.
func (c *Client) deleteQueryRaw(ctx context.Context, path string, query map[string]string) ([]byte, error) {
	return c.do(ctx, http.MethodDelete, path, query, nil)
}

// postRaw sends a JSON body to path and returns the response body verbatim,
// for mutating endpoints whose echoed entity the CLI both decodes (for a table
// confirmation) and passes through to --json.
func (c *Client) postRaw(ctx context.Context, path string, in any) ([]byte, error) {
	return c.do(ctx, http.MethodPost, path, nil, in)
}

// patchRaw sends a JSON body to path via PATCH and returns the response body
// verbatim, mirroring postRaw. Used by the mutating flowsheet endpoints that
// update or reorder an entry and echo the projected row.
func (c *Client) patchRaw(ctx context.Context, path string, in any) ([]byte, error) {
	return c.do(ctx, http.MethodPatch, path, nil, in)
}

// deleteRaw sends a JSON body to path via DELETE and returns the response body
// verbatim. DELETE-with-body is unusual but the backend reads req.body.entry_id;
// http.NewRequestWithContext derives GetBody from the *bytes.Reader, so the auth
// Transport's 401-retry rewind still works.
func (c *Client) deleteRaw(ctx context.Context, path string, in any) ([]byte, error) {
	return c.do(ctx, http.MethodDelete, path, nil, in)
}

// do is the single request path behind every method above. It encodes query
// into the URL and, when in is non-nil, sends it as a JSON body; every request
// asks for JSON back. The response body is returned verbatim. A non-2xx
// becomes a StatusError carrying at most maxErrorBody bytes of the body; on a
// 2xx, a failed read (e.g. the connection dropped mid-body) is an error rather
// than a truncated success, as is a body that isn't JSON (e.g. a proxy's HTML
// error page), named by path. An empty body (204 No Content) is returned as is.
func (c *Client) do(ctx context.Context, method, path string, query map[string]string, in any) ([]byte, error) {
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
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorBody))
		return nil, &StatusError{Code: res.StatusCode, Path: path, Body: string(b)}
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: reading response: %w", path, err)
	}
	if len(bytes.TrimSpace(body)) > 0 && !json.Valid(body) {
		return nil, fmt.Errorf("%s: response is not valid JSON", path)
	}
	return body, nil
}

// decodeJSON is the tail of a typed write method: it passes an error through,
// otherwise decodes raw into a T and returns it alongside raw for --json.
func decodeJSON[T any](raw []byte, err error) (T, []byte, error) {
	var v T
	if err != nil {
		return v, nil, err
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, raw, fmt.Errorf("decoding response: %w", err)
	}
	return v, raw, nil
}
