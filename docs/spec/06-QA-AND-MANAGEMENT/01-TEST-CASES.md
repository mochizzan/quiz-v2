# Test Cases — quiz-v2 (verification matrix)

Status: DRAFT | Owner: 06-QA | Updated: 2026-10-14 (F-65/F-67/F-68/F-69/F-70/F-71 closed: AC-108 parent link, AC-115 N5 timer-less tail, AC-102 intake-key alignment, AC-208 payload-field owners named, AC-221 back-button rationale, ERR-107/108 precedence added to AC-101/AC-113)

Given-When-Then, objectively observable. Each case: linked IDs, setup, action,
expected system + persisted + response/UI state, and level (unit/integration/
e2e/manual as known). No unobservable assertions. No wiring/tautology tests.

## A. Authoring and quiz config

- **AC-101** (BR-001/004/005, FLOW-001, EDGE-027 delete-last; integration): Given teacher creates
  quiz + adds single (2 opts/1 correct), multiple (3 opts/2 correct), essay,
  When opens, Then status=open, student lists it; Given empty title → 422
  ERR-100 with values preserved; Given empty quiz, When
  opens, Then 422 ERR-108 and draft persists; Given `type` change attempted
  (even with zero attempts), Then 422 ERR-106 and type unchanged (BR-001);
  Given delete of the last remaining question of an `open` quiz, Then 422
  ERR-108 and question kept (DEC-089 maintain-gate — close first, then delete);
  Given an essay-only quiz, When opens, Then 422 ERR-108 and
  the gate is derived from the question set, no ESSAY-only acknowledgement
  checkbox exists (Q9, 2026-10-14).
  **ERR-107 vs ERR-108 precedence (F-70, closed 2026-10-14 — BR Precedence
  ladder):** given a delete that would BOTH empty an `open` quiz AND violate
  the freeze rule, Then 422 **ERR-107** (freeze runs first and wins), not
  ERR-108 — the ordering is defined in `03-BUSINESS-RULES.md` §Precedence; the
  ERR-108 leg above applies only when no freeze violation exists.
- **AC-102** (BR-004, DEC-107; integration): duplicate option text / 11 options / zero
  correct single → 422 ERR-102, no row written; double-submit of question create
  is prevented client-side (DEC-107: button disable + in-flight `jqXHR.abort()`); Given a duplicate was created, When teacher
  deletes it, Then the duplicate is deleted manually (there is NO server-side duplicate claim: no intake key is stored, none is echoed back, and no server dedupe or
  duplicate-suppression guarantee exists — the intake key, where the editor carries one, is logging-only and never changes an outcome, FLOW-001 step 3 / DEC-107).
- **AC-103** (EDGE-003, ERR-100; unit+integration): essay 10.001 chars → 422, no
  silent trim; 10.000 → accepted.
- **AC-104** (BR-010, DEC-021; integration): EXE-as-jpg, 2nd image, bad MIME at intake →
  422 ERR-103, no row written; valid PNG intake → 202 `processing`, job
  promotes to `ready`; corrupt bytes → `failed` + ERR-104 reason text, teacher re-uploads.
  Final bytes served only with quiz access: student image GET on a hidden quiz
  → 404 (BR-007 existence-hiding); cross-quiz anon/student → 403; signed-URL
  forgery → 403.
- **AC-105** (BR-010, DEC-021, F-004; integration): 25 MB valid photo intake → 202
  immediately (answer flow unaffected); worker compresses to the 10 MiB = 10485760-byte
  limit → `ready` + note. Queue-full intake → 429 ERR-112 with `Retry-After: 1`.
- **AC-106** (BR-003, DEC-005; unit): timer matrix (none+seconds→reject; custom 0/−5/
  86401→reject; custom 1/45/3600/86400→accept; NULL required iff mode none) →
  ERR-101 or accept.
- **AC-107** (BR-040; integration): bad-format code, unknown code, or code of a
  HIDDEN quiz → 404
  ERR-400 with the identical generic message (no enumeration, hidden codes stop
  working); duplicate open
  code → 409 ERR-114 with suggestion.
- **AC-108** (BR-040/041, DEC-091; validation; integration): display name 101 chars/blank → 422
  ERR-401; emoji-only name ≤100 unicode chars → accepted.
- **AC-109** (uniqueness/delete guards, EDGE-029; integration): create user with existing
  email → 409 ERR-109, no row; delete quiz with ≥1 `submitted` attempt → 409
  ERR-111, quiz unchanged; delete quiz with live `registered`/`answering` rows
  (even zero submitted) → 409 ERR-111, quiz unchanged (DEC-082); delete quiz with
  only `cancelled`/`revoked` rows or zero rows → succeeds (cascade, DEC-092).
- **AC-110** (idempotency, ERR-203; integration): the start/submit form renders a
  server-generated `Idempotency-Key` (DEC-084, hidden field + header); double-click
  Start with the same key → single state transition + echo; double Submit with the
  same key → single `submitted` row + identical receipt (ERR-203 mapped to 200 echo,
  not an error); browser-close mid-submit + relogin → same key resolves to the
  committed receipt or resumes `answering`.
- **AC-111** (BR-008; integration): register twice / register while
  registered-or-answering row exists → 409 ERR-201 echo existing row.
