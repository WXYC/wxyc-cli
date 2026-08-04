package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

func newLibraryCmd(app *App) *cobra.Command {
	lib := &cobra.Command{Use: "library", Short: "Query the music library catalog"}
	lib.AddCommand(
		newLibrarySearchCmd(app),
		newGenresCmd(app),
		newFormatsCmd(app),
		newRotationCmd(app),
	)
	return lib
}

func newGenresCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "genres",
		Short: "List the genre catalog",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			genres, raw, err := app.client.Genres(cmd.Context())
			if err != nil {
				return err
			}
			// The API's genre `plays` is a dead legacy counter (always 0;
			// BS#1486), so it is omitted from the table. The raw field is
			// still passed through in --json.
			rows := make([][]string, 0, len(genres))
			for _, g := range genres {
				rows = append(rows, []string{strconv.Itoa(g.ID), g.GenreName})
			}
			return app.render.EmitRaw(raw, []string{"ID", "GENRE"}, rows)
		},
	}
}

func newFormatsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "formats",
		Short: "List the media formats",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			formats, raw, err := app.client.Formats(cmd.Context())
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(formats))
			for _, f := range formats {
				rows = append(rows, []string{strconv.Itoa(f.ID), f.FormatName})
			}
			return app.render.EmitRaw(raw, []string{"ID", "FORMAT"}, rows)
		},
	}
}

func newRotationCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "rotation",
		Short: "Show the current rotation",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := app.client.Rotation(cmd.Context())
			if err != nil {
				return err
			}
			// Rotation's wire shape is deep; project a table from a generic
			// decode and pass the full JSON through in --json mode.
			var items []map[string]any
			_ = json.Unmarshal(raw, &items)
			rows := make([][]string, 0, len(items))
			for _, m := range items {
				rows = append(rows, []string{
					field(m, "rotation_bin"),
					field(m, "artist_name"),
					field(m, "album_title"),
					field(m, "record_label"),
				})
			}
			return app.render.EmitRaw(raw, []string{"BIN", "ARTIST", "ALBUM", "LABEL"}, rows)
		},
	}
}

// field formats a value from a generic JSON object as a display string,
// tolerating nulls and non-string types.
func field(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func newLibrarySearchCmd(app *App) *cobra.Command {
	var artist, album string
	var n int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search the catalog by artist and/or album",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := map[string]string{}
			if artist != "" {
				params["artist_name"] = artist
			}
			if album != "" {
				params["album_title"] = album
			}
			if n > 0 {
				params["n"] = strconv.Itoa(n)
			}
			albums, raw, err := app.client.LibrarySearch(cmd.Context(), params)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, []string{"ID", "ARTIST", "ALBUM", "FORMAT", "SHELF"}, albumRows(albums))
		},
	}
	cmd.Flags().StringVar(&artist, "artist", "", "artist name")
	cmd.Flags().StringVar(&album, "album", "", "album title")
	cmd.Flags().IntVarP(&n, "limit", "n", 0, "max results")
	return cmd
}

func albumRows(albums []api.Album) [][]string {
	rows := make([][]string, 0, len(albums))
	for _, a := range albums {
		rows = append(rows, []string{strconv.Itoa(a.ID), a.ArtistName, a.AlbumTitle, a.Format, a.ShelfCode()})
	}
	return rows
}
