# Initial Framework (Hello World) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bootstrap the quiz-v2 Go monolith from an empty docs-only repo to a running "Hello World" web server, containerized with Docker, with MariaDB wired in via docker-compose.

**Architecture:** Single Go binary serving SSR pages with Fiber v3 and `html/template`, backed by MariaDB 12.3 through raw `database/sql`. The repo starts as docs-only (no git, no Go code); this plan creates the module, a layered server (`cmd/server` → `internal/config` → `internal/httpserver` → `internal/store`), a Dockerfile, and a compose stack (app + MariaDB). `GET /` renders an SSR page, `GET /healthz` reports app+DB readiness.

**Tech Stack:** Go 1.26 (local toolchain go1.26.4), Fiber v3.6.1, `html/template`, `github.com/go-sql-driver/mysql` v1.10.1, MariaDB 12.3 (Docker official image), Docker 29.4.3 + Compose v5.1.3.

**Spec:** `docs/superpowers/specs/2026-10-20-quiz-system-design.md`

## Global Constraints

- Stack per `docs/README.md:61`: "Go monolith · Fiber v3 · SSR (`html/template`) · MariaDB 12.3 · Docker Compose".
- Spec §11: "No ORM; raw SQL via database/sql." — driver is `github.com/go-sql-driver/mysql`, nothing else.
- Spec §11: migrations are "plain numbered SQL files in `/migrations`" — this plan creates the directory only; migration content is a later plan.
- Spec §14: "Single process, no cache tier, no JWT." "All pages server-rendered (SSR)".
- Spec §12: `COOKIE_SECURE` defaults to false; `SESSION_SIGNING_KEY` signs cookies — both read from env by config, no auth logic in this plan.
- Spec §15.10: UI copy is Indonesian; `<html lang="id">`; spec/plan artifacts stay in English.
- Spec §14: "No CDN in production." — nothing in this plan may reference an external origin from a rendered page.
- Dependencies pinned: `github.com/gofiber/fiber/v3 v3.6.1`, `github.com/go-sql-driver/mysql v1.10.1`. No other runtime deps (test framework is stdlib `testing` only).
- Go module: path `quiz-v2`, `go 1.26.4` (the full version `go mod init` writes with toolchain go1.26.4). Server listens on `APP_ADDR` (default `:8080`).
- Testing policy (user decision): **unit tests only** (`go test ./...`). The plan contains no smoke tests and no end-to-end tests; only compile/static checks (`go build`, `go vet`, `docker build`, `docker compose config`) accompany the unit suite.
- Every task commits; repo has no git history, so Task 1 runs `git init`.

## Review Focus

Conditions the spec implies but no single task's happy-path test exercises; each line is pinned by a test in the owning task, except where the unit-test-only policy leaves it uncovered (noted inline).

1. **Two DB failure modes, two different right answers**: a malformed `DB_DSN` must fail startup immediately with a clear error, not a panic; an *unreachable* DB must not kill the server — it keeps serving and `/healthz` returns 503 until the DB recovers (spec §2 anti-goal: no hard block on a transient drop) — Task 4 pins both with the `store.Open` error test and the dead-pinger 503 test.
2. **`/healthz` must reflect DB health** (compose uses it as the gate): if the DB is down the endpoint must return 503, not 200 — Task 4 pins it with a 503 test on a server built with a dead store.
3. **Config env parsing** (`APP_ADDR` set to empty): an empty string must fall back to the `:8080` default, never become a broken listener — Task 2 pins default and override tests.
4. **Template render failure** (missing/unparsable template file): must produce a 500 response, never a 200 with an empty body or a panic — Task 3 pins it with a test pointing the view loader at a nonexistent template.
5. **Docker image must be self-contained** (spec §14: LAN works with the internet down): the built image must serve `/` without fetching anything — **not covered by unit tests** (unit-test-only policy); the only guard is the `docker build` in Task 6, and any runtime defect surfaces at deployment.
6. **Compose ordering** (app must not report healthy before MariaDB accepts connections): `depends_on.condition: service_healthy` on `db` plus the app healthcheck hitting `/healthz` — **not covered by unit tests**; the only guard is `docker compose config` in Task 7, and any defect surfaces at deployment.

---

## File Structure

