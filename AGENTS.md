# quiz-v2 — AGENTS.md

Go 1.26.4 monolith: Fiber v3 SSR, MariaDB 12.3, Bootstrap 6 + jQuery 4 vendored.
Module `quiz-v2`. Entry point `cmd/server/main.go`.

## Dev environment

- Go 1.26.4 (`go version`). No Node/npm needed — all JS is vendored under `static/`.
- Dependencies are in `go.mod`/`go.sum`; `go mod download` fetches them.
- Docker Compose spins up MariaDB + app: `docker-compose up` (app on `:8080`).
- Config is env-driven (`internal/config/config.go`): `APP_ADDR` (default `:8080`),
  `DB_DSN` (default `quiz:quiz@tcp(db:3306)/quiz?parseTime=true`),
  `COOKIE_SECURE`, `SESSION_SIGNING_KEY`, `APP_DEMO` (enables `?state=` previews and
  the demo strip). `.env` is git-ignored.

## Build & test

```
go build ./...          # builds the whole module
go run ./cmd/server     # runs on :8080 (DB must be up)
go test ./...           # all tests
go test ./internal/httpserver/...   # HTTP/server + template conformance
go test ./internal/config/...       # config env loading
go test ./internal/store/...        # store Open/Ping
```

Tests use Fiber's `app.Test(httptest.NewRequest(...))`. The `httpserver` package's
`TestMain` (in `server_test.go`) chdirs to the repo root before running, so
`./templates` and `./static` resolve correctly — do not change that.

## Conventions

**Adding a page** — never edit `server.go`. Create a new `internal/httpserver/routes_<group>.go`
file with an `init()` that calls `registerPage(PageRoute{Method, Path, Template, Nav, Build})`.
One file per route group; `server.go` iterates `RegisteredPages()` (sorted so static
paths beat parameterised ones) and registers a Fiber route per entry. `/Template` is
slash-relative to `templates/`. POST routes use `Action` (UI-only PRG redirect),
static redirects use `Redirect`; `Handler` exists for non-page endpoints (SSE, 204).
Every page builder returns a `viewmodel.PageData{Data: <typed page model>, ...}`;
templates read `.Data` and the promoted `.Base` chrome fields.

**Route groups** (72 routes total, pinned by `conformance_test.go`):
`routes_home`, `routes_error`, `routes_attempt`, `routes_auth_public`,
`routes_auth_account`, `routes_teacher_core`, `routes_teacher_authoring`,
`routes_teacher_grading`, `routes_teacher_mgmt`, `routes_student`.
The full inventory is `internal/httpserver/fixtures` (data) + `internal/viewmodel/pages.go`
(one struct per screen). `APP_DEMO` is the only switch for state previews; production
ignores `?state=` and never renders the demo strip.

**Templates** (`templates/`, parsed by `internal/httpserver/views.go`):
- Page files: `{{define "content"}}...{{end}}`, optionally overriding
  `{{define "partials/title"}}`.
- Layout files: `{{define "layouts/<name>.html"}}...{{template "content" .}}...{{end}}`.
- Partials: `{{define "partials/<name>.html"}}...{{end}}` — never define `content`.
- `partials/*-style.html` are raw CSS fragments (no markup), pulled into
  `partials/head.html`'s single `<style>` block: app-frame CSS first, shared
  `ui-style` components next, then the `partials/page-style` slot a page may
  override. Add every new partial to `pinnedPartials` in `partials_test.go`.
- Layout mapping: `pages/auth/*`→`auth`, `pages/teacher/*`→`teacher`,
  `pages/student/*`→`student`, `pages/profile.html`→`student`,
  `pages/attempt/*`→`base`, `pages/error/*`→`error`, else `base`.
- Shared chrome: `partials/flash.html` reads `.Flash`; `partials/demo-strip.html`
  reads `.Demo`; `partials/score.html` takes a score; `partials/quiz-card.html`
  takes a `viewmodel.QuizCard`; `partials/tabs.html` and `partials/pagination.html`
  take their typed view models.
- `partials/head.html` links `bootstrap.min.css` then `tokens.css`, exactly one
  `<style>` (the `@font-face` block). `partials/foot.html` order is fixed:
  `<noscript>` → bootstrap bundle (module) → jQuery (defer, classic) → app.js (module).

**Design contract** (`docs/DESIGN.md` is authoritative; conformance tests enforce it):
- Only the 24 palette hexes may appear anywhere. `#293681` is the single action
  colour; buttons are `btn-solid theme-primary`, never `btn-primary`.
- Bootstrap 6 + jQuery 4 + Gotham Rounded are vendored under `static/` — no CDN.
- No Bootstrap v5 class names, no icon fonts (use the SVG sprite in `partials/icons.html`),
  no `outline: none`, no `window.bootstrap`, no `data-bs-toggle="modal"`.
- Tables go in `.table-responsive`; scores/timers use `font-variant-numeric: tabular-nums`.
- `static/js/app.js` must not call `fetch`/`XMLHttpRequest`/`WebSocket`/etc.

**Store** (`internal/store/store.go`): `store.Open(dsn) → *sql.DB`, `store.Ping(ctx, db) error`.
`/healthz` pings the DB with a 2s timeout — 200 `"ok"` or 503 `"db unavailable"`.

## Pitfalls

- `internal/httpserver/routes_test.go` is in package `httpserver` (not `_test`).
  It must not call `registerPage`: that registry is package-global and a test
  registration leaks into every later test. Test route helpers directly instead.
- `conformance_test.go` pins exactly 72 method+path pairs and renders every GET
  page / POST action; `partials_test.go` pins the partial set and 51-icon sprite.
  Update both pins deliberately when the surface changes.
- Tests must run from the repo root (or rely on `TestMain`'s chdir). Running
  `go test` from a subdirectory breaks `./templates` resolution.
- `bin/`, `*.exe`, `*.test`, and `.env` are git-ignored. Don't commit built binaries.
- Docker build excludes `docs/`, `assets/`, `*.md`, `.env` — docs and prototypes are
  reference only, not shipped in the image.
- No migration files exist yet (`migrations/` is empty except `.gitkeep`); the DB
  schema is created by `docker-compose.yml`'s `MARIADB_DATABASE: quiz`.