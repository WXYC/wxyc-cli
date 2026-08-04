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
wxyc library genres
wxyc library formats
wxyc library rotation            # --json keeps the full nested shape
wxyc flowsheet tail -n 20
wxyc labels search "june appal"
wxyc labels list
wxyc schedule
wxyc bin list
wxyc djs playlists               # your past shows; pass a dj_id for another DJ
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
| `wxyc library search [--artist <name>] [--album <title>] [-n <limit>]` | Search the catalog by artist and/or album. |
| `wxyc library genres` | List the genre catalog. |
| `wxyc library formats` | List the media formats. |
| `wxyc library rotation` | Show the current rotation. `--json` keeps the full nested shape. |

### `flowsheet` — the on-air log

| Command | Description |
|---------|-------------|
| `wxyc flowsheet tail [-n <limit>]` | Show the most recent entries (default 10). |
| `wxyc flowsheet start [--name <s>] [--as <s>] [--specialty <id>]` | **[write]** Start your show, or join the active show as co-host. |
| `wxyc flowsheet add --track <t> [--artist <a>] [--album <a>] [--label <l>] [--album-id <id>] [--rotation-id <id>] [--segue] [--request]` | **[write]** Add a played track. `--track` is required; `--artist`/`--album` are required unless `--album-id` is given (the server backfills from it). |
| `wxyc flowsheet talkset` | **[write]** Log a talkset. |
| `wxyc flowsheet breakpoint` | **[write]** Log a breakpoint (top of hour). |
| `wxyc flowsheet move <entry_id> <new_position>` | **[write]** Reorder an entry to a new 1-based position. |
| `wxyc flowsheet edit <entry_id> [field flags…]` | **[write]** Edit fields of an entry. Accepts the same field flags as `add` plus `--message` (for marker rows); only flags you pass are changed, and `--flag ""` clears a field. |
| `wxyc flowsheet rm <entry_id>` | **[write]** Remove an entry. |
| `wxyc flowsheet end` | **[write]** End your show, or leave the active show as co-host. |

### `bin` — the DJ mail bin

| Command | Description |
|---------|-------------|
| `wxyc bin list` | List albums in your bin. |
| `wxyc bin add <album_id>` | **[write]** Add an album to your bin. |

### `djs` — DJ playlists

| Command | Description |
|---------|-------------|
| `wxyc djs playlists [dj_id]` | List a DJ's past shows. Defaults to your own (read from the token). |

### `labels` — record labels

| Command | Description |
|---------|-------------|
| `wxyc labels list` | List all record labels. |
| `wxyc labels search <query> [-n <limit>]` | Search labels by name (default 10 results). |

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

Mutating commands (`bin add` and the `flowsheet` write commands) are blocked
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
- Remaining read surface (label/genre detail).

## Development

```sh
go test ./...        # red-green TDD throughout
go vet ./...
```

Layout: `cmd/` (cobra commands) → `internal/api` (typed client),
`internal/auth` (sign-in, token exchange, transport, session store),
`internal/safety` (write gate), `internal/output` (renderers),
`internal/config`.