- **AC-113** (BR-006 freeze, EDGE-026, DEC-103; integration): Given an `open+hidden`
  quiz with only `registered` rows, When teacher edits question/option, Then 200 and edits
  persist; Given an `open+show` quiz with a `registered` row, When teacher edits, Then 422
  ERR-107 and content unchanged; Given an `answering`/`submitted` row exists anywhere, When
  teacher edits, Then 422 ERR-107; in-flight attempt grades from its frozen snapshot unchanged.
  **Frozen + would-empty precedence (F-70, closed 2026-10-14):** given a frozen quiz
  (`answering`/`submitted` row anywhere, or `open+show` with any row) whose LAST question is
  deleted, Then 422 **ERR-107** (freeze wins over the maintain-gate) and content unchanged —
  never ERR-108; the teacher closes the quiz first (freeze lifts), then the ERR-108 leg
  applies. Ordering defined in `03-BUSINESS-RULES.md` §Precedence.
- **AC-114** (BR-021 audit; integration): re-grade same essay 5× rapidly →
  last value wins, 5 `audit_log` rows with old→new chain, `final_score` = last.
- **AC-115** (BR-002/009, N5; integration): register beyond `max_attempts`
  (e.g. 3rd concurrent when max=2) → 409/410 ERR-208 with attempts-exhausted message,
  no row; start on a stranded `registered` row that finds attempts exhausted
  (max lowered or double-tab race) → 410 ERR-208 AND the stranded row
  auto-cancels in the same transaction (audit-logged, DEC-083).
  **Timer-less tail (DECIDED N5, 2026-10-14 — accepted limitation):** the
  auto-cancel leg applies ONLY to an exhaustion race. On a `timer_mode=0`
  (none) quiz there is no deadline and no sweep, so a started `answering` row
  is NEVER auto-submitted and never auto-cancelled on its own — it stays
  `answering` until the teacher closes the quiz (the row then revokes) or
  revokes it manually, and its `taker_label` stays 409/410-blocked under
  DEC-106/DEC-091. Assert: `timer_mode=0` + one `answering` row + no teacher
  action → the row still holds `state=answering`, `attempt_deadline_utc` NULL,
  after any elapsed time; close the quiz → the row revokes.
- **AC-116** (BR-008, STATE-003; integration): student cancels a `registered`
  row → `cancelled` terminal, slot freed, re-register creates a NEW row with
  incremented `attempt_no`; teacher revokes a `registered` row → `revoked`;
  cancel/revoke post-start → 422 ERR-207, state unchanged; replay cancel/revoke on an
  already-`cancelled`/`revoked` row → `200` echo of the terminal state (EDGE-02A).
- **AC-117** (BR-050, FLOW-006; integration): fresh boot with no teacher →
  seed creates exactly one teacher from env (`must_change_password=1`);
  login forces `/change-password` gate (nothing else clickable until changed);
  second boot with a teacher present skips seed (no duplicate); seed email
  colliding with an existing student email → boot aborts with a clear log line,
  no silent role change (R8 fix).
- **AC-118** (API-000/001/002, ERR-001/002; integration): wrong credentials OR
  inactive account OR expired session → identical generic ERR-001 (no
  user-enumeration, no status leak); unauthenticated page GET → 302 to
  `/login?next=`; unauthenticated fetch → 401 ERR-001; POST
  without/invalid CSRF → 403 ERR-002 with reload guidance; logout deletes the
  server session + closes its SSE streams and is idempotent (second logout
  still 200/303, no error).

## B. Normal lifecycle, timers, scoring

- **AC-201** (FLOW-002, STATE-001, BR-007/008; e2e): register → start → answer 10 →
  review jump → submit → card shows thank-you + temporary score; DB: 1 attempt
  `submitted` with 10 embedded `answers_json` entries, cheat 0; second start blocked until max reached.
- **AC-202** (BR-011, FLOW-009, DEC-123; integration): Given per-Q 5 s timer, When student answers Q1 at 6 s,
  Then 422 ERR-205, index advanced, Q1 NULL→grades 0; Given back-GET Q1, When requested, Then 410
  read-only; Given unsaved input, When leaving, Then two-layer unsaved protection applies (DEC-123).
- **AC-203** (BR-012, FLOW-009; e2e): total 300 s, free nav + review edits, expiry →
  auto-submit `auto_submitted=true`, SSE `time_expired`, card renders; per-question
  answer POST at deadline+1 ms → 410 ERR-206 (zero grace, DEC-006); submit POST
  after the deadline (timer already auto-submitted) → `200` + the auto-submitted
  receipt (DEC-086, never ERR-206).
- **AC-204** (BR-013; integration): pause shifts `attempt_deadline_utc` by paused
  duration; answers during pause → 423 ERR-302.
- **AC-205** (BR-020/022/023; unit+integration): mixed attempt (single
  right/wrong, multiple exact/partial/empty, essays 7 & 9 pending) →
  temporary = objective sum; after grades → final = obj + 0.7 + 0.9 (2-dec);
  unanswered = 0, no negatives.
- **AC-206** (BR-030/031; e2e): flag matrix 2×2 (show_score × show_correct)
  → card/history reveal exactly per flags; toggling applies retroactively.
- **AC-207** (BR-009; integration): max=3, three submits → 3 immutable rows,
  newest-first recap, cheat counts independent.

## C. Matching synchronous

