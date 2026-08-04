package cmd

import (
	"fmt"
	"strconv"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

func newFlowsheetCmd(app *App) *cobra.Command {
	fs := &cobra.Command{Use: "flowsheet", Short: "Read and manage the on-air flowsheet log"}
	fs.AddCommand(
		newFlowsheetTailCmd(app),
		newFlowsheetStartCmd(app),
		newFlowsheetAddCmd(app),
		newFlowsheetMarkerCmd(app, "talkset", "Log a talkset", "Talkset", "talkset"),
		newFlowsheetMarkerCmd(app, "breakpoint", "Log a breakpoint (top of hour)", "Breakpoint", "breakpoint"),
		newFlowsheetMoveCmd(app),
		newFlowsheetEditCmd(app),
		newFlowsheetRmCmd(app),
		newFlowsheetEndCmd(app),
	)
	return fs
}

func newFlowsheetStartCmd(app *App) *cobra.Command {
	var name, as string
	var specialty int
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start your show (or join the active show as co-host)",
		Args:  cobra.NoArgs,
		// Mutating: the root PersistentPreRunE blocks this unless writes are unlocked.
		Annotations: map[string]string{annMutates: "true", annOp: "flowsheet:start"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			// dj_id must match the authenticated user; read it from the token.
			djID, err := resolveDJID(cmd, app, nil)
			if err != nil {
				return err
			}
			req := api.StartShowRequest{DJID: djID, ShowName: name, DJNameOverride: as}
			if specialty > 0 {
				req.SpecialtyID = &specialty
			}
			s, raw, err := app.client.FlowsheetStart(cmd.Context(), req)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, showHeaders, showRows(s, "started", "joined"))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "show name")
	cmd.Flags().StringVar(&as, "as", "", "per-show DJ display name override")
	cmd.Flags().IntVar(&specialty, "specialty", 0, "specialty show id")
	return cmd
}

func newFlowsheetAddCmd(app *App) *cobra.Command {
	var track, artist, album, label string
	var albumID, rotationID int
	var segue, request bool
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a played track to the flowsheet",
		Args:  cobra.NoArgs,
		// Mutating: the root PersistentPreRunE blocks this unless writes are unlocked.
		Annotations: map[string]string{annMutates: "true", annOp: "flowsheet:add"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if track == "" {
				return fmt.Errorf("--track is required")
			}
			// With --album-id the server backfills artist/album/label; without
			// it those fields are required, so fail fast rather than 400.
			if albumID == 0 && (artist == "" || album == "") {
				return fmt.Errorf("--artist and --album are required unless --album-id is given")
			}
			t := api.FlowsheetTrack{
				TrackTitle:  track,
				ArtistName:  artist,
				AlbumTitle:  album,
				RecordLabel: label,
				Segue:       segue,
				RequestFlag: request,
			}
			if albumID > 0 {
				t.AlbumID = &albumID
			}
			if rotationID > 0 {
				t.RotationID = &rotationID
			}
			e, raw, err := app.client.FlowsheetAddTrack(cmd.Context(), t)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, entryHeaders, entryResultRows(e))
		},
	}
	cmd.Flags().StringVar(&track, "track", "", "track title (required)")
	cmd.Flags().StringVar(&artist, "artist", "", "artist name")
	cmd.Flags().StringVar(&album, "album", "", "album title")
	cmd.Flags().StringVar(&label, "label", "", "record label")
	cmd.Flags().IntVar(&albumID, "album-id", 0, "library album id (backfills artist/album/label)")
	cmd.Flags().IntVar(&rotationID, "rotation-id", 0, "rotation id")
	cmd.Flags().BoolVar(&segue, "segue", false, "mark as a segue into the next track")
	cmd.Flags().BoolVar(&request, "request", false, "mark as a listener request")
	return cmd
}

// newFlowsheetMarkerCmd builds a talkset/breakpoint command that appends a
// fixed non-track entry, tagging it with the given entry_type explicitly.
func newFlowsheetMarkerCmd(app *App, use, short, message, entryType string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		// Mutating: the root PersistentPreRunE blocks this unless writes are unlocked.
		Annotations: map[string]string{annMutates: "true", annOp: "flowsheet:" + entryType},
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, raw, err := app.client.FlowsheetAddMarker(cmd.Context(), message, entryType)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, entryHeaders, entryResultRows(e))
		},
	}
}

func newFlowsheetMoveCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "move <entry_id> <new_position>",
		Short: "Reorder a flowsheet entry to a new 1-based position",
		Args:  cobra.ExactArgs(2),
		// Mutating: the root PersistentPreRunE blocks this unless writes are unlocked.
		Annotations: map[string]string{annMutates: "true", annOp: "flowsheet:move"},
		RunE: func(cmd *cobra.Command, args []string) error {
			entryID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("entry_id must be an integer: %q", args[0])
			}
			newPos, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("new_position must be an integer: %q", args[1])
			}
			if newPos < 1 {
				return fmt.Errorf("new_position must be >= 1 (positions are 1-based)")
			}
			e, raw, err := app.client.FlowsheetMove(cmd.Context(), entryID, newPos)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, entryHeaders, entryResultRows(e))
		},
	}
}

