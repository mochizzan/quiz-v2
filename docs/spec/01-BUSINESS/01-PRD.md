# PRD — quiz-v2 (Daily Quiz for Teachers)

Status: DRAFT | Owner: 01-BUSINESS | Updated: 2026-10-14 (F-29 closed: §7 freshness date + supersession policy; F-31 closed: A1 rewritten to the canonical DEC-125 dead-stream freeze)

## 1. Summary

quiz-v2 is a responsive-web, single-school daily-quiz system that lets teachers
create quizzes with mixed question types, run them with timers and anti-cheating
detection, monitor progress live, grade essays manually, and recap scores
without manual spreadsheets. It replaces two current practices:

- OBSERVED (user-reported): teachers write quizzes manually from books.
- OBSERVED (user-reported): teachers use Google Forms and recap scores manually.

Target stack (DECIDED by user): Go monolith, server-side rendering (SSR),
MariaDB 12, Docker deployment, realtime via SSE + POST.

## 2. Problem and Goals

| ID | Statement | Type |
|----|-----------|------|
| REQ-001 | A teacher can create a quiz containing single-choice, multiple-choice, and essay questions, optionally with one image per question, without retyping into external tools. | Functional, P0 |
| REQ-002 | A student can discover open quizzes, register, start, answer one-question-per-page, review, and submit entirely from a mobile or desktop browser. | Functional, P0 |
| REQ-003 | The system auto-grades objective questions and computes a deterministic total; the teacher grades essays 0–10, scaled to 0–1 point each. | Functional, P0 |
| REQ-004 | The system recaps every attempt (score, answers, cheat count, timing) per student so the teacher never recaps manually. | Functional, P0 |
| REQ-005 | The system detects focus-loss cheating (browser-reported hidden tab / blur / minimized that produces a blur-or-hidden event), warns the student immediately, and records a count per attempt visible to the teacher. OS overlays that produce no blur/hidden event are a documented limitation (BR-032). | Functional, P0 |
| REQ-006 | A matching-type quiz runs synchronously from a waiting room with teacher start / pause / stop / finish control pushed to all students in near realtime. | Functional, P0 |
| REQ-007 | A token/code mode allows non-registered (anonymous) participants to join via a code, with ephemeral in-memory registration and persisted attempts labelled by entered name. | Functional, P0 |
| REQ-008 | Teacher dashboard shows analytics (students, scores given, questions, quizzes); student dashboard shows personal analytics with 3/7/30-day/all filters. | Functional, P1 |
| REQ-009 | All pages are usable on a phone browser (responsive), with server as time authority and graceful handling of disconnects. | Non-functional, P0 |
| REQ-010 | Single Docker image + MariaDB 12 via Compose; single-school scope; no multi-tenant isolation in MVP. | Constraint, P0 |

Success gates (DECIDED DEC-001, 2026-10-12): automated test coverage minimum
85% overall with 100% on critical paths — grading (BR-020/022), timer
enforcement (BR-011/012/013, FLOW-009), submit idempotency (F-006, API-038),
auth/session (BR-050/053/054, API-001), authorization (RBAC R1–R5), matching
control verbs (API-043, STATE-002). CI enforces both gates; see
`06-QA-AND-MANAGEMENT/01-TEST-CASES.md` exit criteria.
S6 CI trio (CLOSED 2026-10-14 per §5b, owner: CICD §4 + ARCH §6): (a) integration
DB = MariaDB 12.3.x service, sqlite-allowed locally with divergence runbook; (b)
deploy-fail = rollback blip, not continued serving (corrected from the factually-wrong
`up -d` claim); (c) no waiver — DEC-001 gates must pass. ARCH §6 S6 line reworded to
the canonical trio. See `05-DEVOPS-AND-INFRA/02-CICD-SPEC.md` §4.

## 3. Actors

| Actor | Goals | Capabilities (summary; canonical in `../02-ARCHITECTURE-AND-TECHNICAL/04-RBAC-MATRIX.md`) |
|-------|-------|------------------------------------------------------------------------------------------|
| Teacher (guru) | Create quizzes fast; run fair quizzes; see results instantly. | Full dashboard: manage self + all users (create teacher AND student accounts — PRD §3: the seeded teacher creates further teachers/students; teacher-to-teacher mutate other than create, i.e. edit/deactivate/delete another teacher → 403 ERR-003, DEC-109), manage quizzes and questions, set quiz settings, run waiting room, monitor live, grade essays, view all recaps. |
| Student (murid) | Join quickly; answer without confusion; see result per teacher setting. | View open+visible quizzes, register/cancel before start, answer, review, submit, view own history and profile, view score card per visibility setting. |
| Anonymous participant | Join via code without an account. | Enter name/class once, same answer flow as a student for that quiz only; no login, no history page, no profile. Data persisted as an attempt with a name label, not an account. |
| Seeded teacher | Bootstrap the system. | First teacher account created by database seed; can then create further teachers/students. |