- **AC-208** (FLOW-003, STATE-002, F-002/007, DEC-023/110/119/120; e2e + 2 browsers): Given a matching
  quiz, When teacher opens the monitor, Then two surfaces render (DEC-110: normal monitor vs
  matching roster+controls); Given occupants join, Then roster counts update; Given start, Then
  both auto-navigate ≤2 s while streams live (SSE); Given a dead
  stream, Then the F-002 notice shows and navigation happens only after user refresh
  (DEC-102 — no poll); Given pause, Then blocker modal + frozen timers; Given resume, Then
  continue; Given stop, Then auto-submit all present from last saved; Given a matching quiz with
  `per_q`, Then a single guru-full index paces all occupants with no auto-advance (DEC-119);
  Given cheat events, Then counts read via `occupant_progress` (DEC-120); never-present no-show
  rows stay `registered`
  (DEC-088, excluded from recap zeros); Given finish, Then terminal; Given late join, Then 410 ERR-301.
  **Payload-field ownership (F-69, closed 2026-10-14):** every field this case asserts is defined
  by its owner document — `occupant_progress` = `{attempt,current_index,last_answer,cheat_count}`
  per DEC-120/API-044 (PRD §6 glossary); `guru-full` index per DEC-119; roster counts per
  API-042. This case asserts NO standalone payload field (in particular no `matching-membership`
  field exists in any contract — the F-69 observation was against an earlier revision and the
  assertion was removed rather than added; no new field is introduced here).
- **AC-112** (ERR-300/116; integration): concurrent pause+stop (distinct verbs) → one 200, one 409
  with current state; no forked session; SAME verb re-clicked within 10 s with a
  STALE version → 429 ERR-116 + current state, changing nothing (DEC-085, refined
  R8); same verb re-POSTed with the CURRENT version (re-read first) → 200 echo,
  never 429; retry after a failed (503/409) verb → echo-vs-409, never 429.
  **Restart leg (S3 closed, DECIDED 2026-10-14):** after a process restart, the
  ERR-116 in-memory 10 s guard window is RESET — a same-verb repeat that was
  rejected pre-restart is now judged on its merits (echo-vs-409 applies, 429
  is NOT guaranteed post-restart); the 409 ERR-300 state-conflict guards persist
  and always apply (S3 closed; accept in-memory loss).
- **AC-155** (EDGE-055, DEC-011; integration): start with 0 occupants → 422
  ERR-113 BLOCKED with message, session stays `waiting`, no attempt rows transitioned.
- **AC-155B** (EDGE-055B, DEC-093/094; integration): finish on an empty `waiting`
  room (zero ever-present) → `waiting → finished` abort (SSE `room_closed`,
  audit-logged); close/hide while `waiting` → same auto-finish; occupants'
  `registered` rows stay no-show and auto-revoke on close.
- **AC-155C** (EDGE-055C, DEC-093; integration): close/hide while the session is
  `running`/`paused` → 409 "hentikan sesi dulu", session and rows unchanged;
  stop → finish → close then succeeds.

## D. Anti-cheat

- **AC-140** (BR-032, FLOW-008, DEC-108; e2e/manual): Given Alt-Tab/new-tab/minimize/lock, When
  each event fires, Then count +1, modal shows N, monitor/recap reflect N; Given a `pagehide`
  beacon, When posted without CSRF, Then 200 (CSRF-exempt beacon, DEC-108).
- **AC-142/143** (BR-032, DEC-108; integration): blur+hidden same gesture → +1 only;
  triple-retried beacon (same client_event_id) → +1 only; fresh-`eid` spam
  (10 POSTs with distinct eids within 1 s) → +1 only (2 s server-time window,
  R8 fix — eid dedupe alone is insufficient); cheat POST on a
  `submitted`/`cancelled`/`revoked` row → idempotent echo, never an increment;
  `pagehide` beacon posts without CSRF → 200 (CSRF-exempt beacon, DEC-108).
- **AC-141** (EDGE-041, DEC-108; manual): Given an OS toast with no reported blur/hidden event,
  When evaluated, Then count unchanged (accepted: only reported events count); Given a `pagehide`
  beacon, When posted without CSRF, Then 200 (CSRF-exempt beacon, DEC-108).
- **AC-144** (EDGE-044, DEC-108; manual): Given DevTools open without blur, When evaluated,
  Then count unchanged (documented; no proctoring claim beyond focus-loss); Given a `pagehide`
  beacon, When posted without CSRF, Then 200 (CSRF-exempt beacon, DEC-108).
- **AC-145** (EDGE-045, DEC-108/123/128; manual): Given JS disabled, When student answers, Then
  form flow works, honest banner shown, submit accepted (no-JS three items, DEC-128);
  Given unsaved input, When leaving, Then two-layer unsaved protection applies (DEC-123);
  Given a `pagehide` beacon, When posted without CSRF, Then 200 (CSRF-exempt beacon, DEC-108).

## E. RBAC and visibility (mandatory IDOR sweep)

- **AC-120** (BR-007, STATE-004, DEC-115; integration): Given a hidden quiz, When student GETs it
  directly, Then 404; Given a closed+visible quiz, When student GETs it directly, Then 200
  closed view ("sudah-ditutup", no join/register/start buttons, AC-226 extension); Given either,
  When owner-teacher GETs it, Then 200.
- **AC-121** (IDOR, ERR-003; integration): student B GET/POST
  `/attempts/:A_id/...` → 403/404; anonymous token of quiz X on quiz Y → 403.
