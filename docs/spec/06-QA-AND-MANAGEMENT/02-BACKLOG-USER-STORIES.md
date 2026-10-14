# Backlog / User Stories — quiz-v2

Status: DRAFT | Owner: 06-QA | Updated: 2026-10-14 (F-71/F-72 closed: US-403 aligned to DEC-111 + one pagination default; US-703 timing claim dropped (no owner); US-103 re-scoped to the teacher actor)

Epics → stories with actor/need/outcome, Given-When-Then acceptance, boundaries,
and risk. No estimates, points, sprints, or priorities beyond P0/P1 already in
PRD (skill §8.20 forbids inventing them). Sequencing follows dependency order.

## EPIC-1 — Authoring (teacher creates fair quizzes)

- **US-101** As a teacher, I create a quiz with mixed types + image (+alt text
  and student-view preview) so that I stop retyping from books. AC: Given valid
  metadata+questions, When I open, Then students see it (AC-101); `Pratinjau`
  renders prompt+image+options without correctness. Boundary: freeze post-start
  (BR-006). Depends: auth. Risk: low.
- **US-102** As a teacher, I set attempts/shuffle/timer (custom typed seconds 1–86400)/visibility/token so
  that each quiz enforces its own fairness. AC: matrix BR-002/003/030/031/040
  accepted/rejected exactly. Risk: medium (timer semantics).
- **US-103** As a teacher, I manage student accounts (create/deactivate/
  delete) so that classes stay current. AC: BR-051/052 lifecycle (AC-212);
  deactivate blocked on live open-quiz rows (DEC-081, 409 ERR-115).
  Boundary: actor scope is the TEACHER managing students — the student-side
  flows are deliberately out of this story: student self-register + redirect to
  manual login (DEC-087, no auto-login) belongs to the auth story US-701, and
  the account-delete anonymisation of attempt rows (BR-052) is asserted by
  AC-124 rather than by this story's teacher-manage scope (F-72, 2026-10-14 —
  the earlier draft mixed both actor flows here).
  Risk: medium (deletion retention).

## EPIC-2 — Taking (student/anon completes quizzes)

- **US-201** As a student, I register → start → answer one-per-page → review →
  submit → see the thank-you card with no step requiring more than its single
  defined action (one click per question, one confirm per submit). AC:
  FLOW-002 e2e (AC-201) + visibility flags (AC-206). Risk: medium.
- **US-202** As a student, per-question locks are forward-only and total expiry
  auto-submits from server time (AC-202/203/204 assert lock, auto-submit, and
  pause-shifted deadlines) so that timing matches the server clock. AC: AC-202/203/204. Risk: high
  (server-time authority, pause shifting).
- **US-203** As an anonymous participant, I join via code + name so that I can
  take a quiz without an account. AC: AC-150/151. Risk: medium (ephemeral vs
  persisted split).

## EPIC-3 — Synchronous matching

- **US-301** As a teacher, I run waiting-room → start/pause/stop/finish with
  the roster reflecting joins within one heartbeat (SSE),
  and all present occupants auto-navigating to Q1 on `session_started`. A dead
  stream shows the F-002 notice; occupants refresh to rebuild (DEC-102 — no
  poll, no auto-reconnect). AC: AC-208 + race
  AC-112. Risk: high (fan-out, notice-refresh, auto-submit sweep).
- **US-302** As a student, a paused session rejects my answer POSTs with 423
  ERR-302 and resume shifts my deadlines by the paused duration (AC-204),
  so that pausing never costs nor grants me time. AC: AC-204/208. Risk: high.

## EPIC-4 — Fairness signals (anti-cheat + monitor)

- **US-401** As a student, focus-loss shows the blocking modal with the recorded
  count N (AC-140: each reported event +1 with debounce/dedupe per AC-142/143).
  AC: AC-140/142/143. Risk: medium (false-positive honesty).
- **US-402** As a teacher, I see per-occupant progress + cheat counts in the
  monitor and the same numbers in recap plus one-click XLSX export (AC-209/211/219
  assert identical values across monitor, recap, and file). AC:
  AC-209/211/219. Risk: medium.
