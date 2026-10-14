# Transport Contracts — quiz-v2 (SSR + SSE + POST)

Status: DRAFT | Owner: 02-ARCHITECTURE | Updated: 2026-10-14 (S1a/S2/S4a/S5/N1/N2/N4/N5/N9/N10/N16/N17 applied; Q10 applied outside API-000/010/012/023/036/039b: transport payloads use the canonical schema column names `attempt_deadline_utc`/`paused_at`)

## 0. Contract format statement

This project does NOT expose a REST/JSON API or OpenAPI document as its
primary interface. The canonical transports are:

1. **SSR HTML pages** (`text/html`) rendered by Go templates through the Fiber
  template engine, navigated by full-page loads and POST-then-redirect (PRG).
2. **Action endpoints** accepting `application/x-www-form-urlencoded` (page
   forms, incl. `serialize()`-built bodies) or `application/json`
   (`jQuery.ajax`/`post`/`getJSON` per UI-002, NOT raw fetch; the cheat beacon
   is the sole exception — one native `sendBeacon` call for the `pagehide`
   unload case only, API-03A), returning either a redirect /
   HTML partial or a JSON envelope.
3. **SSE streams** (`text/event-stream`, NATIVE `EventSource` only — never
   jQuery, UI-002) for server→client push; all
   client→server mutations remain plain POSTs (carried by jQuery AJAX where
   JS is present). Canonical event catalog (exact names — prose/flows MUST use
   these, no aliases):
   - attempt stream (API-036): `heartbeat`, `question_locked {qid, advance_to}`,
     `time_expired`, `session_paused`, `session_resumed`, `session_stopped`,
     `cheat_recorded {cheat_count}`, `graded {final_score}` (result-page-only
     per DEC-104 + Q5 2026-10-14 — the final result page after submit
     `/attempts/:id/result`, open stream, never the review page or the card);
   - matching student stream (API-041): `session_started {attempt_id}`,
     `session_paused`, `session_resumed`, `session_stopped {result_url}`,
     `room_closed` (empty-room abort / close-hide while waiting, DEC-093),
     `roster_count`;
   - teacher stream (API-042): `occupant_joined {name, connection}`,
     `occupant_left {name, connection}`, `occupant_progress {attempt,
     current_index, last_answer, cheat_count}`, `session_state {state}` +
     `heartbeat` every 30 s.

No OpenAPI YAML is generated for MVP (no third-party API consumers). If a
machine API is later required, it MUST reuse the operation semantics below.
Field names use `snake_case` in JSON; transport aliases map at the boundary:
`type` → `quiz_type`/`qtype`, `quiz_id`/`attempt_id`/`question_id` → `id`,
`shuffle_questions`/`shuffle_answers` → `shuffle_q`/`shuffle_a`,
`submitted_wib` is display-only (derived); words↔codes per ERD §1.

Conventions applying to every operation (API-000):

- Auth: `session_id` HttpOnly cookie (`quiz_session`, SameSite=Lax, Secure in
  prod, 60-minute sliding idle expiry per BR-054, DECIDED DEC-003). Anonymous endpoints use a signed
  `attempt_token` (HMAC, 24 h TTL) instead. Anon token TTL (DECIDED N9, 2026-10-14,
  qualified per DEC-117): the 24 h TTL gates ENTRY only — identify-to-start
  (API-050→051→033). Once the attempt row exists (`answering`/`submitted`),
  membership wins in-flight: answering, submit, result, and cheat endpoints
  re-check row membership, never the TTL. AC-227 asserts this split.
- CSRF: every POST requires either a hidden `csrf_token` form field or
  `X-CSRF-Token` header matching the session. Missing/invalid → ERR-002.
  EXEMPTION (DEC-108, cheat CSRF exemption): `POST /attempts/:id/cheat` (API-03A) is CSRF-exempt (exemption) —
  the beacon cannot reliably attach a token on `pagehide`; cookie session +
  attempt membership + `client_event_id` dedupe + the 2 s debounce window
  replace CSRF there. No other POST is exempt.
- Authorization gates (RBAC, DEC-030/031 — canonical rule, endpoint sections
  state the outcome): attempt endpoints (API-031..039, API-03A, API-040/041)
  require `requireStudent` (teacher role → 403) + attempt ownership
  (`student_id=self`, or single-quiz `attempt_token` scope R3 for anon);
  teacher recap/grading/monitor/controls (API-022/024/042/043/044/060..063/070)
  require `requireTeacher` + quiz ownership (`owner_id=self`, cross-owner → 403).
- Request ID: server generates `req_<ulid>`; returned as `X-Request-ID`
  header and echoed in JSON errors and server logs.
- Time: all timestamps UTC ISO-8601 with millis (`2026-10-12T07:00:00.123Z`).
  Server clock authoritative (BR-013, FLOW-009).
- Idempotency: every rendered start/submit form carries a server-generated
  `Idempotency-Key` (UUID in a hidden field + response header; DECIDED
  DEC-084 — the server, not the client, guarantees the key always exists, so
  double-click and browser-close dedupe always work). `POST .../attempts`
  (start) and `POST .../submit` accept the key; replay within 24 h returns
  the original result without duplicate rows. Forms rendered without JS read
  the key from the hidden field; jQuery AJAX sends the same value as header.
- Errors: page forms → `422` re-rendered HTML with field errors preserved;
  jQuery-AJAX/JSON (`Accept: application/json`) → JSON
  `{code, message, details, request_id}` with the HTTP mapping in
  `../../03-RESILIENCE-AND-ERRORS/01-ERROR-MATRIX.md`. SSE POST failures follow JSON.
  Error precedence (DEC-122 fragment-wins): for list endpoints (API-010/022/060/062),
  `X-Requested-With` fragment WINS — the server returns the HTML
  fragment error, not a full page — UNLESS `Accept: application/json` is
  explicit with a non-list endpoint, which returns JSON. Table:

  | `X-Requested-With` | `Accept` | Endpoint kind | Error shape |
  |---------------------|----------|---------------|-------------|
  | present | anything | list | HTML fragment error |
  | absent | `application/json` explicit | non-list | JSON `{code, message, details, request_id}` |
  | absent | page | any page form | `422` re-rendered HTML |