- **AC-122** (STATE-001, BR-020, R2 key-material absence; integration): Given a started attempt, When any client-facing payload is returned (question page, monitor row, recap row, SSE event, JSON envelope), Then `answer_key_json` content and any correct-set material are absent; assert by serializing every fixture response and scanning for key bytes.
- **AC-123** (EDGE-023/028, BR-051; integration): deactivate requested while the
  account holds `registered`/`answering` rows on an open quiz → 409 ERR-115,
  no session revoked, SSE untouched; deactivate with no live rows on open quizzes
  → SSE closed, sessions revoked, attempts persist, relogin → generic ERR-001.
- **AC-124** (EDGE-024, BR-052; integration): delete account (from inactive,
  button confirm) → user row gone, attempts anonymised
  (`student_id NULL`, `taker_label='deleted-user-<id>'`), recap balances unchanged.
- **AC-125** (EDGE-025; integration): back-button POST to a `submitted` attempt
  → 410 ERR-204 + receipt link, no state change.
- **AC-126A** (EDGE-021 hide, BR-007; integration): hide quiz mid-answering
  → in-flight rows continue and still submit under frozen deadlines; new
  joins get 404 (BR-007 existence-hiding); monitor keeps the rows; recap includes them.
- **AC-126B** (EDGE-022 close, BR-007, ERR-202; integration): close quiz mid-answering
  → in-flight rows continue and still submit under frozen deadlines; new
  register/start → 410 ERR-202; monitor keeps the rows; recap includes them.
- **AC-212** (BR-051/053/054, STATE-006, DEC-109/129; integration): Given `role=teacher` in register body, When submitted, Then ignored;
  self role/status edit rejected; Given teacher A, When mutating another teacher row, Then 403
  (DEC-109 teacher-to-teacher mutate); password rule matrix (7 chars/whitespace/no
  digit/no uppercase → ERR-110 with rule text); deactivate → login blocked + SSE closed +
  attempts intact; delete from inactive via two-step button confirm (no typed
  text, DEC-042) → anonymised recap row; both roles may self-delete via inactive plus
  confirm (DEC-129); password change revokes other sessions
  + closes their SSE realtime (BR-054).
- **AC-213** (ownership DEC-031 strict, ERR-003, DEC-109/129; integration): Given teacher B, When
  B reads or edits teacher A's quiz (GET/POST/grading/recap/monitor/control), Then 403 in every
  case (DEC-109 teacher-to-teacher mutate); Given any teacher, When self-deleting via inactive plus
  confirm, Then allowed for both roles (DEC-129 both-can-self-delete).

## F. Anonymous token mode

- **AC-150** (FLOW-004, BR-040/041; e2e): code → name → identical answer flow;
  attempt persisted with `taker_label` (`student_id NULL`), no user row; intent evicted post-close;
  result card printable; same-name second join → same quota (DEC-091: shared
  `max_attempts`, 410 ERR-208 when exhausted); different name → new participant;
  anon double-start with the same server key → single row + echo (DEC-084 pattern).
- **AC-151** (EDGE-051, DEC-096; integration): restart drops intent AND entered-anon
  presence (memory-only), submitted attempt
  intact; rejoin requires code+name again; non-returners never ever-present at start.
- **AC-152** (EDGE-052; integration): enter waiting room after lock
  (`running`+) → 410 ERR-301, no row change.
- **AC-153** (EDGE-053, STATE-007; e2e/manual): occupant disconnects mid-running, refreshes
  within 60 s (presence-stale threshold, STATE-007) → page rebuilds at server `current_index` with recomputed
  remaining time; until refresh the F-002 notice shows and missed pause stays invisible but enforced (ERR-302 on stale POST, DEC-102);
  Given stream-dead or matching pause, When the countdown displays, Then it freezes at the last
  server sync (DEC-125 countdown freeze).
  **Stale-occupant leg (N3, DECIDED 2026-10-14):** an occupant whose `last_seen` is older than 60 s at
  teacher stop is STILL ever-present — it auto-submits from last saved answers like any present occupant
  (the AC-154 leg); only rows with `last_seen IS NULL` (never entered) are no-shows and stay `registered`,
  never auto-submitted. Assert: >60 s-stale occupant at stop → `answering` row auto-submitted, not a no-show.
- **AC-154** (EDGE-054; integration): present occupant never returns + teacher stops →
  auto-submit from last saved answers (possibly empty → 0), `auto_submitted`
  flagged per row; never-present no-show row stays `registered`, never submitted (DEC-088).
- **AC-156** (EDGE-056/054B, DEC-091/096; e2e): mixed matching roster (account + anon via token)
  → both labelled correctly (`taker_label` vs `(anon) ` prefix + name), identical control
  semantics and auto-submit on stop; after a restart, entered anons re-identify
  (code+name) and non-returners are never ever-present at start (DEC-096/088).

## G. Resilience and timing

- **AC-130** (EDGE-030, F-001, ERR-501, DEC-122/124; e2e/manual): Given offline mid-answering, When
  answers never reached the endpoint, Then the banner is generic with a saved-tick rule (DEC-124:
  only post-200 saved-tick answers are committed; no Q1-Q3 exact unsent list) and dropped answers
  carry notice with no auto-flush and no retry of never-received answers; Given committed answers,
  When offline starts, Then they survive server-side; Given DB-down mutations, When attempted, Then
  503 ERR-501 with "answers safe" messaging and explicit user retry; Given a list XHR failure, When
  it occurs, Then the last rendered fragment wins with an inline error, never a blank list (DEC-122
  fragment-wins).
