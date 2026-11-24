package api

import (
	"context"
	"errors"
	"strconv"
)

// LibrarySearch queries the catalog. Accepted params mirror the backend, e.g.
// artist_name, album_title, on_streaming ("true"/"false"), n (page size). Returns the decoded albums and the
// raw JSON for --json passthrough.
func (c *Client) LibrarySearch(ctx context.Context, params map[string]string) ([]Album, []byte, error) {
	var albums []Album
	raw, err := c.getInto(ctx, "/library/", params, &albums)
	return albums, raw, err
}

// Flowsheet returns the most recent on-air log entries, newest last. The raw
// return is the full server envelope ({"entries":[...]}).
func (c *Client) Flowsheet(ctx context.Context, limit int) ([]FlowsheetEntry, []byte, error) {
	q := map[string]string{}
	if limit > 0 {
		q["limit"] = strconv.Itoa(limit)
	}
	var resp flowsheetResponse
	raw, err := c.getInto(ctx, "/flowsheet", q, &resp)
	return resp.Entries, raw, err
}

// ErrNoOpenShow is GET /flowsheet's on_air: null — automation, no show open.
var ErrNoOpenShow = errors.New("no show is on the air")

// ErrOnAirUnknown is an absent on_air: the server could not resolve it.
var ErrOnAirUnknown = errors.New("the server could not say which show is on the air")

// FlowsheetOnAir reads the on_air banner from GET /flowsheet, returning it
// decoded and as the server's raw on_air object for --json passthrough.
func (c *Client) FlowsheetOnAir(ctx context.Context) (OnAir, []byte, error) {
	p, _, err := c.FlowsheetPage(ctx, 1)
	if err != nil {
		return OnAir{}, nil, err
	}
	o, err := p.OnAir()
	if err != nil {
		return OnAir{}, nil, err
	}
	return o, p.OnAirRaw, nil
}
