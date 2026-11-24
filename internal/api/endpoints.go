package api

import (
	"context"
	"strconv"
)

// LibrarySearch queries the catalog. Accepted params mirror the backend, e.g.
// artist_name, album_title, n (page size). Returns the decoded albums and the
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

// FlowsheetSearch queries the public playlist archive. Params mirror the
// server: q, limit (1-100), page, sort (date|artist|song|dj), order
// (asc|desc) and cursor. A malformed cursor is a server 400.
func (c *Client) FlowsheetSearch(ctx context.Context, params map[string]string) (FlowsheetSearchResponse, []byte, error) {
	var resp FlowsheetSearchResponse
	raw, err := c.getInto(ctx, "/flowsheet/search", params, &resp)
	return resp, raw, err
}