```
quiz-v2/
├── .gitignore                        # bin/, *.exe, .env — Task 1
├── .dockerignore                     # .git, docs, assets — Task 6
├── go.mod / go.sum                   # module quiz-v2 — Task 1
├── Dockerfile                        # multi-stage build — Task 6
├── docker-compose.yml                # app + db — Task 7
├── migrations/.gitkeep               # spec §11 directory — Task 1
├── cmd/
│   └── server/
│       └── main.go                   # wire config → store → server, exit codes — Task 4
├── internal/
│   ├── config/
│   │   ├── config.go                 # env loading — Task 2
│   │   └── config_test.go
│   ├── httpserver/
│   │   ├── server.go                 # Fiber app, routes, view rendering — Task 3
│   │   ├── server_test.go
│   │   ├── views.go                  # html/template loading + render helper — Task 3
│   │   └── views_test.go
│   └── store/
│       ├── store.go                  # Open/Ping — Task 4
│       └── store_test.go
└── templates/
    └── hello.html                    # SSR page for GET / — Task 3
```

Responsibilities: `config` reads env and validates it; `store` owns the `*sql.DB` lifecycle only; `httpserver` owns routing, template rendering and HTTP concerns; `main` is the only place that knows the startup order. Files that change together live in the same package.

---

### Task 1: Repository bootstrap

**Files:**
- Create: `.gitignore`
- Create: `migrations/.gitkeep`
- Create: `go.mod`, `go.sum` (generated)

**Interfaces:**
- Consumes: nothing.
- Produces: Go module `quiz-v2` (go 1.26.4) that later tasks add packages to; initialized git repo every later task commits to.

- [ ] **Step 1: Initialize git and the module**

```bash
cd D:/Project/Mother/quiz-v2
git init
go mod init quiz-v2
```

Expected: `go.mod` exists containing `module quiz-v2` and `go 1.26.4`.

- [ ] **Step 2: Write `.gitignore`**

```
/bin/
*.exe
*.test
.env
```

- [ ] **Step 3: Create the migrations directory placeholder**

```bash
mkdir -p migrations && touch migrations/.gitkeep
```

(Spec §11 fixes the location; migration files arrive with the schema plan.)

- [ ] **Step 4: Verify the module builds an empty program**

```bash
go build ./...
```

Expected: exits 0 (no packages yet — that is success).

- [ ] **Step 5: Commit**

```bash
git add .gitignore go.mod migrations/.gitkeep
git commit -m "chore: initialize go module and repo scaffolding"
```

---

### Task 2: Config loading

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `config.Config` struct; `func Load() Config` reading env; fields `Addr string` (Fiber listen address, e.g. `:8080`), `DBDSN string`, `CookieSecure bool`, `SessionSigningKey string`, `DatabaseURL` fields as pinned below. Later tasks read `cfg.Addr`, `cfg.DBDSN`, `cfg.CookieSecure`, `cfg.SessionSigningKey`.

```go
type Config struct {
    Addr              string // APP_ADDR, default ":8080"
    DBDSN             string // DB_DSN, default "quiz:quiz@tcp(db:3306)/quiz?parseTime=true"
    CookieSecure      bool   // COOKIE_SECURE, default false (spec §12)
    SessionSigningKey string // SESSION_SIGNING_KEY, default "" (empty until auth task)
}
func Load() Config
```

- [ ] **Step 1: Write the failing tests**

```go
func TestLoadDefaults(t *testing.T) {
    t.Setenv("APP_ADDR", "")
    t.Setenv("DB_DSN", "")
    cfg := config.Load()
    if cfg.Addr != ":8080" { t.Errorf("Addr = %q, want %q", cfg.Addr, ":8080") }
    if cfg.CookieSecure != false { t.Error("CookieSecure must default to false (spec §12)") }
    if cfg.DBDSN == "" { t.Error("DBDSN must fall back to its default") }
}

func TestLoadOverridesFromEnv(t *testing.T) {
    t.Setenv("APP_ADDR", ":9090")
    t.Setenv("COOKIE_SECURE", "true")
    t.Setenv("SESSION_SIGNING_KEY", "test-key")
    cfg := config.Load()
    if cfg.Addr != ":9090" { t.Errorf("Addr = %q, want %q", cfg.Addr, ":9090") }
    if cfg.CookieSecure != true { t.Error("CookieSecure should be true when COOKIE_SECURE=true") }
    if cfg.SessionSigningKey != "test-key" { t.Errorf("SessionSigningKey = %q", cfg.SessionSigningKey) }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -v`
