package api

import (
	"context"
	"strconv"
)

// Each write method returns the decoded echo (for the table confirmation) and
// the server's raw JSON bytes (for --json passthrough), mirroring the read
// methods so the two views can't drift.

// FlowsheetStart starts the caller's show, or — if a show is already active —
// adds the caller as a co-host. Mutating: gated behind --write.
func (c *Client) FlowsheetStart(ctx context.Context, req StartShowRequest) (ShowSession, []byte, error) {
	return decodeJSON[ShowSession](c.postRaw(ctx, "/flowsheet/join", req))
}

// FlowsheetEnd ends the caller's show, or — if the caller is a co-host rather
// than the primary DJ — removes them from the show. Mutating: gated behind
// --write. djID must match the authenticated user.
func (c *Client) FlowsheetEnd(ctx context.Context, djID string) (ShowSession, []byte, error) {
	return decodeJSON[ShowSession](c.postRaw(ctx, "/flowsheet/end", map[string]string{"dj_id": djID}))
}

// FlowsheetAddTrack appends a played track to the active show's flowsheet.
// Mutating: gated behind --write. Requires an active show (400 otherwise).
func (c *Client) FlowsheetAddTrack(ctx context.Context, t FlowsheetTrack) (FlowsheetResult, []byte, error) {
	return decodeJSON[FlowsheetResult](c.postRaw(ctx, "/flowsheet", t))
}

// FlowsheetAddMarker appends a non-track entry (talkset, breakpoint, message)
// to the active show's flowsheet. Mutating: gated behind --write.
func (c *Client) FlowsheetAddMarker(ctx context.Context, message, entryType string) (FlowsheetResult, []byte, error) {
	return decodeJSON[FlowsheetResult](c.postRaw(ctx, "/flowsheet", flowsheetMarker{Message: message, EntryType: entryType}))
}

// FlowsheetAddSpotlight records who is on air in the open show (role is one of
// performer, dj, reader, guest, host). Mutating: gated behind --write. The
// server 403s with code spotlight_disabled while the feature flag is off.
func (c *Client) FlowsheetAddSpotlight(ctx context.Context, role, name string) (FlowsheetResult, []byte, error) {
	req := flowsheetSpotlight{EntryType: "spotlight", SpotlightRole: role, SpotlightName: name}
	return decodeJSON[FlowsheetResult](c.postRaw(ctx, "/flowsheet", req))
}

// FlowsheetMove reorders an entry to a new 1-based position. Mutating: gated
// behind --write. 404s if the target row is gone.
func (c *Client) FlowsheetMove(ctx context.Context, entryID, newPosition int) (FlowsheetResult, []byte, error) {
	req := flowsheetMoveRequest{EntryID: entryID, NewPosition: newPosition}
	return decodeJSON[FlowsheetResult](c.patchRaw(ctx, "/flowsheet/play-order", req))
}

// FlowsheetUpdate edits an entry's allowlisted fields. Mutating: gated behind
// --write. A fully-empty data payload is a 400; 404s if the row is gone.
func (c *Client) FlowsheetUpdate(ctx context.Context, entryID int, data FlowsheetUpdateFields) (FlowsheetResult, []byte, error) {
	req := flowsheetUpdateRequest{EntryID: entryID, Data: data}
	return decodeJSON[FlowsheetResult](c.patchRaw(ctx, "/flowsheet", req))
}

// FlowsheetDelete removes an entry, echoing the removed (projected) row.
// Mutating: gated behind --write. 404s on a double-delete.
func (c *Client) FlowsheetDelete(ctx context.Context, entryID int) (FlowsheetResult, []byte, error) {
	return decodeJSON[FlowsheetResult](c.deleteRaw(ctx, "/flowsheet", flowsheetDeleteRequest{EntryID: entryID}))
}

// ShowSetRequestMode sets the show's request-line mode (requests, feedback or
// closed); nil sends request_mode: null, returning the show to its program's
// default. Mutating: gated behind --write. The server applies mayEditShow: a
// non-member of the open show gets a 400, and an ended show a 403 closed_show.
func (c *Client) ShowSetRequestMode(ctx context.Context, showID int, mode *string) ([]byte, error) {
	return c.patchRaw(ctx, "/flowsheet/shows/"+strconv.Itoa(showID), showRequestModePatch{RequestMode: mode})
}
