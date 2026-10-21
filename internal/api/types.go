package api

import "fmt"

// Album is a row from the library catalog. Fields mirror the backend's
// /library response; only the subset the CLI renders is modeled.
type Album struct {
	ID               int    `json:"id"`
	CodeLetters      string `json:"code_letters"`
	CodeArtistNumber int    `json:"code_artist_number"`
	CodeNumber       int    `json:"code_number"`
	ArtistName       string `json:"artist_name"`
	AlphabeticalName string `json:"alphabetical_name"`
	AlbumTitle       string `json:"album_title"`
	Format           string `json:"format_name"`
	GenreName        string `json:"genre_name"`
	Label            string `json:"label"`
}

// ShelfCode renders the album's physical filing location, e.g. "Jazz DA 11/4"
// (the 4th release by the 11th artist filed under "DA" in the Jazz section).
// The genre names the section; code_letters/code_artist_number/code_number
// locate the record within it. Returns "" when the album has no code letters,
// i.e. no shelf assignment.
func (a Album) ShelfCode() string {
	if a.CodeLetters == "" {
		return ""
	}
	code := fmt.Sprintf("%s %d/%d", a.CodeLetters, a.CodeArtistNumber, a.CodeNumber)
	if a.GenreName == "" {
		return code
	}
	return a.GenreName + " " + code
}

// FlowsheetEntry is one row of the on-air log. A row is either a played track
// (entry_type "track") or a marker (show_start, show_end, dj_join, …). Marker
// rows leave the track fields empty and carry dj_name.
type FlowsheetEntry struct {
	ID          int    `json:"id"`
	ShowID      int    `json:"show_id"`
	PlayOrder   int    `json:"play_order"`
	EntryType   string `json:"entry_type"`
	ArtistName  string `json:"artist_name"`
	AlbumTitle  string `json:"album_title"`
	TrackTitle  string `json:"track_title"`
	RecordLabel string `json:"record_label"`
	DJName      string `json:"dj_name"`
	RequestFlag bool   `json:"request_flag"`
	Segue       bool   `json:"segue"`
	RotationID  *int   `json:"rotation_id"`
}

// flowsheetResponse is the envelope the /flowsheet endpoint wraps entries in.
type flowsheetResponse struct {
	Entries []FlowsheetEntry `json:"entries"`
}

// StartShowRequest is the POST /flowsheet/join body. DJID must match the
// authenticated user (the backend 403s otherwise); the CLI fills it from the
// token. When no show is active this starts one; when a show is live it adds
// the caller as a co-host. SpecialtyID/ShowName/DJNameOverride are optional.
type StartShowRequest struct {
	DJID           string `json:"dj_id"`
	ShowName       string `json:"show_name,omitempty"`
	SpecialtyID    *int   `json:"specialty_id,omitempty"`
	DJNameOverride string `json:"dj_name_override,omitempty"`
}

// FlowsheetTrack is the POST /flowsheet body for a played track. When AlbumID
// is set the backend backfills artist/album/label from the library, so the
// free-text fields may be empty; otherwise ArtistName, AlbumTitle, and
// TrackTitle are all required by the server.
type FlowsheetTrack struct {
	ArtistName  string `json:"artist_name,omitempty"`
	AlbumTitle  string `json:"album_title,omitempty"`
	TrackTitle  string `json:"track_title"`
	RecordLabel string `json:"record_label,omitempty"`
	AlbumID     *int   `json:"album_id,omitempty"`
	RotationID  *int   `json:"rotation_id,omitempty"`
	RequestFlag bool   `json:"request_flag,omitempty"`
	Segue       bool   `json:"segue,omitempty"`
}

// flowsheetMarker is the POST /flowsheet body for a non-track entry (talkset,
// breakpoint, message). EntryType is passed explicitly so a message whose text
// happens to contain "Talkset"/"Breakpoint" isn't misclassified by the
// server's content-based inference.
type flowsheetMarker struct {
	Message   string `json:"message"`
	EntryType string `json:"entry_type,omitempty"`
}

// ShowSession is the response to POST /flowsheet/join and /flowsheet/end.
// Starting or ending a show echoes the show row (ID set); joining or leaving as
// a co-host echoes the show_djs row (ShowID set) instead. EffectiveShowID
// reconciles the two.
type ShowSession struct {
	ID          int     `json:"id"`
	ShowID      int     `json:"show_id"`
	ShowName    string  `json:"show_name"`
	PrimaryDJID string  `json:"primary_dj_id"`
	StartTime   string  `json:"start_time"`
	EndTime     *string `json:"end_time"`
}

// EffectiveShowID returns the show id regardless of whether the response was a
// show row (ID) or a co-host show_djs row (ShowID).
func (s ShowSession) EffectiveShowID() int {
	if s.ID != 0 {
		return s.ID
	}
	return s.ShowID
}

