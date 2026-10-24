# Business Logic Specifications — Integration Contract

**Date:** 2026-10-23
**Status:** binding for the five domain specs; product calls flagged for review
**Scope:** the shared contract the five business-logic specs implement. This is
not a domain spec. It fixes authority, cross-domain rulings, invariants, route
ownership, and the required shape of each domain document.

---

## 1. Source precedence

When two sources disagree, the higher row wins:

| Rank | Source | Wins on |
|---|---|---|
| 1 | `migrations/*.up.sql`, `internal/model/*.go`, `2026-10-23-schema-and-migrations-design.md` | data shape, constraints, storage semantics |
| 2 | `2026-10-23-frontend-data-contract-design.md` §4–§19, §22–§24 | screens, routes, view models, forms, copy, product decisions |
| 3 | `2026-10-20-quiz-system-design.md` §1–§10, §12–§14 | business rules not superseded by rank 2 |
| 4 | `docs/DESIGN.md` | visual system only — never business logic |

`2026-10-20-quiz-system-design.md` §11 is superseded and must not be implemented.
Where rank 2 is silent, rank 3 supplies the rule. Where rank 2 and rank 3
conflict, rank 2 wins. Where either conflicts with rank 1, rank 1 wins and the
conflict is recorded in the owning spec's §11.

No domain spec may re-litigate a ruling in §3. If an agent believes a ruling is
wrong, it implements the ruling and records the objection in its §11.

---

## 2. The five domains

| # | Domain | Owns | File |
|---|---|---|---|
| 1 | Identity and access | accounts, students, sessions, password reset, approval, login/logout, profile, CSRF, rate limits, teacher/student management, authorization matrix | `2026-10-23-business-logic-01-identity-and-access.md` |
| 2 | Authoring and lifecycle | quizzes, questions, options, images, create/edit/publish/close/delete, invite token and code generation, reorder, ownership | `2026-10-23-business-logic-02-authoring-and-lifecycle.md` |
| 3 | Participation and eligibility | anonymous identity, join/invite, registrations, cancellation, eligibility, student dashboard/history, majors, anonymous management | `2026-10-23-business-logic-03-participation-and-eligibility.md` |
| 4 | Attempts, grading, results | attempt start/resume, shuffle, answer save, submit, objective scoring, essay grading, final score, review modes, result/history data, monitor/SSE, beacons, teacher dashboard | `2026-10-23-business-logic-04-attempts-grading-and-results.md` |
| 5 | Integrity and operations | invariants, DB-vs-app enforcement, soft-delete cascades, transactions, concurrency, idempotency, time, retention, error taxonomy, route coverage, schema-delta register | `2026-10-23-business-logic-05-integrity-and-operations.md` |

Each domain spec owns the commands for its routes. A command that crosses
domains calls the owning domain's rule by ID; it does not redefine it.

---

## 3. Binding rulings

### R-01 — Authority and immutability of the schema

The schema is fixed. Domain specs may propose a delta only in their §11, with:
the table/column, the rule that cannot be expressed, and the smallest change.
No one edits `migrations/` or `internal/model/`. `schema_migrations` and the
runner are out of scope.

### R-02 — Anonymous identity is the cookie, not the name

An anonymous participant is one row per browser identity. `cookie_token_hash`
is the identity; `display_name`, `grade_level`, and `major_id` are mutable
profile data refreshed on each join. Two people with the same name are two
rows. Name-based lookup is forbidden.

Retakes reuse the registration through the same identity cookie. Clearing
cookies creates a new identity and a fresh retake allowance; this is accepted
and documented. This supersedes the contract §9.7 sentence "the full name
entered on the same quiz is what identifies the returning participant" and the
old spec's name grouping. The schema spec §6.4 note is authoritative.

Cancelling an anonymous registration deletes only that registration; the
identity row and its other registrations stay. This supersedes contract §10.4's
"clears the identity cookie binding for that quiz", which the schema cannot
represent.

### R-03 — First teacher

The registration form never auto-activates a teacher. The bootstrap teacher is
a runner step (out of app scope) that inserts one active teacher with
`must_change_password = 1`. Until that step has run, teacher registrations stay
pending and no one can approve them; the pending page must say so without
leaking whether a bootstrap exists. After bootstrap, any active teacher may
approve any pending teacher. No self-approval is possible because pending
accounts cannot authenticate.