- Page mapping (DECIDED S2a, 2026-10-14): invalid `?page` values are clamped
  to the valid range by DEC-100 (BR-055, EDGE-063) — `?page=abc` → default (1),
  `?page=9999` or `>200` → last page; AC-220 asserts page-1 and last-page
  rendering for these inputs. Never a 4xx. The closed-view layout (BR-007, DEC-115) and
  the list-fragment template table (below) are both decided; no S2 scope remains open.
- List-fragment template table (S2b, narrowed): for each list endpoint, the
  `X-Requested-With` fragment render maps to one template partial:

  | Endpoint | Fragment template | Rendered when |
  |----------|------------------|---------------|
  | API-010 `/teacher/users` | `users-fragment` | `?q=` search only (non-q filters + pagination = SSR GET, DEC-112) |
  | API-022 `/teacher/quizzes` | `quizzes-fragment` | `?q=` search only |
  | API-060 `/teacher/quizzes/:id/grading` | `grading-fragment` | `?q=` search only |
  | API-062 `/teacher/quizzes/:id/recap` | `recap-fragment` | `?q=` search only |
  | API-030 `/student` | `student-cards-fragment` | full list (DEC-113, no fragment claim) |

  Non-search filters and pagination are SSR GETs (DEC-112); the fragment wins
  only for the `?q=` search path (DEC-122). Closed-view layout (BR-007, DEC-115)
  is owned by the frontend slice, not this spec.
- Input normalization (DEC-111, applies to every endpoint): leading/trailing whitespace
  is trimmed from all free-text fields before length checks, so
  whitespace-only title/name/prompt/class counts as blank (422 ERR-100/401);
  overlong token codes (>12 chars) are 422 ERR-400 format rejects (never
  truncated); `class_label` >50 is 422; option text keeps its BR-004
  case-insensitive-trim uniqueness; malformed answer bodies follow the
  API-035 rule (422 ERR-100, never coerced).

## 1. Auth and accounts

### API-001 POST /login — password login
- Caller/owner: anyone / Auth. Auth: none. No rate limiting in MVP (DECIDED DEC-073).
- Body: `{email, password}` (form or JSON). Validation: email format, both
  present; wrong credentials → generic ERR-001 (no user-enumeration).
- Success: `303 → /` (role dashboard) + `Set-Cookie`; JSON variant returns
  `{user: {id, name, role}, must_change_password}`.
- Side effects: new `sessions` row; forced rotation if `must_change_password`
  (BR-050) → `303 → /change-password`.
- Example request JSON: `{"email":"guru@sekolah.id","password":"Sinta2024"}` (valid per BR-053: ≥8 chars, no whitespace, digit + uppercase).
- Links: BR-050, ERR-001/002, STATE-006.

### API-002 POST /logout — terminate session
- Deletes server session row, clears cookie, closes SSE streams for that
  session. Idempotent (no error if already logged out).

### API-003 POST /register — student self-register
- Body: `{name 1–100, email unique, password per BR-053 (at least 8 characters,
  no whitespace, at least one digit and one uppercase), class_label
  optional, grade_level ∈ {10,11,12} optional}`. Creates `role=student,
  status=active` immediately (BR-051). Role is server-fixed; any `role`
  parameter is ignored. Success → `303 → /login` (manual login, DECIDED
  DEC-087 — NO session is created at register); the student logs in with the
  just-registered credentials.
- Failures: duplicate email → ERR-109 (form: "email already used"); password
  rule violation → ERR-110 with the exact rule text.

### API-004 GET/POST /profile — view/update own profile
- Student may edit `email, name, class_label, grade_level, password`;
  teacher additionally `name, email, password`. Nobody edits own `role` or
  `status`. Email change re-checks uniqueness inside a transaction. Password
  change follows BR-053 and revokes all other sessions in realtime with their
  SSE streams closed at once (BR-054, DEC-032); the changing session survives
  with a rotated csrf_token.

## 2. Teacher user management

### API-010 GET /teacher/users + POST /teacher/users (create teacher or student)
- List follows BR-055: `?q=` (name/email substring) + `?page/&per_page=`
  (SSR default 25, up to 200); realtime 300 ms debounce + replaceState
  (DEC-098); no-JS = plain GET submit. Self-vs-other (DEC-109, widened per
  PRD §3 + Q2, 2026-10-14): a teacher may create BOTH student and further teacher
  accounts (the seeded teacher creates further teachers/students; self-registration never
  creates a teacher — API-003 fixes `role=student` on register). Create is exempt from the
  teacher-to-teacher 403; teacher-to-teacher WRITE (update/deactivate/delete on another
  `role=teacher` row) → 403 ERR-003; self + all students remain writable. Ownership DEC-031 unchanged.
### API-011 POST /teacher/users/:id (update) — name/class/grade/email reset password
- Self-vs-other (DEC-109): teacher-to-teacher write (target `role=teacher`,
  actor ≠ target) → 403 ERR-003, changing nothing; self + all students editable.
### API-012 POST /teacher/users/:id/deactivate | /reactivate | /delete
- All require teacher role (ERR-003 otherwise). Self-vs-other (DEC-109):
  teacher-to-teacher write (target `role=teacher`, actor ≠ target) → 403
  ERR-003; self + all students remain writable. Live-session block (DEC-105):
  deactivate targeting a user who owns a `running`/`paused` matching session
  → 409 ERR-115, changing nothing; self-deactivate carries the same guard.
  Delete follows BR-051/052:
  only from `inactive` + two-step button confirm (explicit dialog buttons, no
  typed text, DEC-042); attempts anonymised, never dropped. New/changed passwords
  follow BR-053. Deactivate revokes sessions and closes SSE
  immediately; attempt rows persist (deactivate is guard-BLOCKED, not
  force-submitting, while an open quiz or a live session exists — BR-051,
  DEC-081/DEC-105, so no sessionless submit is ever created by this path);
  the teacher must stop the session, wait for submit/auto-submit or close the
  quiz first.
  Owned-quiz guard (DECIDED DEC-081/DEC-092, Q3 2026-10-14): deactivate is
  REJECTED with 409 ERR-115 whenever the target owns any `open` quiz. The
  auto-close clause is superseded by the user decision 2026-10-14 (N-A5): no
  quiz is ever closed automatically by this path, and in-flight attempts
  continue normally (AC-126B semantics). The guard is checked inside the
  deactivate transaction, so a blocked action changes nothing. No quiz is ever
  left open-but-unmanageable: the steward teacher closes or hands over each
  owned quiz first (there is no ownership transfer endpoint, DEC-031 strict).
