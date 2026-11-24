package api

import "context"

// SearchResult is one track row from GET /flowsheet/search. Only the fields
// the table renders are modeled; record_label, show_id, rotation_bin,
// request_flag and on_streaming pass through --json.
type SearchResult struct {
	ID         int    `json:"id"`
	PlayDate   string `json:"play_date"`
	ArtistName string `json:"artist_name"`
	TrackTitle string `json:"track_title"`
	AlbumTitle string `json:"album_title"`
	DJName     string `json:"dj_name"`
}

// SearchResponse is the GET /flowsheet/search envelope. NextCursor is set only
// when the server produced one (date sort and a full page).
type SearchResponse struct {
	Results    []SearchResult `json:"results"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	TotalPages int            `json:"totalPages"`
	NextCursor string         `json:"nextCursor,omitempty"`
}

// FlowsheetSearch searches the public playlist archive. Params mirror the
// server's: q, page, limit, sort, order, cursor. Returns the decoded envelope
// and the raw JSON for --json passthrough.
func (c *Client) FlowsheetSearch(ctx context.Context, params map[string]string) (SearchResponse, []byte, error) {
	var resp SearchResponse
	raw, err := c.getInto(ctx, "/flowsheet/search", params, &resp)
	return resp, raw, err
}
