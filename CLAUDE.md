# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`wxyc` is a read-only-by-default CLI for the WXYC backend API
(https://api.wxyc.org), designed to be driven by both humans and agents. It
authenticates with dj.wxyc.org credentials. The module is
`github.com/WXYC/wxyc-cli`; the built binary is `wxyc`.

The backend service this CLI talks to is a separate repo:
<https://github.com/WXYC/Backend-Service>. Its source is the best source of
truth for the API — endpoints, request/response shapes, auth semantics,
error codes — aside from the live production server itself. Check it when
the API's behavior is unclear or undocumented here.

## Commands

```sh
go build -o wxyc ./cmd/wxyc   # build
go install github.com/WXYC/wxyc-cli/cmd/wxyc@latest  # or install directly
go test ./...                 # run all tests (red-green TDD throughout)
go test ./internal/api/...    # test a single package
go test ./... -run TestBin_Add_PostsAlbumID  # run a single test
go vet ./...
```

There is no separate lint config beyond `go vet`.

## Architecture

Layered, one direction of dependency: `cmd/` (cobra commands) →
`internal/api` (typed HTTP client) → `internal/auth` (sign-in, token
exchange, transport, session store). `internal/safety` (write gate) and
`internal/output` (renderers) are used by `cmd/` alongside `internal/api`.
`internal/config` resolves environment-derived settings for all of the above.

- **`cmd/wxyc/main.go`** — the actual `main()`, just calls `cmd.Execute()`.
- **`cmd/app.go`** — `App` is the per-invocation dependency bag (config, gate,
  renderer, auth store/provider, API client). `App.build()` wires it all up
  from resolved flags/env inside the root command's `PersistentPreRunE`, so
  every subcommand sees the same instances. This is the place to look when
  tracing how a flag or env var actually reaches a command.
- **`cmd/root.go`** — builds the cobra command tree and, critically, runs the
  write gate (`app.gate.Authorize`) before *any* command executes, based on
  two annotations a mutating command must set on itself:
  `Annotations: map[string]string{"mutates": "true", "op": "<cmd>:<verb>"}`
  (see `cmd/bin.go`'s `bin add` for the pattern). Read commands need no
  annotation.
- **`cmd/exit.go`** — maps errors to process exit codes (`mapExit`). This is
  the CLI's machine-readable contract for scripts/agents: 0 ok, 1
  unclassified, 2 write blocked, 3 unauthenticated, 4 forbidden, 5 not found.
  Any new error type that should surface as its own exit code gets mapped
  here via `errors.As`/`errors.Is`.
- **`internal/api`** — `Client` wraps `http.Client` (which already carries the
  auth `Transport`, so this layer never touches tokens itself). Two access
  patterns per read endpoint, both used together: `get`/`getInto` decode into
  a typed struct for the table view, while `getRaw` returns the response body
  verbatim for `--json` passthrough — the same request backs both, so the two
  views can't drift and no server field is ever silently dropped. `post` is
  used by mutating endpoints (e.g. `BinAdd`). Non-2xx responses become
  `StatusError{Code, Path, Body}`.
- **`internal/auth`** — `Store` (interface) persists only a long-lived
  *session* token, never the password: `KeyringStore` (OS keychain) with
  `FileStore` (0600 file) fallback when no keychain is usable
  (`NewStore()` probes and picks). `TokenProvider` exchanges that session
  token for short-lived signed JWTs on demand and refreshes near expiry.
  `Transport` (an `http.RoundTripper`) injects the bearer JWT on every
  request and retries exactly once on a 401 after calling `Refresh` — this is
  where token lifecycle bugs tend to hide, and `rewind`/`GetBody` handling
  matters for any request with a body. A `WXYC_JWT` env var bypasses the
  whole session/provider path (see `App.build`) for agents/CI managing their
  own tokens.
- **`internal/safety`** — `Gate.Authorize(op, mutates)` is the entire write
  gate: reads always pass; writes require `AllowWrite` (from `--write` or
  `WXYC_ALLOW_WRITE=1`), else a `BlockedError` naming `op`. This is a
  client-side guard *in addition to* the server's own role check (dj role
  gets 403 on catalog writes regardless) — don't treat it as the only
  enforcement layer when reasoning about safety.
- **`internal/output`** — `Renderer.Emit` vs `Renderer.EmitRaw`: `Emit` JSON-encodes
  a Go value passed in by the command; `EmitRaw` instead re-indents and passes
  through the server's raw response bytes for `--json`, guaranteeing
  byte-for-byte fidelity even for fields the table projection doesn't model
  (e.g. `library rotation`'s deep/undocumented shape, decoded generically with
  `map[string]any` in `cmd/library.go`). Table mode truncates any cell over
  `maxCellWidth` (48 runes) so one long value can't blow out column widths for
  every row — this only affects the table, never `--json`.
- **`internal/config`** — env var resolution with defaults (`WXYC_API_URL`,
  derived `WXYC_AUTH_URL`, `WXYC_PROFILE`). Takes a `getenv func(string)
  string` for testability rather than reading `os.Getenv` directly.

## Conventions to preserve when adding commands

- **Read commands**: add a typed method on `api.Client` in `internal/api`
  returning `(T, []byte, error)` (decoded value + raw bytes), then in `cmd/`
  build a headers/rows projection and call `app.render.EmitRaw(raw, headers,
  rows)`. Never hand-model a field only for the table and skip it in
  `--json` — the raw bytes must always carry the full server response.
- **Mutating commands**: set both `annMutates`/`annOp` annotations, gate it
  with `--write`/`WXYC_ALLOW_WRITE=1` (already enforced centrally — don't
  re-check the gate inside `RunE`), and return a `StatusError`-compatible
  error path so `mapExit` classifies failures correctly.
- **Tests**: colocated `_test.go` per package, using `httptest` servers for
  HTTP-touching code (see `internal/api/*_test.go`,
  `internal/auth/transport_test.go`) and an in-memory `runCLI` helper in
  `cmd/root_test.go` that builds a fresh `App`/root command per test rather
  than a shared global. Table-driven where useful but not forced.
- **Red-green TDD, always** — including bug fixes. Write (or amend) the test
  first, run it, and confirm it *fails for the right reason* (red) before
  touching the code that makes it pass (green). Never write the fix first and
  backfill the test — a test you didn't watch fail proves nothing. For a bug,
  the failing test should reproduce the bug via a fixture that mirrors the real
  server response (e.g. the actual JSON key names), so it would have caught the
  original defect.