- **AC-131** (EDGE-031, F-006, ERR-500; integration): submit timeout with unknown outcome
  + browser closed → relogin resolves via the same `Idempotency-Key` to the
  committed receipt (or resumes `answering` if never committed); never a
  duplicate row; key lifetime runs terminal+24 h (never strands a live attempt);
  unexpected failures surface 500 ERR-500 with `request_id`
  (no stack/secret leak).
- **AC-132** (EDGE-032, BR-012; e2e): total timer expires during review →
  auto-submit `auto_submitted=true` + SSE `time_expired` → result card.
- **AC-133** (EDGE-033, BR-011; e2e/manual): per-question timer expires while
  typing → lock + advance; typed-but-unsent text lost with an explicit notice
  (accepted limitation, notice text asserted).
- **AC-134** (EDGE-034/034B; integration): answer POST racing a pause event (pause
  committed first server-side) → 423 ERR-302; attempt state unchanged; answer
  committed first → both effects persist (answer saved AND pause succeeds);
  stop-first → answer 410 ERR-204; resume-vs-stop → version decides, loser 409.
- **AC-134B** (BR-054 ordering; integration): answer/cheat POST racing a
  password-change revocation commit — session validity is checked before the
  txn begins (revoked → 401, no write); in-txn revocation cannot interleave
  (single request scope).
- **AC-137** (EDGE-037; e2e/manual): client clock 10 min fast → countdown
  re-syncs from `server_now_utc`; deadline judged server-side only.
- **AC-135** (F-002, STATE-007, ERR-502; manual): SSE-blocked proxy → persistent
  F-002 notice with the exact refresh wording within 45 s of silence (no poll,
  no auto-reconnect — assert zero `fetch`/`XMLHttpRequest` to any `?poll` URL
  and zero `EventSource` reconnection after `close()`); user refresh rebuilds
  rows from server truth; pause still enforced server-side; missed pause
  invisible until refresh (honest degradation, DEC-102; client event, no state fork);
  Given stream-dead, When the countdown displays, Then it freezes at the last server sync
  with the persistent banner (DEC-125 countdown freeze).
- **AC-136** (F-003 timer sweep, FLOW-009; integration): restart with live timers → sweep respawns,
  past-due auto-fires; fairness against persisted deadlines.
- **AC-160** (EDGE-060, BR-054; e2e): 60-min idle expiry mid-answer → 401, draft kept,
  relogin resumes at server index.
- **AC-161** (EDGE-061, BR-054, DEC-032; integration): password change on device A →
  device B session revoked immediately + SSE closed realtime; changing session
  survives with rotated csrf_token.
- **AC-162** (EDGE-062; integration): login with `inactive` account → generic
  ERR-001 identical to bad credentials (no status leak); no session created.

## H. Dashboards and monitor

- **AC-209** (FLOW-007; e2e): Given a normal quiz, When teacher opens the normal monitor, Then
  rows appear at start with q-index/last-answer/review/cheat and vanish on submit; Given a
  matching quiz, When teacher opens the matching monitor, Then the roster shows pre-start +
  controls and a single `per_q` index shared by all occupants (DEC-119, no auto-advance,
  teacher-paced); Given cheat events, When teacher reads any monitor row, Then the count comes
  from `occupant_progress` (DEC-120); counts match DB.
- **AC-210** (FLOW-005, STATE-005, BR-021/022, DEC-104, C-004; e2e): Given a submitted attempt
  with 2 essays, When teacher opens the queue, Then it lists oldest-first; Given grade 7 then 9,
  When each grade commits, Then `final_score` recomputes per grade (BR-022, 2-dec); Given re-grade
  9→5, When committed, Then value overwrites + audit row; Given submit closes the attempt stream
  (DEC-104), When the student refreshes or reopens the card, Then graded scores are visible
  (next-render, no live swap); Given grade 11 or -1, When committed, Then 422 ERR-105 and queue
  unchanged.
- **AC-211** (BR-014; integration): recap ordering newest-first, `taker_label`
  (+`(anon)` prefix for `student_id NULL`) and `taker_class`,
  subtotals reconcile to answers+grades; pagination stable.
- **AC-214** (REQ-008, FLOW-007, dashboards, API-070 DEC-004; integration): window filters 3/7/30d/all
  change aggregates exactly per the decided formulas — student
  attempts/total/average/answered/unanswered/questions_done; teacher
  students/quizzes/questions/attempts/scores_given/pending_essays/cheat_total/
  anon_attempts (separate line); Daily Pulse shows today's counts + live
  session + at most one reminder (essays beat cheat). Fixtures assert every
  number; `average` NULL renders "—"; `answering` rows never aggregate.
- **AC-219** (FLOW-007, BR-014/052, XLSX export, API-063 DEC-002; integration): Given a quiz with
  submitted attempts (incl. a repeat and an anon row), When teacher GETs
  `recap.xlsx`, Then workbook streams with header + freeze pane, identical
  ordering/content to API-062 JSON, filename `recap-<id>-<ts>.xlsx`,
  content-type XLSX; cross-owner GET → 403; `?window=7d` filters identically.

## I. Accessibility and responsive (manual + automated-lite)

