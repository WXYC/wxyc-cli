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

// RecentShows lists shows that started in the last windowHours hours, newest
// first. The server accepts only 1-168 and rejects anything else with a 400.
func (c *Client) RecentShows(ctx context.Context, windowHours int) ([]RecentShow, []byte, error) {
	var resp recentShowsResponse
	raw, err := c.getInto(ctx, "/flowsheet/shows/recent",
		map[string]string{"window_hours": strconv.Itoa(windowHours)}, &resp)
	return resp.Shows, raw, err
}
