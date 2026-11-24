package api

import (
	"context"
	"strconv"
)

// Each read method returns the decoded slice (for the table projection) and
// the server's raw JSON bytes (for --json passthrough).

// Genres lists the genre catalog.
func (c *Client) Genres(ctx context.Context) ([]Genre, []byte, error) {
	var g []Genre
	raw, err := c.getInto(ctx, "/library/genres", nil, &g)
	return g, raw, err
}

// Formats lists the media formats.
func (c *Client) Formats(ctx context.Context) ([]Format, []byte, error) {
	var f []Format
	raw, err := c.getInto(ctx, "/library/formats", nil, &f)
	return f, raw, err
}

// Labels lists all record labels.
func (c *Client) Labels(ctx context.Context) ([]Label, []byte, error) {
	var l []Label
	raw, err := c.getInto(ctx, "/labels", nil, &l)
	return l, raw, err
}

// LabelSearch finds labels matching q.
func (c *Client) LabelSearch(ctx context.Context, q string, limit int) ([]Label, []byte, error) {
	params := map[string]string{"q": q}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}
	var l []Label
	raw, err := c.getInto(ctx, "/labels/search", params, &l)
	return l, raw, err
}

// Schedule returns the recurring show schedule.
func (c *Client) Schedule(ctx context.Context) ([]ScheduleEntry, []byte, error) {
	var s []ScheduleEntry
	raw, err := c.getInto(ctx, "/schedule", nil, &s)
	return s, raw, err
}

// Playlists returns the past shows credited to the given DJ, newest data as the
// server orders it, each with a short preview of its opening tracks. dj_id is
// required by the backend (400 otherwise).
func (c *Client) Playlists(ctx context.Context, djID string) ([]Playlist, []byte, error) {
	var p []Playlist
	raw, err := c.getInto(ctx, "/djs/playlists", map[string]string{"dj_id": djID}, &p)
	return p, raw, err
}

// Show returns one show's metadata and every flowsheet entry in it, in play
// order. The endpoint is public; a missing show is a 404.
func (c *Client) Show(ctx context.Context, showID int) (Show, []byte, error) {
	var s Show
	raw, err := c.getInto(ctx, "/flowsheet/playlist", map[string]string{"show_id": strconv.Itoa(showID)}, &s)
	return s, raw, err
}

// Artist returns one catalog artist's card. genreID selects the membership
// when the artist is filed under more than one genre; nil asks for the lowest.
func (c *Client) Artist(ctx context.Context, artistID int, genreID *int) (ArtistCard, []byte, error) {
	query := map[string]string{}
	if genreID != nil {
		query["genre_id"] = strconv.Itoa(*genreID)
	}
	var card ArtistCard
	raw, err := c.getInto(ctx, "/library/artists/"+strconv.Itoa(artistID), query, &card)
	return card, raw, err
}

// ArtistReleases returns one page of an artist's releases in shelf order.
func (c *Client) ArtistReleases(ctx context.Context, artistID int, genreID *int, page, limit int) (ArtistReleasesResponse, []byte, error) {
	query := map[string]string{"page": strconv.Itoa(page), "limit": strconv.Itoa(limit)}
	if genreID != nil {
		query["genre_id"] = strconv.Itoa(*genreID)
	}
	var resp ArtistReleasesResponse
	raw, err := c.getInto(ctx, "/library/artists/"+strconv.Itoa(artistID)+"/releases", query, &resp)
	return resp, raw, err
}

// Rotation returns the current rotation as raw JSON. The shape is deep
// (nested reconciled_identity, COALESCE fallbacks), so there is no decoded
// projection at the client layer; the command decodes generically.
func (c *Client) Rotation(ctx context.Context) ([]byte, error) {
	return c.getRaw(ctx, "/library/rotation", nil)
}

// SpecialtyShows lists the specialty-show catalog (public, no auth), in
// catalog order.
func (c *Client) SpecialtyShows(ctx context.Context) ([]SpecialtyShow, []byte, error) {
	var s []SpecialtyShow
	raw, err := c.getInto(ctx, "/specialty-shows", nil, &s)
	return s, raw, err
}
