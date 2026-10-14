# Business Rules — quiz-v2

Status: DRAFT | Owner: 01-BUSINESS | Updated: 2026-10-14 (Q10 applied: BR-013 uses the canonical `paused_at`; F-36 closed: BR-054 names the `ajaxComplete` component (C-010) and its asserting ACs)

Deterministic rules. Each rule states trigger, inputs, exact condition,
outcome, enforcement layer, and acceptance sketch. Vague adjectives are banned;
thresholds carry units and inclusivity.

## Quiz configuration

### BR-001 — Quiz type is immutable after creation
- **Trigger:** update quiz. **Inputs:** `quiz.type`. **Condition:** `type` is
  set once at creation and MUST NOT change afterwards — even with zero
  attempts rows (the single normal↔matching switch in R1 history is
  superseded; see BEFORE-AFTER CHG-001). Rationale: waiting-room, attempts,
  and timer semantics diverge irreversibly; allowing a pre-live flip would
  fork BR-002/BR-008/STATE-002 assumptions.
- **Rationale:** waiting-room, attempts, and timer semantics diverge
  irreversibly. **Enforcement:** server. **Outcome pass:** n/a (creation only).
  **Fail:** ERR-106. **Example:** create as normal → stays normal; a matching
  quiz must be recreated, not converted.
- **Acceptance:** Given a quiz with 0 attempts, When teacher changes type,
  Then 422 ERR-106 and type unchanged.

### BR-002 — Attempts setting applies to normal quizzes only
- **Trigger:** create/update quiz. **Inputs:** `type`, `max_attempts`.
- **Condition:** `type=normal` ⇒ `1 <= max_attempts <= 10`, integer, default 1.
  `type=matching` ⇒ `max_attempts` forced to 1 and input ignored.
- **Rationale:** DECIDED by user; synchronous sessions cannot repeat.
- **Enforcement:** server validation inside the quiz transaction (no DB CHECK).
- **Acceptance:** Given matching quiz, When teacher sets attempts=3, Then
  stored as 1 with notice.

### BR-003 — Timer modes and units
- **Trigger:** create/update/start. **Inputs:** `timer_mode ∈ {none,
  per_question, total}`, `timer_seconds` integer > 0 in seconds.
- **Condition:** `none` ⇒ `timer_seconds` MUST be NULL. `per_question` and
  `total` ⇒ `timer_seconds` is a teacher-typed custom integer with
  `1 <= timer_seconds <= 86400` (DECIDED DEC-005; the upper bound is a
  technical guard against overflow and absurd sessions, not a product preset —
  the teacher types any value, no preset options).
- **Rationale:** the teacher decides the duration per quiz; the server remains
  the time authority. **Enforcement:** server validation in the quiz transaction
  (no DB CHECK; R2 stores modes as TINYINT codes). **Fail:** ERR-101.
- **Matching per-question (DEC-119):** `per_question` is allowed on matching
  quizzes, but there is no auto-advance; per-question deadlines are recorded for
  recap accuracy only, and movement follows teacher cues only (monitor single index).
- **Trace:** DEC-119; EDGE-070; AC-228.
- **Example:** teacher types 45 s per question for a 10-question quiz = 450 s maximum occupancy.

### BR-004 — Choice-question option integrity
- **Trigger:** question create/update. **Inputs:** options array, correct flags.
- **Condition:** single-choice ⇒ 2–10 options, exactly 1 correct.
  multiple-choice ⇒ 2–10 options, >=1 correct. Option text 1–500 chars,
  unique within question (case-insensitive trim). Essay ⇒ zero options.
- **Enforcement:** server. **Fail:** ERR-102.

