# Integrity and Operations — Business Logic Specification

**Date:** 2026-10-23
**Status:** binding cross-cutting spec; implements contract rulings R-01–R-25 without amendment
**Scope:** invariants and their enforcement layer, state-transition legality, soft-delete
cascades, transaction boundaries, concurrency and idempotency, time and retention, error
and status taxonomy, route coverage, operational prerequisites, and the schema-delta
register.
**Lens:** systems, concurrency, and data lifecycle. This is not a fifth command spec.
**Sources:** `2026-10-23-business-logic-00-contract.md` (binding); the eight
`migrations/*.up.sql`; all fifteen `internal/model` structs; the schema design §5–§10; the
frontend data contract §2, §17, §21–§24; the old design §11 (superseded context only),
§14, §16; `AGENTS.md`; `docker-compose.yml`; `internal/config`, `internal/store`,
`internal/httpserver` (route registry, views, conformance and partials tests).
**Rule prefix:** `OPS-###`. Domain invariants use `OPS-INV-##` so they cannot collide with
a sibling's added `INV-31+`. Command rules remain owned by `IDN`, `AUT`, `PAR`, `ATT`.

**Shape.** Contract §7 is written for a command-owning domain. OPS owns no business command,
so its command/query chapters are replaced by cross-cutting matrices: §3 states, §4
invariants, §5 cascades, §6 transaction boundaries (with §6.1 scoring), §7 concurrency, §8
time and retention, §9 error taxonomy, §10 route coverage, §11 operations. §12–§15 are
contract §7 items 10–13 in order. No item is omitted; the command layer lives in the four
sibling specs and is cited, never restated.

---

## 1. Intent and boundaries

**OPS-001** This document makes the seams between `IDN`, `AUT`, `PAR`, and `ATT`
auditable. It states one enforcement layer per invariant, one lock order per mutation, one
timestamp per event, one status per failure, and one owner per route. It adds no product
behaviour beyond what the contract already requires.

**OPS-002** Authority is contract §1 unchanged: migrations and models win on data shape;
the frontend contract wins on screens and product decisions; the old design supplies only
unsuperseded rules; `docs/DESIGN.md` never supplies business logic. This document may not
weaken a higher source. Where it interprets a binding ruling, the interpretation is named
as such in §14.

**OPS-003** OPS owns: enforcement-layer assignment, transaction and lock protocol, cascade
stamping and file cleanup, idempotency and race outcomes, the clock and expiry semantics,
HTTP status and PRG mechanics, the route-coverage audit, and deployment prerequisites.

**OPS-004** OPS does not own: credentials, role rules, approval copy, quiz authoring
fields, join/eligibility copy, scoring formulas as product rules, review-mode rendering,
or any command's user-visible workflow. Those belong to `IDN`, `AUT`, `PAR`, and `ATT`.
Citations below name the owning prefix and the route, never a redefined command.

**OPS-005** No rule here requires a schema change. Gaps are collected in §14 and are either
resolved without DDL or marked as a config/interface delta. OPS never edits `migrations/`,
`internal/model/`, `schema_migrations`, or the migration runner (R-01).

**OPS-006** The application is one process (old design §14; schema §2). Rate limiting,
flash storage, and the SSE hub are in-process. Horizontal scaling is out of scope; §11
states the consequences. The database is the only shared state.

**OPS-007** Vocabulary used normatively below:

| Term | Meaning |
|---|---|
| **live** | `deleted_at = '1970-01-01 00:00:00'` on the row and on every ancestor the rule names |
| **T** | the single event timestamp of one transaction, read once from the database clock |
| **actor** | the authenticated account performing a human action |
| **system** | the application acting without a human (`deleted_by_account_id IS NULL`) |
| **aggregate root** | the row whose `FOR UPDATE` lock serialises a mutation and its children |
| **owning domain** | the spec named by contract §6 for a route |

**OPS-008** Every write path is classified by exactly one enforcement layer: **DB** (a
constraint rejects the state), **TX** (a transaction plus lock plus assertion rejects it),
or **APP** (a pure validation/GUARD in Go before or inside the transaction). “App” is not
a synonym for “best effort”: every APP rule names its acceptance test in §15.

**OPS-009** A failure that violates an invariant is never rendered as a user mistake. It
aborts the transaction, logs an integrity fault with IDs only, and returns the 500 page
(R-19). User-correctable conflicts return 422 or a 303 with an error flash.

---

## 2. Actors, authority, and entry conditions

**OPS-010** The closed actor set:

| Actor | Entry condition | May reach |
|---|---|---|
| visitor | no valid session | `/`, `/login`, `/register/*`, `/reset-password`, `/join*`, `/invite/:token`, `/auth/pending`, `/error/*`, `/healthz` |
| anonymous participant | signed identity cookie whose hash matches a live row | public-quiz join, intro, attempt, beacon, result; never `/teacher/*` or `/profile` |
| student | live `role='student'`, `status='active'`, live profile, live session | `/student*`, `/profile`, `/quiz/*` for eligible quizzes, `/attempt/*` they own |
| teacher (pending) | cannot authenticate (R-03); pending token only | `/auth/pending` |
| teacher (active) | live `role='teacher'`, `status='active'`, live session | `/teacher*` scoped to ownership or school-wide management |
| teacher (`must_change_password=1`) | live session | `GET/POST /change-password`, `POST /logout`, every other authenticated route redirects to `/change-password` |
| system | a transaction started by a request or an expiry sweep | session expiry, cascade stamping, clock reads |
| operator | shell/runner, never HTTP | migrations, bootstrap teacher, backups |

**OPS-011** Status policy is fixed:

| Condition | Status | Body / target |
|---|---|---|
| missing, soft-deleted, wrong owner, wrong role, cross-tenant ID | 404 | 404 page; never 403 |
| CSRF missing/mismatched/replayed | 403 | error layout, exact R-19 `csrf` copy |
| validation failure | 422 | same template, submitted non-password values, `FieldErrors` |
| rate limit exceeded | 429 | exact R-19 `rate limited` copy, `Retry-After` seconds |
| database or invariant failure | 500 | 500 page; no SQL text, no stack |
| successful POST or state conflict | 303 | PRG target, optional one-shot flash |
| expired/missing session on an authenticated route | 303 | `/login` with R-19 `session expired` flash |
| `must_change_password=1` on any other authenticated route | 303 | `/change-password` with R-19 `must change password` flash |
| beacon accepted | 204 | empty body |
| upload not modified | 304 | empty body |

**OPS-012** Middleware order is normative because it decides which status wins: recover →
request ID/access log → security headers → per-IP flood limit → session resolution →
route rate limit → CSRF (all POST except `/attempt/:id/beacon`) → `must_change_password`
gate → authorization → handler. A request over the route limit gets 429 even if its CSRF
token is also bad; a request under the limit with a bad token gets 403 before any
authorization or write.

**OPS-013** Middleware identity is a hint, not a write authorization. Every mutating
transaction re-reads the actor and the aggregate root with a locking/current read and
re-validates liveness, role, ownership, and state before writing (`OPS-INV-04`). A session
revoked or an account deleted between middleware and handler must fail the write.

**OPS-014** Ownership scoping happens in the SQL predicate, not after the fetch:
`quizzes.owner_account_id = :actor` for every `AUT` and `ATT` teacher route. A row fetched
first and rejected in Go is a bug because it creates a timing/ID oracle.

**OPS-015** `system` writes are limited to: soft-deleting expired sessions during the
sweep, stamping cascade descendants, and time reads. No system path creates, grades,
approves, publishes, or restores.

**OPS-016** The operator is outside the HTTP surface. There is no admin route, no restore
route, no purge route, no hard-delete route (INV-29). Operator actions require shell
access and are logged outside the app.

---

## 3. State-transition legality

**OPS-020** **Account.** Student: `(absent) → active` at registration; `active → soft-deleted`
by a teacher. Teacher: `(absent) → pending` at registration; `pending → active` by approval;
`active` is terminal; teacher soft delete is **forbidden** (R-05, `OPS-055`). `must_change_password`
`1 → 0` only by a successful password change. Forbidden: pending → deleted, active → pending,
student → teacher or teacher → student, any status change without the matching
`approved_at` (DB `chk_accounts_teacher_approval`). Owner: `IDN`.

**OPS-021** **Quiz.** `draft → open` on publish (owner; ≥1 live valid question; private
target pair present); `open → closed` on close (owner); `draft → soft-deleted` on delete
(owner; zero live registrations and zero live attempts, `OPS-121`); `closed` is terminal.
Forbidden: `open → draft`, `closed → open`, `open|closed → deleted`, visibility change after
creation, settings/question writes after publish. `published_at` is non-NULL for open and
closed; `closed_at` is non-NULL iff closed (INV-09). Owner: `AUT`.

**OPS-022** **Question and option.** Question: `(absent) → live` in a draft; `live → live`
edits in a draft; `live → soft-deleted` in a draft; position changes only through reorder or
delete-renumber. Option: created/updated/soft-deleted only with its question's editor; no
independent lifecycle and no route. Forbidden: any write when the quiz is not draft (INV-10);
a live question with zero/one option for objective types or with an invalid key (INV-11).
Owner: `AUT`.

**OPS-023** **Registration.** `(absent) → live` by register, code join, invite auto-register,
or anonymous identify+register; `live → soft-deleted` by cancel only while the quiz is open
and no live attempt exists; re-registering creates a new row. Derived participation state
(never stored): `registered` = no live attempts; `in_progress` = ≥1 live attempt with
`submitted_at IS NULL`; `completed` = ≥1 live attempt and none in progress (R-06). Forbidden:
two live registrations for one participant+quiz (INV-06); cancel after the first attempt;
cancel while closed; registration on a draft or closed quiz. Owner: `PAR`.

**OPS-024** **Attempt.** `(absent) → in_progress` at start; `in_progress → submitted` at
submit; `submitted` is terminal. Graded is derived: `final_score IS NOT NULL`, not a state
transition. Soft delete only by an ancestor cascade; no route deletes an attempt. Forbidden:
submit twice as two writes (second is idempotent), answer write after submit, two in-progress
attempts per registration, attempt number outside 1..255, start on a non-open quiz. Owner:
`ATT` for the rules, `PAR` for the hand-off route `POST /quiz/:id/attempts`.

**OPS-025** **Answer slot.** `(absent) → empty` at attempt start (one row per live question);
`empty → answered` by save; `answered → answered` by later save; `→ scored` at submit for
objective and at grade for essay; `scored` essay is write-once (`OPS-142`). After submit the
slot body and selections are frozen; only the essay score/feedback may be written by the
grading route. Forbidden: a slot for a question outside the attempt's quiz; a selection of an
option outside the slot's question; an objective score other than 0/1; a score without
`graded_at` (DB `chk_answers_grading`). Owner: `ATT`.

**OPS-026** **Session.** `(absent) → active` on login; `active → active` with a later
`expires_at` on authenticated use; `active → revoked` on logout, password reset, or
change-password-other-sessions; `active → expired` when `expires_at <= now`; revoked and
expired are terminal. Login rotates by inserting a new row and soft-deleting the old one
when a session already exists. Forbidden: authenticating an expired, revoked, or
account-deleted session; reusing a rotated token. Owner: `IDN`.

**OPS-027** **Password reset token.** `(absent) → issued` by a teacher; `issued → used` at
consume; `issued → expired` when `expires_at <= now`; `issued → revoked` when the student is
deleted or a later token for the same student is issued (`OPS-108`). Used/expired/revoked are
terminal; no reinstate. Owner: `IDN`; consume mechanics are `OPS-109`.

**OPS-028** **Major.** `(absent) → live`; `live → live` rename (single row, no propagation);
`live → soft-deleted` only while no live student, anonymous participant, or quiz target
references it. `soft-deleted` is terminal; no restore. Forbidden: delete while referenced, even
by one live child (INV-23); cascade into children. Owner: `PAR`.

**OPS-029** **Anonymous participant.** `(absent) → live` on first identify; `live → live` with
refreshed `display_name`, `grade_level`, `major_id`, `last_seen_at` on each join; `live →
soft-deleted` by a teacher; re-joining after deletion creates a **new** row because the cookie
hash is freed (R-02). Forbidden: name-based lookup or merge; deleting registrations on other
quizzes when one registration is cancelled. Owner: `PAR`.

**OPS-030** **Pending-teacher token.** A signed, stateless token: `waiting` while the account
is pending and `issued_at + 24h > now`; `approved` while the account is active; `expired`
after 24h; unknown/soft-deleted account renders the generic invalid state. Viewing never
consumes it and never mutates the account. Owner: `IDN`.

**OPS-031** **SSE subscription.** `(absent) → connected` only for the owner teacher while the
quiz is open; `connected → closed` on quiz close (`quiz.closed`), client disconnect, slow-client
eviction, or process shutdown. No replay buffer; a reconnect gets a fresh `snapshot`. Forbidden:
connecting to a draft/closed quiz, a non-owned quiz, or a student/anonymous session. Owner:
`ATT`; hub lifecycle is `OPS-235`.

**OPS-032** **Terminal states.** `closed` quiz, soft-deleted row, used/expired reset token,
revoked session, submitted attempt (body frozen), and deleted teacher (never reached) have no
outgoing transition. Any route that would produce one returns 404 (missing/deleted/cross-tenant)
or 303 with a state-conflict flash (legal resource, illegal transition).

**OPS-033** **Transition guards are transactional.** Every arrow in OPS-020–031 is checked
against a **current** read of the row inside the transaction that performs the write, under
the aggregate lock named in §6. A guard evaluated only on a prior GET is stale and must not
authorize the POST.

---

## 4. Invariant enforcement matrix

**OPS-040** The matrix is the audit of contract §4. “DB” means a declared constraint/index
rejects the state on any writer, including a future bug. “TX” means one transaction, one
timestamp, and a locking assertion. “APP” means a Go guard whose bypass is still caught by
one of the other two wherever the schema allows it. A blank bypass failure is not allowed.