### R-04 — `must_change_password`

Only the bootstrap teacher starts with `1`. On successful login with `1`, a
session is created but every authenticated route except `GET/POST
/change-password` and `POST /logout` redirects to `/change-password` with an
explanatory flash. A successful change clears the flag.

Teacher-issued password resets set `0`: the student chooses the new password on
the reset page, so forcing another change is redundant. The schema spec's
"1 for resets and bootstrap" is read as "1 for bootstrap and any future
admin-set temporary password"; the token flow is not a temporary password.
Flagged for review.

### R-05 — Teacher ownership and school-wide management

Quizzes are private to `owner_account_id`. Quiz list, dashboard stats,
authoring, publish/close/delete, results, recap, grading, monitor, and SSE are
scoped to the signed-in owner. Another teacher receives 404, never 403, so quiz
existence does not leak.

Students, anonymous participants, majors, and the teacher directory are
school-wide: any active teacher may view/manage them, and any active teacher may
approve a pending teacher. Teachers are never edited, deactivated, or deleted;
their name and email are fixed at registration. There is no teacher profile
screen; teachers may change only their password. Contract §10.3 is student-only.
Flagged for review: a teacher typo in name/email cannot be corrected in v1.

### R-06 — Registration and attempt quota

Registration is a first-class row; every attempt belongs to one registration.
States are derived, never stored:

| State | Definition |
|---|---|
| `registered` | no live attempts |
| `in_progress` | at least one live attempt with `submitted_at IS NULL` |
| `completed` | at least one live attempt, and none in progress |

`completed` means submitted, whether or not essays are graded. At most one
in-progress attempt exists per registration; starting while one exists resumes
it. `max_attempts` is the total allowed, including the first. `allow_retake =
false` forces `max_attempts = 1` on every write. `allow_retake = true` accepts
1–10; 0 is never written even though the DB check allows it. Attempt number is
`1 + count(live attempts)`. Every attempt is stored; there is no `counted` flag;
the best score is the maximum visible `final_score`.

### R-07 — Attempt state and score

`in_progress` = `submitted_at IS NULL`; `completed` = `submitted_at IS NOT
NULL`; `graded` = `final_score IS NOT NULL`. Start time is `created_at`. There
is no status column and no `started_at`.

`final_score` is the arithmetic mean of the per-question `score` values, on
0–1 with four decimals, rounded half-up; it is `NULL` until every question has
a score. Objective answers score exactly 1 or 0 at submit; essays score 0–1 when
the teacher grades them. There is no manual final-score override and no
re-grade route in v1. A blank objective answer scores 0; a blank essay is
graded like any other essay.

### R-08 — Deterministic shuffle

`shuffle_seed` is a crypto-random `uint32` generated once per attempt. When a
toggle is on, order is derived, never stored:

```
key(item) = SHA-256( decimal(seed) + "|" + namespace + "|" + decimal(item_id) )
namespace = "q" for questions, "o" for answer options
order = ascending key bytes, tie-break ascending item_id
```

When a toggle is off, order is `position` ascending. Displayed question
positions, option letters (A, B, C…), and `QuestionNav` all derive from the
same function on every read. The seed never changes. This applies the schema
spec §6.9 note "seed, not snapshot".

### R-09 — Answer rows, no autosave

At attempt start, insert one `attempt_answers` row per live question of the
quiz, in the derived question order. The row is the slot; it starts with no
selections and `answer_text NULL`, `score NULL`.

Saving an answer upserts the slot and, for objective questions, replaces the
selection set in one transaction (delete/soft-delete old selections, insert the
new set). Essay questions write `answer_text` only. A blank answer is no
selections and `NULL` text. The endpoint is idempotent for retries and
double-submits.

There is no autosave in v1. The form POST on navigation is the save. The schema
note about re-posting every 450 ms is stale; the before-unload warning is the
contract's chosen behavior. Flagged.

### R-10 — Close and delete

Close (`open → closed`) sets `state`, `closed_at`, makes the code inert, and
ends the monitor stream. In-progress attempts remain answerable and
submittable; their result, recap, and history remain available. No new
registration, cancellation, or attempt start is allowed after close. There is
no reopen.

