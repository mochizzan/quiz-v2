# Identity and Access — Business Logic Specification

**Date:** 2026-10-23
**Status:** binding for implementation; product calls flagged in §12
**Scope:** accounts, teacher/student registration, approval, login/logout, sessions, CSRF, password reset and change, profile, teacher management of students, teacher directory and approval, and the authorization matrix every other domain consumes.
**Lens:** security and adversarial. Every rule below assumes an attacker who can read the public pages, replay cookies, submit forms cross-site, enumerate identifiers, race two requests, and choose their own input.
**Sources (authority order):** `migrations/*.up.sql` + `internal/model/*.go` + schema spec (shape); frontend data contract (screens, forms, copy); `2026-10-20-quiz-system-design.md` §3/§12/§14 (rules not superseded); `docs/DESIGN.md` (visual only); contract `2026-10-23-business-logic-00-contract.md` (rulings R-01–R-25, invariants INV-01–INV-30, §7 shape).
**Rule prefix:** `IDN-###`. Commands: `IDN-C01` … `IDN-C22`. Domain invariants: `IDN-I01` …
**Contract note:** Contract §7 fixes §11 as traceability and §12 as the open-question/schema-delta register; this file follows that shape. The authoring brief calls the register "§11", so §11.5 records ruling objections and points into §12 for the register entries.

---
## 1. Intent and boundaries
**IDN-001.** This domain decides *who a request is*, *whether that identity may enter a route*, and *the lifecycle of every credential that proves it*: password, session cookie, CSRF cookie, pending-registration token, and student reset token.
**IDN-002.** It also owns the school-wide management surface for people: the student roster and its edit/delete/reset actions, the teacher directory, and teacher approval. It does not own quizzes, registrations, attempts, grading, majors, or anonymous identities.
**IDN-003.** "Authenticated" means: a live `sessions` row whose `expires_at > UTC_TIMESTAMP()`, joined to a live `accounts` row with `status = 'active'`. A pending account, a soft-deleted account, an expired session, and a forged cookie are all unauthenticated. A pending teacher can never hold a valid session (R-03, INV-04, INV-21).
**IDN-004.** Delegations. This spec never redefines these; it calls them by ID:

| Concern | Owner | Rule |
|---|---|---|
| Anonymous identity cookie and participant CRUD | PAR | PAR-2xx; cookie name must not collide with `quiz_session`/`quiz_csrf`/`quiz_pending`/`quiz_reset_reveal` |
| Quiz ownership, codes, invite, lifecycle | AUT | AUT-1xx; IDN only supplies the session identity |
| Registration eligibility, cancellation, quiz intro | PAR | PAR-3xx; IDN guarantees the profile's grade/major are current |
| Attempt ownership, beacon, grading, results, monitor | ATT | ATT-1xx/4xx; IDN supplies session identity and CSRF exemption metadata |
| Soft-delete cascade mechanics, transaction style, error taxonomy | OPS | OPS-2xx; IDN states its cascade set and transaction boundary |
| 404/500 pages, security headers, server error mapping | OPS | OPS-7xx; IDN adds the 403 page and auth-specific cache headers |
| Quiz lifecycle, review modes, scores | AUT/ATT | not this domain |

**IDN-005.** Non-goals: email delivery or verification, password recovery for teachers, SSO, MFA, "remember me", device/session management UI, account merging, teacher deactivation, role changes after creation, audit tables, and admin roles (R-22, R-05).
**IDN-006.** The database is fixed (R-01). No rule here may depend on a column that does not exist. Changes that would need one are isolated in §12, and none are required for v1.
**IDN-007.** Every command in §5 is a page command: it either renders a template or returns `303`. There is no JSON auth API, no Basic auth, and no `401`. The only non-page endpoint in this domain is the health check, which OPS owns.
**IDN-008.** Copy is Indonesian; the exact strings are in §5 and reuse every R-19 key verbatim. A message not listed here is not invented by a handler.

---
## 2. Actors, permissions, entry conditions
### 2.1 Actors
| Actor | Definition | Can authenticate | Sees |
|---|---|---|---|
| Visitor | no valid session | no | guest-only routes; join/invite (PAR) |
| Pending teacher | `accounts.role='teacher'`, `status='pending'` | **no**; credentials may be correct | `/auth/pending` with a valid token; login refusal page |
| Active teacher | `role='teacher'`, `status='active'`, live | yes | every teacher route it owns or that AUT/PAR/ATT scopes by owner |
| Student | `role='student'`, `status='active'`, live, one live `students` row | yes | student routes, own profile, own history |
| Bootstrap teacher | an active teacher inserted by the runner with `must_change_password=1` | yes, but blocked by IDN-070 until the password changes | only `/change-password` and `/logout` at first |
| Deleted account | `accounts.deleted_at != sentinel` | no; treated as absent | nothing |
| Anonymous participant | PAR cookie, no account | n/a | PAR routes only; never a `CurrentUser` |

**IDN-010.** Role is read from `accounts.role` on every request. It is never taken from the URL, a form field, or a cookie claim (R-02 analog; a client-supplied role is not identity).
**IDN-011.** Teacher and student are the only account roles. `anonymous` is not an account and never authenticates (`accounts.role` enum enforces the storage side).

### 2.2 Authorization classes
Every route registered by `registerPage` (and every non-page route) declares exactly one class. The middleware consults the class table; a route without a class is a startup error, not a default-allow.

| Class | Meaning | Unauthenticated result | Wrong role result |
|---|---|---|---|
| `public` | no session needed; the handler decides (root and logout redirect) | render or `303` | render |
| `guest-only` | only without a session; authenticated users go home | render | `303` home (teacher) or `/student` |
| `any-auth` | either role may enter | `303 /login` (GET) / `303 /login` (POST) | — |
| `student` | active student only | `303 /login` | `404` |
| `teacher` | active teacher only | `303 /login` | `404` |
| `owner-teacher` | active teacher **and** resource owner | `303 /login` | `404` (including a non-owner teacher) |
| `anonymous-or-public` | PAR guests and any signed-in actor | render | — |

**IDN-012.** `404` for wrong role is deliberate (R-05, R-19): a student probing `/teacher/students` learns nothing about whether the route or the resource exists. A teacher probing `/profile` gets `404`, because R-05 and contract §10.3 make profile student-only.
**IDN-013.** `403` is reserved for exactly two situations: CSRF verification failure (R-19, INV-28) and a login attempt with **correct** credentials on a pending teacher account (contract §9.1 "403-style message"). Everything else that is not allowed is `404` or a `303` redirect.
**IDN-014.** `303` is the only redirect status. `301`/`302`/`307`/`308` are not emitted by this domain.
**IDN-015.** No `401`. No `405`: an unregistered method falls to the 404 catch-all, which OPS owns.

### 2.3 Authorization matrix (owned routes)
| Route | Class | Command | Additional resource check |
|---|---|---|---|
| `GET /` | `public` | IDN-C01 | role comes from the session only |
| `GET /login` | `guest-only` | IDN-C02 | — |
| `POST /login` | `guest-only` | IDN-C03 | — |
| `GET /register/teacher` | `guest-only` | IDN-C04 | — |
| `POST /register/teacher` | `guest-only` | IDN-C05 | — |
| `GET /register/student` | `guest-only` | IDN-C06 | — |
| `POST /register/student` | `guest-only` | IDN-C07 | major must be live |
| `GET /reset-password` | `guest-only` | IDN-C08 | — |
| `POST /reset-password` | `guest-only` | IDN-C09 | token must be live, unused, unexpired |
| `GET /change-password` | `any-auth` | IDN-C10 | must-change sessions allowed |
| `POST /change-password` | `any-auth` | IDN-C11 | must-change sessions allowed |
| `GET /auth/pending` | `public` | IDN-C12 | token must verify; no session required |
| `POST /logout` | `public` (idempotent) | IDN-C13 | CSRF still required |
| `GET /profile` | `student` | IDN-C14 | own account only; no `:id` exists |
| `POST /profile` | `student` | IDN-C15 | own account only |
| `GET /teacher/students` | `teacher` | IDN-C16 | school-wide (R-05) |
| `GET /teacher/students/:id` | `teacher` | IDN-C17 | live student by `students.id` |
| `POST /teacher/students/:id` | `teacher` | IDN-C18 | live student; email unique |
| `POST /teacher/students/:id/delete` | `teacher` | IDN-C19 | live student |
| `POST /teacher/students/:id/reset-token` | `teacher` | IDN-C20 | live student |
| `GET /teacher/teachers` | `teacher` | IDN-C21 | school-wide |
| `POST /teacher/teachers/:id/approve` | `teacher` | IDN-C22 | target live teacher with `status='pending'` |

**IDN-016.** Routes owned by other domains use the same classes: AUT and ATT teacher routes are `owner-teacher` (owner check by AUT/ATT, 404 for non-owner); PAR's `/student*` are `student`; PAR's `/join*`, `/invite/*`, `/quiz/:id*` are `anonymous-or-public`; OPS's `/healthz`, `/error/*`, `/uploads/:name` are `public`. IDN does not implement those ownership checks; it supplies `CurrentUser` and the class.
**IDN-017.** `POST /attempt/:id/beacon` is the sole CSRF-exempt route in the system (R-16, R-20, INV-28). Its authorization and same-origin check belong to ATT. No IDN route is exempt.

### 2.4 Status-code policy (the complete matrix)
| Situation | Status | Response |
|---|---|---|
| GET renders a page | `200` | template, `Cache-Control: no-store` |
| POST succeeds or state-conflicts benignly | `303` | `Location` to the PRG target + flash key |
| Validation fails, including bad credentials | `422` | same template, repopulated non-secrets, `FieldErrors`/form flash |
| CSRF cookie missing, unsigned, forged, or mismatched | `403` | 403 error page, copy `csrf` |
| Login with correct credentials on a pending teacher | `403` | 403 page, `pending_teacher`, optional pending link |
| Token invalid, expired, used, or for a missing student (reset flow) | `422` | reset form, `FieldErrors["token"]`, copy `invalid_token`/`token_used` |
| Rate limit exceeded | `429` | 429 page + `Retry-After` seconds, copy `rate_limited` |
| Missing, soft-deleted, wrong role, not school-scoped | `404` | 404 page |
| Database error | `500` | 500 page |
| Unauthenticated on a protected GET | `303` | `/login` (no `next`); no flash on plain root, `session_expired` flash otherwise |
| Unauthenticated on a protected POST | `303` | `/login`; `session_expired` flash; body is not replayed |

**IDN-018.** A `404` page is returned with status `404`, never `200`. A `403` page is returned with status `403`. The 403 template is `pages/error/403.html`; OPS owns the error family and this domain owns the 403 copy (see §12.2).
**IDN-019.** No response ever includes a stack trace, SQL, an account id in an error body, or a password value. Errors log internally with request id; the page shows only the copy table.

### 2.5 Entry conditions and middleware pipeline
**IDN-020.** The global pipeline runs in this order for every request:

1. **Static/public bypass** — `/static/*` is served by the static middleware; no session lookup, no CSRF.
2. **Session resolution** — read `quiz_session`; SHA-256 it; look up a live `sessions` row joined to a live `accounts` row. Compute `CurrentUser` only if `status='active'` and `expires_at > UTC_TIMESTAMP()`.
3. **Sliding expiry** — for a valid session, set `expires_at = UTC_TIMESTAMP() + INTERVAL 60 MINUTE` (IDN-042).
4. **CSRF issuance/verification** — on every request, ensure a signed `quiz_csrf` cookie exists (issue on GET if absent/invalid); on every POST except the beacon, verify (IDN-049).
5. **Must-change guard** — a valid session with `must_change_password=1` may reach only `GET/POST /change-password` and `POST /logout`; every other path gets `303 /change-password` + `must_change` flash (IDN-070).
6. **Route class** — apply §2.2 to the registered route. Unknown path/method falls to the OPS 404.
7. **Handler** — validation, rate limit, transaction, render/redirect.

**IDN-021.** Steps 1–4 run before any handler-side rate-limit or bcrypt work. A request that fails CSRF consumes no password compare and no rate-limit budget.
**IDN-022.** Session resolution performs at most one indexed lookup (`uq_sessions_token`) and one PK lookup on `accounts`. It performs no writes when the cookie is absent, malformed, unknown, or expired; only a valid live session slides.
**IDN-023.** The CSRF cookie is issued to guests on their first GET of any HTML page, including `/login`, so the login form is protected before a session exists. A POST that arrives with no CSRF cookie is rejected even if the form body carries a plausible token.
**IDN-024.** Guest-only redirects: an authenticated student at `/login`, `/register/student`, `/register/teacher`, or `/reset-password` is sent `303 /student` (or `/change-password` if the must-change flag is set). An authenticated teacher is sent `303 /teacher` (or `/change-password`). `GET /auth/pending` is not guest-only: it is a status page and renders for anyone with a token.
**IDN-025.** No `next`, `return_to`, `redirect`, or `continue` parameter exists on any auth route. Login always lands on the role home. This removes the open-redirect class entirely rather than validating it.
**IDN-026.** `GET /` is the role router: no session → `303 /login`; student → `303 /student`; teacher → `303 /teacher`; must-change session → `303 /change-password`. It never renders and never reflects a query string.
**IDN-027.** The app trusts no proxy header for identity or rate-limit keys. The client IP is the transport peer address unless OPS configures a trusted proxy; if it does, the same trust decision applies to logging and rate limits (IDN-066). A forwarded header alone never changes authorization.