### BR-005 — At least one gradable question to open (and to stay open)
- **Trigger:** open quiz; delete question. **Condition:** quiz MUST have >=1 question AND every
  choice question satisfies BR-004 AND at least one non-essay question (DECIDED Q9, 2026-10-14:
  there is NO essay-only acknowledgement checkbox — an essay question has no answer key, so it can
  never satisfy or be waived for a gradable-question gate; the gate is derived from the question set
  alone, and no storage/transport/UI artifact exists for it). Maintain-gate (DECIDED
  DEC-089): deleting the last remaining question of an `open` quiz is BLOCKED
  with 422 ERR-108 (same code and message as open-gate — "Quiz butuh minimal
  1 soal valid"), changing nothing; the teacher must close the quiz first,
  then delete. Draft quizzes may go to zero questions freely.
- **Rationale:** essay-only quizzes have no temporary score and cannot be auto-graded, so the
  gate simply requires at least one non-essay question; no consent step exists (DECIDED Q9).
  **Fail:** ERR-108.

### BR-006 — Freeze authoring once live (DEC-103)
- **Trigger:** any question/quiz-setting mutation. **Condition:** draft quizzes
  are always editable. On `open+hidden` quizzes, timer/shuffles/max_attempts/questions
  stay editable while the quiz holds only `registered`/`cancelled` rows (no
  `answering`/`submitted` rows anywhere). On `open+show` quizzes, ANY attempts row
  (`registered`/`cancelled`/`answering`/`submitted`) freezes questions, timer,
  shuffles, and max_attempts → 422 ERR-107. Any `answering`/`submitted` row ANYWHERE
  (including hidden or closed quizzes) always freezes those fields → 422 ERR-107.
  (`type` is not listed here — it is NEVER editable after creation per BR-001,
  even with zero rows.) Title, description,
  visibility, token toggle remain editable.
- **Rationale:** started attempts snapshot their shuffle/correct mapping;
  editing would corrupt grading fairness. Visible quizzes freeze early so
  registered students see stable content (EDGE-065).
- **Trace:** DEC-103; ERR-107; AC-113; EDGE-065.

### BR-007 — Student quiz visibility predicate (DEC-115)
- **Trigger:** dashboard list, direct URL access. **Condition:** student sees
  quiz in list iff `status=open AND visible=true AND (type=normal OR
  (type=matching AND session != finished))` (words here; R2 stores TINYINT codes).
  hidden quizzes return 404 hidden response to students on list and direct access
  (not 403) to avoid leaking existence. Closed-but-visible quizzes
  (`status=closed AND visible=true`) return 200 closed view ("sudah ditutup",
  no join/register/start buttons) on direct access but are excluded from the
  discover list. Closed+hidden returns 404.
- **Enforcement:** server on every GET.
- **Trace:** DEC-115; EDGE-068; AC-226.
- **S2 page mapping (CLOSED 2026-10-14 per §5b/§5c):** S2a (invalid `?page`
  clamp to page-1/last-page, never 4xx) DECIDED in API-000 per DEC-100/BR-055/EDGE-063/
  AC-220. S2b (final closed-view page layout + button mapping + list-fragment template
  table) is CLOSED per user decision 2026-10-14: the closed-but-visible quiz renders a
  200 closed view with the "sudah ditutup" message and no join/register/start buttons and
  no further action. See `02-ARCHITECTURE-AND-TECHNICAL/
  02-OPENAPI-SPEC.md` API-000.

## Registration and attempts

### BR-008 — Registration cancellability window
- **Trigger:** cancel registration. **Condition:** student may cancel iff
  the row is still `registered` (never started). Teacher may revoke iff
  the row is still `registered` (no frozen start snapshot). After start, neither party cancels;
  the attempt must be submitted or auto-submitted.
- **Outcome:** `registered → cancelled` terminal; slot freed for re-register
  (normal quizzes, within max_attempts).
- **No-show exception (DECIDED DEC-088):** a matching occupant that never
  became present (`last_seen IS NULL` at session start) stays `registered`
  (marked no-show) and is never auto-submitted; the teacher may still revoke
  such a never-present row even after start (the only post-start revoke —
  it holds no frozen snapshot).

### BR-009 — Repeat attempts duplicate, never overwrite
- **Trigger:** start attempt N>1 (normal only). **Condition:** allowed iff
  `completed_attempts < max_attempts`. New attempt row with fresh shuffle
  snapshot; cheat count starts at 0; previous attempts immutable.
- **Example:** max=3 → student may hold 3 submitted rows with different scores;
  recap shows all three newest-first.
- **Exhaustion race (DECIDED DEC-083):** register is the primary gate
  (attempts-left checked before INSERT; see schema §5). If a `registered` row
  exists but start finds attempts exhausted (max lowered, or double-tab race),
  start rejects with 410 ERR-208 AND auto-cancels that stranded row
  (`registered → cancelled` in the same transaction, audit-logged) so no
  unstartable row survives; the student may re-register only if a slot frees.

### BR-010 — Question image constraints
- **Trigger:** image upload. **Inputs:** file bytes, MIME.
- **Condition:** formats JPG/PNG/WEBP/JPEG/GIF (GIF first frame only, no
  animation preserved — DECIDED simplification); exactly ≤1 image per
  question; the upload is stored immediately to a temp path with
  `questions.image_status=processing` and the request returns at once
  (DECIDED DEC-021: compression runs in a bounded in-process background worker
  pool, 2 workers, queue 100 — F-004). The exact limit is 10 MiB = 10485760 bytes
  (Q8, 2026-10-14): files above 10485760 bytes are compressed to
  ≤10485760 bytes then promoted to the final path (`ready`); files at or below 10485760 bytes
  promote without recompression. Unreadable, corrupt, or queue-timed-out
  (10 min) files → `failed` plus `image_error`, and the teacher re-uploads.
  The quiz stays openable and answerable while processing (image optional);
  students see a placeholder until `ready`. Stored outside web root with random
  filename; served via signed handler checking quiz access. Optional `image_alt`
  (≤500 chars, teacher-supplied, never invented) renders as the image `alt`
  attribute; when absent the image renders with an empty `alt` (DESIGN.md §10).
- **Enforcement:** server. Intake validation (format/count/MIME) is synchronous;
  compression/conversion runs in the bounded background worker pool (DECIDED
  DEC-021, F-004). **Rationale:** teacher uploads never block on CPU work;
  students see a placeholder until `ready`.

### BR-011 — Per-question timer is forward-only (DEC-119, DEC-130)
- **Trigger:** answer save under `timer_mode=per_question`.
- **Condition:** each question's deadline is computed server-side as `question_deadline_utc = server_now_utc + timer_seconds` at the moment the server advances the index (server clock; `attempt_deadline_utc`/`question_deadline_utc` are the sole truth for enforcement and recap — DEC-130 deadline-only: there is NO `question_started_at` column and no derived start timestamp; the deadline is set from server-now plus the configured timer seconds). POST after deadline → rejected ERR-205, current answer (if any)
  locked, server advances index. No back-navigation: `GET` for `n <
  current_index` returns 410 with read-only snapshot (row `ERR-209` in
  `03-RESILIENCE-AND-ERRORS/01-ERROR-MATRIX.md`; the read-only page shows the
  locked question with its saved answer and offers forward-only navigation). Review page shows
  read-only trail only. On matching quizzes `per_question` is allowed but there is
  no auto-advance; deadlines are recorded for recap accuracy only and movement follows
  teacher cues only (monitor single index).
- **Acceptance:** Given Q1 deadline passed unanswered, When student answers
  Q1, Then 422 ERR-205 and Q2 displayed, Q1 recorded unanswered.
- **Trace:** DEC-119; DEC-130; EDGE-070; AC-228.

### BR-012 — Total timer allows free navigation with server auto-submit
- **Trigger:** any save under `timer_mode=total`. **Condition:** student may
  visit any question and the review page until `attempt_deadline_utc`. At
  expiry the timer goroutine auto-submits with `auto_submitted=true`.
- **Late answer save:** any per-question answer POST received with server time
  `now > attempt_deadline_utc` → 410 ERR-206 (`attempt_expired`), changing nothing;
  the attempt is already submitted with the data present at the deadline.
- **Late submit (DECIDED DEC-086):** `POST .../submit` received after the
  deadline (i.e. after the timer path already auto-submitted) → `200` with the
  auto-submitted receipt (same body as the auto-submit result, `auto_submitted=true`),
  NOT a 4xx error. Rationale (user decision 2026-10-12): a 400 ms-late click is
  the normal case, not a client fault; ERR-206 is reconsidered to cover answer
  saves only, never the submit itself.
- **Rationale:** DECIDED DEC-006 — zero grace keeps the deadline absolute and
  auditable; in-flight saves are preserved up to the deadline itself.

### BR-013 — Pause freezes matching timers
- **Trigger:** teacher pause/resume. **Condition:** on pause, server records
  `paused_at`; on resume, every occupant `attempt_deadline_utc += now - paused_at`.
  Question-level deadlines shift identically. No answers accepted while
  `session=paused` (ERR-302).
- **Enforcement:** server transaction over session + attempts.

### BR-014 — Attempt ordering in recap (DEC-116)
- **Trigger:** recap render. **Condition:** rows ordered by `submitted_at`
  DESC, NULLS (unsubmitted live rows in monitor only) first in monitor;
  recap shows submitted only. Anonymous rows display `(anon) ` prefix +
  `taker_label` (e.g. `(anon) Budi`). Export note: recap XLSX export honors
  `window`/`class`/`q`/`date`/`status` and page-ignored export — IGNORES
  `page`/`per_page` (DEC-116).

## Scoring

### BR-020 — Objective grading is all-or-nothing
- **Trigger:** submit/auto-submit. **Inputs:** per-question saved answer vs
  snapshot correct set.
- **Condition:** single-choice: 1 iff selected == correct. Multiple-choice:
  1 iff selected set equals correct set exactly (order-insensitive); else 0,
  including empty and partially-correct. Essay: 0 until graded.
- **Rationale:** DECIDED by user. **Example:** correct {A,C}; student {A} →
  0; {A,B,C} → 0; {A,C} → 1.

### BR-021 — Essay grade domain
- **Trigger:** grade essay. **Condition:** integer 0–10 inclusive.
  Non-integer or out-of-range → ERR-105. Re-grade allowed, overwrites with
  audit entry (old value, new value, grader, timestamp). Grades live inside
  `attempts.answers_json` (R2); history in `audit_log`.

### BR-022 — Final score formula
- **Trigger:** each essay grade / submit. **Condition:**
  `final = objective_sum + Σ(essay_grade / 10.0)`, rounded to 2 decimals
  (half-up). `temporary = objective_sum` (essays count 0) until all essays
  graded, then `temporary` label replaced by `final`.
- **Example:** 8 single correct + essays 7 and 9 → 8 + 0.7 + 0.9 = 9.60 / 10.
- **Enforcement:** server recompute in the grading transaction.

### BR-023 — Unanswered and locked questions score 0
- **Trigger:** grading. **Condition:** NULL answer, auto-locked unanswered,
  or rejected-late answer all grade 0 for that question. No negative marking.

## Visibility of results

### BR-030 — Score visibility flag
- **Trigger:** result card + history render. **Condition:**
  `show_score=true` ⇒ card shows temporary (or final) numeric score.
  `false` ⇒ card shows only thank-you + "score hidden by teacher".
  History respects the flag at view time (toggling applies retroactively).

### BR-031 — Correct-answer reveal flag
- **Trigger:** result/review render. **Condition:** `show_correct=true` ⇒
  student sees per-question correct marking. `false` ⇒ sees own answers
  without correctness. Independent of BR-030: score number may show while
  per-question correctness stays hidden.

## Anti-cheat

### BR-032 — Every focus-loss event counts, no threshold
- **Trigger:** client reports hidden/blur during answering. **Condition:**
  each qualifying event atomically `cheat_count += 1`; no auto-submit, no
  lockout, no reset within the attempt. Repeat attempts start at 0.
- **Qualifying events:** `document.visibilityState=hidden`, `window.blur`
  with no refocus within 1 s (debounce to avoid double-counting blur+hidden
  pairs — only one increment per 2 s window, DECIDED),
  `pagehide`. Resize/scroll/DevTools-open without blur do NOT count.
- **Enforcement:** server idempotency: the `eid` is checked against
  `attempts.cheat_events_json` under the attempt row lock; retries with the same key do not double-count (R2, no incident table). Focus loss while the session is paused still counts (the attempt is still `answering`); POSTs on terminal rows (`submitted`/`cancelled`/`revoked`) echo without increment.

## Token mode

### BR-040 — Code format and lookup
- **Trigger:** anonymous join. **Condition:** code 6–12 chars `[A-Za-z0-9]`;
  lookup only among `token_enabled=true AND status=open AND visible=true` quizzes; miss →
  generic ERR-400 ("code invalid or quiz unavailable") to prevent enumeration.
  A hidden quiz's code stops working (same hiding semantics as BR-007 for accounts).
  Codes unique among open token-enabled quizzes (app-checked in the quiz transaction; plain index, no partial UNIQUE).

### BR-041 — Ephemeral registration, persisted attempt (DEC-106, DEC-117)
- **Trigger:** anonymous identify. **Condition:** display name 1–100 chars
  required; stored on attempt as `taker_label` with `student_id NULL`, never as user row; in-memory
  intent TTL 24 h or until quiz closes; server restart drops pending intents
  but never submitted attempts. one-active rule: one `taker_label` holds at most one
  active (`registered` intent or `answering`) row per quiz; a second start while an
  active row exists → 409 active conflict (same message family as ERR-201), else quota
  exhausted → 410 ERR-208. TTL-entry gate: the 24 h TTL is an entry gate only;
  an `answering` attempt and its result card use row membership and never re-check TTL.
- **Quota (DECIDED DEC-091, R8):** the same display name (exact match) on the
  same quiz shares one `max_attempts` quota — a re-identify with the identical
  name counts against the same quota (410 ERR-208 when exhausted, same message);
  a different name is a new participant. Name-spoofing (`Budi` → `Budi2`) is
  trivially possible and documented: the teacher stops abuse by closing the
  quiz; duplicate open codes stay rejected (ERR-114).
- **Trace:** DEC-106; DEC-117; EDGE-067; EDGE-069; AC-224; AC-227.

## Accounts

### BR-050 — Seeded teacher and forced rotation
- **Trigger:** first deploy. **Condition:** seed creates exactly one teacher
  from env vars; `must_change_password=true`; login forces password change
  before any other action. Seed is idempotent (skips if any teacher exists).

### BR-051 — Deactivate vs delete (DEC-105, DEC-129)
- **Trigger:** teacher or student account action. **Condition:** deactivate ⇒
  `status=inactive`, sessions revoked, SSE closed, attempts preserved —
  EXCEPT deactivate is BLOCKED while the user holds any `attempts` row with
  `state` registered or answering on a quiz with `status=open` (DECIDED
  DEC-081: guard against mid-exam lockout — the teacher must wait for submit/
  auto-submit, or close the quiz first; the server checks
  `attempts WHERE student_id=? AND state IN (0,1) AND quiz_id IN
  (SELECT id FROM quizzes WHERE status=2)` inside the deactivate transaction
  and rejects with 409 ERR-115 when a row matches, changing nothing).
  Live-session block (DEC-105, live-session block): deactivate is also BLOCKED with 409 ERR-115 when
  the target owns a running or paused matching session — including self-deactivate
  by the owning teacher — until the session is stopped (deactivate still auto-closes
  owned open quizzes per BR-052, but a live session must be stopped first).
  Open-quiz guard precedence (DECIDED Q3, 2026-10-14): the deactivate is BLOCKED, not
  force-submitting, while an open quiz still exists. The guard is checked first, inside the
  deactivate transaction, before any auto-close runs; a target user holding any
  `registered`/`answering` row on a quiz with `status=open` (DEC-081 guard above) makes the
  whole action fail with 409 ERR-115, changing nothing. The force-submit clause is NOT operative
  behaviour: an already-closed quiz's live rows are not force-submitted by this path either —
  the teacher must stop the session, wait for submit/auto-submit or close the quiz first.
  Because the open-quiz guard rejects before the auto-close (DEC-092) executes, no
  `status → closed` transition, no force-submit and no session revocation can leave the account
  deactivated with an orphaned open quiz or a live session.
  Delete ⇒ two-step button confirm (explicit dialog buttons, no typed text —
  DECIDED DEC-042), only from inactive; attempts anonymised
  (`student_id → NULL`, `taker_label='deleted-user-<id>'`); hard delete of user
  row. Delete self is allowed for both roles via inactive plus confirm (both-can-delete);
  a student
  may edit own profile but never change own role.
- **Self-delete qualifier (DECIDED DEC-129):** both roles may self-delete via
  inactive plus confirm, BUT a teacher self-delete is BLOCKED while the teacher
  owns ANY quizzes (open, closed, or draft — ownership is not limited to live
  sessions). The teacher must delete every owned quiz first; only
  a teacher with zero owned quizzes may self-delete. A student self-delete is
  never blocked by quiz ownership (students own no quizzes).
- **Trace:** DEC-105; DEC-129; ERR-115; EDGE-066; AC-223.
- **S1 Retry-After (DECIDED S1a, 2026-10-14):** 429 ERR-112 queue-full responses carry
  `Retry-After: 1` (time-based retry; AC-105 asserts the header). The 429 path is ERR-112
  only; 409 responses are conflicts, not time-based, and never carry Retry-After (S1b
  scope narrowed to the 429 path). See `02-ARCHITECTURE-AND-TECHNICAL/02-OPENAPI-SPEC.md`
  API-023.

### BR-052 — Attempt retention
- **Trigger:** any delete path. **Condition:** attempt rows are NEVER hard
  deleted except quiz deletion (which requires zero rows with `state` in
  (`registered`, `answering`, `submitted`) on that quiz — i.e. no live or
  scored rows; only `cancelled`/`revoked`-only or zero-row quizzes are
  deletable, DECIDED DEC-082; blocked delete → 409 ERR-111, changing nothing)
  or explicit DB purge outside MVP. Account deletion anonymises, recap continues
  to balance.
- **Cascade (DECIDED DEC-092, R8):** quiz delete cascades to its remaining
  `cancelled`/`revoked` rows (hard-deleted with the quiz, audit-logged) — no
  dangling `quiz_id` rows survive, ever.
- **Owned-quiz guard (DECIDED DEC-081/DEC-092, Q3 2026-10-14):** deactivate is REJECTED with 409 ERR-115 whenever the target owns ANY `open` quiz — no quiz is closed automatically by this path. The user decision 2026-10-14 (Q3, N-A5) supersedes the earlier auto-close wording: the guard is the operative behaviour. In-flight attempts continue normally under frozen deadlines (AC-126B semantics). No quiz is ever left open-but-unmanageable: the steward teacher must close or hand over each owned quiz before the account can be deactivated (there is no ownership transfer endpoint, DEC-031 strict — the guard, not auto-close, is the stewardship rule).

### BR-053 — Password complexity (DEC-003)

- **Trigger:** register, profile password change, teacher create/reset
  password, seed. **Condition:** length at least 8 characters, zero whitespace
  characters, at least one ASCII digit, at least one ASCII uppercase letter.
  Evaluated server-side; fail → ERR-110 with the exact rule text. The seed
  aborts boot with a clear log line when the env password is non-compliant.
- **Rationale:** DECIDED DEC-003. Teacher-set passwords follow the same rule.

### BR-054 — Session lifetime and realtime revocation (DEC-003/032)

- **Trigger:** login and every authenticated request. **Condition:** sessions
  slide idle — `expires_at = last_activity + 60 min`. Expiry → 401 with draft
  preserved client-side; relogin resumes at the server position (EDGE-060).
  A password change revokes ALL other sessions immediately and closes their
  SSE streams at once (DECIDED DEC-032 realtime); the changing session survives
  with a rotated csrf_token. The rotation response carries the new token in
  the `X-CSRF-Token` response header AND re-renders it into subsequent forms;
  jQuery AJAX swaps it via the global `ajaxComplete` handler owned by
  **C-010 (Global chrome — banner/modal/toast layer)** in
  `04-FRONTEND-UIUX/02-COMPONENT-STATES.md` (one global
  `$(document).ajaxComplete(...)` registration in the page-shell inline script,
  reading `X-CSRF-Token` off every response and updating the hidden
  `csrf_token` fields in place — UI-002/DEC-114 full-jQuery carrier, no poll, no
  reconnect) — asserted by **AC-161** (rotated csrf_token survives and the next
  AJAX POST succeeds) and **AC-118** (a POST with the stale token after rotation
  → 403 ERR-002 with reload guidance).
- **Enforcement:** server middleware plus session service. Check order: session
  validity is checked BEFORE the transaction begins (revoked → 401, no write);
  in-transaction revocation cannot interleave (single request scope) — an
  answer/cheat POST racing a password-change commit resolves deterministically.

## Precedence

Conflicts resolve: safety/auth (BR-050/051, visibility BR-007) > freeze
(BR-006) > timing (BR-011/012/013) > grading (BR-020..023) > display
(BR-030/031). Within grading, snapshot-at-start beats later edits always.

**ERR-107 vs ERR-108 precedence (F-70, closed 2026-10-14):** when a delete
would BOTH violate the freeze rule (BR-006 → 422 ERR-107) and empty an `open`
quiz (BR-005 maintain-gate → 422 ERR-108), the FREEZE check runs first and its
result wins: the response is 422 ERR-107 with the freeze message, changing
nothing. The teacher must close the quiz first (then the freeze lifts), and
only a closed quiz can be deleted empty. The two rules never compose into a
single ambiguous rejection, and the ordering follows the ladder above (freeze
outranks the open/delete gate). Asserted by AC-113 (frozen leg) + AC-101
(empty-open leg after closing). ERR-108 remains the response for a delete that
empties an open quiz WITHOUT any freeze violation (no `answering`/`submitted`
row, and `open+show` holds no attempts row at all).

## Listing (search, pagination, filters)

### BR-055 — List query contract (search + pagination + filters) (DEC-111, DEC-112, DEC-113, DEC-116)
- **Trigger:** teacher lists + history pages (teacher quiz list, recap, grading
  queue, attempt history, user management). Student dashboard `GET /student` is
  excluded — student-full-list exclusion: it renders the full open+visible list
  (student-full-list, no search/pagination)
  with no search and no pagination (BR-007 predicate only).
- **Condition (DECIDED DEC-098/099/100 R9 + DEC-102 R10 + DEC-111/112/113 2026-10-12):**
  - Canonical query keys: `?q=` (substring search, server-side, case-insensitive;
    teacher quiz list searches title; recap searches `taker_label`; user mgmt
    searches name/email; history searches quiz title snapshot), `?page=` (1-based,
    default 1), `?per_page=` (SSR default 25, max 200 — see below),
    `?window=3d|7d|30d|all` (recap/dashboards, default all),
    `?class=` (exact `taker_class` match, recap only),
    `?status=draft|open|closed|all` (teacher quiz list, default all),
    `?date=YYYY-MM-DD` (Asia/Jakarta day filter on `submitted_at`, recap/history
    only). Unknown keys are ignored
    (never 4xx); invalid values fall back to
    defaults (malformed `?page=`/out-of-range page → page 1 / last page;
    `?per_page=` >200 → clamped to 200 with no error). There is NO `?poll=`
    or `?format=` key — polling does not exist (DEC-102); JSON envelopes are
    content-negotiated per API-000 (`Accept: application/json`), never a URL key.
  - Pagination (DEC-099): every covered list paginates. SSR pages default `per_page=25`
    (the `?per_page=` key MAY override up to 200 — user decision "default 25,
    max 200 custom"); JSON/API lists (recap export
    selection) use explicit `?page/&per_page=` up to max 200. Page links are
    server-rendered (`components/pagination`, UI-004); current page
    `aria-current="page"`. Every page query is a plain `LIMIT/OFFSET` over the
    §6 hot-read predicate — no cursor, no extra index (R2 unchanged).
  - Realtime searchbar (DEC-098 as refined by DEC-111/112): every search input is
    realtime with a 300 ms debounce (user decision): the client waits 300 ms after
    the last keystroke, then issues the SAME canonical URL (`?q=` + reset to `page=1`,
    other keys preserved) via jQuery AJAX partial swap (UI-002 bounded carrier) and
    `history.replaceState` — replaceState-only: no history entries are pushed, so
    browser back exits the list (no restore claim). Fragment rule: AJAX list requests
    carry jQuery's default `X-Requested-With: XMLHttpRequest` header → server returns
    ONLY the table/cards + pagination HTML fragment (no layout); without the header →
    full page. Non-q filters (`window`/`class`/`status`/`date`) and pagination links use
    full-page SSR GET by default; an AJAX fragment is used only when explicitly needed
    for the search `q` input (SSR+AJAX-minimal). In-flight rule: a new keystroke aborts
    the previous request (`jqXHR.abort()`) before starting the 300 ms timer — a late
    response can never overwrite newer results.
    No-JS fallback: plain form submit (GET) to the same URL — identical
    results, full-page render.
  - Export (DEC-116): recap XLSX export honors `window`/`class`/`q`/`date`/`status`
    filters and page-ignored export — IGNORES `page`/`per_page` (always the full
    filtered set, never the current page).
- **Rationale:** one URL contract for SSR + client-server: server stays source
  of truth, debounce bounds request rate without a rate limit (DEC-073
  untouched — 300 ms client pacing is politeness, not enforcement), URLs stay
  shareable.
- **Enforcement:** server (canonical parsing + clamping); client (debounce +
  replaceState-only navigation).
- **Trace:** DEC-111; DEC-112; DEC-113; DEC-116.
- **Acceptance:** Given 60 attempts, When teacher opens recap, Then page 1
  shows 25 rows + `?page=2,3` links; Given `?per_page=5000`, Then clamped to
  200 with no error;
  Given typing "bud" with 300 ms pauses, Then one request per pause with
  `?q=bud&page=1`; Given no-JS submit, Then identical rows full-page.
