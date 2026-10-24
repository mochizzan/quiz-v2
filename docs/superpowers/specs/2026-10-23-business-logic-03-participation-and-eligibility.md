# Business Logic — Participation and Eligibility

**Date:** 2026-10-23
**Status:** binding for PAR; implements contract rulings R-01–R-25 without re-litigation
**Scope:** anonymous identity, join/invite/identify, registrations, cancellation, eligibility, student dashboard and history reads, anonymous-participant management, majors management.
**Lens:** participant fairness and access. Every rule asks who is let in, who is kept out, what they are told, and whether two identically named people are still two people.
**Sources:** `2026-10-23-business-logic-00-contract.md` (binding; cited as C-§); `2026-10-23-schema-and-migrations-design.md` (S-§); `0002`–`0005` migrations and `internal/model/{reference,participation,authoring,attempt}.go` (data shape); `2026-10-23-frontend-data-contract-design.md` (F-§); `2026-10-20-quiz-system-design.md` §3, §4, §6 (join only), §14 (old design; §11 forbidden); current routes, view models, templates.
**Rule prefix:** `PAR-###`. Domain invariants: `PAR-INV-##`. Cross-domain references use the owning prefix plus a bracketed rule name where the sibling document's number is not yet fixed.

---

## 1. Header and reading contract

1.1. This document owns exactly the PAR rows of C-§6. It does not own account creation, quiz authoring, attempt creation/scoring, or cascade mechanics; it cites them.
1.2. Authority order is C-§1. Where the frontend contract and the schema disagree, the schema wins and the disagreement is recorded in §12. The two known disagreements are the anonymous retake identity (R-02 wins; cookie, never name) and private codes (R-11 wins; private rows store `NULL`, so no private code can resolve).
1.3. Every storage read filters `deleted_at = '1970-01-01 00:00:00'` (the live sentinel) unless the rule says otherwise. "Live" in this document always means that sentinel. UTC is stored; WIB is displayed; dates never gate access (R-15).
1.4. States are derived, never stored (R-06, INV-25). No rule in this document writes a status, counted, current-question, or last-seen column except `anonymous_participants.last_seen_at`, which the schema provides for exactly that purpose.
1.5. Every POST is CSRF-protected and ends in 303 PRG except the documented 422 re-render and the documented 404/429/500 responses (R-16, R-19). Every GET is side-effect free except the two capability landings the product contract makes idempotent: invitation auto-registration and the identity-switch link (§8.4).
1.6. Indonesian copy in this document is exact. Reused keys from R-19 and F-§5.3 are cited, not re-typed with variants. New PAR copy lives in §5.0 and must be implemented verbatim.
1.7. "Student" means an `accounts.role='student'` row with a live `students` profile and `status='active'` (students are always active; INV-04). "Teacher" means an approved active teacher. "Anonymous participant" means a live `anonymous_participants` row; it is not an account and never receives a session.

## 2. Intent and boundaries

2.1. Intent: turn a code, an invitation URL, a direct quiz URL, or an identity form into at most one live registration for the correct participant; expose that registration honestly at every later visit; and never let a name, a guess, or a stale form grant access it should not have.
2.2. PAR decides: whether a viewer may see a quiz intro; whether a registration may be created or cancelled; which quizzes a student is assigned; how history is grouped, ordered, and scored; which anonymous identities and majors a teacher may manage; and what every rejection says.
2.3. PAR delegates: account/session/role/approval/password/CSRF/rate-limit mechanics to IDN; quiz state, visibility, targets, code and invite generation, publish/close, question count, and title lock to AUT; attempt start/resume, numbering, answer slots, shuffle, submit, grading, `final_score`, review modes, monitor, and SSE to ATT; the sentinel, cascade, transaction discipline, and error taxonomy to OPS.
2.4. PAR does not grade, does not compute `final_score`, does not create attempt rows, does not count focus events except to read them, and does not decide whether a quiz is open. It asks the owning domain and maps the answer to a participant-visible outcome.
2.5. PAR is the only writer of `registrations` and `anonymous_participants`, and the only writer of `majors` except the seed migration. AUT writes `quizzes.code`, `quizzes.invite_token`, and `quizzes.target_*`; PAR reads them and never rotates them.
2.6. Non-goals: anonymous-to-account linking (no schema, no route), merging duplicate anonymous identities, blocking cookie clearing, IP/device fingerprinting, email delivery, notifications, and score aggregation beyond "maximum visible score".

## 3. Actors, permissions, entry conditions

3.1. Actor classes: visitor (no session, no identity cookie), anonymous participant (live identity cookie, no session), student (live student session), teacher (live active teacher session), and system (the request handler in a transaction). A viewer may accidentally hold both a session and an identity cookie; the session wins and login clears the identity (§10.3).
3.2. Unauthenticated access. `GET /join`, `GET /invite/:token`, `GET /join/identify` (with a valid ticket), and `POST /join` / `POST /join/identify` are reachable without a session. Every other PAR route requires a session; IDN's middleware owns the redirect to `/login` and the `must_change_password` gate. PAR never renders a login redirect itself.
3.3. Student-only routes: `GET /quiz/:id` (student viewer), `POST /quiz/:id/register`, `POST /quiz/:id/cancel`, `POST /quiz/:id/attempts`, `GET /student`, `GET /student/history`, `GET /student/history/:quizID`. A teacher or anonymous participant reaching these gets 404, except `/quiz/:id` and the two POSTs, which have explicit anonymous/teacher branches in §5.
3.4. Teacher-only routes: `GET /teacher/anonymous`, `GET/POST /teacher/anonymous/:id`, `POST /teacher/anonymous/:id/delete`, `GET /teacher/majors`, `POST /teacher/majors`, `POST /teacher/majors/:id`, `POST /teacher/majors/:id/delete`. Access is school-wide: any active teacher, not only the quiz owner (R-05). No ownership check exists on these routes.
3.5. 404-vs-403 policy. Missing, soft-deleted, or out-of-scope resources return the 404 page with status 404. PAR never returns 403 except CSRF failure (R-19). A private quiz the viewer cannot see, another student's registration, a non-owner teacher's quiz, and an unknown route id are all indistinguishable 404s. 401 is never used; unauthenticated users are redirected by IDN or receive 404 where no login would help.
3.6. Public vs private quiz access. Public open quizzes are visible and joinable by every student and, through code/invite, by anonymous participants. Private open quizzes are visible and joinable only by students whose live `grade_level` equals `target_grade` and whose live `major_id` equals `target_major_id`; anonymous participants are excluded by R-12. Draft quizzes are visible only to the owner teacher in the authoring screens; students and visitors get 404. Closed quizzes are visible only to participants with a live registration (intro, history, result); everyone else gets 404.
3.7. Codes are not a gate for logged-in students on public quizzes: every open public quiz is discoverable on the student dashboard, so a code is a shortcut, not an authorization. Codes and invitations are the only gates for anonymous participants. Private quizzes have no code at all.
3.8. Entry preconditions shared by every command: the actor's session/identity is live; the target quiz is live; the target participant row is live; the request carries a valid CSRF token (POST only); the route parameters parse as positive integers; and the request is not rate limited. A failing precondition maps to the error family in §3.5 or §5.0, never to a panic.

## 4. States and transitions

### 4.1 Participation state (derived, per registration)

| State | Derivation over live attempts of the registration | UI label |
|---|---|---|
| `registered` | count = 0 | Terdaftar |
| `in_progress` | count ≥ 1 and at least one `submitted_at IS NULL` | Sedang berlangsung |
| `completed` | count ≥ 1 and none `submitted_at IS NULL` | Selesai |

4.1.1. `completed` includes ungraded essays (R-06). `graded` is not a participation state.
4.1.2. In-progress attempts outrank completed ones: a registration with one submitted and one open attempt is `in_progress`.
4.1.3. A registration with all attempts soft-deleted cannot occur through any PAR route; if it does (corruption), it reads as `registered` and is hidden from history because the registration itself must be soft-deleted in the same cascade (OPS).
4.1.4. The state is recomputed on every read from the attempt rows; no transition is written.

### 4.2 Registration lifecycle

```
(no row) --register/invite/identify--> registered --start attempt--> in_progress
     ^                                      |                          |
     |                                      | cancel (0 attempts)      | submit last open attempt
     |                                      v                          v
     +----------- (soft-deleted) <------ cancelled                  completed --start--> in_progress
                                                                        |
                                                                        +--cancel? never
```

