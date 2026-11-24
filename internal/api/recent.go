package api

import (
	"context"
	"strconv"
)

// FlowsheetRecent is the GET /flowsheet/shows/recent response: shows that
// started in the lookback window, newest first (capped at 200 by the server).
type FlowsheetRecent struct {
	Shows []RecentShow `json:"shows"`
}

// RecentShow is one show in the handoff read, with the on-air handles of the
// DJs who had the room. DJs is empty when the server could resolve none.
type RecentShow struct {
	ID        int       `json:"id"`
	ShowName  *string   `json:"show_name"`
	StartTime string    `json:"start_time"`
	EndTime   *string   `json:"end_time"` // nil while on air, or if its sign-off was lost
	DJs       []OnAirDJ `json:"djs"`
}

// OnAirDJ is a DJ's on-air handle (the server's OnAirDJ schema). ID is nil
// for a legacy DJ with no account; DJName is nil when the handle was
// filtered away.
type OnAirDJ struct {
	ID     *string `json:"id"`
	DJName *string `json:"dj_name"`
}

// FlowsheetRecent reads the shows that started in the last hours hours. The
// server 400s (rather than clamping) unless hours is 1-168; it requires the
// flowsheet: read permission.
func (c *Client) FlowsheetRecent(ctx context.Context, hours int) (FlowsheetRecent, []byte, error) {
	var r FlowsheetRecent
	raw, err := c.getInto(ctx, "/flowsheet/shows/recent", map[string]string{
		"window_hours": strconv.Itoa(hours),
	}, &r)
	return r, raw, err
}