Expected: FAIL — package `internal/config` does not exist.

- [ ] **Step 3: Implement `Load() Config` in `internal/config/config.go`**

Read each env var; use the default when unset or empty. `CookieSecure` is `true` only when `COOKIE_SECURE` equals `true` (case-insensitive). No other validation — the DSN is validated by the driver in Task 4.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/ && git commit -m "feat: add env-based config loading"
```

---

### Task 3: Fiber server with SSR Hello World

**Files:**
- Create: `internal/httpserver/server.go`
- Create: `internal/httpserver/views.go`
- Create: `templates/hello.html`
- Test: `internal/httpserver/server_test.go`
- Test: `internal/httpserver/views_test.go`

**Interfaces:**
- Consumes: `config.Config` (Task 2) — only `Addr` is used here.
- Produces:
  - `func New(cfg config.Config) *fiber.App` — builds the app with routes registered (`GET /`, `GET /healthz`); Task 4 adds the store-backed healthz logic.
  - `type ViewLoader` + `func (v *ViewLoader) Render(w io.Writer, name string, data any) error` — Task 4 and future page tasks render through this.
  - `GET /` returns 200 with an HTML page containing `lang="id"` and the text `Halo, Dunia!` (Indonesian copy per spec §15.10).
  - `GET /healthz` returns 200 `ok` in this task; Task 4 changes it to depend on DB ping.

- [ ] **Step 1: Add the Fiber dependency**

Run: `go get github.com/gofiber/fiber/v3@v3.6.1`

Expected: `go.mod` gains `require github.com/gofiber/fiber/v3 v3.6.1` (the version pinned in Global Constraints).

- [ ] **Step 2: Write the failing tests (routes + view error)**

```go
func TestHelloRoute(t *testing.T) {
    app := httpserver.New(config.Config{Addr: ":8080"})
    req := httptest.NewRequest("GET", "/", nil)
    resp, err := app.Test(req)
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != 200 { t.Fatalf("status = %d, want 200", resp.StatusCode) }
    body, _ := io.ReadAll(resp.Body)
    if !strings.Contains(string(body), `lang="id"`) { t.Error("page must declare lang=id (spec §15.10)") }
    if !strings.Contains(string(body), "Halo, Dunia!") { t.Error("page must render the hello copy") }
}

func TestHealthzRoute(t *testing.T) {
    app := httpserver.New(config.Config{Addr: ":8080"})
    req := httptest.NewRequest("GET", "/healthz", nil)
    resp, err := app.Test(req)
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != 200 { t.Fatalf("status = %d, want 200", resp.StatusCode) }
}

