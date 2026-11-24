package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/WXYC/wxyc-cli/internal/output"
	"github.com/spf13/cobra"
)

// The review routes take ids as 1..2147483647 (a Postgres integer) and 400
// anything else, so ids are checked before any request.
const reviewIDMax = math.MaxInt32

// albumReviewsLimitMax is GET /album-reviews' page-size ceiling.
const albumReviewsLimitMax = 100

// Review authors, revision editors and archive reviewers can be legal names
// (station-only, never for logs or telemetry). They are printed to the user
// only: nothing here puts one in an error message.
func newReviewsCmd(app *App) *cobra.Command {
	reviews := &cobra.Command{
		Use:   "reviews",
		Short: "Read album reviews: in-app reviews and the form archive",
		Long: `Read the station's two review stores: in-app reviews (GET /reviews) and the
Google Form archive (GET /album-reviews). A submission brought into the
in-app reviews is left out of the archive while that review exists, so the
two never overlap. Read-only.

Authors and reviewers can be legal names: keep them inside the station.`,
	}
	reviews.AddCommand(
		newReviewsListCmd(app),
		newReviewsShowCmd(app),
		newReviewsArchiveCmd(app),
		newReviewsForCmd(app),
	)
	return reviews
}

func newReviewsListCmd(app *App) *cobra.Command {
	var album, intake int64
	var mine bool
	cmd := &cobra.Command{
		Use:   "list [--album <id>] [--intake <id>] [--mine]",
		Short: "List in-app reviews",
		Long: `List the in-app reviews you may see; filters combine with AND. A draft is
visible only to its author and whoever recorded it. With --album, the reviews
on the release's cover come first (COVER column); otherwise newest first.
The REVIEW column is an excerpt: "wxyc reviews show <id>" prints it whole,
and --json carries the server's list verbatim.`,
		Example: `  wxyc reviews list --album 45029
  wxyc reviews list --mine`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := map[string]string{}
			if err := idFlag(cmd, "album", album, "album_id", params); err != nil {
				return err
			}
			if err := idFlag(cmd, "intake", intake, "intake_item_id", params); err != nil {
				return err
			}
			// mine=false is the server's no-filter, so only true is sent.
			if mine {
				params["mine"] = "true"
			}
			rs, raw, err := app.client.Reviews(cmd.Context(), params)
			if err != nil {
				return err
			}
			cover := cmd.Flags().Changed("album")
			return app.render.EmitRaw(raw, reviewHeaders(cover), reviewRows(rs, cover, app.loc))
		},
	}
	cmd.Flags().Int64Var(&album, "album", 0, "only reviews of this library album id")
	cmd.Flags().Int64Var(&intake, "intake", 0, "only reviews of this intake item id")
	cmd.Flags().BoolVar(&mine, "mine", false, "only your own reviews")
	return cmd
}

// idFlag adds an explicitly set id flag to params as key, refusing a value
// outside 1..reviewIDMax.
func idFlag(cmd *cobra.Command, flag string, v int64, key string, params map[string]string) error {
	if !cmd.Flags().Changed(flag) {
		return nil
	}
	if v < 1 || v > reviewIDMax {
		return fmt.Errorf("--%s must be an id from 1 to %d, got %d", flag, reviewIDMax, v)
	}
	params[key] = strconv.FormatInt(v, 10)
	return nil
}

// parseReviewID parses a positional id, refusing one outside 1..reviewIDMax.
func parseReviewID(name, s string) (int, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id < 1 || id > reviewIDMax {
		return 0, fmt.Errorf("%s must be an integer from 1 to %d, got %q", name, reviewIDMax, s)
	}
	return int(id), nil
}

func reviewHeaders(cover bool) []string {
	h := []string{"ID", "ALBUM", "AUTHOR", "STATUS", "SUBMITTED", "REVIEW"}
	if cover {
		h = append(h, "COVER")
	}
	return h
}

// reviewRows is the in-app review list. ALBUM falls back to the intake item
// for a review whose record isn't filed yet. The table cuts REVIEW to its
// cell width.
func reviewRows(rs []api.Review, cover bool, loc *time.Location) [][]string {
	rows := make([][]string, 0, len(rs))
	for _, r := range rs {
		subject := ""
		if r.AlbumID != nil {
			subject = strconv.Itoa(*r.AlbumID)
		} else if r.IntakeItemID != nil {
			subject = "item " + strconv.Itoa(*r.IntakeItemID)
		}
		submitted := ""
		if r.SubmittedAt != nil {
			submitted = localDate(*r.SubmittedAt, loc)
		}
		row := []string{strconv.Itoa(r.ID), subject, deref(r.Author), r.Status, submitted, reviewExcerpt(r.Review, r.Medium)}
		if cover {
			row = append(row, yes(r.OnCover))
		}
		rows = append(rows, row)
	}
	return rows
}

