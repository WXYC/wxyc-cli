package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// ErrNotInBin is returned by BinRemove when the album wasn't in the DJ's bin.
// The server signals this with a 200 and an empty body (its delete's
// .returning() yields no row), so the client turns it into an error rather
// than reporting a removal that didn't happen.
var ErrNotInBin = errors.New("album is not in your bin")

// Bin returns the albums in the authenticated DJ's bin, plus the raw JSON for
// --json passthrough.
func (c *Client) Bin(ctx context.Context) ([]BinItem, []byte, error) {
	var items []BinItem
	raw, err := c.getInto(ctx, "/djs/bin", nil, &items)
	return items, raw, err
}

// BinAdd saves an album to the DJ's bin. Mutating: gated behind --write.
func (c *Client) BinAdd(ctx context.Context, albumID int) error {
	return c.post(ctx, "/djs/bin", map[string]int{"album_id": albumID}, nil)
}

// BinRemove removes an album from the DJ's bin and returns the deleted bins
// row plus the raw JSON. The server reads album_id from the query string, not
// the body. Mutating: gated behind --write.
func (c *Client) BinRemove(ctx context.Context, albumID int) (BinEntry, []byte, error) {
	var e BinEntry
	const path = "/djs/bin"
	raw, err := c.deleteQueryRaw(ctx, path, map[string]string{"album_id": strconv.Itoa(albumID)})
	if err != nil {
		return e, nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return e, raw, fmt.Errorf("album %d: %w", albumID, ErrNotInBin)
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return e, raw, fmt.Errorf("%s: decoding response: %w", path, err)
	}
	return e, raw, nil
}
