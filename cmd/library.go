package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

func newLibraryCmd(app *App) *cobra.Command {
	lib := &cobra.Command{Use: "library", Short: "Query the music library catalog"}
	lib.AddCommand(
		newLibrarySearchCmd(app),
		newLibraryArtistCmd(app),
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

func newLibraryArtistCmd(app *App) *cobra.Command {
	var genreID, page, limit int
	cmd := &cobra.Command{
		Use:   "artist <artist_id>",
		Short: "Show an artist's card and releases",
		Long: "Show an artist's card and a page of their releases.\n\n" +
			"In --json mode this emits an object with two keys, \"artist\" and \"releases\",\n" +
			"each holding that endpoint's response body verbatim.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("artist_id must be an integer: %q", args[0])
			}
			if limit < 1 || limit > 100 {
				return fmt.Errorf("--limit must be between 1 and 100, got %d", limit)
			}
			if page < 0 {
				return fmt.Errorf("--page must be zero or greater, got %d", page)
			}
			var genre *int
			if cmd.Flags().Changed("genre") {
				genre = &genreID
			}
			card, cardRaw, err := app.client.Artist(cmd.Context(), id, genre)
			if err != nil {
				return err
			}
			releases, releasesRaw, err := app.client.ArtistReleases(cmd.Context(), id, genre, page, limit)
			if err != nil {
				return err
			}
			if !app.render.JSON {
				writeArtistCard(app.stdout, card)
			}
			rows := make([][]string, 0, len(releases.Releases))
			for _, r := range releases.Releases {
				rows = append(rows, []string{
					strconv.Itoa(r.ID), r.AlbumTitle, r.Format, r.ShelfCode(),
				})
			}
			combined, err := json.Marshal(map[string]json.RawMessage{
				"artist":   cardRaw,
				"releases": releasesRaw,
			})
			if err != nil {
				return fmt.Errorf("combining responses for --json: %w", err)
			}
			return app.render.EmitRaw(combined, []string{"ID", "ALBUM", "FORMAT", "SHELF"}, rows)
		},
	}
	cmd.Flags().IntVar(&genreID, "genre", 0, "restrict to one genre membership (id)")
	cmd.Flags().IntVar(&page, "page", 0, "zero-based page number")
	cmd.Flags().IntVarP(&limit, "limit", "n", 50, "releases per page (1-100)")
	return cmd
}

func writeArtistCard(w io.Writer, card api.ArtistCard) {
	comp := ""
	if card.CodeCompLetter != nil && *card.CodeCompLetter != "" {
		comp = " · comp " + *card.CodeCompLetter
	}
	fmt.Fprintf(w, "Artist: %s\n", card.ArtistName)
	fmt.Fprintf(w, "Filing: %s %d%s\n", card.CodeLetters, card.CodeArtistNumber, comp)
	fmt.Fprintf(w, "Genre: %d · releases: %d\n\n", card.GenreID, card.ReleaseCount)
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
