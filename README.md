# wxyc-cli

A read-only-by-default CLI for the [WXYC backend API](https://api.wxyc.org),
designed to be driven by both humans and agents. Authenticates with your
dj.wxyc.org credentials.

## Install

```sh
go build -o wxyc ./cmd/wxyc
# or: go install github.com/WXYC/wxyc-cli/cmd/wxyc@latest
```

## Quick start

```sh
wxyc login                       # prompts for email/username + password (hidden)
wxyc whoami                      # shows your identity and role

wxyc library search --artist "aphex twin"
wxyc library show 45029           # album detail + play counts
wxyc library artist 812          # artist card + releases with shelf codes
wxyc library genres
wxyc library formats
wxyc library rotation            # --json keeps the full nested shape
wxyc flowsheet tail -n 20
wxyc reviews for 45029           # in-app reviews + form archive for an album
wxyc labels search "june appal"
wxyc labels list
wxyc schedule
wxyc bin list
wxyc djs playlists               # your past shows; pass a dj_id for another DJ
wxyc djs playlists --full        # every track of every one of your past shows
```

Add `--json` to any command for machine-readable output. The rule is uniform:
**the table is a curated projection; `--json` is the API's response verbatim**
(byte-for-byte, including fields the table doesn't show), so an agent never
misses a field the CLI didn't bother to model.

## Global flags

These persistent flags work on every command:

| Flag | Env | Purpose |
|------|-----|---------|
| `--profile <name>` | `WXYC_PROFILE` | credential profile / keychain namespace (default `default`) |
| `--json` | — | emit the API response verbatim instead of a table |
| `--write` | `WXYC_ALLOW_WRITE=1` | unlock mutating commands (see [Read-only by default](#read-only-by-default)) |

## Commands

Commands marked **[write]** mutate station state and require `--write` (or
`WXYC_ALLOW_WRITE=1`); everything else is read-only.

### Auth

| Command | Description |
|---------|-------------|
| `wxyc login` | Prompt for email/username + password (hidden) and store a session token. The password is never persisted. |
| `wxyc whoami` | Show the identity, role, and expiry of the current token. |

### `library` — music library catalog

| Command | Description |
|---------|-------------|
| `wxyc library search [--artist <name>] [--album <title>] [--on-streaming[=false]] [-n <limit>]` | Search the catalog by artist, album, and/or streaming availability. The `ARTIST_ID` column feeds `library artist`. |
| `wxyc library show <album_id>` | Show an album's catalog record and its flowsheet plays by arm (direct, rotation, legacy; never summed). `--json` emits `{"album": <GET /library/info>, "play_counts": <GET /library/{id}/flowsheet-play-counts>}`, both bodies verbatim. |
| `wxyc library artist <artist_id> [--genre <id>] [--page N] [-n <limit>]` | Show an artist's card (genre, shelf code, release and cross-reference counts), then a page of their releases with shelf codes. `--genre` scopes both to one genre; `--page` is zero-based. `--json` emits `{"artist": <GET /library/artists/{id}>, "releases": <GET /library/artists/{id}/releases>}`, both bodies verbatim. |
| `wxyc library genres` | List the genre catalog. |
| `wxyc library formats` | List the media formats. |
| `wxyc library rotation` | Show the current rotation. `--json` keeps the full nested shape. |

### `flowsheet` — the on-air log

| Command | Description |
|---------|-------------|
| `wxyc flowsheet now` | Show what's on air right now: status (`live`, `Auto DJ` on automation, or `unknown` when the server does not say), the DJs on air, and while live the show name, specialty program, request-line mode and its source, and the current spotlight; then the last track played. `--json` prints `{"flowsheet": <GET /flowsheet>, "djs_on_air": <GET /flowsheet/djs-on-air>}`, both server bodies verbatim. |
| `wxyc flowsheet tail [-n <limit>]` | Show the most recent entries (default 10). |
| `wxyc flowsheet show <show_id>` | Show every entry of one past or current show, in play order. Show ids come from `djs playlists`. |
| `wxyc flowsheet search [query] [-n <limit>] [--page <n>] [--sort date\|artist\|song\|dj] [--order asc\|desc] [--cursor <token>]` | Search the playlist archive by artist, track, album, label or DJ. With no query, lists the most recent tracks. `--limit` is 1 to 100 (default 50). Table shows date, DJ, artist, track and album, then a paging footer; date-sorted full pages print a `--cursor` token for the next page. |
| `wxyc flowsheet range --since <time> [--until <time>]` | Show every entry logged in a time window, across shows, in play order and labeled by show. Times are RFC 3339, a local date and time (`2026-11-20 17:00`), a date (meaning that whole local day), `today`, `yesterday` or `now`. Without `--until` the window runs to the end of `--since`'s day if it is a date, otherwise to now. The window must end after it starts and be at most 8 days; a bad window is refused before any request. `--json` prints the server's `{shows, entries}` body. |
| `wxyc flowsheet recent [--hours N]` | Show who was on recently: shows that started in the last N hours (1–168, default 24), newest first, with start, end (`on air` while open), show name and DJ handles (`?` for a hidden handle). Show ids feed `flowsheet show`. An out-of-range `--hours` is refused before any request. `--json` prints the server's `{shows}` body. |
| `wxyc flowsheet start [--name <s>] [--as <s>] [--specialty <slug>]` | **[write]** Start your show, or join the active show as co-host. `--specialty` takes a specialty show's slug (e.g. `new-science-experience`); a numeric id is still accepted. |
| `wxyc flowsheet add --track <t> [--artist <a>] [--album <a>] [--label <l>] [--album-id <id>] [--rotation-id <id>] [--segue] [--request]` | **[write]** Add a played track. `--track` is required; `--artist`/`--album` are required unless `--album-id` is given (the server backfills from it). |
| `wxyc flowsheet talkset` | **[write]** Log a talkset. |
| `wxyc flowsheet breakpoint` | **[write]** Log a breakpoint (top of hour). |
| `wxyc flowsheet spotlight --role <role> <name>` | **[write]** Record who is on air in the open show. `--role` is one of `performer`, `dj`, `reader`, `guest`, `host`; the name is 1 to 128 characters. The name is public: it shows in the listener apps, on the Lock Screen and in the public playlist archive, so a `dj` entry takes the name the DJ uses on air. |
| `wxyc flowsheet request-line <mode>` | **[write]** Choose what the listener request line is for during the open show: `requests`, `feedback` (Messages only), `closed`, or `default` (back to the show's usual setting). The choice lasts for this show only. Prints the effective mode and its source; `--json` prints the server's `on_air` object. |
| `wxyc flowsheet move <entry_id> <new_position>` | **[write]** Reorder an entry to a new 1-based position. |
| `wxyc flowsheet edit <entry_id> [field flags…]` | **[write]** Edit fields of an entry. Accepts the same field flags as `add` plus `--message` (for marker rows); only flags you pass are changed, and `--flag ""` clears a field. |
| `wxyc flowsheet rm <entry_id>` | **[write]** Remove an entry. |
| `wxyc flowsheet end` | **[write]** End your show, or leave the active show as co-host. |

### `bin` — the DJ mail bin

| Command | Description |
|---------|-------------|
| `wxyc bin list` | List albums in your bin. |
| `wxyc bin add <album_id>` | **[write]** Add an album to your bin. |
| `wxyc bin rm <album_id>` | **[write]** Remove an album from your bin. Exits 5 if it wasn't in the bin. |

### `djs` — DJ playlists

| Command | Description |
|---------|-------------|
| `wxyc djs playlists [dj_id] [--full]` | List a DJ's past shows. Defaults to your own (read from the token). Each show carries only a short preview; `--full` fetches every show's complete track list (one request per show) and lists one row per track — with `--json`, an array of full show bodies. |

### `labels` — record labels

| Command | Description |
|---------|-------------|
| `wxyc labels list` | List all record labels. |
| `wxyc labels search <query> [-n <limit>]` | Search labels by name (default 10 results). |
| `wxyc labels show <id>` | Show a label's name, parent label id, child label count and how many library, rotation, flowsheet and intake rows reference it. `--json` prints the server's body verbatim. A missing label exits 5. |

### `reviews` — album reviews

Read-only. Authors and reviewers can be legal names: keep them inside the station.

| Command | Description |
|---------|-------------|
| `wxyc reviews list [--album <id>] [--intake <id>] [--mine]` | List the in-app reviews you may see (filters combine with AND). The REVIEW column is an excerpt; `--json` carries every review whole. With `--album`, cover reviews come first and are marked in a COVER column. |
| `wxyc reviews show <id> [--revisions]` | Show one review's details and its full text (buzzwords, artist blurb, review, recommended tracks, FCC notes). `--revisions` adds the edit history. A draft you may not see exits 5. `--json` prints the body verbatim; with `--revisions`, `{"review": …, "revisions": …}`. |
| `wxyc reviews archive [--album <id>] [--artist <name>] [--page N] [-n N]` | List the Google Form review archive, newest first (pages from 1; 1–100 per page, default 50). `--artist` ignores case and a leading "the". |
| `wxyc reviews for <album_id>` | An album's in-app reviews and archived form reviews together. `--json` emits `{"reviews": <GET /reviews?album_id=>, "archive": <GET /album-reviews?album_id=>}`. |

### `schedule`

| Command | Description |
|---------|-------------|
| `wxyc schedule` | Show the recurring show schedule. |

## Auth model

Your **password is never stored**. `login` exchanges it once for a long-lived
*session token* (kept in the OS keychain, or a `0600` file if no keychain is
available). Every request transparently exchanges that session token for a
short-lived signed JWT and refreshes on expiry. This mirrors the two-call
handshake the website uses: `POST /auth/sign-in/*` → `GET /auth/token`.

An agent or CI job that manages its own token can skip the keychain entirely by
exporting a JWT directly:

```sh
export WXYC_JWT="$(...)"          # used verbatim; no login/keychain needed
```

## Read-only by default

Mutating commands (`bin add`, `bin rm` and the `flowsheet` write commands) are blocked
unless you explicitly unlock writes:

```sh
wxyc bin add 45029               # exits 2 (blocked)
wxyc bin add 45029 --write       # permitted
WXYC_ALLOW_WRITE=1 wxyc bin add 45029   # permitted (scripted)
```

This is a client-side guard *in addition to* the server's own role check, so an
agent can't mutate station state by accident. Your `dj` role also can't perform
catalog writes regardless — the server returns 403.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | success |
| 1 | unclassified error |
| 2 | write blocked (read-only; pass `--write`) |
| 3 | not authenticated (no session, or 401) |
| 4 | forbidden (authenticated, but role lacks permission) |
| 5 | not found |
| 130 | interrupted (Ctrl-C) |

## Configuration

| Env var | Default | Purpose |
|---------|---------|---------|
| `WXYC_API_URL` | `https://api.wxyc.org` | backend REST base |
| `WXYC_AUTH_URL` | `<API>/auth` | better-auth base |
| `WXYC_PROFILE` | `default` | credential profile (keychain namespace) |
| `WXYC_JWT` | — | supply a JWT directly, bypassing login |
| `WXYC_ALLOW_WRITE` | — | `1` unlocks mutating commands |

## Roadmap

- **v2 auth:** device-authorization (QR) sign-in — no password in the CLI at
  all; approve from the iOS app. The `SignInStrategy` interface is already in
  place for it.
- OIDC + PKCE as a registered trusted client.
- Remaining read surface (genre detail).

## Development

```sh
go test ./...        # red-green TDD throughout
go vet ./...
```

Layout: `cmd/` (cobra commands) → `internal/api` (typed client),
`internal/auth` (sign-in, token exchange, transport, session store),
`internal/safety` (write gate), `internal/output` (renderers),
`internal/config`.
