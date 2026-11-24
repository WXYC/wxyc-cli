package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// Fixtures with the server's real key names (Backend-Service app.yaml:
// Review, ReviewRevision, AlbumReviewsResponse).
const (
	reviewLongText = "Their most polished record: motorik grooves, Moog drones and lounge-pop horns " +
		"stretched over seventy minutes.\nSide two drifts, but Brakhage alone earns a spin."
	reviewOne = `{"id":812,"album_id":45029,"intake_item_id":3307,` +
		`"author":"Jordan Example","author_user_id":"u_8f2c","recorded_by_user_id":null,` +
		`"medium":"typed","status":"submitted","add_date":"2026-09-14",` +
		`"submitted_at":"2026-09-15T02:11:40.000Z","last_modified":"2026-09-20T17:05:12.000Z",` +
		`"in_use":true,"on_cover":true,"printed_revision_id":1204,"printed_at":"2026-09-16T14:00:00.000Z",` +
		`"revision_count":2,` +
		`"buzzwords":"motorik, lounge, Moog","artist_blurb":"Anglo-French group led by Tim Gane and Laetitia Sadier.",` +
		`"review":"Their most polished record: motorik grooves, Moog drones and lounge-pop horns stretched over seventy minutes.\nSide two drifts, but Brakhage alone earns a spin.",` +
		`"recommended_tracks":"1!, 3!!, 7","fcc":"Track 4 has one f-word at 2:10.",` +
		`"publish_website":false,"publish_apps":false,"publish_instagram":false,"credit":null}`
	// A handwritten draft on an intake item not yet filed: no album, no text.
	reviewDraft = `{"id":815,"album_id":null,"intake_item_id":3390,` +
		`"author":"Riley Example","author_user_id":null,"recorded_by_user_id":"u_11aa",` +
		`"medium":"handwritten","status":"draft","add_date":"2026-10-02",` +
		`"submitted_at":null,"last_modified":"2026-10-02T15:00:00.000Z",` +
		`"in_use":false,"on_cover":false,"printed_revision_id":null,"printed_at":null,"revision_count":0,` +
		`"buzzwords":null,"artist_blurb":null,"review":null,"recommended_tracks":null,"fcc":null,` +
		`"publish_website":false,"publish_apps":false,"publish_instagram":false,"credit":null}`
	reviewRevisions = `[` +
		`{"id":1205,"review_id":812,"revision":2,"edited_by":"Pat Director","edited_by_user_id":"u_11aa",` +
		`"edited_at":"2026-09-20T17:05:12.000Z","review":"Their most polished record: motorik grooves, Moog drones and lounge-pop horns stretched over seventy minutes.",` +
		`"artist_blurb":null,"buzzwords":"motorik, lounge, Moog","recommended_tracks":"1!, 3!!, 7","fcc":null},` +
		`{"id":1204,"review_id":812,"revision":1,"edited_by":"Jordan Example","edited_by_user_id":"u_8f2c",` +
		`"edited_at":"2026-09-15T02:11:40.000Z","review":"Their most polished record.",` +
		`"artist_blurb":null,"buzzwords":null,"recommended_tracks":null,"fcc":null}]`
	albumReviewsPage = `{"album_reviews":[{"id":5521,"album_id":45029,"artist_name":"Stereolab",` +
		`"album_title":"Dots and Loops","record_label":"Duophonic","artist_blurb":null,` +
		`"review":"Great late-night record.","recommended_tracks":"Brakhage!","buzzwords":"lounge",` +
		`"fcc_violations":"None","review_purpose":"New addition","rotated":true,` +
		`"released_within_six_months":false,"social_consent":null,` +
		`"submitted_at":"2019-02-03T18:44:00.000Z","reviewer":"Sam Example 2/3/19"}],` +
		`"pagination":{"page":2,"limit":10,"total":31,"hasMore":true}}`
	// The first page of the same archive, as "reviews for" asks for it.
	albumReviewsFirstPage = `{"album_reviews":[{"id":5521,"album_id":45029,"artist_name":"Stereolab",` +
		`"album_title":"Dots and Loops","record_label":"Duophonic","artist_blurb":null,` +
		`"review":"Great late-night record.","recommended_tracks":"Brakhage!","buzzwords":"lounge",` +
		`"fcc_violations":"None","review_purpose":"New addition","rotated":true,` +
		`"released_within_six_months":false,"social_consent":null,` +
		`"submitted_at":"2019-02-03T18:44:00.000Z","reviewer":"Sam Example 2/3/19"}],` +
		`"pagination":{"page":1,"limit":100,"total":131,"hasMore":true}}`
	albumReviewsEmpty = `{"album_reviews":[],"pagination":{"page":1,"limit":50,"total":0,"hasMore":false}}`
)