- Self-delete (DEC-129 companion): both roles may self-delete via
  `inactive` + two-step confirm; a teacher self-delete is BLOCKED while owning
  ANY quizzes (any status) to honor no-orphans (conservative), audit-logged.

## 3. Quizzes and questions

### API-020 POST /quizzes — create quiz
- Teacher only. Body: `{title 1–200, description 0–2000 optional,
  type normal|matching, max_attempts 1–10 (ignored if matching → 1),
  shuffle_questions bool, shuffle_answers bool,
  timer_mode none|per_question|total, timer_seconds custom teacher-typed
  1–86400 (NULL iff mode none; DEC-005),
  visible bool default true, show_score bool default true,
  show_correct bool default false, token_enabled bool default false}`.
- `timer_mode=per_question` is allowed on `matching` with guru-full semantics
  (DEC-119 mirror of BR-003): no auto-advance — the teacher controls pacing.
- Creates `status=draft`. Response `303 → /teacher/quizzes/:id/edit`.
  Teacher only, own quiz scope from creation (`owner_id=self`, DEC-031).
- Errors: ERR-101 (timer; stray `timer_seconds` with mode `none` → reject, never ignored).

### API-021 POST /quizzes/:id — update (freeze-guarded by BR-001/BR-006, DEC-103 hidden/show rule)
- Freeze rule mirrors BR-006 (business owns wording): `open+hidden+registered-only`
  editable; `open+show+any row` frozen; `answering`/`submitted` rows always
  frozen regardless of `visible`.
- Editable even when frozen: title/description/visible/show flags/token toggle.
- Editable only when NOT frozen (draft pre-live, or open+hidden with zero
  `answering`/`submitted` rows — registered-only): timer, shuffles,
  max_attempts, and `questions` (add/edit/delete per BR-006/DEC-103 — DECIDED
  N17, 2026-10-14; the editable list previously dropped `questions`). `type`
  is NEVER editable after creation (BR-001 — any type change attempt → 422
  ERR-106, even with zero rows).
  Frozen-field mutation → ERR-107. Token code change checks
  uniqueness among open+enabled quizzes → duplicate yields 409 ERR-114 with a
  fresh suggestion (BR-040).

