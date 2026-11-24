package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

// The server's window_hours bounds (RECENT_SHOWS_*_WINDOW_HOURS). It 400s
// outside them rather than clamping, so they are checked before any request.
const (
	recentDefaultHours = 24
	recentMaxHours     = 168
)

func newFlowsheetRecentCmd(app *App) *cobra.Command {
	var hours int
	cmd := &cobra.Command{
		Use:   "recent [--hours N]",
		Short: "Show who was on recently: shows started in the last N hours",
		Long: `Show the shows that started in the last N hours (1 to 168, default 24),
newest first, with their start and end times and the on-air handles of their
DJs. It answers "who was just on?" after djs-on-air has gone empty.

Show ids feed straight into ` + "`wxyc flowsheet show <id>`" + `.`,
		Example: `  wxyc flowsheet recent
  wxyc flowsheet recent --hours 6`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if hours < 1 || hours > recentMaxHours {
				return fmt.Errorf("--hours must be 1 to %d, got %d", recentMaxHours, hours)
			}
			r, raw, err := app.client.FlowsheetRecent(cmd.Context(), hours)
			if err != nil {
				return err
			}
			loc := rangeLoc()
			rows := make([][]string, 0, len(r.Shows))
			for _, s := range r.Shows {
				end := "on air"
				if s.EndTime != nil {
					end = localTime(*s.EndTime, loc)
				}
				rows = append(rows, []string{
					strconv.Itoa(s.ID), localTime(s.StartTime, loc), end, deref(s.ShowName), recentDJs(s.DJs),
				})
			}
			return app.render.EmitRaw(raw, []string{"ID", "START", "END", "SHOW", "DJS"}, rows)
		},
	}
	cmd.Flags().IntVar(&hours, "hours", recentDefaultHours, fmt.Sprintf("lookback window in hours (1 to %d)", recentMaxHours))
	return cmd
}

// recentDJs joins a show's DJ handles; one the server filtered to null is "?".
func recentDJs(djs []api.OnAirDJ) string {
	names := make([]string, len(djs))
	for i, d := range djs {
		names[i] = "?"
		if d.DJName != nil && *d.DJName != "" {
			names[i] = *d.DJName
		}
	}
	return strings.Join(names, ", ")
}

// deref is a nullable string's value, or "" for nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