Delete is allowed only for drafts. A draft delete soft-deletes the quiz,
questions, and options, and removes image files after commit. Open and closed
quizzes are never deleted. There is no undelete. Because drafts cannot have
attempts, no attempt data is ever deleted by a quiz delete.

### R-11 — Visibility, code, invite

Visibility is chosen on the create form and is immutable thereafter; the
builder's settings form does not contain it. Public ⇒ both targets NULL and a
6-digit code exists. Private ⇒ both targets non-NULL and `code` is NULL. This
matches the schema XOR checks.

The code is generated at quiz creation for public quizzes (crypto-random,
unique among live rows, retry on collision). It is accepted only when
`state = open` and `visibility = public`. Draft and closed codes resolve to
`not_open`, never to a join. The contract §23 #3 "generated for every quiz but
surfaced only for public" is narrowed by the schema: private rows store NULL.

Every quiz has an `invite_token` from creation. It resolves for draft and
closed to `not_open`; it never reveals a quiz to a non-owner teacher.

### R-12 — Join and eligibility

| Quiz | Logged-in student | Visitor / anonymous |
|---|---|---|
| public, open | any student; code or invite; register then start | code or invite → identify → register → start |
| private, open | exact `grade_level` + `major_id`; invite auto-registers; intro can register | rejected: private admits no anonymous participant |
| draft | owner teacher only; students 404 | 404 |
| closed | no new join; registered students see intro/history; in-progress attempts resume | 404 |

Invitation URLs auto-register when the viewer is eligible and not already
registered. Codes do not auto-register logged-in students; they land on the
intro with **Daftar**. An existing live registration makes join idempotent:
land on the intro with an informational flash, never create a duplicate.
Eligibility is checked at registration and at attempt start; a student who
registered while eligible may finish existing attempts even if a teacher later
changes their grade or major.

### R-13 — Majors are referenced by id

Rename is a single-row update. There is no denormalized propagation write;
every screen joins the current name. Delete is refused while any live student,
anonymous participant, or quiz target references the major; soft-delete
otherwise. This preserves the contract §16.3 semantics without the old
denormalization.

### R-14 — Soft delete is the only delete

`deleted_at = '1970-01-01 00:00:00'` means live. Every read filters on that
sentinel. Cascades are application-level, in one transaction, stamping one
timestamp on the root and every descendant. Roots in the five provenance
tables (`accounts`, `majors`, `quizzes`, `questions`, `anonymous_participants`)
also set `deleted_by_account_id` to the actor; cascaded rows leave it NULL.

| Delete | Cascades to |
|---|---|
| student account | student profile, sessions, reset tokens, registrations, attempts, answers, selections, focus events |
| anonymous participant | registrations, attempts, answers, selections, focus events |
| draft quiz | questions, options; image files after commit |
| draft question | options; image file after commit |
| unused major | nothing |
| teacher account | forbidden |

No hard-delete route and no restore. Unique keys include `deleted_at`, so
re-registering an email, re-creating a major name, reusing a code, or
re-registering for a quiz after cancellation is allowed.

### R-15 — Time

Store UTC. The DSN must set `loc=UTC` and the connection `time_zone='+00:00'`;
forms parse WIB (UTC+7, fixed, no DST) and display WIB. Sessions expire 60
minutes after last use and slide on every authenticated request. Reset tokens
expire after 24 hours. Pending-registration tokens expire after 24 hours. Quiz
`starts_at`/`ends_at` are display-only and never gate access. No timers or
countdowns exist.

### R-16 — Sessions and CSRF

Session cookie: 256-bit random token, only SHA-256 stored, `HttpOnly`,
`SameSite=Lax`, `Secure` from `COOKIE_SECURE`, path `/`, 60-minute sliding
expiry. Login rotates the token. Logout soft-deletes the current session.
Password reset revokes every session of the student; change-password revokes
every other session and keeps the current one.

CSRF uses a signed double-submit cookie: a random token in an `HttpOnly` signed
cookie, mirrored in every form's `csrf_token`, compared in constant time. A
guest receives one; login and logout rotate it. Every POST requires it except
`POST /attempt/:id/beacon`, which instead requires a valid session/anonymous
identity, attempt ownership, and a same-origin `Origin`/`Referer`.