| ID | Invariant | Primary enforcement | Secondary | If bypassed | Verification |
|---|---|---|---|---|---|
| INV-01 | Live email unique, case-insensitive | DB `uq_accounts_email` (`ascii_general_ci`) | APP trim+lowercase | Two accounts share a login/reset identity | Two concurrent registrations, same email, mixed case → one 303, one 422 `Email sudah terdaftar.` (A-15) |
| INV-02 | Profile only for a student, one live per account | DB `uq_students_account` + FK | APP role check at registration | Teacher gains a grade/major; student invisible to roster | Insert profile for teacher → app 404/500 guard; duplicate profile → 1062 (A-15) |
| INV-03 | Actor FKs are role-typed | APP (DB cannot assert role) | TX re-read of actor | Student approves/grades/deletes; teacher owns student data | Cross-role probes on every actor column (§15 A-37) |
| INV-04 | Students active; teachers pending until approved; must-change blocks | DB `chk_accounts_student_active`, `chk_accounts_teacher_approval` | APP gate + middleware | Pending teacher authenticates; approval without timestamp | Direct insert of invalid status fails; login pending → 403-style message (A-16, A-18) |
| INV-05 | One participant, student XOR anonymous | DB `chk_reg_participant` | APP constructor writes one column | Registration with two/zero participants | Direct insert both/NULL fails (A-28) |
| INV-06 | ≤1 live registration per (quiz, participant) | DB `uq_reg_student`, `uq_reg_anon` | TX idempotent upsert | Duplicate roster/history/attempt quota | Concurrent register ×2 → one row; second lands intro with info flash (A-19, A-46) |
| INV-07 | Public ⇒ no targets + code; private ⇒ both + no code | DB `chk_quizzes_target`, `chk_quizzes_code_visibility` | APP create/save | Public quiz silently restricted; private quiz joinable by code | Direct insert contradictions fail; visibility not editable after create (A-23, A-43) |
| INV-08 | Code 6 digits unique live; invite token non-empty unique live | DB `chk_quizzes_code`, `uq_quizzes_code`, `uq_quizzes_invite` | APP crypto generation + bounded retry | Ambiguous join; two quizzes share an invite | Collision injection at create → retry, then 500 after 10 attempts (A-43) |
| INV-09 | draft→open→closed only; `published_at`/`closed_at` coherence | DB `chk_quizzes_lifecycle` | APP transition guard | Timeless open quiz; reopened closed quiz | Direct SQL to illegal states fails; POST reopen → 404/303 conflict (A-23) |
| INV-10 | Editable only draft; positions dense 1..N unique | APP state guard; DB `uq_questions_position`, `uq_options_position` | TX quiz lock + two-phase renumber | Open-quiz edits change live attempts; duplicate/gapped order | Edit open quiz → 404/conflict; delete question then assert positions 1..N (A-24, A-27) |
| INV-11 | single=1 correct, multiple≥2, essay=0; objective≥2 options | APP publish/editor validation | TX publish check | Unanswerable/unscorable question goes live | Publish invalid fixture → 422, no state change (A-26) |
| INV-12 | Question∈quiz, option∈question, answer∈attempt quiz | APP (FKs are parent-only) | TX ownership joins | Cross-quiz data leak/corrupt scoring | Crafted IDs from another quiz → 404, no write (A-37) |
| INV-13 | ≤1 in-progress attempt; `attempt_no` sequential from 1 | DB `uq_attempt_number` + APP count | TX registration lock | Double attempts, quota bypass, wrong attempt number | Concurrent start ×2 → same attempt ID; attempt_no 1,2,3… (A-20) |
| INV-14 | Attempt∈registration, slot∈attempt, selection∈slot+same question | APP (FKs are parent-only) | TX locks + option join | Selection of another question's option scores wrong | Crafted option ID → 422/404, slot unchanged (A-19, A-37) |
| INV-15 | Final score = mean of per-question scores; NULL until all scored; 4dp | APP one scoring function (`OPS-132`) | DB `chk_attempt_score` | Wrong/early final score; hidden divergence between screens | Score vectors A-21/A-22; recompute from answers equals stored |
| INV-16 | Objective exactly 0/1; essay/final 0–1 | DB `chk_answers_score`, `chk_attempt_score` | APP conversion (`OPS-131`) | Out-of-range score | Insert 1.0001 fails; input 100.1 → 422 (A-21, A-22) |
| INV-17 | Scores visible only per `show_score`; hidden ≠ 0 | APP render contract (ATT) | Shared score formatter | Hidden score leaks as `0%` | Review matrix A-17; hidden renders `—` |
| INV-18 | No live child under a soft-deleted parent; one cascade timestamp | TX (`OPS-050`–`OPS-060`) | DB is no help (soft delete) | Orphan live child; partial cascade | Cascade + concurrent child write A-28; assert zero live descendants |
| INV-19 | Every unique key includes `deleted_at`; re-creation allowed | DB (all unique keys) | APP inserts fresh rows | Deleted email/code/major/registration cannot be reused | Delete then recreate each root A-13 |
| INV-20 | Stored UTC; displayed WIB | APP conversion + DSN `loc=UTC`, `time_zone` | TX reads `UTC_TIMESTAMP()` once | Silent +7h/-7h error; wrong expiry | A-03: write/read across DSN, render WIB, assert UTC storage |
| INV-21 | Session belongs to live account; expired/revoked authenticates nothing | APP auth query joins live account | TX write revalidation | Deleted student keeps acting; expired cookie works | A-18: delete/expire/revoke then request |
| INV-22 | Reset tokens student-only, teacher-issued, 24h, single-use | DB FKs + `chk_reset_window`; APP consume | TX `FOR UPDATE` | Teacher reset, reuse, expiry bypass | A-17: reuse, expire, wrong role |
| INV-23 | Referenced major cannot be deleted | APP reference check under major lock | TX shared-lock protocol (`OPS-150`) | Students/quizzes point at a hidden major | A-14: delete vs concurrent create; four reference kinds |
| INV-24 | Zero/one image; path/MIME/bytes/hash all-or-none; allowed MIME; ≤10 MiB | DB `chk_questions_image`, `chk_questions_mime`, `chk_questions_image_bytes` | APP sniffing + size cap | Broken image row; script content served | A-25/A-42: each partial tuple fails; >10 MiB rejected |
| INV-25 | Registration/attempt states derived, never stored | APP derivation only | — (no column exists) | Two sources of truth drift | Schema has no status column; queries derive (A-28, A-32) |
| INV-26 | Only owner reaches authoring/results/recap/grading/monitor/stream | APP SQL predicate | TX re-check | Cross-tenant leak; 403 existence oracle | A-37 probes with second teacher → 404 |
| INV-27 | Anonymous identity = cookie hash; names are not identity | DB `uq_anon_cookie` | APP hash lookup only | Same-name merge or identity theft | Two same-name cookies → two rows; name change reuses row (A-46) |
| INV-28 | Every POST CSRF except beacon (ownership+same-origin) | APP middleware | TX ownership recheck | CSRF on state changes; forged beacons | A-06: missing/bad token → 403; beacon without Origin → 403 |
| INV-29 | No hard delete, no restore route | APP route surface | DB FKs `RESTRICT` | Data loss or resurrection races | A-45: grep route table; FK blocks physical delete |
| INV-30 | Review output is a pure function of mode, `show_score`, grading, ownership | APP (ATT) | Shared view model | Mode leak; hidden score leak | A-17 matrix for four modes × score × owner |

**OPS-041** Additional cross-table invariants. These are OPS-owned; the owning domains
must cite them and may not weaken them.

| ID | Invariant | Enforcement | If bypassed | Verification |
|---|---|---|---|---|
| OPS-INV-01 | A child write holds its aggregate root lock and revalidates the live parent chain in the same transaction | TX | Live child under deleted parent; cascade half-complete | A-28, A-36 |
| OPS-INV-02 | One write event uses one T for every row it stamps; no repeated `NOW()` | TX (`OPS-051`) | Cascade rows carry different timestamps; restore undefined | A-10: group by `deleted_at` = 1 |
| OPS-INV-03 | A committed row never references a missing upload; orphan files are allowed | APP file protocol (`OPS-123`) | Broken images; 500 on serve | A-25/A-35/A-36 |
| OPS-INV-04 | Authorization and state guards are re-read inside the write transaction | TX | TOCTOU: revoked actor writes, closed quiz accepts start | A-18/A-29 |
| OPS-INV-05 | No app code soft-deletes a `role='teacher'` account | APP guard + test | Quizzes/results become ownerless | A-12 |
| OPS-INV-06 | Every POST ends 303, 422, 403, 404, 429, or 500; beacon may 204 | APP middleware | Double-submit, retry ambiguity | A-06/A-08/A-09 |
| OPS-INV-07 | Every expiry predicate uses the same T read once per request/transaction | TX | Token/session lives seconds too long or dies early | A-03/A-17 |
| OPS-INV-08 | A soft-deleted root has zero live descendants after commit | TX + post-commit assertion | Orphans invisible to reads but counted by aggregates | A-10/A-28 |
| OPS-INV-09 | Every route is owner-scoped or deliberately public; no route relies on hidden UI | APP route matrix | Direct URL bypass | A-32/A-37 |
| OPS-INV-10 | Logs never contain passwords, tokens, cookies, CSRF values, essay text, emails, or full names | APP logging policy | Credential/PII leak | A-33 |
| OPS-INV-11 | No background mutation except the session sweep, which only expires already-dead sessions | APP ops | Surprise data change; grade/score drift | A-39 |
| OPS-INV-12 | An idempotent retry produces the same logical state and never a duplicate row | TX natural keys | Duplicate attempts/answers/registrations | A-19–A-23 |

**OPS-042** Constraint-to-rule map for the columns with no DB assertion. Postgres-style
partial indexes do not exist here, so these stay APP/TX: role-typed actors (INV-03),
student-profile cardinality (INV-02), audience eligibility, question ownership chains
(INV-12/14), final-score arithmetic (INV-15), major reference protection (INV-23), image
file existence (OPS-INV-03), and every soft-delete cascade (INV-18).

**OPS-043** A CHECK/foreign-key failure that reaches the database from user input is a
mapping bug, not a validation strategy. Handlers validate first; if MariaDB still returns
error 3819 (CHECK), 1452 (FK), or 1406 (too long), the transaction rolls back and the
response is 500, because the app failed to predict a legal input shape.

---

## 5. Soft-delete cascade matrix

**OPS-050** Deletion is soft, application-level, and one transaction (R-14). Live means
`deleted_at = '1970-01-01 00:00:00'`; deleted means a real UTC timestamp. No read may use
`deleted_at IS NULL`; the schema never stores NULL there. No route hard-deletes or restores
(INV-29). The only physical delete in the system is `DROP`-free: none.

**OPS-051** **One timestamp.** At the start of a delete transaction, read T once
(`SELECT UTC_TIMESTAMP()`) and bind T to every `UPDATE ... SET deleted_at = :T`. Never use
`NOW()` per statement and never let `CURRENT_TIMESTAMP` default different rows. `updated_at`
will also change via `ON UPDATE CURRENT_TIMESTAMP`; it is audit noise, not the event marker.
Restore (operator-only, out of app scope) is one predicate: `WHERE deleted_at = :T`.

**OPS-052** **`deleted_by_account_id`.** Exactly the five provenance roots set it to the
actor: `accounts`, `majors`, `quizzes`, `questions`, `anonymous_participants`. Every cascaded
descendant leaves it NULL. `questions` is both root and cascade target: direct delete from
the question editor sets the actor; delete caused by a quiz cascade leaves NULL. A NULL on a
root therefore means “system or cascade”, never “unknown human”. No other table has the
column; do not add one.

**OPS-053** Cascade matrix. “T” is the transaction timestamp from OPS-051; “owner” is the
actor column value.

| Root event | Cascaded descendants (all stamped T) | Root `deleted_by` | Live-filter consequence |
|---|---|---|---|
| Student account delete (`accounts` root) | `students`, `sessions`, `password_reset_tokens`, `registrations` → `quiz_attempts` → `attempt_answers` → `attempt_answer_selections`, and those attempts' `focus_loss_events` | actor | Email freed; login impossible; history gone from student and teacher views; attempt counts drop |
| Anonymous participant delete | `registrations` → attempts → answers → selections; attempts' focus events | actor | Cookie hash freed; re-join creates a new identity; monitor roster shrinks |
| Draft quiz delete | `questions` → `answer_options`; question image files after commit | actor | Code/invite freed; builder/list no longer see it |
| Draft question delete | `answer_options`; its image file after commit; remaining live questions renumbered 1..N | actor | Positions stay dense; attempt slots never exist for a draft question |
| Unused major delete | none | actor | Name and code freed for re-creation; no child rewritten |
| Teacher account delete | **forbidden** (OPS-055) | — | — |
| Session revoke/expiry | none; row itself | NULL (system) | Token stops authenticating |
| Registration cancel | `quiz_attempts` → answers → selections/focus (defensively; normally zero attempts) | NULL (no column) | Participant may re-register as a new row; quota resets only via new registration |

**OPS-054** **Descendant scoping is by FK, not by walk.** Each statement is a set update:
`registrations WHERE student_id = :sid`, `quiz_attempts WHERE registration_id IN (...)`,
`attempt_answers WHERE attempt_id IN (...)`, `attempt_answer_selections WHERE
attempt_answer_id IN (...)`, `focus_loss_events WHERE attempt_id IN (...)`. Use ordered
`IN`/joins; never fetch IDs and loop in a way that can exceed packet limits. At this scale a
single multi-table `UPDATE ... JOIN` per level is expected; the spec fixes the scope, not the
SQL spelling.

**OPS-055** **Teacher deletion is forbidden, not cascaded.** No HTTP route and no code path
may set `accounts.deleted_at` where `role='teacher'`, and none may soft-delete a teacher's
quizzes, questions, attempts, or results. R-05 makes teachers fixed. A future operator
soft delete of a teacher account (schema §5.7) hides the account and sessions only; the quiz
owner FK remains valid and results remain attributable. The app must render an ownerless quiz
as an integrity state, not crash; but it must never create that state itself (OPS-INV-05).

**OPS-056** **Draft-delete assertions.** R-10 promises that no attempt data is deleted by a
quiz delete. Before stamping a draft quiz, assert `NOT EXISTS` live registrations **and**
`NOT EXISTS` live attempts under any registration for that quiz. If either exists, roll back
and return 500 (integrity fault), never cascade it silently. The same assertion guards a
question delete: `NOT EXISTS` live `attempt_answers` for the question. These states are
unreachable through legal flows (drafts cannot be joined; open quizzes cannot be edited), so
their presence means a bug or manual data edit.

**OPS-057** **Renumber after question delete.** Deleting question at position k leaves a gap.
In the same transaction: soft-delete the question and its options, then renumber the
remaining live questions to 1..N with the two-phase offset procedure in OPS-160. Do not
renumber before stamping: a live row cannot take a position still held by a live row, but it
may take a position held by a just-deleted row because `deleted_at` differs in the unique key.

**OPS-058** **Unique-key re-creation.** All unique keys include `deleted_at` (INV-19), so
after a delete the old value is free: email, major name, major code, quiz code, invite token,
anonymous cookie hash, session token hash, reset token hash, registration (quiz, participant),
attempt number under a **new** registration, question/option position, answer slot under a new
attempt. Re-creation always inserts a new row; it never revives or updates the deleted row.
The old row's history stays visible only to a DBA/restore, not to the app.

**OPS-059** **Live sentinel on every read and join.** Every SELECT, aggregate, EXISTS, and
UPDATE predicate filters the root and every joined table at the sentinel. This includes
counts (dashboards, monitor, results, grading queue), uniqueness pre-checks, ownership joins,
and cascade descendants. A query that joins a parent must also filter the parent; filtering
only the child is the classic orphan-leak bug.

**OPS-060** **No live child under a deleted parent.** The cascade is the only writer allowed
to create a deleted parent with live children, and it must not commit that state. A
post-commit assertion job (or test fixture) verifies OPS-INV-08 for each root table:
`accounts`, `anonymous_participants`, `quizzes`, `questions`, `majors`, `registrations`,
`quiz_attempts`, `attempt_answers`. A violation is logged with IDs, never repaired
automatically.

**OPS-061** **Image-file cleanup.** Files are deleted **after** the DB commit, best effort,
never before. Collect the exact `image_path` values while the rows are still readable, commit
the soft deletes, then unlink each path under the uploads root. A failed unlink is logged
(`upload_orphan`) and tolerated; an orphan file is harmless. A missing file for a live row is
an integrity fault (`upload_missing`), returns 404 on serve, and is logged. Never delete a file
still referenced by a live question; no file is shared, so path equality is sufficient.

**OPS-062** **Replace image.** Write the new file first (OPS-123), update the row to the new
path/metadata, commit, then unlink the old path. If the DB update fails, unlink the new file
(best effort) and keep the old row/file intact. A crash between commit and unlink leaves an
orphan, not a broken row.

**OPS-063** **What must never be cascaded.** Teacher delete → quizzes/questions/results
(forbidden generally). Quiz delete → accounts, majors, teacher data, or another quiz's rows.
Question delete → attempts or answers (assert none). Major delete → students/anonymous/quizzes
(refuse instead). Student delete → quizzes, other students, or the student's own email
history outside its own rows. Anonymous delete → the browser cookie itself (the app cannot
and must not try; the hash simply resolves to nothing). Session revoke → account or profile.
Registration cancel → the anonymous identity row or the student profile.

