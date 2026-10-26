# Project Instructions

Twisty Quiz (`quiz-v2`) is a server-rendered school quiz system. It is a Go monolith with an Indonesian UI, not a JSON API plus SPA.

## Stack

- Go 1.26.4, Fiber v3.6.1, `html/template`, MariaDB 12.3.
- Raw `database/sql`; there is no ORM and no query builder.
- Vendored Bootstrap 6.0.0-alpha.1, jQuery 4, and Gotham Rounded. No frontend build step.
- `static/js/app.js` is global behavior and must stay network-free. `static/js/monitor.js` (SSE) and `static/js/beacon.js` (focus beacon) are the only network modules.

## Commands

```sh
gofmt -w <changed .go files>
go vet ./...
go build ./...
go test ./... -count=1
```

Format only files you touch. On a Windows checkout with `core.autocrlf=true`, `gofmt -l .` flags unchanged CRLF files; do not mass-rewrite line endings.

Database integration tests skip when `TEST_DB_DSN` is unset. To run them against the compose database:

```sh
TEST_DB_DSN='quiz:quiz@tcp(127.0.0.1:3306)/quiz?parseTime=true&loc=UTC' go test ./... -count=1
```

```sh
go run ./cmd/migrate                      # apply pending migrations
go run ./cmd/migrate bootstrap-teacher    # migrate + create first teacher
go run ./cmd/migrate hash                 # hash a password from stdin
```

Full stack: copy `.env.example` to `.env`, fill every empty value, then `docker compose up --build`. The app listens on `:8080`; health is `/healthz`. Docker runs migrations and bootstrap automatically. A local `go run ./cmd/server` does not load `.env`: export `SESSION_SIGNING_KEY` (32+ bytes) and a `DB_DSN` whose host is `127.0.0.1`.

## Architecture

- `cmd/server` starts the HTTP process; `cmd/migrate` is the only place DDL runs.
- `internal/httpserver` owns the Fiber app, route registry, request pipeline, rendering, and error mapping.
- Domain packages: `identity` (accounts/roles), `authoring` (quiz/question lifecycle), `participation` (join/eligibility), `attempts` (autosave/grading/results/SSE).
- Supporting packages: `authn`, `secure`, `store`, `viewmodel`, `config`, `migrate`.
- `templates/` is SSR, `static/` is vendored assets and JS, `migrations/` is embedded forward-only SQL.

Request lifecycle: Fiber → `serveRoute` → `prepare` (body cap → session → CSRF → must-change guard → flash → auth class) → `Load` / `Act` / `ActForm` / `HandlerCtx` → typed page data or `*apperr.Error` → HTTP layer renders the page, 422 form, error page, or 303 redirect.

## Non-Negotiable Rules

1. **Routes:** add or extend a `internal/httpserver/routes_<group>.go` file, register from `init()`, add the exact method+path row to `authz.go`, and update the pinned surface in `conformance_test.go`. Never edit `server.go` to add a page. A missing auth class fails startup.
2. **Handler seam:** page handlers receive `*Ctx`, never Fiber, and never write status codes. GET uses `Load`; POST uses `Act` or `ActForm`; SSE/beacon use `HandlerCtx`. Return `*apperr.Error` through the shared taxonomy.
3. **Authorization:** signed-out users are redirected to `/login` (the SSE stream returns an empty 404); once authenticated, missing, wrong-role, and foreign-owner resources all answer 404. 403 is only CSRF failure or a pending teacher. Do not leak existence or ownership.
4. **Database:** keep SQL in the owning domain package. Wrap writes in `store.Tx`; use `store.TxRetry` only when the operation is idempotent. Use `SELECT UTC_TIMESTAMP()` for writes and `deleted_at = store.LiveAt` for live rows. Scores are stored 0–1 and displayed 0–100.
5. **Copy and errors:** user-facing copy is Indonesian. Never render raw driver or internal errors. Cite the relevant spec rule ID on typed errors when one applies.
6. **Authority:** business behavior is defined by `docs/superpowers/specs/2026-10-23-business-logic-00-contract.md` and specs 01–05. Visual rules are in `docs/DESIGN.md`. Code and conformance tests win over stale docs or comments; the current route pin is 77 entries in `conformance_test.go`.
7. **Frontend:** use Bootstrap 6 and DESIGN tokens only. No inline event handlers, arbitrary hex colours, or native `confirm`/`alert`/`prompt`. Every POST form includes `csrf_token`. `app.js` must not use `fetch`, `XMLHttpRequest`, SSE, WebSockets, or `sendBeacon`; attempt autosave uses hidden-iframe form POSTs.
8. **Security:** every POST is CSRF-checked except `POST /attempt/:id/beacon`. Preserve body caps and the must-change-password guard. The session signing key is mandatory and checked at startup.

## Testing

- Tests live beside source as `*_test.go`; HTTP tests use `httptest` through `app.Test`.
- Integration tests skip without `TEST_DB_DSN`.
- `internal/httpserver/conformance_test.go` renders every page and enforces route, template, palette, accessibility, and JS wiring rules.
- There is no CI workflow and no browser/e2e suite. Format touched files, then run `go vet ./...`, `go build ./...`, and `go test ./... -count=1` before declaring work done.

## Where to Change Things

| Task | Location |
|---|---|
| Add/change a page or action | `internal/httpserver/routes_*.go` + `authz.go` |
| Business rule or validation | Relevant spec in `docs/superpowers/specs/` + owning domain package |
| Schema change | New `migrations/000N_*.up.sql`; never edit an applied migration |
| Visual or component change | `docs/DESIGN.md`, `static/css/tokens.css`, `templates/` |
| Request/error plumbing | `internal/httpserver/pipeline.go`, `routes.go`, `respond.go` |
| Error copy or status mapping | `internal/apperr/apperr.go` |
| Deployment/config | `.env.example`, `docker-compose.yml`, `Dockerfile`, `internal/config` |
