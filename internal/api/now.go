package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// FlowsheetPageResult is GET /flowsheet's envelope: the newest entries plus
// the on_air banner, kept raw so its three states stay distinct (see OnAir).
type FlowsheetPageResult struct {
	Entries  []FlowsheetEntry `json:"entries"`
	OnAirRaw json.RawMessage  `json:"on_air"`
}

// OnAir decodes the on_air banner. It returns ErrNoOpenShow for JSON null
// (automation) and ErrOnAirUnknown when the field is absent.
func (p FlowsheetPageResult) OnAir() (OnAir, error) {
	switch {
	case len(p.OnAirRaw) == 0:
		return OnAir{}, ErrOnAirUnknown
	case string(p.OnAirRaw) == "null":
		return OnAir{}, ErrNoOpenShow
	}
	var o OnAir
	if err := json.Unmarshal(p.OnAirRaw, &o); err != nil {
		return OnAir{}, fmt.Errorf("decoding on_air: %w", err)
	}
	return o, nil
}

// FlowsheetPage reads GET /flowsheet whole: the newest limit entries and the
// on_air banner, plus the raw body for --json passthrough.
func (c *Client) FlowsheetPage(ctx context.Context, limit int) (FlowsheetPageResult, []byte, error) {
	q := map[string]string{}
	if limit > 0 {
		q["limit"] = strconv.Itoa(limit)
	}
	var p FlowsheetPageResult
	raw, err := c.getInto(ctx, "/flowsheet", q, &p)
	return p, raw, err
}

// DJsOnAir lists the on-air handles of the DJs on the current show; it is
// empty while the station is on automation.
func (c *Client) DJsOnAir(ctx context.Context) ([]OnAirDJ, []byte, error) {
	var djs []OnAirDJ
	raw, err := c.getInto(ctx, "/flowsheet/djs-on-air", nil, &djs)
	return djs, raw, err
}
