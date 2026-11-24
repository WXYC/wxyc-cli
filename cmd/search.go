package cmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// These mirror Backend-Service search.controller.ts. The server silently
// falls back to date/desc on an unknown sort/order, so the CLI checks them
// locally rather than quietly returning a differently-sorted result.
var (
	searchSorts  = []string{"date", "artist", "song", "dj"}
	searchOrders = []string{"asc", "desc"}
)

const searchMaxLimit = 100

func newFlowsheetSearchCmd(app *App) *cobra.Command {
	var limit, page int
	var sort, order, cursor string
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search the playlist archive (no query = most recent tracks)",
		Long: `Search the public playlist archive of every track played on WXYC.

With no query it lists the most recent tracks. Results page by offset
(--page) or, for date-sorted searches, by the cursor printed under the
table (--cursor), which stays stable as new tracks are logged.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			f := cmd.Flags()
			if f.Changed("limit") && (limit < 1 || limit > searchMaxLimit) {
				return fmt.Errorf("--limit must be from 1 to %d, got %d", searchMaxLimit, limit)
			}
			if page < 0 {
				return fmt.Errorf("--page must be 0 or more, got %d", page)
			}
			if f.Changed("sort") && !slices.Contains(searchSorts, sort) {
				return fmt.Errorf("--sort must be one of %s, got %q", strings.Join(searchSorts, ", "), sort)
			}
			if f.Changed("order") && !slices.Contains(searchOrders, order) {
				return fmt.Errorf("--order must be asc or desc, got %q", order)
			}

			// Send only what was given so the server's own defaults apply.
			params := map[string]string{}
			if q := strings.Join(args, " "); q != "" {
				params["q"] = q
			}
			if f.Changed("limit") {
				params["limit"] = strconv.Itoa(limit)
			}
			if f.Changed("page") {
				params["page"] = strconv.Itoa(page)
			}
			if f.Changed("sort") {
				params["sort"] = sort
			}
			if f.Changed("order") {
				params["order"] = order
			}
			if f.Changed("cursor") {
				params["cursor"] = cursor
			}

			res, raw, err := app.client.FlowsheetSearch(cmd.Context(), params)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(res.Results))
			for _, r := range res.Results {
				rows = append(rows, []string{r.PlayDate, r.DJName, r.ArtistName, r.TrackTitle, r.AlbumTitle})
			}
			if err := app.render.EmitRaw(raw, []string{"DATE", "DJ", "ARTIST", "TRACK", "ALBUM"}, rows); err != nil {
				return err
			}
			if !app.render.JSON {
				// --json stays the raw server body; the footer is table-only.
				fmt.Fprint(app.stdout, searchFooter(res.Page, res.TotalPages, res.Total, f.Changed("cursor"), res.NextCursor))
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 50, fmt.Sprintf("results per page, 1 to %d", searchMaxLimit))
	cmd.Flags().IntVar(&page, "page", 0, "0-based page number (offset paging)")
	cmd.Flags().StringVar(&sort, "sort", "date", "sort by: "+strings.Join(searchSorts, ", "))
	cmd.Flags().StringVar(&order, "order", "desc", "sort order: asc or desc")
	cmd.Flags().StringVar(&cursor, "cursor", "", "continue from a previous page's next cursor (date sort only)")
	return cmd
}

// searchCountCap mirrors Backend-Service's COUNT_CAP: the server bounds its
// count and reports COUNT_CAP+1 as a "10000+" sentinel (BS#1681).
const searchCountCap = 10000

// searchFooter summarizes paging under the table and, when the server handed
// back a cursor, how to fetch the next page with it. In cursor mode the
// server's page is always 0, so no page number is shown.
func searchFooter(page, totalPages, total int, cursorMode bool, next string) string {
	count := strconv.Itoa(total)
	if total > searchCountCap {
		count = strconv.Itoa(searchCountCap) + "+"
	}
	var b strings.Builder
	if cursorMode {
		fmt.Fprintf(&b, "\n%s results\n", count)
	} else {
		fmt.Fprintf(&b, "\npage %d of %d (%s results)\n", page+1, max(totalPages, 1), count)
	}
	if next != "" {
		fmt.Fprintf(&b, "next page: --cursor %s\n", next)
	}
	return b.String()
}
