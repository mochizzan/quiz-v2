# Backend foundation — the contract every domain builds on

**Date:** 2026-10-23
**Status:** built and verified (`go build ./...`, `go vet ./...`, `go test ./...` green)
**Audience:** the four domain implementation agents (IDN, AUT, PAR, ATT) and the
later OPS audit.

This document is the shared context. It describes only what already exists and
what a domain handler must do. Business rules live in the five specs; this file
never restates them.

---

## 1. What is verified, and what is not

Verified without a database:

- the application builds and every package passes `go vet`;
- the full existing test suite passes, including the 72-route conformance pin;
- CSRF issue/verify, signed tokens, password policy and hashing, email
  normalisation, the rate limiter, the error taxonomy, DSN validation and MySQL
  error classification all have unit tests.

**Not verified: anything that needs MariaDB.** The Docker engine on this machine
is not running (`com.docker.service` stopped, no listener on 3306), so no
integration test has executed. Integration tests are written to skip when
`TEST_DB_DSN` is unset. Treat every SQL statement as unproven until the database
is up and the suite runs with:

```
TEST_DB_DSN='quiz:quiz@tcp(127.0.0.1:3306)/quiz?parseTime=true&loc=UTC' go test ./...
```

---

## 2. Frozen surfaces — do not edit

- `migrations/*.sql` and `internal/model/*.go` — the schema is fixed (R-01).
  If a rule seems to need a column that does not exist, stop and report.
- `templates/**` and `static/**` — the UI is finished.
- `docs/DESIGN.md`, `docs/superpowers/specs/**` — reference only.
- `internal/viewmodel/**` — the typed page models already match every screen.
  You may read them; adding a field is allowed only when the spec requires a
  value no existing model carries, and must be reported.
- Shared infrastructure:
  `internal/apperr`, `internal/store` (except adding a domain method is not
  allowed — keep domain SQL in your own package), `internal/secure`,
  `internal/authn`, and
  `internal/httpserver/{authz.go,ctx.go,pipeline.go,respond.go,routes.go,server.go}`.
  If one of these needs a change, report it in your summary instead of editing.

---

## 3. The handler seam

Every route is a `PageRoute` in `internal/httpserver/routes_<group>.go`. A
production handler is added **next to** the existing fixture handler:

```go
registerPage(PageRoute{
    Method:   "GET",
    Path:     "/teacher/students",
    Template: "pages/teacher/students.html",
    Nav:      "siswa",
    Auth:     ClassTeacher,              // must equal the matrix entry
    Load: func(x *httpserver.Ctx) (viewmodel.PageData, error) { ... },   // GET
    // Build stays untouched: it serves APP_DEMO previews.
})
```

- `Load func(*Ctx) (viewmodel.PageData, error)` — GET.
- `Act func(*Ctx) (Redirect, error)` — POST.
- `Redirect{Path: "/teacher/students", Flash: "saved"}` — the HTTP layer turns it
  into a 303 and sets the one-shot flash cookie.
- `Handler func(fiber.Ctx) error` — only for SSE/beacon; already used.

The framework decides the status code. A handler never calls `c.Status`, never
writes a header, and never redirects by returning a string.

### `*Ctx` — what a handler receives

| Field | Meaning |
|---|---|
| `DB *store.DB` | the pool; `nil` in the fixture-preview build |
| `User *viewmodel.CurrentUser` | resolved identity, `nil` when anonymous |
| `Session *authn.Session` | the live session (`ID`, `AccountID`, `Role`, `MustChangePassword`, …) |
| `CSRF string` | the validated token to render in forms |
| `Demo bool` | APP_DEMO |
| `IP string` | transport peer address (never a proxy header) |
| `ReqID string` | correlation id for logs |
| `Params map[string]string` | path params, keyed without `:` |
| `Query url.Values`, `Form url.Values` | parsed input |
| `Flash *viewmodel.Flash` | the one-shot message, already consumed |