// reviewsServer serves bodies by path and records each path's query string.
// A status other than 0 answers every request with it.
func reviewsServer(t *testing.T, bodies map[string]string, status int) map[string]string {
	t.Helper()
	var mu sync.Mutex
	queries := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries[r.URL.Path] = r.URL.RawQuery
		mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			w.Write([]byte(`{"message":"refused"}`))
			return
		}
		body, ok := bodies[r.URL.Path]
		if !ok {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
	return queries
}

// utcApp is a hermetic App whose local zone is UTC, so dates are fixed.
func utcApp(t *testing.T) *App {
	app := newTestApp(t)
	app.loc = time.UTC
	return app
}

func TestReviewsList_Table(t *testing.T) {
	reviewsServer(t, map[string]string{"/reviews": "[" + reviewOne + "," + reviewDraft + "]"}, 0)
	out, err := runApp(t, utcApp(t), "reviews", "list")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want header + 2 rows:\n%s", len(lines), out)
	}
	if got := strings.Fields(lines[0]); strings.Join(got, " ") != "ID ALBUM AUTHOR STATUS SUBMITTED REVIEW" {
		t.Errorf("header = %q", lines[0])
	}
	// The review is cut to the table's cell width, its line break flattened.
	excerpt := []rune(strings.ReplaceAll(reviewLongText, "\n", " "))
	wantCut := string(excerpt[:47]) + "…"
	for _, want := range []string{"812", "45029", "Jordan Example", "submitted", "2026-09-15", wantCut} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("row 1 missing %q:\n%s", want, lines[1])
		}
	}
	if strings.Contains(out, "Brakhage") {
		t.Errorf("table carries the whole review:\n%s", out)
	}
	// A review with no album yet names its intake item; a handwritten one
	// with no text says so.
	for _, want := range []string{"815", "item 3390", "Riley Example", "draft", "(handwritten)"} {
		if !strings.Contains(lines[2], want) {
			t.Errorf("row 2 missing %q:\n%s", want, lines[2])
		}
	}
}

func TestReviewsList_JSONCarriesWholeReview(t *testing.T) {
	body := "[" + reviewOne + "]"
	reviewsServer(t, map[string]string{"/reviews": body}, 0)
	out, err := runCLI(t, "reviews", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertSameJSON(t, "reviews", json.RawMessage(out), body)
}

func TestReviewsList_FiltersMapToQuery(t *testing.T) {
	cases := []struct {
		args []string
		want url.Values
	}{
		{nil, url.Values{}},
		{[]string{"--album", "45029"}, url.Values{"album_id": {"45029"}}},
		{[]string{"--intake", "3307"}, url.Values{"intake_item_id": {"3307"}}},
		{[]string{"--mine"}, url.Values{"mine": {"true"}}},
		// mine=false is the same as no filter, so it is not sent.
		{[]string{"--mine=false"}, url.Values{}},
		{[]string{"--album", "45029", "--intake", "3307", "--mine"},
			url.Values{"album_id": {"45029"}, "intake_item_id": {"3307"}, "mine": {"true"}}},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			q := reviewsServer(t, map[string]string{"/reviews": "[]"}, 0)
			if _, err := runCLI(t, append([]string{"reviews", "list"}, c.args...)...); err != nil {
				t.Fatal(err)
			}
			if got := q["/reviews"]; got != c.want.Encode() {
				t.Errorf("query = %q, want %q", got, c.want.Encode())
			}
		})
	}
}