- **AC-215** (REQ-009, UI-006/007/009; manual): full quiz completable keyboard-only at 360 px; focus order sane;
  countdown `aria-live` throttled; pause blocker announced; contrast 4.5:1/3:1
  verified against the decided UI-007 palette (Gotham + the ten verified text
  pairs pinned in §5 of `04-FRONTEND-UIUX/01-DESIGN-SYSTEM.md`, which is the
  SOLE token authority per UI-009 — `docs/DESIGN.md` is NON-authoritative and
  MUST NOT be cited as a token source); `metric-lg` = 36/39.6 exactly (not a
  range); `#71828C`/`#6B7B85` are large-text/non-text only; focus ring =
  Bootstrap `helpers/focus-ring` 3 px (no override); motion values per §3A only;
  any failing pair blocks FINAL.
- **AC-216** (REQ-009, UI-003/005; manual): `prefers-reduced-motion` disables smoothing; touch targets
  `btn-lg` / full-width labels on phone (Bootstrap-bound, no bare 44 px custom rule).

## J. Frontend conformance — Bootstrap full + inline CSS only (automated grep, CI)

- **AC-217** (REQ-009, UI-002/003/004/006/007, DEC-114; automated): Given the template tree, When CI
  runs `grep -r '<style' templates/` Then the ONLY matches are single
  `@font-face`-only blocks for Gotham Rounded (zero selectors, zero visual
  rules, UI-003 exception); When CI runs `grep -r '\.css' templates/` Then
  matches are ONLY the two vendored Bootstrap files (no other
  stylesheet link); When CI runs `grep -rho '<script[^>]*src="[^"]*"' templates/ | sort -u`
  Then every src is `bootstrap.bundle.min.js` or `/static/jquery/jquery-4.0.0.min.js`
  (vendored scripts only: no CDN script, no other local JS file — DEC-114); When CI scans
  inline `<script>` blocks, Then all JS lives inline-only (no other JS file referenced);
  jQuery 4.0.0 is a full carrier including show/hide/toggle methods (DEC-114 supersedes DEC-080:
  show/hide/toggle permitted); When CI scans every `style=""` attribute Then each
  property is in the §2 closed list (incl. UI-006 font stack + UI-007 exact
  hex) and every colour hex is in the UI-007 exact set. Any violation fails
  the build.
- **AC-218** (REQ-009, UI full mapping, UI-001..008; automated + review): Given
  `04-FRONTEND-UIUX/01-DESIGN-SYSTEM.md` §4, When CI checks Then every USED
  row has ≥1 template reference (class/component name grep) and every NOT
  USED row keeps its documented reason; a new Bootstrap class in templates
  that is not in §4 fails review (mapping must be updated first).

## K. List contract — search, pagination, query params (BR-055, DEC-098/099/100)

- **AC-220** (EDGE-063, BR-055, DEC-113; integration): Given 60 submitted attempts, When
  teacher opens recap with no keys, Then page 1 shows 25 rows + `?page=2,3`
  links; Given `?per_page=5000`, Then clamped to 200 with no error; Given
  `?page=abc` or `?page=9999`, Then page 1 / last page; Given unknown `?foo=1`,
  Then ignored, rows unchanged; Given `GET /student`, When opened, Then the full open+visible
  list renders with no `?q`/`?page` keys (DEC-113 student-full exclusion); Given attempt history,
  When opened, Then it still paginates. No combination returns 4xx.
- **AC-221** (EDGE-064, BR-055, DEC-098/111/122; e2e): Given the recap searchbar, When
  typing 10 keystrokes in 1 s, Then requests fire only per 300 ms pause
  (`?q=` + reset `page=1`, other keys preserved); Given each new keystroke, When fired, Then
  the in-flight request aborts (`jqXHR.abort()` — late responses never overwrite);
  Given AJAX responses, When returned, Then they carry table+pagination fragment only (via
  `X-Requested-With: XMLHttpRequest`); Given any visible rows, When rendered, Then the URL
  always matches via `history.replaceState` only (DEC-111 replaceState-only: URL
  shareable and refreshable; replaceState pushes NO history entry, so browser back
  EXITS the list rather than restoring a previous filter state — there is no
  history-restore claim); Given a list XHR failure, When
  it occurs, Then the last rendered fragment wins with an inline error (DEC-122 fragment-wins);
  Given no-JS submit, When posted, Then identical rows render full-page.
- **AC-222** (BR-055, DEC-100/116; integration): Given a recap filtered to
  `?window=7d&class=XII-1&q=bud&date=2026-10-01&page=2&per_page=25`, When exporting XLSX, Then the file
  contains exactly the filtered rows in the same order (export honors window/class/q/date/status and
  ignores page/per_page, DEC-116); Given the same URL shared to a second teacher
  session, Then identical rows render (URL fully reproduces state).

## L. Audit-driven additions (DEC-105..130, Modes C/D five-explorer audit)

- **AC-223** (BR-051, DEC-105/109, EDGE-066; integration): Given a teacher owning a `running` or `paused`
  matching session, When self-deactivating, Then 409 ERR-115 and
  the session and rows stay unchanged (no session revoked, no auto-close until stopped);
  Given the same owner, When a peer teacher attempts the deactivate, Then 403 ERR-003 first
  (teacher-to-teacher write, DEC-109 — the 409 guard is never reached).
- **AC-224** (BR-041, DEC-106, EDGE-067; integration): Given one `taker_label` holding an active
  (`registered` intent or `answering`) row on a quiz, When a second start with the same label fires
  while active, Then 409 active conflict echo; Given quota exhausted, When starting, Then 410 ERR-208.
