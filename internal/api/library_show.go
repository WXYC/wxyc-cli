package api

import (
	"context"
	"fmt"
	"strconv"
)

// AlbumDetail is GET /library/info's full album record
// (libraryService.getAlbumFromDB). It extends the search-row Album with the
// detail-only fields the CLI renders; everything else (reconciled_identity,
// artist/genre/format ids, …) stays in the raw bytes.
type AlbumDetail struct {
	Album
	LegacyReleaseID     *int     `json:"legacy_release_id"`
	AlbumArtist         *string  `json:"album_artist"`
	AlternateArtistName *string  `json:"alternate_artist_name"`
	DiscQuantity        *int     `json:"disc_quantity"`
	OnStreaming         *bool    `json:"on_streaming"`
	AddDate             string   `json:"add_date"`
	LastModified        string   `json:"last_modified"`
	DateLost            *string  `json:"date_lost"`
	DateFound           *string  `json:"date_found"`
	URLs                []string `json:"urls"`
	// The discogs-unavailable trio is camelCase on the wire (BS#1895).
	DiscogsUnavailable     bool    `json:"discogsUnavailable"`
	DiscogsUnavailableNote *string `json:"discogsUnavailableNote"`
}

// PlayCounts is GET /library/{id}/flowsheet-play-counts (BS#2592): the
// release's flowsheet plays by link arm. The arms are disjoint views the
// server says must never be summed.
type PlayCounts struct {
	Direct         int `json:"direct"`
	RotationLinked int `json:"rotation_linked"`
	LegacyLinked   int `json:"legacy_linked"`
}

// AlbumInfo returns one album's full catalog record. A missing album is a 404.
func (c *Client) AlbumInfo(ctx context.Context, albumID int) (AlbumDetail, []byte, error) {
	var a AlbumDetail
	raw, err := c.getInto(ctx, "/library/info", map[string]string{"album_id": strconv.Itoa(albumID)}, &a)
	return a, raw, err
}

// FlowsheetPlayCounts returns the album's flowsheet plays by arm. A missing
// album is a 404.
func (c *Client) FlowsheetPlayCounts(ctx context.Context, albumID int) (PlayCounts, []byte, error) {
	var p PlayCounts
	raw, err := c.getInto(ctx, fmt.Sprintf("/library/%d/flowsheet-play-counts", albumID), nil, &p)
	return p, raw, err
}
