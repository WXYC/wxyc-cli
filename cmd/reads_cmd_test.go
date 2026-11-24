package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// readServer serves body for exactly GET wantPath?wantQuery and points the
// CLI at it with a static JWT.
func readServer(t *testing.T, wantPath, wantQuery, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != wantPath || r.URL.RawQuery != wantQuery {
			t.Errorf("got %s %s, want GET %s?%s", r.Method, r.URL, wantPath, wantQuery)
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WXYC_API_URL", srv.URL)
	t.Setenv("WXYC_JWT", "unused")
}

// tableLines splits table output into lines with runs of spaces collapsed,
// so assertions don't depend on column padding.
func tableLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		lines = append(lines, strings.Join(strings.Fields(l), " "))
	}
	return lines
}

func assertTable(t *testing.T, out string, want ...string) {
	t.Helper()
	got := tableLines(out)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("table =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// assertJSONPassthrough checks --json output carries every server field,
// including ones the table never shows.
func assertJSONPassthrough(t *testing.T, out, body string) {
	t.Helper()
	var got, want any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("--json output isn't JSON: %v\n%s", err, out)
	}
	json.Unmarshal([]byte(body), &want)
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(want)
	if string(g) != string(w) {
		t.Errorf("--json = %s, want server body %s", g, w)
	}
}

func TestReadCommands(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		path      string
		query     string
		body      string
		wantTable []string
	}{
		{
			name:      "bin list",
			args:      []string{"bin", "list"},
			path:      "/djs/bin",
			body:      `[{"album_id":45029,"album_title":"Wax","artist_name":"Dave","label":"Blue Note","code_letters":"DA"}]`,
			wantTable: []string{"ALBUM_ID ARTIST ALBUM", "45029 Dave Wax"},
		},
		{
			name:      "labels list",
			args:      []string{"labels", "list"},
			path:      "/labels",
			body:      `[{"id":7,"label_name":"Merge Records","parent_label_id":null},{"id":8,"label_name":"Sub Pop","parent_label_id":7}]`,
			wantTable: []string{"ID LABEL", "7 Merge Records", "8 Sub Pop"},
		},
		{
			name:      "labels search",
			args:      []string{"labels", "search", "merge rec", "-n", "3"},
			path:      "/labels/search",
			query:     "limit=3&q=merge+rec",
			body:      `[{"id":7,"label_name":"Merge Records","parent_label_id":null}]`,
			wantTable: []string{"ID LABEL", "7 Merge Records"},
		},
		{
			name:      "library genres",
			args:      []string{"library", "genres"},
			path:      "/library/genres",
			body:      `[{"id":3,"genre_name":"Jazz","description":null,"plays":0,"add_date":"2001-01-01"}]`,
			wantTable: []string{"ID GENRE", "3 Jazz"},
		},
		{
			name:      "library formats",
			args:      []string{"library", "formats"},
			path:      "/library/formats",
			body:      `[{"id":1,"format_name":"vinyl","date_added":"2001-01-01"}]`,
			wantTable: []string{"ID FORMAT", "1 vinyl"},
		},
		{
			name: "schedule",
			args: []string{"schedule"},
			path: "/schedule",
			body: `[{"id":1,"day":0,"start_time":"20:00:00","show_duration":120,"specialty_id":null,"assigned_dj_id":"u1","assigned_dj_id2":null},
			        {"id":2,"day":6,"start_time":"06:00:00","show_duration":60,"specialty_id":4,"assigned_dj_id":null,"assigned_dj_id2":null},
			        {"id":3,"day":9,"start_time":"00:00:00","show_duration":30,"specialty_id":null,"assigned_dj_id":null,"assigned_dj_id2":null}]`,
			// An out-of-range day falls back to its number rather than panicking.
			wantTable: []string{"DAY START DURATION", "Sun 20:00:00 120m", "Sat 06:00:00 60m", "9 00:00:00 30m"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/table", func(t *testing.T) {
			readServer(t, tc.path, tc.query, tc.body)
			out, err := runCLI(t, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			assertTable(t, out, tc.wantTable...)
		})
		t.Run(tc.name+"/json", func(t *testing.T) {
			readServer(t, tc.path, tc.query, tc.body)
			out, err := runCLI(t, append(tc.args, "--json")...)
			if err != nil {
				t.Fatal(err)
			}
			assertJSONPassthrough(t, out, tc.body)
		})
	}
}

// claimsJWT builds an unsigned token carrying the given claims.
func claimsJWT(claims string) string {
	return "h." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".s"
}

const whoamiClaims = `{"sub":"u_abc","email":"dj@wxyc.org","role":"dj","exp":1767225600}`

func TestWhoami_Table(t *testing.T) {
	t.Setenv("WXYC_JWT", claimsJWT(whoamiClaims))

	out, err := runCLI(t, "whoami")
	if err != nil {
		t.Fatal(err)
	}
	assertTable(t, out,
		"FIELD VALUE",
		"sub u_abc",
		"email dj@wxyc.org",
		"role dj",
		"expires 2026-01-01T00:00:00Z",
	)
}

func TestWhoami_JSON(t *testing.T) {
	t.Setenv("WXYC_JWT", claimsJWT(whoamiClaims))

	out, err := runCLI(t, "whoami", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("--json isn't a JSON object: %v\n%s", err, out)
	}
	want := map[string]string{"sub": "u_abc", "email": "dj@wxyc.org", "role": "dj", "expires": "2026-01-01T00:00:00Z"}
	if len(got) != len(want) {
		t.Errorf("--json = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("--json[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestWhoami_MalformedTokenIsAnError(t *testing.T) {
	t.Setenv("WXYC_JWT", "not-a-jwt")

	_, err := runCLI(t, "whoami")
	if want := "malformed JWT: expected 3 segments, got 1"; err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}
