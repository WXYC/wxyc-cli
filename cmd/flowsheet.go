package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

func newFlowsheetCmd(app *App) *cobra.Command {
	fs := &cobra.Command{Use: "flowsheet", Short: "Read and manage the on-air flowsheet log"}
	fs.AddCommand(
		newFlowsheetTailCmd(app),
		newFlowsheetShowCmd(app),
		newFlowsheetSearchCmd(app),
		newFlowsheetStartCmd(app),
		newFlowsheetAddCmd(app),
		newFlowsheetMarkerCmd(app, "talkset", "Log a talkset", "Talkset", "talkset"),
		newFlowsheetMarkerCmd(app, "breakpoint", "Log a breakpoint (top of hour)", "Breakpoint", "breakpoint"),
		newFlowsheetSpotlightCmd(app),
		newFlowsheetRequestLineCmd(app),
		newFlowsheetMoveCmd(app),
		newFlowsheetEditCmd(app),
		newFlowsheetRmCmd(app),
		newFlowsheetEndCmd(app),
	)
	return fs
}

func newFlowsheetStartCmd(app *App) *cobra.Command {
	var name, as string
	var specialty string
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
			if cmd.Flags().Changed("specialty") {
				id, err := resolveSpecialty(cmd, app, specialty)
				if err != nil {
					return err
				}
				req.SpecialtyID = &id
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
	cmd.Flags().StringVar(&specialty, "specialty", "", "specialty show slug, e.g. new-science-experience (a numeric id is also accepted)")
	return cmd
}

// resolveSpecialty turns a --specialty value into a catalog id. A numeric value
// is used as the id; anything else is matched against the active programs'
// slugs before the join is sent, so a typo fails fast and names the valid slugs.
func resolveSpecialty(cmd *cobra.Command, app *App, v string) (int, error) {
	if id, err := strconv.Atoi(v); err == nil {
		return id, nil
	}
	shows, _, err := app.client.SpecialtyShows(cmd.Context())
	if err != nil {
		return 0, err
	}
	var slugs []string
	for _, s := range shows {
		if !s.Active {
			continue
		}
		if s.Slug == v {
			return s.ID, nil
		}
		slugs = append(slugs, s.Slug)
	}
	return 0, fmt.Errorf("unknown specialty %q; valid slugs: %s", v, strings.Join(slugs, ", "))
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

// spotlightRoles is the server's closed set of who's-on-air roles.
var spotlightRoles = []string{"performer", "dj", "reader", "guest", "host"}

const spotlightNameMax = 128 // characters (code points), as the server counts them

func newFlowsheetSpotlightCmd(app *App) *cobra.Command {
	var role string
	cmd := &cobra.Command{
		Use:   "spotlight --role <role> <name>",
		Short: "Record who is on air in the open show",
		Long: `Record who is on air in the open show: a band playing live, a reader, a
guest DJ mixing, a studio guest, or the host taking the focus back.

--role is one of: ` + strings.Join(spotlightRoles, ", ") + `.
The name is the remaining arguments, trimmed, 1 to 128 characters.

The name is public once entered: it shows in the listener apps, on the
Lock Screen and in the public playlist archive. For a dj entry, use the
name the DJ uses on air, never a real name.`,
		Example: `  wxyc flowsheet spotlight --role performer The Weirs --write`,
		// Mutating: the root PersistentPreRunE blocks this unless writes are unlocked.
		Annotations: map[string]string{annMutates: "true", annOp: "flowsheet:spotlight"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkSpotlightRole("--role", role); err != nil {
				return err
			}
			name := strings.TrimSpace(strings.Join(args, " "))
			if err := checkSpotlightName("name", name); err != nil {
				return err
			}
			e, raw, err := app.client.FlowsheetAddSpotlight(cmd.Context(), role, name)
			if err != nil {
				return spotlightError(err)
			}
			return app.render.EmitRaw(raw, entryHeaders, entryResultRows(e))
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "who is on air: "+strings.Join(spotlightRoles, ", "))
	return cmd
}

// checkSpotlightRole refuses a role outside spotlightRoles before any request.
func checkSpotlightRole(flag, role string) error {
	if !slices.Contains(spotlightRoles, role) {
		return fmt.Errorf("%s must be one of: %s", flag, strings.Join(spotlightRoles, ", "))
	}
	return nil
}

// checkSpotlightName refuses a (trimmed) name the server would 400.
func checkSpotlightName(what, name string) error {
	if n := utf8.RuneCountInString(name); n < 1 || n > spotlightNameMax {
		return fmt.Errorf("%s must be 1 to %d characters, got %d", what, spotlightNameMax, n)
	}
	return nil
}

// spotlightLabel is a spotlight row's table cell, e.g. "Jane Doe (guest)". An
// unrecognized role is shown as-is: the server's role list is open-ended.
func spotlightLabel(name, role string) string {
	if role == "" {
		return name
	}
	return name + " (" + role + ")"
}

// spotlightError turns the server's refusals into one readable line while
// keeping the *api.StatusError in the chain, so mapExit still classifies it.
func spotlightError(err error) error {
	var se *api.StatusError
	if !errors.As(err, &se) {
		return err
	}
	var body struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	_ = json.Unmarshal([]byte(se.Body), &body)
	switch {
	case se.Code == http.StatusForbidden && body.Code == "spotlight_disabled":
		return &oneLineError{msg: "who's on air is not turned on yet", err: err}
	case se.Code == http.StatusBadRequest && strings.Contains(body.Message, "not a member of show"):
		return &oneLineError{msg: "you are not on the open show; run `wxyc flowsheet start` to join it", err: err}
	case se.Code == http.StatusBadRequest && body.Message != "":
		return &oneLineError{msg: body.Message, err: err}
	}
	return err
}

// oneLineError replaces a wrapped error's text with a short message but still
// unwraps to it, so errors.As (and so the exit code) sees the original.
type oneLineError struct {
	msg string
	err error
}

func (e *oneLineError) Error() string { return e.msg }
func (e *oneLineError) Unwrap() error { return e.err }

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
	var track, artist, album, label, message, spotRole, spotName string
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
			spotlight := false
			if f.Changed("spotlight-role") {
				if err := checkSpotlightRole("--spotlight-role", spotRole); err != nil {
					return err
				}
				data.SpotlightRole, changed, spotlight = &spotRole, true, true
			}
			if f.Changed("spotlight-name") {
				spotName = strings.TrimSpace(spotName)
				if err := checkSpotlightName("--spotlight-name", spotName); err != nil {
					return err
				}
				data.SpotlightName, changed, spotlight = &spotName, true, true
			}
			if !changed {
				return fmt.Errorf("no fields to edit: pass at least one field flag")
			}
			e, raw, err := app.client.FlowsheetUpdate(cmd.Context(), entryID, data)
			if err != nil {
				if spotlight {
					return spotlightError(err)
				}
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
	cmd.Flags().StringVar(&spotRole, "spotlight-role", "", "spotlight rows only: "+strings.Join(spotlightRoles, ", "))
	cmd.Flags().StringVar(&spotName, "spotlight-name", "", "spotlight rows only: who is on air, 1 to 128 characters (public)")
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
	switch e.EntryType {
	case "track":
	case "spotlight":
		artist, title = spotlightLabel(e.SpotlightName, e.SpotlightRole), e.Message
	default:
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

func newFlowsheetShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show <show_id>",
		Short: "Show every entry of one show (ids from `djs playlists`)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			showID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("show_id must be an integer, got %q", args[0])
			}
			show, raw, err := app.client.Show(cmd.Context(), showID)
			if err != nil {
				return err
			}
			if !app.render.JSON {
				// --json stays the raw server body; the header is table-only.
				fmt.Fprint(app.stdout, showHeader(show))
			}
			return app.render.EmitRaw(raw, []string{"TYPE", "ARTIST", "TRACK", "ALBUM"}, entryRows(inPlayOrder(show.Entries)))
		},
	}
}

// showHeader is the table view's preamble for one show: its name, specialty
// program, airtime, DJs (marking any who left) and the adjacent show ids to
// page through the archive with, followed by a blank line.
func showHeader(s api.Show) string {
	var b strings.Builder
	fmt.Fprintf(&b, "show %d", s.ID)
	if s.ShowName != nil && *s.ShowName != "" {
		fmt.Fprintf(&b, ": %s", *s.ShowName)
	}
	b.WriteByte('\n')
	if s.Specialty != nil {
		fmt.Fprintf(&b, "program: %s\n", s.Specialty.Name)
	}
	end := "(no end)"
	if s.EndTime != nil {
		end = *s.EndTime
	}
	fmt.Fprintf(&b, "time:    %s – %s\n", s.StartTime, end)
	if djs := showDJNames(s); len(djs) > 0 {
		fmt.Fprintf(&b, "djs:     %s\n", strings.Join(djs, ", "))
	}
	fmt.Fprintf(&b, "prev:    %s\n", showIDOrNone(s.PreviousShowID))
	fmt.Fprintf(&b, "next:    %s\n\n", showIDOrNone(s.NextShowID))
	return b.String()
}

// showDJNames lists a show's DJs, "(left)" marking any no longer on it. A
// legacy show whose DJ had no account has no show_djs rows; its legacy_dj_name
// stands in.
func showDJNames(s api.Show) []string {
	var names []string
	for _, dj := range s.ShowDJs {
		name := ""
		switch {
		case dj.DJName != nil && *dj.DJName != "":
			name = *dj.DJName
		case dj.ID != nil:
			name = *dj.ID
		default:
			continue
		}
		if !dj.Active {
			name += " (left)"
		}
		names = append(names, name)
	}
	if len(names) == 0 && s.LegacyDJName != nil && *s.LegacyDJName != "" {
		names = append(names, *s.LegacyDJName)
	}
	return names
}

func showIDOrNone(id *int) string {
	if id == nil {
		return "none"
	}
	return strconv.Itoa(*id)
}

// inPlayOrder returns a show's entries sorted first-played first. The server
// sends them newest-first; --json keeps that order, the table reads top-down.
func inPlayOrder(entries []api.FlowsheetEntry) []api.FlowsheetEntry {
	sorted := slices.Clone(entries)
	slices.SortStableFunc(sorted, func(a, b api.FlowsheetEntry) int { return a.PlayOrder - b.PlayOrder })
	return sorted
}

func entryRows(entries []api.FlowsheetEntry) [][]string {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		artist, track := e.ArtistName, e.TrackTitle
		switch e.EntryType {
		case "track":
		case "spotlight":
			// Spotlight rows carry who is on air instead of a DJ name.
			artist = spotlightLabel(e.SpotlightName, e.SpotlightRole)
		default:
			// Marker rows (show_start/end, dj_join/leave) carry the DJ name.
			artist = e.DJName
		}
		rows = append(rows, []string{e.EntryType, artist, track, e.AlbumTitle})
	}
	return rows
}
