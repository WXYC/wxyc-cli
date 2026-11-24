package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// Each write method returns the decoded echo (for the table confirmation) and
// the server's raw JSON bytes (for --json passthrough), mirroring the read
// methods so the two views can't drift.

// FlowsheetStart starts the caller's show, or — if a show is already active —
// adds the caller as a co-host. Mutating: gated behind --write.
func (c *Client) FlowsheetStart(ctx context.Context, req StartShowRequest) (ShowSession, []byte, error) {
	return decodeShowSession(c.postRaw(ctx, "/flowsheet/join", req))
}

// FlowsheetEnd ends the caller's show, or — if the caller is a co-host rather
// than the primary DJ — removes them from the show. Mutating: gated behind
// --write. djID must match the authenticated user.
func (c *Client) FlowsheetEnd(ctx context.Context, djID string) (ShowSession, []byte, error) {
	return decodeShowSession(c.postRaw(ctx, "/flowsheet/end", map[string]string{"dj_id": djID}))
}

// FlowsheetAddTrack appends a played track to the active show's flowsheet.
// Mutating: gated behind --write. Requires an active show (400 otherwise).
func (c *Client) FlowsheetAddTrack(ctx context.Context, t FlowsheetTrack) (FlowsheetResult, []byte, error) {
	return decodeFlowsheetResult(c.postRaw(ctx, "/flowsheet", t))
}

// FlowsheetAddMarker appends a non-track entry (talkset, breakpoint, message)
// to the active show's flowsheet. Mutating: gated behind --write.
func (c *Client) FlowsheetAddMarker(ctx context.Context, message, entryType string) (FlowsheetResult, []byte, error) {
	return decodeFlowsheetResult(c.postRaw(ctx, "/flowsheet", flowsheetMarker{Message: message, EntryType: entryType}))
}

// FlowsheetAddSpotlight records who is on air in the open show (role is one of
// performer, dj, reader, guest, host). Mutating: gated behind --write. The
// server 403s with code spotlight_disabled while the feature flag is off.
func (c *Client) FlowsheetAddSpotlight(ctx context.Context, role, name string) (FlowsheetResult, []byte, error) {
	req := flowsheetSpotlight{EntryType: "spotlight", SpotlightRole: role, SpotlightName: name}
	return decodeFlowsheetResult(c.postRaw(ctx, "/flowsheet", req))
}

// FlowsheetMove reorders an entry to a new 1-based position. Mutating: gated
// behind --write. 404s if the target row is gone.
func (c *Client) FlowsheetMove(ctx context.Context, entryID, newPosition int) (FlowsheetResult, []byte, error) {
	req := flowsheetMoveRequest{EntryID: entryID, NewPosition: newPosition}
	return decodeFlowsheetResult(c.patchRaw(ctx, "/flowsheet/play-order", req))
}

// FlowsheetUpdate edits an entry's allowlisted fields. Mutating: gated behind
// --write. A fully-empty data payload is a 400; 404s if the row is gone.
func (c *Client) FlowsheetUpdate(ctx context.Context, entryID int, data FlowsheetUpdateFields) (FlowsheetResult, []byte, error) {
	req := flowsheetUpdateRequest{EntryID: entryID, Data: data}
	return decodeFlowsheetResult(c.patchRaw(ctx, "/flowsheet", req))
}

// FlowsheetDelete removes an entry, echoing the removed (projected) row.
// Mutating: gated behind --write. 404s on a double-delete.
func (c *Client) FlowsheetDelete(ctx context.Context, entryID int) (FlowsheetResult, []byte, error) {
	return decodeFlowsheetResult(c.deleteRaw(ctx, "/flowsheet", flowsheetDeleteRequest{EntryID: entryID}))
}

func decodeShowSession(raw []byte, err error) (ShowSession, []byte, error) {
	if err != nil {
		return ShowSession{}, nil, err
	}
	var s ShowSession
	if err := json.Unmarshal(raw, &s); err != nil {
		return ShowSession{}, raw, fmt.Errorf("decoding show response: %w", err)
	}
	return s, raw, nil
}

func decodeFlowsheetResult(raw []byte, err error) (FlowsheetResult, []byte, error) {
	if err != nil {
		return FlowsheetResult{}, nil, err
	}
	var e FlowsheetResult
	if err := json.Unmarshal(raw, &e); err != nil {
		return FlowsheetResult{}, raw, fmt.Errorf("decoding flowsheet response: %w", err)
	}
	return e, raw, nil
}