// reviewExcerpt is a review's text, or its medium for one with no text (a
// handwritten slip needs none).
func reviewExcerpt(text *string, medium string) string {
	if t := deref(text); strings.TrimSpace(t) != "" {
		return t
	}
	if medium != "" && medium != "typed" {
		return "(" + medium + ")"
	}
	return ""
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

// localDate is a server timestamp's calendar date in loc, or the value as is
// if it doesn't parse.
func localDate(s string, loc *time.Location) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.In(loc).Format("2006-01-02")
}

func newReviewsShowCmd(app *App) *cobra.Command {
	var revisions bool
	cmd := &cobra.Command{
		Use:   "show <id> [--revisions]",
		Short: "Show one in-app review in full",
		Long: `Show one in-app review: who wrote it, its status and dates, then its text in
full (buzzwords, artist blurb, review, recommended tracks, FCC notes). A draft
you may not see is "not found" (exit 5). --revisions adds its edit history,
newest first.

--json prints the server's GET /reviews/{id} body verbatim; with --revisions,
{"review": <GET /reviews/{id}>, "revisions": <GET /reviews/{id}/revisions>}.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseReviewID("review id", args[0])
			if err != nil {
				return err
			}
			r, raw, err := app.client.Review(cmd.Context(), id)
			if err != nil {
				return err
			}
			blocks := reviewBlocks(r, app.loc)
			if revisions {
				revs, revsRaw, err := app.client.ReviewRevisions(cmd.Context(), id)
				if err != nil {
					return err
				}
				if raw, err = combineRaw(map[string]json.RawMessage{"review": raw, "revisions": revsRaw}); err != nil {
					return err
				}
				blocks = append(blocks, output.Table{
					Headers: []string{"REV", "EDITED", "EDITED BY", "REVIEW"},
					Rows:    revisionRows(revs, app.loc),
					Empty:   "no revisions",
				})
			}
			return app.render.EmitRawBlocks(raw, blocks...)
		},
	}
	cmd.Flags().BoolVar(&revisions, "revisions", false, "also show the edit history")
	return cmd
}

// reviewBlocks is a review's FIELD/VALUE card followed by each non-empty
// text field printed whole.
func reviewBlocks(r api.Review, loc *time.Location) []output.Block {
	rows := [][]string{{"id", strconv.Itoa(r.ID)}}
	add := func(name, value string) {
		if value != "" {
			rows = append(rows, []string{name, value})
		}
	}
	if r.AlbumID != nil {
		add("album", strconv.Itoa(*r.AlbumID))
	}
	if r.IntakeItemID != nil {
		add("intake item", strconv.Itoa(*r.IntakeItemID))
	}
	add("author", deref(r.Author))
	add("medium", r.Medium)
	add("status", r.Status)
	add("added", r.AddDate)
	if r.SubmittedAt != nil {
		add("submitted", localTime(*r.SubmittedAt, loc))
	}
	add("modified", localTime(r.LastModified, loc))
	if r.PrintedAt != nil {
		add("printed", localTime(*r.PrintedAt, loc))
	}
	add("in use", strconv.FormatBool(r.InUse))
	add("revisions", strconv.Itoa(r.RevisionCount))

	blocks := []output.Block{output.Table{Headers: []string{"FIELD", "VALUE"}, Rows: rows}}
	for _, f := range []struct {
		heading string
		text    *string
	}{
		{"buzzwords", r.Buzzwords},
		{"artist blurb", r.ArtistBlurb},
		{"review", r.Review},
		{"recommended tracks", r.RecommendedTracks},
		{"fcc", r.FCC},
	} {
		if t := deref(f.text); strings.TrimSpace(t) != "" {
			blocks = append(blocks, output.Text{Heading: f.heading, Body: t})
		}
	}
	return blocks
}

func revisionRows(revs []api.ReviewRevision, loc *time.Location) [][]string {
	rows := make([][]string, 0, len(revs))
	for _, v := range revs {
		rows = append(rows, []string{strconv.Itoa(v.Revision), localTime(v.EditedAt, loc), deref(v.EditedBy), deref(v.Review)})
	}
	return rows
}

func newReviewsArchiveCmd(app *App) *cobra.Command {
	var album int64
	var artist string
	var page, limit int
	cmd := &cobra.Command{
		Use:   "archive [--album <id>] [--artist <name>] [--page N] [-n|--limit N]",
		Short: "List the album-review form archive",
		Long: `List the Google Form album-review archive, newest first, one page at a time
(pages start at 1; the server's default page size is 50). --artist matches
the artist name ignoring case and a leading "the". The REVIEW column is an
excerpt; --json carries the server's page verbatim.`,
		Example: `  wxyc reviews archive --artist stereolab
  wxyc reviews archive --album 45029 --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := map[string]string{}
			if err := idFlag(cmd, "album", album, "album_id", params); err != nil {
				return err
			}
			if cmd.Flags().Changed("artist") {
				if strings.TrimSpace(artist) == "" {
					return fmt.Errorf("--artist must not be blank")
				}
				params["artist"] = artist
			}
			if cmd.Flags().Changed("page") {
				if page < 1 {
					return fmt.Errorf("--page starts at 1, got %d", page)
				}
				params["page"] = strconv.Itoa(page)
			}
			if cmd.Flags().Changed("limit") {
				if limit < 1 || limit > albumReviewsLimitMax {
					return fmt.Errorf("--limit must be from 1 to %d, got %d", albumReviewsLimitMax, limit)
				}
				params["limit"] = strconv.Itoa(limit)
			}
			p, raw, err := app.client.AlbumReviews(cmd.Context(), params)
			if err != nil {
				return err
			}
			blocks := []output.Block{archiveTable(p, app.loc, "")}
			// An empty page past the end still says where it is, so it doesn't
			// read as an artist or album with no reviews at all.
			pg := p.Pagination
			if len(p.AlbumReviews) > 0 || pg.Page > 1 || (pg.Total != nil && *pg.Total > 0) {
				blocks = append(blocks, output.Note(archivePageLine(p)))
			}
			return app.render.EmitRawBlocks(raw, blocks...)
		},
	}
	cmd.Flags().Int64Var(&album, "album", 0, "only submissions linked to this library album id")
	cmd.Flags().StringVar(&artist, "artist", "", "only submissions for this artist")
	cmd.Flags().IntVar(&page, "page", 1, "page number, from 1")
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, fmt.Sprintf("submissions per page, 1 to %d (unset = server default, 50)", albumReviewsLimitMax))
	return cmd
}

func archiveTable(p api.AlbumReviewsPage, loc *time.Location, title string) output.Table {
	rows := make([][]string, 0, len(p.AlbumReviews))
	for _, a := range p.AlbumReviews {
		album, submitted := "", ""
		if a.AlbumID != nil {
			album = strconv.Itoa(*a.AlbumID)
		}
		if a.SubmittedAt != nil {
			submitted = localDate(*a.SubmittedAt, loc)
		}
		rows = append(rows, []string{
			strconv.Itoa(a.ID), album, deref(a.ArtistName), deref(a.AlbumTitle),
			deref(a.Reviewer), submitted, deref(a.Review),
		})
	}
	return output.Table{
		Title:   title,
		Headers: []string{"ID", "ALBUM", "ARTIST", "TITLE", "REVIEWER", "SUBMITTED", "REVIEW"},
		Rows:    rows,
		Empty:   "no archived reviews",
	}
}

// archivePageLine places the page, e.g. "page 2 of 4 (31 reviews, 10 per
// page); next: --page 3". total and hasMore are optional in the contract.
func archivePageLine(p api.AlbumReviewsPage) string {
	pg := p.Pagination
	line := "page " + strconv.Itoa(pg.Page)
	if pg.Total != nil && pg.Limit > 0 {
		pages := (*pg.Total + pg.Limit - 1) / pg.Limit
		line += fmt.Sprintf(" of %d (%d reviews, %d per page)", pages, *pg.Total, pg.Limit)
	} else if pg.Limit > 0 {
		line += fmt.Sprintf(" (%d per page)", pg.Limit)
	}
	if hasNextPage(p) {
		line += fmt.Sprintf("; next: --page %d", pg.Page+1)
	}
	return line
}

// hasNextPage reports whether a page follows p: hasMore when the server
// sends it, else whether total runs past this page.
func hasNextPage(p api.AlbumReviewsPage) bool {
	pg := p.Pagination
	if pg.HasMore != nil {
		return *pg.HasMore
	}
	return pg.Total != nil && pg.Limit > 0 && *pg.Total > pg.Page*pg.Limit
}

func newReviewsForCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "for <album_id>",
		Short: "Show every review of an album from both stores",
		Long: `Show an album's in-app reviews (cover reviews first) and its form-archive
submissions together. The album id is the ID column of "wxyc library search".
The archive part is its first page of 100; any more are named with the
command that lists them.

--json emits {"reviews": <GET /reviews?album_id=>, "archive": <GET
/album-reviews?album_id=>}, both server bodies verbatim.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseReviewID("album_id", args[0])
			if err != nil {
				return err
			}
			albumID := strconv.Itoa(id)
			rs, rsRaw, err := app.client.Reviews(cmd.Context(), map[string]string{"album_id": albumID})
			if err != nil {
				return err
			}
			p, pRaw, err := app.client.AlbumReviews(cmd.Context(), map[string]string{
				"album_id": albumID, "limit": strconv.Itoa(albumReviewsLimitMax),
			})
			if err != nil {
				return err
			}
			combined, err := combineRaw(map[string]json.RawMessage{"reviews": rsRaw, "archive": pRaw})
			if err != nil {
				return err
			}
			blocks := []output.Block{
				output.Table{
					Title:   "in-app reviews",
					Headers: reviewHeaders(true),
					Rows:    reviewRows(rs, true, app.loc),
					Empty:   "no in-app reviews",
				},
				archiveTable(p, app.loc, "form archive"),
			}
			if hasNextPage(p) {
				blocks = append(blocks, output.Note(fmt.Sprintf(
					"more archived reviews: wxyc reviews archive --album %s --page %d --limit %d",
					albumID, p.Pagination.Page+1, albumReviewsLimitMax)))
			}
			return app.render.EmitRawBlocks(combined, blocks...)
		},
	}
}