### 2.6 Flash transport
**IDN-028.** Flash is a closed query key (`?flash=<key>`) mapped by `viewmodel.FlashFor`, not user text. The set of keys is fixed in Go; an unknown key renders nothing. Keys carry no names, emails, ids, or counts. This is spoofable only into showing oneself a canned message, which changes no state; a signed one-shot cookie is the post-v1 hardening (see §12.2).
**IDN-029.** Flash is consumed by the redirect target's GET. A refresh may re-show the same canned message; that is acceptable and carries no data. Flash never substitutes for a `404`/`403`: those are statuses, not messages.

---
## 3. States and transitions
### 3.1 Account lifecycle
```
(no row) --student register--> ACTIVE(student) --delete--> DELETED
(no row) --teacher register--> PENDING(teacher) --approve--> ACTIVE(teacher)
PENDING --never--> DELETED (no reject route)
ACTIVE(teacher) --never--> PENDING/DELETED (R-05: teacher delete forbidden)
```

| Transition | Guard | Effects (single transaction) | Conflict behaviour |
|---|---|---|---|
| create student | email live-unique; major live; policy valid | insert `accounts(role=student,status=active,must_change_password=0)`, insert `students`; then create session | duplicate email → `422 email_taken` |
| create teacher | email live-unique; policy valid | insert `accounts(role=teacher,status=pending,approved_at=NULL,approved_by_account_id=NULL,must_change_password=0)`; issue pending token | duplicate email → `422 email_taken` |
| pending → active | actor active teacher; target live teacher, pending | set `status='active'`, `approved_at=UTC_TIMESTAMP()`, `approved_by_account_id=actor`; one `UPDATE` with predicate | already active → idempotent success; missing/student → `404` |
| active student → deleted | actor active teacher; target live student | cascade per IDN-C19 with one timestamp | second delete → `404` |
| active teacher → deleted | **forbidden** | none | no route; a forged path is `404` |