### API-022 POST /quizzes/:id/open | /close | /visibility | /delete
- Open requires BR-005 (ERR-108 if empty/invalid). Close keeps in-flight
  attempts submittable; blocks new register/start. Matching sessions (DEC-093):
  close/hide while `waiting` auto-finishes the session (SSE `room_closed`,
  occupants' rows stay no-show + auto-revoke on close); close/hide while
  `running`/`paused` → 409 "hentikan sesi dulu" (stop first); from the quiz
  list directly (DEC-094 — no stop required when no live session runs).
  Delete requires zero rows
  with `state` in (`registered`, `answering`, `submitted`) on that quiz (BR-052
  guard, DEC-082 — blocked delete → 409 ERR-111, changing nothing).
- Reopen mirror (DEC-126/127): reopen does NOT reset `session_state` —
  `finished` is permanent (finished-permanent; STATE mirror, business owns wording).
- Teacher quiz list supports status filters All/Draft/Open/Closed plus search
  (source doc `docs/DESIGN.md` §6.3): server-side `status` predicate + title substring; filters
  are distinct from show/hide visibility (BR-007). Query keys per BR-055:
  `?status=` + `?q=` (title substring) + `?page/&per_page=` (SSR default 25,
  up to 200); searchbar debounces 300 ms with replaceState (DEC-098).

### API-023 POST /quizzes/:id/questions — add question (multipart)
- Fields: `type`, `prompt 1–5000`, `options[] {text 1–500, is_correct}` for
  choice types, `image` file + optional `image_alt` ≤500 chars (BR-010). The exact
  size limit is 10 MiB = 10485760 bytes (Q8, 2026-10-14): files above 10485760 bytes
  are compressed to ≤10485760 bytes, files at or below promote unchanged. The editor form
  carries a server-generated intake key for logging only (DEC-107 — same DEC-084
  pattern, DECIDED DEC-095): the key is server-generated for logging/traceability,
  with NO duplicate-suppression guarantee on timeout-retry — no echo promise,
  no echo guarantee (a retry may create
  a second row; dedupe is not promised here). Intake validates format/count
  synchronously and returns at once with `image_status=processing`; compression
  runs in the background worker pool (F-004). Queue full → ERR-112 (429) with
  `Retry-After: 1` header (DECIDED S1a, 2026-10-14 — time-based retry; 409s are
  conflicts, not time-based, so no header there). Client backoff: wait the
  indicated second, then retry once; AC-105 asserts the header on the 429 path.
  Unreadable/corrupt/timed-out job → row marked `failed` with reason, teacher
  re-uploads (reason codes reuse ERR-103/104 text).
- Example (jQuery AJAX, JSON without image — note `contentType: "application/json"`, plain `$.post` would form-encode the string):
  `$.ajax({url, method: "POST", contentType: "application/json", data: JSON.stringify({"type":"multiple","prompt":"Which are prime?","options":[
  {"text":"2","is_correct":true},{"text":"3","is_correct":true},
  {"text":"4","is_correct":false}]})})` → `201 {question_id}` (JSON Accept) or HTML partial (page form). `oid` values are server-assigned ordinals, never client-chosen.

### API-024 POST /questions/:qid (+ /delete)
- Same validation; freeze-guarded (ERR-107). Delete re-numbers `position`
  transactionally. Deleting the last remaining question of an `open` quiz is
  BLOCKED with 422 ERR-108 (maintain-gate, BR-005/DEC-089 — close first, then
  delete); drafts may go to zero freely. Image replace deletes old file after commit (never before).

## 4. Student quiz lifecycle (normal)

### API-030 GET /student (list) — visibility predicate BR-007 (DECIDED N16, 2026-10-14)
- Route identity aligned to `/student` everywhere (N16): the student full-list
  endpoint is `GET /student`, not `/student/quizzes`. All references, nav, and
  prototype shells use `/student`; the old `/student/quizzes` alias is retired.
- Returns only `open+visible` (matching `finished` excluded). Each card:
  `{quiz_id, title, type, question_count, timer_mode, attempts_used,
  attempts_max, registration_state}` where `attempts_used` = COUNT submitted
  rows by this taker, `registration_state` ∈ `none` (no active row) |
  `registered` | `answering` (latest active row), and the card additionally
  carries `exhausted=true` when `attempts_used >= max_attempts` (start would
  410 ERR-208). Student list is FULL (DEC-111/112/113 REWRITE): no search or
  slice keys, no slicing claim — the server always returns every eligible card.
  Hidden (DEC-115 hidden) direct GET → 404; closed+visible direct GET → 200 closed
  view (no join; existence leak accepted as decided).

### API-031 POST /quizzes/:id/registrations — register
- Guards: quiz open+visible, no active (registered/answering) row, attempts remaining.
  Matching session lock (DECIDED DEC-093, R8): register once the session left
  `waiting` (`running`/`paused`/`stopped`) → 410 ERR-301 (room locked — same
  code as late enter); such never-present rows never occur. Close/hide of the
  quiz ends a `waiting` session (see API-043), so no stranded no-show survives.
  Hidden-mid-click (visible flipped false while the form was open) → 404 per
  BR-007 (indistinguishable from missing); closed-mid-click → 410 ERR-202.
  Register is the PRIMARY exhaustion gate (DEC-083): when
  `completed_attempts >= max_attempts`, reject before INSERT (410 ERR-208,
  no row).
- Storage (R2): INSERTs an `attempts` row with `state=registered` (no separate registration table); the same row later advances to `answering` at start.
- Returns `303 → waiting/enter` (matching) or `303 → start page` (normal).
- Duplicate → ERR-201 (echo existing); closed mid-click → ERR-202.

### API-032 POST /registrations/:id/cancel (student) and /revoke (teacher)
- Guard BR-008: only while the row is still `registered`. Post-start → ERR-207 —
  EXCEPT the teacher may still revoke a never-present no-show row (`last_seen IS
  NULL`, DEC-088) even after session start (the only post-start revoke; it holds
  no frozen snapshot).
- Replay on an already-`cancelled`/`revoked` row → `200` echo of the terminal
  state (same echo philosophy as ERR-201/203; no new row, no error).
- Anonymous participants hold no `registered` row (memory intent only) — there
  is nothing to cancel; abandoning = discarding the token (intent TTL).
- Storage (R2): UPDATEs the same `attempts` row to `cancelled`/`revoked` (terminal codes on the row).

### API-033 POST /quizzes/:id/attempts — start (Idempotent)
- Key: the start form carries a server-generated `Idempotency-Key` (DEC-084,
  hidden field + header); the client returns it verbatim. Teacher role is rejected here
  (`requireStudent`; teachers never sit quizzes, DEC-030). Server transaction: lock the registered row, re-check
  guards, compute `attempt_no`, freeze `question_order` /
  `option_orders` permutations (Fisher–Yates, `crypto/rand`), snapshot correctness into
  `answer_key_json` plus `question_snapshots` and an empty `answers_json` skeleton, compute deadlines, UPDATE the same row to `answering`.
- Anonymous (`attempt_token`, API-051): no `registered` row exists — start INSERTs
  `state=1` directly with `taker_label` + the server-generated key (same DEC-084
  pattern as accounts: key-first lookup before INSERT, so double-click/retry
  echoes, never a second row); guards: live
  intent + quiz open + visible + code still valid, else generic ERR-400; quota
  (DEC-091): COUNT `submitted` rows with the same `taker_label` on this quiz
  `>= max_attempts` → 410 ERR-208, no row. One-active check (DEC-106 one-active): the same
  `taker_label` with an active intent/`answering` row → 409 conflict; quota
  exhausted for that label → 410 ERR-208.
- Response: `303 → /attempts/:id/questions/0` + JSON variant
  `{attempt_id, total_questions, timer_mode, attempt_deadline_utc, server_now_utc}`.
- Failure: beyond `max_attempts` → 410 ERR-208 with the attempts-exhausted
  message (BR-002/009, AC-115). Register is the primary gate (no row on
  register-time exhaustion). If a `registered` row exists but start finds
  attempts exhausted (max lowered, or double-tab race), start rejects with
  410 ERR-208 AND auto-cancels that stranded row (`registered → cancelled` in
  the same transaction, audit-logged — DEC-083), so no unstartable row survives.
- Opens SSE (API-036) on navigation.
- Stuck answering rows on timer-less quizzes (DECIDED N5, 2026-10-14, accepted
  as documented): `timer_mode=0` (none) quizzes have no deadline and no sweep
  (ERD §9 — no auto-submit path exists), so an `answering` row never transitions
  on its own. Its `taker_label` stays permanently 409/410-blocked under
  DEC-106 (one-active) and DEC-091 (quota) — the teacher's only recovery is to
  close the quiz (which finishes the session and revokes the row) or to revoke
  it manually. This is the accepted, documented behavior; no stale-row sweep
  is implemented. AC-224/150 assert the block and the no-sweep property.

### API-034 GET /attempts/:id/questions/:n — one question page
- Returns question N in frozen order with shuffled options (correct flags
  NEVER rendered). Per-question mode: `n < current_index` → 410 read-only;
  `n > current_index` → 403 (no skipping ahead). Total/none: any `0..N-1`.
- Includes `server_now_utc`, `attempt_deadline_utc`, `cheat_count` for client sync.

### API-035 POST /attempts/:id/questions/:qid/answer — save answer
- Body single: `{selected_option_id}`; multiple: `{selected_option_ids[]}`;
  essay: `{essay_text 0–10000 chars}`. Empty allowed (records NULL = 0).
  Malformed body (wrong JSON type — e.g. scalar where an array is required,
  non-object, unknown `qid` not on the snapshot) → 422 ERR-100 with a field
  error, changing nothing (never coerced, never partially applied).
- Guards: attempt `answering`, server time `now <= attempt_deadline_utc` (zero grace,
  DEC-006; past deadline → ERR-206), session not paused
  (ERR-302), per-question lock not passed (ERR-205). Returns `{saved:true, server_now_utc,
  current_index, cheat_count}`.
- Retry safety: same payload twice = overwrite, no duplicate; safe to retry
  on network loss after timeout.

### API-036 GET /attempts/:id/events — attempt SSE (student)
- Events: `heartbeat`, `question_locked {qid, advance_to}`,
  `time_expired`, `session_paused`, `session_resumed`, `session_stopped` (matching),
  `cheat_recorded {cheat_count}`, `graded {final_score}` — the graded event is
  result-page-only (DEC-104, Q5 2026-10-14): it fires on the open stream for the
  final result page after submit (`GET /attempts/:id/result`), never on the review
  page and never on any card surface. No replay: the stream is live-only; a dropped connection delivers
  nothing retroactively — the F-002 notice + user refresh is the recovery
  (DEC-102). Names are canonical per the §0 event catalog — no aliases.
- Heartbeat payload (DECIDED S4a, 2026-10-14): every stream emits a
  `heartbeat` event every 30 s carrying `{server_now_utc}` — the same field
  already canonical in answer envelopes (API-034/035). Dead-stream semantics
  (unchanged, DEC-102): 45 s with no event → client treats as dead
  (`EventSource.close()` + F-002 notice); a >60 s-stale occupant stays
  EVER-PRESENT — stale presence never marks an occupant never-present, and such an
  occupant's answers are auto-submitted on stop exactly like any present occupant
  (only `last_seen IS NULL` rows are no-show, DEC-088/STATE-007/N3 — corrected per
  Q4, 2026-10-14). Submit-vanish is already decided
  (API-044 + FLOW-007 + C-006 + AC-209: the answering row disappears from the
  monitor on submit) — the S4 vanish half is closed; the event name stays
  `occupant_left`-style row removal, no new event.

### API-037 GET /attempts/:id/review — review page (total/none only)
- Lists all N with answered/unanswered + jump links. Per-question mode →
  read-only trail (BR-011).

### API-038 POST /attempts/:id/submit — submit (Idempotent)
- Grades objectives synchronously in-transaction (BR-020), sets
  `submitted`, closes SSE (DEC-104: the close stays; the graded result renders
  on next render, never pushed to the closed stream). Duplicate key or double click → same receipt
  (ERR-203 mapped to 200 echo). Past-deadline submit: the timer path has
  already auto-submitted — returns `200` with the auto-submitted receipt
  (`auto_submitted=true`), NOT a 4xx error (DECIDED DEC-086; ERR-206 covers
  per-question answer saves only, never the submit itself).

### API-039 GET /attempts/:id/result — thank-you card
- `submitted` only: direct GET on a `registered`/`answering` row → 303 to the
  current question page (total/none) or waiting room (matching), never the
  card. Always `200` post-submit: `"Thank you for completing quiz <title>"` +
  temporary/final score and per-question correctness strictly per BR-030/031.
  Essay pending → "awaiting grading" placeholder. Printable; the only receipt
  for anonymous participants.
- Cancelled/revoked result card (DECIDED S5, 2026-10-14): a `cancelled` or
  `revoked` row renders a terminal-state card — `"Percobaan dibatalkan/dicabut"`,
  no score, no correct answers, printable. No receipt is issued for never-present
  no-shows (`last_seen IS NULL` registered rows) — recap already excludes them
  (DEC-088), and there is nothing to print. AC-206 asserts this leg; S5 is closed.

### API-039b GET /attempts/history — student attempt history (DECIDED N10, 2026-10-14)
- Fills the history-list gap (BR-055 + spec closing note): lists the calling
  student's submitted attempts, newest-first, under the query contract.
  This operation stays a separate operation (not folded into API-062) per Q6
  (2026-10-14), is advertised in `00-INDEX.md` §2, and is asserted by AC-233.
- Path: `GET /attempts/history` (account; `requireStudent`, R1 `student_id=self`
  predicate). No `attempt_token` variant — history endpoints do NOT accept
  `attempt_token` (API-051), so anonymous participants have no history view;
  their only record is the single result card (API-039).
- Row source: `attempts WHERE student_id=? AND state=2 ORDER BY submitted_at DESC`
  (schema §6 hot-read; `quiz_title`/`quiz_type`/`taker_label` snapshots mean no
  quizzes/users JOIN). Each row: `{attempt_id, quiz_id, quiz_title, quiz_type,
  attempt_no, final_score, objective_score, essay_score, duration_s,
  submitted_at, auto_submitted}`.
- Query keys (BR-055/DEC-100): `?q=` (quiz_title substring), `?page/&per_page=`
  (SSR default 25, max 200), `?window=`, `?class=`, `?date=`, `?status=`.
  `?status=` values (DECIDED N1, 2026-10-14): `submitted` (default, the only
  history-eligible state — history lists only `state=2` rows) and `all`
  (treated as `submitted`); unknown clamped per DEC-100. Parity with recap
  `?status=` (API-062/063, N1). Unknown keys ignored; invalid values clamped
  (DEC-100, never 4xx). Resume
  link: rows with `state=2` are terminal — reopen goes to the result card, not
  the attempt (finished-permanent, DEC-127).
- Errors: 403 for teacher role (`requireStudent`); 404 if the student row is
  missing/inactive. No write path here.

### API-03A POST /attempts/:id/cheat — focus-loss beacon
- Body: `{client_event_id uuid, event_type hidden|blur|pagehide,
  occurred_at}`. Server locks the attempt row: requires `state=answering`
  (POST on a `submitted`/`cancelled`/`revoked` row → idempotent echo of the
  current count, never an increment); if `eid` already exists in
  `cheat_events_json`, returns the current count unchanged (idempotent echo);
  else if `now − max(occurred_at) < 2 s` (server time, independent of `eid`),
  the event is ignored (BR-032 debounce — fresh-`eid` spam cannot inflate);
  else appends the event + `cheat_count++` in one UPDATE → 200 `{cheat_count}`. Sent with NATIVE
  `navigator.sendBeacon` fallback to NATIVE fetch `keepalive:true` (never
  jQuery — jQuery has no beacon transport, UI-002). Never fails the attempt; errors swallowed client-side
  after one retry. CSRF-exempt per API-000 (DEC-108): cookie + membership +
  `client_event_id` dedupe + 2 s window replace CSRF here.

## 5. Matching control and monitor

### API-040 POST /matching/:id/enter — join waiting room
- Marks the occupant's `attempts` row present (`SET connected=1, last_seen=now()`
  in the same UPDATE — the write that makes DEC-088 ever-present true) + ephemeral presence (R2: no waiting-entry table); requires
  a `registered` row — OR, for anonymous holders, a live memory intent (API-051),
  whose presence is recorded in the anon set and materialized to `state=1` rows
  at start. After lock (`running`+) → ERR-301.

