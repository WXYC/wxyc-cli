package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/WXYC/wxyc-cli/internal/output"
	"github.com/spf13/cobra"
)

// artistReleasesMax mirrors GET /library/artists/{id}/releases's MAX_LIMIT;
// the server 400s any limit outside 1..100.
const artistReleasesMax = 100

func newLibraryArtistCmd(app *App) *cobra.Command {
	var genre, page, limit int
	cmd := &cobra.Command{
		Use:   "artist <artist_id>",
		Short: "Show an artist's card and a page of their releases with shelf codes",
		Long: `Show an artist's card (name, genre, shelf code, release and cross-reference
counts), then one page of their releases in shelf order. The artist id is the
ARTIST_ID column of "wxyc library search".

An artist filed under more than one genre has one card per genre. Without
--genre the card is the artist's lowest-numbered genre and the releases span
every genre; --genre scopes both to one (genre ids: "wxyc library genres").
Pages are zero-based.

--json emits {"artist": <GET /library/artists/{id}>, "releases": <GET
/library/artists/{id}/releases>}, both server bodies verbatim.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil || id < 1 {
				return fmt.Errorf("artist_id must be a positive integer, got %q", args[0])
			}
			if genre < 0 {
				return fmt.Errorf("--genre must be a genre id (or 0 for all genres), got %d", genre)
			}
			if page < 0 {
				return fmt.Errorf("--page is zero-based and cannot be negative, got %d", page)
			}
			if limit < 0 || limit > artistReleasesMax {
				return fmt.Errorf("--limit must be from 1 to %d (or 0 for the server default), got %d", artistReleasesMax, limit)
			}
			params := map[string]string{}
			if genre > 0 {
				params["genre_id"] = strconv.Itoa(genre)
			}
			if cmd.Flags().Changed("page") {
				params["page"] = strconv.Itoa(page)
			}
			if limit > 0 {
				params["limit"] = strconv.Itoa(limit)
			}

			card, cardRaw, err := app.client.ArtistCard(cmd.Context(), id, genre)
			if err != nil {
				return err
			}
			rels, relsRaw, err := app.client.ArtistReleases(cmd.Context(), id, params)
			if err != nil {
				return err
			}
			combined, err := json.Marshal(map[string]json.RawMessage{"artist": cardRaw, "releases": relsRaw})
			if err != nil {
				return err
			}
			if app.render.JSON {
				return app.render.EmitRaw(combined, nil, nil)
			}
			// Release rows carry only genre_id; the shelf code needs the name.
			genres, _, err := app.client.Genres(cmd.Context())
			if err != nil {
				return err
			}
			names := make(map[int]string, len(genres))
			for _, g := range genres {
				names[g.ID] = g.GenreName
			}
			return app.render.EmitRawTables(combined,
				output.Table{Headers: []string{"FIELD", "VALUE"}, Rows: artistCardRows(card, rels, names)},
				output.Table{Headers: []string{"ID", "ALBUM", "FORMAT", "SHELF"}, Rows: artistReleaseRows(rels.Releases, names), Empty: "no releases"},
			)
		},
	}
	cmd.Flags().IntVar(&genre, "genre", 0, "scope the card and releases to one genre id (0 = all genres)")
	cmd.Flags().IntVar(&page, "page", 0, "zero-based page of releases (unset = server default, the first page)")
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, fmt.Sprintf("releases per page, 1 to %d (0 = server default)", artistReleasesMax))
	return cmd
}

// artistCardRows is the card's FIELD/VALUE table plus the releases page
// position. genre and shelf are for the card's one genre membership; the
// counts are artist-wide.
func artistCardRows(a api.ArtistCard, p api.ArtistReleasesPage, genres map[int]string) [][]string {
	genre := genreLabel(a.GenreID, genres)
	rows := [][]string{
		{"id", strconv.Itoa(a.ArtistID)},
		{"artist", a.ArtistName},
	}
	if a.AlphabeticalName != "" && a.AlphabeticalName != a.ArtistName {
		rows = append(rows, []string{"alphabetical", a.AlphabeticalName})
	}
	rows = append(rows,
		[]string{"genre", genre},
		[]string{"shelf", fmt.Sprintf("%s %s %d", genre, a.CodeLetters, a.CodeArtistNumber)},
	)
	if c := deref(a.CodeCompLetter); c != "" {
		rows = append(rows, []string{"comp letter", c})
	}
	rows = append(rows,
		[]string{"releases", strconv.Itoa(a.ReleaseCount)},
		[]string{"cross refs (source)", strconv.Itoa(a.CrossReferenceSourceCount)},
		[]string{"cross refs (target)", strconv.Itoa(a.CrossReferenceTargetCount)},
		[]string{"release cross refs", strconv.Itoa(a.LibraryCrossReferenceCount)},
		[]string{"compilation credits", strconv.Itoa(a.CompilationCreditCount)},
	)
	if p.TotalPages > 0 {
		rows = append(rows, []string{"page", fmt.Sprintf("%d of 0-%d", p.Page, p.TotalPages-1)})
	}
	return rows
}

func artistReleaseRows(rels []api.ArtistRelease, genres map[int]string) [][]string {
	rows := make([][]string, 0, len(rels))
	for _, r := range rels {
		shelf := fmt.Sprintf("%s %s %d/%d", genreLabel(r.GenreID, genres), r.CodeLetters, r.CodeArtistNumber, r.CodeNumber)
		if v := deref(r.CodeVolumeLetters); v != "" {
			shelf += " " + v
		}
		rows = append(rows, []string{strconv.Itoa(r.ID), r.AlbumTitle, r.Format, shelf})
	}
	return rows
}

// genreLabel names a genre id, falling back to the bare id for one the
// genre list doesn't carry.
func genreLabel(id int, genres map[int]string) string {
	if n, ok := genres[id]; ok {
		return n
	}
	return "genre " + strconv.Itoa(id)
}
