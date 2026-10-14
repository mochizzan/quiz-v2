# Edge-Case Plan — quiz-v2

Status: DRAFT | Owner: 03-RESILIENCE | Updated: 2026-10-14 (F-39 closed: EDGE-045 no-JS banner copy block pinned to the same three items as C-010 / DS §1+§5)

Candidate scenarios evaluated for MVP. Each: preconditions, trigger, expected
behaviour, persisted state, client-visible result, recovery/terminal, test
link. "Covered" means specified + tested; "Accepted" means known limitation
with documented behaviour (not silent). All prior BLOCKED items are RESOLVED
(DEC-003/011/032 decided 2026-10-12); no scenario below awaits a decision.

## Input boundaries

| ID | Scenario | Expected | Test |
|----|----------|----------|------|
| EDGE-001 | Empty quiz title / empty prompt / zero options; overlong title >200 / prompt >5000 / option text >500 / description >2000 / image_alt >500 / display name covered by EDGE-008 | 422 ERR-100/102, values preserved; overlong rejected, no silent trim | AC-101 |
| EDGE-002 | 11th option, duplicate option text (case-insensitive) | 422 ERR-102 | AC-102 |
| EDGE-003 | Essay 10.001 chars | 422 truncate-reject (no silent trim) | AC-103 |
| EDGE-004 | Image 2nd per question / EXE renamed .jpg (bad MIME at intake) | 422 ERR-103 (MIME sniff, not extension), no row written | AC-104 |
| EDGE-004B | Valid-MIME but unreadable/corrupt bytes (job failure) | Row `failed` + reason (ERR-104 text surfaced), teacher re-uploads; quiz flow unaffected | AC-104 |
| EDGE-005 | Image 25 MB valid | Intake 202 `processing`, job compresses to ≤10 MB → `ready` + note | AC-105 |
| EDGE-006 | Timer 0 / negative / >86400 s | 422 ERR-101 (bounds BR-003, custom teacher value 1–86400) | AC-106 |
| EDGE-007 | Code 5 chars / lowercase+symbols / duplicate open code | 422 ERR-400 (format) or 409 ERR-114 with new suggestion (dup) | AC-107 |
| EDGE-008 | Display name 101 chars / blank / emoji-only | 422 ERR-401; emoji allowed if ≤100 chars (unicode count) | AC-108 |

## Repetition and concurrency

| ID | Scenario | Expected | Test |
|----|----------|----------|------|
| EDGE-010 | Double-click Start / double Submit | Single row + receipt echo (idempotency key) | AC-110 |
| EDGE-011 | Register twice / register while active | 409 ERR-201 echo existing | AC-111 |
| EDGE-012 | Same owner fires pause+stop simultaneously (two tabs/sessions) | One wins (version), loser 409 ERR-300 + re-sync (cross-owner is 403 per DEC-031) | AC-112 |
| EDGE-013 | Teacher edits question while student answers | Blocked 422 ERR-107 (freeze BR-006, DEC-103 hidden/show rule); in-flight unaffected (snapshot) | AC-113 |
| EDGE-014 | Re-grade same essay 5× rapidly | Last write wins, 5 audit rows, score = last | AC-114 |
| EDGE-015 | Start 11th attempt when max=10 | 410 ERR-208 with attempts-exhausted message + stranded `registered` row auto-cancelled in the same transaction (audit-logged, DEC-083), no `answering` row | AC-115 |

## Staleness and deletion

| ID | Scenario | Expected | Test |
|----|----------|----------|------|
| EDGE-020 | Student opens hidden-quiz URL / closed-quiz URL | 404 (indistinguishable) per BR-007 | AC-120 |
| EDGE-021 | Teacher hides quiz mid-attempt | In-flight continues; new joins 404; monitor keeps row | AC-126A |
| EDGE-022 | Teacher closes quiz mid-attempt | Same as hide + register/start blocked ERR-202 | AC-126B |
| EDGE-023 | Account deactivated mid-attempt | Deactivate BLOCKED with 409 ERR-115 while the user holds `registered`/`answering` rows on an open quiz (BR-051 guard, DEC-081) — the mid-attempt paradox cannot occur; SSE/sessions untouched on the blocked path | AC-123 |
| EDGE-024 | Account deleted → recap row | Anonymised label, scores intact (BR-052) | AC-124 |
| EDGE-025 | Back-button POST to submitted attempt | 410 ERR-204 + receipt link | AC-125 |
| EDGE-026 | Delete question while live rows exist | Blocked 422 ERR-107 (freeze BR-006, DEC-103 hidden/show rule); snapshots intact | AC-113 |
| EDGE-027 | Delete LAST question of an open quiz | Blocked 422 ERR-108 (maintain-gate BR-005/DEC-089); close first, then delete; drafts may go to zero | AC-101 |
| EDGE-028 | Deactivate account holding live rows on an open quiz | Blocked 409 ERR-115 (BR-051 guard, DEC-081); no session revoked | AC-123 |
| EDGE-029 | Delete quiz with live `registered`/`answering` rows (even zero submitted) | Blocked 409 ERR-111 (BR-052 guard, DEC-082) | AC-109 |
| EDGE-02A | Double-cancel / double-revoke replay | `200` echo of the terminal state (no new row, no error) | AC-116 |

