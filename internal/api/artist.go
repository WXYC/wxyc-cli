package api

import (
	"context"
	"fmt"
	"strconv"
)

// ArtistCard is GET /library/artists/{id} (BS#2156): the artist-card field
// set (keyed artist_id, not id) plus the BS#2597 dependent counts. The card is
// for one genre membership: genre_id/code_artist_number are genre-scoped, and
// unscoped the server answers the artist's lowest genre_id (BS#2637). The
// counts are artist-wide either way.
type ArtistCard struct {
	ArtistID                   int     `json:"artist_id"`
	ArtistName                 string  `json:"artist_name"`
	AlphabeticalName           string  `json:"alphabetical_name"`
	GenreID                    int     `json:"genre_id"`
	CodeLetters                string  `json:"code_letters"`
	CodeArtistNumber           int     `json:"code_artist_number"`
	CodeCompLetter             *string `json:"code_comp_letter"`
	ReleaseCount               int     `json:"release_count"`
	CrossReferenceSourceCount  int     `json:"cross_reference_source_count"`
	CrossReferenceTargetCount  int     `json:"cross_reference_target_count"`
	LibraryCrossReferenceCount int     `json:"library_cross_reference_count"`
	CompilationCreditCount     int     `json:"compilation_credit_count"`
}

// ArtistRelease is one row of GET /library/artists/{id}/releases
// (artistReleasesQuery): a release with its full call number. Rows carry
// genre_id but no genre name.
type ArtistRelease struct {
	ID                  int     `json:"id"`
	LastModified        string  `json:"last_modified"`
	Format              string  `json:"format_name"`
	GenreID             int     `json:"genre_id"`
	CodeLetters         string  `json:"code_letters"`
	CodeArtistNumber    int     `json:"code_artist_number"`
	CodeCompLetter      *string `json:"code_comp_letter"`
	CodeNumber          int     `json:"code_number"`
	CodeVolumeLetters   *string `json:"code_volume_letters"`
	AlbumTitle          string  `json:"album_title"`
	AlternateArtistName *string `json:"alternate_artist_name"`
}

// ArtistReleasesPage is the GET /library/artists/{id}/releases envelope. Page
// is zero-based.
type ArtistReleasesPage struct {
	ArtistID   int             `json:"artist_id"`
	Releases   []ArtistRelease `json:"releases"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	TotalPages int             `json:"totalPages"`
}

// ArtistCard returns an artist's card. genreID > 0 scopes it to that genre
// membership; 0 sends no genre_id. An unknown artist, or a genre the artist
// is not filed under, is a 404.
func (c *Client) ArtistCard(ctx context.Context, artistID, genreID int) (ArtistCard, []byte, error) {
	var q map[string]string
	if genreID > 0 {
		q = map[string]string{"genre_id": strconv.Itoa(genreID)}
	}
	var a ArtistCard
	raw, err := c.getInto(ctx, fmt.Sprintf("/library/artists/%d", artistID), q, &a)
	return a, raw, err
}

// ArtistReleases returns one page of an artist's releases in shelf order.
// Params mirror the server's: page (zero-based), limit, genre_id.
func (c *Client) ArtistReleases(ctx context.Context, artistID int, params map[string]string) (ArtistReleasesPage, []byte, error) {
	var p ArtistReleasesPage
	raw, err := c.getInto(ctx, fmt.Sprintf("/library/artists/%d/releases", artistID), params, &p)
	return p, raw, err
}
