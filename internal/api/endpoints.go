package api

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
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

// NowPlaying composes the on-air answer: GET /flowsheet for the on_air object
// and the recent entries (to find the last track), plus GET
// /flowsheet/djs-on-air for the handles. on_air is an object while live, JSON
// null on automation and absent when the server does not report it; the three
// become OnAirLive, OnAirAuto and OnAirUnknown. Raw is a two-key object with
// both response bodies verbatim, for --json.
func (c *Client) NowPlaying(ctx context.Context) (NowPlaying, error) {
	fsRaw, err := c.getRaw(ctx, "/flowsheet", map[string]string{"limit": "20"})
	if err != nil {
		return NowPlaying{}, err
	}
	var env struct {
		Entries []FlowsheetEntry `json:"entries"`
		OnAir   json.RawMessage  `json:"on_air"`
	}
	if err := json.Unmarshal(fsRaw, &env); err != nil {
		return NowPlaying{}, err
	}

	djsRaw, err := c.getRaw(ctx, "/flowsheet/djs-on-air", nil)
	if err != nil {
		return NowPlaying{}, err
	}
	var djs []OnAirDJ
	if err := json.Unmarshal(djsRaw, &djs); err != nil {
		return NowPlaying{}, err
	}

	now := NowPlaying{DJs: djs, State: OnAirUnknown}
	switch {
	case len(env.OnAir) == 0:
		now.State = OnAirUnknown
	case strings.TrimSpace(string(env.OnAir)) == "null":
		now.State = OnAirAuto
	default:
		var info OnAirInfo
		if err := json.Unmarshal(env.OnAir, &info); err != nil {
			return NowPlaying{}, err
		}
		now.OnAir = &info
		now.State = OnAirLive
	}

	for i := len(env.Entries) - 1; i >= 0; i-- {
		if env.Entries[i].EntryType == "track" {
			track := env.Entries[i]
			now.LastTrack = &track
			break
		}
	}

	now.Raw, err = json.Marshal(map[string]json.RawMessage{
		"flowsheet":  fsRaw,
		"djs_on_air": djsRaw,
	})
	if err != nil {
		return NowPlaying{}, err
	}
	return now, nil
}
