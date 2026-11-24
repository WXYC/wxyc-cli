package cmd

import (
	"errors"
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

// maxRange is the server's MAX_RANGE_MS: the longest window it will serve.
const maxRange = 8 * 24 * time.Hour

// The local zone and clock that bare dates, zoneless times and
// today/yesterday/now resolve against; swapped in tests.
var (
	rangeLoc = func() *time.Location { return time.Local }
	rangeNow = time.Now
)

// Zoneless forms, read as local wall time.
var localLayouts = []string{
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
}

// parseRangeTime reads one --since/--until value. A whole-day form (a bare
// date, today, yesterday) returns that day's local midnight with day=true.
func parseRangeTime(s string, loc *time.Location, now time.Time) (t time.Time, day bool, err error) {
	midnight := func(t time.Time) time.Time {
		y, m, d := t.In(loc).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	switch strings.ToLower(s) {
	case "now":
		return now, false, nil
	case "today":
		return midnight(now), true, nil
	case "yesterday":
		y, m, d := now.In(loc).Date()
		return time.Date(y, m, d-1, 0, 0, 0, 0, loc), true, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, false, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return t, true, nil
	}
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, false, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("can't parse %q; use RFC 3339 (2026-11-20T22:00:00Z), a local date and time (2026-11-20 17:00), a date (2026-11-20), today, yesterday or now", s)
}

// nextDay is the midnight after a whole-day value: the end of that day. It
// is computed on the calendar, so a DST day is 23 or 25 hours long.
func nextDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, loc)
}

// rangeWindow resolves --since/--until into the server's half-open [start,
// end) window in epoch milliseconds, refusing anything the server would 400.
// A whole-day --until includes that day; a missing --until means the end of
// --since's day when that is a whole day, else now.
func rangeWindow(since, until string, loc *time.Location, now time.Time) (int64, int64, error) {
	if since == "" {
		return 0, 0, errors.New("--since is required")
	}
	start, sinceDay, err := parseRangeTime(since, loc, now)
	if err != nil {
		return 0, 0, fmt.Errorf("--since: %w", err)
	}
	var end time.Time
	switch {
	case until == "" && sinceDay:
		end = nextDay(start, loc)
	case until == "":
		end = now
	default:
		t, untilDay, err := parseRangeTime(until, loc, now)
		if err != nil {
			return 0, 0, fmt.Errorf("--until: %w", err)
		}
		end = t
		if untilDay {
			end = nextDay(t, loc)
		}
	}
	s, e := start.UnixMilli(), end.UnixMilli()
	if e <= s {
		return 0, 0, fmt.Errorf("--until must be after --since (window is %s to %s)",
			start.In(loc).Format(time.RFC3339), end.In(loc).Format(time.RFC3339))
	}
	if d := time.Duration(e-s) * time.Millisecond; d > maxRange {
		return 0, 0, fmt.Errorf("window is %s; the most is 8 days", days(d))
	}
	return s, e, nil
}

// days renders a window length as whole days when it is one, else as a
// duration.
func days(d time.Duration) string {
	if d%(24*time.Hour) == 0 {
		return strconv.Itoa(int(d/(24*time.Hour))) + " days"
	}
	return d.String()
}

func newFlowsheetRangeCmd(app *App) *cobra.Command {
	var since, until string
	cmd := &cobra.Command{
		Use:   "range --since <time> [--until <time>]",
		Short: "Show every entry logged in a time window (at most 8 days)",
		Long: `Show every flowsheet entry logged in a time window, across shows.

--since and --until take an RFC 3339 time (2026-11-20T22:00:00Z), a local
date and time (2026-11-20 17:00), a date (2026-11-20), today, yesterday or
now. Dates and zoneless times are in your local time zone. A date means the
whole day: as --until it includes that day. Without --until, the window runs
to the end of --since's day if it is a date, otherwise to now.

The window ends after it starts and is at most 8 days long.`,
		Example: `  wxyc flowsheet range --since yesterday
  wxyc flowsheet range --since 2026-11-20 --until 2026-11-22
  wxyc flowsheet range --since 2026-11-20T22:00:00Z --until 2026-11-21T02:00:00Z`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			loc := rangeLoc()
			start, end, err := rangeWindow(since, until, loc, rangeNow())
			if err != nil {
				return err
			}
			r, raw, err := app.client.FlowsheetRange(cmd.Context(), start, end)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, []string{"TIME", "SHOW", "TYPE", "ARTIST", "TRACK", "ALBUM"}, rangeRows(r, loc))
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "start of the window (required)")
	cmd.Flags().StringVar(&until, "until", "", "end of the window (default: end of --since's day, or now)")
	return cmd
}

// rangeRows labels each entry with its show and lists them in play order.
// The server orders by logging time; within each run of one show's entries,
// play order (which `flowsheet move` changes) wins.
func rangeRows(r api.FlowsheetRange, loc *time.Location) [][]string {
	labels := make(map[int]string, len(r.Shows))
	for _, s := range r.Shows {
		labels[s.ID] = rangeShowLabel(s)
	}
	var ordered []api.FlowsheetEntry
	for run := range chunkByShow(r.Entries) {
		run = slices.Clone(run)
		slices.SortStableFunc(run, func(a, b api.FlowsheetEntry) int { return a.PlayOrder - b.PlayOrder })
		ordered = append(ordered, run...)
	}
	rows := entryRows(ordered)
	for i, e := range ordered {
		label, ok := labels[e.ShowID]
		if !ok {
			label = strconv.Itoa(e.ShowID)
		}
		rows[i] = append([]string{localTime(e.AddTime, loc), label}, rows[i]...)
	}
	return rows
}

// chunkByShow yields the maximal runs of consecutive entries sharing a show.
func chunkByShow(entries []api.FlowsheetEntry) iter.Seq[[]api.FlowsheetEntry] {
	return func(yield func([]api.FlowsheetEntry) bool) {
		for i := 0; i < len(entries); {
			j := i + 1
			for j < len(entries) && entries[j].ShowID == entries[i].ShowID {
				j++
			}
			if !yield(entries[i:j]) {
				return
			}
			i = j
		}
	}
}

// rangeShowLabel is a show's table cell: its id (for `flowsheet show`), DJ
// and name, e.g. "101 DJ Ryan: Night Moves".
func rangeShowLabel(s api.RangeShow) string {
	label := strconv.Itoa(s.ID)
	if s.DJName != nil && *s.DJName != "" {
		label += " " + *s.DJName
	}
	if s.ShowName != nil && *s.ShowName != "" {
		if s.DJName != nil && *s.DJName != "" {
			label += ":"
		}
		label += " " + *s.ShowName
	}
	return label
}

// localTime shows a server timestamp in the local zone, to the minute; an
// unparseable one is shown as sent.
func localTime(s string, loc *time.Location) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.In(loc).Format("2006-01-02 15:04")
}
