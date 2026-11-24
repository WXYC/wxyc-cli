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

// FlowsheetRange reads the public flowsheet in the half-open window
// [startMs, endMs). The server caps the window at eight days.
func (c *Client) FlowsheetRange(ctx context.Context, startMs, endMs int64) (FlowsheetRangeResponse, []byte, error) {
	var resp FlowsheetRangeResponse
	raw, err := c.getInto(ctx, "/flowsheet/range", map[string]string{
		"start": strconv.FormatInt(startMs, 10),
		"end":   strconv.FormatInt(endMs, 10),
	}, &resp)
	return resp, raw, err
}