Methods: `Param`, `IntParam`, `QueryValue`, `Value` (trimmed form value),
`RawValue` (untrimmed, for passwords), `Has`, `Checked`, `State` (demo-only
`?state=`), `IsAuthenticated`, `IsTeacher`, `IsStudent`, `Redirect`, `Signer`,
`Now`, `SetSessionCookie`, `ClearSessionCookie`, `RotateCSRF`, `SetCookie`,
`ClearCookie`.

Domain logic belongs in `internal/<domain>` and must accept a
`store.Querier` (pool or transaction), never `*Ctx`.

---

## 4. Authorization

`internal/httpserver/authz.go` holds `routeClasses`, keyed `"METHOD /path"`, and
it is authoritative. `validateRouteClasses` fails application startup when a
route is missing, cites an unknown class, or declares an `Auth` field that
disagrees with the table.

| Class | Behaviour |
|---|---|
| `public` | no identity needed |
| `guest-only` | signed-in visitor is redirected to their role home |
| `any-auth` | signed-out visitor is redirected to `/login` |
| `student` | signed-out → `/login`; other role → **404** |
| `teacher` / `owner-teacher` | signed-out → `/login`; other role → **404** |
| `anonymous-or-public` | PAR entry routes; ownership is the handler's job |

`owner-teacher` admits any active teacher; **the handler returns 404 for a
non-owner**, never 403, because 403 would confirm the row exists (R-05).

Enforcement is skipped **only** for the fixture-preview build: `APP_DEMO` on
with no database. Production always enforces.

---

## 5. Pipeline order (already implemented)

For every route, in order:

1. static assets (`/static`, no session or CSRF work);
2. body cap (1 MiB form, 4 KiB beacon, 10 MiB multipart upload);
3. session resolve + slide;
4. CSRF: issue on GET, verify on POST (beacon excepted, same-origin required);
5. must-change-password guard;
6. flash read;
7. authorization class;
8. handler.

Consequences for handlers:

- A handler only runs after CSRF and the class check passed.
- A must-change session can only reach `/change-password` and `POST /logout`.
- The CSRF token is always in `x.CSRF`; render it, do not mint one.
- `POST /attempt/:id/beacon` is the only CSRF-exempt route and must verify
  same-origin itself if it needs to (the pipeline already does).

---

## 6. Data rules

- **Soft delete.** A row is live when `deleted_at = store.LiveAt`
  (`'1970-01-01 00:00:00'`). Every read filters on it. Every "delete" writes
  the timestamp; nothing is `DELETE`d except by the documented cascade.
- **Time comes from the database.** Write `UTC_TIMESTAMP()` in SQL. Never pass
  `time.Now()` into a column: the app clock and the DB clock are different
  authorities and the expiry rules assume one.
- **Transactions.** `db.Tx(ctx, fn)` for a unit that must be all-or-nothing;
  `db.TxRetry(ctx, fn)` only when the unit is idempotent (it retries a deadlock
  once). Pass the `*sql.Tx` to every repository call in the unit.
- **`sql.Null*` everywhere a column is nullable.** Do not assume non-null.
- **Classification.** `store.IsDuplicate(err)` for 1062;
  `store.Classify(err)` for deadlock (retry), lock wait, FK missing, too long.
  Never match on driver text.
- **Pagination.** `Page`, `PageSize`, total, and `pages` are derived once in the
  read model; `pageNumber` already clamps bad input.

---

## 7. Errors, statuses, copy

Return an `*apperr.Error`; the HTTP layer renders it.

| Constructor | Status | Page |
|---|---|---|
| `apperr.NotFoundf()` | 404 | `pages/error/404.html` |
| `apperr.Forbiddenf("")` | 403 | `pages/error/403.html` |
| `apperr.Field("email", msg)` / `Validationf(map)` | 422 | re-renders the route's template **when the handler returns data with it** |
| `apperr.RateLimitedf(retryAfter)` | 429 | `pages/error/429.html` + `Retry-After` |
| `apperr.Internalf(err)` | 500 | `pages/error/500.html` |