### R-17 — Question images

Accepted: JPEG, PNG, WebP, content-sniffed; hard maximum 10 MiB; one per
question. No resize, crop, or automatic compression in v1; the old 5 MiB
auto-compress rule is superseded by the contract's 10 MiB limit. Store under a
configured uploads root (default `./uploads`) with a random 128-bit filename;
the DB stores relative path, MIME, byte size, SHA-256, and optional `image_alt`
(max 255).

`image_alt` is author-supplied. The question editor therefore adds an optional
`image_alt` field when an image is present; this amends the contract §18
closed field list. Serve `GET /uploads/:name` with an unguessable name,
`Cache-Control: private, no-cache, must-revalidate`, `ETag` = SHA-256, and
`Last-Modified`. Replace and delete remove the old file after the DB commit;
orphans are tolerated. No file is shared between questions.

### R-18 — Review modes

| Mode | Question body | Student answer | Key / verdict | Per-row score |
|---|---|---|---|---|
| `questions_and_key` | yes | yes | yes | yes |
| `key_only` | no | no | key list only | no |
| `questions_only` | yes | yes | no | no |
| `answers_only` | yes | yes | no | no (also no verdict color) |

`show_score = false` removes the score tile and every per-row score, but leaves
verdicts that the mode permits; a hidden score renders `—`, never `0`.
Essays have no key. A graded essay shows score and feedback; an ungraded one
shows "Menunggu penilaian". The student result page exists for every submitted
attempt; `final_score IS NULL` shows the waiting state. The teacher recap always
shows the key regardless of mode. The pre-submit review is unaffected by mode.

### R-19 — HTTP, errors, copy

Every POST ends in 303 PRG. Validation failure re-renders the same template
with 422, the submitted non-password values, and `FieldErrors` (Indonesian).
Missing, soft-deleted, or unauthorized resources return 404. State conflicts
redirect back with an error flash; there is no 409 page. Rate limit returns 429
with `Retry-After`. CSRF failure returns 403. Database failure returns the 500
page. Join errors use the contract §5.3 copy exactly. Passwords are never
repopulated.

Global copy:

| Key | Indonesian |
|---|---|
| invalid credentials | Email atau kata sandi salah. |
| email taken | Email sudah terdaftar. |
| password policy | Kata sandi minimal 8 karakter, tanpa spasi, mengandung angka dan huruf besar. |
| password mismatch | Konfirmasi kata sandi tidak cocok. |
| current password wrong | Kata sandi saat ini salah. |
| same password | Kata sandi baru harus berbeda dari kata sandi saat ini. |
| invalid token | Token tidak valid atau sudah kedaluwarsa. |
| token used | Token sudah digunakan. |
| csrf | Sesi formulir tidak valid. Muat ulang halaman. |
| rate limited | Terlalu banyak percobaan. Coba lagi nanti. |
| session expired | Sesi Anda berakhir. Silakan masuk lagi. |
| pending teacher | Menunggu persetujuan guru lain. |
| must change password | Ubah kata sandi Anda sebelum melanjutkan. |
| not owner / not found | 404 page. |
| server error | 500 page. |

Domain specs add their own messages and must reuse these keys.

### R-20 — Live monitor and beacons

SSE `GET /teacher/quizzes/:id/stream` is owner-only and only while the quiz is
open. On connect send a `snapshot`; then `participant.added`,
`participant.updated`, `quiz.closed`, and a 30-second `heartbeat`. No replay
buffer. On close, send `quiz.closed` and end. `ParticipantRow` derives:
`answered` = slots with selections or non-empty essay text; `total` = live
questions; `state` = registration state; `focus_loss` = event count;
`last_seen` = `attempt.updated_at` when an attempt exists, else
`registration.created_at` (never null). There is no current-question column;
the contract's `ParticipantRow` does not display one.