4.2.1. `registered → cancelled` is allowed only while the quiz is open and the registration has zero live attempts (R-10, R-12, F-§23 #9). Cancellation is a soft delete of the registration.
4.2.2. `in_progress → completed` happens when ATT submits the last open attempt; PAR never writes it. `completed → in_progress` happens when ATT starts a retake; PAR authorizes that start.
4.2.3. Cancellation after any attempt (open or submitted) is never allowed. The only way a registration with attempts disappears is an OPS cascade from deleting the student account or the anonymous identity (R-14).
4.2.4. A cancelled registration frees the unique key because `deleted_at` is part of it. The same participant may register again while the quiz is open; the new row starts with attempt 1.
4.2.5. A registration on a closed quiz is terminal: no cancel, no start, no new attempt. If it had zero attempts it is a dead row, retained in history for honesty (§8.3).
4.2.6. There is no reopen of a quiz and no restore of a registration (R-10, R-14, INV-29).

### 4.3 Quiz access state machine (viewer × entry × quiz)

4.3.1. Code entry (`POST /join`) resolves only live quizzes with a non-null code, then checks state. Private quizzes have `code IS NULL` (INV-07), so a private code is indistinguishable from an unknown code: both yield `unknown_code`. This is a deliberate schema-over-contract conflict, recorded in §12 OQ-06.
4.3.2. Invitation entry resolves the token first, so `not_open`, `wrong_audience`, and `already_registered` are provable outcomes for a quiz the viewer was invited to. An unknown or malformed token yields `unknown_code`. Resolution order for a student or visitor is: live registration → state → audience → auto-register/identify. A live registration always wins and lands on the intro with `already_registered`, even after close (R-12).
4.3.3. Direct quiz URL (`GET /quiz/:id`) is an access check by id, not by capability: students see public open quizzes and private quizzes they are eligible for; registered participants see their quiz in any state except draft; everyone else gets 404. A visitor with no identity or no registration gets 404, except when a valid join ticket for that quiz redirects to `/join/identify`.
4.3.4. Identify entry (`POST /join/identify`) requires a valid join ticket. Without it the route is 404, so a guessed `?quiz=` cannot bypass the code/invite gate. The ticket is the proof of provenance; the query parameter is cosmetic.
4.3.5. The full matrix is §4.4. The matrix is normative; command text in §5 explains effects.

### 4.4 Join matrix

| Entry | Viewer | Quiz condition | Outcome |
|---|---|---|---|
| code | visitor / anonymous | public, open | 303 `/join/identify?quiz=:id` with ticket |
| code | visitor / anonymous | public, open, live registration exists | 303 intro, info `already_registered` |
| code | student | public, open, no registration | 303 intro, `CanRegister` true |
| code | student | public, open, live registration exists | 303 intro, info `already_registered` |
| code | any | public draft or closed | 303 `/join?error=not_open` |
| code | any | unknown, malformed, or private (no code) | 303 `/join?error=unknown_code` |
| code | teacher | any | 303 `/teacher` (no resolution) |
| invite | student | any state, live registration exists | 303 intro, info `already_registered` |
| invite | student | public open, no registration | auto-register, 303 intro, success `registered` |
| invite | student | private open, eligible, no registration | auto-register, 303 intro, success `registered` |
| invite | student | private open, ineligible | 200 invite page, `wrong_audience` |
| invite | student | draft or closed, no registration | 200 invite page, `not_open` |
| invite | visitor | public open, identity + registration exists | 303 intro, info `already_registered` |
| invite | visitor | public open, otherwise | 303 identify with ticket |
| invite | visitor | private open | 200 invite page, `anonymous_not_allowed` |
| invite | visitor | draft or closed | 200 invite page, `not_open` |
| invite | teacher | owns quiz | 303 `/teacher/quizzes/:id` |
| invite | teacher | not owner, or unknown token | 404 |
| direct `/quiz/:id` | student | draft or soft-deleted | 404 |
| direct | student | open public | 200 intro |
| direct | student | open private, eligible | 200 intro |
| direct | student | open private, ineligible, no registration | 404 |
| direct | student | open private, ineligible, registration exists | 200 intro + `wrong_audience` warning |
| direct | student | closed, registration exists | 200 intro |
| direct | student | closed, no registration | 404 |
| direct | anon identity | live registration exists (any state) | 200 intro |
| direct | anon identity | no registration, valid ticket for quiz | 303 identify |
| direct | anon identity | no registration, no ticket | 404 |
| direct | visitor | no identity, valid ticket | 303 identify |
| direct | visitor | no identity, no ticket | 404 |
| direct | teacher | owns quiz | 303 `/teacher/quizzes/:id` |
| direct | teacher | not owner | 404 |
| identify POST | visitor | valid ticket, public open | ensure identity + registration, 303 intro |
| identify POST | visitor | valid ticket, closed, registration exists | refresh profile, 303 intro |
| identify POST | visitor | valid ticket, closed, no registration | 303 `/join?error=not_open` |
| identify POST | visitor | missing/expired/forged ticket | 404 |
| identify POST | student | any | 303 intro (never creates an anonymous row) |
| identify POST | teacher | any | 404 |

4.4.1. `POST /quiz/:id/register` is the logged-in student's explicit registration command; it never applies to anonymous participants (their registration is created by identify). A visitor POSTing it gets 303 `/login` via IDN; an anonymous identity POSTing it gets 404.
4.4.2. `POST /quiz/:id/attempts` is reachable only with a live registration; it is the only start path and always hands off to ATT (§10.6).
4.4.3. The word "intro" means `GET /quiz/:id` rendered for that participant. The word "ticket" means the signed join-ticket cookie in §5.2.

### 4.5 Terminal and dead states

4.5.1. Quiz `closed` is terminal (R-10, INV-09). PAR never reopens.
4.5.2. A registration with zero attempts on a closed quiz is a dead registration. It is visible in history and intro (read-only) but cannot be cancelled or started. This is a direct consequence of R-10; the fairness objection is in §12 OQ-05.
4.5.3. A soft-deleted anonymous identity or student account leaves no live registration; any cookie pointing at it resolves to "no identity". Rejoining creates a new identity, not a restore.
4.5.4. A soft-deleted major cannot be selected or referenced; an in-flight form that selects it re-renders with a field error (§5.7.3, §5.12.3).

## 5. Commands

### 5.0 Copy and status registry (normative)

5.0.1. Join rejection copy is exactly F-§5.3 and R-19; it is never paraphrased: `unknown_code` = "Kode kuis tidak ditemukan."; `not_open` = "Kuis ini belum dibuka atau sudah ditutup."; `wrong_audience` = "Kuis ini hanya untuk kelas dan jurusan tertentu."; `already_registered` = "Anda sudah terdaftar di kuis ini."; `anonymous_not_allowed` = "Kuis privat hanya dapat diikuti oleh siswa yang masuk."
5.0.2. PAR flash keys (added to the closed `FlashFor` set; kind, title, optional body):

| Key | Kind | Title | Body |
|---|---|---|---|
| `registered` | success | Anda terdaftar di kuis ini. | — |
| `cancelled` | info | Pendaftaran dibatalkan. | — |
| `not-open` | error | Kuis ini belum dibuka atau sudah ditutup. | — |
| `not-registered` | error | Anda belum terdaftar di kuis ini. | — |
| `cancel-blocked` | error | Pendaftaran tidak dapat dibatalkan karena percobaan sudah dimulai. | — |
| `attempts-exhausted` | error | Batas percobaan untuk kuis ini sudah tercapai. | — |
| `quiz-empty` | error | Kuis ini belum memiliki soal. | Hubungi guru Anda. |
| `identity-switched` | info | Identitas peserta di perangkat ini dihapus. | Isi identitas baru. |
| `major-in-use` | error | Jurusan tidak dapat dihapus karena masih dipakai. | — |

5.0.3. PAR field errors (422 re-render, exact Indonesian): `full_name` required "Nama lengkap wajib diisi.", too short "Nama lengkap minimal 2 karakter.", too long "Nama lengkap maksimal 100 karakter.", control characters "Nama lengkap mengandung karakter yang tidak didukung."; `grade_level` "Pilih kelas yang valid."; `major_id` "Pilih jurusan yang valid."; `name` (major) required "Nama jurusan wajib diisi.", too short "Nama jurusan minimal 2 karakter.", too long "Nama jurusan maksimal 50 karakter.", duplicate "Nama jurusan sudah dipakai."
5.0.4. Status registry: GET pages 200; successful POST 303; POST validation failure 422 with the submitted non-secret values and `FieldErrors`; state conflict 303 with the flash above; missing, soft-deleted, wrong-role, non-owner, or out-of-audience resource 404; CSRF 403 with R-19 copy; rate limit 429 with `Retry-After`; unexpected database failure 500. Invite outcomes are 200 even when negative because the landing page is the explanation (F-§9.6). 409 is never used.
5.0.5. Query parameters never 422. Unknown `error`/`flash`/`sort`/`q` values are ignored; a bad `page` falls back to 1. Only form bodies produce 422.

### 5.1 Anonymous identity cookie (PAR-001–PAR-012)

5.1.1. **PAR-001 — Cookie name and value.** Name `quiz_participant`; value `v1.` followed by the unpadded base64url encoding of 32 crypto-random bytes (46 ASCII characters total). `v1.` is part of the value and part of the hash input. The cookie is the identity; the display name never is (R-02, INV-27).
5.1.2. **PAR-002 — Attributes.** `HttpOnly`, `SameSite=Lax`, `Secure` exactly when `COOKIE_SECURE` is true, `Path=/`, `Max-Age=31536000` (365 days), no `Domain`. The long lifetime is what makes retakes possible without an account; it is a product call flagged in OQ-02.
5.1.3. **PAR-003 — No signature.** The token is an opaque 256-bit bearer secret, not a signed payload. Integrity is proven by exact SHA-256 lookup: a modified or invented value hashes to a row that does not exist. A signature would add a second source of truth and a key-rotation failure mode without adding entropy. "Signing" therefore means "format prefix + exact hash"; this is deliberate, and an HMAC envelope is offered as OQ-03 but not required.
5.1.4. **PAR-004 — Hash storage.** Store `SHA-256(UTF-8(cookie value))` as 32 raw bytes in `anonymous_participants.cookie_token_hash BINARY(32)`, unique together with `deleted_at` (INV-19). Never store the token, never log it, never put it in a URL or HTML. The hash is not a password hash: the input already has 256 bits of entropy, so a fast hash is correct.
5.1.5. **PAR-005 — Lookup.** On every request that may act anonymously, read the cookie, reject it unless it matches `^v1\.[A-Za-z0-9_-]{43}$`, hash the exact string, and select the live row by the unique key. Hit = identity. Miss = no identity. There is no partial, case-insensitive, or name-based fallback (R-02).
5.1.6. **PAR-006 — Missing or corrupt handling.** Missing, malformed, non-base64url, wrong length, unknown hash, or soft-deleted row all mean "visitor". The request continues as a visitor; no error page is shown for a bad cookie. On a GET, clear the bad cookie (`Max-Age=0`) so it is not resent. On `POST /join/identify`, a bad cookie is ignored and a new identity is created after the transaction commits.
5.1.7. **PAR-007 — Issue.** The identity row and cookie are created only by a successful `POST /join/identify` inside the transaction that also creates the registration. The cookie is written only after commit. The token is derived deterministically from the join ticket's nonce (PAR-019) so a double-submitted form produces one identity, not two.
5.1.8. **PAR-008 — Expiry and last seen.** The server has no expiry column. `last_seen_at` is set to now on identity creation and refreshed on every successful identify, registration reuse, and attempt start by that identity. It is not refreshed by read-only GETs. Teacher list "last activity" is `max(last_seen_at, registration.created_at, attempt.updated_at, attempt.submitted_at)` so a reader still sees recent activity without a write per page view.
5.1.9. **PAR-009 — Rotation.** A live token is never rotated: rotation would break other tabs and make the same person two rows for no security gain. The cookie's `Max-Age` slides on each successful identify/registration/start. A new token is minted only when no live identity exists (new visitor, corrupt cookie, deleted identity, or explicit identity switch). The old row is never rewritten; it is deleted or left alone.
5.1.10. **PAR-010 — Cancellation semantics.** Cancelling an anonymous registration soft-deletes only that registration. The identity row, its cookie, and its other registrations stay (R-02). The contract F-§10.4 sentence "also clears the identity cookie binding for that quiz" is superseded: the schema has no per-quiz cookie binding to clear, and deleting the identity would destroy other quizzes' history.
5.1.11. **PAR-011 — Teacher deletion.** When a teacher deletes an identity, the browser cookie is not actively cleared (no channel to that browser); the next request finds no live row and is treated as a visitor (PAR-006). Re-identifying creates a new identity with a new token.
5.1.12. **PAR-012 — Cap-bypass honesty.** Clearing cookies, using a private window, another browser, or another device creates a new identity and a fresh retake allowance. This is accepted by R-02 and cannot be prevented without violating "the cookie, not the name". Teachers who need graded fairness must use private quizzes with student accounts. Same-name participants are distinct rows; a teacher may see two identical names and must disambiguate by class and activity, not by merging. There is no merge route.

### 5.2 Join ticket (PAR-013–PAR-020)

5.2.1. **PAR-013 — Purpose.** A short-lived, signed proof that this browser resolved a specific code or invitation for a specific quiz and may proceed to `/join/identify`. It stops a guessed `?quiz=` from bypassing the gate and carries the nonce that seeds the identity token.
5.2.2. **PAR-014 — Name and attributes.** `quiz_join_ticket`, `HttpOnly`, `SameSite=Lax`, `Secure` per `COOKIE_SECURE`, `Path=/join`, `Max-Age=1800` (30 minutes), no `Domain`.
5.2.3. **PAR-015 — Value.** `v1.` + unpadded base64url(payload) + `.` + unpadded base64url(HMAC-SHA256(`SESSION_SIGNING_KEY`, "join-ticket|" + payload)). Payload is the ASCII string `v1|q=<quiz_id>|m=<code|invite>|s=<secret>|iat=<unix>|n=<32 hex chars>`, where `secret` is the exact six-digit code or the exact invite token presented. The nonce is 128 crypto-random bits. The signature is compared in constant time.
5.2.4. **PAR-016 — Verification.** Accept only if: the shape and signature verify; `iat` is not more than 30 minutes old and not more than 60 seconds in the future; `q` is a positive integer equal to `?quiz=`; the secret still resolves to that quiz; and the quiz is still public. A failure is 404 for the identify page. A closed quiz is a state outcome, not a forged ticket.
5.2.5. **PAR-017 — One-shot.** The ticket is cleared after a successful identify. Replaying the same ticket after success is harmless: the derived identity key is the same, so it reuses the existing identity and registration instead of duplicating. A stolen ticket is a bearer capability for at most 30 minutes and one public quiz; this is accepted and recorded in §12.
5.2.6. **PAR-018 — Overwrite.** A new successful code or invite resolution overwrites the ticket. There is only one pending identify at a time; a second quiz must be entered after the first is finished or abandoned.
5.2.7. **PAR-019 — Identity derivation.** The identity token for a first-time identify is `base64url(HMAC-SHA256(SESSION_SIGNING_KEY, "participant|" + nonce))`, truncated to 32 bytes, with the `v1.` prefix added. Because it is deterministic from the ticket, two concurrent submissions of the same form compute the same hash and collide on `uq_anon_cookie`; the loser reuses the winner's row. The signing key never leaves the server.
5.2.8. **PAR-020 — Clearing.** Login, logout, and identity switch clear the ticket. Expiry is enforced server-side from `iat`, so a client clock change cannot extend it.

### 5.3 `GET /join` — code entry screen

5.3.1. **PAR-021 — Route/actor.** `GET /join`. Visitor, anonymous identity, or student; a teacher is redirected 303 to `/teacher`.
5.3.2. **PAR-022 — Authorization/entry.** No session required. The page is available whether or not a code is currently valid. It never resolves a quiz by itself.
5.3.3. **PAR-023 — Input.** Query `error` is an optional JoinError key; query `flash` is the generic flash key. Both are untrusted and ignored when unknown. The form has one visible field, `code`, and a CSRF token.
5.3.4. **PAR-024 — Effects.** Render the join page. If `error` maps to `unknown_code` or `not_open`, render the exact copy in the page's error block. If the viewer has a live anonymous identity, render a one-line identity note ("Anda akan bergabung sebagai <nama>."); a switch link appears only when a valid join ticket already exists (PAR-052), because switching requires a ticket. Never prefill the code. Student viewers see the same page (a student may enter a code for a quiz not on their dashboard).
5.3.5. **PAR-025 — Postconditions/status.** 200 HTML, no writes, no cookie changes except issuing the guest CSRF cookie (IDN). Unknown query values change nothing. Empty code is not prefilled.
5.3.6. **PAR-026 — Idempotency.** Safe GET, arbitrarily repeatable. It must not create a registration, ticket, or identity.

### 5.4 `POST /join` — resolve a code

5.4.1. **PAR-027 — Route/actor.** `POST /join`. Visitor, anonymous identity, or student; teacher gets 303 `/teacher` without resolving. CSRF required.
5.4.2. **PAR-028 — Preconditions.** A form body with `code`; a live CSRF token; not rate limited (§8.7). The code is trimmed of leading/trailing ASCII whitespace; internal whitespace is not removed.
5.4.3. **PAR-029 — Validation.** The trimmed code must be exactly six ASCII digits. Anything else (empty, letters, five or seven digits, Unicode digits) is a non-resolution and follows PAR-031; it is not a 422, because the join screen's closed copy set has exactly one "not found" message and a malformed code is not found. This deliberately keeps F-§5.3 as the only copy.
5.4.4. **PAR-030 — Resolution.** Select the live quiz whose `code` equals the string. No match → `unknown_code`. Match with `state != open` → `not_open` (draft and closed codes are inert, R-11). Match with `state = open` and `visibility = public` → resolved. A row with `visibility = private` and a code cannot exist (INV-07); if corruption produces one, treat it as `unknown_code` and log ERROR.
5.4.5. **PAR-031 — Failure responses.** `unknown_code` or `not_open`: 303 to `/join?error=<key>`; the GET renders the exact copy. The submitted code is not echoed in the URL. `wrong_audience` and `anonymous_not_allowed` are impossible on this route because private quizzes have no code; they are never fabricated. Rate limit: 429 with `Retry-After` and R-19 copy.
5.4.6. **PAR-032 — Logged-in student success.** No registration is created by this route (R-12: codes do not auto-register students). If a live registration exists → 303 intro with info flash `already_registered`. Otherwise → 303 intro with no flash; the intro offers **Daftar**. The code is not stored in the session or URL after this point.
5.4.7. **PAR-033 — Anonymous/visitor success.** If the viewer is an anonymous identity with a live registration for the quiz → 303 intro with info flash `already_registered`. Otherwise: mint/overwrite the join ticket (method `code`, secret = the code), 303 to `/join/identify?quiz=<id>`. No identity is created yet, so a visitor who abandons the form leaves no row.
5.4.8. **PAR-034 — Effects/transaction.** None beyond the ticket cookie; the resolution is a read. If the quiz closes between resolution and the identify POST, the identify command re-checks and refuses (PAR-057). Postcondition: the browser holds a ticket bound to that quiz and secret.
5.4.9. **PAR-035 — Provenance/idempotency.** No audit row. Repeating the same code yields the same outcome and refreshes `iat`/nonce only when a new ticket is minted; an existing valid ticket for the same quiz may be reused to avoid invalidating a half-filled form. A retry after success is a no-op on the same participant.

### 5.5 `GET /invite/:token` — invitation landing

5.5.1. **PAR-036 — Route/actor.** `GET /invite/:token`. Student, visitor, anonymous identity; teachers are handled by PAR-041. Safe GET with the contract-mandated auto-registration side effect for students (F-§4.2 #13, R-12).
5.5.2. **PAR-037 — Token validation.** Trim nothing (tokens are exact, `ascii_bin`); accept 1–32 characters of `[A-Za-z0-9_-]`; anything else is `unknown_code`. Select the live quiz by `invite_token`. No match → 200 invite page with `unknown_code`, no quiz title. Never 404 for an unknown token: the landing page is the explanation (F-§9.6).
5.5.3. **PAR-038 — Resolution order.** (a) If the viewer has a live registration for the resolved quiz, 303 intro with info `already_registered`, regardless of quiz state (R-12). (b) Otherwise check state: draft or closed → 200 invite page with `not_open`, showing the quiz title and audience because the token is the capability. (c) Otherwise check audience. (d) Otherwise auto-register or hand to identify.
5.5.4. **PAR-039 — Student, public open, no registration.** Run the registration transaction (PAR-064–PAR-068) with source `invite`. Success → 303 intro with success flash `registered`. If the transaction loses a uniqueness race, treat it as success and redirect with info `already_registered` instead; never show two flashes.
5.5.5. **PAR-040 — Student, private open.** Eligible (live grade and major equal both targets) → auto-register as PAR-039. Ineligible → 200 invite page with `wrong_audience`, title and audience shown, no auto-registration. A closed private quiz with no registration → `not_open` (state is checked before audience, F-§9.6).
5.5.6. **PAR-041 — Teacher viewer.** If the teacher owns the resolved quiz → 303 `/teacher/quizzes/:id`; the invite is not a student entry and must never create a registration. If the teacher does not own it, or the token is unknown → 404. This enforces R-05's "never reveals a quiz to a non-owner teacher" without a second 403 path.
5.5.7. **PAR-042 — Visitor/anonymous viewer.** Public open → if the identity has a live registration, PAR-038(a); otherwise mint an `invite` ticket (secret = token) and 303 `/join/identify?quiz=<id>`. Private open → 200 invite page with `anonymous_not_allowed`, title and audience shown, login link. Draft/closed → `not_open`. Never create an identity on a GET.
5.5.8. **PAR-043 — Effects/transaction/status.** The only write is the student registration transaction. The page is 200; success is 303. A repeated GET after success hits PAR-038(a) and is idempotent. A link prefetcher may trigger auto-registration: accepted risk, bounded to one idempotent registration and no attempt start (OQ-07). Provenance: `registrations.created_at`; no actor column exists. No audit row.
5.5.9. **PAR-044 — Failure.** A database failure inside auto-registration → 500 page, no partial registration (transaction rollback). A quiz soft-deleted between token selection and insert → `not_open`? No: a soft-deleted quiz has no live row, so resolution re-checks and yields `unknown_code`; the delete transaction and the registration lock serialize (§9.3).

### 5.6 `GET /join/identify` — anonymous identity form

5.6.1. **PAR-045 — Route/actor.** `GET /join/identify`. Visitor or anonymous identity only. Student → 303 to the ticket's intro if the ticket is valid, otherwise 303 `/join`; teacher → 404; no valid ticket → 404.
5.6.2. **PAR-046 — Preconditions.** A valid join ticket (PAR-016); the ticket's quiz is public and open; `?quiz=` equals the ticket quiz (if absent or different, 303 to the canonical `/join/identify?quiz=<id>`). The CSRF token is issued by IDN and rendered in the form.
5.6.3. **PAR-047 — Input.** Fields `full_name`, `grade_level`, `major_id`; hidden CSRF. Majors are the live majors, alphabetical. If a live identity cookie exists, prefill its current `display_name`, `grade_level`, `major_id` and show the identity note + switch link (PAR-052).
5.6.4. **PAR-048 — Effects.** Render the identify page with the quiz title, the audience note (public quiz; no target), and the prefilled values. No identity or registration is created by the GET.
5.6.5. **PAR-049 — Postconditions/status.** 200. The ticket stays valid for its 30-minute window. The identity cookie, if any, is untouched.
5.6.6. **PAR-050 — Failure.** Expired/forged ticket → 404. Closed quiz (ticket minted earlier) → if the identity has a live registration, 303 intro; otherwise 303 `/join?error=not_open`. Private quiz (should be impossible; tickets are minted only for public) → 404 and log ERROR.
5.6.7. **PAR-051 — Idempotency.** Safe GET; repeated loads do not refresh `last_seen_at` and do not mint tokens.
5.6.8. **PAR-052 — Identity switch.** `GET /join/identify?quiz=<id>&switch=1` with a valid ticket and a live identity: expire the `quiz_participant` cookie (`Max-Age=0`), do not touch the row, and 303 to `/join/identify?quiz=<id>` with info flash `identity-switched`. The old identity remains visible to teachers with its attempts; the browser will create a new identity on submit. This is the shared-computer escape hatch; without it, a second student on the same browser would silently inherit the first student's identity and attempts. It is a GET that changes only the caller's own cookie (no row write, idempotent), so it does not need CSRF; it is recorded as an interface extension in OQ-01. A student or teacher reaching it is handled by PAR-045/role 404.

### 5.7 `POST /join/identify` — create/reuse identity and register

5.7.1. **PAR-053 — Route/actor.** `POST /join/identify`. Visitor or anonymous identity; student → 303 intro (never creates an anonymous identity); teacher → 404. CSRF required.
5.7.2. **PAR-054 — Preconditions.** Valid ticket (PAR-016) and matching `?quiz=` (PAR-046); quiz live, public, and open **unless** a live registration already exists for this identity, in which case the command may refresh the profile and land on the intro even if the quiz has since closed; rate limit not exceeded.
5.7.3. **PAR-055 — Validation.** `full_name`: trim, collapse internal runs of whitespace to one space, reject control characters; rune length 2–100; store as submitted otherwise. `grade_level`: exactly `10`, `11`, or `12`. `major_id`: a positive integer naming a live major; a soft-deleted or unknown major is a field error, not a 404. All three are required. On failure: 422, re-render the identify page with the submitted values (name, grade, major), `FieldErrors`, and the same valid ticket; no identity/registration write and no cookie change. A blank name is `Nama lengkap wajib diisi.`, never a generic message.
5.7.4. **PAR-056 — Transaction and locks.** One transaction: (1) if a live identity cookie exists, lock that identity row exclusively; (2) shared-lock the selected live major row; (3) shared-lock the quiz row and re-read state, visibility, and live question count; (4) if no identity exists, insert the identity with the hash of the token derived from the ticket nonce (PAR-019), `last_seen_at = now`; otherwise update `display_name`, `grade_level`, `major_id`, `last_seen_at = now`; (5) if no live registration exists for (quiz, identity), insert it; if the quiz is closed and no registration exists, roll back with `not_open`; (6) commit. The identity insert and the registration insert are in the same transaction, so a failure leaves no orphan identity and no orphan registration.
5.7.5. **PAR-057 — State outcomes.** Quiz closed and no live registration → 303 `/join?error=not_open`, clear the ticket. Quiz closed and live registration → update the profile, commit, 303 intro with no flash (the participant may resume an in-progress attempt; if none, the intro is read-only). Quiz soft-deleted → 404. Zero live questions → do not create a registration; 303 `/join/identify?quiz=<id>` with flash `quiz-empty` and keep the ticket so the teacher can fix the data (registration is defense in depth; AUT must prevent opening an empty quiz, §10.5).
5.7.6. **PAR-058 — Effects.** Identity created or profile refreshed; registration created if absent. `last_seen_at` refreshed. Cookie `quiz_participant` set/refreshed after commit with the new or existing token and a fresh 365-day `Max-Age`. Ticket cleared after commit. No attempt is created (attempt start is a separate command).
5.7.7. **PAR-059 — Success responses.** New registration → 303 intro with success flash `registered`. Existing live registration (double submit, returning retake entry, or click on an old form) → 303 intro with info flash `already_registered`. A duplicate-key collision on `uq_anon_cookie` or `uq_reg_anon` is caught, the existing live row is selected, and the command continues as the "existing" case; it is never a 500.
5.7.8. **PAR-060 — Provenance/idempotency.** `anonymous_participants.created_at/updated_at` and `registrations.created_at`; no actor column and no audit row. `last_seen_at` is the only mutable evidence. Repeating the same POST with the same ticket is idempotent by derivation + unique key: one identity, one live registration, same intro. A retry after the ticket was cleared but with the identity cookie present reuses the identity (and registers if needed). Concurrent submissions are serialized by the identity row lock; the loser waits and then observes the registration.

### 5.8 `POST /quiz/:id/register` — explicit student registration

5.8.1. **PAR-061 — Route/actor.** `POST /quiz/:id/register`. Authenticated student only. A visitor is redirected to `/login` by IDN; an anonymous identity or teacher gets 404. CSRF required.
5.8.2. **PAR-062 — Authorization/preconditions.** Quiz live; `state = open`; student live and active; the student is eligible (public always, private exact grade + major, §4.3); no live registration exists. A draft or closed quiz without a registration is 404 (no join after close, R-10). A private ineligible student is 404, never `wrong_audience`, so a forged id cannot probe the audience of a private quiz; the invitation landing is the only place `wrong_audience` is shown (OQ-04).
5.8.3. **PAR-063 — Validation.** No body fields; the quiz id is the path. A non-integer or non-positive id is 404. An unknown id, soft-deleted quiz, or wrong role is 404. There is nothing to 422.
5.8.4. **PAR-064 — Transaction and locks.** One transaction: shared-lock the student profile row (so a concurrent IDN grade/major edit cannot change eligibility under us); shared-lock the quiz row; re-check state, visibility, targets, and live question count; select the live registration for (quiz, student); if present, commit and return the idempotent outcome; insert the registration; commit. Lock order participant → quiz is fixed (§9.1).
5.8.5. **PAR-065 — Effects.** A `registrations` row with exactly `quiz_id` and `student_id` (anonymous column NULL, INV-05). `created_at` is the join time. No attempts, no answers, no score, no code consumption. The quiz's code, invite token, and counters are untouched.
5.8.6. **PAR-066 — Postconditions.** Derived state `registered`; the student is now assigned to the quiz; `CanRegister` false, `CanCancel` true, `CanStart` true on the intro. The response is 303 `/quiz/:id` with success flash `registered`.
5.8.7. **PAR-067 — Errors/copy.** Already live → 303 intro with info `already_registered`, no duplicate. Closed between page load and POST → 404 (resource no longer joinable) or, if a registration exists, the idempotent 303; the lock decides and the response matches the committed state. Zero questions → 303 intro with error flash `quiz-empty`, no registration. Database failure → 500 with rollback.
5.8.8. **PAR-068 — Provenance/idempotency.** `created_at` only; students have no deleter column on registrations and PAR does not invent one. A double click, refresh-and-resubmit, or two tabs: the first inserts, the second sees the row under the quiz lock or catches the duplicate key and returns the same 303 with `already_registered`. Exactly one live row (INV-06).
5.8.9. **PAR-069 — Invite path reuse.** Invite auto-registration invokes PAR-064–PAR-068 with the actor resolved from the token instead of a POST; the same locks, checks, and unique-key handling apply. The only difference is the success flash (`registered`) and the 303 target.

### 5.9 `POST /quiz/:id/cancel` — cancel a registration

5.9.1. **PAR-070 — Route/actor.** `POST /quiz/:id/cancel`. Student with a live registration, or anonymous identity with a live registration. CSRF required. A teacher or visitor gets 404 (after IDN's login redirect for visitors).
5.9.2. **PAR-071 — Authorization.** The registration is derived from the actor (student id or identity cookie), never from a form field, so one participant cannot cancel another's. No live registration for this actor and quiz → 404. Quiz draft/soft-deleted → 404.
5.9.3. **PAR-072 — Preconditions.** Quiz `state = open`; registration live; zero live attempts. Any attempt (in progress or submitted) blocks cancellation (R-10, R-12, F-§23 #9). Closed quiz blocks cancellation even with zero attempts (R-10).
5.9.4. **PAR-073 — Transaction and locks.** One transaction: shared-lock the quiz row and read state; exclusive-lock the registration row; count live attempts with a locking read (the registration lock serializes starts); if state is not open → rollback with `not-open`; if count > 0 → rollback with `cancel-blocked`; otherwise set `deleted_at = now`; commit. Lock order quiz → registration (§9.1); ATT start takes the same order, so a start and a cancel cannot both win and a concurrent close waits for both.
5.9.5. **PAR-074 — Effects.** Soft-delete exactly the registration row. No attempts exist by guarantee, so no cascade is needed. The anonymous identity and its cookie stay; the student account stays; other registrations stay. `updated_at` changes by the row update.
5.9.6. **PAR-075 — Postconditions.** Student: 303 `/quiz/:id` with info flash `cancelled`; the intro now offers **Daftar** while the quiz remains open. Anonymous: 303 `/join` with info flash `cancelled`, because the anonymous intro requires a live registration and would 404; the identity cookie remains so a re-entry through code/invite prefills and reuses it.
5.9.7. **PAR-076 — Errors/copy.** Attempts exist → 303 intro (if it remains visible) with error flash `cancel-blocked`; anonymous → 303 `/quiz/:id` (still visible because the registration exists) with the same flash. Closed → 303 intro with error flash `not-open`. Missing registration → 404. CSRF → 403. Database failure → 500, registration untouched.
5.9.8. **PAR-077 — Provenance/idempotency.** Only `deleted_at` records the cancellation; there is no actor column on registrations and no audit row (OQ-09). A double-submit: first commits, second finds no live registration and returns 404. This is the one non-idempotent-looking outcome; it is safe because the first request's 303 is the user's page and a retry cannot resurrect or double-delete anything. The UI disables the button after submit; the 404 page offers "Ke dasbor".

### 5.10 `POST /quiz/:id/attempts` — start or resume, handoff to ATT

5.10.1. **PAR-078 — Route/actor.** `POST /quiz/:id/attempts`. Student or anonymous participant with a live registration. CSRF required. Visitor → login; teacher → 404.
5.10.2. **PAR-079 — Preconditions.** Quiz live; live registration for this actor; at least one live question (defense in depth); for students, current eligibility for the quiz (R-12: checked at registration and at attempt start); for anonymous, public quiz (always eligible). No live registration → 303 intro with error `not-registered` for students (recoverable), 404 for anonymous (no intro to return to).
5.10.3. **PAR-080 — Resume rule.** If a live attempt with `submitted_at IS NULL` exists, do not create anything; 303 to that attempt page. This is allowed even when the quiz is closed, because R-10 preserves in-progress attempts. Resume wins over every quota or eligibility check.
5.10.4. **PAR-081 — Start guards.** Quiz must be `open`; otherwise 303 intro with error flash `not-open` (closed) or 404 (draft). Attempt count = live attempts. `max_attempts` is the total allowed; `allow_retake = false` or `max_attempts` 0/1 means one attempt. If count ≥ max, or if count > 0 and `allow_retake = false`, 303 intro with error flash `attempts-exhausted`. Treat a stored `max_attempts = 0` as 1 and a stored `allow_retake = false` as max 1, logging WARN; the row is invalid under R-06, so fail closed rather than grant unlimited attempts.
5.10.5. **PAR-082 — Transaction and handoff.** One transaction: shared-lock the student profile row when the actor is a student, shared-lock the quiz row and re-read state, visibility, and live question count; exclusive-lock the registration row; re-check state, eligibility, question count, and quota; if an in-progress attempt exists, read its id and commit with resume; otherwise invoke ATT's attempt-start rule in the same transaction ([ATT: attempt start]), passing the locked registration, quiz id, and next attempt number = 1 + live count. ATT inserts the attempt (`attempt_no`, crypto-random `shuffle_seed`, `created_at`), the answer-slot rows, and returns the id. Commit. PAR never inserts an attempt or answer row.
5.10.6. **PAR-083 — Effects.** Exactly one new `quiz_attempts` row and its answer slots from ATT; no registration change; no quiz change. `created_at` is the start time. No status column, no `started_at` (R-07). The attempt's presence flips the derived participation state to `in_progress`.
5.10.7. **PAR-084 — Responses.** New attempt → 303 `/attempt/:id` with success flash `started`. Resume → 303 `/attempt/:id` with no flash (the page already shows progress). Blocked → 303 intro with the exact flash. Zero questions → 303 intro with `quiz-empty`. DB failure → 500, no attempt.
5.10.8. **PAR-085 — Concurrency/idempotency.** The registration lock serializes two start clicks: the first inserts attempt N, the second sees it (if unsubmitted) and resumes; if the first submitted before the second, the second starts N+1 only if quota remains, else exhausts. The unique key `uq_attempt_number` is ATT's backstop. `attempt_no` is sequential from 1; no gaps are created by PAR. A retry after a lost response lands on the same attempt (resume).
5.10.9. **PAR-086 — Eligibility after profile change.** A student registered while eligible keeps existing attempts, results, and history even if IDN later changes grade or major (R-12). They may resume an in-progress attempt and view results. A **new** attempt (including a retake) requires current eligibility; if no longer eligible, `CanStart` is false and a forged start is 404. This is the literal reading of R-12 and is flagged in OQ-05.

### 5.11 `GET /quiz/:id` — quiz intro

5.11.1. **PAR-087 — Route/actor.** `GET /quiz/:id`. Student, anonymous identity with a live registration, owner teacher (redirect), or visitor with a ticket (redirect identify). Everyone else 404.
5.11.2. **PAR-088 — Student access.** Draft/soft-deleted → 404. Open public → 200. Open private eligible → 200. Open private ineligible with no registration → 404; with a registration → 200 with a `wrong_audience` warning in `Error`. Closed with a live registration → 200; closed without → 404. The ineligible-with-registration case exists only after IDN changes the profile; the page must still show their attempts, history link, and (if zero attempts and open) cancel.
5.11.3. **PAR-089 — Anonymous access.** Requires the identity cookie **and** a live registration for this quiz; otherwise 404 (a ticket redirects to identify, §4.4). Any quiz state except draft is visible; draft/soft-deleted → 404. R-12's closed-row 404 for visitors/anonymous means "no live registration"; an anonymous identity holding one keeps read-only intro and history access under R-10.
5.11.4. **PAR-090 — Teacher access.** Owner → 303 `/teacher/quizzes/:id` (the authoring detail is the teacher's intro); non-owner → 404.
5.11.5. **PAR-091 — Model.** `QuizIntro` with quiz (title, teacher name, question count, state, visibility/audience, code for public only, schedule labels, review-mode label), description, participation state, attempts compact, attempts used, max attempts, can-register/can-cancel/ can-start/can-retry, primary and secondary actions, joined-at, error. No score is invented; a hidden score renders `—`.
5.11.6. **PAR-092 — Action derivation.** `CanRegister` = student + open + eligible + no live registration (always false for anonymous: identify already registered). `CanCancel` = live registration + open + zero attempts. `CanStart` = live registration + open + no in-progress + quota remaining + (student: current eligibility) + (count = 0 or allow_retake). `CanRetry` = `CanStart` and count ≥ 1. Resume is not `CanStart`: an in-progress attempt is always offered as **Lanjutkan** regardless of state. Precedence: resume → register → start/retry → last result → history.
5.11.7. **PAR-093 — Copy/status.** 200. Closed: warning "Kuis sudah ditutup." plus "Jawaban tidak lagi dapat diubah atau ditambah." only if no in-progress attempt; if one exists, say it can be resumed. Hidden scores: `—`. No question body, no key, no score list is rendered on the intro.
5.11.8. **PAR-094 — Chrome.** Anonymous viewers get a participant chrome, not the student account chrome: no Dasbor/Riwayat/Profil links, no logout, display name + class, "Ganti identitas" link, back link to `/join`. The current student layout hardcodes those links and a "Kembali ke dasbor" label; that is a view-layer gap recorded in OQ-08 and must be fixed without changing this route's data contract.
5.11.9. **PAR-095 — Idempotency.** Read-only; no `last_seen_at` write, no cookie change except clearing a corrupt identity cookie (PAR-006).

### 5.12 `POST /teacher/anonymous/:id` — edit an anonymous identity

5.12.1. **PAR-096 — Route/actor.** `POST /teacher/anonymous/:id`. Any active teacher (school-wide, R-05). CSRF required. `:id` is `anonymous_participants.id`, not a registration id. Unknown, soft-deleted, or non-positive id → 404.
5.12.2. **PAR-097 — Preconditions.** The identity is live. The edit does not require the identity to have a registration; an identity with all registrations cancelled is still editable. A teacher with `must_change_password` never reaches the handler (IDN gate).
5.12.3. **PAR-098 — Validation.** Exactly the identify rules (PAR-055): `full_name` trim/collapse, 2–100 runes, no control characters; `grade_level` 10/11/12; `major_id` live major. Duplicate names are allowed and expected (R-02). There is no uniqueness check on names, no cookie/token field, and no registration field. On failure: 422 re-render the edit page with submitted values, the live major list, and `FieldErrors`; no write.
5.12.4. **PAR-099 — Transaction/effects.** One transaction: exclusive-lock the identity row, shared-lock the selected live major row, then single-row update of `display_name`, `grade_level`, `major_id`; `updated_at` moves. No `last_seen_at` change (a teacher edit is not participant activity). No cookie change: the browser keeps the same identity token and therefore sees the new values on its next page. No registration, attempt, score, or answer is rewritten.
5.12.5. **PAR-100 — Postconditions/status.** 303 `/teacher/anonymous?flash=saved`. The list and every screen that shows the name/class (intro, history, results, monitor, recap) now join the new value; there are no name snapshots (R-25), so historical labels change. This is deliberate and recorded in OQ-10.
5.12.6. **PAR-101 — Errors/provenance/idempotency.** Missing → 404; DB failure → 500 with no partial update. No audit row and no last-editor column exist; `updated_at` is the only trace. Two teachers editing concurrently: last write wins; the second overwrites the first. Repeating the same values is a no-op that still returns the saved flash.

### 5.13 `POST /teacher/anonymous/:id/delete` — delete an anonymous identity

5.13.1. **PAR-102 — Route/actor.** `POST /teacher/anonymous/:id/delete`. Any active teacher. CSRF required. `:id` is the participant id; unknown/soft-deleted → 404 (a second delete is 404, not a success).
5.13.2. **PAR-103 — Authorization/preconditions.** School-wide. No confirmation field is trusted; the browser's confirm dialog is UX only. Deleting an identity with attempts is allowed and destructive (R-14): the confirmation copy warns that identity and attempt history are removed.
5.13.3. **PAR-104 — Transaction and locks.** OPS cascade in one transaction, root first: exclusive-lock the identity row; exclusive-lock its live registrations; exclusive-lock the attempts under them; stamp one UTC timestamp on the identity, registrations, attempts, answers, selections, and focus events; set `deleted_by_account_id` on the identity root to the acting teacher and leave it NULL on every descendant (R-14, INV-18). Commit. Lock order identity → registration → attempt → answer → selection is mandatory (§9.1).
5.13.4. **PAR-105 — Effects.** The identity and all its participation data disappear from every live read. The browser cookie is untouched but resolves to nothing (PAR-011). The quiz's distinct-live-registration count drops; monitor/results/history for that quiz lose the rows. No other participant is touched.
5.13.5. **PAR-106 — Postconditions/status.** 303 `/teacher/anonymous?flash=deleted`. The list no longer shows the row. The old cookie's next identify creates a new identity (the unique key includes `deleted_at`, INV-19).
5.13.6. **PAR-107 — Errors.** Missing/already deleted → 404. In-flight submit race: ATT submit must lock the registration/attempt and re-check liveness; after the cascade it sees no live row and returns 404, never writing a live answer under a deleted parent (INV-18, §9.2). DB failure → 500 with the whole cascade rolled back.
5.13.7. **PAR-108 — Provenance/idempotency.** Root deleter recorded; descendants NULL. A retry is 404; no restore exists (INV-29). There is no "undelete" and no way to attach the old attempts to a new identity.

### 5.14 `POST /teacher/majors` — create a major

5.14.1. **PAR-109 — Route/actor.** `POST /teacher/majors`. Any active teacher; CSRF. No owner scope. The form has exactly `name`; the contract has no code field.
5.14.2. **PAR-110 — Validation.** Trim, collapse internal whitespace, 2–50 runes, at least one letter or digit, no control characters. Case-insensitive uniqueness among live majors (the `utf8mb4_unicode_ci` name collation makes "IPA" and "ipa" collide); a duplicate is a 422 field error `Nama jurusan sudah dipakai.` A name that matches only a soft-deleted major is allowed (INV-19). On failure: 422 re-render the majors page with the submitted create value and the field error; existing rows still render.
5.14.3. **PAR-111 — Code generation.** The schema requires `majors.code` NOT NULL, unique, ≤10 ASCII, but the form has no code. Generate one, never show it, and never change it on rename: uppercase ASCII letters/digits from the name, first 10 characters; if empty use `JUR`; if the candidate is taken by a live major, append `2`, `3`, … within the 10-character ceiling; if the suffix space is exhausted, use a crypto-random 4-character suffix and retry. This is the smallest schema-compatible resolution; the missing form field is recorded in OQ-11.
5.14.4. **PAR-112 — Transaction/effects.** One transaction: insert the major with `code`, `name`; `created_at/updated_at` default. Catch `uq_majors_name` → 422 duplicate; catch `uq_majors_code` → regenerate once, then 500. No propagation write to students, anonymous participants, or quizzes (there are none yet, R-13).
5.14.5. **PAR-113 — Postconditions/status.** 303 `/teacher/majors?flash=saved`. The major is immediately selectable in identify, student registration/edit, anonymous edit, and quiz targets.
5.14.6. **PAR-114 — Provenance/idempotency.** `created_at` only; no creator column. A retry with the same name is a duplicate 422, not a second row. Soft-deleted names are reusable.

### 5.15 `POST /teacher/majors/:id` — rename a major

5.15.1. **PAR-115 — Route/actor.** `POST /teacher/majors/:id`. Any active teacher; CSRF. `:id` is a live major id; unknown/soft-deleted → 404.
5.15.2. **PAR-116 — Validation.** Same name rules as PAR-110. A rename to the current name is a success no-op (no duplicate error against itself). A rename to another live major's name is a 422 duplicate field error on the majors page; the submitted value is preserved for that row.
5.15.3. **PAR-117 — Transaction/effects.** One transaction: exclusive-lock the major row, then single-row update of `name` (and `updated_at`). The `code` is immutable. There is no denormalized propagation write (R-13): every screen joins the current name, so students' class labels, anonymous labels, quiz audience labels, and historical result labels change at once. No student/anonymous/quiz row is touched.
5.15.4. **PAR-118 — Postconditions/status.** 303 `/teacher/majors?flash=saved`. Eligibility is unaffected because it compares ids, not names. A private quiz's audience text re-renders with the new name.
5.15.5. **PAR-119 — Errors/provenance/idempotency.** 404 missing; 422 duplicate/length; 500 on DB failure. No audit row, no last-editor column. Concurrent renames: last write wins. Repeating the same rename returns the saved flash.

### 5.16 `POST /teacher/majors/:id/delete` — delete an unused major

5.16.1. **PAR-120 — Route/actor.** `POST /teacher/majors/:id/delete`. Any active teacher; CSRF. Unknown/soft-deleted → 404.
5.16.2. **PAR-121 — Preconditions.** No live student references it, no live anonymous participant references it, and no live quiz targets it (open or closed; drafts included). This is INV-23 and R-13. There is no force option.
5.16.3. **PAR-122 — Transaction and locks.** One transaction: exclusive-lock the major row; count live references in `students`, `anonymous_participants`, and `quizzes`; if any count > 0, roll back with the `major-in-use` flash (303, not 422; it is a state conflict, not a field error); otherwise set `deleted_at = now` and `deleted_by_account_id = actor`; commit. Writers that set a major reference (IDN student create/edit, PAR identify/edit, AUT quiz create) must take a shared lock on the major row in their own transaction before writing, or the delete's exclusive lock cannot serialize them (§9.3.4, §10.2). Without that protocol a concurrent insert could leave a live child under a deleted major, violating INV-18/INV-23.
5.16.4. **PAR-123 — Effects.** Soft-delete exactly the major row. No cascade: an unused major has no children. The name and code become reusable because the unique keys include `deleted_at` (INV-19). Existing soft-deleted children, if any, are untouched.
5.16.5. **PAR-124 — Postconditions/status.** 303 `/teacher/majors?flash=deleted`. The row leaves the list. Referenced majors cannot be deleted even if all referencing students were deleted long ago but the quiz remains; a closed quiz keeps its target forever because quizzes are only deletable as drafts (R-10). This permanent pinning is a product consequence recorded in OQ-12.
5.16.6. **PAR-125 — Errors/idempotency.** Referenced → 303 `major-in-use`; the list re-renders with the reason "Masih dipakai." (the button is disabled when `CanDelete` is false, but the server still checks). Missing → 404. A retry after success is 404. No restore.

## 6. Queries and read models

### 6.1 `GET /student` — student dashboard (PAR-126–PAR-141)

6.1.1. **PAR-126 — Route/actor.** Authenticated student only; teacher/anonymous/visitor never reach it (IDN redirect/404). The whole page is built in one read-only transaction with a consistent snapshot so the five stats and four lists agree.
6.1.2. **PAR-127 — Eligibility predicate.** A live quiz is assigned to a student when it is `open` and either `public` (no targets) or `private` with `target_grade = student.grade_level` and `target_major_id = student.major_id`, both compared by value/id. Schedule fields are ignored (R-15). Drafts and closed quizzes are not newly assigned.
6.1.3. **PAR-128 — Assigned.** Count distinct quiz ids in the union of (assigned open quizzes) and (live registrations of this student, any quiz state). A registration on a closed quiz still counts; a registration on a quiz the student is no longer eligible for still counts through the registration arm. A cancelled registration does not count. No double counting.
6.1.4. **PAR-129 — Registered.** Count live registrations with zero live attempts **and** quiz `open`. Closed zero-attempt registrations are excluded from the tile because they are not actionable; they remain in history (§8.3). This is a product call consistent with the demo fixture.
6.1.5. **PAR-130 — InProgress/Completed.** Count live registrations in the derived states `in_progress` / `completed`, regardless of quiz state (in-progress after close is real, R-10).
6.1.6. **PAR-131 — BestScore.** Maximum `final_score` over live attempts of this student's live registrations where the quiz's `show_score = true` and `final_score IS NOT NULL` (R-21). Hidden scores and NULL scores never contribute. Empty → nil, rendered `—`; zero is a real value rendered `0%`. Convert to a whole percent with half-up rounding at the model boundary (F-§6.1).
6.1.7. **PAR-132 — Ready list.** One card per live registration with zero live attempts on an `open` quiz. Sort: registration `created_at` descending, tie registration id ascending (R-21). Uncapped. Primary **Mulai kuis** (POST attempts), secondary **Batalkan pendaftaran** (POST cancel); a stale page may fail those, which is handled by §5.9/§5.10.
6.1.8. **PAR-133 — Live list.** One card per live registration with an in-progress attempt, any quiz state. Sort: latest attempt `updated_at` descending, tie attempt id ascending. Uncapped. Primary **Lanjutkan** to `/attempt/:id`. A closed quiz card still shows the closed badge and still offers resume (R-10).
6.1.9. **PAR-134 — Latest list.** Open, eligible, live quizzes with no live registration by this student. Sort: `published_at` descending (the moment it became available), tie quiz id ascending. Cap 6 (R-21). Primary **Lihat kuis** to the intro. There is no "all quizzes" route; the cap is the discovery surface (F-§10.1). A quiz with zero live questions is excluded and logged if open.
6.1.10. **PAR-135 — Recent list.** The 5 most recent submitted attempts of this student across all live registrations, ordered `submitted_at` descending, tie attempt id ascending. Each row carries quiz title, attempt id, submitted-at label, score (nil when hidden), waiting flag, result link. `Waiting` = `final_score IS NULL`; it is shown even when `show_score = false` (waiting is not a score, R-18). A hidden but graded score renders `—`, never `0`.
6.1.11. **PAR-136 — Card contract.** Title, teacher name, question count, audience label for private only, code for public only, start/end labels, state badge, participation badge, attempts used/max, joined-at, one primary action. Attempts used = live attempt count. Max = quiz `max_attempts` (0 rendered as 1 after the fail-closed normalization). No score is shown on a card except through the Recent table.
6.1.12. **PAR-137 — Empty state.** Rendered only when all four lists are empty and all four counts are zero: title "Belum ada kuis untuk Anda.", body "Kuis baru akan muncul di sini setelah guru membukanya untuk kelas Anda.", action **Gabung dengan kode** → `/join`. If counts are non-zero but lists are empty (dead closed registrations), render the stats and no empty state; the history link remains the path.
6.1.13. **PAR-138 — Greeting/chrome.** First word of the full name; WIB date line in the long form; class label roman grade + major; nav key `dasbor`. No relative dates (F-§6.2).
6.1.14. **PAR-139 — Forbidden data.** No other student's rows, no quiz the student cannot see, no answer bodies, no keys, no teacher-only counts, no focus-loss totals on student surfaces (the current history template's focus flag is a view gap, OQ-08).
6.1.15. **PAR-140 — Query semantics.** No query parameters are read; `?page`, `?q`, `?sort` are ignored rather than 422. The page is read-only, idempotent, and safe to refresh.
6.1.16. **PAR-141 — Failure.** Database failure → 500 page. A partially built page is never sent; the snapshot transaction is rolled back and the error page is rendered.

### 6.2 `GET /student/history` — history list (PAR-142–PAR-153)

6.2.1. **PAR-142 — Route/actor.** Authenticated student only. One row per live registration of this student, including closed quizzes and zero-attempt registrations.
6.2.2. **PAR-143 — Filters/search.** None. `q`, `grade`, `major_id`, `sort`, and `filter` are ignored; there is no filtered-empty state because there is no filter. Unknown query values never change the page and never 422 (R-21).
6.2.3. **PAR-144 — Sort.** Last activity descending, tie registration id ascending. Last activity = max(registration `created_at`, every live attempt's `created_at`, `submitted_at`, `updated_at`). A zero-attempt row's last activity is its registration time. Attempts do not create separate rows (F-§4.4 #29).
6.2.4. **PAR-145 — Pagination.** Page size 10 (R-21 student list). `page` non-numeric, zero, or negative → 1; greater than the last page → last page (clamp, never an empty page). Total = live registrations. The pagination partial is built from the same query (only `page` is emitted).
6.2.5. **PAR-146 — Row model.** Quiz ref, teacher name, participation state, attempts (all live, attempt_no ascending), best score, show-score flag, last-attempt label, detail link. `BestScore` = max visible `final_score` of that quiz's attempts; hidden or all-NULL → nil → `—`.
6.2.6. **PAR-147 — Show-score behavior.** `show_score = false` forces every per-attempt score and the best score to nil; a graded score renders `—`, an ungraded attempt renders "Menunggu penilaian" because grading state is not a score (R-18). `show_score = true` renders whole percentages; zero is `0%`.
6.2.7. **PAR-148 — State.** Derived per registration. An in-progress attempt on a closed quiz still shows "Sedang berlangsung" and a resume link. A zero-attempt registration on a closed quiz shows "Terdaftar"; the detail page explains that the quiz is closed.
6.2.8. **PAR-149 — Attempts count label.** "N dari M percobaan" using live attempt count and the quiz's normalized max. The row model must carry the max; the current `HistoryRow` lacks it (view delta, OQ-08). A max of 1 renders "1 percobaan", not "1 dari 1".
6.2.9. **PAR-150 — Class/identity.** The page is the student's own; class label appears in the chrome, not per row. No other participant's name appears.
6.2.10. **PAR-151 — Empty state.** "Belum ada riwayat kuis." / "Kuis yang Anda ikuti akan tercatat di sini beserta setiap percobaannya." / action **Gabung kuis** → `/join`. Shown when the student has zero live registrations; a cancelled registration alone yields the empty state.
6.2.11. **PAR-152 — Link targets.** Detail link `/student/history/:quizID` using the quiz id (not the registration id); result links `/attempt/:id/result` for submitted attempts and `/attempt/:id` for in-progress ones.
6.2.12. **PAR-153 — Failure/idempotency.** Read-only, repeatable. 500 on DB failure; no partial pagination.

### 6.3 `GET /student/history/:quizID` — history detail (PAR-154–PAR-162)

6.3.1. **PAR-154 — Route/actor.** Authenticated student only. `:quizID` positive integer; otherwise 404.
6.3.2. **PAR-155 — Authorization.** The student must have a live registration for that quiz. No registration → 404 (never another student's data, never a quiz they merely can see). A soft-deleted quiz or registration → 404.
6.3.3. **PAR-156 — Content.** Quiz ref/title, teacher, audience label, current participation state, show-score flag, all live attempts ordered attempt_no ascending (start, submit, state, score/waiting, result/resume link), max attempts, can-retry, retry link, back link, current class label.
6.3.4. **PAR-157 — Start/retry action.** The detail's start action is enabled iff quiz `open`, live registration, no in-progress attempt, live count < normalized max, `(count = 0 or allow_retake = true)`, and (student) current eligibility. The button label is **Mulai kuis** when count = 0 and **Coba lagi** otherwise; this is the detail-surface form of `CanStart`, including the first attempt. A closed quiz is always false. An in-progress attempt is not a retry; the row's action is **Lanjutkan**.
6.3.5. **PAR-158 — Scores.** Same visibility rules as PAR-147. Focus-loss counts are not shown on student surfaces (OQ-08); the model supplies 0 so the template's focus flag never renders.
6.3.6. **PAR-159 — Empty attempts (zero-attempt registration).** Show the empty-attempt panel "Belum ada percobaan." / "Percobaan pertama akan tercatat setelah Anda memulai kuis dari halaman kuis." with **Buka halaman kuis** when retry is possible; if closed, replace the action with the closed explanation and no start button.
6.3.7. **PAR-160 — Sort/pagination.** No pagination: all attempts of one registration. Attempts ascending; ties impossible (`uq_attempt_number`).
6.3.8. **PAR-161 — Query parameters.** Ignored; no 422. Read-only.
6.3.9. **PAR-162 — Failure.** 404 for missing/foreign; 500 on DB failure.

### 6.4 `GET /teacher/anonymous` — anonymous list (PAR-163–PAR-172)

6.4.1. **PAR-163 — Route/actor.** Any active teacher; school-wide. No owner scope.
6.4.2. **PAR-164 — Row grain.** One row per live `anonymous_participants` identity, never per registration and never grouped by name (R-02, S-§6.4). Two identical names are two rows.
6.4.3. **PAR-165 — Search.** `q` is trimmed, capped at 100 runes, and matched as a case-insensitive substring of `display_name` using the column collation. Empty `q` = no filter. Invalid/overlong values are truncated, not 422.
6.4.4. **PAR-166 — Sort.** Last activity descending, tie participant id ascending. Last activity = max(`last_seen_at`, live registration `created_at`, live attempt `updated_at`, `submitted_at`); null only if the row has no timestamps (impossible in practice) → `—`.
6.4.5. **PAR-167 — Columns.** Name, class label (current grade + current major name), distinct live quizzes joined, last activity, edit/delete links. Quizzes joined = count of distinct live registrations; a cancelled registration does not count.
6.4.6. **PAR-168 — Pagination.** Page size 10; `page` clamped as PAR-145. Total = matching live identities. Search and page compose; links preserve `q`.
6.4.7. **PAR-169 — Empty vs filtered-empty.** No rows and no `q`: "Belum ada peserta anonim." / "Peserta yang bergabung tanpa akun akan muncul di sini." No rows with `q`: "Tidak ada peserta anonim yang cocok." / "Coba kata kunci lain atau atur ulang pencarian." with **Atur ulang pencarian** → `/teacher/anonymous`.
6.4.8. **PAR-170 — Forbidden data.** No cookie value or hash, no account link (there is none), no attempt answers, no score editing. The page may state that identity is cookie-based.
6.4.9. **PAR-171 — Detail link.** `/teacher/anonymous/<participant_id>`; the current fixture uses a registration id and falls back — that is demo-only and must be replaced (OQ-08).
6.4.10. **PAR-172 — Failure.** Read-only; 500 on DB failure. A teacher sees all identities, not just those in their quizzes (R-05).

### 6.5 `GET /teacher/anonymous/:id` — anonymous edit form (PAR-173–PAR-178)

6.5.1. **PAR-173 — Route/actor.** Any active teacher; `:id` participant id; live row required, otherwise 404.
6.5.2. **PAR-174 — Content.** `full_name`, `grade_level`, `major_id`, live majors, action `/teacher/anonymous/:id`, cancel `/teacher/anonymous`, delete action, no cookie/token display.
6.5.3. **PAR-175 — Prefill.** Current live values, not the latest registration's snapshot (there are none).
6.5.4. **PAR-176 — Security.** The form includes the injected CSRF token; the GET itself only reads.
6.5.5. **PAR-177 — Query/empty.** No filters. A soft-deleted id is 404, not an empty form.
6.5.6. **PAR-178 — Failure.** 500 on DB failure; no partial form.

### 6.6 `GET /teacher/majors` — majors list (PAR-179–PAR-188)

6.6.1. **PAR-179 — Route/actor.** Any active teacher; school-wide.
6.6.2. **PAR-180 — Grain/sort.** One row per live major, name ascending using the column collation, tie id ascending. No pagination (the school list is small and the create form lives on the same page).
6.6.3. **PAR-181 — Counts.** `StudentCount` = live students with the major; `QuizCount` = live quizzes (any state) targeting it. `CanDelete` = StudentCount = 0 **and** QuizCount = 0 **and** live anonymous participants with the major = 0. The anonymous count is not in the current row model; add it or fold it into the disabled reason (OQ-11). A disabled button still has a server-side check (PAR-121).
6.6.4. **PAR-182 — Actions.** Inline rename form (`name`), create form (`name`), delete form. All carry CSRF.
6.6.5. **PAR-183 — Search/filter.** None; no query parameters are read.
6.6.6. **PAR-184 — Empty state.** "Belum ada jurusan." / "Tambahkan jurusan pertama agar siswa dan kuis dapat memakainya." The create form remains visible.
6.6.7. **PAR-185 — Copy.** Delete confirmation warns that deletion is permanent; a referenced row shows "Masih dipakai." and a disabled button.
6.6.8. **PAR-186 — Class label source.** Every class label in the app joins this list by id; a rename is visible immediately everywhere (R-13).
6.6.9. **PAR-187 — Forbidden.** No code column (internal), no student names, no per-major attempt counts.
6.6.10. **PAR-188 — Failure.** Read-only; 500 on DB failure.

### 6.7 Formatting and derivation helpers (PAR-189–PAR-196)

6.7.1. **PAR-189 — Score.** Storage 0–1, four decimals; model whole percent 0–100, half-up; display `84%`; absent `—`; average one decimal with comma (teacher surfaces only).
6.7.2. **PAR-190 — Class label.** Roman grade (`X`, `XI`, `XII`) + current major name, e.g. `XII IPA`; no section/rombel (F-§6.3).
6.7.3. **PAR-191 — Dates.** UTC stored, WIB displayed; `20 Okt 2026`, `09.42 WIB`, full form for headers; unset `—`; no relative dates.
6.7.4. **PAR-192 — Counts.** Thousands `.`; no decimals; "N dari M percobaan"; "N kuis"; "N peserta".
6.7.5. **PAR-193 — Attempt state.** `in_progress` = no `submitted_at`; `completed` = submitted; waiting = submitted and `final_score` null. No third state.
6.7.6. **PAR-194 — Best score.** Max of non-null visible final scores; never an average, never the latest.
6.7.7. **PAR-195 — Participation state.** Derived table §4.1; no stored value.
6.7.8. **PAR-196 — Null/zero distinction.** A missing score is nil and renders `—`; zero is a real score and renders `0%`. A missing date renders `—`; a zero count renders `0`.

## 7. Invariants owned

7.1. Cited global invariants: INV-03 (role-typed FKs), INV-04 (student active), INV-05 (exactly one participant), INV-06 (one live registration per quiz+participant), INV-17 (score visibility), INV-18 (no live child under a deleted parent), INV-19 (unique keys include `deleted_at`), INV-20 (UTC), INV-23 (referenced major cannot be deleted), INV-25 (derived states), INV-27 (cookie is identity), INV-28 (CSRF), INV-29 (no hard delete/restore).
7.2. **PAR-INV-01 — Identity is the hash.** An anonymous participant is selected only by the live row whose `cookie_token_hash` equals SHA-256 of the presented cookie. No query filters, groups, or matches on `display_name`, grade, or major. Enforcement: app + `uq_anon_cookie`.
7.3. **PAR-INV-02 — One live registration.** At most one live registration per (quiz, student) and per (quiz, anonymous participant). Enforcement: DB `uq_reg_student`, `uq_reg_anon` + app retry.
7.4. **PAR-INV-03 — Derived participation.** Registration state is a pure function of live attempt timestamps; no stored status. Enforcement: app.
7.5. **PAR-INV-04 — Quota arithmetic.** Attempt count = count(live attempts); start allowed iff count < normalized max and (count = 0 or `allow_retake`) and no in-progress attempt; an in-progress attempt is resumed, never duplicated. Enforcement: app + ATT registration lock + `uq_attempt_number`.
7.6. **PAR-INV-05 — Cancellation window.** Cancel is allowed iff the registration is live, the quiz is open, and live attempts = 0. Enforcement: app transaction.
7.7. **PAR-INV-06 — Eligibility tested twice.** Registration and every start test the same predicate against the live profile and the quiz targets; existing attempts survive later profile changes. Enforcement: app.
7.8. **PAR-INV-07 — No zero for hidden.** A hidden or absent score is nil/`—`; it is never coerced to 0 anywhere in dashboard, history, or intro. Enforcement: app read model + template test.
7.9. **PAR-INV-08 — Major reference guard.** A major with any live student, anonymous participant, or quiz target cannot be soft-deleted. Enforcement: app transaction + writer shared-lock protocol.
7.10. **PAR-INV-09 — Ticket provenance.** An identify POST is accepted only with a valid signed ticket bound to the same quiz and a still-resolvable secret. Enforcement: app.
7.11. **PAR-INV-10 — Current labels, not snapshots.** Names, class labels, quiz titles, and major names are read live; there is no per-attempt snapshot. Enforcement: schema has no snapshot columns + app joins.
7.12. **PAR-INV-11 — Actor scoping.** Every registration/history/attempts read and write is scoped to the session student or the cookie identity; ids in the path never select another participant. Enforcement: app + 404.
7.13. **PAR-INV-12 — Close is a hard gate.** New registration, cancellation, and new attempt start are refused after close; resume of an existing in-progress attempt is allowed. Enforcement: app + ATT.
7.14. **PAR-INV-13 — One cookie, one identity.** A browser holds one `quiz_participant` cookie; a copied token is the same bearer identity on another device. A distinct identity requires clearing or replacing the cookie. Enforcement: cookie mechanics + unique hash.
7.15. **PAR-INV-14 — Provenance.** Deleting an anonymous participant or a major sets `deleted_by_account_id` on the root and NULL on descendants; edits have only `updated_at`. Enforcement: app + OPS cascade.

## 8. Edge cases and failure modes

### 8.1 Stale forms, retries, navigation

8.1.1. A rendered form is a snapshot of authorization at render time, never a grant. Every POST re-evaluates actor, liveness, state, eligibility, and quota under lock (§9.1). The following outcomes are normative and override any friendlier UI affordance:

| Stale situation | Outcome |
|---|---|
| Register from an intro that has since closed | 404, no row |
| Register after another tab registered | 303 intro, info `already_registered` |
| Cancel after an attempt started | 303 intro, error `cancel-blocked`; row intact |
| Cancel after close | 303 intro, error `not-open`; row intact |
| Cancel a second time | 404; nothing restored |
| Start from an intro after close | 303 intro, error `not-open`, unless an in-progress attempt exists → resume |
| Start twice | First creates; second resumes the same attempt, or starts N+1 only per quota |
| Identify after close, no registration | 303 `/join?error=not_open`; ticket cleared |
| Identify after close, live registration | 303 intro; profile refreshed |
| Identify with a ticket older than 30 minutes | 404; no identity, no registration |
| Teacher edit on a deleted identity | 404 |
| Teacher delete a second time | 404 |
| Major rename/delete from a stale list | 404 |
| Major create with a name just taken | 422 duplicate field error |
| Identity-switch link with a stale ticket | 404 |

8.1.2. Only 422 preserves the submitted body. A 303 discards it and the flash is the whole explanation; 404 is deliberately uninformative (§3.5). A 429 or 500 also discards it, so the user must re-enter the action from the intro or list.
8.1.3. Every HTML response rendered for a session or an identity cookie carries `Cache-Control: no-store` and `Pragma: no-cache`. The join and invite landings may be cached only when they contain no identity data. This prevents the back button from disclosing a previous participant's name, class, or history after logout or identity switch.
8.1.4. A POST arriving after session expiry is stopped by IDN's login redirect before PAR runs; PAR writes nothing. The body is lost by design, and the post-login landing is the current intro.
8.1.5. A bfcache-restored page may show stale buttons and counts. Every button is still guarded by 8.1.1. `app.js` may refresh passive counts on `pageshow` but must never auto-submit a mutation.

### 8.2 Deleted or vanished entities

8.2.1. A quiz is deletable only while draft (R-10), so a live participant's quiz cannot vanish through a PAR route. If a quiz is soft-deleted out of band: code and invite resolution miss (`unknown_code`), direct intro/register/cancel/start/history-detail return 404, and list rows disappear on the next read. An in-flight transaction that locked the quiz first commits; one that reads after the delete aborts.
8.2.2. A deleted anonymous identity makes its browser a visitor on the next request; the bad cookie is cleared (PAR-006). Intro and every action return 404 because no live registration exists; the teacher list, monitor, and results lose the rows by cascade. There is no restore and no way to attach the old cookie to a new row (INV-29).
8.2.3. A deleted or deactivated student account has its sessions invalidated by IDN. PAR then treats the actor as absent: 404, no registration resurrection, no history. Cascades follow OPS.
8.2.4. A soft-deleted major cannot be selected. An in-flight identify/edit form that submits it gets 422 `Pilih jurusan yang valid.` A live student, anonymous participant, or quiz target cannot reference it (INV-23). If corruption leaves a dangling target, PAR logs ERROR and refuses to let the quiz be joined rather than inventing a label; the teacher surface must show the integrity problem, not a blank audience.
8.2.5. A teacher who becomes unapproved or deleted mid-session is stopped by IDN on the next request. No management write from that request commits.
8.2.6. If a registration is cancelled while the student still has the intro open, the next cancel or start is 404/`not-registered`; the open quiz intro still renders with **Daftar** for a student, while an anonymous visitor gets 404 because no live registration remains.
8.2.7. Attempts have no PAR delete route. If attempts vanish out of band while the registration survives, the derived state falls back to `registered` and a start may reuse attempt 1. This is corruption, must be logged, and must not be used to justify a "repair" route.

### 8.3 Closed-quiz dead and live registrations

8.3.1. A zero-attempt registration on a closed quiz is a dead row: visible in history and on the intro (read-only), counted in Assigned, excluded from the Registered tile, never cancellable or startable. The history detail explains closure and offers no action.
8.3.2. Close races are decided by the quiz row lock. Close takes it exclusively; register, cancel, and start take it shared before the registration lock. If close wins, the command sees `closed` and returns `not-open`/404; if the command wins, close waits and the committed registration or attempt is valid but no further action is possible. No transaction observes a half-closed quiz.
8.3.3. An in-progress attempt after close is resumable until submitted. Submitting it yields `completed` and a visible result under ATT's rules; retry is false forever (PAR-INV-12).
8.3.4. Schedule fields never gate access (R-15). A quiz past `ends_at` but still `open` behaves open; a quiz closed before `ends_at` is closed. Only the state column decides.
8.3.5. Keeping dead rows is the honest reading of R-10, but it produces a registration a student can never remove. The product objection and the alternative are recorded in OQ-05; no rule here creates a hidden cleanup path.

### 8.4 Identity and shared devices

8.4.1. The shared-browser case is explicit: the identity note plus **Ganti identitas** invokes PAR-052, which clears only the cookie. The first person's row, registrations, and attempts stay; the second person's identify creates a distinct identity; teachers see both, disambiguated by class and activity, never merged (PAR-012).
8.4.2. Switching without a valid ticket is 404. There is no identity picker, no name search, and no `?participant=` parameter; a participant can only act as the cookie they hold.
8.4.3. A user who clears cookies loses the browser's handle on that identity. Rejoining creates a new identity and a fresh attempt allowance; the old attempts remain teacher-visible. This is the accepted cap bypass (PAR-012), not a defect to patch with fingerprinting.
8.4.4. A copied cookie used in two browsers is the same bearer identity. Both tabs may act; the unique keys permit one live registration, starts serialize on the registration lock, and `last_seen_at` records the latest write. There is no device binding and no per-device revocation.
8.4.5. A student who still holds an anonymous cookie: the session wins (§3.1), IDN clears the identity cookie and ticket at login, and the anonymous row is neither merged nor shown in the student's history. Logging out does not resurrect that cookie.
8.4.6. An anonymous identity calling a student-only route gets 404, not a login redirect and not another participant's page. A visitor is redirected to login by IDN where a login could help.
8.4.7. A malformed `Cookie` header (wrong length, illegal characters, several values, oversized) is treated as no identity, never as a 500. The server clears only a cookie whose name is `quiz_participant`; it never echoes the header.
8.4.8. The token never appears in a URL, HTML, log line, or error page. Only the SHA-256 hash is stored; only the browser holds the token (PAR-004).
8.4.9. A teacher deleting an identity while that browser sits on the intro leaves a page that looks alive but every action 404s; the next GET clears the cookie and renders as a visitor. No flash survives from the deleted row.
8.4.10. Two identities with identical names are two rows, two histories, and two attempt allowances. A teacher must not be offered a merge, rename-to-unique, or "same person" shortcut; none exists.

### 8.5 Join, invite, and ticket corner cases

8.5.1. Codes are exact six-character digit strings, including leading zeros. They are never parsed as integers: `000123` and `123` are different inputs, and only the former can match.
8.5.2. Only leading and trailing ASCII whitespace is trimmed. An interior space, a non-breaking space, or a full-width digit fails the six-digit test and yields `unknown_code`, never 422 (PAR-029).
8.5.3. Invite tokens are exact, case-sensitive, 1–32 characters of `[A-Za-z0-9_-]`. URL decoding happens exactly once; encoded slashes and dots are rejected, not normalized.
8.5.4. Rotating a code or invite token invalidates every outstanding ticket at PAR-016 because the secret no longer resolves to the quiz. Identify then returns 404 and creates nothing. A live registration is unaffected and is reached through the dashboard or a fresh code.
8.5.5. An invite to a closed quiz with a live registration lands on the intro with `already_registered`; without one it renders the invite page with `not_open`. State is checked before audience, so a closed private quiz with an ineligible viewer still says `not_open` (PAR-038).
8.5.6. A direct `/quiz/:id` carrying a ticket for a different quiz does not redirect. The ticket is bound to one quiz; mismatch is 404, never a cross-quiz handoff.
8.5.7. On `GET /join/identify`, an absent or mismatched `?quiz=` redirects 303 to the canonical ticket quiz (PAR-046). On POST the same mismatch is 404: a form that was edited in transit is not a navigation request.
8.5.8. A ticket that expires while the form is being filled produces 404 on POST. The submitted name and class are lost, no identity is created, and the user must re-enter through the code.
8.5.9. A live registration always outranks quota on code/invite entry: the landing is `already_registered`, never `attempts-exhausted`. Quota is explained only on start.
8.5.10. A student entering a private quiz's nonexistent code gets `unknown_code`, never `wrong_audience` and never a hint that the quiz exists (OQ-06).
8.5.11. A teacher entering a code or invite is redirected before resolution. Teachers never mint tickets, never create registrations, and never see a participant landing.
8.5.12. Names are HTML-escaped on every surface. Control characters are rejected; zero-width and bidi characters are not silently stripped, but they cannot break the layout because the cell is escaped and length-capped.
8.5.13. An identity cookie plus a ticket for a public quiz whose registration was cancelled: entry mints a fresh ticket, identify reuses the identity, and only an explicit new submit creates a new registration. The cancelled row is not restored.

### 8.6 Quota, score, and formatting anomalies

8.6.1. `max_attempts = 0` and an incoherent `allow_retake = false` with `max_attempts > 1` are normalized to one attempt, with a WARN. Fail closed; never grant unlimited attempts from bad data (PAR-081).
8.6.2. Only live attempts count. A soft-deleted attempt is invisible to quota, state, and score, but no PAR route creates one; only an OPS cascade does, and then the registration is gone too.
8.6.3. Hidden score plus pending grading: the row says "Menunggu penilaian" because waiting is not a score. Hidden and graded: `—`. Visible and zero: `0%` (PAR-147, PAR-196).
8.6.4. A `final_score` outside `[0,1]`, non-finite, or non-numeric is rendered `—` and logged ERROR. It is never clamped to 0 or 100, because either value would fabricate a result.
8.6.5. If every live question disappears after registration (an AUT integrity failure), start returns `quiz-empty` without an attempt. Cancel remains available while the quiz is open and the registration has zero attempts.
8.6.6. A registration whose attempts were all soft-deleted out of band reads `registered` and is hidden from history per §4.1.3; this is an incident, not a state to design around.
8.6.7. Unset timestamps render `—`; a zero count renders `0`; neither is ever replaced by the other (PAR-196).
8.6.8. Attempt caps and page sizes are applied after filtering and inside the same snapshot as the rows, so a rendered "N dari M" never disagrees with the table beside it.

### 8.7 Session, CSRF, rate limit, and infrastructure

8.7.1. An expired session on a protected GET is redirected to login by IDN; on a public join GET the request still renders as a visitor. PAR itself never emits a login redirect.
8.7.2. CSRF failure is 403 with R-19 copy, before any read of the body or write. The transaction is not opened.
8.7.3. Rate limiting is 429 with `Retry-After` and R-19 copy, before any write. The identity cookie and join ticket survive, so a later retry needs no re-entry.
8.7.4. A database failure is 500 with the whole transaction rolled back. Read pages render the error page, never a partially filled model; list count, rows, and pagination come from one snapshot.
8.7.5. A template/render failure after a successful commit is still a 500 to the client, but the write stands. A retry must therefore be idempotent (§9.4); this is why cancel's second response is a safe 404 rather than a second delete.
8.7.6. A lost response after commit is resolved by reloading the target page. Start resumes, identify and register report `already_registered`, cancel reports 404, teacher edits show the saved values, and deletes report 404.
8.7.7. Client clock changes cannot extend a ticket; `iat` is checked server-side (PAR-020).
8.7.8. Rate-limit storage and keys are IDN's. PAR only declares the protected actions: code resolution, identify submit, register, cancel, start, and teacher management writes.
8.7.9. PAR never returns 401, 409, or 403 except the CSRF case. Missing scope, wrong role, wrong owner, deleted row, and foreign id are all 404.

## 9. Concurrency and idempotency

### 9.1 Lock protocol

9.1.1. Canonical lock order, highest rank first: (1) participant root — anonymous identity or student profile; (2) major; (3) quiz; (4) registration; (5) attempt, then answers, selections, and focus events. A transaction may skip ranks, but it must never take a lower rank before a higher one. Shared locks gate checks and hold the quiz row stable until commit; exclusive locks mark the row being mutated or deleted. PAR never mutates a quiz row, so no PAR command takes the quiz exclusively; close/publish is AUT's exclusive lock.
9.1.2. The per-command protocol:

| Command | Locks, in order | Serializes against |
|---|---|---|
| identify POST | identity X if cookie, major S, quiz S, registration insert | double identity, close, major delete |
| invite auto-register | student S, quiz S, registration insert | double registration, close |
| register POST | student S, quiz S, registration insert | double registration, close |
| cancel POST | quiz S, registration X | close, start, second cancel |
| attempts POST | student S, quiz S, registration X, then ATT's attempt rows | close, cancel, double start |
| teacher anonymous edit | identity X, major S | identify submit, second edit |
| teacher identity delete | identity X, registration X, attempt X, answer X, selection X, focus X | start/submit, second delete |
| major rename | major X | second rename |
| major delete | major X, then reference counts | any writer that holds major S |
| student profile edit (IDN) | student X, major S | start eligibility, major delete |
| quiz close/publish (AUT) | quiz X | register, cancel, start |

9.1.3. A cascade takes locks root to leaf in the same order and stamps one UTC timestamp. ATT's submit locks the registration before the attempt; no writer may lock an attempt before its registration. This is a requirement on ATT (PAR-204), not an option.
9.1.4. Deadlock or lock-wait timeout: roll back the whole transaction and retry once with a fresh transaction and re-read state. A second failure is 500. No external effect exists to duplicate.
9.1.5. A lock is never held across an HTTP write, log flush, mail, or file operation. Cookies and tickets are emitted only after commit (PAR-058).

### 9.2 Unique-key races

9.2.1. `uq_reg_student` / `uq_reg_anon`: the loser catches the duplicate, selects the winner's live row, and returns the idempotent outcome (`already_registered`, or the invite success path per PAR-039). If the winner was cancelled between insert and select, the loser retries the insert once; a second collision is 500.
9.2.2. `uq_anon_cookie`: the identity token is derived from the ticket nonce (PAR-019), so a double submit computes the same hash. The loser selects the live row and continues; it does not overwrite the winner's token, and it updates the profile to the submitted values (last writer wins) before ensuring the registration.
9.2.3. `uq_majors_name` / `uq_majors_code`: duplicate name is 422; duplicate generated code is regenerated once (PAR-111) and then 500. A concurrent create with the same name cannot produce two live rows.
9.2.4. `uq_attempt_number` is ATT's backstop. PAR's registration lock should make it unreachable; if it fires, ATT returns a conflict that PAR maps to 500, never to a silent second attempt.
9.2.5. Every unique key includes `deleted_at`, so a cancelled registration does not block a new one and a soft-deleted major name is reusable. Re-registration always creates a new row; no cancel is ever undone (INV-19, INV-29).
9.2.6. An invite auto-registration that loses the registration race still returns 303; the page shows one flash, never both `registered` and `already_registered`.

### 9.3 State races

9.3.1. Close vs register/cancel/start is resolved by the quiz lock (§8.3.2). The response always matches the committed state; a 303 `not-open` never accompanies a committed row.
9.3.2. Identity delete vs start: delete holds identity then registration; start holds quiz then waits for registration. If delete wins, start sees no live registration and returns 404 without an attempt. If start wins, delete waits, then cascades the fresh attempt. Neither leaves a live child under a deleted parent (INV-18).
9.3.3. Identity delete vs submit: submit must take the registration first and re-check liveness. If delete wins, submit is 404 and writes nothing; if submit wins, delete cascades the submitted attempt and its answers. An answer is never written after the root's delete timestamp.
9.3.4. Major delete vs a writer: the writer holds major S before inserting a student, identity, or quiz target; delete needs major X and therefore waits. After delete commits, no writer can obtain S, so the reference counts are stable. This is the only thing preventing a live child under a deleted major (PAR-122).
9.3.5. Student profile edit vs start: start holds student S. If the edit commits first, start sees the new grade/major and may refuse; if start commits first, the attempt is valid even if the profile changes a moment later (PAR-086). Eligibility is frozen at the start commit, not retroactively revoked.
9.3.6. Teacher edit vs identify submit: both take identity X, so the last committed writer wins all three fields. There is no field-level merge. The browser sees the teacher's edit or the participant's submitted values depending on order; both are authorized, and no attempt is touched.
9.3.7. Cancel vs register by the same person in two tabs: both take the quiz lock. If cancel commits first, register may insert a new row (quiz still open); if register commits first, cancel may immediately cancel it. Both outcomes are legal; the final state is whatever committed last.
9.3.8. Identity switch vs an in-flight identify form: the switch clears the browser cookie but not the ticket. Submitting the old form creates the new identity derived from the ticket; the old row is untouched. The form's hidden values do not select an identity.
9.3.9. Two teacher renames or two edits: last write wins. No version column exists and none is invented; the UI does not promise optimistic concurrency.
9.3.10. Two deletes of the same identity/major: the first commits; the second waits, then finds no live row and returns 404. There is no double cascade and no second timestamp.

### 9.4 Idempotency and retry semantics

9.4.1. Repeat outcomes:

| Request | Repeat outcome |
|---|---|
| GET join / identify form / intro / dashboard / history / lists | identical read, no write |
| GET invite | auto-registration is idempotent; 303 `already_registered` on repeat |
| GET identify `switch=1` | first clears cookie and flashes; repeat without cookie renders the plain form |
| POST join | same resolution; a fresh ticket may be minted, no registration |
| POST identify | same ticket → same identity and registration; 303 `already_registered` |
| POST register | 303 `already_registered`, one row |
| POST cancel | 404, no second delete |
| POST attempts | resume the open attempt, or start the next only if quota remains |
| POST teacher anonymous edit | 303 `saved`; same values are a no-op update |
| POST teacher anonymous delete | 404 |
| POST major create | 422 duplicate, never a second row |
| POST major rename | 303 `saved` when the name is unchanged |
| POST major delete | 404 |

9.4.2. There are no client-supplied idempotency keys. Identity is derived from the ticket, and uniqueness is enforced by the natural keys; this is sufficient because every effect is a single database transaction.
9.4.3. A retry after 429 is safe and keeps the ticket. A retry after 500 is safe because the transaction rolled back. A retry after 303 is a new request against the new state.
9.4.4. Exactly-once means exactly one committed database effect per logical command. No external side effect exists, so there is no outbox and no compensation.

### 9.5 Isolation and consistency

9.5.1. Every state decision uses a locking read (`FOR UPDATE`/`FOR SHARE`) in a READ COMMITTED transaction; a stale repeatable-read snapshot must never authorize a write.
9.5.2. Read models that render several aggregates (dashboard, history list, majors list) use one consistent snapshot, so counts, rows, and pagination agree (PAR-126, PAR-145).
9.5.3. No HTTP cache, ETag, or cross-request memoization is used for identity-bearing pages (§8.1.3). There are no denormalized counters to reconcile.
9.5.4. The join ticket and identity cookie are written only after commit; if the response is lost, the database and the browser converge on the retry (§9.4).

## 10. Cross-domain contracts

10.1. **PAR-197 — IDN supplies the actor.** IDN authenticates the session, enforces role, active status, `must_change_password`, CSRF, session expiry, login redirects, and rate limits. PAR requires the resolved student/teacher id and never re-parses a cookie for a session actor. PAR guarantees it never writes `accounts`, `students`, `sessions`, passwords, or reset tokens, and never reads a password hash.
10.2. **PAR-198 — Profile and major locks.** IDN's student create/edit and PAR's identify/edit take the major row shared before writing `major_id`, and IDN takes the student row exclusively. PAR's register/start take the student row shared before eligibility. Without this, PAR-122's major delete can race a writer and create a live child under a deleted major.
10.3. **PAR-199 — Login and identity.** Login and logout clear `quiz_participant` and `quiz_join_ticket` (PAR-020). Login never adopts, merges, or displays an anonymous identity. A student session outranks an identity cookie everywhere (§3.1). PAR never creates a session for an anonymous participant.
10.4. **PAR-200 — AUT owns quiz access facts.** PAR requires `state`, `visibility`, `target_grade`, `target_major_id`, `code`, `invite_token`, `max_attempts`, `allow_retake`, `show_score`, `title`, and the live question count to be readable and current. It trusts AUT to keep private `code` NULL, codes six digits and unique, invite tokens unique, targets both-or-neither, and `max_attempts ≥ 1`. PAR re-reads these under the quiz lock; it never writes them.
10.5. **PAR-201 — AUT lifecycle.** Opening requires at least one live question; closing is terminal and stamps `closed`; only drafts are deletable; a published title is locked. PAR maps those facts to `not_open`, `quiz-empty`, and 404. Rotating a code or token invalidates outstanding tickets (PAR-016); PAR does not rotate them.
10.6. **PAR-202 — ATT owns attempt start.** `POST /quiz/:id/attempts` locks the student profile shared when applicable, the quiz shared, and the registration exclusively, then calls ATT's start rule in the same transaction ([ATT: attempt start]). ATT inserts the attempt and answer slots, honors resume-before-quota, computes `attempt_no = 1 + live count`, and returns the id. A failure rolls back PAR's transaction; PAR never inserts an attempt or answer row (PAR-082).
10.7. **PAR-203 — ATT owns submit and liveness.** ATT's submit locks the registration before the attempt and re-checks that the registration and identity/student are live. After an identity or student cascade it returns 404 and writes no answer under a deleted parent. PAR's 404 for a missing registration is not sufficient on its own (PAR-107).
10.8. **PAR-204 — ATT owns scores.** `final_score` is 0–1, NULL while pending, and PAR only reads it. `show_score = false` hides every per-attempt and best score; waiting is still shown. PAR never computes, averages, rounds at storage, or writes a score (PAR-131, PAR-147, PAR-189).
10.9. **PAR-205 — ATT owns attempt-page authorization.** The attempt, result, and review pages must verify that the attempt belongs to the session student or the cookie identity. PAR's registration check at start does not authorize a later attempt id. Focus-loss counts are never shown on student surfaces (PAR-158).
10.10. **PAR-206 — OPS owns the sentinel and transactions.** Live means `deleted_at = '1970-01-01 00:00:00'`; every PAR read filters it. OPS provides the error taxonomy (404/403 CSRF/429/500), UTC timestamps, and the single-timestamp cascade. PAR's transactions are short and never span an external call.
10.11. **PAR-207 — OPS owns cascade provenance.** Deleting an anonymous identity or student cascades registrations, attempts, answers, selections, and focus events in one transaction, sets `deleted_by_account_id` only on the root, and never hard-deletes or restores (INV-18, INV-29).
10.12. **PAR-208 — Shared read surfaces.** Teacher dashboard, results, monitor, SSE `ParticipantRow`, and recap read PAR's live registrations; their counts are distinct live identities/registrations, never cached across requests. They must not expose anonymous cookie hashes or tokens, and they must degrade when a row is deleted mid-stream by re-reading or by ending the stream (OPS/AUT).
10.13. PAR guarantees no schema delta is required to implement any rule above. It requires the sibling domains not to add a denormalized participant name or major label that would break PAR-INV-10.

## 11. Traceability

11.1. Every PAR route in C-§6 is owned here. Sibling ATT rules are cited by name because their final numbers are not fixed in this document set; the contract's route ownership is unchanged.

| Route | Command rules | Invariants | Acceptance |
|---|---|---|---|
| `GET /join` | PAR-021–026 | C-§3.5 (read-only) | AC-06 |
| `POST /join` | PAR-027–035 | PAR-INV-09, INV-07 | AC-07–10 |
| `GET /invite/:token` | PAR-036–044 | PAR-INV-02, PAR-INV-09 | AC-11–15 |
| `GET /join/identify` | PAR-045–052 | PAR-INV-09, PAR-INV-13 | AC-16–17 |
| `POST /join/identify` | PAR-053–060 | PAR-INV-01, PAR-INV-02, PAR-INV-09 | AC-18–22 |
| `GET /quiz/:id` | PAR-087–095 | PAR-INV-06, PAR-INV-11, PAR-INV-12 | AC-33–36 |
| `POST /quiz/:id/register` | PAR-061–069 | PAR-INV-02, PAR-INV-06 | AC-23–25 |
| `POST /quiz/:id/cancel` | PAR-070–077 | PAR-INV-05, PAR-INV-12 | AC-26–28 |
| `POST /quiz/:id/attempts` | PAR-078–086 | PAR-INV-04, PAR-INV-06, PAR-INV-12 | AC-29–32 |
| `GET /student` | PAR-126–141 | PAR-INV-03, PAR-INV-07, PAR-INV-11 | AC-37–41 |
| `GET /student/history` | PAR-142–153 | PAR-INV-03, PAR-INV-07, PAR-INV-11 | AC-42–44 |
| `GET /student/history/:quizID` | PAR-154–162 | PAR-INV-03, PAR-INV-07, PAR-INV-11 | AC-45–47 |
| `GET /teacher/anonymous` | PAR-163–172 | PAR-INV-01 | AC-48–49 |
| `GET /teacher/anonymous/:id` | PAR-173–178 | C-§3.4 (school-wide) | AC-50 |
| `POST /teacher/anonymous/:id` | PAR-096–101 | PAR-INV-10, PAR-INV-14 | AC-51 |
| `POST /teacher/anonymous/:id/delete` | PAR-102–108 | PAR-INV-14, INV-18, INV-29 | AC-52–53 |
| `GET /teacher/majors` | PAR-179–188 | PAR-INV-08, PAR-INV-10 | AC-54 |
| `POST /teacher/majors` | PAR-109–114 | INV-19 | AC-55 |
| `POST /teacher/majors/:id` | PAR-115–119 | PAR-INV-10 | AC-56 |
| `POST /teacher/majors/:id/delete` | PAR-120–125 | PAR-INV-08 | AC-57–58 |

11.2. Cross-cutting rules and their acceptance: §3.5 → AC-60; §5.0 copy/status → AC-60; §8.3 close gate → AC-59; §9 lock protocol → AC-32, AC-53, AC-58; §10 IDN → AC-22, AC-60; §10 AUT → AC-21, AC-33, AC-59; §10 ATT → AC-31, AC-46; §10 OPS → AC-52, AC-60.

## 12. Open questions and schema deltas

12.1. Every question has a recommended default; none blocks implementation.

| ID | Question | Recommendation | Impact |
|---|---|---|---|
| OQ-01 | The contract has no identity-switch affordance, but a shared browser otherwise forces cookie clearing by hand. | Keep `GET /join/identify?quiz=&switch=1` (PAR-052): cookie-only, no row write, no CSRF, one info flash. | Adds a route variant and UI link; no schema. |
| OQ-02 | A 365-day identity cookie is long for a shared device and enables cap bypass by clearing it. | Keep 365 days and the switch link; do not fingerprint. Accept R-02's bypass (PAR-012). | Privacy notice and fairness expectations; no code change. |
| OQ-03 | Should the identity token be an HMAC envelope rather than an opaque hash? | No. A 256-bit bearer token plus exact SHA-256 lookup is sufficient; a signature adds key-rotation failure without entropy gain (PAR-003). | None now; revisit only if tokens need embedded claims. |
| OQ-04 | A direct private-quiz URL by an ineligible student returns 404, while the invite landing explains `wrong_audience`. | Keep 404: no probing of private audiences. The invite is the only explanatory surface (PAR-062). | Support burden when a student's major changed; document in help copy. |
| OQ-05 | After a profile change, an existing registration cannot start a new attempt; after close, a zero-attempt registration can never be cancelled. | Keep the literal R-10/R-12 reading: existing attempts survive, new starts require current eligibility, and dead closed rows stay (PAR-086, §8.3). | Visible unfairness; changing it needs a contract ruling, not a PAR patch. |
| OQ-06 | F-§9.5 implies a private code can yield `anonymous_not_allowed`; the schema stores `code IS NULL` for private quizzes. | Schema wins: private codes do not exist; any private code input is `unknown_code` (PAR-030, §4.3.1). | Update F-§9.5 and fixtures; no private-code tests. |
| OQ-07 | `GET /invite/:token` auto-registers, so a prefetcher or chat preview can create a registration. | Accept: the effect is one idempotent registration, no attempt, and the contract requires the GET side effect (PAR-043). | Inflated registration counts; no data loss. |
| OQ-08 | The current views diverge: anonymous intro uses the student layout and account links; `HistoryRow` lacks `MaxAttempts`; history detail renders focus counts; the anonymous edit fixture uses a registration id; the majors list lacks an anonymous-reference count. | Fix the view layer, not the routes: participant layout, `HistoryRow.MaxAttempts`, hide focus, pass participant id, add the reference count to `CanDelete`'s reason. | Template/view-model/fixture/test changes; AC-35, AC-42, AC-46, AC-50, AC-54. |
| OQ-09 | Registration cancellation has no actor column; only `deleted_at` records it. | Accept for now. Add `registrations.deleted_by_account_id` only if an audit requirement appears. | Accountability gap for student self-cancel; no blocking delta. |
| OQ-10 | Renaming a major or an anonymous participant retroactively changes historical labels. | Accept R-25/no-snapshot: current names everywhere, including past results (PAR-100, PAR-117). | Transcripts and exports change after rename; disclose to teachers. |
| OQ-11 | `majors.code` is NOT NULL and unique but the create form has only `name`; the list also lacks an anonymous-reference count. | Generate the code from the name (PAR-111) and add the anonymous count to the delete guard/disabled reason. | Collision handling and a small view-model addition; no schema. |
| OQ-12 | A major targeted by a closed quiz can never be deleted, because closed quizzes are never deletable and the target is permanent. | Keep R-13/R-10: no force delete. If operations need cleanup, add an explicit `archived` flag later; do not cascade targets. | Majors accumulate; a future migration may be wanted. |

12.2. Schema deltas. No delta is required for PAR-001–PAR-208. Optional, non-blocking deltas: (a) `registrations.deleted_by_account_id` for OQ-09; (b) a `majors.archived_at` flag for OQ-12; (c) a per-identity merge pointer for shared devices, which is explicitly not recommended because R-02 says the cookie, not the person, is the identity. View-model deltas are listed in OQ-08 and are not schema changes.
12.3. Recorded contract conflicts: F-§10.4's "also clears the identity cookie binding for that quiz" is superseded by PAR-010 (no per-quiz binding exists); F-§9.5's private-code `anonymous_not_allowed` is superseded by OQ-06; the frontend's anonymous history/chrome assumptions are view gaps under OQ-08.

## 13. Acceptance criteria

Each criterion is executable against the HTTP surface and the database. "No write" means no row insert/update and no cookie change other than the stated one.

- **AC-01** A set identity cookie matches `^v1\.[A-Za-z0-9_-]{43}$`, carries `HttpOnly`, `SameSite=Lax`, `Path=/`, `Max-Age=31536000`, `Secure` iff configured, no `Domain`, and its SHA-256 selects exactly one live `anonymous_participants` row.
- **AC-02** Any of {missing, `v1.`, 42 chars, 44 chars, non-base64url, unknown hash, deleted row} renders the join page as a visitor with 200, no 500, and clears the cookie when one was sent.
- **AC-03** A failed identify validation creates zero identity and zero registration rows and sets no identity cookie; a successful one creates both in one transaction and sets the cookie only after commit.
- **AC-04** `last_seen_at` changes on identify, registration reuse, and attempt start, and does not change on GET join, GET identify, GET intro, dashboard, history, or teacher reads.
- **AC-05** Cancelling an anonymous registration sets only that registration's `deleted_at`; the identity row, cookie, and other registrations are unchanged. Deleting an identity does not emit a `Set-Cookie` for the browser but its next request is a visitor.
- **AC-06** `GET /join` with `error=unknown_code`, `error=bogus`, `flash=x`, or `page=99` returns 200, renders no quiz data, and performs no insert/update.
- **AC-07** `POST /join` with `""`, `12345`, `1234567`, `12a456`, ` 123456 `, or Arabic-Indic digits redirects 303 to `/join?error=unknown_code` and never 422.
- **AC-08** A code for a draft/closed quiz yields `not_open`; a code for a public open quiz yields the intro (student) or identify ticket (visitor); a corrupted private row with a code yields `unknown_code`; the code never appears in `Location`.
- **AC-09** A student's valid code does not create a registration; with an existing registration the response is 303 intro with `already_registered`, otherwise 303 intro with **Daftar**.
- **AC-10** A visitor's valid code returns 303 `/join/identify?quiz=:id`, sets a signed ticket, and creates zero identities and zero registrations.
- **AC-11** An unknown, empty, 33-character, or illegal-character invite token returns 200 with `unknown_code`, never 404 and never a quiz title.
- **AC-12** An invite to a closed quiz with a live registration returns 303 intro `already_registered` and changes nothing; without a registration it returns 200 `not_open`.
- **AC-13** An invite to an eligible student on an open quiz creates one registration and returns 303 `registered`; two concurrent GETs create one row and both return 303 with one flash.
- **AC-14** A private invite returns `wrong_audience` for an ineligible student and `anonymous_not_allowed` for a visitor, with no row; an eligible student is auto-registered.
- **AC-15** An owner teacher's invite returns 303 to the authoring detail; a non-owner teacher's invite and an unknown token for a teacher return 404; no registration is created.
- **AC-16** `GET /join/identify` without a ticket is 404; with a valid ticket it renders 200 and prefills a live identity; a student is redirected to the intro; a missing/mismatched `quiz` canonicalizes with 303.
- **AC-17** `switch=1` with a valid ticket and live identity returns 303 to the canonical form, expires only `quiz_participant`, flashes `identity-switched`, and executes no identity update/delete; repeating without a cookie renders the plain form with no flash.
- **AC-18** Invalid name/grade/major on identify returns 422 with the exact §5.0.3 messages, preserves the submitted values, keeps the ticket, and writes nothing.
- **AC-19** A valid identify creates one identity and one live registration with the sentinel `deleted_at`, sets `last_seen_at`, returns 303 `registered`, sets the cookie and clears the ticket; it creates no attempt.
- **AC-20** Two concurrent identical identify POSTs yield one identity, one registration, two 303 responses, and no 500; the loser reports `already_registered`.
- **AC-21** Identify on a closed quiz with no registration returns 303 `/join?error=not_open` and clears the ticket; with a registration it refreshes the profile and lands on the intro; with zero live questions it returns `quiz-empty` and creates no registration.
- **AC-22** A student POSTing identify is redirected to the intro and creates no anonymous row; a teacher POSTing it gets 404.
- **AC-23** A visitor is redirected to login; an anonymous identity or teacher gets 404; a draft/closed quiz and an ineligible private quiz get 404; no registration is created.
- **AC-24** A valid student register returns 303 `registered` and one row; a repeat or double click returns 303 `already_registered` with the same one row.
- **AC-25** Registering for an open quiz with zero live questions returns 303 `quiz-empty` and creates no registration.
- **AC-26** Cancel with zero attempts on an open quiz returns 303 `cancelled` and sets `deleted_at`; with any live attempt it returns `cancel-blocked` and leaves the row; on a closed quiz it returns `not-open` and leaves the row.
- **AC-27** Anonymous cancel returns 303 `/join` `cancelled`; the identity row, cookie, and other registrations are unchanged; the intro then 404s.
- **AC-28** A second cancel returns 404, does not change `deleted_at`, and cannot restore the row.
- **AC-29** Start with a live in-progress attempt returns 303 to that attempt even when the quiz is closed, the student is now ineligible, or the quota is exhausted; no new attempt is created.
- **AC-30** Start on a closed quiz returns `not-open`; draft returns 404; count ≥ normalized max or `allow_retake=false` with count > 0 returns `attempts-exhausted`; no registration returns `not-registered` (student) or 404 (anonymous); `max_attempts=0` behaves as 1.
- **AC-31** A first start creates exactly one attempt with `attempt_no=1` and returns 303 `/attempt/:id` `started`; SQL tracing shows PAR issued no insert into `quiz_attempts` or `attempt_answers`.
- **AC-32** Two concurrent starts on one registration yield one attempt id for the second while the first is open; after the first is submitted the second creates `attempt_no=2` only if quota allows, otherwise `attempts-exhausted`, with no duplicate number.
- **AC-33** The intro returns 200/404/303 exactly per the §4.4 direct matrix for student, anonymous, visitor, and teacher viewers, and never leaks another participant's data.
- **AC-34** Intro actions match PAR-092; closed copy distinguishes resumable from closed; hidden scores render `—`; resume takes precedence over register/start/retry.
- **AC-35** An anonymous intro contains no Dasbor, Riwayat, Profil, or logout link, shows the participant's name/class, a switch link, and a back link to `/join`.
- **AC-36** Repeating an intro GET performs no insert/update and no cookie write except clearing a corrupt identity cookie.
- **AC-37** Dashboard Assigned counts the union of open eligible quizzes and all live registrations; Registered counts only open zero-attempt registrations; InProgress/Completed include closed quizzes; a closed zero-attempt row is Assigned but not Registered.
- **AC-38** Best score is the maximum non-null visible `final_score`; hidden or all-NULL is `—`; a visible zero is `0%`; no average or latest-score substitution occurs.
- **AC-39** Ready, Live, Latest, and Recent lists follow PAR-132–135 sort, tie-break, caps, and action targets; Latest is capped at 6 and Recent at 5.
- **AC-40** The dashboard empty state appears only when all four lists and all four counts are empty; non-zero counts with empty lists show stats and no empty state.
- **AC-41** Dashboard data comes from one snapshot; no other student, answer body, key, teacher-only count, or focus total is rendered; a DB failure is 500 with no partial page.
- **AC-42** History has one row per live registration including closed and zero-attempt rows, sorted by last activity descending then registration id ascending, with `N dari M percobaan` and attempt links per PAR-142–149.
- **AC-43** History page size is 10; `page=0`, `-1`, or `abc` renders page 1; a page beyond the end clamps to the last page; the total equals live registrations and pagination uses one query.
- **AC-44** Hidden scores render `—`, pending attempts render "Menunggu penilaian", and a student with zero live registrations sees the exact empty state and join action.
- **AC-45** A foreign, missing, or soft-deleted `:quizID` returns 404; the student's own live registration renders 200 and no other student's attempt can be reached.
- **AC-46** Detail attempts are ordered by `attempt_no` ascending with result/resume links; retry follows PAR-157; focus counts are absent; no pagination is offered.
- **AC-47** A zero-attempt closed registration shows the closed explanation and no start action; an open one shows **Buka halaman kuis**.
- **AC-48** Two identities with the same name render as two rows; the list is never grouped or merged; each row shows its own distinct-quiz count and last activity.
- **AC-49** `q` filters by case-insensitive substring and is capped at 100 runes; sort and page follow PAR-165–171; an empty result from a search shows the filtered-empty copy, and a truly empty list shows the create-oriented empty state.
- **AC-50** `GET /teacher/anonymous/:id` prefills the live identity and its current major; an unknown/soft-deleted id returns 404; the edit form contains no cookie, token, or registration field.
- **AC-51** Editing with invalid values returns 422 and writes nothing; a valid edit returns 303 `saved`, changes only name/grade/major/`updated_at`, leaves the cookie and `last_seen_at` untouched, and the new label appears on every read surface; a repeat is a no-op success.
- **AC-52** Deleting an identity cascades registration, attempts, answers, selections, and focus rows in one transaction; only the identity root has `deleted_by_account_id`; the list returns 303 `deleted`; the browser's next request is a visitor.
- **AC-53** An identity delete racing a submit yields either a completed submit followed by a full cascade, or a 404 submit; no live answer exists under a deleted identity.
- **AC-54** The majors list shows live rows with usage counts, hides `code`, disables delete with "Masih dipakai." for any live student/anonymous/quiz reference, and shows the exact empty state when none exist.
- **AC-55** Creating a major with blank/short/long/control/duplicate names returns 422 with the exact field errors and no row; a valid create makes a unique generated-code row and returns 303 `saved`; a duplicate-generated code is retried, not 500.
- **AC-56** Renaming to an existing name returns 422 duplicate; a valid rename changes only `name`/`updated_at`, keeps `code`, updates every class/audience label immediately, and is idempotent for the same name.
- **AC-57** Deleting an unreferenced major sets `deleted_at` and returns 303 `deleted`; a major referenced by any live student, identity, or quiz (draft or closed) returns 303 `major-in-use` and changes nothing; a second delete is 404.
- **AC-58** A major delete racing an identify/edit/quiz-target writer either deletes before the writer takes its shared lock or waits; no live child is ever left under a deleted major.
- **AC-59** After close, register/cancel/start are refused per INV-12 while resume, intro, and history reads still work; no transaction can commit a new registration or start after the close commits.
- **AC-60** PAR returns 403 only for CSRF failure, 404 for every scope/role/owner miss, 429 with `Retry-After` for rate limits, 500 with rollback on DB failure, and never 401 or 409.
- **AC-61** Nil scores/dates render `—`, zero scores render `0%`, zero counts render `0`, class labels use roman grade + current major, dates render WIB, and score rounding is half-up at the model boundary.