**OPS-064** **No purge and no restore.** Soft-deleted rows and their files' metadata are
retained indefinitely in v1. There is no purge job, no retention window, no vacuum, no
undelete route, and no UI affordance. Backups are the only recovery. The session sweep
(OPS-188) is the sole exception, and it acts only on sessions already past `expires_at`.

**OPS-065** **Manual repair is out of band.** If an operator restores rows by setting
`deleted_at` back to the sentinel, unique collisions (a re-used email/code/major) must fail
loudly and abort the restore, never merge silently (schema §5.7). The app has no code path
for this and must not acquire one without a new ruling.

---

## 6. Transaction boundaries

**OPS-100** Every mutating command runs in one explicit transaction with
`BEGIN … COMMIT` (or `ROLLBACK`) and a request context deadline of 10 s. Isolation is the
MariaDB default `REPEATABLE READ`; every read that feeds a write is a **locking/current
read** (`FOR UPDATE` or a shared lock), because a consistent snapshot read can authorize a
row that a concurrent transaction has already deleted or submitted. The one exception is
immutable published content (questions, options, published settings), which may be read from
the snapshot because `INV-10` forbids all writers (`OPS-101`). Read-only pages use one
snapshot for all queries that feed one view model, so counts cannot disagree within a page.

**OPS-101** **Lock order.** When a transaction needs more than one row lock, acquire in this
order — parent before child — and within a level by ascending `id`:
`majors → accounts → students → sessions → password_reset_tokens → quizzes → questions →
options → anonymous_participants → registrations → quiz_attempts → attempt_answers →
attempt_answer_selections → focus_loss_events`. A flow locks only the rows it needs, but
whatever it locks must appear in this relative order. The root named in the flow is locked
first within its level. Consequences that are easy to get wrong:

- A write that sets `students.major_id`, `anonymous_participants.major_id`, or
  `quizzes.target_major_id` takes the **shared major lock before** the account or quiz lock,
  even when the route id is the account or quiz (`OPS-150`).
- Reset-consume pre-reads the token to learn its account, then locks the **account before the
  token** (`OPS-109`); reset-issue locks the account/profile before the token rows.
- A student delete locks the account, then the profile, sessions/tokens, and descendants
  (`OPS-120`); an attempt start locks the quiz and the anonymous identity before the
  registration (`OPS-110`).

Never lock a child first and then its parent in the same transaction: that is the deadlock
shape this order forbids. Immutable published content — questions, options, and a published
quiz's settings — may be read from the transaction snapshot instead of being locked, because
`INV-10` forbids every writer from changing it after publish; it cannot move under the reader.

**OPS-102** `innodb_lock_wait_timeout` is 5 s per session; a lock timeout (1205) or deadlock
(1213) aborts the transaction. The handler retries **once** only for flows declared
idempotent in §7 (answer save, attempt start, submit, beacon progress), and only when no
external side effect has happened. It never retries a file write, a delete, a grade, or a
publish. A second failure is 500.

**OPS-103** T is read once per transaction (`SELECT UTC_TIMESTAMP()`) and bound to every
timestamp the transaction writes: `submitted_at`, `closed_at`, `published_at`, `approved_at`,
`graded_at`, `used_at`, `occurred_at`, every `deleted_at`. `created_at` may keep its DB default;
`updated_at` is maintained by the table. No business decision may read the app clock while the
transaction writes T, and no statement may call `NOW()` itself.

**OPS-104** External side effects are ordered around the commit: file **creation** happens
before the transaction that references it; file **deletion**, SSE publication, and flash
cookies happen after commit. No SSE write, HTTP write, or unlink ever runs while the
transaction is open. If the process dies after commit and before the side effect, the DB is
correct and the side effect is retried by the next request or left as an orphan/heartbeat
gap, never compensated by a rollback.

**OPS-105** **Student registration** (`IDN POST /register/student`). Locks: shared lock on the
live `majors` row, then insert. Writes: `accounts` (`role='student'`, `status='active'`,
`approved_*` NULL, `must_change_password=0`), then `students` (`grade_level`, `major_id`).
Rollback: any failure removes both rows (one transaction); never leave an account without a
profile (INV-02). Conflict: duplicate live email → 1062 → 422 `Email sudah terdaftar.`; major
deleted concurrently → shared lock/current read fails → 422 field error on `major_id`.
After commit: create session + CSRF; 303 `/student`. No files.

**OPS-106** **Teacher registration** (`IDN POST /register/teacher`). Locks: none beyond the
unique key. Writes: one `accounts` row `role='teacher'`, `status='pending'`, `approved_*`
NULL, `must_change_password=0`. Never writes an `students` row, never sets `active`, never
approves (R-03/R-24). After commit: issue the signed 24 h pending token cookie and 303
`/auth/pending?token=…`. Duplicate email → 422; the losing request creates nothing.

**OPS-107** **Teacher approval** (`IDN POST /teacher/teachers/:id/approve`). Locks: actor and
target `accounts` rows `FOR UPDATE` in ascending id order. Validate: actor live active teacher;
target live `role='teacher'` and `status='pending'`. Writes: `status='active'`,
`approved_by_account_id=actor`, `approved_at=T`, `updated_at` auto. If the target is already
active, do not write: 303 with an informational flash (idempotent double-click). If the target
is a student, soft-deleted, or missing: 404. No files; no session is created for the target.

**OPS-108** **Reset-token issue** (`IDN POST /teacher/students/:id/reset-token`). Locks, in
`OPS-101` order: target `accounts` row `FOR UPDATE`, then the `students` profile row
`FOR UPDATE` (serialises issue, consume, and student delete). Validate actor live active
teacher and target live student. Writes: stamp `deleted_at=T` on
every prior live unused token of that student (revocation, `deleted_by` NULL), then insert one
`password_reset_tokens` row with a 32-byte random token, SHA-256 hash, `expires_at=T+24h`,
`used_at=NULL`, `issued_by=actor`. Return the raw token once in the response; it is never
stored or logged. Rollback on any failure; no file. Two concurrent issues serialise, last
committed wins, the first teacher's displayed token is already revoked (`OPS-167`).

**OPS-109** **Reset consume** (`IDN POST /reset-password`). Pre-read the token row by hash
**without a lock** to learn its `account_id`; then lock the account row `FOR UPDATE`, then the
token row by hash `FOR UPDATE` (parent before child, `OPS-101`) and re-validate: token live,
`used_at IS NULL`, `expires_at > T`, student account live. The pre-read is not authoritative;
only the locked re-validation decides. Writes: `used_at=T`; account `password_hash` =
new hash, `must_change_password=0` (R-04); stamp `deleted_at=T` on **every** live session of
that account and rotate CSRF. Commit; 303 `/login` with success flash. A second consume sees
`used_at` and returns 422 `Token sudah digunakan.`; expired/unknown/deleted → 422 `Token tidak
valid atau sudah kedaluwarsa.` No file.

**OPS-110** **Attempt start** (`PAR POST /quiz/:id/attempts`, executing `ATT` start rule). One
transaction; ATT's start rule is invoked inside it, not as a second committing call (`Q-06`).
Locks, in `OPS-101` order: `accounts` shared + `students` shared for a student actor (or
nothing for anonymous); quiz row shared; `anonymous_participants` shared for an anonymous
actor; then the `registrations` row `FOR UPDATE`. Current-read the live questions. Validate
under those locks: registration live, quiz live and `state='open'`, actor owns the registration
(student account or anonymous cookie), and student eligibility still holds. If a live attempt
with `submitted_at IS NULL` exists, **return it** and write nothing (resume). Count live
attempts; if `max_attempts > 0` and count ≥ cap → 303 intro with error flash; if
`max_attempts = 0` (never written by the app) treat as unlimited but still honour the R-06
contract cap of 10 on every write. Insert `quiz_attempts` (`attempt_no = count+1`,
crypto-random `shuffle_seed`, `submitted_at NULL`, `final_score NULL`), then insert one
`attempt_answers` row per live question in the derived question order (`answer_text NULL`,
`score NULL`, `graded_at NULL`). Unique `(registration_id, attempt_no, deleted_at)` is the
backstop. A failed insert rolls back the whole unit; if the registration was created earlier it
stays, but no attempt or slot is left. Commit; 303 `/attempt/:id`. No files; SSE after commit.

**OPS-111** **Answer save** (`ATT POST /attempt/:id/answer`). Locks: attempt row
`FOR UPDATE`; slot row `FOR UPDATE` (insert if absent). Published quiz/question/option content
is immutable and read from the snapshot (`OPS-101`); the lock on the attempt serialises against
submit and against the cascade that stamps the attempt. Validate: attempt live, owned by the
actor, `submitted_at IS NULL`; quiz live; question live and belongs to the attempt's quiz;
every selected option live and belongs to that question; cardinality matches the type. Writes:
upsert the slot; objective — compute the **set difference**, soft-delete removed selections
with T, insert added selections (never insert a duplicate live pair); essay — normalise line
endings, trim, set `answer_text` (empty → NULL); always touch `quiz_attempts.updated_at=T` so
`last_seen` moves. Commit; 303 to the next question/review per `nav`. Identical retry is a
no-op (`OPS-160`). No files; SSE after commit.

**OPS-112** **Submit** (`ATT POST /attempt/:id/submit`). Locks, in `OPS-101` order: the
`registrations` row **shared** (PAR-203; this serialises against cancel and student/identity
delete), then the attempt `FOR UPDATE`; then all live slots `FOR UPDATE` ordered by `id`.
Published question/option content is immutable (`INV-10`), so it is read from the transaction
snapshot rather than locked. If the registration is no longer live, return 404 and write
nothing. If `submitted_at IS NOT NULL`, commit nothing and 303 the existing result
(idempotent). Assert one live slot per live question and no extra live slot; a mismatch is a
500 integrity fault. Writes: `submitted_at=T`; for each objective slot set `score` to 0 or 1
(`OPS-130`) and `graded_at=T` with `graded_by_account_id=NULL` (the CHECK requires the pair;
system provenance is NULL); leave essay `score`/`graded_at` NULL; recompute `final_score` only
if every live answer now has a score (`OPS-131`), else leave NULL. Commit; 303 result. No
files; SSE after commit.

**OPS-113** **Grade essay** (`ATT POST /teacher/grading/:answerID`). Pre-read the answer
(non-locking) to find its attempt, then lock `quiz_attempts` `FOR UPDATE`, then the
`attempt_answers` row `FOR UPDATE` (order respected). Validate: actor live teacher, quiz owner
= actor, answer live, question type essay, attempt live and submitted, answer score `IS NULL`.
Writes: `score` from the 0–100 one-decimal input (`OPS-131`), `feedback` (≤1000 chars,
optional), `graded_by_account_id=actor`, `graded_at=T`; if every live answer of the attempt is
now scored, set `final_score` via `OPS-131`, else leave NULL. Write-once: a second POST with
the same score/feedback is idempotent success; a different value is a state conflict with
`Jawaban ini sudah dinilai.` and no write. Commit; 303 queue. No files; SSE after commit.

**OPS-114** **Publish** (`AUT POST /teacher/quizzes/:id/publish`). Locks: quiz `FOR UPDATE`.
Validate: owner = actor, live, `state='draft'`, ≥1 live question, every live question valid per
INV-11, private target pair present. Writes: `state='open'`, `published_at=T`, `closed_at=NULL`,
`updated_by_account_id=actor`. **No code is minted here**: R-11 creates the public code at
creation and private rows store NULL; “activating” the code is the open-state guard on
resolution. Commit; 303 builder. Double publish: second sees open and returns 303 with an
informational flash; closed → 303 with error flash. No files.

**OPS-115** **Close** (`AUT POST /teacher/quizzes/:id/close`). Locks: quiz `FOR UPDATE`.
Validate owner and `state='open'`. Writes: `state='closed'`, `closed_at=T`,
`updated_by_account_id=actor`; code and invite token remain stored but resolve to `not_open`.
No attempt is touched: in-progress attempts stay answerable/submittable (R-10). Commit; 303
builder; after commit broadcast `quiz.closed` and close subscribers. Double close is
idempotent informational; publish/close serialize on the quiz lock.

**OPS-116** **Reorder** (`AUT POST /teacher/quizzes/:id/questions/reorder`). Locks: quiz
`FOR UPDATE`; every live question of the quiz `FOR UPDATE` ordered by `id`. Validate owner,
draft, the submitted id list is exactly the live set (no duplicates, no omissions, no
foreign ids). Write positions 1..N using the two-phase offset (`OPS-161`) in one transaction.
If the set changed since the page render, roll back and 303 builder with error flash
`Daftar soal berubah. Muat ulang halaman.` Concurrent reorder serialises; last committed wins.
No files.

**OPS-117** **Question create/update with image** (`AUT POST …/questions[/:qid]`). Validate
all fields and the file first (OPS-123); write the new file to its final random name; then
`BEGIN`, lock quiz `FOR UPDATE` (owner, draft), lock question `FOR UPDATE` when editing.
Writes: insert/update the question (`updated_by_account_id=actor`) and its image metadata;
replace options as a set (soft-delete removed, insert added, preserve `id` where the option
body is unchanged is optional; positions 1..N); create appends position N+1. Commit. On any
DB failure unlink the new file (best effort) and leave the old row/file untouched. On success
of a replace, unlink the old file after commit. No orphan is ever referenced by a committed
row.

**OPS-118** **Quiz delete** (`AUT POST /teacher/quizzes/:id/delete`). Locks: quiz `FOR UPDATE`.
Validate owner, live, draft, and the OPS-056 assertions. Collect `image_path` values; stamp
quiz (root, actor), questions and options (NULL) with T. Commit; 303 list; unlink files after
commit, best effort. A non-draft → 303 with error flash `Hanya draf yang dapat dihapus.`; a
missing/non-owned quiz → 404.

**OPS-119** **Question delete** (`AUT POST …/questions/:qid/delete`). Locks: quiz
`FOR UPDATE`, then question `FOR UPDATE`. Validate owner, draft, live question, OPS-056
assertion. Collect its path; stamp question (root, actor) and options (NULL) with T;
renumber remaining questions 1..N (`OPS-161`). Commit; unlink file after commit.

**OPS-120** **Student delete** (`IDN POST /teacher/students/:id/delete`). Locks, in `OPS-101`
order: actor and target accounts `FOR UPDATE` ascending id; then the target profile, sessions
and reset tokens; then the student's registrations/attempts/answers/selections/focus rows as
needed by the set updates. Validate actor live active teacher, target live `role='student'`,
profile live. Stamp T on: account root (actor), profile, sessions, reset tokens,
registrations, attempts, answers, selections, focus events (NULL). Commit; 303 list. No files.
The student's browser gets 404/login on its next request; in-flight requests may complete
(`OPS-171`).

**OPS-121** **Quiz-delete preflight** is part of OPS-118 and is repeated here for traceability:
zero live registrations and zero live attempts are asserted inside the transaction, not on the
prior GET. This is the rule that keeps R-10's “no attempt data is ever deleted by a quiz delete”
true even if a manual data edit created an impossible row.

**OPS-122** **Anonymous delete** (`PAR POST /teacher/anonymous/:id/delete`). Locks: anonymous
row `FOR UPDATE`; then its registrations/attempts. Validate actor live active teacher and row
live. Stamp T on the identity (root, actor), registrations, attempts, answers, selections,
focus events (NULL). Commit; 303 list; SSE roster snapshot after commit (`OPS-236`). No files.
Cancelling one registration is NOT this flow: it stamps only that registration and its
descendants (normally none) and leaves the identity live (R-02).

