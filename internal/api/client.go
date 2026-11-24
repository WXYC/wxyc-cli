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

// get decodes the response into out, discarding the raw bytes. Retained for
// callers that don't need --json passthrough.
func (c *Client) get(ctx context.Context, path string, query map[string]string, out any) error {
	_, err := c.getInto(ctx, path, query, out)
	return err
}

// getRaw is like get but returns the response body verbatim, for endpoints
// whose full shape the CLI passes through to --json without modeling it.
func (c *Client) getRaw(ctx context.Context, path string, query map[string]string) ([]byte, error) {
	u := c.BaseURL + path
	if len(query) > 0 {
		vals := url.Values{}
		for k, v := range query {
			vals.Set(k, v)
		}
		u += "?" + vals.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &StatusError{Code: res.StatusCode, Path: path, Body: string(body)}
	}
	return body, nil
}

// postRaw sends a JSON body to path and returns the response body verbatim,
// for mutating endpoints whose echoed entity the CLI both decodes (for a table
// confirmation) and passes through to --json. Non-2xx becomes a StatusError.
func (c *Client) postRaw(ctx context.Context, path string, in any) ([]byte, error) {
	return c.bodyRaw(ctx, http.MethodPost, path, in)
}

// patchRaw sends a JSON body to path via PATCH and returns the response body
// verbatim, mirroring postRaw. Used by the mutating flowsheet endpoints that
// update or reorder an entry and echo the projected row.
func (c *Client) patchRaw(ctx context.Context, path string, in any) ([]byte, error) {
	return c.bodyRaw(ctx, http.MethodPatch, path, in)
}

// deleteRaw sends a JSON body to path via DELETE and returns the response body
// verbatim. DELETE-with-body is unusual but the backend reads req.body.entry_id;
// http.NewRequestWithContext derives GetBody from the *bytes.Reader, so the auth
// Transport's 401-retry rewind still works.
func (c *Client) deleteRaw(ctx context.Context, path string, in any) ([]byte, error) {
	return c.bodyRaw(ctx, http.MethodDelete, path, in)
}

// deleteQuery sends DELETE with query parameters and no body, for endpoints
// that read their identifier from the query string (e.g. /djs/bin).
func (c *Client) deleteQuery(ctx context.Context, path string, query map[string]string) error {
	u := c.BaseURL + path
	if len(query) > 0 {
		vals := url.Values{}
		for k, v := range query {
			vals.Set(k, v)
		}
		u += "?" + vals.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	b, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &StatusError{Code: res.StatusCode, Path: path, Body: string(b)}
	}
	return nil
}

// bodyRaw is the shared implementation behind postRaw/patchRaw/deleteRaw: it
// marshals in as JSON, sends it with the given method, and returns the response
// body verbatim. Non-2xx becomes a StatusError.
func (c *Client) bodyRaw(ctx context.Context, method, path string, in any) ([]byte, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	b, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &StatusError{Code: res.StatusCode, Path: path, Body: string(b)}
	}
	return b, nil
}

// post sends a JSON body to path. out may be nil when the response is ignored.
func (c *Client) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return &StatusError{Code: res.StatusCode, Path: path, Body: string(b)}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("%s: decoding response: %w", path, err)
	}
	return nil
}