## 4. Scope

### 4.1 In scope (MVP)

- CRUD users (teacher-managed — teachers and students; student self-register creates `role=student` only, API-003).
- CRUD quizzes and questions owned by a quiz (no cross-quiz bank).
- Question types: single-choice (exactly one correct), multiple-choice (one or more correct, all-or-nothing), essay (manual 0–10).
- One image per question (JPG/PNG/WEBP/JPEG/GIF; >10 MiB auto-compressed server-side; uploader: teacher only).
- Per-quiz settings: attempts (normal only), shuffle questions, shuffle answers, timer mode (none / per-question / total, seconds), result visibility (show score yes/no; show correct answers yes/no), token/code + anonymous toggle.
- Normal flow: dashboard → pick quiz → register → start → one-question-per-page → review → submit → thank-you card.
- Matching flow: register → waiting room → teacher start/pause/stop/finish with SSE push; waiting room locks once started.
- Anti-cheat focus-loss counter + immediate warning modal.
- Live monitor (different views for normal vs matching).
- Essay grading queue; temporary vs final score semantics per visibility setting.
- Student/teacher overview dashboards with stated analytics and filters.
- Auth: email + password (at least 8 characters, no whitespace, at least one digit and one uppercase letter), 60-minute sliding sessions; student self-register creates the account then redirects to manual login (no auto-login, DEC-087); deactivate is blocked while the user is registered/answering on an open quiz (DEC-081); delete semantics per BR-051/052.
- Teacher one-click recap export to XLSX per quiz (API-063, AC-219).
- Teacher dashboard Daily Pulse panel (today's activity plus live session state plus at most one attention reminder; organizes existing data only, no new scheduling or recommendation).

### 4.2 Explicitly out of scope

- Multi-school tenancy, per-class teacher scoping (single flat school).
- Cross-quiz reusable question bank, question versioning, import/export of questions.
- Rich text editor, LaTeX, audio/video questions, student image upload.
- SSO/OAuth, OTP, email verification, password policy beyond bcrypt minimum.
- CSV/PDF formats (XLSX is the single MVP export format, DEC-002), printing beyond the result receipt, charts beyond stated analytics.
- Quiz duplication (copy-as-new) is explicitly excluded from MVP (DEC-010).
- Native mobile apps, offline mode, push notifications.
- Automated essay scoring, plagiarism detection, proctoring beyond focus-loss.

## 5. Constraints and Assumptions

- C1: Go Fiber + SSR + MariaDB 12 + Docker Compose, exposed to the internet through Cloudflare (NGINX removed per ARCH-009). Every library choice must be explicit and justified in `../02-ARCHITECTURE-AND-TECHNICAL/01-ARCHITECTURE-OVERVIEW.md` (Fiber framework, MariaDB driver, XLSX library — DEC-020 companions).
- C2: Server is the authority for time, scoring, shuffle order, and state transitions. Browser timers and order are display-only.
- C3: Single deployable instance for MVP; horizontal scaling is out of scope (documented as NOT APPLICABLE in infra spec with reason).
- C4: International English is used in all spec artifacts; user-supplied Indonesian domain terms (guru, murid, KKM if later added) are preserved and defined in the glossary.
- A1 (INFERRED): school network supports SSE (plain HTTP, no buffering proxy that breaks streams). A buffering proxy yields the F-002 persistent dead-stream notice + user refresh — polling is NOT a fallback (DECIDED DEC-102, R10). SSE stays friendly under DEC-104 (graded events only for open streams, closed streams get nothing live) and DEC-125 (countdown freeze on BOTH triggers — dead-stream and matching pause — never invents ticks while disconnected; the matching-pause freeze is BR-013 server-side deadline freezing and is covered by the same DEC-125 rule).
- A2 (INFERRED): 1 teacher runs at most a few parallel quizzes; peak concurrency in the tens, not thousands. Load numbers are capped explicitly in architecture.

## 6. Glossary

| Term | Meaning |
|------|---------|
| Quiz | A teacher-owned assessment with settings, questions, and attempts. |
| Normal quiz | Asynchronous quiz; each student starts on their own after registering. |
| Matching quiz | Synchronous quiz with a waiting room and teacher-driven start/pause/stop/finish broadcast to all occupants. |
| Attempt | One full working of a quiz by one student (or one anonymous name); repeated attempts duplicate rows, never overwrite. |
| Registration | Intent to take a quiz on the attempts row (`state=registered`); cancellable before start; the same row advances to `answering` at start (R2, no separate table). |
| Waiting room | Pre-start holding state for matching quizzes; locked once the session starts. |
| Shuffle | Per-attempt permutation of question order and/or option order, fixed at start and stored. |
| Cheat count | Number of focus-loss events recorded for one attempt. |
| Temporary score | Score shown on the post-submit card when the teacher allows it; excludes ungraded essays. |
| Final score | Score after all essays for the attempt are graded. |
| Show / Hide | Visibility flag controlling whether a quiz appears on the student dashboard regardless of open/closed. |
| Open / Closed | Availability flag controlling whether new registrations/starts are accepted. |
| Deactivate | Block login but keep all data. Delete removes the account per retention rules. |
| Closed view | 200 response for closed+visible quizzes ("sudah ditutup", no join); hidden quizzes return 404 (DEC-115). |
| Occupant progress | Teacher fan-out payload `{attempt,current_index,last_answer,cheat_count}` for cheat/monitor updates (DEC-120). |
| Entry gate | 24 h anonymous TTL checked only at entry; answering attempts and result cards use row membership (DEC-117). |
| One-active | At most one active (intent or answering) row per quiz per student or per taker_label (DEC-106). |
| Finished-permanent | A finished matching session never resets on quiz reopen; a new run needs a new quiz (DEC-127). |

## 7. Resolved Decisions (DEC-001..130 decided 2026-10-12; DEC-080/DEC-095/DEC-114 superseded 2026-10-13; no open decisions)

| ID | Resolution | Trace |
|----|-----------|-------|
| DEC-001 | Coverage at least 85% overall, 100% on critical paths listed in §2. | Enforced by CI gates. |
| DEC-002 | XLSX export in MVP (API-063, AC-219); CSV/PDF stay out. | Export scope fixed. |
| DEC-003 | Password at least 8 characters, no whitespace, at least one digit plus one uppercase; session 60 minutes sliding idle (BR-053/054). | Auth params fixed. |
| DEC-004 | Architect-recommended formulas decided: exact formulas in API-070 plus schema analytics (student attempts/total/average/answered/unanswered; teacher students/quizzes/questions/attempts/scores_given/pending_essays/cheat_total/anon_attempts). | Dashboard queries fixed. |
| DEC-005 | Timer seconds teacher-typed custom 1–86400 (NULL iff none); no presets. | BR-003, API-020, AC-106. |
| DEC-006 | Zero grace for answer saves — per-question POST past deadline rejected 410 (ERR-206); late submit returns 200 + auto-receipt (DEC-086). | BR-012, AC-203. |
| DEC-010 | Quiz duplication excluded from MVP. | FLOW-001. |
| DEC-011 | Matching start with 0 occupants BLOCKED with message. | STATE-002, AC-155. |
| DEC-020 | Go Fiber + `database/sql` MariaDB driver + explicit XLSX library. | ARCH-001/002/007. |
| DEC-021 | Bounded in-process background worker pool (2/100/10-min) for images. | BR-010, F-004, AC-104/105. |
| DEC-022 | SUPERSEDED by ARCH-009 (2026-10-14): NGINX removed — Docker exposed through Cloudflare directly, TLS terminates at Cloudflare, `proxy_buffering off` withdrawn (SSE drop risk accepted). | Infra + security. |
| DEC-023 | SSE push budget <2 s CONFIRMED. | ARCH-003, AC-208. |
| DEC-024 | bcrypt cost 12 CONFIRMED. | Schema + auth. |
| DEC-030 | Teachers never sit quizzes (strict). | RBAC, API-033. |
| DEC-031 | Strict quiz ownership (`owner_id=self`). | RBAC, AC-213. |
| DEC-032 | Password change revokes other sessions + SSE realtime. | BR-054, AC-161. |
| DEC-040/041 | Gotham Rounded self-hosted (UI-006) + DESIGN.md §3 palette (UI-007); text wordmark, no logo file. | Theme fixed; AC-215 verifies contrast. |
| DEC-042 | Two-step BUTTON confirm, no typed text. | BR-051, C-006/C-009. |
| DEC-050/052/053 | DEFERRED (skipped): backup/RTO/hardware operator-runbook concerns. | Infra deferred. |
| DEC-060/061/062 | DEFERRED (skipped): scanners/review/access operator-runbook concerns. | CI/CD deferred. |
| DEC-070/071/072/073 | Logs stdout-only, no files retained (070 revised R8 per user — persistent logging unnecessary; attempts/grades/audit persist in DB); thresholds deploy-time config (071); NO audit UI (072); NO rate limits (073, sole exception DEC-085 10 s guard). | Observability fixed. |
| DEC-080 | SUPERSEDED by DEC-114 (2026-10-13): jQuery 4.0.0 vendored (OBSERVED `assets/jquery-4.0.0/jquery-4.0.0.min.js` 78,749 B full build → `/static/jquery/`), FULL carrier — all jQuery APIs usable incl. modal/toast/collapse/tab; native-only = SSE/EventSource, sendBeacon, SPA/router. Old allow-list/ban lists deleted. | Client carrier fixed; arch/infra/CI/tests wired; AC-217 enforces vendored-scripts + inline-only. |
| DEC-081 | Deactivate BLOCKED (409 ERR-115) while the user holds `registered`/`answering` rows on an open quiz; teacher waits for submit/auto-submit or closes the quiz first. | Mid-exam lockout impossible; no sessionless-submit paradox. |
| DEC-082 | Quiz delete requires zero `registered`/`answering`/`submitted` rows (only `cancelled`/`revoked`-only or zero-row quizzes deletable); blocked → 409 ERR-111. | Live-delete orphan impossible (no FK). |
| DEC-083 | Attempts-exhaustion: register is the primary gate (no row); a stranded `registered` row that fails start auto-cancels in the same transaction (audit-logged). | No unstartable rows survive. |
| DEC-084 | Server generates the `Idempotency-Key` per rendered start/submit form (hidden field + header); client returns it verbatim. | Dedupe survives double-click + browser-close. |
| DEC-085 | Same matching control verb repeated within 10 s rejected 429 ERR-116 (server-side, in-memory per quiz); distinct verbs + post-10 s follow echo-vs-409. | Double-click impossible server-side. |
| DEC-086 | Past-deadline submit → `200` + auto-submitted receipt (not 4xx); ERR-206 covers per-question answer saves only. | 400 ms-late click is normal, not a fault. |
| DEC-087 | Student self-register creates the account then redirects to manual `/login` (no session at register). | Mistyped email caught at first login. |
| DEC-088 | Matching start transitions only EVER-PRESENT occupants (`connected=1 OR last_seen IS NOT NULL`); never-present stays `registered` no-show (never auto-submitted, revocable post-start); presence-stale 60 s. | Recap never shows false zeros. |
| DEC-089 | Deleting the last question of an `open` quiz BLOCKED with 422 ERR-108 (close first, then delete); drafts may go to zero. | Open-empty quizzes impossible. |
| DEC-090 | Offline answer queue dropped with notice (no auto-flush, no manual retry): answers that never reach the endpoint are dropped; the banner states what was lost. No answer-overflow column. | User decision R8; no silent application of stale answers. |
| DEC-091 | Anonymous quota per display name (exact match): the same name on the same quiz shares one `max_attempts` quota; a different name is a new participant; duplicate open codes stay rejected (ERR-114). | User decision R8; spoof-triviality documented. |
| DEC-092 | Quiz delete cascades to `cancelled`/`revoked` rows (audit-logged); deactivate is REJECTED with 409 ERR-115 while ANY owned quiz is `open` (Q3 + N-A5, 2026-10-14 — the auto-close clause is superseded; in-flight attempts continue normally). No orphan rows, no unmanageable quizzes. | User decision R8 + Q3 2026-10-14. |
| DEC-093 | Close/hide of a quiz ends its waiting session (`waiting → finished`, SSE `room_closed`); occupants' `registered` rows stay no-show (auto-revoke on close); Start on empty roster stays BLOCKED (ERR-113). Close is forbidden while the session is `running`/`paused` (409 — stop first). | User decision R8; empty-room deadlock closed. |
| DEC-094 | Quiz close/hide may come straight from the quiz list (no session stop required) — EXCEPT a live `running`/`paused` matching session: close/hide then returns 409 until stop. Explicit list action, no typed text. | User decision R8. |
| DEC-095 | SUPERSEDED by DEC-107: no intake key — editor prevents doubles via button disable + `jqXHR.abort()`; timeout-retry duplicates removed manually by teacher (AC-102). | Superseded by DEC-107; see FLOW-001. |
| DEC-096 | Entered-anon presence is memory-only: after a restart, entered anons re-identify (code+name) like pending intents; non-returners are never ever-present at start (DEC-088). | User decision R8; no new persistence. |
| DEC-097 | ERR-116 guard refined (R8): fires ONLY on same-verb repeats with a STALE `session_version` (accidental-double-click shape); current-version repeats echo; failed-verb (503/409) retries follow echo-vs-409, never 429. | Auditor P1-15 fix; echo promise kept. |
| DEC-098 | Realtime searchbar: 300 ms debounce on every search input (recap, quiz list, users, history); same canonical URL via jQuery AJAX partial swap + `history.replaceState` (reset `page=1`, other keys preserved; replaceState-only, no history entries — back exits list, DEC-111); no-JS = plain GET submit. | User decision R9; one URL contract, bounded request rate. |
| DEC-099 | Pagination: SSR default `per_page=25` (the `?per_page=` key MAY override up to 200); JSON/API lists use explicit `?page/&per_page=` up to 200. | User decision R9 ("default 25, max 200 custom"). |
| DEC-100 | Canonical list query keys `?q/page/per_page/window/class/status/date` — all list state lives in the URL (shareable/refreshable); unknown keys ignored, invalid values clamped to defaults (never 4xx). | User decision R9; URL fully reproduces state. |
| DEC-101 | Persistence via host bindmounts (`data/` → `/data/uploads` + datadir, gitignored, never in image) — replaces named volumes; container paths unchanged (ARCH-006). | User decision R9; backup unit = both host dirs + env inventory. |
| DEC-102 | NO polling of any kind and NO SSE auto-reconnect: on stream death the client closes the stream and shows a persistent notice ("Koneksi live terputus — jawaban tersimpan aman. Refresh halaman untuk menyambung ulang."); the ONLY recovery is a user page refresh rebuilding from server truth. Missed pause/stop invisible until refresh but still enforced server-side. | User decision R10; F-002/STATE-007/ERR-502/EDGE-035/AC-135 rewritten. |
| DEC-103 | Freeze+visible: draft always editable; open+hidden with registered/cancelled-only stays editable; open+show with ANY row frozen 422 ERR-107; answering/submitted rows anywhere always frozen. | BR-006; FLOW-001 step 4; ERR-107; AC-113; EDGE-065. |
| DEC-104 | Graded refresh: submit closes submitter stream; graded event only for open-stream viewers on the FINAL RESULT page `/attempts/:id/result` (Q5, 2026-10-14 — never the review page); result card updates only on next render (refresh/reopen), never in place. | FLOW-002 steps 7–8; FLOW-005 step 3. |
| DEC-105 | Deactivate-live: deactivate BLOCKED 409 ERR-115 while target owns running/paused matching session (self-deactivate included); stop first, auto-close otherwise blocked when live. | BR-051; FLOW-006; ERR-115; EDGE-066; AC-223. |
| DEC-106 | Anon active: one taker_label holds max one active (intent or answering) row per quiz; second start while active → 409 active conflict (ERR-201 family), else quota exhausted → 410 ERR-208. | BR-041; FLOW-004; EDGE-067; AC-224. |
| DEC-107 | Intake-key removal (supersedes DEC-095): no echo key; editor prevents doubles via button disable + jqXHR abort; timeout-retry duplicate removed manually by teacher. | FLOW-001 step 3; AC-102. |
| DEC-108 | Cheat CSRF exemption: POST .../cheat is CSRF-exempt (cookie+membership+dedupe+2s window replace CSRF); no other POST exempt. | API-000/03A, ARCH-003, F-005, AC-140..145. |
| DEC-109 | Teacher scope: teacher manages self + teachers + students for CREATE (teacher-creates-teacher allowed, PRD §3); teacher-to-teacher WRITE (edit/deactivate/delete another teacher) → 403 ERR-003. | PRD actors; FLOW-006; ERR-003. |
| DEC-110 | Two surfaces: normal-attempt progress surface + matching session-control surface; no separate monitor path. | FLOW-003 step 3; FLOW-007. |
| DEC-111 | ReplaceState-only: list search uses history.replaceState only (no history entries, back exits list); no history-restore claim. | BR-055. |
| DEC-112 | SSR+AJAX-minimal: non-q filters + pagination default full-page SSR GET; AJAX fragment only when explicitly needed (search q). | BR-055. |
| DEC-113 | Student-list-full: GET /student renders full open+visible list, no search/pagination; excluded from every-list rule (rule covers teacher lists + history). | BR-055; BR-007. |
| DEC-114 | jQuery-full R7 (supersedes DEC-080 allow/ban): all jQuery 4.0.0 usable; native-only = SSE/EventSource, sendBeacon, SPA/router. | UI-002, ARCH, CI, AC-217. |
| DEC-115 | Closed page: hidden → 404; closed+visible → 200 closed view (sudah ditutup, no join), excluded from discover list. | BR-007; STATE-004; FLOW-002 guard; EDGE-068; AC-226. |
| DEC-116 | Export: recap XLSX honors window/class/q/date/status, IGNORES page/per_page (full filtered set). | BR-014; BR-055. |
| DEC-117 | TTL entry-only: 24 h TTL is entry gate only; answering attempt + result card use row membership, never re-check TTL. | BR-041; FLOW-004; EDGE-069; AC-227. |
| DEC-118 | MariaDB 12.3.x current GA (example mariadb:12.3); exact patch pinned at build time. | ERD §1, ARCH-002, INFRA, CI. |
| DEC-119 | Guru-full per_q matching: per_question allowed on matching with no auto-advance; deadlines recorded for recap accuracy; movement follows teacher cues; monitor single index. | BR-003; BR-011; FLOW-003; EDGE-070; AC-228. |
| DEC-120 | Cheat via progress: teacher fan-out uses occupant_progress {attempt,current_index,last_answer,cheat_count}. | FLOW-008. |
| DEC-121 | Leave: POST /matching/:id/leave sets connected=0, keeps last_seen (still ever-present), broadcasts occupant_left, pre-start only. | FLOW-003; STATE-002; EDGE-071; AC-229. |
| DEC-122 | Fragment-wins: list fragment rule canonical in ARCH/FRONTEND (mentioned in FLOW-002 step 5 only). | FLOW-002 step 5 note. |
| DEC-123 | Two-layer unsaved: unsaved-draft semantics canonical in ARCH/FRONTEND (mentioned in FLOW-002 step 5 only). | FLOW-002 step 5 note. |
| DEC-124 | Generic banner: dead-stream banner is generic + saved-tick; no per-answer unsent list. | FLOW-002 offline note. |
| DEC-125 | Freeze countdown: the countdown FREEZES on BOTH triggers (dead-stream AND matching pause), plus the F-002 banner; timers frozen server-side. | FLOW-009; BR-013; F-002. |
| DEC-126 | Waiting verbs 200 echo: idempotent waiting-room verbs return 200 echo with current roster. | STATE-002; EDGE-072; AC-231. |
| DEC-127 | Finished permanent: session finished never resets on quiz reopen; new run needs new quiz. | STATE-004; EDGE-073; AC-232. |
| DEC-128 | No-JS three items: banner lists cheat detection + SSE push + toasts. | DS §1, EDGE-045, AC-145. |
| DEC-129 | Self-delete both: delete self allowed for both roles via inactive + confirm; student never-delete-self ban removed. | BR-051; FLOW-006. |
| DEC-130 | Deadline-only: attempt_deadline_utc/question_deadline_utc are sole truth for enforcement and recap. | BR-011; FLOW-002 step 4. |

Freshness + supersession policy (F-29, closed 2026-10-14): this table's rows
carry their own dates only where a decision was later superseded — a row marked
SUPERSEDED (DEC-095 → DEC-107, DEC-080 → DEC-114) keeps the losing wording for
the audit trail and names its winner. The rest of DEC-001..130 were decided in
the single 2026-10-12 session and are not re-dated. No row in this table is
undecided, and no open decision is recorded here; the open/open-resolved status
layer lives in `../00-INDEX.md` §5/§5b/§5c. When a future row is superseded,
the winning row is the only one a reader may follow.

Traceability: every REQ above maps to flows in `02-FEATURE-FLOWS.md`, rules in
`03-BUSINESS-RULES.md`, states in `04-STATE-MACHINES.md`, and tests in
`../06-QA-AND-MANAGEMENT/01-TEST-CASES.md`. Index: `../00-INDEX.md`.