func TestReviewsList_AlbumMarksCover(t *testing.T) {
	reviewsServer(t, map[string]string{"/reviews": "[" + reviewOne + "]"}, 0)
	out, err := runCLI(t, "reviews", "list", "--album", "45029")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out, "\n")
	if got := strings.Fields(lines[0]); got[len(got)-1] != "COVER" {
		t.Errorf("header = %q, want a COVER column", lines[0])
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[1]), "yes") {
		t.Errorf("row = %q, want on_cover shown as yes", lines[1])
	}
}

func TestReviewsList_RefusesBadIDsLocally(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--album", "0"}, "--album must be an id from 1 to 2147483647, got 0"},
		{[]string{"--album", "-4"}, "--album must be an id from 1 to 2147483647, got -4"},
		{[]string{"--album", "2147483648"}, "--album must be an id from 1 to 2147483647, got 2147483648"},
		{[]string{"--intake", "0"}, "--intake must be an id from 1 to 2147483647, got 0"},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			t.Setenv("WXYC_JWT", "unused")
			_, err := runCLI(t, append([]string{"reviews", "list"}, c.args...)...)
			if err == nil || err.Error() != c.want {
				t.Errorf("err = %v, want %q", err, c.want)
			}
		})
	}
}

func TestReviewsList_ForbiddenIsExitForbidden(t *testing.T) {
	reviewsServer(t, nil, http.StatusForbidden)
	_, err := runCLI(t, "reviews", "list")
	if got := mapExit(err); got != ExitForbidden {
		t.Errorf("exit = %d (err %v), want ExitForbidden(%d)", got, err, ExitForbidden)
	}
}

func TestReviewsShow_CardAndWholeText(t *testing.T) {
	reviewsServer(t, map[string]string{"/reviews/812": reviewOne}, 0)
	out, err := runApp(t, utcApp(t), "reviews", "show", "812")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"id":          "812",
		"album":       "45029",
		"intake item": "3307",
		"author":      "Jordan Example",
		"medium":      "typed",
		"status":      "submitted",
		"added":       "2026-09-14",
		"submitted":   "2026-09-15 02:11",
		"modified":    "2026-09-20 17:05",
		"printed":     "2026-09-16 14:00",
		"in use":      "true",
		"revisions":   "2",
	} {
		if got := nowField(t, out, name); got != want {
			t.Errorf("%s = %q, want %q\n%s", name, got, want, out)
		}
	}
	// Every text field is printed whole, line breaks kept, under a heading.
	for _, want := range []string{
		"\nbuzzwords:\n  motorik, lounge, Moog\n",
		"\nartist blurb:\n  Anglo-French group led by Tim Gane and Laetitia Sadier.\n",
		"\nreview:\n  " + strings.ReplaceAll(reviewLongText, "\n", "\n  ") + "\n",
		"\nrecommended tracks:\n  1!, 3!!, 7\n",
		"\nfcc:\n  Track 4 has one f-word at 2:10.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestReviewsShow_DraftOmitsEmptyFields(t *testing.T) {
	reviewsServer(t, map[string]string{"/reviews/815": reviewDraft}, 0)
	out, err := runCLI(t, "reviews", "show", "815")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"album", "submitted", "printed"} {
		if got := nowField(t, out, name); got != "" {
			t.Errorf("unexpected %s row = %q", name, got)
		}
	}
	for _, heading := range []string{"review:", "buzzwords:", "fcc:"} {
		if strings.Contains(out, "\n"+heading+"\n") {
			t.Errorf("unexpected empty %s section:\n%s", heading, out)
		}
	}
}

