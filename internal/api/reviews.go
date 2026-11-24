package api

import (
	"context"
	"fmt"
)

// ReviewText is the slip text a Review and a ReviewRevision share
// (ReviewFields on the server). An empty field is null, never "".
type ReviewText struct {
	Review            *string `json:"review"`
	ArtistBlurb       *string `json:"artist_blurb"`
	Buzzwords         *string `json:"buzzwords"`
	RecommendedTracks *string `json:"recommended_tracks"`
	// FCC is station-only.
	FCC *string `json:"fcc"`
}

// Review is one in-app review (GET /reviews, GET /reviews/{id}; ADR 0006).
// Author can hold a legal name: shown to the user only, never put in an
// error or log. Publishing consent and the user ids stay in the raw bytes.
type Review struct {
	ReviewText
	ID            int     `json:"id"`
	AlbumID       *int    `json:"album_id"`
	IntakeItemID  *int    `json:"intake_item_id"`
	Author        *string `json:"author"`
	Medium        string  `json:"medium"`
	Status        string  `json:"status"`
	AddDate       string  `json:"add_date"`
	SubmittedAt   *string `json:"submitted_at"`
	LastModified  string  `json:"last_modified"`
	InUse         bool    `json:"in_use"`
	OnCover       bool    `json:"on_cover"`
	PrintedAt     *string `json:"printed_at"`
	RevisionCount int     `json:"revision_count"`
}

// ReviewRevision is one saved version of a submitted review (GET
// /reviews/{id}/revisions). EditedBy can hold a legal name, like Author.
type ReviewRevision struct {
	ReviewText
	ID       int     `json:"id"`
	ReviewID int     `json:"review_id"`
	Revision int     `json:"revision"`
	EditedBy *string `json:"edited_by"`
	EditedAt string  `json:"edited_at"`
}

// AlbumReview is one archived Google Form submission (GET /album-reviews;
// ADR 0011). Reviewer can hold a legal name.
type AlbumReview struct {
	ID                int     `json:"id"`
	AlbumID           *int    `json:"album_id"`
	ArtistName        *string `json:"artist_name"`
	AlbumTitle        *string `json:"album_title"`
	RecordLabel       *string `json:"record_label"`
	ArtistBlurb       *string `json:"artist_blurb"`
	Review            *string `json:"review"`
	RecommendedTracks *string `json:"recommended_tracks"`
	Buzzwords         *string `json:"buzzwords"`
	FCCViolations     *string `json:"fcc_violations"`
	ReviewPurpose     *string `json:"review_purpose"`
	Rotated           *bool   `json:"rotated"`
	SubmittedAt       *string `json:"submitted_at"`
	Reviewer          *string `json:"reviewer"`
}

// AlbumReviewsPage is GET /album-reviews (AlbumReviewsResponse). Total and
// HasMore are optional in the contract.
type AlbumReviewsPage struct {
	AlbumReviews []AlbumReview `json:"album_reviews"`
	Pagination   struct {
		Page    int   `json:"page"`
		Limit   int   `json:"limit"`
		Total   *int  `json:"total"`
		HasMore *bool `json:"hasMore"`
	} `json:"pagination"`
}

// Reviews lists the in-app reviews the caller may see, filtered by query
// (album_id, intake_item_id, mine). Unpaginated.
func (c *Client) Reviews(ctx context.Context, query map[string]string) ([]Review, []byte, error) {
	var rs []Review
	raw, err := c.getInto(ctx, "/reviews", query, &rs)
	return rs, raw, err
}

// Review reads one in-app review. A draft the caller may not see is a 404.
func (c *Client) Review(ctx context.Context, id int) (Review, []byte, error) {
	var r Review
	raw, err := c.getInto(ctx, fmt.Sprintf("/reviews/%d", id), nil, &r)
	return r, raw, err
}

// ReviewRevisions returns a review's edit history, newest first: empty for a
// draft, and a 404 for a draft the caller may not see.
func (c *Client) ReviewRevisions(ctx context.Context, id int) ([]ReviewRevision, []byte, error) {
	var revs []ReviewRevision
	raw, err := c.getInto(ctx, fmt.Sprintf("/reviews/%d/revisions", id), nil, &revs)
	return revs, raw, err
}

// AlbumReviews lists the form archive, newest first, filtered and paged by
// query (album_id, artist, page, limit).
func (c *Client) AlbumReviews(ctx context.Context, query map[string]string) (AlbumReviewsPage, []byte, error) {
	var p AlbumReviewsPage
	raw, err := c.getInto(ctx, "/album-reviews", query, &p)
	return p, raw, err
}