## Network and timing

| ID | Scenario | Expected | Test |
|----|----------|----------|------|
| EDGE-030 | Offline mid-answering (answers never reach endpoint) | Generic offline banner + saved-tick rule (generic banner, DEC-090 dropped with notice, DEC-124): only saved-tick (post-200 toast) guaranteed committed — no auto-flush, no manual retry of uncommitted answers; conflicts server-wins for what was committed | AC-130 |
| EDGE-031 | Submit timeout, unknown outcome, browser closed | Relogin resolves via idempotency key (F-006, terminal+24 h window) | AC-131 |
| EDGE-032 | Total timer expires during review | Auto-submit + SSE time_expired → result card | AC-132 |
| EDGE-033 | Per-Q timer expires while typing | Lock + advance; typed-but-unsent text lost with explicit notice (Accepted limitation) | AC-133 |
| EDGE-034 | Pause arrives 200 ms before answer POST; stop-vs-answer; resume-vs-stop | Pause-first → POST rejected ERR-302; answer-before-stop stands AND stop still auto-submits remainder; stop-first → answer 410 ERR-204; resume-vs-stop → version decides, loser 409 ERR-300 (FLOW-009 precedence) | AC-134 |
| EDGE-034B | Answer POST arrives 200 ms before pause commit | Answer stands AND pause still succeeds (both effects persist; server ordered) | AC-134 |
| EDGE-035 | SSE blocked proxy | Persistent notice + user refresh rebuilds from server truth (F-002, DEC-102 — no polling, no auto-reconnect); missed pause/stop invisible until refresh but still enforced server-side | AC-135 |
| EDGE-036 | Process restart with 40 live timers | Recovery sweep respawns; past-due fire immediately (F-003) | AC-136 |
| EDGE-037 | Client clock 10 min fast | Irrelevant — server deadlines only; countdown re-syncs | AC-137 |

## Cheating and trust

| ID | Scenario | Expected | Test |
|----|----------|----------|------|
| EDGE-040 | Alt-Tab, new tab, minimize, lock screen, incoming call overlay | Each counts +1 (modal + counter) per BR-032 | AC-140 |
| EDGE-041 | OS toast without blur (if browser reports no event) | Not counted (Accepted: only reported events count) | AC-141 |
| EDGE-042 | blur+hidden same gesture | Single increment (2 s debounce) | AC-142 |
| EDGE-043 | Beacon retried 3× (network flap); fresh-`eid` spam burst | Single incident (client_event_id dedupe + 2 s server-time window, BR-032) | AC-143 |
| EDGE-044 | DevTools open without blur | Not counted (documented; not a proctoring claim) | AC-144 |
| EDGE-045 | Student disables JS | Form submit still works; cheat detection + SSE absent with the honest no-JS banner. Pinned copy block (DEC-124/128 — the SAME three items in `04-FRONTEND-UIUX/02-COMPONENT-STATES.md` C-010 and `04-FRONTEND-UIUX/01-DESIGN-SYSTEM.md` §1/§5, verbatim): heading "pengawasan nonaktif — aktivitas tetap tercatat server" + exactly three listed items: (1) cheat detection, (2) SSE push, (3) toasts. No fourth item, no per-item explanation beyond those three (Accepted: server timestamps/answers persist; cheat count stays 0-unmonitored per API-062) | AC-145 |

## Anonymous and matching specifics