422 contract:

```go
data, err := buildForm(x)                       // the form, repopulated
return data, apperr.Validationf(map[string]string{"email": apperr.MsgEmailTaken})
```

`finish` sets `data.Base.Errors` and renders the route template at 422. If the
handler returns a validation error with no data, the generic 422 page is used —
avoid that; a user must get their form back.

Copy constants are in `internal/apperr` (`MsgInvalidCredentials`, `MsgEmailTaken`,
`MsgPasswordPolicy`, `MsgSessionExpired`, …). Reuse them; never invent a
competing sentence for the same condition.

Flash keys are closed: `viewmodel.FlashFor` must know the key. If you need a new
one, add it there and report it.

---

## 8. Package ownership and files

Each agent owns its package and its route files exclusively. No two agents edit
the same file.

| Agent | Spec | Owns |
|---|---|---|
| **IDN** | `...-01-identity-and-access.md` | `internal/identity/**`; `routes_auth_public.go`, `routes_auth_account.go`, `routes_profile.go`, `routes_teacher_mgmt.go`, `routes_home.go` |
| **AUT** | `...-02-authoring-and-lifecycle.md` | `internal/authoring/**`; `routes_teacher_core.go`, `routes_teacher_authoring.go`; `GET /uploads/:name` |
| **PAR** | `...-03-participation-and-eligibility.md` | `internal/participation/**`; `routes_join.go`, `routes_student.go`, `routes_teacher_catalogue.go` |
| **ATT** | `...-04-attempts-and-grading.md` | `internal/attempts/**`; `routes_attempt.go`, `routes_teacher_dashboard.go`, `routes_teacher_grading.go` |

Route split follows contract §6 exactly. `routes_error.go` and the foundation
files belong to OPS.

---

## 9. Cross-domain composition

Domain packages are siblings: **they never import each other.** Composition
happens in the route adapter (package `httpserver`), which may import any domain.

Example — `POST /quiz/:id/attempts` is PAR's command whose attempt start is
ATT's rule:

```go
err := x.DB.TxRetry(ctx, func(tx *sql.Tx) error {
    if err := participation.CheckEligibility(ctx, tx, ...); err != nil { return err }
    return attempts.Start(ctx, tx, ...)
})
```

One transaction, two domains, no cycle. Reads that one domain owns for another
go through an exported function on the owning package.

---

## 10. Testing

- `go build ./...` and `go test ./...` must pass before you report.
- Unit-test pure rules (validation, derivations, state guards) with table tests.
- Integration tests use `internal/store/storetest`:
  `db := storetest.DB(t)` (skips without `TEST_DB_DSN`) and
  `storetest.Reset(t, db)` before a writing test.
- Never weaken an existing test to make it pass. The 72-route pin in
  `conformance_test.go` and the authorization matrix test are deliberate; if a
  change genuinely alters the surface, say so — do not silently edit the pin.
- Do not add a test that requires a live database to the default suite without
  the `storetest` skip.

---

## 11. Definition of done for one route

1. `Load` or `Act` implements the spec's command, using the package of its
   domain, over one transaction where the spec requires one.
2. Every guard from the spec is present: authorization/ownership (404), state
   legality, soft-delete visibility, uniqueness, eligibility.
3. Every validation failure returns a 422 **with the re-rendered form** and the
   exact copy key.
4. Every success returns a 303 to the screen named in the spec, with a closed
   flash key.
5. `Auth` is set and agrees with the matrix.
6. Tests: unit for the rules; integration (skipped without a DB) for the SQL.
7. The fixture `Build`/`Action` is left intact so `APP_DEMO` still renders.
8. The report lists: routes implemented, rules cited, transactions used, tests
   added, and anything the schema or shared files could not support.