### API-040b POST /matching/:id/leave — leave waiting room (DEC-121)
- Pre-start only (`waiting`): sets `connected=0`, keeps `last_seen`, broadcasts
  `occupant_left`; post-lock (`running`+) → ERR-301, changing nothing.
- Anon leave representation (DECIDED N4, 2026-10-14): account occupants flip
  the row (`connected=0`, `last_seen` kept — DEC-121). Anonymous pre-start
  presence is memory-only (DEC-096: no `attempts` row exists to flip), so an
  anon leave clears the in-memory intent set for that `taker_label` and the
  `occupant_left` broadcast is identical to the account case (same event name
  and payload shape, RBAC §1). AC-229 asserts both representations.

### API-041 GET /matching/:id/events — matching SSE (student)
### API-042 GET /teacher/matching/:id + /events — matching control + teacher SSE (DEC-110)
- Split: matching control + SSE = `GET /teacher/matching/:id` (control surface)
  + `GET /teacher/matching/:id/events` (this API-042 stream); normal monitor +
  matching live rows = `GET /teacher/monitor/:quizId` + `/events` (API-044).
  Both surfaces may open; version is re-read per surface.
- Student events: `session_started {attempt_id}`, `session_paused`,
  `session_resumed`, `session_stopped {result_url}`, `room_closed` (DEC-093 abort),
  `roster_count`.
