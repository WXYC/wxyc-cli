package api

import (
	"context"
	"strconv"
)

// FlowsheetRange is the GET /flowsheet/range response: every entry logged in
// a half-open time window, ordered by add_time, and every show those entries
// (or the window) touch. Entries segment into shows on show_id.
type FlowsheetRange struct {
	Shows   []RangeShow      `json:"shows"`
	Entries []FlowsheetEntry `json:"entries"`
}

// RangeShow is one show overlapping a range window (the server's public-safe
// FlowsheetRangeShow projection). specialty_id passes through --json.
type RangeShow struct {
	ID        int           `json:"id"`
	ShowName  *string       `json:"show_name"`
	DJName    *string       `json:"dj_name"`
	Specialty *SpecialtyRef `json:"specialty"` // nil for a regular show
	StartTime string        `json:"start_time"`
	EndTime   *string       `json:"end_time"` // nil while open (or never closed)
}

// FlowsheetRange reads the entries and shows in [start, end), both in epoch
// milliseconds. The endpoint is public; the server 400s unless end > start
// and the window is at most 8 days.
func (c *Client) FlowsheetRange(ctx context.Context, start, end int64) (FlowsheetRange, []byte, error) {
	var r FlowsheetRange
	raw, err := c.getInto(ctx, "/flowsheet/range", map[string]string{
		"start": strconv.FormatInt(start, 10),
		"end":   strconv.FormatInt(end, 10),
	}, &r)
	return r, raw, err
}
