package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/WXYC/wxyc-cli/internal/auth"
	"github.com/spf13/cobra"
)

func newDJsCmd(app *App) *cobra.Command {
	djs := &cobra.Command{Use: "djs", Short: "DJ playlists and profiles"}
	djs.AddCommand(newDJsPlaylistsCmd(app))
	return djs
}

func newDJsPlaylistsCmd(app *App) *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "playlists [dj_id]",
		Short: "List a DJ's past shows (defaults to your own)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			djID, err := resolveDJID(cmd, app, args)
			if err != nil {
				return err
			}
			playlists, raw, err := app.client.Playlists(cmd.Context(), djID)
			if err != nil {
				return err
			}
			if full {
				return emitFullPlaylists(cmd, app, playlists)
			}
			return app.render.EmitRaw(raw, []string{"DATE", "SHOW", "DJS", "PREVIEW"}, playlistRows(playlists))
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "fetch every track of every show (one request per show)")
	return cmd
}

// emitFullPlaylists fetches each listed show's complete flowsheet. --json
// emits a JSON array of the server's show bodies, byte-for-byte; the table
// lists one row per track, tagged with its show's date and id.
func emitFullPlaylists(cmd *cobra.Command, app *App, playlists []api.Playlist) error {
	bodies := make([]json.RawMessage, 0, len(playlists))
	var rows [][]string
	for _, p := range playlists {
		show, raw, err := app.client.Show(cmd.Context(), p.Show)
		if err != nil {
			return fmt.Errorf("show %d: %w", p.Show, err)
		}
		bodies = append(bodies, raw)
		date := p.Date
		if len(date) >= 10 {
			date = date[:10]
		}
		for _, e := range inPlayOrder(show.Entries) {
			if e.EntryType != "track" {
				continue
			}
			rows = append(rows, []string{date, strconv.Itoa(p.Show), e.ArtistName, e.TrackTitle, e.AlbumTitle})
		}
	}
	combined, err := json.Marshal(bodies)
	if err != nil {
		return err
	}
	return app.render.EmitRaw(combined, []string{"DATE", "SHOW", "ARTIST", "TRACK", "ALBUM"}, rows)
}

// resolveDJID uses the positional arg when given, otherwise falls back to the
// authenticated DJ's own id read from the current token's claims.
func resolveDJID(cmd *cobra.Command, app *App, args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	jwt, err := app.token(cmd.Context())
	if err != nil {
		return "", err
	}
	claims, err := auth.ParseClaims(jwt)
	if err != nil {
		return "", err
	}
	return claims.Sub, nil
}

func playlistRows(playlists []api.Playlist) [][]string {
	rows := make([][]string, 0, len(playlists))
	for _, p := range playlists {
		date := p.Date
		if len(date) >= 10 {
			date = date[:10] // trim ISO timestamp to YYYY-MM-DD
		}
		rows = append(rows, []string{
			date,
			strconv.Itoa(p.Show),
			djNames(p.DJs),
			previewTracks(p.Preview),
		})
	}
	return rows
}

func djNames(djs []api.PlaylistDJ) string {
	names := make([]string, 0, len(djs))
	for _, d := range djs {
		if d.DJName != "" {
			names = append(names, d.DJName)
		}
	}
	return strings.Join(names, ", ")
}

// previewTracks renders the opening tracks of a show as "Artist – Track"
// joined with "; ". Non-track rows (show_start markers) carry no track and are
// skipped.
func previewTracks(entries []api.FlowsheetEntry) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.EntryType != "track" {
			continue
		}
		parts = append(parts, e.ArtistName+" – "+e.TrackTitle)
	}
	return strings.Join(parts, "; ")
}