- Teacher events: `occupant_joined {name, connection}`,
  `occupant_left {name, connection}`,
  `occupant_progress {attempt, current_index, last_answer, cheat_count}`,
  `session_state {state}` (DEC-120: no per-cheat push event here — cheat is
  visible via `occupant_progress.cheat_count`; the student-stream-only cheat
  event stays per API-036). Heartbeat 30 s both. Names canonical per §0.
- S4 heartbeat payload shape DECIDED (API-036: `{server_now_utc}`, 30 s beat / 45 s dead / 60 s stale); submit-vanish behavior DECIDED (API-044 + FLOW-007 + AC-209: row disappears on submit). The vanish event name is declined per DEC-102 refresh (no new event; row removal follows `occupant_left`-style). This endpoint is a consumer of those contracts — see API-036 (DECIDED S4a, 2026-10-14).

### API-043 POST /matching/:id/start | /pause | /resume | /stop | /finish
- Teacher only, optimistic `session_version` check on the quizzes row (409 ERR-300 on race).
  Start transitions all EVER-PRESENT occupant `registered` rows
  (`connected=1 OR last_seen IS NOT NULL`, DECIDED DEC-088) to `answering` with
  frozen shuffle (R2 same-row); never-present rows (`last_seen IS NULL`) stay
  `registered` marked no-show — never auto-submitted, excluded from recap
  zeros (revocable post-start, BR-008 exception); only entered anon intents
  materialize (the rest expire by TTL); stop
  auto-submits all `answering` (FLOW-003); finish is terminal. Double verbs are
  idempotent echoes. Monitor single index note (DEC-119): monitor/rows reads
  elsewhere use one `(quiz_id, state)` scan for both normal and matching rows.
  Start with 0 EVER-PRESENT occupants (never-present no-show
  rows do not count) is BLOCKED with 422 ERR-113 + message (DEC-011).
  Empty-room exit (DECIDED DEC-093/094, R8): the teacher ends a `waiting`
  session directly — `waiting → finished` via `POST .../finish` when zero
  EVER-PRESENT occupants (explicit abort: SSE `room_closed` to the empty room,
  audit-logged; NOT an error); close/hide of the quiz ends a `waiting` session
  the same way (auto-finish on close/hide, SSE `room_closed`); occupants'
  `registered` rows stay no-show and auto-revoke on close (no revoke click
  needed). Close/hide while the session is `running`/`paused` → 409 with
  "hentikan sesi dulu" (stop first); stop/finish stay the only exits from a
  live session.