func newFlowsheetEditCmd(app *App) *cobra.Command {
	var track, artist, album, label, message string
	var albumID, rotationID int
	var segue, request bool
	cmd := &cobra.Command{
		Use:   "edit <entry_id>",
		Short: "Edit fields of a flowsheet entry",
		Args:  cobra.ExactArgs(1),
		// Mutating: the root PersistentPreRunE blocks this unless writes are unlocked.
		Annotations: map[string]string{annMutates: "true", annOp: "flowsheet:edit"},
		RunE: func(cmd *cobra.Command, args []string) error {
			entryID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("entry_id must be an integer: %q", args[0])
			}
			// Only send fields whose flags were explicitly changed, so an
			// unset flag is left alone and a --flag "" clears the field. This
			// also distinguishes booleans from their zero value.
			var data api.FlowsheetUpdateFields
			f := cmd.Flags()
			changed := false
			if f.Changed("track") {
				data.TrackTitle, changed = &track, true
			}
			if f.Changed("artist") {
				data.ArtistName, changed = &artist, true
			}
			if f.Changed("album") {
				data.AlbumTitle, changed = &album, true
			}
			if f.Changed("label") {
				data.RecordLabel, changed = &label, true
			}
			if f.Changed("message") {
				data.Message, changed = &message, true
			}
			if f.Changed("album-id") {
				data.AlbumID, changed = &albumID, true
			}
			if f.Changed("rotation-id") {
				data.RotationID, changed = &rotationID, true
			}
			if f.Changed("segue") {
				data.Segue, changed = &segue, true
			}
			if f.Changed("request") {
				data.RequestFlag, changed = &request, true
			}
			if !changed {
				return fmt.Errorf("no fields to edit: pass at least one field flag")
			}
			e, raw, err := app.client.FlowsheetUpdate(cmd.Context(), entryID, data)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, entryHeaders, entryResultRows(e))
		},
	}
	cmd.Flags().StringVar(&track, "track", "", "track title")
	cmd.Flags().StringVar(&artist, "artist", "", "artist name")
	cmd.Flags().StringVar(&album, "album", "", "album title")
	cmd.Flags().StringVar(&label, "label", "", "record label")
	cmd.Flags().StringVar(&message, "message", "", "freeform message (marker rows)")
	cmd.Flags().IntVar(&albumID, "album-id", 0, "library album id")
	cmd.Flags().IntVar(&rotationID, "rotation-id", 0, "rotation id")
	cmd.Flags().BoolVar(&segue, "segue", false, "mark as a segue into the next track")
	cmd.Flags().BoolVar(&request, "request", false, "mark as a listener request")
	return cmd
}

func newFlowsheetRmCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <entry_id>",
		Short: "Remove a flowsheet entry",
		Args:  cobra.ExactArgs(1),
		// Mutating: the root PersistentPreRunE blocks this unless writes are unlocked.
		Annotations: map[string]string{annMutates: "true", annOp: "flowsheet:rm"},
		RunE: func(cmd *cobra.Command, args []string) error {
			entryID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("entry_id must be an integer: %q", args[0])
			}
			e, raw, err := app.client.FlowsheetDelete(cmd.Context(), entryID)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, entryHeaders, entryResultRows(e))
		},
	}
}

func newFlowsheetEndCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "end",
		Short: "End your show (or leave the active show as co-host)",
		Args:  cobra.NoArgs,
		// Mutating: the root PersistentPreRunE blocks this unless writes are unlocked.
		Annotations: map[string]string{annMutates: "true", annOp: "flowsheet:end"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			djID, err := resolveDJID(cmd, app, nil)
			if err != nil {
				return err
			}
			s, raw, err := app.client.FlowsheetEnd(cmd.Context(), djID)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, showHeaders, showRows(s, "ended", "left"))
		},
	}
}

var (
	showHeaders  = []string{"SHOW", "NAME", "PRIMARY_DJ", "STATUS"}
	entryHeaders = []string{"ID", "TYPE", "ARTIST", "TITLE"}
)

// showRows renders a start/end confirmation. A show row (ID set) uses the solo
// label; a co-host show_djs row (ShowID set) uses the cohost label.
func showRows(s api.ShowSession, solo, cohost string) [][]string {
	status := solo
	if s.ID == 0 {
		status = cohost
	}
	return [][]string{{strconv.Itoa(s.EffectiveShowID()), s.ShowName, s.PrimaryDJID, status}}
}

// entryResultRows renders the echoed entry. Marker rows carry no artist/track,
// so their message goes in the title column.
func entryResultRows(e api.FlowsheetResult) [][]string {
	artist, title := e.ArtistName, e.TrackTitle
	if e.EntryType != "track" {
		artist, title = "", e.Message
	}
	return [][]string{{strconv.Itoa(e.ID), e.EntryType, artist, title}}
}

func newFlowsheetTailCmd(app *App) *cobra.Command {
	var n int
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Show the most recent flowsheet entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries, raw, err := app.client.Flowsheet(cmd.Context(), n)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, []string{"TYPE", "ARTIST", "TRACK", "ALBUM"}, entryRows(entries))
		},
	}
	cmd.Flags().IntVarP(&n, "limit", "n", 10, "number of entries")
	return cmd
}

func entryRows(entries []api.FlowsheetEntry) [][]string {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		artist, track := e.ArtistName, e.TrackTitle
		if e.EntryType != "track" {
			// Marker rows (show_start/end, dj_join/leave) carry the DJ name.
			artist = e.DJName
		}
		rows = append(rows, []string{e.EntryType, artist, track, e.AlbumTitle})
	}
	return rows
}