| ID | Scenario | Expected | Test |
|----|----------|----------|------|
| EDGE-050 | Two anons same name, same quiz | Second join shares the same `max_attempts` quota (DEC-091 exact-match); exhausted → 410 ERR-208; recap disambiguates by attempt id + joined_at | AC-150 |
| EDGE-051 | Anonymous intent after server restart (incl. entered-anon presence — memory-only, DEC-096) | Intent/presence lost → re-enter code+name; submitted attempts intact; non-returners never ever-present at start | AC-151 |
| EDGE-052 | Enter waiting room after lock | 410 ERR-301 | AC-152 |
| EDGE-053 | Occupant disconnects mid-running, refreshes within 60 s (presence-stale threshold, STATE-007) | Refresh rebuilds from server position (F-002 notice until then); missed pause still enforced | AC-153 |
| EDGE-054 | Occupant never returns; teacher stops | Present occupant: auto-submit from last saved (possibly empty → 0); never-present no-show row stays `registered`, never submitted (DEC-088) | AC-154 |
| EDGE-054B | Start with never-present occupants | Ever-present → `answering`; never-present (`last_seen IS NULL`) stays `registered` no-show, excluded from recap zeros, revocable post-start (DEC-088) | AC-156 |
| EDGE-055 | Start with 0 occupants | BLOCKED with message (DEC-011); no session transition, roster unchanged | AC-155 |
| EDGE-055B | Empty waiting room exit (zero ever-present, or close/hide while waiting) | `waiting → finished` abort via finish / auto-finish on close-hide (SSE `room_closed`, DEC-093/094); rows stay no-show + auto-revoke on close | AC-155B |
| EDGE-055C | Close/hide while session `running`/`paused` | 409 "hentikan sesi dulu" (stop first, DEC-093) — stop/finish stay the only exits | AC-155C |
| EDGE-056 | Matching quiz with token mode + account students mixed | Roster shows both labels; identical control semantics | AC-156 |

## Auth and sessions

| ID | Scenario | Expected | Test |
|----|----------|----------|------|
| EDGE-060 | Session expires mid-answer (60-min idle, BR-054) | Save → 401; draft preserved client-side; re-login resumes | AC-160 |
| EDGE-061 | Password changed on device A, device B active | B revoked immediately + SSE closed realtime (DEC-032, BR-054) | AC-161 |
| EDGE-062 | Login with inactive account | Generic ERR-001 (no status leak) | AC-162 |
| EDGE-063 | Malformed list query (`?page=abc`, `?page=9999`, `?per_page=5000`, unknown `?foo=1`) | Falls back to defaults (page 1 / last page / clamped 200 / ignored) — never 4xx (BR-055); page mapping is CLOSED — invalid `?page` clamps to page-1/last-page per DECIDED S2a (DEC-100, AC-220) and the closed-view layout is DEC-115/BR-007; no decision is required | AC-220 |
| EDGE-064 | Rapid typing in a searchbar (10 keystrokes in 1 s) | One request per 300 ms pause only (debounce pacing, DEC-098); in-flight request superseded, results always match the URL `?q=` via replaceState-only navigation (DEC-111/112, BR-055) | AC-221 |

## New rows (owned here, EDGE-065..073)

| ID | Scenario | Expected | Test |
|----|----------|----------|------|
| EDGE-065 | FREEZE-HIDDEN/SHOW: edit frozen field on hidden versus shown quiz | Open+hidden with only registered/cancelled rows → 200 edits persist; OPEN+SHOW+ANY ROW → 422 ERR-107; ANSWERING/SUBMITTED ANYWHERE → 422 ERR-107 (DEC-103, BR-006, ERR-107) | AC-113 |
| EDGE-066 | DEACTIVATE-LIVE-BLOCK: deactivate account owning a running/paused session (incl. self) | Self-deactivate → 409 ERR-115; peer-teacher attempt → 403 ERR-003 first (DEC-109); terminal state unchanged, no session revoked; stop session first (DEC-105, BR-051, ERR-115) | AC-223 |
| EDGE-067 | ANON-ACTIVE-CONFLICT: same-label second start while active; quota exhausted path | Active same-label → 409 conflict echo; quota exhausted → 410 ERR-208 (DEC-106, ERR-201, ERR-208) | AC-224 |
| EDGE-068 | CLOSED-PAGE-200: direct GET of closed-but-visible quiz page | 200 closed page, no join/register/start controls; excluded from discover list; closed+hidden → 404 (DEC-115, BR-007) | AC-226 |
| EDGE-069 | TTL-ENTRY: anonymous intent TTL expiry while attempt in flight | TTL ENTRY-ONLY: in-flight answering attempt and result card continue on row membership, never re-check TTL; re-identify only for new entry (DEC-117, ERR-400, ERR-401) | AC-227 |
| EDGE-070 | PER_Q-GURU-FULL: matching quiz with per-question timer pacing | Single GURU-FULL index paces all occupants; no auto-advance; deadlines recorded for recap accuracy (DEC-119/130, BR-011) | AC-228 |
| EDGE-071 | LEAVE-ENDPOINT: occupant leaves waiting room via leave endpoint before start | Row keeps ever-present mark, connected=0, last_seen retained; occupant_left presence BROADCAST to roster; no row deleted (DEC-121, F-007) | AC-229 |
| EDGE-072 | WAITING-VERBS-ECHO: pause/stop/resume on waiting session | 200 ECHO of waiting, no state change; NEVER 409/429 (DEC-126, ERR-300, ERR-116, F-007) | AC-231 |
| EDGE-073 | REOPEN-FINISHED-PERMANENT: control verb or reopen on finished session | FINISHED PERMANENT: no verb reopens; submit resolves via normal state checks, never a reopen (DEC-126/127, F-006, F-007) | AC-232 |