- Re-click guard (DECIDED DEC-085, refined R8): the server rejects the SAME control verb
  repeated for the same quiz within 10 s of its previous acceptance with 429
  ERR-116 ("tunggu 10 detik sebelum mengulang perintah") + the current state,
  changing nothing — but ONLY when the repeat carries a stale `session_version`
  (the accidental-double-click shape: both clicks carry the pre-click version).
  A repeat with the CURRENT version (client re-read state, then re-POSTed —
  intentional) → 200 echo, never 429. After 10 s, any same-state repeat → 200
  echo. A retry after a FAILED verb (503/409 — never accepted, no guard
  timestamp) follows the normal echo-vs-409 rule, never 429. Distinct
  verbs (pause→stop) always pass the guard. The guard is enforced in-memory
  per quiz (single process, ARCH-004); a restart resets the 10 s window
  (documented). Rule: same verb + stale version + within 10 s → 429;
  same verb + current version → 200 echo; state-changing verb on a stale
  version → 409 ERR-300 + current state.
- Waiting verbs waiting-echo (DEC-126/127): `pause`/`stop`/`resume` on `waiting` → 200 echo,
  no state change:

  | Verb on `waiting` | Result |
  |-------------------|--------|
  | `pause` | 200 echo, stays `waiting` |
  | `stop` | 200 echo, stays `waiting` |
  | `resume` | 200 echo, stays `waiting` |
- Reopen (DEC-126/127 finished-permanent): reopen does NOT reset `session_state` — `finished` is
  permanent (STATE mirror).
- Finished-session verb contract (DECIDED N2, 2026-10-14): on a `finished`
  quiz, control verbs (`start`/`pause`/`resume`/`stop`/`finish`) → 409 ERR-300
  with the current state (no state change); submit (`POST /attempts/:id/submit`)
  → 200 receipt echo if the attempt was already submitted (DEC-086, idempotent),
  else the normal guard applies (410 if past deadline, 409 if not `answering`,
  etc.). AC-232 asserts this split; EDGE-073 routes submits via the normal
  state checks, not the control-verb path.

### API-044 GET /teacher/monitor/:quizId (+ /events SSE) — live monitor (DEC-110)
- Split mirror: normal monitor + matching live rows = `GET
  /teacher/monitor/:quizId` (rows surface) + `/events` (this stream); matching
  control + SSE = `GET /teacher/matching/:id` + `/events` (API-042). Both may
  open; version re-read per surface.
- Normal: rows of `answering` attempts (name, q-index/N, last answer
  summary, review flag, cheat count, connection). Row vanishes on submit.
- Matching: roster (pre-start) + same rows (running) + control buttons.
  Never-present no-show rows stay `registered` with a "tidak hadir" badge,
  never auto-submitted (DEC-088). No polling variant exists: when the monitor
  stream dies, the teacher sees the F-002 persistent notice and refreshes to
  rebuild rows from server truth (DEC-102). Monitor uses a single index note
  (DEC-119): one `(quiz_id, state)` scan serves both normal and matching rows.
- S4 heartbeat payload shape DECIDED (API-036: `{server_now_utc}`, 30 s beat / 45 s dead / 60 s stale); submit-vanish behavior DECIDED (this endpoint: row disappears on submit, AC-209). The vanish event name is declined per DEC-102 refresh (no new event). This endpoint is a consumer of the API-036 contract — see API-036 (DECIDED S4a, 2026-10-14).

## 6. Anonymous token mode