// FlowsheetResult is the projected flowsheet row echoed by the mutating
// /flowsheet endpoints (POST/PATCH/DELETE). Track rows carry the artist/track
// fields; marker rows carry Message and leave them empty.
type FlowsheetResult struct {
	ID         int    `json:"id"`
	ShowID     int    `json:"show_id"`
	PlayOrder  int    `json:"play_order"`
	EntryType  string `json:"entry_type"`
	ArtistName string `json:"artist_name"`
	AlbumTitle string `json:"album_title"`
	TrackTitle string `json:"track_title"`
	Message    string `json:"message"`
}

// FlowsheetUpdateFields is the data payload for PATCH /flowsheet. Every field
// is a pointer so that only flags the caller actually changed are marshalled
// (omitempty on a pointer drops nil but keeps a set-to-zero value), letting the
// CLI distinguish "change this field" from "leave it alone". For the string
// fields this cleanly expresses a clear (a set-to-empty-string pointer sends
// ""); the int fields can only send 0, not null, so they can reassign an
// association but not clear one back to null. The backend allowlists exactly
// these keys in pickUpdateEntryFields.
type FlowsheetUpdateFields struct {
	ArtistName    *string `json:"artist_name,omitempty"`
	AlbumTitle    *string `json:"album_title,omitempty"`
	TrackTitle    *string `json:"track_title,omitempty"`
	TrackPosition *string `json:"track_position,omitempty"`
	RecordLabel   *string `json:"record_label,omitempty"`
	LabelID       *int    `json:"label_id,omitempty"`
	AlbumID       *int    `json:"album_id,omitempty"`
	RotationID    *int    `json:"rotation_id,omitempty"`
	RequestFlag   *bool   `json:"request_flag,omitempty"`
	Segue         *bool   `json:"segue,omitempty"`
	Message       *string `json:"message,omitempty"`
}

// flowsheetUpdateRequest is the PATCH /flowsheet body: the target entry id plus
// the allowlisted data fields to change.
type flowsheetUpdateRequest struct {
	EntryID int                   `json:"entry_id"`
	Data    FlowsheetUpdateFields `json:"data"`
}

// flowsheetMoveRequest is the PATCH /flowsheet/play-order body. NewPosition is
// 1-based.
type flowsheetMoveRequest struct {
	EntryID     int `json:"entry_id"`
	NewPosition int `json:"new_position"`
}

// flowsheetDeleteRequest is the DELETE /flowsheet body: the backend reads
// req.body.entry_id.
type flowsheetDeleteRequest struct {
	EntryID int `json:"entry_id"`
}

// BinItem is an album a DJ has saved to their bin (mailbox).
type BinItem struct {
	AlbumID     int    `json:"album_id"`
	AlbumTitle  string `json:"album_title"`
	ArtistName  string `json:"artist_name"`
	Label       string `json:"label"`
	CodeLetters string `json:"code_letters"`
}

// Genre is a row from the genres catalog.
type Genre struct {
	ID          int     `json:"id"`
	GenreName   string  `json:"genre_name"`
	Description *string `json:"description"`
	Plays       int     `json:"plays"`
	AddDate     string  `json:"add_date"`
}

// Format is a physical/media format (vinyl, cd, …).
type Format struct {
	ID         int    `json:"id"`
	FormatName string `json:"format_name"`
	DateAdded  string `json:"date_added"`
}

// Label is a record label.
type Label struct {
	ID            int    `json:"id"`
	LabelName     string `json:"label_name"`
	ParentLabelID *int   `json:"parent_label_id"`
}

// Playlist is one past show a DJ was on, as returned by /djs/playlists. The
// server caps Preview at the show's first few flowsheet entries (in play order,
// message rows excluded); there is no endpoint for a show's full track list.
type Playlist struct {
	Show          int              `json:"show"`
	ShowName      string           `json:"show_name"`
	Date          string           `json:"date"`
	DJs           []PlaylistDJ     `json:"djs"`
	SpecialtyShow string           `json:"specialty_show"`
	Preview       []FlowsheetEntry `json:"preview"`
}

// PlaylistDJ names one of the DJs credited on a show. DJName may be empty.
type PlaylistDJ struct {
	DJID   string `json:"dj_id"`
	DJName string `json:"dj_name"`
}

// ScheduleEntry is one recurring show slot. Day is 0-6.
type ScheduleEntry struct {
	ID            int     `json:"id"`
	Day           int     `json:"day"`
	StartTime     string  `json:"start_time"`
	ShowDuration  int     `json:"show_duration"`
	SpecialtyID   *int    `json:"specialty_id"`
	AssignedDJID  *string `json:"assigned_dj_id"`
	AssignedDJID2 *string `json:"assigned_dj_id2"`
}

// SpecialtyShow is one program in the public specialty-show catalog. Only the
// fields the CLI resolves slugs with are modeled; the rest pass through --json.
type SpecialtyShow struct {
	ID     int    `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}