All EDGE rows map to Given-When-Then cases in
`../06-QA-AND-MANAGEMENT/01-TEST-CASES.md`. Every EDGE ID above has a defined
AC (AC-101..162 series incl. AC-126A/126B split legs, plus EDGE-02A→AC-116,
EDGE-054B→AC-156 and EDGE-063/064→AC-220/221; new EDGE-065→AC-113, EDGE-066→AC-223,
EDGE-067→AC-224, EDGE-068→AC-226, EDGE-069→AC-227, EDGE-070→AC-228, EDGE-071→AC-229,
EDGE-072→AC-231, EDGE-073→AC-232); implementation MUST NOT treat
any scenario as
optionally-blocked — all decisions backing them (DEC-003/011/032/081..089,
DEC-090..097, DEC-098/099/100/101/102, DEC-103..130) were decided
2026-10-12. R9/R10 history intact (R9 list-contract DEC-098/099/100/101, R10
no-poll-notice-refresh DEC-102 — superseded wording retained, not rewritten).

Audit decision list S1..S6 — status per §5b re-audit (2026-10-14), cited here for
QA coverage (owned by the files named in each row; no item below is still open).
Every item whose decision is in fact final is marked CLOSED (Q1, 2026-10-14): S1a,
S2a, S2b, S3, S4a, S4b, S5, S6a/b/c are CLOSED in their owner files, and N13's
late-flow page is resolved with `/attempts/:id/result` (the final result page after
submit). Any row below that still says DECISION REQUIRED is a stale marker, not an
open question.

- S1 Retry-After (429 ERR-112): CLOSED — API-023 DECIDED S1a: `Retry-After: 1` on
  429 ERR-112 only (time-based); 409s are conflicts, no header (AC-105 asserts it).
- S2 page mapping: CLOSED (Q1, 2026-10-14) — S2a (invalid `?page` clamp to page-1/last-page,
  never 4xx) DECIDED in API-000 per DEC-100/BR-055/EDGE-063/AC-220, and S2b (final closed-view
  page layout + list-fragment template table) is CLOSED
  (2026-10-14 user decision): closed-view layout BR-007/DEC-115 renders 200 with no
  join/register/start controls (EDGE-068, AC-226); the fragment-template mapping is
  DECIDED per DEC-112/113/122 in API-000 (`?q=` search only, others SSR GET).
- S3 ERR-116 restart: CLOSED — STATE-002 DECIDED: in-memory per quiz, does NOT survive
  restart; 409 ERR-300 still guards; accept in-memory loss (**AC-114** restart leg — the earlier AC-112 reference is corrected per user decision N-C1 2026-10-14).
- S4 heartbeat+vanish: CLOSED — S4a (heartbeat payload `{server_now_utc}`) DECIDED and
  FINAL in API-036 (user decision 2026-10-14); the stale number is single-valued per
  Q7: ONE threshold, 60 s, with no second 45 s figure. S4b (submit-vanish: the answering
  row disappears from the monitor on submit) DECIDED in API-044 + FLOW-007 + C-006 +
  AC-209. C-006/C-010 stray-spinner clean-up also applies that same 60 s number, so no
  residual open question remains (the "Residual … see N13 territory" note is stale and
  has been removed).
- S5 cancelled card: CLOSED — API-039 DECIDED: `cancelled`/`revoked` row renders a
  terminal-state card ("Percobaan dibatalkan/dicabut", no score, no correct answers,
  printable); no receipt for never-present no-shows (`last_seen IS NULL`); AC-206 leg.
- S6 CI trio: CLOSED — CICD §4 RESOLVED: (a) integration DB = MariaDB 12.3.x service,
  sqlite-allowed locally with divergence runbook; (b) deploy-fail = rollback blip, not
  continued serving (corrected from the factually-wrong `up -d` claim); (c) no waiver —
  DEC-001 gates must pass. ARCH §6 S6 line reworded to the canonical trio.