### API-050 GET /join + POST /join — code entry
- Body `{code 6–12 alnum}`. Lookup among `token_enabled=true AND status=open AND visible=true` only; miss →
  generic ERR-400 (anti-enumeration; a hidden quiz's code stops working, BR-040). Success → `303 → /join/:quizId/identify`.

### API-051 POST /join/:quizId/identify — name capture
- Body `{display_name 1–100, class_label optional}`. Creates in-memory intent
  (TTL 24 h) + signed `attempt_token` cookie (NOT a login session) + a
  `csrf_token` bound to that attempt token (issued at identify, rotated never
  within the attempt). Every subsequent anon POST sends it as hidden field or
  `X-CSRF-Token` header, else ERR-002 — the CSRF-every-POST rule (API-000)
  applies to attempt-token callers too. Then
  identical to API-033..039 with `taker_label` labelling (`student_id NULL`); no history/profile
  endpoints accept `attempt_token`.
- TTL 24 h is the entry-gate TTL (entry gate, DEC-117): it gates identify-to-start only;
  answering/result legs use membership, not the clock. No sessions change
  (intent stays memory-only).

## 7. Grading and recap

### API-060 GET /teacher/quizzes/:id/grading — essay queue (oldest first)
- Paginates per BR-055 (`?page/&per_page=`, SSR default 25, up to 200).
### API-061 POST /attempts/:aid/essays/:qid/grade — grade {grade 0–10 int}
- Guards: attempt `state=submitted` (grade on `answering`/`registered` →
  409 ERR-300-class `grade_not_ready`, changing nothing); `qid` must be an
  essay question (grade on a choice `qid` → 422 ERR-102, changing nothing).
  Recomputes `final_score` (BR-022) in-transaction + audit entry; pushes SSE
  `graded` if student viewing (DEC-104: close stays — the graded result renders
  on next render; the closed stream is never reused). Re-grade overwrites (ERR-105 on domain fail).
  A submit racing the grade commits first (row lock); the grade then applies
  to the submitted row normally — no silent overwrite of unsubmitted work.

### API-062 GET /teacher/quizzes/:id/recap — teacher recap table
- Every SUBMITTED attempt (`state=submitted` only; cancelled/revoked/no-show
  `registered` rows excluded, never-present no-show rows excluded from recap entirely (not submitted) per DEC-088),
  newest-first: `{attempt_id, taker_label (account snapshot or
  anon display name shown with `(anon) ` prefix), taker_class, attempt_no,
  per-question correctness, objective_score, essay_score, final_score
  (on-screen `total` = `final_score`, same names as schema columns),
  cheat_count, cheat_overflow (a queued event was dropped before a flush after an offline gap, F-005),
  last_cheat_at (UTC ISO of the newest recorded event, NULL when none —
  derived `max(cheat_events_json.occurred_at)`, not stored),
  duration_s, submitted_at, auto_submitted}`. Monitored-vs-unmonitored is read
  from existing columns, not a new flag: `last_seen NOT NULL` + `cheat_count=0`
  = monitored-clean; `last_seen NULL` + `cheat_count=0` = unmonitored (JS-off,
  EDGE-045). `last_cheat_at` is derived (`max(cheat_events_json.occurred_at)`,
  ISO UTC, NULL when none) — never a stored column; both screen and export use
  the same key (`last_cheat_at_utc` in XLSX headers is the same value under the
  the export's `_utc`-suffixed header convention). Pagination
  follows BR-055 (SSR default 25, `?per_page=` up to 200; newest-first).
  List keys: `?q=` (taker_label substring), `?page/&per_page=`,
  `?window=`, `?class=` (exact taker_class), `?date=` (Jakarta day),
  `?status=` (DECIDED N1, 2026-10-14 — recap/export parity). The `?status=`
  value set is `submitted` (default; the only recap-eligible state, BR-014) and
  `all` (treats as `submitted` — recap has no other terminal state to show;
  cancelled/revoked/no-show rows are excluded by predicate, never by filter).
  An unknown `?status=` value is clamped to `submitted` (DEC-100, never 4xx).
  API-062 (screen) and API-063 (export) apply the identical predicate and
  ordering, so screen/export parity holds by construction; AC-220/222 are
  deterministic with this definition.
  Realtime: the recap searchbar debounces 300 ms then GETs the same canonical
  URL (`?q=` + reset `page=1`) via jQuery AJAX partial swap +
  `history.replaceState` (replaceState-only, no history entries — back exits the list; no-JS = plain GET submit).
  On-screen recap is the
  primary surface; one-click export is API-063 (DECIDED DEC-002).

### API-063 GET /teacher/quizzes/:id/recap.xlsx — recap export (DEC-002, DEC-116)
- Teacher only, own quiz (ownership DEC-031). Streams one XLSX workbook
  (single sheet `Recap`, header bold + freeze pane, no other styling):
  columns `attempt_id | taker_label | taker_class | attempt_no | state |
  objective_score | essay_score | final_score | cheat_count | cheat_overflow |
  last_cheat_at_utc | duration_s | submitted_at_utc | submitted_wib | auto_submitted |
  per_question (compact tokens per qid: `ok` objective-correct, `miss`
  objective answered-wrong, `empty` unanswered, `locked` auto-locked,
  `pending` essay-ungraded, `grade:N` essay graded N) — where `submitted_wib`
  is `submitted_at` + 07:00 (Asia/Jakarta, no DST) rendered `YYYY-MM-DD HH:MM:SS WIB`.
  Same predicate as API-062 (`state=submitted` only) and same ordering and visibility
  (newest-first, repeats and anonymised rows included). Filters honored:
  `?window=` + `?class=` + `?q=` + `?date=` + `?status=` (same values and parity
  as API-062 per N1); ignores page
  (`?page`/`?per_page` have no effect — always full filtered view, same order as screen).
- Generation: streaming row writer from the recap SELECT (no full in-memory
  materialization; bounded by attempt count). Filename
  `recap-<quiz_id>-<yyyymmdd-hhmm>.xlsx`. Content-Type
  `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`.
  Failure mid-stream → truncated download is detected client-side by size
  mismatch + error toast; retry is safe (read-only). No CSV/PDF writers exist.
- Links: BR-014/052, FLOW-007, ERR-5xx, AC-219.

## 8. Dashboards

### API-070 GET / (role dashboard) — overview analytics (DEC-004)

Window `?window=3d|7d|30d|all` filters on `attempts.submitted_at` (submitted
attempts only; `answering` rows never enter aggregates). All aggregates are
single GROUP BY queries over `attempts` (no JOINs, R2). Architect-recommended
formulas below are DECIDED (DEC-004, 2026-10-12); they count what the product
already stores, no new concepts.

- Student (scope: own `student_id` only):
  - `attempts` = COUNT rows `state=submitted` in window.
  - `total_score` = SUM(`final_score`) over those rows (2-dec).
  - `average_score` = `total_score / attempts` (NULL when 0 attempts; renders "—").
  - `answered` = SUM over rows of answered entries in `answers_json`
    (selected non-empty OR essay_text non-null); `unanswered` = total
    questions across rows − answered; `questions_done` = answered.
  - Cards: open+visible quizzes, registered (`state=0`) rows, closed+visible
    quizzes (BR-007 predicate per card).
- Teacher (scope: own quizzes `owner_id=self`, DEC-031):
  - `total_students` = COUNT DISTINCT `student_id` (non-null) with ≥1 submitted
    attempt on own quizzes in window (all-time default includes anon? NO —
    anonymous counted separately, see below).
  - `total_quizzes` = COUNT own quizzes (any status); `total_questions` =
    COUNT questions on own quizzes; `total_attempts` = COUNT submitted
    attempts on own quizzes in window.
  - `scores_given` = SUM(`final_score`) over those attempts (label: "total
    skor diberikan"); `pending_essays` = COUNT `answers_json` entries with
    `kind=essay AND grade IS NULL` on submitted attempts (grading queue depth).
  - `cheat_total` = SUM(`cheat_count`) over those attempts.
  - `anon_attempts` = COUNT submitted attempts with `student_id IS NULL`
    (reported as its own line, never merged into `total_students`).
- Daily Pulse panel (teacher dashboard, source doc `docs/DESIGN.md` §4.4; organizes existing
  data only): today's submitted count + today's cheat total + live
  `session_state` of own matching quizzes + at most ONE attention reminder
  (pending_essays>0 takes precedence over cheat_total>0; else none). No new
  scheduling, recommendation, or notification concept.

Idempotency, pagination (`page/per_page`, max 200), and concurrency (409 +
current-state echo) apply uniformly. List endpoints other than the student
full list (quiz lists, recap,
grading queue, history, user mgmt) follow the BR-055 query contract
(`?q/page/per_page/window/class/status/date`, SSR default 25, max 200,
unknown keys ignored, invalid values clamped to defaults). All operations link to flows
(FLOW-001..009), rules (BR-###), states (STATE-###), errors (ERR-###), and
tests (AC-###) via the traceability index. References only (owned by other
slices): EDGE-065..073, AC-223..232.