- **AC-225** (BR-055, DEC-112; integration): Given a list GET with `X-Requested-With: XMLHttpRequest`,
  When requested, Then only the table/cards + pagination fragment renders (no layout); Given the same
  URL without the header, When requested, Then the full page renders with identical rows.
- **AC-226** (BR-007, DEC-115, EDGE-068; integration): Given a hidden quiz, When student GETs it directly,
  Then 404; Given a closed+visible quiz, When student GETs it directly, Then 200 closed view
  ("sudah ditutup", no join/register/start buttons); Given list discovery, When rendered, Then
  closed+visible quizzes are excluded from the list (extends AC-120); Given either leg, When
  owner-teacher GETs it, Then 200 (AC-120 owner-teacher leg).
- **AC-233** (API-039b, BR-055/DEC-100, Q6 2026-10-14; integration): Given `GET /attempts/history`,
  When opened by the owning student, Then the calling student's SUBMITTED (`state=2`) attempts render
  newest-first under the query contract (`?q/page/per_page/window/class/date/status`, default page 1
  of 25, unknown keys ignored, invalid values clamped, never 4xx); Given another student's request, Then
  no row of the calling student leaks (R1 `student_id=self` scoping — assert with two seeded students);
  Given a teacher session, Then 403 ERR-003 (`requireStudent`); Given a request presenting an
  `attempt_token` (API-051) instead of a session, Then it is rejected — history never accepts
  `attempt_token`; Given the `?status=` key, Then `submitted` (default) and `all` are treated as
  `submitted` and unknown values clamp to `submitted`.
- **AC-227** (BR-041, DEC-117, EDGE-069; integration): Given an anonymous intent started at hour 23 of
  its 24 h TTL on a total-timer quiz, When the timer runs past TTL expiry, Then the attempt continues
  to hour 30; Given the result card at hour 30, When opened, Then 200 via row membership (TTL is
  entry-only, never ejects in-flight occupants).
- **AC-228** (BR-003/011, DEC-119, EDGE-070; integration): Given a matching quiz with `per_question`
  mode, When running, Then no auto-advance fires and movement follows teacher cues only with a single
  guru-full monitor index; Given per-question deadlines, When recorded, Then they serve recap accuracy
  only.
- **AC-229** (FLOW-003, DEC-121, EDGE-071; integration): Given an ever-present occupant, When POSTing
  `leave`, Then `connected=0` with `last_seen` kept (still ever-present) and `occupant_left`
  broadcasts to the roster; Given rejoin, When entering again, Then allowed.
- **AC-230** (FLOW-002, DEC-130; integration): Given start on any timer mode, When deadlines compute,
  Then `attempt_deadline_utc`/`question_deadline_utc` from the server clock are the sole enforcement and recap
  truth; Given client countdown, When displayed, Then it re-syncs from `server_now_utc` and never
  overrides server judgement.
- **AC-231** (STATE-002, DEC-126, EDGE-072; integration): Given a `waiting` session, When pause/stop/resume
  fires, Then 200 echo of `waiting` with nothing changed (never 409 ERR-300, never 429 ERR-116).
- **AC-232** (STATE-002, DEC-127, EDGE-073; integration): Given a `finished` session, When any control verb
  fires, Then rejected with current state and the session stays `finished` (permanent reopen block; no
  verb reopens it).

## M. Audit decision list S1..S6 — all CLOSED as of the 2026-10-14 resolution set (status per `00-INDEX.md` §5b)

Q1 (2026-10-14): every audit item below whose decision is in fact final is recorded as CLOSED
with its decision text, not described as open. Nothing in S1..S6 remains genuinely open.

- **S1 Retry-After:** CLOSED (2026-10-14, DECIDED S1a). `Retry-After: 1` is emitted on
  429 ERR-112 only (time-based queue-full); 409s are conflicts and carry no header.
  AC-105 asserts it. Owner file: `02-ARCHITECTURE-AND-TECHNICAL/02-OPENAPI-SPEC.md` (API-023).
- **S2 page mapping:** CLOSED (2026-10-14, user decisions DEC-115/BR-007, DEC-100/112/113/122).
  S2a: invalid `?page` clamps to page-1/last-page, never 4xx (DEC-100, BR-055, EDGE-063, AC-220).
  S2b: the closed-view page layout is DECIDED (BR-007/DEC-115: 200 closed page, no
  join/register/start controls, closed+hidden → 404; EDGE-068, AC-226) and the
  list-fragment template mapping is DECIDED (`?q=` search only; non-q filters and
  pagination are SSR GETs — DEC-112/113/122). No S2 scope remains open.
  Owner files: `01-BUSINESS/03-BUSINESS-RULES.md` (BR-007),
  `02-ARCHITECTURE-AND-TECHNICAL/02-OPENAPI-SPEC.md`. Refs: EDGE-063, EDGE-068.
- **S3 ERR-116 restart:** CLOSED (2026-10-14, audit finding S3). The 10 s same-verb guard is
  in-memory per quiz and does NOT survive process restart — after a restart the window is reset
  (accept in-memory loss; 409 ERR-300 state-conflict guards persist and always apply). Promoted to a
  decision in STATE-002, asserted by the **AC-114** restart leg (the earlier AC-112
  reference is corrected per user decision N-C1). Owner file: `01-BUSINESS/04-STATE-MACHINES.md`
  (STATE-002) + AC-114. Refs: EDGE-065..073.