`POST /attempt/:id/beacon` accepts JSON or text: `kind` is `progress` or one of
`blur`, `hidden`, `fullscreen_exit`. The generic `focus_loss` kind in the
contract is replaced by those three so the schema check is satisfiable. Progress
is idempotent and touches the attempt's `updated_at`. Focus inserts one
`focus_loss_events` row with `occurred_at = now`; it never blocks submission.
Limits: progress 30/min/attempt, focus 10/min/attempt. Both are CSRF-exempt but
require ownership and same-origin.

### R-21 — Lists, aggregates, pagination

Filters, search, sort, page, and per-page follow the contract; invalid query
values fall back to defaults and never 422. Ties break by `id` ascending unless
the contract says otherwise. Page sizes: 10 for students and quizzes, 20 for
grading. Dashboard caps: 5 recent quizzes, 6 latest student quizzes, 3
most/least joined. Best score and top scorer use only `final_score IS NOT NULL`
and `show_score = true`; hidden scores are excluded, not shown as zero. Top
scorer ties break by earliest `submitted_at`. Most/least joined count distinct
live registrations. Empty and filtered-empty states are distinct.

### R-22 — Profiles and email

`/profile` is student-only: `full_name` and `email` editable; grade and major
read-only. Email is trimmed, lowercased, and unique case-insensitively across
all live accounts. No email verification exists. Changing an email does not
revoke sessions. Teachers have no profile edit; they change only their password.

### R-23 — Pending-teacher token

A signed token carrying `account_id` and `issued_at`, valid 24 hours, set at
registration in an `HttpOnly` cookie and accepted from `?token=`. The pending
page is read-only and reveals only full name and status (waiting, approved,
rejected/expired). Viewing does not consume it. Approved shows a login link.
Unknown/expired shows a generic invalid-token state.

### R-24 — Bootstrap is external

The app never promotes a teacher from the registration form. If no active
teacher exists, the teacher directory is empty and no approval is possible;
the pending page says approval is not available yet. This is an operational
prerequisite, not a user action.

### R-25 — No application logic may assume a schema delta

The known gaps are resolved without schema changes:

| Gap | Resolution |
|---|---|
| no `last_seen` on attempts | `attempt.updated_at`; beacon touches it |
| no current-question column | not displayed; derived order only |
| no `status` columns | derive from timestamps |
| no `counted` flag | every attempt equal; best = max |
| no `option_order`/`position` on answers | SHA-256 seed order (R-08) |
| no CSRF column | signed double-submit cookie (R-16) |
| no focus counter | `COUNT(*)` of events |
| no quiz-title/participant snapshots | quiz title locks at publish; names are current |
| `image_alt` unused by the contract form | add the optional field (R-17) |
| `must_change_password` reset semantics | bootstrap-only (R-04) |
| private code stored as NULL | generate only for public (R-11) |

Anything not in this table that needs a column is a §11 schema delta, not a
silent workaround.

---

## 4. Global invariants

Domain specs must cite these and may add domain invariants, never weaken these.