**OPS-123** **Image write protocol.** Generate a 128-bit random name as 32 lowercase hex
characters plus an extension derived from the **sniffed** MIME (`.jpg`, `.png`, `.webp`).
Write to a temp file in the uploads root, read back/fsync, then rename atomically to the final
name (same filesystem). Reject empty, >10,485,760 bytes, or any sniffed type outside the three
allowed. Store `image_path` relative to the root (just the name), `image_mime` from the sniff,
`image_bytes` from the actual byte count, `image_sha256` of the bytes, and optional
`image_alt` (≤255, empty → NULL). Never trust the client filename, extension, or
`Content-Type`. A DB failure after the file write unlinks the new file; a failure to unlink is
an orphan, not a broken row.

**OPS-124** **Major delete** (`PAR POST /teacher/majors/:id/delete`). Locks, in `OPS-101`
order: major `FOR UPDATE`, then the actor account shared. Validate actor live active teacher,
major live. Check live
references in `students`, `anonymous_participants`, and `quizzes.target_major_id`; any hit →
rollback, 303 majors with error flash `Jurusan masih digunakan oleh siswa, peserta, atau kuis.`
No reference → stamp the major (root, actor) with T; commit; 303 list. The shared-lock protocol
in OPS-150 closes the check-then-insert race.

**OPS-125** **Session lifecycle.** Login: lock the account row shared, validate credentials and
status, then insert a session row (32-byte token, SHA-256 hash, `expires_at=T+60m`, IP/UA),
update `last_login_at=T`; rotate CSRF; commit. Logout: lock the session row `FOR UPDATE`,
stamp `deleted_at=T`; rotate CSRF; commit. Change password: lock the account `FOR UPDATE`,
verify the current hash, compare-and-swap `password_hash` (`WHERE id=? AND password_hash=?`),
set `must_change_password=0`, stamp every **other** live session with T; commit. A lost CAS
means the old password changed concurrently → 422 `Kata sandi saat ini salah.`; no revocation
happens in the losing transaction. After a successful login **or** logout commit, expire both
PAR cookies: `quiz_participant` (`Path=/`) and `quiz_join_ticket` (`Path=/join`), each with
`Max-Age=0` and the same `HttpOnly`/`SameSite=Lax`/`Secure` attributes it was issued with
(PAR-020, PAR-199). IDN never reads, validates, or logs either cookie; a signed-in session and
an anonymous identity never merge (`IDN-157`).

**OPS-126** **Quiz settings save** (`AUT POST /teacher/quizzes/:id`). Locks: quiz
`FOR UPDATE`. Validate owner, live, `state='draft'`; the field list is AUT's, but OPS fixes
these cross-cutting rules: `visibility` is immutable and is never written by this route;
`allow_retake=false` forces `max_attempts=1`, `true` accepts 1–10, and 0 is never written
(R-06); `review_mode` must be one of the four storage values; booleans are normalised to 0/1;
`updated_by_account_id=actor`. Validation failure is 422 with the field kept; no partial
update. Concurrent saves serialise on the quiz and last-write-wins (no version column). No
file.

**OPS-127** **Invite auto-registration** (`PAR GET /invite/:token`). The only GET that may
write, because R-12 requires it. Resolve the live quiz by `invite_token`; draft/closed →
`invite.html` with `not_open` (200, no write); private + visitor → login prompt
`anonymous_not_allowed` (no write); public + visitor → 303 `/join/identify?quiz=:id` with the
signed join cookie (no write). Logged-in student: check live registration first; absent and
eligible → one transaction: shared lock on the live `accounts` row and the `students` profile,
then the quiz row shared, re-check `state='open'` and eligibility against the locked profile,
then insert the registration. Duplicate `uq_reg_student` → informational flash, no write.
Commit; 303 `/quiz/:id`. No attempt is created; the intro offers **Mulai**. A concurrent
close/delete makes the shared-lock re-read fail → `not_open`/404 and no row; a concurrent
student delete holds the profile lock, so after it commits the re-read fails and no
registration is inserted (`IDN-154`, `OPS-101`). Idempotent by the unique key. `SameSite=Lax`
means a cross-site subresource request does not carry the session cookie; a top-level invite
link is the intended flow and its only effect is the viewer's own idempotent registration.

**OPS-128** **Anonymous identify** (`PAR POST /join/identify`). Requires a valid signed join
cookie from `OPS-129`/`OPS-127`; absent/expired → 303 `/join`. Validate the form (PAR's field
rules); failure is 422 with the identity form and no transaction. Transaction, in `OPS-101`
order: shared lock on the live `majors` row when the resulting `major_id` is non-NULL; quiz row
shared; re-check `state='open'` and public visibility; upsert the identity by
`uq_anon_cookie (cookie_token_hash, deleted_at)` — absent → insert with T; live → lock the
identity row, then update `display_name`, `grade_level`, `major_id`, `last_seen_at=T`; the
row is never looked up by name. Then insert the registration under `uq_reg_anon`; duplicate →
existing registration, informational flash. Commit; set the anonymous cookie (raw 32-byte
token; only its SHA-256 is stored) and 303 `/quiz/:id`. Concurrent double submit yields one
identity and one registration. A private quiz, deleted major, or closed quiz → no write and
the PAR error state.

**OPS-129** **Join code resolution** (`PAR GET, POST /join`). Read-only. POST validates exactly
six digits; lookup is one live `quizzes` read by code with `deleted_at = sentinel`. A private
match is inert (R-11): `unknown_code` for a logged-in student, `anonymous_not_allowed` for a
visitor. A draft match is 404 for every non-owner (R-12). A closed match → 303 back to `/join`
with `not_open`. An open public match: logged-in student → 303 `/quiz/:id` and **no**
auto-registration (R-12); visitor → 303 `/join/identify?quiz=:id` with a signed, `HttpOnly`,
`SameSite=Lax` join cookie holding the quiz id and a 30-minute expiry; unknown code → 303
`/join` with `unknown_code`. Every branch is 303 except the draft 404; no row is written and no
attempt or registration is created. Rate-limited per IP with the login bucket (`OPS-231`).

### 6.1 Scoring arithmetic (one shared function)

**OPS-130** **Objective score at submit.** Compare sets, never order. `single`: the live
selection set must have exactly one member and equal the live correct set (which INV-11
requires to have exactly one) → 10,000 units (1.0000); otherwise 0. `multiple`: the live
correct set has ≥2 members; the selection set equals it exactly → 10,000; otherwise 0. No
partial credit. A blank answer (zero selections) → 0. An extra incorrect selection → 0. Any
selection whose option is not live/part of the question is an integrity fault (rollback 500),
not a silent 0.

**OPS-131** **Essay score at grade.** The form accepts 0–100 with at most one decimal; the
server accepts `.` or `,` as the decimal separator, normalises, and rejects `>100`, negative,
more than one decimal, or non-numeric with 422 `Nilai harus antara 0 dan 100 dengan satu angka
desimal.` Convert exactly: `units = tenths × 10`, so 84.5 → 8,450 units (0.8450), 100 → 10,000,
0 → 0. No float arithmetic and no rounding at this step.

**OPS-132** **Final score.** Let S be the sum of `score` units over all live answer slots of
the attempt and N their count. Require N ≥ 1 and every score non-NULL; otherwise
`final_score` stays NULL. Compute `units = floor((2S + N) / (2N))` (half-up mean), clamp to
0…10,000, store `units / 10000` as `DECIMAL(5,4)`. The function is deterministic and must be
the only place the mean is computed; submit and grade call it after their writes, never before.
It is never computed from a stale subset: read every live slot in the same transaction.

**OPS-133** **Display rounding.** Whole percent = `floor((units + 50) / 100)`; hidden score
renders `—`, never `0`. `show_score` controls rendering only; `final_score` is stored
regardless and is never nulled to hide it. Best-score queries exclude `show_score=false`
quizzes (R-21), not zero them.

**OPS-134** **No override.** There is no manual final-score write, no re-grade route, and no
recompute job. The only writers of `final_score` are submit (objective-only quizzes) and grade
(when the last essay becomes scored), both through OPS-132.

---

## 7. Concurrency and idempotency

**OPS-140** **Code and invite generation.** Public quiz codes are six crypto-random digits
(leading zeros preserved, string compare, never parsed as an int). Invite tokens are 128-bit
crypto-random values encoded in at most 32 `ascii_bin` characters. Generate inside the create
transaction; on a 1062 duplicate, regenerate and retry, up to 10 attempts, then 500. Two
concurrent creates may collide; the loser retries with a fresh value, never reuses the winner's.
Private quizzes store `code = NULL` (R-11); a code submitted for a private row is ignored, not
stored.

**OPS-141** **Unique-key races are settled by the database, then mapped.** Pre-checks are UX
only. The authority is the unique key: 1062 on email → 422 `Email sudah terdaftar.`; on
registration → treat as an idempotent duplicate (`OPS-175`); on code/invite → retry
(`OPS-140`); on major name/code → 422 field error; on attempt number → re-read and resume;
on session/reset token hash → regenerate. The losing transaction makes no other write.

**OPS-142** **Grading is write-once.** `attempt_answers.score IS NULL` is the compare-and-set
predicate. The first committed grade wins. A retry with byte-identical score and feedback is
idempotent success (303 queue); a retry with a different value is 303 with error flash
`Jawaban ini sudah dinilai.` and no write. There is no re-grade route, no undo, and no
second writer of an essay score.

**OPS-143** **Idempotency is expressed by natural keys, not a request-id table.** The unique
keys and the state predicates above are the idempotency store. No `idempotency_keys` table
exists and none is proposed; a client retry after a lost response must converge on the same
row or the same state conflict.

**OPS-144** **Read-modify-write is never done from a snapshot.** Any value used in a `SET`
that depends on the old value (`expires_at`, `position`, `attempt_no`, `final_score`) is read
with a locking/current read in the same transaction, or the statement itself is written as a
monotonic operation (for example `GREATEST`).

**OPS-150** **Major reference lock protocol.** `majors` is the one root that cannot cascade
and whose FK still accepts a soft-deleted parent. Therefore:
(a) every write that sets `students.major_id`, `anonymous_participants.major_id`, or
`quizzes.target_major_id` takes a **shared** row lock on the live major and validates it in the
same transaction that writes the reference;
(b) major delete takes an **exclusive** lock on the major row, then checks live references;
(c) an insert that starts after the delete's X lock blocks on the FK/shared lock and, once
released, must re-read the major and fail validation because it is no longer live.
This closes the check-then-insert race that a plain `SELECT` would leave open.

**OPS-151** **Lock/transaction matrix.** Each row is the complete lock set for the flow; the
writes and rollback are in §6.

| Flow | Aggregate lock (`FOR UPDATE`) | Other current reads | Serialises against |
|---|---|---|---|
| Student registration | none | shared live major | major delete |
| Teacher registration | none | — | duplicate email |
| Approval | actor + target account (id order) | — | double approve, student delete |
| Reset issue | target account, then profile | prior tokens | consume, student delete, second issue |
| Reset consume | account, then token | — | second consume, delete |
| Attempt start | registration | shared student/anon, shared quiz, live questions | double start, cancel, student/anon delete, close |
| Answer save | attempt | slot; immutable question/options from snapshot | submit, second save, cascade |
| Submit | registration shared, then attempt | all slots; immutable content | answer save, second submit, cancel |
| Grade | attempt, then answer | quiz ownership, question type from snapshot | second grade, submit |
| Publish / close | quiz | questions (publish) | settings/question/reorder/delete |
| Reorder | quiz, then all live questions (id) | — | edit/delete/publish |
| Question create/update/delete | quiz, then question | options | reorder/delete/publish |
| Quiz delete | quiz | questions/options paths | every authoring write |
| Student delete | actor + target account (id), then profile, sessions/tokens, descendants | registrations, attempts | login, start, any child write |
| Anonymous delete | anonymous row | registrations/attempts | join, start, answer |
| Major delete | major | live references (no lock; writers hold shared major) | reference-creating writes |
| Change password | account | other sessions | login, reset consume |

**OPS-160** **Answer-save idempotency.** Normalise the submitted selection ids to a set
(deduplicate; reject an id outside the question). Compute the set difference against the live
selections: removed → stamp T; added → insert; unchanged → untouched. Essay text is compared
after line-ending/trim normalisation; equal means no text write. The attempt is always touched
(`updated_at=T`) because a save is activity for `last_seen`, but a byte-identical retry creates
no row and changes no logical answer. Concurrent saves serialise on the attempt lock; the later
commit wins whole-slot, never a merged selection set. A stale tab can therefore overwrite a
newer answer; the reload shows the winner, and this is accepted (no per-answer version column).

**OPS-161** **Two-phase position rewrite.** Because `uq_questions_position (quiz_id, position,
deleted_at)` and `uq_options_position` are live, a direct swap collides. In one transaction:
(1) add a temporary offset greater than `max(position) + N` to every live row of the parent;
(2) write the final dense 1..N positions. Use the same procedure after a question delete. If
the offset would exceed the `SMALLINT UNSIGNED` ceiling, abort 500 rather than corrupt order.
Soft-deleted rows keep their old positions and never collide because their `deleted_at`
differs; never renumber a deleted row.

**OPS-162** **Double start** (`PAR POST /quiz/:id/attempts`). Registration lock serialises.
Winner inserts attempt N+1 and its slots. Loser, after acquiring the lock, sees the live
in-progress attempt and **resumes it**: same attempt id, no second row, no second slot set.
Both requests 303 to the same `/attempt/:id`; the user sees one attempt.

**OPS-163** **Double submit.** Winner sets `submitted_at=T`, objective scores, and the final
score when possible. Loser sees `submitted_at` non-NULL and 303s the existing result without
recomputing. The user sees one result page; no duplicate grade, no second timestamp.

**OPS-164** **Publish/close races.** Both lock the quiz. Publish then close is legal
(`open → closed`); close then publish sees closed and refuses; double publish sees open and
returns an informational flash; double close sees closed and returns an informational flash.
The final state is always one of draft/open/closed, never a mixed timestamp pair
(INV-09). There is no reopen.

**OPS-165** **Double delete.** The first commit stamps; the second request finds the row
soft-deleted and returns 404 (R-19). PRG normally prevents this; a double-click that wins the
race may show the 404 page once. No cascade runs twice, and the second transaction writes
nothing.

**OPS-166** **Concurrent/stale reorder.** The submitted id set must equal the live set under
the quiz lock. If a question was added or deleted since the page render, roll back and 303
builder with `Daftar soal berubah. Muat ulang halaman.` If the set matches, last committed
order wins; the earlier reorder is overwritten, which is the expected drag-and-drop semantic.