func TestReviewsShow_JSONIsTheReviewVerbatim(t *testing.T) {
	reviewsServer(t, map[string]string{"/reviews/812": reviewOne}, 0)
	out, err := runCLI(t, "reviews", "show", "812", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertSameJSON(t, "review", json.RawMessage(out), reviewOne)
}

func TestReviewsShow_Revisions(t *testing.T) {
	reviewsServer(t, map[string]string{"/reviews/812": reviewOne, "/reviews/812/revisions": reviewRevisions}, 0)
	out, err := runApp(t, utcApp(t), "reviews", "show", "812", "--revisions")
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(out, "REV  EDITED")
	if i < 0 {
		t.Fatalf("no revisions table:\n%s", out)
	}
	rows := strings.Split(strings.TrimRight(out[i:], "\n"), "\n")
	if len(rows) != 3 {
		t.Fatalf("revisions table has %d lines, want header + 2:\n%s", len(rows), out[i:])
	}
	for n, want := range [][]string{
		{"2", "2026-09-20 17:05", "Pat Director", "Their most polished record: motorik grooves, Mo…"},
		{"1", "2026-09-15 02:11", "Jordan Example", "Their most polished record."},
	} {
		for _, w := range want {
			if !strings.Contains(rows[n+1], w) {
				t.Errorf("revision row %d missing %q:\n%s", n+1, w, rows[n+1])
			}
		}
	}
}

func TestReviewsShow_RevisionsJSONWrapsBothBodies(t *testing.T) {
	reviewsServer(t, map[string]string{"/reviews/812": reviewOne, "/reviews/812/revisions": reviewRevisions}, 0)
	out, err := runCLI(t, "reviews", "show", "812", "--revisions", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Errorf("keys = %d, want review and revisions only:\n%s", len(got), out)
	}
	assertSameJSON(t, "review", got["review"], reviewOne)
	assertSameJSON(t, "revisions", got["revisions"], reviewRevisions)
}

func TestReviewsShow_HiddenDraftIsExitNotFound(t *testing.T) {
	reviewsServer(t, nil, http.StatusNotFound)
	_, err := runCLI(t, "reviews", "show", "815")
	if got := mapExit(err); got != ExitNotFound {
		t.Errorf("exit = %d (err %v), want ExitNotFound(%d)", got, err, ExitNotFound)
	}
}

func TestReviewsShow_RefusesBadID(t *testing.T) {
	for _, arg := range []string{"abc", "0", "-1", "2147483648"} {
		t.Run(arg, func(t *testing.T) {
			t.Setenv("WXYC_JWT", "unused")
			_, err := runCLI(t, "reviews", "show", "--", arg)
			want := `review id must be an integer from 1 to 2147483647, got "` + arg + `"`
			if err == nil || err.Error() != want {
				t.Errorf("err = %v, want %q", err, want)
			}
		})
	}
}

func TestReviewsArchive_TableAndPage(t *testing.T) {
	reviewsServer(t, map[string]string{"/album-reviews": albumReviewsPage}, 0)
	out, err := runApp(t, utcApp(t), "reviews", "archive")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out, "\n")
	if got := strings.Join(strings.Fields(lines[0]), " "); got != "ID ALBUM ARTIST TITLE REVIEWER SUBMITTED REVIEW" {
		t.Errorf("header = %q", lines[0])
	}
	for _, want := range []string{"5521", "45029", "Stereolab", "Dots and Loops", "Sam Example 2/3/19", "2019-02-03", "Great late-night record."} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("row missing %q:\n%s", want, lines[1])
		}
	}
	if !strings.Contains(out, "\npage 2 of 4 (31 reviews, 10 per page); next: --page 3\n") {
		t.Errorf("missing page line:\n%s", out)
	}
}

func TestReviewsArchive_Empty(t *testing.T) {
	reviewsServer(t, map[string]string{"/album-reviews": albumReviewsEmpty}, 0)
	out, err := runCLI(t, "reviews", "archive")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "no archived reviews\n") {
		t.Errorf("out = %q", out)
	}
}