| ID | Invariant | Enforced by |
|---|---|---|
| INV-01 | Live email is unique across all accounts, case-insensitively. | DB `uq_accounts_email` + app normalization |
| INV-02 | A student profile exists only for a `role='student'` account; at most one live profile per account. | DB unique + app role check |
| INV-03 | Every actor FK is role-typed: approver/deleter/owner/editor/issuer/grader is a teacher; `students.account_id` and `registrations.student_id` are students. | app (DB cannot) |
| INV-04 | Students are `active`; teachers are `pending` until approved; `must_change_password` blocks all but password change. | DB checks + app |
| INV-05 | A registration has exactly one participant (student XOR anonymous). | DB check |
| INV-06 | At most one live registration per (quiz, student) and per (quiz, anonymous). | DB unique keys |
| INV-07 | Public quiz ⇒ no targets + code; private ⇒ targets + no code. | DB check |
| INV-08 | Quiz code is exactly six digits, unique among live rows; invite token is non-empty and unique among live rows. | DB unique + app format |
| INV-09 | Quiz transitions are draft → open → closed only; `published_at` is set for open/closed; `closed_at` iff closed. | DB check + app |
| INV-10 | Questions and options are editable only in a draft; positions are dense 1..N and unique per parent among live rows. | app + DB unique |
| INV-11 | Single-choice has exactly one correct option; multiple-choice at least two; essay none. Objective questions have at least two options. | app |
| INV-12 | A question belongs to its quiz; an option belongs to its question; an answer's question belongs to the attempt's quiz. | app (DB FKs only partial) |
| INV-13 | At most one in-progress attempt per registration; `attempt_no` is sequential from 1. | app + DB unique |
| INV-14 | An attempt belongs to a registration; an answer slot belongs to the attempt; a selection belongs to the answer and an option of the same question. | app |
| INV-15 | `final_score` is the mean of per-question scores, NULL until all are scored, 0–1 at 4 decimals. | app |
| INV-16 | Objective score is exactly 0 or 1; essay score and final score are 0–1. | DB checks + app |
| INV-17 | Scores are visible only per `show_score`; hidden score is never rendered as zero. | app |
| INV-18 | No live child exists under a soft-deleted parent; cascade stamps one timestamp. | app transaction |
| INV-19 | Every unique key includes `deleted_at`; re-creating a deleted email, major, code, or registration is allowed. | DB |
| INV-20 | All stored timestamps are UTC; all display is WIB. | app + DSN |
| INV-21 | A session belongs to a live account; expired or soft-deleted sessions authenticate nothing. | app |
| INV-22 | Reset tokens are student-only, teacher-issued, 24-hour, single-use. | DB + app |
| INV-23 | A major referenced by a live student, anonymous participant, or quiz target cannot be deleted. | app |
| INV-24 | A question has zero or one image; path, MIME, bytes, and hash are all-or-none; MIME is allowed; bytes ≤ 10 MiB. | DB check + app |
| INV-25 | Registration and attempt states are derived, never stored. | app |
| INV-26 | Only the owner teacher reaches a quiz's authoring, results, recap, grading, monitor, and stream. | app |
| INV-27 | Anonymous identity is the unique cookie hash; names are not identities. | DB unique + app |
| INV-28 | Every POST is CSRF-protected except the beacon, which is ownership-checked and same-origin. | app |
| INV-29 | No hard delete and no restore route exist. | app |
| INV-30 | Review output is a pure function of mode, `show_score`, grading state, and ownership. | app |

---

## 5. Shared vocabulary

Storage values are lowercase snake case. Labels are Indonesian and live in Go.

| Concept | Values |
|---|---|
| Role | `teacher`, `student`, `anonymous` |
| Account status | `pending`, `active` |
| Quiz state | `draft`, `open`, `closed` |
| Visibility | `public`, `private` |
| Question type | `single`, `multiple`, `essay` |
| Review mode | `questions_and_key`, `key_only`, `questions_only`, `answers_only` |
| Participation state | `registered`, `in_progress`, `completed` |
| Attempt state | `in_progress`, `completed` (graded is `final_score != NULL`) |
| Essay state | `ungraded`, `graded` |
| Join error | `unknown_code`, `not_open`, `wrong_audience`, `already_registered`, `anonymous_not_allowed` |
| Focus kind | `blur`, `hidden`, `fullscreen_exit` |
| Sort order | `newest`, `oldest`, `name_asc`, `name_desc` |

Score scale: storage 0–1 (DECIMAL 5,4); view model whole percent 0–100; essay
input 0–100 with one decimal; half-up at the view-model boundary. Averages one
decimal with a comma.

---

## 6. Route ownership

The authoritative route list is the contract §8 plus the current `registerPage`
registry. `/healthz` is operational. `/uploads/:name` is authoring's read route.