**OPS-167** **Concurrent reset issue.** Both teachers lock the student account, so they
serialise. Last committed wins; the earlier transaction's token is revoked by the later
issuance (OPS-108). The first teacher's displayed token then returns `Token tidak valid atau
sudah kedaluwarsa.` This is the accepted cost of “one live token per student” and is recorded
in §14.

**OPS-168** **Reset-token reuse.** Token-row lock plus `used_at IS NULL` is the CAS. One
consume wins and revokes all sessions; the loser sees `used_at` and gets 422 `Token sudah
digunakan.` Expired/unknown/deleted → 422 `Token tidak valid atau sudah kedaluwarsa.` The
password is never changed by the loser.

**OPS-169** **Double approval.** Target row lock; winner sets active + approver + timestamp.
Loser re-reads, sees active, and returns 303 with an informational flash; it never overwrites
`approved_by`/`approved_at`. A student target → 404, never a promotion.

**OPS-170** **Session sliding.** Every authenticated non-static request extends the session to
`expires_at = GREATEST(expires_at, T + 60 minutes)`. `GREATEST` makes concurrent requests
monotonic: an older commit cannot shorten a newer expiry. A session whose `expires_at <= T`
authenticates nothing; it may be lazily stamped deleted with T (system) or left for the sweep.
Writes to `expires_at` are one indexed PK update per request; no throttling in v1.

**OPS-171** **Revocation window.** Logout, reset-consume, change-password-other-sessions, and
student delete take effect for requests that **begin** after the revoking transaction commits.
A request already past middleware may complete, because OPS does not lock the session row in
every write. The guarantee that matters is enforced separately: every write revalidates the
live actor/aggregate chain (`OPS-INV-04`), and a cascade is atomic, so no live child can be
created under a row deleted by a concurrent transaction.

**OPS-172** **Beacon bursts.** Progress: idempotent, touches `attempt.updated_at=T`, and may
coalesce to at most one write per second per attempt while still returning 204. Focus: one
`focus_loss_events` row per accepted event, `occurred_at=T` (the client `at` is ignored), no
dedupe, no attempt touch. Limits 30/min (progress) and 10/min (focus) per attempt; excess is
429 with `Retry-After`, never queued. Unknown kind / malformed body / body >4 KB → 422.
Missing or cross-origin `Origin`/`Referer` → 403. An attempt not owned by the cookie/session
→ 404. After submit, progress is a 204 no-op; focus still records if the attempt is live.

**OPS-173** **SSE connect/close.** A connect validates owner + `state='open'`, subscribes,
then re-checks state; if it closed in between, send `quiz.closed` and return. Send one
`snapshot`, then events; 30 s heartbeat per hub. No replay buffer: a reconnect gets a fresh
snapshot. On close, broadcast `quiz.closed`, flush with a 1 s deadline, close. Per
teacher+quiz connections are capped (10); excess gets 429 before the stream starts. The hub
publishes only after the DB commit and never blocks on a subscriber.

**OPS-174** **Slow SSE client.** Each subscriber has a small bounded queue (16 events). A full
queue evicts that subscriber and closes its connection; it cannot slow the hub, the request
path, or another teacher. The client reconnects and gets a fresh snapshot. Eviction is logged
with quiz id and count only.

**OPS-175** **Double registration / join.** `uq_reg_student` / `uq_reg_anon` decide. The loser
writes nothing and lands on the intro with an informational flash using the `already_registered`
copy `Anda sudah terdaftar di kuis ini.` (R-12). Invite auto-register is the same upsert: an
existing live registration is success, not an error page. Anonymous identify first upserts the
identity by cookie hash, then the registration; both are unique-key idempotent.

**OPS-176** **Attempt quota.** Count live attempts under the registration lock. `max_attempts`
is the total allowed, including the first; `allow_retake=false` forces 1; `true` accepts 1–10;
0 is never written (R-06). Concurrent starts cannot exceed the cap because they serialise on
the registration. A resumed in-progress attempt does not consume a new number.

**OPS-177** **Profile/email edit races.** The unique key is the arbiter; a losing concurrent
edit gets 422 `Email sudah terdaftar.` and keeps its old email. There is no lost-update
protection for `full_name`; last write wins. Email change does not revoke sessions (R-22).

**OPS-178** **Cascade vs child write.** A child write locks the child's aggregate root
(attempt for answers/submit, registration for start/cancel, quiz for authoring). A delete
locks the ancestor account/quiz first and then the same descendants. Whichever commits first
wins: if the child write commits first, the cascade includes it; if the delete commits first,
the child write's current read sees the deleted parent and aborts. No interleaving can leave a
live child under a deleted parent.

**OPS-179** **Idempotency summary.**

| Scenario | Winner | Loser | User-visible |
|---|---|---|---|
| Double student/teacher register | one account | 422 email taken; no row | field error on the form |
| Double invite auto-register | one registration | 303 intro, info flash | “already registered” info |
| Double attempt start | one attempt | resumes same attempt | same question page |
| Double answer save | last logical write | no-op/diff | reload shows saved answer |
| Double submit | one submit | 303 existing result | one result page |
| Double grade | one grade | idempotent or conflict flash | queue after save |
| Double publish/close | one transition | informational flash | builder, final state correct |
| Double delete | one cascade | 404 on the second | list (PRG) or one 404 |
| Concurrent reorder | last matching order | stale-set error flash | builder |
| Reset issue | last token only | earlier token invalid | “invalid token” if used |
| Reset consume | one password change | 422 used/invalid | login with new/old password |
| Session slide | later expiry | older cannot shorten | no visible effect |
| Beacon burst | accepted events | 429, dropped | silent (sendBeacon) |
| SSE connect | first/capped subscriber | 429 or evicted | monitor reconnects |

---

## 8. Time and retention

**OPS-180** **Clock authority.** Stored timestamps are UTC. Every transaction that writes a
business timestamp reads the database clock once (`SELECT UTC_TIMESTAMP()`) and uses that T
(OPS-103). Read-only pages use the application UTC clock for “now” and labels; a POST never
trusts it for an expiry decision. The database and application clocks must stay within 60 s;
a larger drift is an operational alert, not a silent behaviour change.

**OPS-181** **WIB is display and form input only.** Fixed UTC+7, no DST, no tzdata dependency.
Parse `datetime-local` values (`YYYY-MM-DDTHH:MM`, optional `:SS`) as WIB and convert to UTC;
reject a trailing `Z`/offset or unparsable text with a field error. Render short `20 Okt 2026`,
clock `09.42 WIB`, full `Rabu, 21 Oktober 2026, 09.42 WIB`; unset optional date renders `—`.
No relative dates in v1.

**OPS-182** **`starts_at` / `ends_at` are display-only.** They never open, close, accept,
reject, or gate anything (R-15). No timer, countdown, cron, or scheduled transition exists.
The only writer of `state` is publish/close. The DB `chk_quizzes_window` still enforces
`ends_at >= starts_at`; a save that violates it is 422, not an auto-correction.

**OPS-183** **Session expiry.** `expires_at = GREATEST(expires_at, T + 60 minutes)` on every
authenticated request. A session is valid iff `deleted_at` is the sentinel, the account is
live, and `expires_at > T`; equality is expired. The cookie is a browser-session cookie
(no `Max-Age`); the server row is authoritative. Login rotates the token (new row, old row
stamped). `last_login_at` is written once per login, not per request.

**OPS-184** **Token expiry.** Reset tokens: `expires_at = T + 24h`, checked `> T`, never
extended. Pending-teacher token: `issued_at + 24h`, checked `now <= issued_at + 24h` with the
database clock at render; at the boundary it is expired. Neither expiry is displayed as a
countdown; show the generic invalid/expired state.

**OPS-185** **Derived durations.** Grading-queue waiting = `max(0, now − attempt.submitted_at)`,
floored to whole minutes. Monitor `last_seen` = `attempt.updated_at` when a live attempt
exists, else `registration.created_at`; never NULL (R-20). Percentages derive from counts and
clamp 0–100. No duration is stored, and no screen subtracts in the template.

**OPS-186** **Precision and ties.** `DATETIME` is second precision; timestamps created in the
same second are common. Every ordered list adds `id` ascending as the final tie-break (R-21).
Never order by a timestamp alone when the result feeds pagination, “latest”, or “first”.

**OPS-187** **Retention.** Soft-deleted domain rows are retained indefinitely; no purge,
vacuum, archive, or restore route exists (OPS-064). Expired/used reset tokens, revoked
sessions, focus events, and submitted attempts stay until their root is deleted. Backups
include soft-deleted rows and the uploads directory; restore restores both together.

**OPS-188** **Session sweep.** On startup and hourly, in bounded batches, stamp
`deleted_at = T_sweep` on sessions with `expires_at <= T_sweep` and the sentinel; `deleted_by`
NULL (system). It never touches a live session, an account, or any domain row. It is
idempotent, safe to run concurrently with requests, and a failure is logged and retried on the
next tick. This is the only background mutation allowed (OPS-INV-11).

**OPS-189** **File retention.** Replaced/deleted question images are unlinked after commit;
failed unlinks are orphans and are retained. Orphan reconciliation is a manual runbook (list
uploads root, compare against live `questions.image_path`, delete only files with no row and an
age > 24 h, log every deletion). Missing files for live rows are never recreated; they 404 and
raise an integrity alert.

**OPS-190** **Startup clock check.** The app asserts `@@session.time_zone = '+00:00'` and that
`UTC_TIMESTAMP()` advances; otherwise it logs a fatal configuration error and refuses to
serve. This is the guard against a DSN that omitted `loc=UTC`/`time_zone` and silently renders
WIB values shifted by seven hours.

**OPS-191** **Cookie lifetimes.** Session cookie: browser session, server 60 min. CSRF cookie:
signed, 24 h, rotated on login/logout. Pending token cookie: 24 h. Anonymous identity cookie:
365 days, refreshed on identify/join, `HttpOnly`, `SameSite=Lax`, `Secure` per `COOKIE_SECURE`,
`Path=/`. Clearing the anonymous cookie creates a new identity by design (R-02); the duration
is an OPS call in §14.

---

## 9. Error and status taxonomy

**OPS-200** The complete response taxonomy is OPS-011 plus the asset/stream cases below.
There is no 409 page, no 201, no 405 page, and no redirect-on-validation-error.

**OPS-201** **PRG.** Every successful or state-conflicting POST answers 303 with a relative
`Location` (`/path` only; never a scheme/host, never a user-supplied URL). The target is the
owning screen from contract §8. Validation (422), CSRF (403), not-found/ownership (404), rate
limit (429), and server failure (500) are the only non-303 POST outcomes; an upload GET may
answer 304.

**OPS-202** **Validation (422).** Re-render the same template with status 422, the submitted
non-password values, and a `FieldErrors` map keyed by the exact field names in frontend §18.
Passwords are never repopulated. No transaction is opened, or it is rolled back before
render. The CSRF token stays valid for the re-render; a fresh token may be issued but the form
must carry one.

**OPS-203** **CSRF (403).** Missing, mismatched, unsigned, or expired token → 403 with the
exact copy `Sesi formulir tidak valid. Muat ulang halaman.`, rendered through the error layout
(no new route). Issue a fresh CSRF cookie so the reload works. The beacon is the only exempt
POST; a beacon without a same-origin `Origin`/`Referer` gets the same 403.

**OPS-204** **404.** One response for missing, soft-deleted, wrong-owner, wrong-role, and
cross-tenant IDs. It never names the resource, its owner, or whether it exists. A teacher
probing another teacher's quiz, a student probing `/teacher/*`, or an anonymous participant
probing an attempt it does not own all get 404. A logged-in user with an expired session gets
the 303 login redirect instead, because that is an authentication condition, not an
authorization one.

**OPS-205** **429.** `Retry-After` is an integer number of seconds until the current window
frees a slot. Form/HTML requests render the error layout with `Terlalu banyak percobaan. Coba
lagi nanti.`; beacon/SSE requests get an empty body. The limiter is in-process, per key
(IP for login/register/reset, attempt+kind for beacon, teacher+quiz for SSE), with a bounded
key table and periodic eviction; restart clears it (OPS-231).

**OPS-206** **500.** Any database error, invariant assertion failure, missing row during a
cascade, position overflow, scoring inconsistency, or render failure → roll back, log one
structured entry with a request/correlation ID and row IDs, and render the 500 page with
`Permintaan Anda belum dapat diproses. Coba muat ulang beberapa saat lagi.` No SQL, stack,
table name, or token appears in the response. A failure after the response has started (SSE)
closes the connection; it cannot change the status.

**OPS-207** **Authentication redirects.** No session / expired / revoked → 303 `/login` with
`Sesi Anda berakhir. Silakan masuk lagi.` and no write. `must_change_password=1` → 303
`/change-password` with `Ubah kata sandi Anda sebelum melanjutkan.`, except the two allowed
routes. A pending teacher login is refused with 403 and `Menunggu persetujuan guru lain.` plus
the pending page link when a token is available; no session is created.

**OPS-208** **Enumeration disclosure.** Deliberately disclosed: email already registered
(registration), pending-teacher status after a correct password, reset token valid/used/
invalid (the three R-19 states), join-code unknown vs not-open, and wrong-audience. Never
disclosed: whether an ID exists for a non-owner, which teacher owns a quiz, whether another
student has an attempt, whether an upload name ever existed, or whether an account is a
student vs teacher beyond the route's own role. 404 and 403 bodies are identical per class.

**OPS-209** **Transaction failure.** Roll back the whole unit; do not write a partial cascade,
a half-scored attempt, or an audit row outside the transaction. The flash is set only after a
successful commit. A deadlock/timeout retries once only for OPS-102 idempotent flows; every
other failure maps to 500. A failure while writing a file leaves an orphan, never a committed
row without a file.

**OPS-210** **Flash storage.** Flash is a signed, `HttpOnly`, `SameSite=Lax`, `Path=/` cookie
with a ≤60 s lifetime and a closed message key plus at most two IDs; it is consumed once on
render and cleared. The app never trusts `?flash=`; only the signed cookie is read. The
cookie carries no PII, no free text, and no token.

**OPS-211** **Health semantics.** `GET /healthz` performs one DB ping with a 2 s timeout:
200 `text/plain` body `ok`, or 503 `text/plain` body `db unavailable`. No auth, no HTML, no
version, no migration/upload/SSE check, `Cache-Control: no-store`. It is liveness for the
compose healthcheck, not readiness for a deploy; a 503 means the process is up but cannot
serve data.

**OPS-212** **Uploads (`GET /uploads/:name`).** Only a name matching
`^[a-f0-9]{32}\.(jpg|png|webp)$` is considered; anything else, including a symlink or other
non-regular file, is 404. A live row + present file → 200 with the stored MIME, `Content-Length`,
`ETag: "<sha256-hex>"`, `Last-Modified` from the file mtime, `Cache-Control: private, no-cache,
must-revalidate`, `X-Content-Type-Options: nosniff`, and `Content-Disposition: inline`.
`If-None-Match` equal → 304. A row with a missing file → 404 + `upload_missing` log. No
directory listing, no range, no transform, no auth beyond the unguessable capability name
(R-17). The response is never cached publicly and never served by the `/static` middleware.

**OPS-213** **SSE failures.** Before the stream starts, use the normal status (404 for
not-owned/not-open, 429 for the connection cap). After `200 text/event-stream` and the first
byte, errors only close the stream; never write an HTML error page into it. A DB failure while
building the snapshot before headers is a normal 500; after headers, close and let the client
reconnect.

**OPS-214** **Unknown route/method.** The final catch-all returns the 404 page for any path
not claimed by a registered route, and for a registered path with the wrong method. There is
no 405 page and no automatic method list.

**OPS-215** **Database error mapping.** 1062 duplicate → domain mapping (OPS-141); 1213
deadlock / 1205 lock wait → retry once if idempotent, else 500; 1452 FK missing parent → 404
if user-supplied ID, else 500; 1406 data too long / 3819 CHECK → 500 (validation missed it);
driver/network/pool errors → 500; context deadline → 500 (or close for SSE). Never parse
driver text into user copy.

**OPS-216** **Copy ownership.** R-19 global keys are reused verbatim. Domain-specific copy is
owned by `IDN`/`AUT`/`PAR`/`ATT`; OPS pins only the new conflict/ops strings used above
(`Hanya draf yang dapat dihapus.`, `Daftar soal berubah. Muat ulang halaman.`, `Jawaban ini
sudah dinilai.`, `Jurusan masih digunakan oleh siswa, peserta, atau kuis.`, `Nilai harus antara
0 dan 100 dengan satu angka desimal.`). A sibling may rename a key only by citing it here.

**OPS-217** **Error page parity.** Direct `GET /error/404` and `GET /error/500` return 200
with the corresponding page (they are preview routes). A real missing resource returns the
same 404 template with status 404; a real failure returns the 500 template with status 500.
The body copy is identical so the two paths cannot drift.

**OPS-218** **Security headers and request caps.** On every HTML response:
`X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`,
and `Cache-Control: no-store` on authenticated HTML. Static assets keep the one-day cache;
uploads use OPS-212. No header may include a token, account id, or email. Request bodies are
capped **before parsing**: 1 MiB for every form POST (`IDN-128`) and 4 KiB for beacon POSTs
(`OPS-172`); an over-cap form body is rejected as 422 with a generic validation error and no
transaction is opened. The upload route uses OPS-123's 10 MiB stream cap instead. A missing or
lying `Content-Length` never bypasses the cap.

**OPS-219** **Log shape.** One structured line per request: request id, method, route pattern
(not raw path), status, duration, actor id or `anon`, quiz/attempt id when relevant. One line
per integrity fault with the invariant/rule ID and row IDs. Never log passwords, hashes,
cookies, CSRF values, raw tokens, essay text, emails, full names, or image bytes. Logs go to
stdout; retention/rotation is an operator concern.

---

## 10. Route coverage matrix

**OPS-220** The audited surface is 74 method+path pairs: the 72 pinned by
`conformance_test.go` plus `GET /healthz` and `GET /uploads/:name`. `GET /` is a redirect, not
a screen. Rows group both methods of one path; the method count in each row still adds to 74.
Owner abbreviations are the contract prefixes. “Anchor” names the owning domain's command for
that route; OPS does not restate its workflow.

| Route(s) | Owner | Anchor / note | OPS rules | AC |
|---|---|---|---|---|
| `GET /` | IDN | redirect to `/login` or role home | OPS-011, OPS-214 | A-32 |
| `GET, POST /login` | IDN | authenticate; rotate session, CSRF | OPS-012, OPS-170, OPS-207, OPS-208 | A-06, A-18, A-38 |
| `GET, POST /register/teacher` | IDN | pending account + pending token | OPS-106, OPS-141, OPS-184 | A-15 |
| `GET, POST /register/student` | IDN | account + profile, one transaction | OPS-105, OPS-150 | A-15 |
| `GET, POST /reset-password` | IDN | consume teacher-issued token | OPS-109, OPS-168 | A-17 |
| `GET, POST /change-password` | IDN | verify current, revoke other sessions | OPS-125, OPS-170, OPS-171 | A-17, A-18 |
| `GET /auth/pending` | IDN | stateless signed token states | OPS-030, OPS-184 | A-17 |
| `GET, POST /join` | PAR | code resolution; signed join cookie; no write | OPS-129, OPS-231 | A-46 |
| `GET /invite/:token` | PAR | auto-register when eligible; otherwise no write | OPS-127, OPS-175 | A-46 |
| `GET, POST /join/identify` | PAR | anonymous identity upsert + registration | OPS-128, OPS-175 | A-46 |
| `POST /logout` | IDN | stamp current session | OPS-125, OPS-171 | A-18 |
| `GET, POST /profile` | IDN | student-only name/email; unique email | OPS-141, OPS-177 | A-15 |
| `GET /teacher/students` | IDN | school-wide roster, live filter | OPS-059, OPS-186 | A-32 |
| `GET, POST /teacher/students/:id` | IDN | edit profile; grade/major change | OPS-141, OPS-150, OPS-177 | A-14, A-15 |
| `POST /teacher/students/:id/delete` | IDN | account cascade | OPS-120, OPS-053, OPS-171 | A-28, A-44 |
| `POST /teacher/students/:id/reset-token` | IDN | issue/revoke 24 h token | OPS-108, OPS-167 | A-17 |
| `GET /teacher/teachers` | IDN | directory; read-only | OPS-059 | A-32 |
| `POST /teacher/teachers/:id/approve` | IDN | pending → active | OPS-107, OPS-169 | A-16 |
| `GET /student` | PAR | dashboard; ATT score contract | OPS-059, OPS-185 | A-32 |
| `GET /student/history` | PAR | one row per quiz | OPS-059, OPS-186 | A-32 |
| `GET /student/history/:quizID` | PAR | attempts for one quiz | OPS-059, OPS-186 | A-32 |
| `GET /quiz/:id` | PAR | intro/registration state | OPS-011, OPS-033 | A-32 |
| `POST /quiz/:id/register` | PAR | registration upsert | OPS-175 | A-19 |
| `POST /quiz/:id/cancel` | PAR | registration-only soft delete | OPS-023, OPS-122 | A-29 |
| `POST /quiz/:id/attempts` | PAR→ATT | PAR command, ATT start rule | OPS-110, OPS-162, OPS-176 | A-20 |
| `GET /attempt/:id` | ATT | owned live attempt, derived order | OPS-013, OPS-059 | A-37 |
| `POST /attempt/:id/answer` | ATT | slot upsert, selection set diff | OPS-111, OPS-160 | A-19, A-21 |
| `GET /attempt/:id/review` | ATT | pre-submit, mode-independent | OPS-013 | A-17, A-32 |
| `POST /attempt/:id/submit` | ATT | freeze, score objectives, final | OPS-112, OPS-132, OPS-163 | A-21 |
| `GET /attempt/:id/result` | ATT | post-submit review projection | OPS-133, OPS-059 | A-17 |
| `POST /attempt/:id/beacon` | ATT | progress/focus, CSRF-exempt | OPS-172, OPS-203, OPS-205 | A-31 |
| `GET /teacher` | ATT | aggregate; PAR registration contract | OPS-059, OPS-185 | A-32 |
| `GET /teacher/quizzes` | AUT | owner-scoped list | OPS-014, OPS-186 | A-37 |
| `GET /teacher/quizzes/new` | AUT | create form | OPS-011 | A-32 |
| `POST /teacher/quizzes` | AUT | create draft, code/token retry | OPS-140, OPS-141 | A-43 |
| `GET /teacher/quizzes/:id` | AUT | owner-only builder | OPS-014, OPS-059 | A-37 |
| `POST /teacher/quizzes/:id` | AUT | settings save, draft-only | OPS-126 | A-23 |
| `POST /teacher/quizzes/:id/publish` | AUT | draft → open, validate questions | OPS-114, OPS-164 | A-23, A-26 |
| `POST /teacher/quizzes/:id/close` | AUT | open → closed, stop stream | OPS-115, OPS-164, OPS-173 | A-23, A-30 |
| `POST /teacher/quizzes/:id/delete` | AUT | draft cascade + file cleanup | OPS-118, OPS-056, OPS-061 | A-27, A-44 |
| `GET /teacher/quizzes/:id/results` | ATT | owner-only results/recap path | OPS-014, OPS-059 | A-37 |
| `GET /teacher/quizzes/:id/questions/new` | AUT | draft-only editor | OPS-022 | A-32 |
| `POST /teacher/quizzes/:id/questions` | AUT | create + image write protocol | OPS-117, OPS-123 | A-25 |
| `GET /teacher/quizzes/:id/questions/:qid` | AUT | draft-only editor | OPS-022, OPS-059 | A-32 |
| `POST /teacher/quizzes/:id/questions/:qid` | AUT | update + replace image | OPS-117, OPS-123 | A-25 |
| `POST /teacher/quizzes/:id/questions/:qid/delete` | AUT | cascade options, renumber | OPS-119, OPS-057 | A-27 |
| `POST /teacher/quizzes/:id/questions/reorder` | AUT | full-set reorder, two-phase | OPS-116, OPS-161, OPS-166 | A-24 |
| `GET /teacher/quizzes/:id/monitor` | ATT | live snapshot page | OPS-173 | A-30 |
| `GET /teacher/quizzes/:id/stream` | ATT | SSE, owner+open | OPS-173, OPS-174, OPS-213 | A-30 |
| `GET /uploads/:name` | AUT | image read; OPS shares serving | OPS-123, OPS-212 | A-25, A-42 |
| `GET /teacher/grading` | ATT | queue, waiting duration | OPS-185, OPS-186 | A-22 |
| `GET /teacher/grading/:answerID` | ATT | one essay, owner-only | OPS-013, OPS-059 | A-37 |
| `POST /teacher/grading/:answerID` | ATT | write-once grade + final | OPS-113, OPS-142, OPS-132 | A-22 |
| `GET /teacher/recap/:attemptID` | ATT | owner-only recap | OPS-014, OPS-059 | A-37 |
| `GET /teacher/anonymous` | PAR | school-wide identity list | OPS-059, OPS-186 | A-32 |
| `GET /teacher/anonymous/:id` | PAR | edit form | OPS-013 | A-32 |
| `POST /teacher/anonymous/:id` | PAR | profile refresh, major lock | OPS-150, OPS-177 | A-14 |
| `POST /teacher/anonymous/:id/delete` | PAR | identity cascade | OPS-122, OPS-053 | A-29, A-44 |
| `GET /teacher/majors` | PAR | reference list | OPS-059 | A-32 |
| `POST /teacher/majors` | PAR | create; code derivation | OPS-124, OPS-150 | A-14 |
| `POST /teacher/majors/:id` | PAR | rename, no propagation | OPS-124, OPS-150 | A-14 |
| `POST /teacher/majors/:id/delete` | PAR | refuse while referenced | OPS-124, OPS-150 | A-14 |
| `GET /error/404` | OPS | preview; real 404 uses same page | OPS-204, OPS-217 | A-32 |
| `GET /error/500` | OPS | preview; real 500 uses same page | OPS-206, OPS-217 | A-32 |
| `GET /healthz` | OPS | liveness, not readiness | OPS-211 | A-04, A-34 |

**OPS-221** **No unowned route.** Every row has an owner and at least one acceptance case.
The cross-owned rows are deliberate and are the only places a route calls another domain:
`POST /quiz/:id/attempts` (PAR command, ATT start), `GET /student*` (PAR reads, ATT scores),
`GET /teacher` (ATT aggregate, PAR registrations), `GET /teacher/quizzes/:id/results`
(ATT read, AUT quiz ownership), `GET /uploads/:name` (AUT read, OPS serving/security), and
`GET /invite/:token` + `GET, POST /join` (PAR registration, AUT quiz lookup). Each owner's
spec must carry the command rules; this spec carries the seam only.

**OPS-222** Two surface facts are audited deliberately: `/healthz` is not in the 72-route pin
and must not be added to it (it is operational); `/uploads/:name` likewise sits outside the
pin but is a real GET route, so `conformance_test.go` must add it only when the backend phase
adds the handler. Until then, OPS-212 is the contract, not a claim that the route exists.

---

## 11. Operational prerequisites

**OPS-230** **Migrations.** Apply `0001`–`0009` with the one-shot runner (schema §9.3),
never as a boot hook. The runner takes `GET_LOCK('quiz.migrations', 30)` on a pinned
connection, applies files lexicographically, writes the ledger row only after every statement
in the file succeeds, and stops on the first error. The app starts even if the DB is down (the
advisory ping only warns), but it cannot serve data until the schema and seed exist. Verify
with `SELECT name FROM schema_migrations WHERE deleted_at = '1970-01-01 00:00:00' ORDER BY name`
returning the nine names. No migration is edited after being applied.

**OPS-231** **Rate limiter storage.** In-process, mutex-protected, bounded map with a sliding
window per key; evict idle keys every 5 minutes and cap the table (10,000 keys). Scopes: login
5/15 min per client IP; teacher/student registration 5/15 min per IP; reset-password 5/15 min
per IP; progress beacon 30/min and focus beacon 10/min per attempt; SSE connect 10 concurrent
per teacher+quiz. `Retry-After = ceil(seconds until a slot frees)`. Restart resets all counters;
a second process would double every allowance (OPS-243).

**OPS-232** **Configuration.** Required/meaningful environment:

| Variable | Default | Rule |
|---|---|---|
| `APP_ADDR` | `:8080` | listen address |
| `DB_DSN` | see OPS-233 | must include UTC + parseTime; no `multiStatements` |
| `COOKIE_SECURE` | `false` | `true` behind TLS; false only on the school LAN |
| `SESSION_SIGNING_KEY` | empty | required in production, ≥32 random bytes; signs CSRF, pending, anonymous, flash cookies |
| `UPLOADS_DIR` | `./uploads` | **config delta**, not schema; writable, same filesystem, outside `static/` |

The runner additionally reads `BOOTSTRAP_TEACHER_NAME`, `BOOTSTRAP_TEACHER_EMAIL`, and either
`BOOTSTRAP_TEACHER_PASSWORD` (hashed by the runner) or `BOOTSTRAP_TEACHER_PASSWORD_HASH`;
these are never read by the HTTP app. Seeding is mandatory: a missing value, a failed
migration, or a run that leaves no live teacher account exits non-zero, and the deployment
must not start the app after that failure.

**OPS-233** **DSN and server flags.** Use
`parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_unicode_ci&time_zone=%27%2B00%3A00%27&timeout=5s&readTimeout=10s&writeTimeout=10s`
with an explicit database name. MariaDB must run with
`--character-set-server=utf8mb4 --collation-server=utf8mb4_unicode_ci --default-time-zone=+00:00`
and `sql_mode=STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION`.
Pool: `MaxOpenConns=25`, `MaxIdleConns=25`, `ConnMaxLifetime=30m`, `ConnMaxIdleTime=5m`;
per-session `innodb_lock_wait_timeout=5`. Without strict mode every CHECK is decorative; without
`loc=UTC` every parsed time is off by the host offset.

**OPS-234** **Signing key and cookies.** `SESSION_SIGNING_KEY` signs the CSRF cookie, the
pending-teacher token, the PAR join ticket, and the flash cookie. It also derives PAR's
anonymous identity token from the join-ticket nonce (PAR-019) but does **not** sign the
`quiz_participant` cookie: that cookie is an opaque bearer value authenticated by exact
SHA-256 lookup (PAR-001–PAR-005, `I-07`). The key is not the session store (sessions are DB
rows by token hash). Session cookie: random 256-bit token, `HttpOnly`, `SameSite=Lax`,
`Path=/`, `Secure` per config, browser-session lifetime. CSRF: signed double-submit,
`HttpOnly`, 24 h, rotated on login/logout. Rotating the key invalidates pending/flash/CSRF
and every outstanding join ticket; live anonymous identity cookies stay valid because they
are hash-matched, while newly derived identities use the new key. Document this before
rotation. An empty, short, or published-default key is a startup failure; there is no
ephemeral fallback.

**OPS-235** **SSE hub lifecycle.** One process-wide registry `quizID → hub`; a hub is created
on first subscriber and destroyed on the last. Each subscriber has one buffered channel (16
events), one writer goroutine, and a write deadline; the hub has one 30 s heartbeat ticker, not
one per client. Connect order: validate owner/open → register subscriber → build snapshot →
send `snapshot` → drain events queued during the snapshot. Publish only after commit, with a
non-blocking send; a full queue evicts the subscriber (OPS-174). On quiz close: broadcast
`quiz.closed`, wait ≤1 s, close. On shutdown: close all hubs; do not fabricate a snapshot the
DB cannot support. No replay buffer and no cross-process fan-out.

**OPS-236** **Roster shrink has no `participant.removed` event.** R-20's event set cannot
express a cancellation or a student/anonymous delete while the monitor is open. Resolution
without a schema change and without inventing an event: after the delete/cancel commits,
broadcast a fresh **`snapshot`**. Clients must treat every `snapshot` as a full authoritative
replace, not only the first one. This is an ATT/frontend interface note, recorded in §14.

**OPS-237** **Uploads root.** Directory exists at boot, writable by the app UID (10001 in the
image), mode `0750`, files `0640`, same filesystem for atomic rename, outside the `/static`
tree, never symlinked. Docker: create `/app/uploads`, chown to the app user, mount a named
volume at `UPLOADS_DIR`; never bake user images into the image. On startup remove only
`*.tmp` files older than 24 h; never remove a `.jpg/.png/.webp` by age. Backups include the
directory; a restore of the DB without the files produces `upload_missing` 404s.

**OPS-238** **Bootstrap teacher.** Run the runner's `bootstrap-teacher` step once with the
three env values. It inserts one `role='teacher'`, `status='active'`, `approved_at=now`,
`must_change_password=1` account, idempotent on email. It is not a SQL migration: a committed
hash is a leaked credential and SQL cannot read the environment. Until it runs, teacher
registration stays pending and no approval is possible; the pending page says approval is not
available yet without revealing whether a bootstrap exists (R-03/R-24). The app never
promotes a teacher from the registration form.

**OPS-239** **Seed majors.** `0007` inserts TKJ/RPL/OTKP idempotently. It never runs
destructively; re-running after a major was renamed must not overwrite the name (the current
`ON DUPLICATE KEY UPDATE name = VALUES(name)` is safe only because a live duplicate is the same
row; a soft-deleted row is a different key). New majors are created through the app, never by
editing the seed.

**OPS-240** **Logging and observability.** JSON lines to stdout with request id, route
pattern, status, duration, actor id/`anon`, and relevant quiz/attempt ids. Log integrity faults
with the OPS rule ID and row ids: `upload_missing`, `upload_orphan`, `cascade_assert`,
`position_overflow`, `score_inconsistent`, `lock_timeout`. Never log passwords, password
hashes, cookies, CSRF values, raw session/reset/anonymous/pending tokens, essay text, emails,
full names, or image bytes (OPS-INV-10). Panic recovery logs the stack server-side and returns
500.

**OPS-241** **Minimum alerts.** 5xx rate, p95 latency, DB pool saturation, lock-timeout count,
`upload_missing` count, `cascade_assert` count, SSE subscriber count per quiz, rate-limit 429
rate, and failed advisory pings. A `cascade_assert` or `upload_missing` alert is a data
integrity page, not a performance warning.

**OPS-242** **Backups.** Take `mariadb-dump --single-transaction` and a filesystem snapshot of
the uploads root at the same point; restore both together. Soft-deleted rows are included.
There is no PITR requirement and no down-migration; rollback is a backup restore (schema
§9.6). Test a restore into a scratch database before relying on it.

**OPS-243** **Single-process consequences.** The limiter and SSE hub are memory-resident. Two
replicas would (a) double every rate allowance, (b) deliver SSE events only from the replica
that wrote, and (c) make `Retry-After`/connection caps inconsistent. Sticky sessions do not fix
(b) because writes can land anywhere. Horizontal scaling requires a shared limiter and a
pub/sub bus; that is out of scope for v1. The compose deployment is one app container.

**OPS-244** **Startup checks.** Fatal: empty signing key in production, missing/unwritable
uploads root, session time zone not `+00:00`, invalid DSN syntax. Non-fatal: DB unreachable
(advisory ping warns; `/healthz` reports 503), missing migrations (log an error; requests will
fail until applied), SSE hub empty. This matches the current `main.go` behaviour for DB uptime.

**OPS-245** **Image/compose changes required by this spec.** Add `UPLOADS_DIR=/app/uploads`,
create and chown `/app/uploads` in the runtime stage, mount a volume, add the MariaDB server
flags and the UTC DSN to compose, and keep the healthcheck on `/healthz`. Do not copy `docs/`,
`assets/`, or `*.md` into the image. The app user stays non-root UID 10001.

**OPS-246** **Probes.** `/healthz` is liveness for the compose healthcheck; it is not a
readiness gate for migrations or uploads. A deploy that starts with a broken uploads root or a
missing schema fails at the first user action, not at the probe; startup logs and OPS-241
alerts cover those cases.

**OPS-247** **Static and dependency posture.** Bootstrap 6, jQuery 4, and fonts stay vendored;
no CDN, no Node build. `/static` keeps the one-day cache. MariaDB is 12.3 and Go is 1.26.4;
the limiter and SSE hub are stdlib-only, no new dependency. `static/js/app.js` remains free of
`fetch`/`XHR`; only the documented `EventSource` and `sendBeacon` paths are allowed in the
backend phase.

**OPS-248** **Operator runbook for the three integrity faults.** (1) `upload_orphan`: list the
root, subtract live `questions.image_path`, delete files older than 24 h, log each. (2)
`upload_missing`: never fabricate a file; hide/repair the question through the normal editor
(replace image) and record the affected question ids. (3) `cascade_assert`: stop writes to the
affected root, snapshot the DB, inspect for live descendants, restore rows only through the
tested restore procedure — never “fix” by hard-deleting children.

---

## 12. Cross-domain contracts

**OPS-250** **Form of a contract.** Each subsection states what OPS guarantees to a sibling
and what OPS requires of it. A sibling rule that does not meet a "requires" line is a spec
defect, not an implementation choice. No command workflow is restated: the citation names the
owning rule and the route anchor.

**OPS-251** **To `IDN`.** OPS guarantees: session insert/rotate/slide/revoke (`OPS-125`,
`OPS-170`, `OPS-171`, `OPS-183`); CSRF sign/verify and the 403 copy (`OPS-203`); the pending
token state machine (`OPS-030`); reset issue/consume (`OPS-108`, `OPS-109`, `OPS-168`); actor
re-read under the write transaction (`OPS-013`); login/logout clearing of PAR's two cookies
(`OPS-125`); rate-limit buckets (`OPS-231`); the 404/403 split (`OPS-204`). OPS requires: every
account read filters the live sentinel; the five roots
set `deleted_by_account_id` and nothing else does (`OPS-052`); no teacher is ever soft-deleted
(`OPS-055`); all timestamps come from T, not `NOW()`; every POST — including idempotent
approve, logout, and reset — ends 303 or a taxonomy status, never 200.

**OPS-252** **To `AUT`.** OPS guarantees: the image write/serve protocol (`OPS-123`,
`OPS-212`); the draft-only editor guard (`OPS-022`, `OPS-117`); publish/close serialisation
(`OPS-114`, `OPS-115`); the two-phase reorder (`OPS-161`); draft cascades (`OPS-118`,
`OPS-119`); code/token generation retry (`OPS-140`, `OPS-141`). OPS requires: INV-11 and
INV-24 are validated before the write; `state` is written only by publish and close; positions
are never written as 0 or duplicated among live rows; deletion goes through the cascade helper,
never a hand-written `UPDATE`; the upload namer and MIME sniffer are the shared ones; the
client-supplied MIME and filename are never trusted.

**OPS-253** **To `PAR`.** OPS guarantees: registration idempotency by natural key
(`OPS-175`); the invite and identify transactions (`OPS-127`, `OPS-128`); read-only code
resolution (`OPS-129`); registration/identity cascades (`OPS-053`, `OPS-122`); the major
reference protocol (`OPS-124`, `OPS-150`); cookie-hash identity (`OPS-128`, INV-27); the
attempt-start lock set (`OPS-110`). OPS requires: eligibility is evaluated against the quiz and
the current profile under the lock — including the profile lock on invite auto-registration so
a deleted student cannot be resurrected; an existing live registration is success, not an
error; anonymous identities are never merged by display name; `majors.code` follows the
derivation in §14 D-01; cancel is allowed only on an open quiz with no live attempt.

**OPS-254** **To `ATT`.** OPS guarantees: attempt start/submit/grade transactions
(`OPS-110`, `OPS-112`, `OPS-113`); one scoring implementation (`OPS-130`–`OPS-134`); grading
write-once (`OPS-142`); the SSE hub and its lifecycle (`OPS-173`, `OPS-174`, `OPS-235`); beacon
semantics (`OPS-172`). OPS requires: ATT's start rule runs inside OPS-110's transaction and
ATT-280's "commit before calling" is read per `Q-06`; submit takes the registration lock before
the attempt (`OPS-112`, PAR-203); no score or final-score write outside submit and grade; no
answer-body write after submit; review output remains a pure function (INV-30); every publish
to the hub happens after commit; the teacher dashboard reads PAR's registration contract
instead of re-deriving participation state.

**OPS-255** **To the frontend contract.** OPS guarantees: the exact status set (`OPS-011`,
`OPS-200`); relative 303 (never 302/201) (`OPS-201`); 422 re-render with `FieldErrors` keyed by
frontend §18 names (`OPS-202`); one-shot flash (`OPS-210`); `Retry-After` on 429 (`OPS-205`);
every `snapshot` is a full replace, including after a roster shrink (`OPS-236`). OPS requires:
forms carry `csrf_token` and the exact field names; `EventSource` is used only against
`/stream`; `sendBeacon` only against `/attempt/:id/beacon`; no client assumes a repeated POST
creates a second row; no client treats a 404 as "retry with another id".

**OPS-256** **To schema and migrations.** OPS guarantees: no DDL from application code
(`OPS-005`); a UTC session and DSN (`OPS-180`, `OPS-190`); sentinel-based reads (`OPS-059`);
recreation after delete (`OPS-058`); one cascade timestamp (`OPS-051`). OPS requires: new
tables keep the `deleted_at` sentinel and put `deleted_at` in every unique key; foreign keys
stay `RESTRICT`; no hard delete and no restore route; only the runner writes
`schema_migrations`; a schema change lands before the code that reads it.

**OPS-257** **Cross-owned seams.** These are the only routes whose command belongs to one
domain while another supplies data or serving:

| Seam | Caller | Callee | OPS rule | Failure response |
|---|---|---|---|---|
| invite landing auto-registers | PAR | AUT quiz lookup | OPS-127 | `not_open` page, no write |
| join code resolves a quiz | PAR | AUT quiz lookup | OPS-129 | 303 `/join` error state |
| start attempt | PAR | ATT start rule | OPS-110 | 303 intro with flash |
| history/dashboard scores | PAR | ATT score contract | OPS-132, OPS-133 | `-` for NULL, no zero |
| teacher dashboard roster | ATT | PAR registration contract | OPS-059 | empty state, not error |
| results/recap quiz header | ATT | AUT quiz row | OPS-014 | 404 on non-owner |
| image serving | AUT | OPS upload serving | OPS-212 | 404, never a listing |

**OPS-258** **One implementation per cross-cutting concern.** There is one transaction
wrapper, one cascade stamping helper, one clock read, one score function, one upload writer and
server, one owner predicate, one error renderer, one rate limiter, and one SSE hub. A second
implementation — even a "small" one — is a review blocker: it will drift on T, status, live
filtering, or lock order, and the drift is exactly what this document exists to prevent.

**OPS-259** **Change protocol.** Adding a route, an invariant, a status, or a schema delta
requires, in one change: a contract amendment if the surface changes; a §10 row; a §4 row for
a new invariant; one executable A-case in §15; and the corresponding conformance pin update.
A rule may not be added only in a sibling spec and assumed here.

---

## 13. Traceability

**OPS-260** The traceability chain is fixed: **route** (§10 row) → **owning domain rule**
(contract §6 / sibling prefix) → **OPS enforcement rule** (§3–§9) → **acceptance case** (§15).
A row without all four is incomplete; a rule cited by no row is either operational (§11) or
dead and must be removed.

**OPS-261** Coverage is auditable by count:

| Surface | Count | Evidence |
|---|---|---|
| method+path routes | 74 = 72 pinned + `/healthz` + `/uploads/:name` | §10 table, OPS-220 |
| contract invariants | 30 (`INV-01`–`INV-30`) | §4 matrix |
| OPS invariants | 12 (`OPS-INV-01`–`OPS-INV-12`) | §4 matrix |
| command flows with a lock set | 25 (`OPS-105`–`OPS-129`) | §6 |
| state machines | 12 (`OPS-020`–`OPS-031`) | §3 |
| acceptance cases | 46 (`A-01`–`A-46`) | §15 |

**OPS-262** **Mechanical audit.** Before calling this spec complete, run: (1) extract
`OPS-[0-9]{3}` definitions and references and diff the sets — every reference resolves to a
definition and no definition is unreachable; (2) extract `A-[0-9]{2}` references and assert
each is defined in §15; (3) count §10's method cells and assert 74 after expanding
`GET, POST`; (4) assert every `INV-` row in §4 names an enforcement layer and at least one
A-case. These four checks are the review gate; a human reading the prose is not a substitute.

**OPS-263** **Deliberately not traced here.** `docs/DESIGN.md` (visual contract), the
vendored Bootstrap/jQuery behaviour, and the client-side validation attributes are not
business rules. Their failures are UI defects, not OPS violations, unless they change a status
code, a stored value, or an invariant named in §4.

---

## 14. Open questions and schema deltas

`OPS-005` forbids requiring a schema change. The register below therefore contains only
resolutions that fit the nine migrations, config, interface, or operations. Each item names
the owner that must adopt it.

### 14.1 Register

| ID | Gap | Recommended resolution | Impact |
|---|---|---|---|
| D-01 | `majors.code` is `NOT NULL` and unique, but the create form sends only `name` (frontend §16.3). | Derive at create: uppercase ASCII letters/digits; split into words and drop the connectors `dan`, `di`, `ke`, `dari`, `untuk`, `&`; if ≥2 content words remain take the initial of up to 4, else the first 4 characters; empty → `MJ`. On 1062 append `-` plus 2 base36 characters from a random 16-bit value; retry 5 times, then 500. Rename never recomputes the code. | PAR create rule; codes are stable but not human-editable; no DDL. |
| D-02 | No uploads location exists in config or compose. | `UPLOADS_DIR`, dev default `./uploads`, image `/app/uploads`; create `0750`, files `0640`, named volume, app UID 10001. | `internal/config`, Dockerfile, compose, backups (OPS-237/245). |
| D-03 | No flash table. | Signed one-shot `HttpOnly` cookie, ≤2 KB, cleared on read; flash loss on cookie clear is acceptable. | OPS-210; no DDL. |
| D-04 | No shared limiter or pub/sub. | In-process limiter and SSE hub; one app replica only. | OPS-231/235/243; horizontal scaling requires a new design. |
| D-05 | Objective grading must satisfy `chk_answers_grading`. | System scoring writes `graded_at=T`, `graded_by_account_id=NULL`; "ungraded" is `score IS NULL`, never `graded_at IS NULL` alone. | ATT queries and queue must not filter on grader NULL. |
| D-06 | No `revoked_at` on reset tokens. | Revoke prior live tokens by stamping `deleted_at=T`, `deleted_by=NULL`; the UI says the earlier link is no longer valid. | IDN issue/consume and audit copy (OPS-108). |
| D-07 | No `participant.removed` SSE event. | Resend a full `snapshot` after any roster shrink; clients replace the roster on every snapshot. | ATT hub + frontend (OPS-236). |
| D-08 | `answer_text` is `TEXT` (65,535 bytes) with no form cap. | Cap at 10,000 characters and 60,000 UTF-8 bytes; trim and CRLF→LF; 422 copy `Jawaban terlalu panjang (maksimal 10.000 karakter).` | ATT answer form; prevents DB truncation 500. |
| D-09 | No policy for anonymous identity lifetime. | 365 days, sliding on identify/join, `HttpOnly`, `SameSite=Lax`, `Secure` per config; clearing the cookie deliberately creates a new identity. | OPS-191; long-lived pseudonymous rows retained indefinitely. |
| D-10 | No title/name snapshots. | Title is immutable after publish because settings are draft-only; display names and major names are read live, not snapshotted. | Historical results show current names; accepted by R-25. |
| D-11 | No upload cleanup queue. | Orphans are tolerated; startup removes only `*.tmp` older than 24 h; operator runbook deletes aged orphans. | OPS-237/248; disk growth needs the alert in OPS-241. |
| D-12 | No optimistic version column. | Row locks plus last-write-wins; no `If-Match`/ETag; concurrent settings saves overwrite. | OPS-126/165; acceptable for one teacher per quiz. |
| D-13 | `/uploads/:name` and `/healthz` are outside the 72-route pin. | Keep them out until the backend phase adds the upload handler; then add `/uploads/:name` to the conformance pin, never `/healthz`. | `conformance_test.go` change is a deliberate surface change. |
| D-14 | DB session time zone is not pinned by schema. | Server/session `time_zone='+00:00'`; DSN `parseTime=true&loc=UTC`; startup asserts both. | Compose flags, OPS-190/233/245. |

### 14.2 Open questions

**Q-01 — Anonymous cookie lifetime.** Keep 365 days (`OPS-191`). A shorter value loses
identity on returning students and creates duplicate rows; a longer value adds no capability.
Impact: pseudonymous data lives as long as the cookie; retention already keeps rows forever.

**Q-02 — Session slide on every request.** Keep the write (`OPS-170`), no throttle in v1. A
throttle saves one indexed PK update but makes the 60-minute boundary fuzzy under bursts.
Impact: one extra UPDATE per authenticated request; revisit if p95 or DB writes become the
bottleneck.

**Q-03 — Essay cap value.** Keep 10,000 characters / 60,000 bytes (§14 D-08). Lower values
reject legitimate long answers; higher values approach the `TEXT` byte limit with multibyte
input. Impact: ATT must enforce both counts before the transaction.

**Q-04 — Major code stability.** Keep the create-time code and never recompute on rename
(§14 D-01). Recomputing would break any external reference and can collide with the seed.
Impact: a renamed major keeps its old code; codes are identifiers, not labels.

**Q-05 — Invite GET writes.** Keep auto-registration on `GET /invite/:token` (`OPS-127`)
because R-12 requires it. `SameSite=Lax` blocks the cross-site subresource case; a top-level
link only registers the viewer, idempotently by `uq_reg_student`. Impact: crawlers without a
session cannot register anyone; adding a confirmation POST would contradict R-12's "registers
immediately".

**Q-06 — Start atomicity vs ATT-280.** Keep one transaction for `OPS-110` (student/quiz shared,
registration exclusive, attempt + slots inserted) and read ATT-280's "commit the registration
before calling ATT-101" as "the registration is committed by the time the start request ends",
not "commit, then start a second transaction". `PAR-202` and `OPS-110` are authoritative for
the boundary. ATT's start rule must therefore run on the caller's transaction/connection; if
implementation makes it an independent committing call, the failure paths differ and the two
specs must be reconciled before coding. Impact: a failed attempt insert rolls back the whole
start, while a registration created by an earlier invite/identify remains; no attempt or slot
survives. Covered by A-20/A-29.

### 14.3 Ruling objections

**None.** R-01–R-25 are implemented as written. The following are interpretations required by
the schema, recorded so a later audit does not mistake them for amendments:

- **I-01 (R-07).** An identical retry of a grade is the same write, not a second grading
  (`OPS-142`); a different value is refused because v1 has no re-grade.
- **I-02 (R-05/R-14).** The schema cascade for a deleted teacher is unreachable: the app never
  deletes a teacher, so no ownerless quiz can exist (`OPS-055`).
- **I-03 (R-10).** Draft quiz/question delete asserts zero live registrations and attempts
  before stamping, even though the schema would permit a wider cascade (`OPS-056`).
- **I-04 (R-20).** Resending `snapshot` after a roster shrink is not a new event type; the
  event set is unchanged (`OPS-236`).
- **I-05 (R-04).** Reset-consume clears `must_change_password` because the student chose the
  password; only bootstrap sets it (`OPS-109`, `OPS-238`).
- **I-06 (R-11).** Publish does not mint a code; the public code exists from creation and a
  private row is NULL (`OPS-114`).
- **I-07 (R-02).** ATT-287 says IDN "owns the anonymous identity cookie". Read with IDN-157
  and PAR-001–PAR-005, that means mechanism, not identity semantics: IDN owns the shared
  signing key and generic cookie handling, while PAR owns the cookie name, value shape,
  issuance, refresh, clearing, and hash-to-row lookup. The `quiz_participant` cookie is not
  signed (`OPS-234`); ATT and IDN never read it or resolve an anonymous row from it. There is
  one identity store and one owner of its meaning.

---

## 15. Acceptance criteria

Each case is executable as written; `A-` cases are the review gate from `OPS-262`. Fault
injection is explicit where the failure is not reachable through the UI.

| ID | Executable statement |
|---|---|
| A-01 | Run `0001`–`0009` twice on an empty database: the second run changes nothing, `schema_migrations` has exactly nine live rows with unique names, and a deliberately broken file leaves no ledger row. |
| A-02 | Extract `INV-01`–`INV-30` and `OPS-INV-01`–`OPS-INV-12`; each appears once as a §4 row with an enforcement layer and at least one A-case; the check fails on a missing or duplicate ID. |
| A-03 | With the process in Asia/Jakarta and the DB session at UTC, create a quiz at 23:30 UTC: the row stores 23:30, the page renders 06:30 WIB next day, `@@session.time_zone` is `+00:00`, and no read differs by 7 h. |
| A-04 | `GET /healthz` with the DB up returns 200, body exactly `ok`, `text/plain`, no session or CSRF requirement, within 2 s. |
| A-05 | Missing, soft-deleted, other-owner, and wrong-role IDs all return 404 with the same error template and no resource name; an unauthenticated protected GET returns 303 `/login`; ownership never returns 403. |
| A-06 | Every POST without `csrf_token` returns 403 with `Sesi formulir tidak valid. Muat ulang halaman.`; a form token that does not match the signed cookie, or an expired token, returns 403; a beacon without same-origin `Origin`/`Referer` returns 403; a same-origin beacon returns 204. |
| A-07 | A successful POST returns 303 with a relative `Location`; its flash appears once on the next GET and not after refresh; replaying the POST does not duplicate the row; no POST returns 200, 201, or 302. |
| A-08 | A status-matrix test asserts POST outcomes ∈ {303, 422, 403, 404, 429, 500}, beacon ∈ {204, 403, 404, 422, 429, 500}, HTML GET ∈ {200, 303, 404, 429, 500}, uploads may 304; `/stream` is 200 only after headers; no 405 or 409 page exists. |
| A-09 | Two concurrent identical POSTs sharing a CSRF cookie (answer save, grade, approve) produce one logical write, both 303, and no duplicate live row; the loser is idempotent or a state conflict, never 500. Double delete follows A-44. |
| A-10 | Delete a student with registrations in two quizzes plus attempts, answers, selections, sessions, reset tokens, and focus rows: every descendant `deleted_at` equals one T, zero live descendants remain, and only the account row carries `deleted_by_account_id`. |
| A-11 | Inject a failure after the first cascade UPDATE: the transaction rolls back, no row is stamped, no file is unlinked; a retry succeeds and stamps one T for the whole cascade. |
| A-12 | Grep the route table and store helpers: no teacher-delete route or helper exists; a direct attempt to stamp a `role='teacher'` account is refused; teacher rows keep the live sentinel after every app flow. |
| A-13 | Soft-delete then recreate a student email, a major name/code, a public quiz code, and a registration: all succeed as new rows, old rows remain, and aggregates count only the new live row. |
| A-14 | Create a major with only a name: it receives a non-empty code ≤10, unique among live rows; rename changes only `name`; delete is refused for each of live student, anonymous participant, and quiz target; delete vs concurrent create serializes; student/anon edits join the current name. |
| A-15 | Duplicate live email → 422 `Email sudah terdaftar.`; reuse after soft delete succeeds; student + profile commit atomically; teacher registration stays pending; profile email is trimmed and lowercased; a concurrent duplicate race yields one success 303 and one 422. |
| A-16 | Approve a pending teacher: status `active`, `approved_by` = actor, `approved_at` = T, no session created; double approve is a 303 informational no-op; student/soft-deleted/missing ids → 404; self-approval is impossible because pending accounts cannot authenticate. |
| A-17 | Issue a reset token twice: the first link is revoked, the second works once, expiry is exactly 24 h, reuse → 422, wrong role/deleted student → 422. The pending page renders waiting/approved/expired from the signed token without consuming it. Separately, a hidden score renders `-` (not `0%`) on result/history/recap, and `review_mode` controls body/key visibility. |
| A-18 | Login rotates or creates a session; logout, reset-consume, and change-password revoke the right sessions; expired/revoked/deleted-account cookies get 303 `/login`; login and logout expire `quiz_participant` and `quiz_join_ticket` with `Max-Age=0`; a write past middleware but after a cascade fails its actor re-read with 404 and writes nothing. |
| A-19 | Save the same selection set twice → one live row per (answer, option); removing an option stamps the removed row with T and inserts the added one; clearing all stamps all; save after submit writes nothing and returns the state conflict; a cancelled registration cannot save. |
| A-20 | Two concurrent starts on one registration produce exactly one live in-progress attempt; both land on its id; `attempt_no` is sequential; an attempt beyond `max_attempts` (never more than 10) is refused; a start on a closed quiz writes nothing; a forced failure after the registration lock rolls back the attempt and all slots without touching the registration or the quiz. |
| A-21 | Score vectors: single exact = 1, wrong/extra/missing = 0; multiple exact = 1, subset/superset = 0; essay NULL until graded; final NULL until all scored; final = floor((2S+N)/(2N)) at 4 dp; recomputation from slots equals the stored value; no override route exists. |
| A-22 | Grade an essay once: score stored, feedback ≤1000, grader = actor, `graded_at` = T, final set when last; byte-identical retry is 303 success with no write; different retry is 303 conflict `Jawaban ini sudah dinilai.`; non-owner 404; objective slot 404/422; grading before submit writes nothing. |
| A-23 | Settings save only in draft; publish requires ≥1 valid question and private targets; publish sets `published_at`, close sets `closed_at`; publish→close and close→publish races serialize; no code is minted at publish. |
| A-24 | Reorder accepts only the complete live id set; partial/stale/foreign ids → 422 with no write; positions become dense 1..N; concurrent reorders serialize; the two-phase rewrite raises no unique-key collision. |
| A-25 | Upload a 10 MiB JPEG: the file exists before commit and the row references it; wrong MIME/magic, >10 MiB, or a partial tuple → 422 with no row or file; replace unlinks the old file only after commit; a DB failure after write triggers a best-effort unlink, while a kill leaves an orphan and no row. |
| A-26 | Publish with zero questions, a single-choice question with zero/two keys, a multiple-choice with one key/option, or an essay with options → 422 and no state change; a valid draft publishes. |
| A-27 | Delete a draft quiz: questions/options stamped with T, images unlinked after commit, zero live children; delete a question: options stamped and later positions renumbered dense; no attempt/answer row is deleted; a closed quiz delete is refused. |
| A-28 | Delete a student with data: sessions, tokens, registrations, attempts, answers, selections, and focus rows all carry one T; zero live descendants; roster/history counts drop; login and reset fail; teacher-owned quiz rows are untouched. |
| A-29 | Cancel is allowed only while open and attempt-free; after cancel the intro offers Daftar again; anonymous identity delete cascades its registrations/attempts but leaves other identities; a concurrent start vs delete yields either a live attempt or a deleted registration, never a live attempt under a deleted registration. |
| A-30 | SSE: owner + open gets a snapshot then events; non-owner/draft/closed → 404; close sends `quiz.closed` and ends; disconnect frees the subscriber; the 11th connection gets 429; a slow client is evicted and reconnects to a fresh snapshot; roster shrink sends a fresh snapshot. |
| A-31 | Beacon progress is idempotent and coalesced to ≤1 write/s; focus inserts one row per accepted event with server T; the 31st progress event/minute → 429 with an integer `Retry-After`; after submit progress is 204; cross-origin is 403; >4 KB is 422. |
| A-32 | Every GET page in §10 renders 200 from zero values and from populated fixtures; `/error/404` and `/error/500` previews return 200; `/` returns 303; no panic, `<no value>`, or missing partial. |
| A-33 | Perform login, reset issue/consume, essay grading, and an image upload: stdout logs contain no password, hash, cookie, CSRF value, raw token, email, full name, essay text, or image bytes; every line carries a request id. |
| A-34 | Stop MariaDB: `/healthz` returns 503 `db unavailable` within 2 s, the process stays up, and the body has no stack trace; start MariaDB: 200 `ok`; an in-flight query fails 500 rather than hanging. |
| A-35 | Kill the process between file write and commit: the file is an orphan, the row is absent; startup does not delete it; a later save works; the OPS-248 listing finds it. |
| A-36 | Delete a committed question image from disk: `/uploads/:name` returns 404, the page renders without a broken-image 500, `upload_missing` is logged with the row id, and the editor can replace the image. |
| A-37 | A second teacher probes every owner-scoped quiz route (builder, questions, publish/close/delete, results, recap, grading, monitor, stream) with the first teacher's ids → 404; a student probing `/teacher/*` → 404; an anonymous participant probes another's attempt → 404; no body exposes owner name, email, or quiz title; school-wide student/anonymous/major/teacher lists still return 200. |
| A-38 | Exceed the login bucket from one IP → 429 with `Retry-After`; the same credentials from another IP still work; a retry after the window succeeds; the 11th SSE connection is 429; beacon buckets are independent per attempt+kind. |
| A-39 | Seed expired sessions and tokens, run the sweep: only rows with `expires_at <= T` are stamped, in bounded batches; live rows and all other tables are untouched; startup and hourly runs are idempotent. |
| A-40 | Hold a quiz row in another transaction: publish waits ≤5 s, then 500 with `lock_timeout` and no partial write; a forced deadlock rolls back one transaction while the other commits; the victim retry succeeds. |
| A-41 | Start with an empty signing key in production → fatal before listen; missing/unwritable uploads dir → fatal; non-UTC session tz → fatal; DB down → warn, process serves `/healthz` 503; no secret appears in logs. |
| A-42 | `/uploads/../…`, encoded traversal, absolute path, NUL, unknown extension, symlink, and a 31/33-character name all return 404; a valid 32-hex JPEG returns 200 with `nosniff` and no directory listing; `.tmp` is never served. |
| A-43 | Force `uq_quizzes_code`/invite-token collisions ten times during create: the app retries with fresh values and succeeds when a unique value is available; with every attempt forced to collide it returns 500 and leaves no row; a private quiz stores `code = NULL`. |
| A-44 | Fire two concurrent deletes on one quiz/question/student: one stamps the cascade, the other returns 404 per OPS-165; files are unlinked once or not at all; no 500 and no duplicate stamps. |
| A-45 | Grep the route table and SQL: no delete/restore route and no `DELETE FROM` on domain tables; physical delete of a referenced parent fails with FK 1451; no UI action exposes restore. |
| A-46 | `POST /join` with a valid open public code sends a student to `/quiz/:id` without creating a registration; a visitor gets `/join/identify` with a signed cookie; `/invite` auto-registers an eligible student exactly once, no-ops for a visitor, and shows `not_open` for draft/closed; identify creates one cookie-hash identity and one registration even on double submit; a private code is `unknown_code` for a student and `anonymous_not_allowed` for a visitor; a draft code is 404; a closed code is `not_open`. |

---

*End of spec. The five domain documents are independent except for the citations above; this
document never amends the contract, the schema, or a sibling's command.*

