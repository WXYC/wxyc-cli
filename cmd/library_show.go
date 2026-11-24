package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

func newLibraryShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show <album_id>",
		Short: "Show an album's catalog record and flowsheet play counts",
		Long: `Show one album's catalog record (artist, title, label, format, genre, shelf
code, …) and its flowsheet play counts. The album id is the ID column of
"wxyc library search".

Plays are reported by how they link to the album: direct (flowsheet.album_id),
rotation (via a rotation entry), and legacy (tubafrenzy-era plays keyed by the
legacy release id). The server counts these as separate arms that must not be
summed, so no total is shown.

--json emits {"album": <GET /library/info>, "play_counts": <GET
/library/{id}/flowsheet-play-counts>}, both server bodies verbatim.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("album_id must be an integer, got %q", args[0])
			}
			album, albumRaw, err := app.client.AlbumInfo(cmd.Context(), id)
			if err != nil {
				return err
			}
			counts, countsRaw, err := app.client.FlowsheetPlayCounts(cmd.Context(), id)
			if err != nil {
				return err
			}
			combined, err := json.Marshal(map[string]json.RawMessage{"album": albumRaw, "play_counts": countsRaw})
			if err != nil {
				return err
			}
			return app.render.EmitRaw(combined, []string{"FIELD", "VALUE"}, libraryShowRows(album, counts))
		},
	}
}

// libraryShowRows is the FIELD/VALUE table. Optional fields the server
// reports as null get no row. The legacy `plays` column is a dead counter
// (BS#1486) and is left to --json.
func libraryShowRows(a api.AlbumDetail, p api.PlayCounts) [][]string {
	rows := [][]string{{"id", strconv.Itoa(a.ID)}}
	add := func(name, value string) {
		if value != "" {
			rows = append(rows, []string{name, value})
		}
	}
	if a.LegacyReleaseID != nil {
		add("legacy id", strconv.Itoa(*a.LegacyReleaseID))
	}
	add("artist", a.ArtistName)
	add("album", a.AlbumTitle)
	add("album artist", deref(a.AlbumArtist))
	add("alternate artist", deref(a.AlternateArtistName))
	add("label", a.Label)
	add("format", a.Format)
	add("genre", a.GenreName)
	add("shelf", a.ShelfCode())
	if a.DiscQuantity != nil {
		add("discs", strconv.Itoa(*a.DiscQuantity))
	}
	if a.OnStreaming != nil {
		add("on streaming", strconv.FormatBool(*a.OnStreaming))
	}
	add("added", a.AddDate)
	add("modified", a.LastModified)
	add("lost", deref(a.DateLost))
	add("found", deref(a.DateFound))
	if a.DiscogsUnavailable {
		v := "true"
		if n := deref(a.DiscogsUnavailableNote); n != "" {
			v += " (" + n + ")"
		}
		add("discogs unavailable", v)
	}
	for _, u := range a.URLs {
		add("url", u)
	}
	rows = append(rows,
		[]string{"plays (direct)", strconv.Itoa(p.Direct)},
		[]string{"plays (rotation)", strconv.Itoa(p.RotationLinked)},
		[]string{"plays (legacy)", strconv.Itoa(p.LegacyLinked)},
	)
	return rows
}