- **S4 heartbeat+vanish:** CLOSED (2026-10-14, user decisions S4a/S4b + Q7). S4a heartbeat
  payload `{server_now_utc}` is FINAL (API-036). S4b submit-vanish is DECIDED: the answering row
  disappears from the monitor on submit (API-044 + FLOW-007 + C-006 + AC-209). The stray-spinner
  clean-up has ONE threshold: 60 s (Q7) — there is no second 45 s number anywhere.
  Owner file: `02-ARCHITECTURE-AND-TECHNICAL/02-OPENAPI-SPEC.md` (SSE contract). Refs: C-006/C-010.
- **S5 cancelled card:** CLOSED (2026-10-14, DECIDED). API-039 decides it: a
  `cancelled`/`revoked` row renders a terminal-state card ("Percobaan dibatalkan/dicabut",
  no score, no correct answers, printable); no receipt is issued for never-present no-shows
  (`last_seen IS NULL`). Asserted by the AC-206 leg. Owner file:
  `02-ARCHITECTURE-AND-TECHNICAL/02-OPENAPI-SPEC.md`.
- **S6 CI DB/deploy/waiver:** CLOSED (2026-10-14, CICD §4 RESOLVED). (a) Integration DB
  = MariaDB 12.3.x service; sqlite is allowed locally with a divergence runbook.
  (b) deploy-fail = rollback blip, not continued serving. (c) no waiver — the
  DEC-001 gates must pass. Owner file: `05-DEVOPS-AND-INFRA/02-CICD-SPEC.md`.

## N. HTTP response caching and compression (ARCH-008, 2026-10-14)

- **AC-234** (ARCH-008 Class A; integration): Given a request for any `/static/…`
  asset, Then the response carries `Cache-Control: public, max-age=86400` and no
  `immutable` token (filenames are version-stable, not content-hashed); Given the
  response is gzip-encodable, Then `Content-Encoding: gzip` is present on
  `text/css`/`application/javascript`/`font/*` (ARCH §8.4).
- **AC-235** (ARCH-008 Class B; integration): Given `GET /uploads/:ref` with a
  valid quiz-access session, Then `Cache-Control: private, max-age=86400,
  must-revalidate` and a valid `ETag` + `Last-Modified` are present; Given a
  repeat request with a matching `If-None-Match`, Then `304` with no body; Given
  the same `:ref` requested by a session without quiz access, Then the access
  guard rejects it (the `private` scope must never widen to `public`).
- **AC-236** (ARCH-008 Class C; integration, exhaustive): Given a GET for each of
  the ARCH §8.2 bypass routes — `/login`, `/register`, `/join?code=…`, `/`,
  `/student`, `/student/history`, `/attempts/history`, `/student/profile`,
  `/attempts/:id/questions/:n`, `/attempts/:id/review`, `/attempts/:id/result`,
  `/matching/:id/waiting`, `/teacher`, `/teacher/quizzes`(+`/new`,`/:id/edit`),
  `/teacher/matching/:id`, `/teacher/monitor/:quizId`,
  `/teacher/quizzes/:id/grading`, `/teacher/quizzes/:id/recap`,
  `/teacher/users`, `/teacher/profile`, `/change-password` — Then every response
  carries `Cache-Control: no-store`; Given any query-string variant of those
  routes (`?q/`, `?page/`, `?per_page/`, `?window/`, `?date/`, `?status/`,
  `?class/`), Then `no-store` still holds.
- **AC-237** (ARCH-008 Class C streams/binaries/health; integration): Given each
  SSE endpoint (`/attempts/:id/events`, `/matching/:id/events`,
  `/teacher/matching/:id/events`, `/teacher/monitor/:quizId/events`), Then
  `Cache-Control: no-store`; Given `GET …/recap.xlsx`, Then `Cache-Control:
  no-store`; Given `/metrics`, `/healthz` and `/readyz`, Then `Cache-Control:
  no-store` on every response.
- **AC-238** (ARCH-008 Rule 4 prohibition; integration): Given any request, When no
  route in ARCH §8.1 Class A or B matches, Then no response carries `public`,
  `max-age` > 86400, or `immutable` — i.e. domain-wide Cache Everything must be
  impossible to configure by accident; Given a POST (incl. the cheat beacon,
  DEC-108), Then it is never served from any cache.
- **AC-239** (ARCH-008 header scrub on error; integration): Given a `4xx`/`5xx`
  response from any Class C route, Then it also carries `Cache-Control: no-store`
  (an error page must never be cached), and given a `304` on Class B, Then it is
  emitted only with the `ETag`/`Last-Modified` pair already established on the
  asset.

Exit criteria for IN_REVIEW: A–H + J + K + N automated in CI (integration vs MariaDB 12
service + template grep incl. AC-219 XLSX + coverage gates DEC-001, incl. automated AC-223..232
where level is integration/e2e), I manual
checklist signed (incl. AC-215 contrast on the decided palette), no open DEC — the audit
list S1..S6 in §M is CLOSED as of 2026-10-14 and no audit item remains genuinely open
(the genuinely open spec decisions, if any, are the ones named in `00-INDEX.md` §5b),
all else decided or deferred 2026-10-12), audit passes A–H recorded in `00-INDEX.md` §7.