| Owner | Routes |
|---|---|
| IDN | `GET /`, `GET/POST /login`, `GET/POST /register/teacher`, `GET/POST /register/student`, `GET/POST /reset-password`, `GET/POST /change-password`, `GET /auth/pending`, `POST /logout`, `GET/POST /profile`, `GET /teacher/students`, `GET/POST /teacher/students/:id`, `POST /teacher/students/:id/delete`, `POST /teacher/students/:id/reset-token`, `GET /teacher/teachers`, `POST /teacher/teachers/:id/approve` |
| AUT | `GET/POST /teacher/quizzes`, `GET /teacher/quizzes/new`, `GET/POST /teacher/quizzes/:id`, `POST /teacher/quizzes/:id/publish`, `POST /teacher/quizzes/:id/close`, `POST /teacher/quizzes/:id/delete`, `GET /teacher/quizzes/:id/questions/new`, `POST /teacher/quizzes/:id/questions`, `GET/POST /teacher/quizzes/:id/questions/:qid`, `POST /teacher/quizzes/:id/questions/:qid/delete`, `POST /teacher/quizzes/:id/questions/reorder`, `GET /uploads/:name` |
| PAR | `GET/POST /join`, `GET /invite/:token`, `GET/POST /join/identify`, `GET /quiz/:id`, `POST /quiz/:id/register`, `POST /quiz/:id/cancel`, `POST /quiz/:id/attempts`, `GET /student`, `GET /student/history`, `GET /student/history/:quizID`, `GET /teacher/anonymous`, `GET/POST /teacher/anonymous/:id`, `POST /teacher/anonymous/:id/delete`, `GET /teacher/majors`, `POST /teacher/majors`, `POST /teacher/majors/:id`, `POST /teacher/majors/:id/delete` |
| ATT | `GET /attempt/:id`, `POST /attempt/:id/answer`, `GET /attempt/:id/review`, `POST /attempt/:id/submit`, `GET /attempt/:id/result`, `POST /attempt/:id/beacon`, `GET /teacher`, `GET /teacher/quizzes/:id/results`, `GET /teacher/quizzes/:id/monitor`, `GET /teacher/quizzes/:id/stream`, `GET /teacher/grading`, `GET/POST /teacher/grading/:answerID`, `GET /teacher/recap/:attemptID` |
| OPS | `GET /healthz`, `GET /error/404`, `GET /error/500`; cross-cutting rules, no business command |

`POST /quiz/:id/attempts` is PAR's command but hands off to ATT's attempt-start
rule. `GET /student/history*` is PAR's read model using ATT's score contract.
`GET /teacher` is ATT's aggregate using PAR's registration contract.

---

## 7. Required shape of every domain spec

Use exactly these sections, in order:

1. **Header** — title, date, status, scope, lens, sources, rule-ID prefix.
2. **Intent and boundaries** — what this domain decides and what it delegates.
3. **Actors, permissions, entry conditions** — including 404-vs-403 policy.
4. **States and transitions** — table or diagram; guards, effects, terminal states.
5. **Commands** — one subsection per route/action. Each command carries: ID,
   route, actor, authorization, preconditions, input and validation, transaction
   boundary, effects, postconditions, errors and copy, redirect/status,
   audit/provenance, idempotency.
6. **Queries and read models** — filters, search, sort, pagination, empty state,
   visibility rules, exact derivations.
7. **Invariants owned** — cite INV IDs; add domain IDs.
8. **Edge cases and failure modes** — stale forms, deleted entities, expired
   sessions, race conditions, partial failures.
9. **Concurrency and idempotency** — locks, unique-key races, double submit,
   retry semantics.
10. **Cross-domain contracts** — what this domain guarantees and requires, by ID.
11. **Traceability** — every owned route → command IDs → invariants → acceptance
    tests.
12. **Open questions and schema deltas** — each with a recommended default and
    impact. No bare TBDs.
13. **Acceptance criteria** — numbered, executable statements. At least one per
    command and per cross-cutting rule.

Rule IDs: `IDN-###`, `AUT-###`, `PAR-###`, `ATT-###`, `OPS-###`. Commands use the
same prefix plus `-C` only if needed; invariants `INV-###`. Cite, don't restate.

Depth: every command must be implementable without asking a question. Every
state-changing rule must say what the transaction locks, what it writes, and
what it does on conflict. Every list must define its query semantics. Every
message must be exact Indonesian or explicitly reuse a key from §3 R-19.

---

## 8. Definition of done for the five-spec set

- Every route in §6 is owned and has command rules.
- Every state transition has guards, effects, and conflict behavior.
- Every invariant names its enforcement layer (DB, transaction, app check).
- Every command has authorization, CSRF, concurrency, idempotency, and copy.
- Every list has filters, sort, pagination, tie-break, empty, filtered-empty.
- Every cross-domain call cites the owning rule ID.
- Every score, date, and count has one derivation and one formatter.
- No rule depends on a schema change; deltas are isolated in §11.
- No TBD without a recommended default.