func TestReviewsArchive_JSONVerbatim(t *testing.T) {
	reviewsServer(t, map[string]string{"/album-reviews": albumReviewsPage}, 0)
	out, err := runCLI(t, "reviews", "archive", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertSameJSON(t, "archive", json.RawMessage(out), albumReviewsPage)
}

func TestReviewsArchive_FiltersMapToQuery(t *testing.T) {
	q := reviewsServer(t, map[string]string{"/album-reviews": albumReviewsEmpty}, 0)
	if _, err := runCLI(t, "reviews", "archive", "--album", "45029", "--artist", "The Stereolab", "--page", "2", "--limit", "10"); err != nil {
		t.Fatal(err)
	}
	want := url.Values{"album_id": {"45029"}, "artist": {"The Stereolab"}, "page": {"2"}, "limit": {"10"}}.Encode()
	if got := q["/album-reviews"]; got != want {
		t.Errorf("query = %q, want %q", got, want)
	}
}

func TestReviewsArchive_RefusesBadInput(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--album", "0"}, "--album must be an id from 1 to 2147483647, got 0"},
		{[]string{"--artist", " "}, "--artist must not be blank"},
		{[]string{"--page", "0"}, "--page starts at 1, got 0"},
		{[]string{"--limit", "0"}, "--limit must be from 1 to 100, got 0"},
		{[]string{"--limit", "101"}, "--limit must be from 1 to 100, got 101"},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			t.Setenv("WXYC_JWT", "unused")
			_, err := runCLI(t, append([]string{"reviews", "archive"}, c.args...)...)
			if err == nil || err.Error() != c.want {
				t.Errorf("err = %v, want %q", err, c.want)
			}
		})
	}
}

func TestReviewsArchive_ForbiddenIsExitForbidden(t *testing.T) {
	reviewsServer(t, nil, http.StatusForbidden)
	_, err := runCLI(t, "reviews", "archive")
	if got := mapExit(err); got != ExitForbidden {
		t.Errorf("exit = %d (err %v), want ExitForbidden(%d)", got, err, ExitForbidden)
	}
}

func TestReviewsFor_QueriesBothStores(t *testing.T) {
	q := reviewsServer(t, map[string]string{
		"/reviews":       "[" + reviewOne + "]",
		"/album-reviews": albumReviewsFirstPage,
	}, 0)
	out, err := runApp(t, utcApp(t), "reviews", "for", "45029")
	if err != nil {
		t.Fatal(err)
	}
	if got := q["/reviews"]; got != "album_id=45029" {
		t.Errorf("/reviews query = %q", got)
	}
	if got, want := q["/album-reviews"], (url.Values{"album_id": {"45029"}, "limit": {"100"}}).Encode(); got != want {
		t.Errorf("/album-reviews query = %q, want %q", got, want)
	}
	for _, want := range []string{"in-app reviews:", "Jordan Example", "form archive:", "Sam Example 2/3/19", "more archived reviews: wxyc reviews archive --album 45029 --page 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestReviewsFor_EmptyStores(t *testing.T) {
	reviewsServer(t, map[string]string{"/reviews": "[]", "/album-reviews": albumReviewsEmpty}, 0)
	out, err := runCLI(t, "reviews", "for", "45029")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"no in-app reviews", "no archived reviews"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestReviewsFor_JSONWrapsBothBodies(t *testing.T) {
	reviewsServer(t, map[string]string{"/reviews": "[" + reviewOne + "]", "/album-reviews": albumReviewsPage}, 0)
	out, err := runCLI(t, "reviews", "for", "45029", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Errorf("keys = %d, want reviews and archive only:\n%s", len(got), out)
	}
	assertSameJSON(t, "reviews", got["reviews"], "["+reviewOne+"]")
	assertSameJSON(t, "archive", got["archive"], albumReviewsPage)
}

func TestReviewsFor_RefusesBadID(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	_, err := runCLI(t, "reviews", "for", "0")
	want := `album_id must be an integer from 1 to 2147483647, got "0"`
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}