**IDN-030.** `accounts.status='pending'` is meaningful only for teachers; `chk_accounts_student_active` guarantees every student row is active. A student insert that omits `status` fails loudly (schema spec §9.7 #10).
**IDN-031.** `must_change_password` starts `0` for both self-registration flows. It is `1` only for the external bootstrap teacher and any future admin-set temporary password (R-04). The teacher-issued student reset sets it to `0` (IDN-C09).
**IDN-032.** There is no reject, deactivate, suspend, restore, role-change, or delete-teacher command. A pending teacher can be left pending forever; that is not an error state. A teacher cannot be edited at all: name and email are fixed at registration (R-05). The only teacher write after creation is approval and own-password change.

### 3.2 `must_change_password` lifecycle
| From | Event | To | Guard/effect |
|---|---|---|---|
| `0` | any login/registration | `0` | normal |
| `1` | bootstrap seed only | `1` | runner inserts active teacher with `approved_at=UTC_TIMESTAMP()`, `approved_by_account_id=NULL` |
| `1` | successful login | `1` | session created, then guard blocks everything but change/logout |
| `1` | successful `POST /change-password` | `0` | same transaction as the password update; other sessions revoked |
| `0 or 1` | teacher-issued student reset | `0` | reset sets `0`, never `1` (R-04) |

**IDN-070.** The guard is evaluated after authentication and before route class. A `must_change_password=1` session may reach exactly: `GET /change-password`, `POST /change-password`, `POST /logout`. Every other path — including `GET /`, `/teacher`, `/profile`, `/teacher/students`, and every AUT/PAR/ATT route — returns `303 /change-password` with the `must_change` flash. The guard applies to both roles if the flag is ever set, not just teachers.
**IDN-071.** The guard does not prevent the CSRF check; a cross-site POST from a must-change session fails `403` before the redirect. It does not create a loop: `/change-password` is allowlisted for GET and POST; `/logout` is allowlisted for POST.
**IDN-072.** Clearing the flag and setting the new hash are one `UPDATE` in one transaction. A crash between them cannot leave a changed password with the flag still set (or vice versa). After commit, the redirect target is the role home (`/teacher`, or `/profile` for a student), never back to `/change-password`.

### 3.3 Session lifecycle
```
issue(login/registration) -> LIVE --request--> LIVE (expires_at slides +60m)
LIVE --idle 60m--> EXPIRED (authenticates nothing)
LIVE --logout--> REVOKED
LIVE --reset consumed--> REVOKED (all of the account)
LIVE --change password--> REVOKED (all except current)
LIVE --student deleted--> REVOKED (cascade)
LIVE --account deleted--> REVOKED (cascade)
```

| Field | Rule |
|---|---|
| token | 32 bytes from `crypto/rand`, base64url no padding (43 chars); only SHA-256 raw bytes stored in `sessions.token_hash` |
| cookie | `quiz_session`, `HttpOnly`, `SameSite=Lax`, `Secure=COOKIE_SECURE`, `Path=/`, no `Domain`, no `Max-Age` (browser-session cookie) |
| `expires_at` | `UTC_TIMESTAMP() + 60 minutes`, rewritten on every authenticated request |
| `ip` | peer address as 4 or 16 raw bytes; audit only, never an authorization factor |
| `user_agent` | UTF-8 truncated to 255 bytes on a rune boundary; audit only |
| revocation | `deleted_at = UTC_TIMESTAMP()`, `deleted_by_account_id` NULL (system revocation, not a human root) |
| multiple sessions | allowed; each device is independent; there is no session list or remote-revoke UI |

**IDN-040.** Login and registration always mint a new token. A pre-existing `quiz_session` cookie is ignored during authentication and overwritten in the success response (fixation defence, IDN-C03). The old value is not linked to the new session.
**IDN-041.** A session row is valid only if `sessions.deleted_at = sentinel AND sessions.expires_at > UTC_TIMESTAMP() AND accounts.deleted_at = sentinel AND accounts.status='active'`. The role comes from `accounts`, not from the session row (schema spec §6.13).
**IDN-042.** Sliding happens at most once per request, as a single `UPDATE sessions SET expires_at = UTC_TIMESTAMP() + INTERVAL 60 MINUTE WHERE id=? AND deleted_at=sentinel`. Concurrent requests from one browser may each slide; the last writer wins and the result is still `now+60m`. A request that arrives after expiry does **not** slide: validity is checked before the update.
**IDN-043.** Expired-session handling: a request carrying a token that matches a live row with `expires_at <= now` is treated as unauthenticated. The row is soft-deleted best-effort (a write failure must not change the response) and the cookie is cleared. Protected GET → `303 /login` + `session_expired`; protected POST → `303 /login` + `session_expired`, body discarded. The root and public pages treat it as a plain visitor.
**IDN-044.** Logout soft-deletes only the current session row (`deleted_at=now`), clears the cookie with `Max-Age=0`, and leaves every other device signed in. It is idempotent: a missing/expired session still returns `303 /login` with the logout flash, after CSRF passes.
**IDN-045.** Change-password revokes every other live session of the account in the same transaction and leaves the current one live (R-16). Reset-password revokes **every** live session, including any current one (the resetter is normally signed out; if a signed-in student uses a token, they are signed out too).
**IDN-046.** Sessions have no absolute lifetime beyond the sliding window: an actively used session can live indefinitely (R-15/R-16 specify sliding only). This is a recorded residual risk, not an invitation to invent a cap in a handler (§12.3).
**IDN-047.** Session lookup never trusts `sessions.account_id` alone: the join to a live active account is mandatory. A session row whose account is pending, soft-deleted, or missing authenticates nothing (INV-21).

### 3.4 Reset-token lifecycle (student-only)
```
issue(teacher) --24h unused--> EXPIRED
issue(teacher) --student consumes--> USED (used_at set)
issue(teacher) --re-issue--> previous unused tokens REVOKED (deleted_at)
student deleted --> all tokens REVOKED (cascade)
```

| Step | Rule |
|---|---|
| generation | 32 bytes `crypto/rand`, base64url no padding (43 chars) |
| storage | only SHA-256 in `password_reset_tokens.token_hash`; raw token never stored or logged |
| owner | `student_id` = `students.id`; `issued_by_account_id` = active teacher; teachers cannot receive one (INV-22) |
| window | `expires_at = DATE_ADD(UTC_TIMESTAMP(), INTERVAL 24 HOUR)`; DB clock, not app clock |
| single use | `used_at IS NULL` required; consumption sets `used_at=UTC_TIMESTAMP()` |
| supersede | issuing a new token soft-deletes every live unused token of that student in the same transaction (IDN-C20) |
| display | raw token appears once, in a 5-minute signed `quiz_reset_reveal` cookie, rendered on the students list (IDN-105) |
| expiry semantics | an expired but unused row stays; lookup filters `expires_at > UTC_TIMESTAMP()`; no sweep is required |
| reuse copy | `token_used` when `used_at IS NOT NULL`; `invalid_token` for expired/missing/deleted |

**IDN-048.** Reset tokens are not sessions and never authenticate a request. Possession of a token permits exactly one thing: `POST /reset-password` with a new password for that student.

### 3.5 Pending-teacher token lifecycle (R-23)
| Step | Rule |
|---|---|
| issue | at successful teacher registration; payload `{purpose:"pending", account_id, issued_at}`; HMAC-SHA256 with `SESSION_SIGNING_KEY`; base64url payload + "." + base64url MAC |
| delivery | `quiz_pending` cookie (`HttpOnly`, `SameSite=Lax`, `Secure=COOKIE_SECURE`, `Path=/`, `Max-Age=86400`) **and** a `?token=` link on the redirect |
| validity | signature valid, `purpose` correct, `issued_at` ≤ now, age < 24h, account live and `role='teacher'` |
| viewing | read-only; never consumes, rotates, or writes |
| states | `waiting` (pending), `approved` (active), `expired` (invalid signature, age ≥24h, missing/deleted account, student id) |
| precedence | `?token=` if present, else cookie; an invalid query token is **not** silently replaced by the cookie (it renders `expired`) |
| success login | the cookie is cleared after a successful active login |
| bootstrap | no active teacher → no one can approve; the page says approval requires an active teacher, statically (R-03/R-24 reconciliation, IDN-106) |

**IDN-088.** The token is not a session and cannot authorize any route other than `GET /auth/pending`. A pending teacher who possesses it still cannot log in (R-03).

### 3.6 CSRF token lifecycle
| Event | Action |
|---|---|
| first GET by a guest | issue new `quiz_csrf` signed token |
| GET with valid cookie | reuse (no rotation) |
| GET with invalid/expired/unsigned cookie | reissue |
| successful login | rotate after session creation |
| successful student registration (auto-login) | rotate |
| logout | rotate (guest token for the login page) |
| successful reset consumption | rotate (all sessions were revoked) |
| change-password success | **not** rotated; current session and form context stay valid |
| missing cookie on POST | `403 csrf`; no auto-issue |
| signature mismatch or form mismatch | `403 csrf`; clear the cookie so the next GET reissues |

**IDN-049.** Cookie value format: `base64url(32 random bytes) + "." + base64url(HMAC-SHA256(key, "csrf.v1|" + nonce))`. The form's `csrf_token` mirrors the **entire** cookie value. Verification: parse exactly one dot, recompute the MAC, `hmac.Equal` it, then `subtle.ConstantTimeCompare(cookie, form)`. No length shortcut before a constant-time compare of equal-length values.
**IDN-050.** The CSRF key is `SESSION_SIGNING_KEY`, minimum 32 bytes. An empty or short key,
or the published development default, is a fatal startup error; tests inject a fixed key. A
process-random fallback is forbidden because it silently invalidates every open form on
restart and hides a misconfiguration.
**IDN-051.** Double-submit is safe here because the cookie is `HttpOnly` and signed: a cross-site attacker can neither read the token nor mint a valid cookie via a subdomain. The signature check is what upgrades plain double-submit from "any subdomain can set it" to "only this server can set it".
**IDN-052.** Multi-tab: all tabs share the same cookie and token until login/logout/reset rotates it. A form rendered before rotation fails `403 csrf` after rotation; the copy tells the user to reload. This is intended (R-16), not a bug; tab B's state-changing intent is discarded rather than replayed under a new session.

---
## 4. Cross-cutting credential rules
### 4.1 Email normalization and validation (R-22, INV-01)
**IDN-053.** Every email input (registration, login, profile, student edit) passes through one function:
1. trim leading/trailing Unicode whitespace;
2. reject any interior whitespace or control character;
3. reject any non-ASCII byte (the column is `ascii_general_ci`);
4. lowercase ASCII `A`–`Z` only;
5. reject length > 254 bytes; reject local part > 64 bytes;
6. require exactly one `@`, non-empty local and domain, a dot in the domain, no leading/trailing dot in either part, no consecutive dots;
7. store and compare the normalized value.

**IDN-054.** Uniqueness is case-insensitive across **all** live accounts, both roles, enforced by `uq_accounts_email(email, deleted_at)` plus normalization (INV-01, INV-19). A soft-deleted account frees its email; re-registration creates a fresh row and is allowed. A student may not take a teacher's email, and vice versa.
**IDN-055.** Login looks up by the normalized email. A request that differs only in case authenticates the same account. Login never creates an account and never repairs an unnormalized legacy row.
**IDN-056.** Messages: empty → `Email wajib diisi.`; malformed/too long → `Format email tidak valid.`; duplicate → `Email sudah terdaftar.` (R-19). Duplicate detection deliberately reveals that an address exists; this is a single-school usability trade-off recorded in §12.3.

### 4.2 Password policy and hashing
**IDN-057.** Policy, evaluated on the raw submitted bytes (never trimmed):
- at least 8 Unicode code points;
- at most 72 UTF-8 bytes (bcrypt's input ceiling; bytes beyond 72 would be silently ignored);
- no Unicode whitespace anywhere;
- at least one ASCII digit `0`–`9`;
- at least one ASCII uppercase `A`–`Z`.

**IDN-058.** One message for every policy failure: `Kata sandi minimal 8 karakter, tanpa spasi, mengandung angka dan huruf besar.` (R-19). Over-length gets `Kata sandi maksimal 72 karakter.` so a user is not told to add an uppercase letter to a password that can never be stored whole.
**IDN-059.** Hashing is bcrypt with cost **12** (old spec §12). `password_hash` is the bcrypt string (`$2a$12$…`, 60 chars, fits `VARCHAR(255)`). Verification uses `bcrypt.CompareHashAndPassword`; no pepper, no SHA pre-hash, no reversible storage. Plaintext exists only in the request buffer and is never logged, echoed, or stored.
**IDN-060.** An unknown email still performs exactly one bcrypt compare against a compiled-in cost-12 dummy hash, so response time and body are indistinguishable from a wrong password (IDN-083). A test injects a compare counter and asserts exactly one compare on both paths.
**IDN-061.** Password fields are never repopulated on `422`, never included in `value=`, never put in a flash, and never logged. Confirmation is compared byte-for-byte; no normalization.

### 4.3 Rate limits and storage
**IDN-062.** Storage is a process-local fixed-window counter map guarded by one mutex: key = purpose + identity, value = `{window_start, count}`. It lives in memory, is lost on restart, and is not shared between processes. Old spec §14 pins a single process; multi-instance deployment would need a shared store and is explicitly out of scope (§12.2).
**IDN-063.** Limits (per fixed 15-minute window unless stated):

| Purpose | Key | Limit | Counted events |
|---|---|---|---|
| login | normalized email + client IP | 5 | failed credentials and pending-account refusals |
| login | client IP | 30 | every `POST /login` |
| teacher registration | client IP | 5 | every POST |
| student registration | client IP | 5 | every POST |
| reset consumption | client IP | 10 | every POST |
| change password | account id | 5 | wrong `current_password` only |
| reset-token issue | teacher account id | 10 | every POST |

**IDN-064.** A successful login clears the `(email, IP)` bucket but not the IP bucket. A successful registration, reset, or change clears its per-account bucket. The IP bucket expires by window, never by success.
**IDN-065.** Over-limit returns `429` with `Retry-After: <seconds until window end>` (minimum 1, maximum 900), the rate-limit copy, and **no form** (so a bot gets no fresh CSRF/form context). The `429` is decided before bcrypt and before any DB write.
**IDN-066.** Rate limits are a denial-of-service control, not an authorization control. A spoofed `X-Forwarded-For` must not bypass them; only a transport peer address (or OPS-configured trusted proxy) is used (IDN-027). The 5-per-email bucket is what makes distributed guessing against one account expensive even when the attacker rotates IPs.

### 4.4 Cookie and response hygiene
**IDN-067.** All four cookies (`quiz_session`, `quiz_csrf`, `quiz_pending`, `quiz_reset_reveal`) are `HttpOnly`, `SameSite=Lax`, `Path=/` (reset-reveal scoped to `/teacher/students`), no `Domain`, `Secure` iff `COOKIE_SECURE=true`. No cookie carries user-controlled text. No token is ever placed in a URL except the pending/reset tokens explicitly designed for it.
**IDN-068.** Auth pages, profile, student list/edit, and teacher directory send `Cache-Control: no-store, private` and `Pragma: no-cache`. The pending page and reset page additionally send `Referrer-Policy: no-referrer`. Back-button after logout must not reveal a cached authenticated page.
**IDN-069.** The session cookie is cleared on logout, on expired/revoked detection, and on password reset consumption. Clearing uses the same `Path`/`Secure`/`SameSite` attributes and `Max-Age=0`; a mismatched clear leaves the old cookie alive and is a defect.

### 4.5 Audit and provenance
**IDN-073.** Writes record what the schema provides, no more:
- `accounts.last_login_at` — successful active login only, one write per login, UTC;
- `accounts.approved_by_account_id` + `approved_at` — approval, together;
- `accounts.deleted_by_account_id` — the teacher who deleted a student (root only);
- `sessions.created_at/updated_at`, `ip`, `user_agent` — session provenance;
- `password_reset_tokens.issued_by_account_id`, `created_at`, `expires_at`, `used_at`;
- `deleted_at` on every cascaded child, all equal to the root timestamp.

**IDN-074.** There is no login-attempt, CSRF-failure, or rate-limit audit table. Failures are logged as structured lines (timestamp, route, outcome, hashed identifier where needed) and never as a new table; adding one is a §12 delta and is not required.
**IDN-075.** "Who deleted this student" is answered by `accounts.deleted_by_account_id` on the account row only. Cascaded `students`, `sessions`, `password_reset_tokens`, `registrations`, `attempts`, `answers`, `selections`, and `focus_loss_events` rows carry the same timestamp but NULL provenance (schema §5.2, INV-18).

---
## 5. Commands
### 5.0 Global command contract
**IDN-076.** Every POST in this domain: verifies CSRF first (IDN-049), applies its rate limit (IDN-063), parses the closed field list of §5.x, validates, runs one transaction if it writes, then returns `303`. No POST renders `200`; a validation failure renders the same template at `422`; a state conflict either returns `303` (benign/idempotent) or `404`.
**IDN-077.** Transaction style: `BEGIN`; `SELECT … FOR UPDATE` on the row that is the serialization point; all writes; `COMMIT`. A deadlock (`1213`) is retried exactly once; a second failure is `500`. A duplicate-key (`1062`) on `uq_accounts_email` is mapped to `422 email_taken`, never to `500`.
**IDN-078.** All time comparisons and writes use the database clock (`UTC_TIMESTAMP()`), not the app clock. Forms and display use WIB; storage and `Retry-After` arithmetic are UTC. This keeps `chk_reset_window` and sliding expiry immune to app/DB skew.
**IDN-079.** Field errors are keyed by the exact input name: `full_name`, `email`, `password`, `password_confirm`, `new_password`, `current_password`, `grade_level`, `major_id`, `token`. A handler never invents a key the template does not render.
**IDN-080.** Exact copy used by more than one command (R-19 keys are cited, not paraphrased):

| Key | Indonesian |
|---|---|
| `invalid_credentials` | Email atau kata sandi salah. |
| `email_taken` | Email sudah terdaftar. |
| `password_policy` | Kata sandi minimal 8 karakter, tanpa spasi, mengandung angka dan huruf besar. |
| `password_mismatch` | Konfirmasi kata sandi tidak cocok. |
| `current_wrong` | Kata sandi saat ini salah. |
| `same_password` | Kata sandi baru harus berbeda dari kata sandi saat ini. |
| `invalid_token` | Token tidak valid atau sudah kedaluwarsa. |
| `token_used` | Token sudah digunakan. |
| `csrf` | Sesi formulir tidak valid. Muat ulang halaman. |
| `rate_limited` | Terlalu banyak percobaan. Coba lagi nanti. |
| `session_expired` | Sesi Anda berakhir. Silakan masuk lagi. |
| `pending_teacher` | Menunggu persetujuan guru lain. |
| `must_change` | Ubah kata sandi Anda sebelum melanjutkan. |
| `name_required` | Nama lengkap wajib diisi. |
| `name_long` | Nama lengkap maksimal 100 karakter. |
| `email_required` | Email wajib diisi. |
| `email_invalid` | Format email tidak valid. |
| `password_required` | Kata sandi wajib diisi. |
| `password_long` | Kata sandi maksimal 72 karakter. |
| `confirm_required` | Konfirmasi kata sandi wajib diisi. |
| `current_required` | Kata sandi saat ini wajib diisi. |
| `grade_required` | Pilih kelas. |
| `major_required` | Pilih jurusan yang tersedia. |
| `token_required` | Token wajib diisi. |
| `no_majors` | Belum ada jurusan. Hubungi guru untuk menambahkan jurusan. |
| `saved` | Perubahan tersimpan. |
| `deleted_student` | Data siswa dihapus. |
| `approved` | Akun guru disetujui. |
| `already_approved` | Akun ini sudah disetujui. |
| `reset_issued` | Token atur ulang dibuat. |
| `reset_body` | Berikan token di bawah kepada siswa. Token berlaku 24 jam. |
| `password_reset` | Kata sandi diperbarui. |
| `password_reset_body` | Silakan masuk dengan kata sandi baru. |
| `logged_out` | Anda telah keluar. |
| `welcome` | Berhasil masuk. |
| `approval_unavailable` | Akun guru baru dapat disetujui setelah sekolah memiliki guru aktif. |

**IDN-081.** `full_name` is trimmed, internal runs of whitespace collapse to one ASCII space, must be 1–100 Unicode code points, and must not contain C0/C1 control characters. It may contain punctuation; templates escape it. Empty → `name_required`; too long → `name_long`.
**IDN-082.** `grade_level` accepts only the strings `10`, `11`, `12` in forms; anything else (including Roman numerals; those are accepted only in list query filters, §6.1) → `grade_required`. `major_id` must parse as a positive integer and name a live major; otherwise → `major_required`. A soft-deleted major is not selectable and does not validate.
**IDN-083.** On a `422`, the response is the same template, status `422`, with every non-secret field repopulated (name, email, grade, major, token) and passwords blank. The form-level alert for `invalid_credentials` is a `Flash{Kind:"error"}` at the top of the form, not a field error on `email` or `password`, so the message does not point at which field was wrong.
**IDN-084.** Idempotency is per command and stated there. In general: GET is safe and side-effect free; a repeated successful POST after its redirect is a new request with a rotated CSRF token and is rejected; a repeated POST with the same body is handled by the unique key, row lock, or conditional update described in §9.

### IDN-C01 — `GET /`
- **Actor:** any. **Authorization:** `public`. **Preconditions:** none. **Input:** none; the query string is ignored.
- **Logic:** resolve the session (IDN-020). No session or invalid session → `303 /login`. `must_change_password=1` → `303 /change-password` + `must_change`. Student → `303 /student`. Teacher → `303 /teacher`. Pending/deleted accounts cannot have a valid session by IDN-041.
- **Effects:** none beyond the normal sliding write for a valid session. **Status/redirect:** always `303`; never `200`, never renders a template.
- **Errors:** none of its own; a DB failure during session lookup → `500` page. **Provenance/idempotency:** no writes beyond sliding; repeated calls converge on the same target.

### IDN-C02 — `GET /login`
- **Actor:** visitor. **Authorization:** `guest-only` (IDN-024). **Preconditions:** no valid session. An authenticated request is redirected before rendering.
- **Input/model:** `LoginPage{Email:""}`; no role field; no `remember_me`; no `next`. CSRF cookie/token present.
- **Effects:** issues the CSRF cookie if absent. **Status:** `200`; `Cache-Control: no-store`. Errors from a prior POST are rendered by the POST itself, not by a redirect to this GET.
- **Security:** the page never reveals whether any account exists, how many teachers are active, or whether bootstrap has run.

### IDN-C03 — `POST /login`
- **Route:** `POST /login`. **Actor:** visitor. **Authorization:** `guest-only`; an already-authenticated request is `303` home without processing credentials.
- **CSRF:** required (IDN-049). Missing/mismatch → `403 csrf`.
- **Rate limit:** check `(email, IP)` and IP buckets (IDN-063). Over limit → `429 rate_limited` + `Retry-After`, before any lookup or bcrypt.
- **Input:** `email`, `password` (raw, never trimmed). Both required; missing email → `422 email_required`; missing password → `422 password_required`. A malformed email is treated as an ordinary credential failure, not a field error, to avoid a format oracle.
- **Lookup:** normalize the email (IDN-053); select the live account by email. If none, or `deleted_at != sentinel`, perform one dummy bcrypt compare (IDN-060) and fail with `422 invalid_credentials`; email repopulated; no session; no `last_login_at`.
- **Wrong password:** one bcrypt compare fails → same `422 invalid_credentials`; increment the `(email, IP)` failure bucket.
- **Pending teacher with correct password:** do **not** create a session, do not update `last_login_at`; return `403` with `pending_teacher`. If a valid `quiz_pending` token for this account exists, the 403 page's action link is `/auth/pending?token=<token>`; otherwise the link is `/login`. Increment the failure bucket (a pending account is not an authenticated success).
- **Active with correct password:** open a transaction; insert `sessions` (`account_id`, `token_hash`, `expires_at = UTC_TIMESTAMP()+60m`, `ip`, `user_agent`), update `accounts.last_login_at = UTC_TIMESTAMP()`; commit. Set `quiz_session` to the new raw token, rotate `quiz_csrf` (IDN-049), clear `quiz_pending`. `must_change_password=1` → `303 /change-password` + `must_change`; else `303 /teacher` or `/student`.
- **Fixation:** any inbound `quiz_session` is ignored; the new token is minted from `crypto/rand` after successful verification (IDN-040).
- **Postconditions:** exactly one new live session; `last_login_at` updated once; no other session is revoked (multi-device is allowed, IDN-044). **Status:** `303` on success; `422` on failure; `403` on pending; `429` on limit; `500` on DB failure.
- **Idempotency:** a second identical POST carries the pre-rotation CSRF token and fails `403 csrf`; a fresh form after the redirect is a new login and creates a second session (allowed). **Provenance:** `sessions.created_at`, `ip`, `user_agent`; `accounts.last_login_at`. No password or token is logged.

### IDN-C04 — `GET /register/teacher`
- **Actor:** visitor. **Authorization:** `guest-only`.
- **Model:** `RegisterPage{Role:teacher, MinPassword:8}` with empty fields; majors not loaded. **Status/effects:** `200`, no-store; issues CSRF if absent. The copy states approval is required; it does not mention bootstrap existence or active-teacher count.

### IDN-C05 — `POST /register/teacher`
- **Route:** `POST /register/teacher`. **Actor:** visitor. **Authorization:** `guest-only`.
- **CSRF/rate limit:** required; 5 POSTs / 15 min / IP.
- **Input:** `full_name`, `email`, `password`, `password_confirm` (contract §18). Validate IDN-053/057/058/081; confirmation byte-equal else `password_mismatch`.
- **Transaction:** single insert into `accounts` with `role='teacher'`, `status='pending'`, `approved_at=NULL`, `approved_by_account_id=NULL`, `must_change_password=0`. No profile row. Unique-key race → `422 email_taken`.
- **Effects:** the account is **never** auto-activated, regardless of how many teachers exist (R-03). A pending token is minted (IDN-088) and set as `quiz_pending`; the redirect carries `?token=`.
- **Postconditions:** exactly one pending account; zero sessions; no `last_login_at`; zero active teachers added. **Status:** `303 /auth/pending?token=<raw>`; `422` validation; `429`; `500`. **Provenance/idempotency:** `created_at`; a repeat with the same email fails `422 email_taken` (including a concurrent duplicate — one wins, one 422). Re-registering after the account is soft-deleted would succeed, but there is no route that deletes a teacher.

### IDN-C06 — `GET /register/student`
- **Actor:** visitor. **Authorization:** `guest-only`.
- **Model:** `RegisterPage{Role:student, MinPassword:8, Majors: live majors sorted by name asc, id asc, GradeLevel:0}`.
- **Empty majors:** if no live major exists, render `no_majors` as a form-level info note and disable the submit control; the server still rejects every POST (IDN-082). This state is expected only before the majors seed. **Status/effects:** `200`, no-store; issues CSRF if absent.

### IDN-C07 — `POST /register/student`
- **Route:** `POST /register/student`. **Actor:** visitor. **Authorization:** `guest-only`.
- **CSRF/rate limit:** required; 5 POSTs / 15 min / IP.
- **Input:** `full_name`, `email`, `password`, `password_confirm`, `grade_level`, `major_id` (contract §18). All validations from IDN-053/057/058/081/082.
- **Transaction:** `BEGIN`; insert `accounts(role='student', status='active', must_change_password=0)`; insert `students(account_id, grade_level, major_id)`; `COMMIT`. The profile insert is mandatory: an account without a profile is invisible to every roster and violates INV-02. Any failure rolls back both.
- **Session:** after commit, insert a session exactly as IDN-C03 (a login event), set `quiz_session`, rotate `quiz_csrf`, update `last_login_at`. The student is signed in (contract §9.2).
- **Race:** two concurrent POSTs with the same email → one inserts, the other hits `uq_accounts_email` and returns `422 email_taken`; no orphan account, no orphan profile.
- **Postconditions:** one active student account, one live profile, one session; eligible for PAR flows immediately. **Status:** `303 /student` + `welcome`; `422`; `429`; `500`. **Provenance:** `accounts.created_at`, `students.created_at`, session audit fields. Passwords never logged.
- **Idempotency:** not idempotent by design; the unique key is the arbiter.

### IDN-C08 — `GET /reset-password`
- **Actor:** visitor. **Authorization:** `guest-only`.
- **Input:** optional `?token=` for prefill only (the teacher may hand a link or a token string).
- **Logic:** render `ResetPasswordPage{Token:<trimmed query value>}`; do **not** hash, look up, validate, or consume the token on GET. A wrong token is discovered only on POST. This prevents a GET-based token oracle and keeps link scanners from consuming anything (consumption is POST-only anyway). **Status/effects:** `200`, `no-store`, `Referrer-Policy: no-referrer`; issues CSRF if absent.
- **Copy:** explains the token comes from a teacher; no email field (contract §9.3).

### IDN-C09 — `POST /reset-password`
- **Route:** `POST /reset-password`. **Actor:** visitor (the student, or anyone holding the token). **Authorization:** `guest-only`; a signed-in user is redirected to home by IDN-024.
- **CSRF/rate limit:** required; 10 POSTs / 15 min / IP.
- **Input:** `token`, `new_password`, `password_confirm` (contract §18 — note: the current template's `password` name is a template defect, §12.2). Trim `token`; reject empty → `token_required`; reject >128 chars as `invalid_token`. Validate new password policy and confirmation.
- **Lookup/lock:** `BEGIN`; `SELECT … FROM password_reset_tokens t JOIN students s ON s.id=t.student_id JOIN accounts a ON a.id=s.account_id WHERE t.token_hash=SHA2(?,256) AND t.deleted_at=sentinel AND s.deleted_at=sentinel AND a.deleted_at=sentinel AND a.role='student' FOR UPDATE`. `t` is the serialization point.
- **Checks under lock:** `used_at IS NOT NULL` → rollback, `422 token_used`; `expires_at <= UTC_TIMESTAMP()` → rollback, `422 invalid_token`; missing row → `422 invalid_token` (no distinction between forged, deleted, and student-deleted).
- **Effects (same transaction):** `UPDATE accounts SET password_hash=?, must_change_password=0 WHERE id=?`; `UPDATE password_reset_tokens SET used_at=UTC_TIMESTAMP() WHERE id=? AND used_at IS NULL` (assert one row); `UPDATE sessions SET deleted_at=UTC_TIMESTAMP() WHERE account_id=? AND deleted_at=sentinel` (revoke **all** sessions, R-16).
- **Postconditions:** new bcrypt hash; flag `0` (R-04: reset is not a temporary password); token single-used; every session of that student invalid; no session created. **Status:** `303 /login` + `password_reset` (body `password_reset_body`); `422` for policy/confirmation/token; `429`; `500`.
- **Concurrency:** two concurrent submissions of the same token serialize on the row lock; the second sees `used_at` set and gets `422 token_used`. A reset racing a change-password: both update the account row; whichever commits last wins, both revoke sessions, and both produce a valid final credential. No half-state. **Provenance:** `used_at`; the issuing teacher is already on the row. Raw token never logged; the page stops echoing the token after success (redirect).
- **Idempotency:** deliberately not idempotent — single use is the security property.

### IDN-C10 — `GET /change-password`
- **Actor:** any authenticated (student or teacher), including a `must_change_password=1` session. **Authorization:** `any-auth`. **Preconditions:** a valid session (IDN-041). Unauthenticated → `303 /login` + `session_expired`.
- **Model:** `ChangePasswordPage{}` with no fields; current/new/confirm inputs are always blank. **Status/effects:** `200`, no-store; issues/reuses CSRF. The page is reachable during the must-change guard and is the guard's redirect target, so it must render without a role home.
- **Copy:** success `password_reset`; wrong current `current_wrong`; same password `same_password`.

### IDN-C11 — `POST /change-password`
- **Route:** `POST /change-password`. **Actor:** any authenticated. **Authorization:** `any-auth` (the only write, with logout, that a must-change session may perform).
- **CSRF/rate limit:** required; 5 wrong-current attempts / 15 min / account.
- **Input:** `current_password`, `new_password`, `password_confirm` (contract §18). All required; policy on `new_password`; byte-equal confirmation.
- **Transaction/lock:** `BEGIN`; `SELECT id, password_hash, must_change_password FROM accounts WHERE id=? AND deleted_at=sentinel FOR UPDATE`. Verify `current_password` against the hash **under the lock**; wrong → rollback, `422 current_wrong`, increment the account bucket. Verify `new_password` is not the current password (bcrypt compare against the same hash) → `422 same_password`. Re-check policy under the lock (the input is unchanged but this keeps validation and write one unit).
- **Effects:** `UPDATE accounts SET password_hash=?, must_change_password=0 WHERE id=?`; `UPDATE sessions SET deleted_at=UTC_TIMESTAMP() WHERE account_id=? AND id<>? AND deleted_at=sentinel` (revoke every **other** session, keep the current one); `COMMIT`.
- **Postconditions:** new hash; flag cleared; current session live and slid; every other device is signed out; no CSRF rotation (R-16 rotates only login/logout). **Status:** `303 /teacher` for a teacher, `303 /profile` + `password_reset` for a student; `422`; `429`; `500`.
- **Concurrency:** the account row lock serializes two submissions; the second re-reads the new hash, so the old `current_password` fails with `422 current_wrong` rather than double-writing. A concurrent reset also locks the account (IDN-C09) and revokes all sessions, including this one: the change either commits first (then reset overwrites) or sees its session already revoked on the next request. **Provenance:** `updated_at`; no password history. **Idempotency:** a sequential replay fails on `current_password`; the outcome never depends on request order beyond last-writer-wins on the hash.

### IDN-C12 — `GET /auth/pending`
- **Actor:** anyone holding a signed token; no session required. **Authorization:** `public`.
- **Input:** `?token=` (preferred) else `quiz_pending`. Both absent → `expired` state. Token parsing never panics on malformed base64; any failure is `expired`.
- **Token verification (IDN-088):** split exactly one `.`, decode payload/MAC, `hmac.Equal` the MAC, check `purpose=="pending"`, `issued_at <= now`, `now-issued_at < 24h`. Then load the account by `account_id`.
- **States:**
  - `waiting` — live account, `role='teacher'`, `status='pending'`. Title "Menunggu persetujuan"; body "Pendaftaran Anda sudah kami terima."; note `pending_teacher`; static line `approval_unavailable`; no login button; only "Kembali ke masuk".
  - `approved` — live account, `role='teacher'`, `status='active'`. Title "Akun disetujui"; body "Akun Anda sudah aktif dan siap dipakai."; primary link "Masuk ke akun" → `/login`.
  - `expired` — invalid signature, age ≥24h, missing/soft-deleted account, or a non-teacher id. Title "Persetujuan kedaluwarsa"; body `invalid_token`; links "Kembali ke masuk" and "Daftar ulang" → `/register/teacher`.
- **Data exposure:** only `full_name` (from the account) and the state. Never email, id, role, timestamps, approver name, or whether any active teacher exists. A malformed/forged token and a genuinely expired one are byte-identical pages.
- **Effects:** none. Viewing does not consume, rotate, extend, or write. **Status:** `200` always, `no-store`, `Referrer-Policy: no-referrer`. A DB failure resolving the account → `500`, not `expired` (do not mask outages as token expiry).
- **Copy note:** "Daftar ulang" on `expired` is a dead end when the original pending account still holds the email (`422 email_taken`). The page keeps the link because a deleted/never-created account can genuinely re-register; the `email_taken` error then explains the block. This is a known limitation, §12.3.

### IDN-C13 — `POST /logout`
- **Route:** `POST /logout`. **Actor:** any, including a must-change session. **Authorization:** `public` (idempotent); CSRF still required.
- **Input:** none; any body fields are ignored.
- **Logic:** if a valid session exists, `UPDATE sessions SET deleted_at=UTC_TIMESTAMP() WHERE id=? AND deleted_at=sentinel`; if it does not, do nothing. Clear `quiz_session` (`Max-Age=0`, same attributes); rotate `quiz_csrf`; clear `quiz_pending` if present. **Status:** `303 /login` + `logged_out`, always. Never `403` unless CSRF fails; never `404` for a missing session. **Effects/postconditions:** at most one session revoked; every other device unchanged; the browser holds a fresh guest CSRF token.
- **Security:** GET `/logout` is not registered and falls to the 404 page; a cross-site image/link cannot sign a user out because logout is POST + CSRF.
- **Idempotency:** repeated POSTs with the same (now dead) cookie still return `303 logged_out` once CSRF passes; the old token cannot be replayed as a session.

### IDN-C14 — `GET /profile`
- **Actor:** active student. **Authorization:** `student`. Teacher → `404` (R-05: no teacher profile); visitor → `303 /login`.
- **Model:** `ProfilePage{FullName, Email, GradeLevel, MajorID, Majors: live majors}` plus the read-only class label `XII IPA` and "Hubungi guru untuk mengubah kelas atau jurusan."
- **Query:** live `accounts` + live `students` + live `majors` by `account_id` of the session. Grade and major are **read-only**; no `:id` path parameter exists, so a student cannot address another student. **Status/effects:** `200`, no-store; issues/reuses CSRF. Never exposes sessions, last login, reset tokens, or `must_change_password`.

### IDN-C15 — `POST /profile`
- **Route:** `POST /profile`. **Actor:** active student. **Authorization:** `student`.
- **CSRF:** required. **Rate limit:** none beyond CSRF; the command is not credential-guessing.
- **Input:** `full_name`, `email` only (contract §18, §10.3). `grade_level`/`major_id` in the body are ignored, not errors: the closed form does not contain them and a hand-crafted value must not change eligibility. `password` fields are ignored here.
- **Validation:** IDN-081 and IDN-053. Uniqueness excluding self: `email=? AND id<>? AND deleted_at=sentinel`; a same-address case change is a no-op success. The unique key remains the final arbiter under a race → `422 email_taken`.
- **Transaction:** `UPDATE accounts SET full_name=?, email=? WHERE id=? AND deleted_at=sentinel`. No `students` write; no session revocation (R-22); no `must_change_password` change; no CSRF rotation.
- **Effects/postconditions:** the account's live name/email change immediately. Every screen that joins `accounts` shows the new name on the next read, including history, results, recaps, and monitor rows — names are current, not snapshotted (R-25). Existing registrations, attempts, and grades are untouched.
- **Status:** `303 /profile` + `saved`; `422`; `500`.
- **Provenance/idempotency:** `updated_at`; repeating the same values is a harmless no-op and still `303`. Two tabs: last writer wins; no optimistic lock exists.
- **Cross-role:** an email owned by a teacher or another student is `422 email_taken`; a soft-deleted owner's email is free (INV-19).

### IDN-C16 — `GET /teacher/students`
- **Actor:** active teacher. **Authorization:** `teacher`, school-wide (R-05); students and visitors never reach it.
- **Query:** filters, search, sort, page per §6.1. Invalid values fall back to defaults, never `422` (R-21). Per page 10.
- **Model:** `StudentList{Students, Pagination, Tabs, Majors, Query, Empty, ResetToken}`; `ResetToken` is populated only when a valid reveal cookie is present **and** a matching live unused token exists (IDN-105). **Status/effects:** `200`, no-store; one read-only transaction for rows + total. Issues/reuses CSRF.
- **Empty vs filtered-empty:** §6.1. A page beyond the last page clamps to the last page; an invalid sort/grade/major falls back to the default rather than emptying the list.

### IDN-C17 — `GET /teacher/students/:id`
- **Actor:** active teacher. **Authorization:** `teacher`; `:id` is `students.id`, not `accounts.id`. **Preconditions:** a live `students` row joined to a live `accounts` row with `role='student'`. Missing, soft-deleted, a teacher's account id, or a malformed id → `404`.
- **Model:** `StudentForm{ID, FullName, Email, GradeLevel, MajorID, Majors, Action, CancelHref, IsNew:false}`. Only live majors; current major preselected. **Status/effects:** `200`, no-store. No attempts, scores, or history are shown here (that is ATT/PAR).
- **Security:** any active teacher may edit any student (school-wide, R-05); there is no per-teacher student ownership. This is intentional, not an IDOR.

### IDN-C18 — `POST /teacher/students/:id`
- **Actor:** active teacher. **Authorization:** `teacher`; live student target.
- **CSRF:** required. **Input:** `full_name`, `email`, `grade_level`, `major_id` (contract §18).
- **Validation:** IDN-081, IDN-053, IDN-082; email unique excluding this account; grade/major must be live. All errors re-render the edit form at `422` with values repopulated.
- **Transaction/lock:** `BEGIN`; `SELECT s.id, a.id FROM students s JOIN accounts a ON a.id=s.account_id WHERE s.id=? AND s.deleted_at=sentinel AND a.deleted_at=sentinel AND a.role='student' FOR UPDATE`; update `accounts.full_name/email` and `students.grade_level/major_id` in the same transaction; `COMMIT`.
- **Effects:** the student's current class/major change for every live read. Future PAR eligibility uses the new values; existing registrations and attempts are **not** deleted or invalidated (R-12: eligibility is checked at registration/attempt start, and a student who registered while eligible may finish). Displayed history class labels become the new class (names/classes are current). **Status:** `303 /teacher/students/:id` + `saved`; `422`; `404`; `500`.
- **Concurrency:** the row lock serializes two teachers; last commit wins. If a delete commits first, the lock/select sees no live row → `404`; if the edit commits first, the delete then cascades the edited row. No lost-child state. **Provenance/idempotency:** `accounts.updated_at`, `students.updated_at`; repeated identical submission is a no-op success.

### IDN-C19 — `POST /teacher/students/:id/delete`
- **Actor:** active teacher. **Authorization:** `teacher`; live student target.
- **CSRF:** required. No confirmation field is required server-side; the dialog is UI. A hand-crafted POST with a valid CSRF token is a deliberate delete.
- **Transaction:** `BEGIN`; take `@ts = UTC_TIMESTAMP()` once; `SELECT a.id FROM accounts a JOIN students s ON s.account_id=a.id WHERE s.id=? AND a.role='student' AND a.deleted_at=sentinel AND s.deleted_at=sentinel FOR UPDATE`; if absent → rollback `404`. Then, all with `deleted_at=@ts` and only where the sentinel is still live:
  1. `accounts` (`deleted_by_account_id = actor`);
  2. `students` (by `account_id`);
  3. `sessions` (by `account_id`);
  4. `password_reset_tokens` (via `student_id`);
  5. `registrations` (by `student_id`);
  6. `quiz_attempts` (via those registrations);
  7. `attempt_answers` (via those attempts);
  8. `attempt_answer_selections` (via those answers);
  9. `focus_loss_events` (via those attempts).
  `COMMIT`.
- **Never touched:** quizzes, questions, options, majors, anonymous participants, other students, the acting teacher. Teacher deletion is forbidden and has no command (R-14, R-05).
- **Postconditions:** the student disappears from the roster, login fails (account hidden), every live session is invalid on its next request, reset tokens are dead, PAR history/registrations and ATT attempts/results/recaps/monitor rows disappear. One timestamp identifies the event; only the account row carries the actor (INV-18). **Status:** `303 /teacher/students` + `deleted_student`; `404` when already deleted/missing; `500` on failure (transaction rolls back; no partial cascade).
- **Concurrency:** racing edit/reset/approval locks or updates the same account/student rows; either the delete wins (other op sees no live row/`invalid_token`) or the other op commits first and the delete then stamps it. A second delete → `404`. **Provenance:** `accounts.deleted_by_account_id = actor`; children NULL. **Idempotency:** not idempotent; the second call is `404`, which is the correct "already gone" answer.

### IDN-C20 — `POST /teacher/students/:id/reset-token`
- **Actor:** active teacher. **Authorization:** `teacher`; live student target.
- **CSRF/rate limit:** required; 10 issues / 15 min / teacher account. **Preconditions:** live student + live student account. Any active teacher may issue for any student (R-05). Teachers cannot issue for teachers or for themselves.
- **Transaction/lock:** `BEGIN`; lock the `students` row `FOR UPDATE`; `UPDATE password_reset_tokens SET deleted_at=UTC_TIMESTAMP() WHERE student_id=? AND used_at IS NULL AND deleted_at=sentinel` (supersede every live unused token); insert a new row `{student_id, issued_by_account_id=actor, token_hash=SHA256(raw), expires_at=DATE_ADD(UTC_TIMESTAMP(), INTERVAL 24 HOUR)}`; `COMMIT`.
- **Effects:** exactly one live unused token for the student. The raw token is placed in a signed `quiz_reset_reveal` cookie (IDN-105), scoped to `/teacher/students`, `Max-Age=300`, `HttpOnly`, `SameSite=Lax`, `Secure` per config, and is **not** placed in the redirect URL. The list page renders it once and clears it. **Status:** `303 /teacher/students` + `reset_issued`; `404`; `429`; `500`.
- **Postconditions:** no session is revoked yet; consumption (IDN-C09) does that. The superseded tokens fail with `invalid_token`, not `token_used` (they were revoked, not used).
- **Concurrency:** two clicks serialize on the student lock; the second supersedes the first, so at most one token is live. The panel shows the last token; the first is dead. Documented as intentional (single live token). **Provenance/idempotency:** `issued_by_account_id`, `created_at`, `expires_at`. Not idempotent by design; a retry mints a new token and invalidates the previous.

### IDN-C21 — `GET /teacher/teachers`
- **Actor:** active teacher. **Authorization:** `teacher`.
- **Query:** live teachers only, split by status. **Pending** section: `status='pending'`, `created_at ASC, id ASC` (oldest request first). **Active** section: `status='active'`, `full_name ASC, id ASC`. No pagination: the contract's `TeachersPage` has two slices and R-21's page sizes cover students/quizzes/grading, not teachers. At the school's scale the whole list is small.
- **Model:** `TeachersPage{Pending, Active, Empty}`; rows carry name, email, status label, `ApprovedBy` display name (self-join, nullable), `ApprovedAt` WIB, `CanApprove` (pending only), `ApproveHref`.
- **Empty states:** pending empty → "Tidak ada pendaftaran guru yang menunggu." with body "Pendaftaran guru baru akan muncul di sini."; both empty → the page still renders both section headers; no page-level 404. (An empty active section is unobservable in practice: only an active teacher can view this page, and if none existed no one could log in.) **Status/effects:** `200`, no-store. Never exposes password hashes, sessions, or `must_change_password`.
- **Security:** no edit/deactivate/delete/reject action is rendered or accepted; a hand-crafted path for those falls to 404. Teacher email is visible to peers (contract §16.4), which is intended.

### IDN-C22 — `POST /teacher/teachers/:id/approve`
- **Actor:** active teacher. **Authorization:** `teacher`; `:id` is `accounts.id` of a live teacher.
- **CSRF:** required.
- **Target resolution:** live account, `role='teacher'`. A student id, a missing id, a soft-deleted id, or a malformed id → `404` (role-typed target, INV-03). A pending teacher can never call this: pending accounts cannot authenticate (R-03), so self-approval is structurally impossible.
- **Transaction:** one conditional statement is the lock:
  `UPDATE accounts SET status='active', approved_at=UTC_TIMESTAMP(), approved_by_account_id=? WHERE id=? AND role='teacher' AND status='pending' AND deleted_at=sentinel`.
  - 1 row → approved.
  - 0 rows → re-read the target: missing/student/deleted → `404`; `status='active'` → **idempotent success** (`303` + `already_approved`); otherwise `404`.
- **Effects/postconditions:** `chk_accounts_teacher_approval` holds because `status='active'` and `approved_at` are set together; `approved_by_account_id` is the actor. The target can now log in; its pending page resolves `approved` (the token is not consumed). No session is created for the target; no notification is sent; the target's quizzes (none yet) are unaffected. **Status:** `303 /teacher/teachers` + `approved`, or `+ already_approved` for the idempotent case; `404`; `500`.
- **Concurrency:** two teachers approving simultaneously: the predicate lets exactly one `UPDATE` match; the other affects 0 rows and follows the re-read path to idempotent success. No double approver, no constraint violation.
- **Bootstrap:** if no active teacher exists, this route is unreachable (no valid session), which is the correct expression of R-03/R-24; the runner must insert the first active teacher. **Provenance/idempotency:** `approved_by_account_id`, `approved_at`, `updated_at`. Idempotent: approval is monotonic and a repeat is a success, not a conflict.

---
## 6. Queries and read models
### 6.1 Student roster (`GET /teacher/students`)
**IDN-090.** Base relation: `accounts a JOIN students s ON s.account_id=a.id JOIN majors m ON m.id=s.major_id`, all three at the live sentinel, `a.role='student'`. A student with a missing/drifted major is rendered with class `—` rather than dropped from the roster; the join is `LEFT JOIN` for that reason, because a disappearing student is worse than a missing label.
**IDN-091.** Filters (all optional, AND-combined):

| Query | Accepted | Invalid/missing fallback | SQL |
|---|---|---|---|
| `grade` | `10`, `11`, `12`, `x`, `xi`, `xii`, `kelas x`… (case-insensitive) | all grades | `s.grade_level = ?` |
| `major_id` | positive id of a **live** major | all majors | `s.major_id = ?` |
| `q` | trimmed, 0–100 chars | no search | `(a.full_name LIKE ? ESCAPE '\\' OR a.email LIKE ? ESCAPE '\\')` with `%`/`_`/`\\` escaped, `%q%` |
| `sort` | `newest`, `oldest`, `name_asc`, `name_desc` | `newest` | see IDN-092 |
| `page` | integer ≥1 | `1` | clamped to `[1, pages]`, `pages≥1` |
| `per_page` | not accepted; fixed | `10` | R-21 |

**IDN-092.** Sort and tie-break:

| Sort | Order |
|---|---|
| `newest` | `a.created_at DESC, a.id ASC` |
| `oldest` | `a.created_at ASC, a.id ASC` |
| `name_asc` | `a.full_name ASC` (collation `utf8mb4_unicode_ci`), `a.id ASC` |
| `name_desc` | `a.full_name DESC`, `a.id ASC` |

The tie-break is always `id ASC`, never `id DESC`, so a page boundary is stable (R-21).

**IDN-093.** `Attempts` on each row is `COUNT(*)` of live `quiz_attempts` whose live `registrations.student_id = s.id`. It counts every attempt, submitted or in progress, on every quiz, because quizzes are never deleted once attempts exist (R-10). It is not a registration count and not a distinct-quiz count.
**IDN-094.** `Total` is `COUNT(*)` over the filtered relation. Rows and total are read in one read-only transaction (`REPEATABLE READ`) so pagination never disagrees with itself. Page size is 10; `Offset=(page-1)*10`; page clamp uses `max(1, min(page, pages))` with `pages=max(1, ceil(total/10))`.
**IDN-095.** Tabs are links `?grade=` with labels Semua / Kelas X / Kelas XI / Kelas XII; the active tab is the one matching the effective grade. The majors dropdown lists live majors `name ASC, id ASC`; it is a filter control, not an editor.
**IDN-096.** Empty states:
- no students at all: title "Belum ada siswa." body "Siswa yang mendaftar akan muncul di sini." no filter-reset action;
- filters/search applied and zero rows: title "Tidak ada siswa yang cocok." body "Ubah kata kunci atau filter kelas dan jurusan." action "Hapus filter" → `/teacher/students`;
- the two states are distinct in the model and in the template (R-21).

### 6.2 Student edit and profile read models
**IDN-097.** `GET /teacher/students/:id` loads `accounts.full_name/email`, `students.grade_level/major_id`, and the live majors list. It does not load attempts, scores, registrations, sessions, or reset tokens. A malformed `:id` (non-numeric, ≤0, overflow) is `404`, not a panic and not a `500`.
**IDN-098.** `GET /profile` loads only the session account's own row: live `accounts` + live `students` + live `majors`. It derives `ClassLabel` from the current grade and major (Roman grade + major name, e.g. `XII IPA`); a missing major renders `—` and the "Hubungi guru" note remains.
**IDN-099.** Neither read model exposes `password_hash`, `must_change_password`, `last_login_at`, `deleted_at`, `approved_by`, or internal ids the template does not need. The edit form carries `ID` for its action path; it is not a secret.

### 6.3 Teacher directory
**IDN-100.** `GET /teacher/teachers` is two queries (or one query split in Go) over live `accounts` with `role='teacher'`: pending ordered `created_at ASC, id ASC`; active ordered `full_name ASC, id ASC`. `ApprovedBy` is a left self-join to the approver's `full_name`; NULL for pending and for any historical row whose approver is null (should not occur for active rows, because the check constraint pairs the columns). No pagination, no search, no filter: the contract's model has no list controls.

### 6.4 Reset-token reveal panel
**IDN-105.** The raw token is never stored in a page model across requests and never put in a URL. On issue, IDN-C20 sets:

`quiz_reset_reveal = base64url(payload) + "." + base64url(HMAC-SHA256(key,"resetreveal.v1|"+payload_json))`, where payload is `{purpose:"resetreveal", teacher_account_id, student_id, token, expires_at}`; cookie `HttpOnly`, `SameSite=Lax`, `Secure` per config, `Path=/teacher/students`, `Max-Age=300`.

On `GET /teacher/students`: verify signature/purpose/teacher id; SHA-256 the embedded token and confirm a live, unused, unexpired row exists for that `student_id` **and** `issued_by_account_id = teacher`; if yes, render `ResetToken{Token, ExpiresAt (WIB)}` and clear the cookie in the same response; if no, suppress the panel and clear the cookie. A tampered, expired, or foreign cookie renders no panel and no error — it is not an oracle. The panel shows the token once; a reload shows nothing.

### 6.5 Pending page read model
**IDN-106.** `GET /auth/pending` derives exactly one of `waiting|approved|expired` (IDN-C12) and exposes `FullName` only for `waiting`/`approved`. `LoginHref` is `/login`; the approved state renders the login button; the waiting state renders no action that implies approval is imminent. The page reads the real account only: there is no preview switch and `?state=` is never read.

### 6.6 Display and formatting rules for this domain
**IDN-107.** Timestamps are stored UTC and rendered WIB (`20 Okt 2026`, `09.42 WIB`); the pending/expiry copy uses the same formatter. Counts (attempts, totals) use no decimals and the Indonesian thousands separator. Class label is Roman grade + major; a missing major is `—`. Status labels come from `viewmodel.AccountStatus.Label()`; raw storage values never reach the DOM.
**IDN-108.** List query values are echoed into the form/links from the **effective** values (after fallback), so a bad `?sort=DROP` renders as `newest` and cannot be reflected as text (R-21: invalid query values fall back, never 422).

---
## 7. Invariants owned
### 7.1 Inherited, cited not restated
| Invariant | How IDN enforces it |
|---|---|
| INV-01 live email unique case-insensitively | `uq_accounts_email` + IDN-053 normalization; duplicate → `422` |
| INV-02 one live student profile per student account | one transaction in IDN-C07; `uq_students_account`; role check before profile use |
| INV-03 role-typed actor FKs | approver is resolved as `role='teacher'` (IDN-C22); delete actor is an active teacher; `students.account_id` only joins `role='student'` |
| INV-04 students active, teachers pending until approved, must-change blocks | IDN-031, IDN-070, IDN-C05/C07/C22 |
| INV-19 unique keys include `deleted_at`; reuse allowed | email re-registration after soft delete (IDN-054); no IDN read ever ignores the sentinel |
| INV-21 a session needs a live active account; expired/deleted sessions authenticate nothing | IDN-041, IDN-047 |
| INV-22 reset tokens student-only, teacher-issued, 24h, single-use | IDN-048, IDN-C09/C20 |
| INV-28 every POST is CSRF-protected except the beacon | IDN-049; no IDN exemption |
| INV-29 no hard delete and no restore route | IDN-C19 is soft only; no restore route exists |
| INV-03/INV-04/INV-18 school-wide teacher authority and cascade consistency | IDN-C19, IDN-C22, §10 |

### 7.2 Domain invariants
**IDN-I01.** A request is authenticated iff a live unexpired session joins a live active account; no cookie claim, query value, or form field can elevate it.
**IDN-I02.** Every account row is created with an explicit `role` and `status`; no insert relies on a default. Students are inserted with their profile in the same transaction.
**IDN-I03.** A pending teacher never has a session and never appears as `CurrentUser`; approval is the only path from pending to active, and it requires an already-active teacher.
**IDN-I04.** A password is stored only as a cost-12 bcrypt hash; plaintext never appears in the database, a log, a template, a flash, or a URL.
**IDN-I05.** Every POST that changes identity state consumes a valid CSRF token bound to the browser's signed cookie; the only exempt route in the system is ATT's beacon.
**IDN-I06.** At most one live unused reset token exists per student; issuing a new one revokes the old in the same transaction.
**IDN-I07.** A reset token is usable exactly once and only within 24 hours; consumption revokes every session of that student and clears `must_change_password`.
**IDN-I08.** Changing a password revokes every other session and keeps exactly the current one; logging out revokes exactly the current one.
**IDN-I09.** A teacher account can never be edited, deactivated, rejected, or deleted by any route; a student account can be edited/deleted only by an active teacher and only through the owned routes.
**IDN-I10.** Student deletion cascades the full set in IDN-C19 with one timestamp; no child is left live under a deleted account (INV-18).
**IDN-I11.** A `must_change_password=1` session can reach only the two password-change methods and logout; the flag clears atomically with the new hash.
**IDN-I12.** Every list read clamps invalid pagination/sort/filter input to a default; no list route returns `422`.
**IDN-I13.** No auth response reveals whether an email exists except through the documented `email_taken` validation on registration/profile edit, and no login response distinguishes unknown-email from wrong-password in body, status, or bcrypt count.
**IDN-I14.** The role home is derived from the account role, never from a request parameter; no `next`/`return_to` exists (IDN-025).
**IDN-I15.** The pending token authorizes only the pending-status page; it is never a session and never survives the 24-hour window.
**IDN-I16.** All timestamps written by this domain are UTC and all comparisons use the DB clock; display is WIB (INV-20).

---
## 8. Edge cases and failure modes
**IDN-110. Stale form after login/logout.** A form rendered before a rotation carries an old CSRF token; POST → `403 csrf`, no state change. The copy says to reload. This is the correct fail-closed behaviour for two tabs and for a shared browser.
**IDN-111. Expired session on a POST.** The CSRF cookie is independent of the session and usually still valid; CSRF passes, then authentication fails → `303 /login` + `session_expired`, body discarded. The user re-authenticates and repeats the action from a fresh page. No half-write occurs because authentication is resolved before the handler.
**IDN-112. Session row deleted between lookup and write.** A handler that already resolved a session may find its `UPDATE … WHERE id=? AND deleted_at=sentinel` affects 0 rows (revoked concurrently). The command must re-check after the write and return `303 /login` + `session_expired` rather than commit a write attributed to a dead session. This matters for change-password and logout.
**IDN-113. Account soft-deleted while a session is live.** The cascade stamps `sessions.deleted_at`, so the next request fails IDN-041. A request already in flight may complete its own read-only render; no write may commit without re-checking `accounts.deleted_at` under its lock. Student edit/delete and reset all do.
**IDN-114. Duplicate registration race.** Two simultaneous student registrations for one email: one transaction inserts, the other blocks on `uq_accounts_email` and then fails `1062` → `422 email_taken`. The losing browser is not signed in and no profile row was created for it. The same applies to teacher registration (one pending row at most).
**IDN-115. Double submit of a non-idempotent action.** Delete: second call `404`. Reset-token issue: second call mints a new token and kills the first (panel shows the newest). Reset consumption: second call `422 token_used`. Approve: second call idempotent success. Change-password: second call fails `current_wrong` because the account lock re-reads the new hash. Login: second call fails CSRF after rotation.
**IDN-116. Token consumed while the page is open.** A teacher issues token A, then issues token B; the student submits A → `422 invalid_token` (revoked, not used). The teacher must hand over B or reissue. If the student submits after the 24-hour boundary, same copy.
**IDN-117. Reset token for a deleted student.** The account/student/token cascade removes the row from the live join; `POST /reset-password` returns `422 invalid_token`, never a `500` and never a partial password update.
**IDN-118. Reset while the student is signed in.** Consumption revokes all sessions including the current browser. The browser's next request is a guest request; the student logs in with the new password. The reset response itself is `303 /login`, so there is no stale authenticated render.
**IDN-119. Change-password while another tab changes it.** The second tab's POST uses the old current password, the row lock forces a re-read, and `422 current_wrong` is returned. The first tab's new password stands. Sessions other than the first tab's were revoked.
**IDN-120. Deleted account's email re-registered.** Allowed (INV-19). A stale reset token for the old student cannot target the new account because `student_id` differs; the old token row was cascaded/deleted, and token lookup joins the live student. A pending token for a deleted teacher (impossible today) would render `expired`.
**IDN-121. Major drift.** A student's major cannot be soft-deleted while the student is live (INV-23), but if data drift produces a missing major, the profile/edit/list render `—` rather than hiding the person. A POST with that major id fails `major_required` on the next edit, forcing a live choice.
**IDN-122. No majors at all.** Student registration renders `no_majors` and rejects POSTs with `major_required`; the teacher majors screen (PAR) is the remedy. Teacher registration and login are unaffected.
**IDN-123. No active teacher (bootstrap not run).** Teacher registration succeeds and stays pending; `GET /auth/pending` shows `waiting` with the static `approval_unavailable` line; no approval route is reachable because no one can log in. The app never auto-promotes (R-03/R-24). If a pending registration already used the bootstrap email before the runner ran, the runner's idempotency-on-email cannot create the active teacher; the runner must fail loudly and the operator must resolve the email first (§12.3).
**IDN-124. Bootstrap teacher tries to approve before changing the password.** The must-change guard redirects to `/change-password`; approval is impossible until the flag clears. This is intentional: the first credential is temporary.
**IDN-125. Teacher forgets their password.** There is no teacher reset route and no reset token for teachers (INV-22). Recovery is out of band: the runner/operator sets a new hash and `must_change_password=1` (or deletes/recreates only if the account has no quizzes; teacher deletion is otherwise forbidden). This is a product gap recorded in §12.3, not something a handler may improvise.
**IDN-126. Database unavailable.** Any command that cannot reach the DB returns the 500 page, never a forged `invalid_credentials`, never a 404, and never a `200` with an empty list. `GET /healthz` is OPS's probe; auth does not silently degrade to "allow".
**IDN-127. Cookie disabled or stripped.** Without `quiz_csrf`, every POST is `403 csrf`; without `quiz_session`, every protected route is a login redirect. The pages render a `<noscript>` note where the design requires it; there is no cookie-less auth fallback.
**IDN-128. Extremely long inputs.** Name >100 code points → `name_long`; email >254 → `email_invalid`; password >72 bytes → `password_long`; token >128 → `invalid_token`. The server rejects before hashing and before SQL, so a multi-megabyte body cannot force a bcrypt or a DB round trip. Body size is capped by the server (OPS) at 1 MiB for auth forms.
**IDN-129. Unicode and homoglyph names.** Names may be non-ASCII (utf8mb4); emails and passwords are ASCII-constrained by policy. Two lookalike names are two accounts; identity is the account id/email, never the name (R-02 analog).
**IDN-130. Back button after logout or delete.** `no-store` prevents a cached authenticated page from being shown; the reload hits a guest request and redirects to login. A deleted student's old tab cannot write because every POST re-resolves the session and the account is gone.
**IDN-131. Session table growth.** Expired rows are lazily stamped on presentation; no sweep is required for correctness because every lookup filters `expires_at`. OPS may add an operational sweep; it must only stamp `deleted_at`, never hard-delete (INV-29).
**IDN-132. Clock skew.** Expiry, token windows, and `Retry-After` use the DB clock; the app never compares a stored UTC timestamp to its own `time.Now()` for an authorization decision. Pending-token age is the one app-clock check (stateless); a skew larger than the 24-hour window is an operational fault, not a bypass of a session.

---
## 9. Concurrency and idempotency
**IDN-140.** Serialization points, in one table:

| Operation | Lock / atomicity | Loser's outcome |
|---|---|---|
| student registration | `uq_accounts_email` insert | `422 email_taken`; rollback both rows |
| teacher registration | `uq_accounts_email` insert | `422 email_taken` |
| login | `INSERT sessions` + `UPDATE last_login_at` in one tx; no account lock | two concurrent logins create two sessions (allowed) |
| logout | `UPDATE … WHERE id=? AND deleted_at=sentinel` | 0 rows → still `303 logged_out` |
| change password | `SELECT accounts … FOR UPDATE` | `422 current_wrong` on the stale password |
| reset consume | `SELECT password_reset_tokens … FOR UPDATE` | `422 token_used` |
| reset issue | `SELECT students … FOR UPDATE`; supersede + insert | second issue supersedes the first; one live token |
| profile edit | `UPDATE accounts` (unique key) | `422 email_taken` on a colliding address |
| student edit | `SELECT student+account FOR UPDATE` | last commit wins; a deleted target → `404` |
| student delete | root `SELECT … FOR UPDATE`, one timestamp for all children | second delete `404`; cascade atomic |
| teacher approve | single conditional `UPDATE … WHERE status='pending'` | 0 rows → idempotent `already_approved` |
| CSRF rotate | response cookie only; no DB | stale form `403` |

**IDN-141.** Multi-statement writes use one transaction and one timestamp taken once (`@ts`). A failure at any child rolls back the root; a committed root always has every child stamped (INV-18). The cascade is a set of `UPDATE … WHERE … deleted_at=sentinel` statements, not a row-by-row walk.
**IDN-142.** Read-modify-write commands (change-password, reset consume, student edit, delete, reset issue) must not trust a SELECT performed outside the transaction. The validating SELECT is the locking SELECT, or the write's predicate re-checks the state (approve, logout).
**IDN-143.** Duplicate-key (`1062`) and deadlock (`1213`) are the only expected engine errors. `1062` maps to a field error; `1213` retries the whole transaction once with the same inputs; any other error → 500 + rollback. No retry may re-run a consumed token or a delete that already committed.
**IDN-144.** Rate-limit counters are incremented under one mutex in the same critical section as the window check, so N concurrent requests cannot all observe "under limit". The key includes the normalized email for login, so casing variants share a bucket.
**IDN-145.** Two-tab semantics are part of the contract: session and CSRF are shared; a rotation in one tab invalidates the other tab's pending form (fail closed); a change-password in one tab revokes the other tab's session; profile/edit last-writer-wins is acceptable because there is no mergeable field.
**IDN-146.** Idempotency summary: `GET` safe; `POST /logout`, `/teacher/teachers/:id/approve` idempotent; `/teacher/students/:id` effectively idempotent (same values); `/login` and `/register/*` guarded by unique keys/rotation; `/reset-password` single-use; `/teacher/students/:id/delete` second call `404`; `/teacher/students/:id/reset-token` intentionally non-idempotent.
**IDN-147.** No command holds a lock across bcrypt for longer than necessary: login does not lock the account (it only reads the hash), while change-password locks and then compares so the comparison is against the row it will update. A login racing a password change either compares the old hash and creates a session that the change then revokes (if the change commits after) or compares the new hash (if after) — both outcomes are consistent.

---
## 10. Cross-domain contracts
### 10.1 What IDN guarantees
**IDN-150.** **Identity.** Every authenticated handler in AUT/PAR/ATT receives a `CurrentUser{ID, Role, FullName, Email, GradeLevel, MajorName, Initials}` or nil, resolved per request. The role is authoritative and typed.
**IDN-151.** **CSRF.** Every non-beacon POST in every domain is verified by the IDN middleware with the same signed double-submit cookie and the same `csrf` copy. A domain that adds a form must render `{{.CSRF}}`; it must not implement its own token.
**IDN-152.** **Session revocation.** Deleting a student, consuming a reset token, or changing a password invalidates sessions immediately; ATT's beacon and PAR's authenticated pages see the next request as unauthenticated. No domain may cache `CurrentUser` beyond one request.
**IDN-153.** **Names are current.** `full_name`/`email` are read live from `accounts`; a profile or teacher edit changes every historical display on the next render. ATT must join the account, not copy the name into an attempt. Quiz titles are the opposite (locked at publish, AUT), and the difference is intentional (R-25).
**IDN-154.** **Student deletion is complete.** After IDN-C19 commits, PAR/ATT can assume no live registration, attempt, answer, selection, or focus event exists for that student. Roster counts, monitor rows, results, recaps, and history must exclude the student; a direct URL to an old attempt is `404`, not an empty page. PAR's cancellation and ATT's `attempt.start` must not "resurrect" a registration for a deleted student.
**IDN-155.** **Eligibility inputs.** `students.grade_level` and `students.major_id` are current and teacher-managed. PAR checks them at registration/attempt start; IDN never deletes a registration when a teacher changes them, so a student who registered while eligible may finish (R-12). A class label shown in ATT results is derived from the current profile, not frozen.
**IDN-156.** **Teacher immutability.** AUT can rely on `quizzes.owner_account_id` always naming a live teacher account: teachers are never deleted or deactivated, and there is no route that changes a teacher's role or email. A quiz never becomes ownerless (R-05, R-14).
**IDN-157.** **Anonymous separation.** IDN's session never applies to an anonymous participant, and PAR's identity cookie never authenticates an account route. A browser may hold both cookies simultaneously; each domain reads only its own.
**IDN-158.** **Pending isolation.** A pending teacher has no session and therefore no access to AUT/PAR/ATT teacher routes. Approval is the only unlock; there is no "pending read-only" mode.

### 10.2 What IDN requires
**IDN-159.** From **PAR**: registration/attempt-start eligibility uses the live student profile; a deleted student is treated as absent; the anonymous cookie uses a distinct name and does not call the session middleware's cookie; quiz intro/registration never writes `accounts`/`students`.
**IDN-160.** From **ATT**: attempt ownership is checked against the current session (or PAR anonymous identity); the beacon performs its own same-origin/ownership check (R-20); results/recaps/monitor join `accounts` for names and never render a deleted student; grading writes `graded_by_account_id` only for an active teacher.
**IDN-161.** From **AUT**: owner checks are `quizzes.owner_account_id = CurrentUser.ID` and non-owner → `404`; the quiz list/dashboard never show another teacher's rows; publish/close/delete do not write accounts. Invite resolution must not authenticate a pending teacher.
**IDN-162.** From **OPS**: the 404/500 pages, the catch-all, security headers, body-size limits, and the request-id logging contract; soft-delete/transaction conventions used by IDN-C19; the `schema_migrations`/bootstrap runner (R-03, R-24). OPS must not add an auth bypass or a second session store.
**IDN-163.** From **PAR/AUT** on majors: a major referenced by a live student cannot be deleted (INV-23); IDN relies on that so a profile read never has to invent a class. Rename is a single-row update and every IDN read joins the current name (R-13).

---
## 11. Traceability
**IDN-170.** Every owned route maps to exactly one command. "AC" numbers refer to §13.

| Route | Command | Invariants | Acceptance |
|---|---|---|---|
| `GET /` | IDN-C01 | IDN-I01, I14 | AC-01, AC-02 |
| `GET /login` | IDN-C02 | I13 | AC-03 |
| `POST /login` | IDN-C03 | INV-04, INV-21; I01, I03, I04, I13 | AC-04–AC-10 |
| `GET /register/teacher` | IDN-C04 | INV-04; I03 | AC-11 |
| `POST /register/teacher` | IDN-C05 | INV-01, INV-04; I02, I03, I15 | AC-11–AC-15 |
| `GET /register/student` | IDN-C06 | INV-02; I02 | AC-16 |
| `POST /register/student` | IDN-C07 | INV-01, INV-02, INV-04; I02 | AC-16–AC-20 |
| `GET /reset-password` | IDN-C08 | I07 | AC-21 |
| `POST /reset-password` | IDN-C09 | INV-22; I04, I07 | AC-21–AC-27 |
| `GET /change-password` | IDN-C10 | I11 | AC-28 |
| `POST /change-password` | IDN-C11 | I04, I08, I11 | AC-28–AC-33 |
| `GET /auth/pending` | IDN-C12 | I03, I15 | AC-34–AC-36 |
| `POST /logout` | IDN-C13 | I08 | AC-37, AC-38 |
| `GET /profile` | IDN-C14 | INV-02, INV-03; I09 | AC-39 |
| `POST /profile` | IDN-C15 | INV-01; I09, I13 | AC-39–AC-42 |
| `GET /teacher/students` | IDN-C16 | I12 | AC-43–AC-46 |
| `GET /teacher/students/:id` | IDN-C17 | I09 | AC-47 |
| `POST /teacher/students/:id` | IDN-C18 | INV-02; I09, I12 | AC-47–AC-49 |
| `POST /teacher/students/:id/delete` | IDN-C19 | INV-18, INV-29; I10 | AC-50–AC-53 |
| `POST /teacher/students/:id/reset-token` | IDN-C20 | INV-22; I06, I07 | AC-54–AC-56 |
| `GET /teacher/teachers` | IDN-C21 | INV-04; I03, I09 | AC-57 |
| `POST /teacher/teachers/:id/approve` | IDN-C22 | INV-03, INV-04; I03 | AC-58–AC-60 |
| every POST | middleware | INV-28; I05 | AC-61–AC-64 |
| every route | middleware | INV-21; I01 | AC-65–AC-70 |
| every list | §6 | I12 | AC-71–AC-72 |
| student deletion | §10 | INV-18; I10 | AC-73–AC-74 |

**IDN-171.** Cross-cutting rules and their acceptance: session sliding/expiry AC-65–67; revocation AC-68; enumeration AC-06/AC-09/AC-13/AC-36/AC-41; rate limiting AC-63/64; stale token AC-25/26/35; cross-role AC-69/70; open redirect AC-02.
**IDN-172.** No route in §2.3 is without a command; no command is without a route. `GET /error/403`, `GET /error/404`, `GET /error/500`, `GET /healthz`, and `/uploads/:name` are OPS/AUT routes and are not counted here.

### 11.5 Objections to binding rulings
Implemented as ruled, with the objection recorded here as contract §1 requires; detail and recommendations are in §12.3.

- **R-04** (reset sets `must_change_password=0`) conflicts with the schema spec's "1 for resets and bootstrap". Implemented as R-04 (0); the schema note's wording is read as "temporary admin-set passwords" (schema §6.2 is descriptive, not a constraint).
- **R-05** (teacher name/email immutable, no teacher profile) means a registration typo is uncorrectable in v1. Implemented; recovery is out of band.
- **R-03 vs R-24** (no bootstrap-existence leak vs "the pending page says approval is not available") are reconciled with a static sentence that is true regardless of whether a bootstrap exists (IDN-106).
- **Old §12** pins login/register at 5 attempts/15 min **per IP**. Implemented as 5/15 per `(email, IP)` plus a 30/15 per-IP ceiling to keep a class behind one NAT usable; recorded as a deliberate refinement, not a silent change.
- **R-22** (no email verification) is implemented; the risk is account-email ownership, accepted for a single school.

---
## 12. Open questions and schema deltas
### 12.1 True schema deltas
**IDN-180.** **None required.** Every rule above is expressible with the current 15 tables and their constraints. The candidates below were considered and rejected as deltas; the app-only resolution is named.

| Candidate delta | Why it is not needed | App-only resolution |
|---|---|---|
| `rate_limits` table | single process, ≤500 users; a table adds a write per attempt | in-memory fixed window (IDN-062) |
| `auth_events`/login-audit table | no screen reads it; logs suffice | structured logs (IDN-074) |
| `pending_teacher_tokens` table | 24h, single-purpose, signed payload is enough | signed cookie/URL token (IDN-088) |
| `email_verification` table/column | R-22 forbids verification | none; email is a login name only |
| `rejected` value in `accounts.status` | no reject action exists (contract §16.4) | pending stays pending; `expired` page state |
| `teachers` table | schema decision 10; role is app-typed | `accounts.role` |
| partial unique "one live reset token per student" | MariaDB has no partial index; a generated column is larger than the rule | student-row lock + supersede (IDN-C20) |
| `password_history`/`password_changed_at` | no screen, no policy | none; `updated_at` only |
| server-side CSRF store / `csrf` column | R-16 rules double-submit | signed cookie (IDN-049) |
| `sessions.last_used_at` / absolute-expiry column | sliding expiry uses `expires_at`; absolute cap is not a v1 rule | `created_at` if a cap is added later |
| flash table/column | closed keys carry no data | `?flash=` keys (IDN-028) |
| reset-reveal table | one-shot 5-minute display | signed cookie + DB liveness recheck (IDN-105) |

**IDN-181.** If a later requirement needs a real audit trail ("who approved whom, when, from where" beyond the two columns), that is a new table and a new §12 entry for OPS, not something a handler may fake in memory.

### 12.2 Resolved without a schema change
**IDN-182.** CSRF: signed double-submit cookie (R-16, R-25).
**IDN-183.** Pending status: signed 24h token in a cookie and URL (R-23).
**IDN-184.** Reset-token reveal: signed 5-minute `HttpOnly` cookie scoped to the students list, revalidated against the live row (IDN-105). This replaces the current demo's raw `?token=` redirect, which leaks the bearer token into history/logs.
**IDN-185.** Rate limits: process-local counters; a restart resets them. Correct for one process; documented as a scaling limit.
**IDN-186.** Flash: closed `?flash=` keys. The set is compile-time; unknown keys render nothing.
**IDN-187.** One live reset token per student: app-enforced under the student row lock, because no index can express it.
**IDN-188.** Reset sets `must_change_password=0` (R-04).
**IDN-189.** A 403 needs a page: add `templates/pages/error/403.html` (error layout, `ErrorPage` model). It is not a route; it uses the existing error layout and OPS's error family. `GET /error/403` is **not** added; the status is returned directly.
**IDN-190.** The reset form's field is `new_password`, not the current template's `password` (contract §18 is rank 2 and the field-name contract is closed). This is a template fix, not a schema change.
**IDN-191.** Role guards need route metadata. `PageRoute` gains an authorization class (or an equivalent table keyed by `method+" "+path`); a route without a class fails startup. This is Go surface, not schema.
**IDN-192.** The 403 page's `HomeHref` is the account's role home when authenticated, `/auth/pending` when a valid pending token exists (pending-login refusal), otherwise `/login`.

### 12.3 Residual ambiguities and product calls
Each has a recommended default; none blocks implementation.

**IDN-193 (RQ-01). Rate limiting behind one school NAT.** Old §12's 5/15 per IP would lock a class sharing one address after five failed logins. **Recommendation:** keep 5/15 per `(email, IP)` failure bucket and 30/15 per IP ceiling (IDN-063). **Impact:** a distributed attacker needs many IPs per account; a class needs one working password. Accepted deviation, recorded in §11.5.
**IDN-194 (RQ-02). Flash transport.** **Recommendation:** keep the closed-key query flash for v1 (cross-domain consistency, no storage). **Impact:** a user can show themselves a canned "saved" message; no state changes. Upgrade to a signed one-shot cookie later without touching templates.
**IDN-195 (RQ-03). Pending page and bootstrap disclosure.** R-03 forbids leaking whether a bootstrap exists; R-24 says the page says approval is unavailable. **Recommendation:** the static sentence `approval_unavailable` on every waiting render, never a conditional. **Impact:** satisfies both; a pending teacher learns the rule, not the current DB state.
**IDN-196 (RQ-04). Teacher password recovery.** No route exists and INV-22 forbids teacher tokens. **Recommendation:** add an out-of-app runner step `reset-teacher-password` (email + new hash, `must_change_password=1`), documented in the ops runbook; do **not** add a teacher reset route in v1. **Impact:** a locked-out teacher needs an operator; acceptable for one school, but must be written down or the account is unrecoverable.
**IDN-197 (RQ-05). Session persistence and absolute cap.** **Recommendation:** browser-session cookie (no `Max-Age`), no absolute cap, 60-minute idle sliding only (IDN-040/046). **Impact:** closing the browser logs out; a stolen token used continuously can live indefinitely. If the school wants a 12-hour cap, enforce `sessions.created_at + 12h` in the same middleware — an app rule, no delta.
**IDN-198 (RQ-06). Registration enumerates emails.** `email_taken` reveals an address is registered. **Recommendation:** keep it (usability for a single school, rate-limited). **Impact:** a determined attacker can test addresses; the mitigation is per-IP/per-email limits, not a vague error that would strand legitimate re-registrations.
**IDN-199 (RQ-07). Pending token in the URL.** The redirect necessarily puts it in the address bar/history; the page is `no-referrer`/`no-store`. **Recommendation:** keep it, with `HttpOnly` cookie as the durable copy. **Impact:** server/proxy logs may contain a 24h status token that reveals only a name and pending/active state; not a session.
**IDN-200 (RQ-08). Reset to the same password.** The reset form does not reject a new password equal to the old one (only change-password does). **Recommendation:** allow it; the token holder already controls the account, and rejecting it leaks a bcrypt comparison oracle and adds a confusing error. **Impact:** a student can "reset" to the same secret; no security gain is lost.
**IDN-201 (RQ-09). Approve is idempotent, not a state conflict.** R-19 says state conflicts redirect with an error flash; double approval is benign. **Recommendation:** `already_approved` as an info flash + `303`, never `409`/`404`. **Impact:** a double-click produces a clean list page; the predicate still prevents a second approver from overwriting `approved_by`.
**IDN-202 (RQ-10). No `rejected` teacher state.** Contract §9.8 lists "rejected/expired", but there is no reject action and the enum has no value. **Recommendation:** map both to the `expired` page; never add a status value in app logic. **Impact:** a pending teacher is either waiting or approved; an operator can leave one pending forever.
**IDN-203 (RQ-11). Bootstrap email collision.** If a teacher self-registers with the bootstrap email first, the runner's idempotent insert does nothing and no active teacher exists. **Recommendation:** the runner must treat an existing email as success only when that row is an active teacher; otherwise it fails loudly and the operator resolves it (delete the pending row out of band or choose another email). **Impact:** a failed bootstrap is visible, not silent.
**IDN-204 (RQ-12). Student deletion while ATT streams hold a snapshot.** A monitor connection opened before the delete may still render the row until its next snapshot/heartbeat refetch. **Recommendation:** ATT's next `participant.updated`/refresh must omit the deleted registration; IDN does not signal SSE. **Impact:** a stale row can survive at most one refresh; no data is readable via a direct URL because attempts 404.
**IDN-205 (RQ-13). Name whitespace.** Collapsing internal runs changes a name like `De  La` to `De La`. **Recommendation:** collapse (IDN-081); display consistency beats preserving accidental double spaces. **Impact:** cosmetic only; search still matches.
**IDN-206 (RQ-14). Multiple live sessions per account.** **Recommendation:** allow (IDN-044); only reset/change/delete revoke. **Impact:** a student on phone + laptop stays signed in on both; no device list to revoke one, which is acceptable at this scale.

---
## 13. Acceptance criteria
Each statement is executable as a server-side test with `app.Test` and a test database (or a fake store), asserting status, `Location`, `Set-Cookie`, rendered copy, and row state.

### Authentication and root
**AC-01.** `GET /` with no cookie → `303 Location:/login`; with a student session → `/student`; with a teacher session → `/teacher`; with a must-change session → `/change-password`. **AC-02.** `GET /?next=https://evil.example` never redirects off-origin; the query is ignored.
**AC-03.** `GET /login` returns `200`, contains `csrf_token`, contains no role field, and sends `Cache-Control: no-store`.
**AC-04.** `POST /login` with a valid CSRF token and correct student credentials → `303 /student`, one new live session, `last_login_at` set. **AC-05.** Correct teacher credentials → `303 /teacher`; a teacher with `must_change_password=1` → `303 /change-password` with `must_change` flash.
**AC-06.** Unknown email and known email + wrong password return the same status (`422`), the same body copy (`invalid_credentials`), and exactly one bcrypt compare each.
**AC-07.** `POST /login` without a CSRF cookie, with a forged cookie, or with a mismatched form token → `403`, no session row, copy `csrf`.
**AC-08.** A pre-set `quiz_session=attacker-chosen` cookie before login is replaced on success; the old value resolves to no session; the new session's token hash matches only the new cookie.
**AC-09.** Pending teacher + correct password → `403`, no session cookie, no `last_login_at` update, copy `pending_teacher`; pending + wrong password → `422 invalid_credentials`.
**AC-10.** The 6th failed login for one `(email, IP)` in 15 minutes → `429` with `Retry-After` between 1 and 900 and copy `rate_limited`; a success before the limit clears that bucket.

### Registration
**AC-11.** `POST /register/teacher` with valid fields → `303 /auth/pending?token=…`, one `role='teacher', status='pending'` row, zero sessions, `approved_at NULL`. **AC-12.** Teacher registration never creates an active teacher even when zero active teachers exist; the pending page shows `waiting`.
**AC-13.** Duplicate teacher email (same case or different case) → `422 email_taken`; a soft-deleted account's email is accepted. **AC-14.** A pending token older than 24 hours, tampered, or for another account → pending page `expired`; a valid one → `waiting`; after approval → `approved`; viewing never writes.
**AC-15.** The pending page never renders email, account id, approver name, timestamps, or a conditional bootstrap message; `waiting` always includes `approval_unavailable`.
**AC-16.** `GET /register/student` lists live majors alphabetically; with zero majors it renders `no_majors` and no POST can succeed. **AC-17.** Valid student registration creates exactly one active account + one profile in one transaction, signs the student in, and redirects `/student`.
**AC-18.** A student registration with `grade_level=13` or a soft-deleted/unknown `major_id` → `422 grade_required`/`major_required`, no rows written.
**AC-19.** Two concurrent student registrations for one email produce exactly one account+profile and one `422 email_taken`; no orphan account and no orphan profile.
**AC-20.** Passwords are never repopulated after `422`; the stored hash starts with a cost-12 bcrypt prefix and is not the plaintext.

### Reset and change password
**AC-21.** `GET /reset-password` never validates or consumes a token and sends `no-referrer`; `POST` consumes exactly once. **AC-22.** A valid token + policy-compliant password → `303 /login`, `used_at` set, new hash verifies, `must_change_password=0`, **all** sessions of that student soft-deleted.
**AC-23.** Reusing the same token → `422 token_used`; an expired token, forged token, or deleted student → `422 invalid_token`; neither writes.
**AC-24.** Weak/mismatched new password → `422` with `password_policy`/`password_mismatch`, token not consumed.
**AC-25.** Two concurrent submissions of one token: exactly one updates the password; the other gets `token_used`; only one `used_at`.
**AC-26.** Issuing a second reset token for the same student revokes the first (first → `invalid_token`, second usable); at most one live unused row exists.
**AC-27.** The reset response never auto-logs-in and never echoes the raw token in HTML after success.
**AC-28.** `GET /change-password` is reachable by student, teacher, and a must-change session; it renders blank password fields. **AC-29.** Wrong `current_password` → `422 current_wrong` and increments the per-account failure bucket; the 6th wrong attempt → `429`.
**AC-30.** New password equal to current → `422 same_password`.
**AC-31.** Successful change clears `must_change_password`, keeps the current session, and soft-deletes every other session of the account.
**AC-32.** A must-change session can reach `GET/POST /change-password` and `POST /logout`; every other route (including `/`, `/teacher`, `/profile`) returns `303 /change-password`.
**AC-33.** After a successful change, the must-change redirect no longer fires and the role home renders.

### Pending page and logout
**AC-34.** `GET /auth/pending` with no token renders the generic `expired` state at `200`, with no account data. **AC-35.** A token for an active teacher renders `approved` with a `/login` link; the token remains valid for viewing (not consumed).
**AC-36.** A token for a student id, a deleted account, or a bad MAC renders the same `expired` page as a 24h-old token (byte-identical).
**AC-37.** `POST /logout` with a valid CSRF token and session → `303 /login`, session row `deleted_at` set, cookie cleared, fresh CSRF cookie.
**AC-38.** Replaying the old session cookie after logout → protected GET is `303 /login`, never a rendered page; GET `/logout` → `404`.

### Profile and teacher student management
**AC-39.** `GET /profile` as a student returns own name/email/read-only class; as a teacher it returns `404`; as a visitor `303 /login`. **AC-40.** `POST /profile` changes name/email without revoking the session and without touching `students`; a case-only email change succeeds as a no-op.
**AC-41.** `POST /profile` with another live account's email → `422 email_taken`; with a soft-deleted account's email → success.
**AC-42.** `POST /profile` cannot change grade or major: submitted values are ignored and the students row is unchanged.
**AC-43.** Students list filters by grade/major/search, sorts by all four orders, paginates at 10, and always tie-breaks `id ASC`; invalid values fall back to defaults with no `422`. **AC-44.** Search escapes `%` and `_`; searching for `%` does not return every student.
**AC-45.** Empty roster and filtered-empty render distinct titles/bodies; filtered-empty offers a reset link.
**AC-46.** A page beyond the last page clamps to the last page; page 0 clamps to 1; totals and rows agree.
**AC-47.** `GET /teacher/students/:id` for a live student renders the edit form; for a missing/deleted id, a teacher's account id, or a non-numeric id → `404`. **AC-48.** `POST /teacher/students/:id` updates account+profile atomically, validates live grade/major and unique email, and returns `422` with repopulated values on failure.
**AC-49.** Two teachers editing the same student serialize on the row lock; the last commit wins; a delete that commits first makes the edit `404`.
**AC-50.** Student delete → `303 /teacher/students`, `deleted_student` flash; the account, profile, sessions, reset tokens, registrations, attempts, answers, selections, and focus events all share one `deleted_at`; only the account has `deleted_by_account_id`. **AC-51.** After student delete, login with the old credentials → `422 invalid_credentials`; an old session cookie → `303 /login`; a direct old attempt URL → `404`.
**AC-52.** A second delete of the same student → `404`; no other student, quiz, or major row is stamped.
**AC-53.** Re-registering the deleted student's email creates a fresh account/profile and does not resurrect any old row.
**AC-54.** Reset-token issue creates one live 24h token, supersedes prior live tokens, and returns `303` with a reveal cookie, not a token in the URL. **AC-55.** The reveal panel renders the token once, clears the cookie, and suppresses a tampered/expired/foreign cookie without an error oracle.
**AC-56.** A student id that is missing/deleted → `404`; a teacher target → `404`; issuing for a teacher is impossible.
**AC-57.** Teacher directory shows pending oldest-first and active name-first; only pending rows have `CanApprove`; no edit/deactivate/delete/reject control or route exists.

### Approval
**AC-58.** Approving a live pending teacher sets `status='active'`, `approved_at`, `approved_by_account_id`, in one statement; the target can then log in.
**AC-59.** Double approval (sequential or concurrent) yields one transition and two successful `303`s (`approved` then `already_approved`); `approved_by` is not overwritten.
**AC-60.** Approving a student id, a missing id, or a soft-deleted id → `404`; a pending teacher cannot call the route (no session).

### Security cross-cuts
**AC-61.** Every owned POST rejects a missing/mismatched/forged CSRF token with `403 csrf`; no owned POST is exempt (only ATT's beacon is). **AC-62.** CSRF cookie and form token are 43+1+43 base64url characters; the cookie is `HttpOnly`, `SameSite=Lax`, `Secure` iff configured; tampering with either half fails.
**AC-63.** Login/registration/reset limits return `429` with `Retry-After` and copy `rate_limited`; limits are per process and reset on restart (documented).
**AC-64.** A spoofed `X-Forwarded-For` does not create a fresh rate-limit bucket unless a trusted proxy is configured.
**AC-65.** A session idle for 61 minutes is invalid and its GET is `303 /login` + `session_expired`; a session used at minute 59 slides to a full 60 minutes. **AC-66.** Session lookup rejects a row whose account is pending, soft-deleted, or missing, even if the token hash matches.
**AC-67.** `Set-Cookie` for the session has no `Max-Age`, `Path=/`, `HttpOnly`, `SameSite=Lax`, and no `Domain`; clearing it uses `Max-Age=0`. **AC-68.** Reset revokes all sessions; change-password revokes all but the current; delete revokes all; logout revokes one.
**AC-69.** A student GET/POST on any `/teacher/*` route → `404`; a teacher GET/POST `/profile` → `404`; a visitor on either → `303 /login`. **AC-70.** A visitor on `/auth/pending` with a valid token gets `200` and only name+state; a pending teacher gets no session from any route.
**AC-71.** Every list route never returns `422` for bad query input; each returns `200` with defaults and echoes the effective values.
**AC-72.** Empty vs filtered-empty is distinguishable in the model for students and for the teacher directory sections.
**AC-73.** Student deletion is atomic: an injected failure at any child leaves every row live and returns `500`; a successful delete leaves no live child (INV-18).
**AC-74.** After a student is deleted, PAR/ATT reads (history, roster, results, recap, monitor, beacon) treat the student as absent; no page renders their name or a zeroed attempt.
**AC-75.** Auth responses carry `Cache-Control: no-store`; pending and reset pages carry `Referrer-Policy: no-referrer`. **AC-76.** No auth page, flash, URL, or log contains a password, a raw session token, a raw reset token (except the one-time panel/pending URL), or a bcrypt hash.
**AC-77.** An invalid `SESSION_SIGNING_KEY` (empty, short, or the published development default) prevents startup; tests inject a valid key and CSRF still verifies. **AC-78.** Route metadata coverage: every registered route has an authorization class; a test fails if one is missing, and `POST /attempt/:id/beacon` is the only route marked CSRF-exempt.
