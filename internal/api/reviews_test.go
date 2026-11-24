package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// reviewBody mirrors one Review from GET /reviews and GET /reviews/{id}
// (Backend-Service app.yaml: Review = ReviewFields + the review row).
const reviewBody = `{"id":812,"album_id":45029,"intake_item_id":3307,` +
	`"author":"Jordan Example","author_user_id":"u_8f2c","recorded_by_user_id":null,` +
	`"medium":"typed","status":"submitted","add_date":"2026-09-14",` +
	`"submitted_at":"2026-09-15T02:11:40.000Z","last_modified":"2026-09-20T17:05:12.000Z",` +
	`"in_use":true,"on_cover":true,"printed_revision_id":1204,"printed_at":"2026-09-16T14:00:00.000Z",` +
	`"revision_count":2,` +
	`"buzzwords":"motorik, lounge, Moog","artist_blurb":"Anglo-French group led by Tim Gane and Laetitia Sadier.",` +
	`"review":"Their most polished record.\nSide two drifts.","recommended_tracks":"1!, 3!!, 7",` +
	`"fcc":"Track 4 has one f-word at 2:10.",` +
	`"publish_website":false,"publish_apps":false,"publish_instagram":false,"credit":null}`

// revisionsBody mirrors GET /reviews/{id}/revisions: a bare ReviewRevision
// array, newest first.
const revisionsBody = `[` +
	`{"id":1205,"review_id":812,"revision":2,"edited_by":"Pat Director","edited_by_user_id":"u_11aa",` +
	`"edited_at":"2026-09-20T17:05:12.000Z","review":"Their most polished record.\nSide two drifts.",` +
	`"artist_blurb":null,"buzzwords":"motorik, lounge, Moog","recommended_tracks":"1!, 3!!, 7","fcc":null},` +
	`{"id":1204,"review_id":812,"revision":1,"edited_by":"Jordan Example","edited_by_user_id":"u_8f2c",` +
	`"edited_at":"2026-09-15T02:11:40.000Z","review":"Their most polished record.",` +
	`"artist_blurb":null,"buzzwords":null,"recommended_tracks":null,"fcc":null}]`

// albumReviewsBody mirrors GET /album-reviews (AlbumReviewsResponse).
const albumReviewsBody = `{"album_reviews":[{"id":5521,"album_id":45029,"artist_name":"Stereolab",` +
	`"album_title":"Dots and Loops","record_label":"Duophonic","artist_blurb":null,` +
	`"review":"Great late-night record.","recommended_tracks":"Brakhage!","buzzwords":"lounge",` +
	`"fcc_violations":"None","review_purpose":"New addition","rotated":true,` +
	`"released_within_six_months":false,"social_consent":null,` +
	`"submitted_at":"2019-02-03T18:44:00.000Z","reviewer":"Sam Example 2/3/19"}],` +
	`"pagination":{"page":2,"limit":10,"total":31,"hasMore":true}}`