- **US-403** As any list user, search inputs respond per 300 ms pause
  (`?q=` + reset `page=1`, URL shareable/refreshable via `history.replaceState` only —
  replaceState-only pushes NO history entry, so browser back EXITS the list
  instead of restoring the previous filter state; there is no history-restore
  claim (DEC-111); no-JS identical. Pagination defaults: every covered list uses
  `per_page=25` (DEC-099/BR-055 — one default for SSR pages and JSON/API lists;
  `?per_page=` may override up to 200). The sole exception is `GET /student`,
  which renders the full open+visible list with no `?q`/`?page` keys (DEC-113
  student-full exclusion; history still paginates). AC: Given typing, When paused,
  Then one request per pause with `?q=` + `page=1` (AC-221); Given `GET /student`, When opened,
  Then full list with no slice keys (AC-220); Given browser back on a filtered list,
  Then the user leaves the list (DEC-111 replaceState-only, no restore). Risk: low.

## EPIC-5 — Grading and results

- **US-501** As a teacher, I grade essays 0–10 (and re-grade auditably) so
  that finals are deterministic. AC: AC-205 + FLOW-005. Risk: low.
- **US-502** As a student, I see temporary→final scores exactly per the
  teacher's visibility setting. AC: AC-206/207. Risk: low.

## EPIC-6 — Dashboards

- **US-601** As a student/teacher, I see overview analytics with 3/7/30d/all
  filters plus the teacher Daily Pulse panel so that progress is visible. AC:
  AC-214 (formulas decided API-070 DEC-004: student attempts/total/average/
  answered/unanswered/questions_done; teacher students/quizzes/questions/
  attempts/scores_given/pending_essays/cheat_total/anon_attempts + Pulse
  one-reminder rule). Risk: low-medium.

## EPIC-7 — Platform (auth, deploy, ops)

- **US-701** As any user, email+password login (BR-053 rule text) with 60-min
  sliding sessions, forced seed rotation, and realtime revocation on password
  change keeps accounts safe. AC: AC-212 + API-001 + AC-160/161. Risk: medium.
- **US-702** As the school operator, Compose deploy + migrate-then-serve +
  smoke + rollback keeps releases boring. AC: pipeline §02 + smoke log.
  Risk: low.
- **US-703** As the operator, structured stdout logs (no files retained, R8) + host-queried audit trail (no UI) + health gates let
  me diagnose exam-day issues. AC: the §03 runbook queries (`audit_log` grades,
  `answers_json`/`submit_retry_total`, `db_latency_ms`, `sse_dead_total`) each return the documented
  fields for a given quiz/attempt id; stdout stays JSON with `X-Request-ID` correlation. Latency is
  NOT fixed here — no owner file states a runbook response-time target (F-72, 2026-10-14: the
  earlier "<1 min" claim had no owner), so the assertion is query-correctness only.
  Risk: low.
- **US-704** As a student on exam day, pages load fast because asset bytes are
  cached for a day in my browser and responses are compressed, while every page
  that shows my own data is never stored anywhere — so speed never comes at the
  cost of someone else seeing my work. AC: AC-234..239 (assets `public,
  max-age=86400` + gzip; images `private` + `ETag`/`Last-Modified` + `304`; all SSR
  HTML/SSE/export/health `no-store`; no `immutable`/1-year value anywhere; error
  pages `no-store`). Risk: medium (a wrong header here leaks private pages, so the
  class list is exhaustive in ARCH §8.2 and AC-236 is exhaustive per route).

## EPIC-8 — Audit-driven additions (DEC-105..130, Modes C/D five-explorer audit)

- **US-404** As a matching occupant, I leave and rejoin the waiting room so that a
  dropped presence never strands my slot. AC: Given ever-present, When I POST leave,
  Then `connected=0` with `last_seen` kept and `occupant_left` broadcasts (AC-229);
  Given left, When I re-enter, Then rejoin allowed. Risk: low.
- **US-405** As a student, I see a closed quiz view so that a closed quiz never looks
  broken. AC: Given closed+visible, When I GET it directly, Then 200 closed view
  ("sudah-ditutup", no join/register/start buttons, AC-226 extends AC-120); Given hidden,
  When I GET it, Then 404. Risk: low.
- **US-406** As a teacher, I export the filtered recap so that the file matches the
  filtered view. AC: Given `?window/class/q/date/status` filters, When I export XLSX,
  Then the file honors those filters and ignores `page/per_page` (AC-222, DEC-116).
  Risk: low.
- **US-407** As a student, I see honest offline state so that unsent answers are never
  silently lost. AC: Given offline mid-answering, When answers never reached the endpoint,
  Then the banner is generic with the saved-tick rule and dropped answers carry notice
  (AC-130, DEC-124). Risk: low.

Suggested build order: EPIC-7 (auth/skeleton) → EPIC-1 → EPIC-2 normal →
EPIC-5 → EPIC-4 → EPIC-3 → EPIC-6. Definition of done per story: linked AC
green in CI (or manual sheet signed), audit §7 passes for touched files,
no new DEC opened silently.