func TestRenderMissingTemplateReturnsError(t *testing.T) {
    v := httpserver.NewViewLoader(t.TempDir()) // empty dir, no templates
    err := v.Render(io.Discard, "hello.html", nil)
    if err == nil { t.Fatal("expected error for missing template, got nil") }
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/httpserver/ -v`
Expected: FAIL — package `internal/httpserver` does not exist.

- [ ] **Step 4: Implement `New` and the view loader**

`server.go`: `func New(cfg config.Config) *fiber.App` — Fiber v3 app, register `GET /` (render `hello.html` through the view loader; render error → 500) and `GET /healthz` (return plain `ok`). Handlers must be `app.Test`-compatible — no middleware yet.

`views.go`: `func NewViewLoader(dir string) *ViewLoader` parses `dir`'s `*.html` with `html/template` at construction; `func (v *ViewLoader) Render(w io.Writer, name string, data any) error` returns a non-nil error for a missing or unparsable template. `dir` is a field so tests can point it at a temp dir; the default is `./templates` (resolves identically in the container's `WORKDIR /app`, see Task 6).

- [ ] **Step 5: Create `templates/hello.html`**

Minimal valid HTML5 document: `<html lang="id">`, `<head>` with `<title>Quiz</title>`, `<body>` containing `Halo, Dunia!`. No external stylesheets or scripts (spec §14: no CDN).

- [ ] **Step 6: Run all tests**

Run: `go test ./internal/httpserver/ -v`
Expected: PASS (3 tests).

- [ ] **Step 7: Commit**

```bash
git add internal/httpserver/ templates/ go.mod go.sum && git commit -m "feat: add fiber server with SSR hello page and healthz"
```

---

### Task 4: Store wiring, healthz depends on DB, main entrypoint

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`
- Create: `cmd/server/main.go`
- Modify: `internal/httpserver/server.go` (healthz handler)

**Interfaces:**
- Consumes: `config.Config` (Task 2), `httpserver.New` (Task 3).
- Produces:
  - `func Open(dsn string) (*sql.DB, error)` — opens `database/sql` with the mysql driver; caller pings.
  - `func Ping(ctx context.Context, db *sql.DB) error` — thin wrapper, exists so healthz can be tested with a stub.
  - `httpserver.New` gains a variant `NewWithStore(cfg config.Config, pinger Pinger) *fiber.App` where `Pinger` is `interface{ Ping(context.Context) error }`; `New` delegates to it with a pinger that always returns nil (keeps Task 3 tests valid).
  - `/healthz` returns 200 `ok` when ping succeeds, 503 `db unavailable` when it fails.
  - `cmd/server/main.go`: load config → open store → start Fiber on `cfg.Addr`. A malformed DSN makes `store.Open` fail → log and `os.Exit(1)` immediately. An *unreachable* DB is NOT fatal: startup attempts one advisory ping (5s timeout), logs a warning on failure, and the server keeps serving with `/healthz` 503 until the DB comes back.

- [ ] **Step 1: Add the MySQL driver dependency**

Run: `go get github.com/go-sql-driver/mysql@v1.10.1`

Expected: `go.mod` gains `require github.com/go-sql-driver/mysql v1.10.1` (pinned in Global Constraints).

- [ ] **Step 2: Write failing store tests**

```go
func TestOpenInvalidDSNReturnsError(t *testing.T) {
    _, err := store.Open("not-a-dsn")
    if err == nil { t.Fatal("expected error for malformed DSN") }
}

func TestPingUnreachableReturnsError(t *testing.T) {
    db, err := store.Open("user:pass@tcp(127.0.0.1:1)/none?timeout=1s")
    if err != nil { t.Skipf("driver rejected DSN setup: %v", err) }
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    if err := store.Ping(ctx, db); err == nil { t.Fatal("expected ping error against closed port") }
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/store/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 4: Implement `store.Open` and `store.Ping`; run tests**

Run: `go test ./internal/store/ -v` → PASS.

- [ ] **Step 5: Write failing healthz-with-dead-DB test in `internal/httpserver/server_test.go`**

```go
type deadPinger struct{}
func (deadPinger) Ping(context.Context) error { return errors.New("db down") }

func TestHealthzReturns503WhenDBDown(t *testing.T) {
    app := httpserver.NewWithStore(config.Config{Addr: ":8080"}, deadPinger{})
    resp, err := app.Test(httptest.NewRequest("GET", "/healthz", nil))
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != 503 { t.Fatalf("status = %d, want 503", resp.StatusCode) }
}
```

- [ ] **Step 6: Run to verify failure, then refactor healthz to use the pinger**

Run: `go test ./internal/httpserver/ -v` → FAIL (route not yet pinger-aware). Implement `NewWithStore`, make `/healthz` ping with a 2s timeout, map error → 503. Run again → PASS (4 tests: 3 from Task 3 + 1 new).

- [ ] **Step 7: Implement `cmd/server/main.go`**

Startup sequence exactly: `config.Load()` → `store.Open(cfg.DBDSN)` (error → log + `os.Exit(1)`) → one advisory `store.Ping` with 5s timeout (failure → log a warning, continue) → `httpserver.NewWithStore(cfg, pinger)` → `app.Listen(cfg.Addr)`. Healthz performs its own ping with a 2s timeout per request, so a DB that drops later is reported through the status code, never fatal. Log each stage to stdout.

`store.Ping(ctx, db)` is a free function, so `main` adapts it to the `httpserver.Pinger` interface with a local type — this is the exact wiring:

```go
type pingerFunc func(context.Context) error

func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }

pinger := pingerFunc(func(ctx context.Context) error { return store.Ping(ctx, db) })
app := httpserver.NewWithStore(cfg, pinger)
```

- [ ] **Step 8: Commit**

```bash
git add internal/store/ internal/httpserver/ cmd/ go.mod go.sum && git commit -m "feat: wire database store and db-aware healthz into main"
```

---

### Task 5: Unit-test pass and module tidy (checkpoint)

**Files:**
- Modify: `go.sum` (generated)

**Interfaces:**
- Consumes: all code from Tasks 1–4.
- Produces: a green `go test ./...` baseline every later task must keep green.

- [ ] **Step 1: Tidy, verify pinned versions, and run the full suite**

```bash
go mod tidy
go list -m github.com/gofiber/fiber/v3 github.com/go-sql-driver/mysql
go vet ./...
go test ./... -count=1
```

Expected: `go list -m` prints `github.com/gofiber/fiber/v3 v3.6.1` and `github.com/go-sql-driver/mysql v1.10.1` (exactly the versions pinned in Global Constraints); `tidy` changes nothing further; vet is clean; every test in the module PASSes.

- [ ] **Step 2: Commit**

```bash
git add go.mod go.sum && git commit -m "chore: tidy module deps and verify test suite"
```

---

### Task 6: Dockerfile and .dockerignore

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

**Interfaces:**
- Consumes: `cmd/server` from Task 4, `templates/` from Task 3.
- Produces: a Linux container image that runs the server on `APP_ADDR` (default `:8080`) and serves `GET /` and `GET /healthz` from bundled files only.

- [ ] **Step 1: Write `.dockerignore`**

```
.git
.gitignore
docs
assets
*.md
.env
bin/
```

(`templates/` must NOT be excluded — it is baked into the image.)

- [ ] **Step 2: Write `Dockerfile`**

Multi-stage:
- Build stage: `golang:1.26-alpine`, `WORKDIR /src`, copy `go.mod go.sum`, `go mod download`, copy rest, `CGO_ENABLED=0 go build -o /out/server ./cmd/server`.
- Runtime stage: `alpine:3.20`, create non-root user, copy `/out/server` and `templates/` to `/app/templates/`, `WORKDIR /app`, `EXPOSE 8080`, `ENTRYPOINT ["./server"]`.

The templates path must resolve relative to the working directory (`/app/templates`) so the view loader finds them in-container; local runs use `./templates`. Same relative path, both work.

- [ ] **Step 3: Build the image**

Run: `docker build -t quiz-v2:dev .`
Expected: builds to completion, exit 0.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile .dockerignore && git commit -m "feat: add multi-stage dockerfile"
```

---

### Task 7: docker-compose stack

**Files:**
- Create: `docker-compose.yml`

**Interfaces:**
- Consumes: image build from Task 6, `DB_DSN`/`APP_ADDR` config vars from Task 2, healthz semantics from Task 4.
- Produces: a compose file that builds the app image, starts MariaDB 12.3 with a healthcheck, gates the app on `depends_on: service_healthy`, and passes the app `APP_ADDR`/`DB_DSN`. Runtime behavior is covered by the unit tests in Tasks 2–4; per the unit-test-only policy nothing is started here.

- [ ] **Step 1: Write `docker-compose.yml`**

```yaml
services:
  db:
    image: mariadb:12.3
    environment:
      MARIADB_ROOT_PASSWORD: quizroot
      MARIADB_DATABASE: quiz
      MARIADB_USER: quiz
      MARIADB_PASSWORD: quiz
    volumes:
      - dbdata:/var/lib/mysql
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 5s
      timeout: 5s
      retries: 12
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      APP_ADDR: ":8080"
      DB_DSN: "quiz:quiz@tcp(db:3306)/quiz?parseTime=true"
    depends_on:
      db:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/healthz"]
      interval: 5s
      timeout: 5s
      retries: 12
      start_period: 10s
volumes:
  dbdata:
```

Notes: `healthcheck.sh` ships in the official `mariadb:12.3` image; `MARIADB_USER`/`MARIADB_PASSWORD` create the `quiz` user with rights on `MARIADB_DATABASE` (the image only auto-creates root otherwise). The `app` healthcheck uses busybox `wget`, which is present in the `alpine` runtime stage — it exits non-zero on the 503 healthz returns while the DB is down, so the service reports `unhealthy` until the DB answers and `healthy` afterwards. (`MARIADB_*` env vars only take effect on the first boot of an empty volume.)

- [ ] **Step 2: Validate the compose file parses**

Run: `docker compose config --quiet`
Expected: exit 0 — Compose resolves the build context, environment, healthchecks and volume without error. Static validation only; per the unit-test-only policy no service is started.

- [ ] **Step 3: Commit**

```bash
git add docker-compose.yml && git commit -m "feat: add compose stack with mariadb and health-gated startup"
```