func TestReviews_DecodesAndSendsFilters(t *testing.T) {
	var q string
	srv := serve(t, "/reviews", "["+reviewBody+"]", &q)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	rs, raw, err := c.Reviews(context.Background(), map[string]string{"album_id": "45029", "mine": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if q != "album_id=45029&mine=true" {
		t.Errorf("query = %q, want album_id=45029&mine=true", q)
	}
	if string(raw) != "["+reviewBody+"]" {
		t.Errorf("raw = %s, want the body verbatim", raw)
	}
	if len(rs) != 1 {
		t.Fatalf("got %d reviews, want 1", len(rs))
	}
	assertReview(t, rs[0])
}

func assertReview(t *testing.T, r Review) {
	t.Helper()
	if r.ID != 812 || r.AlbumID == nil || *r.AlbumID != 45029 || r.IntakeItemID == nil || *r.IntakeItemID != 3307 {
		t.Errorf("ids = %d/%v/%v", r.ID, r.AlbumID, r.IntakeItemID)
	}
	if deref(r.Author) != "Jordan Example" || r.Medium != "typed" || r.Status != "submitted" {
		t.Errorf("author/medium/status = %v/%q/%q", r.Author, r.Medium, r.Status)
	}
	if r.AddDate != "2026-09-14" || deref(r.SubmittedAt) != "2026-09-15T02:11:40.000Z" || r.LastModified != "2026-09-20T17:05:12.000Z" {
		t.Errorf("dates = %q/%v/%q", r.AddDate, r.SubmittedAt, r.LastModified)
	}
	if !r.InUse || !r.OnCover || deref(r.PrintedAt) != "2026-09-16T14:00:00.000Z" || r.RevisionCount != 2 {
		t.Errorf("in_use/on_cover/printed_at/revision_count = %v/%v/%v/%d", r.InUse, r.OnCover, r.PrintedAt, r.RevisionCount)
	}
	if deref(r.Review) != "Their most polished record.\nSide two drifts." ||
		deref(r.ArtistBlurb) != "Anglo-French group led by Tim Gane and Laetitia Sadier." ||
		deref(r.Buzzwords) != "motorik, lounge, Moog" ||
		deref(r.RecommendedTracks) != "1!, 3!!, 7" ||
		deref(r.FCC) != "Track 4 has one f-word at 2:10." {
		t.Errorf("text fields = %+v", r)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestReview_ReadsOne(t *testing.T) {
	srv := serve(t, "/reviews/812", reviewBody, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	r, raw, err := c.Review(context.Background(), 812)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != reviewBody {
		t.Errorf("raw = %s, want the body verbatim", raw)
	}
	assertReview(t, r)
}

func TestReviewRevisions(t *testing.T) {
	srv := serve(t, "/reviews/812/revisions", revisionsBody, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	revs, raw, err := c.ReviewRevisions(context.Background(), 812)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != revisionsBody {
		t.Errorf("raw = %s, want the body verbatim", raw)
	}
	if len(revs) != 2 {
		t.Fatalf("got %d revisions, want 2", len(revs))
	}
	r := revs[0]
	if r.ID != 1205 || r.ReviewID != 812 || r.Revision != 2 || deref(r.EditedBy) != "Pat Director" ||
		r.EditedAt != "2026-09-20T17:05:12.000Z" || deref(r.Review) != "Their most polished record.\nSide two drifts." ||
		deref(r.Buzzwords) != "motorik, lounge, Moog" || deref(r.RecommendedTracks) != "1!, 3!!, 7" {
		t.Errorf("revision = %+v", r)
	}
}

func TestAlbumReviews(t *testing.T) {
	var q string
	srv := serve(t, "/album-reviews", albumReviewsBody, &q)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	page, raw, err := c.AlbumReviews(context.Background(), map[string]string{"artist": "The Stereolab", "page": "2", "limit": "10"})
	if err != nil {
		t.Fatal(err)
	}
	want := url.Values{"artist": {"The Stereolab"}, "page": {"2"}, "limit": {"10"}}.Encode()
	if q != want {
		t.Errorf("query = %q, want %q", q, want)
	}
	if string(raw) != albumReviewsBody {
		t.Errorf("raw = %s, want the body verbatim", raw)
	}
	if len(page.AlbumReviews) != 1 {
		t.Fatalf("got %d reviews, want 1", len(page.AlbumReviews))
	}
	a := page.AlbumReviews[0]
	if a.ID != 5521 || a.AlbumID == nil || *a.AlbumID != 45029 || deref(a.ArtistName) != "Stereolab" ||
		deref(a.AlbumTitle) != "Dots and Loops" || deref(a.RecordLabel) != "Duophonic" ||
		deref(a.Review) != "Great late-night record." || deref(a.RecommendedTracks) != "Brakhage!" ||
		deref(a.Buzzwords) != "lounge" || deref(a.FCCViolations) != "None" ||
		deref(a.ReviewPurpose) != "New addition" || a.Rotated == nil || !*a.Rotated ||
		deref(a.SubmittedAt) != "2019-02-03T18:44:00.000Z" || deref(a.Reviewer) != "Sam Example 2/3/19" {
		t.Errorf("album review = %+v", a)
	}
	p := page.Pagination
	if p.Page != 2 || p.Limit != 10 || p.Total == nil || *p.Total != 31 || p.HasMore == nil || !*p.HasMore {
		t.Errorf("pagination = %+v", p)
	}
}

func TestReview_HiddenDraftIsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Review not found"}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	_, _, err := c.Review(context.Background(), 9)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound || se.Path != "/reviews/9" {
		t.Errorf("err = %v, want a 404 StatusError for /reviews/9", err)
	}
}
