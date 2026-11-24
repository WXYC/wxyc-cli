package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

// nowEntries is how many of the newest flowsheet rows `now` reads to find the
// last track past any markers and spotlights logged after it.
const nowEntries = 20

func newFlowsheetNowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "now",
		Short: "Show what's on air right now",
		Long: `Show what's on air right now: the DJs on the current show, its name and
specialty program, what the request line is set to and why, the current
spotlight, and the last track played.

Status is "live" while a show is open, "Auto DJ" when the station is on
automation, and "unknown" when the server does not report it.

--json emits {"flowsheet": <GET /flowsheet>, "djs_on_air": <GET
/flowsheet/djs-on-air>}, both server bodies verbatim.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			page, pageRaw, err := app.client.FlowsheetPage(cmd.Context(), nowEntries)
			if err != nil {
				return err
			}
			djs, djsRaw, err := app.client.DJsOnAir(cmd.Context())
			if err != nil {
				return err
			}
			rows, err := nowRows(page, djs)
			if err != nil {
				return err
			}
			combined, err := combineRaw(map[string]json.RawMessage{"flowsheet": pageRaw, "djs_on_air": djsRaw})
			if err != nil {
				return err
			}
			return app.render.EmitRaw(combined, []string{"FIELD", "VALUE"}, rows)
		},
	}
}

// nowRows is the FIELD/VALUE table for the three on_air states. Show details
// appear only while a show is live; the DJ list, which is its own endpoint,
// appears whenever it names anyone.
func nowRows(page api.FlowsheetPageResult, djs []api.OnAirDJ) ([][]string, error) {
	o, err := page.OnAir()
	live := err == nil
	var status string
	switch {
	case live:
		status = "live"
	case errors.Is(err, api.ErrNoOpenShow):
		status = "Auto DJ"
	case errors.Is(err, api.ErrOnAirUnknown):
		status = "unknown"
	default:
		return nil, err
	}
	rows := [][]string{{"status", status}}

	switch {
	case len(djs) > 0:
		rows = append(rows, []string{"djs", recentDJs(djs)})
	case live && o.DJName != "":
		rows = append(rows, []string{"djs", o.DJName})
	}

	if live {
		name := "untitled"
		if o.ShowName != nil && *o.ShowName != "" {
			name = *o.ShowName
		}
		specialty := "none (freeform)"
		if o.Specialty != nil {
			specialty = o.Specialty.Name
		}
		requestLine := "unknown"
		if rl := o.RequestLine; rl != nil {
			requestLine = fmt.Sprintf("%s (source: %s)", rl.Mode, rl.Source)
		}
		spotlight := "none"
		if s := o.Spotlight; s != nil {
			spotlight = spotlightLabel(s.SpotlightName, s.SpotlightRole)
		}
		rows = append(rows,
			[]string{"show", fmt.Sprintf("%s (show %d)", name, o.ShowID)},
			[]string{"specialty", specialty},
			[]string{"request line", requestLine},
			[]string{"spotlight", spotlight},
		)
	}

	rows = append(rows, []string{"last track", lastTrack(page.Entries)})
	return rows, nil
}

// lastTrack labels the most recently played track among entries: the one in
// the newest show with the highest play order, regardless of body order.
func lastTrack(entries []api.FlowsheetEntry) string {
	var last *api.FlowsheetEntry
	for i, e := range entries {
		if e.EntryType != "track" {
			continue
		}
		if last == nil || e.ShowID > last.ShowID || (e.ShowID == last.ShowID && e.PlayOrder > last.PlayOrder) {
			last = &entries[i]
		}
	}
	if last == nil {
		return "none"
	}
	label := last.ArtistName + " – " + last.TrackTitle
	if last.AlbumTitle != "" {
		label += " (" + last.AlbumTitle + ")"
	}
	return label
}
