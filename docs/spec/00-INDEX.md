# quiz-v2 — Master Index and Traceability Control Center

Status: DRAFT | Updated: 2026-10-13

## 1. Project and outcome

quiz-v2 is a responsive-web, single-school daily-quiz system (Go monolith,
SSR, MariaDB 12.3 (DEC-118), Docker; realtime SSE + POST) that lets teachers author
mixed-type quizzes, run async or synchronous matching sessions with timers
and focus-loss anti-cheating, monitor live, grade essays, and recap scores
without spreadsheets. Mode: **A — Greenfield / From Scratch**. No BEFORE
implementation exists; BEFORE descriptions are user-reported practice only.

## 2. Canonical tree (all links relative, all files REQUIRED)

```text
docs/spec/
├── 00-INDEX.md (this file)
├── 01-BUSINESS/
│   ├── 01-PRD.md
│   ├── 02-FEATURE-FLOWS.md
│   ├── 03-BUSINESS-RULES.md
│   └── 04-STATE-MACHINES.md
├── 02-ARCHITECTURE-AND-TECHNICAL/
│   ├── 01-ARCHITECTURE-OVERVIEW.md
│   ├── 02-OPENAPI-SPEC.md (SSR+SSE+POST contracts; no REST/OpenAPI doc by design)
│   ├── 03-ERD-DATA-SCHEMA.md
│   └── 04-RBAC-MATRIX.md
├── 03-RESILIENCE-AND-ERRORS/
│   ├── 01-ERROR-MATRIX.md
│   ├── 02-FALLBACK-STRATEGY.md
│   └── 03-EDGE-CASE-PLAN.md
├── 04-FRONTEND-UIUX/
│   ├── 01-DESIGN-SYSTEM.md
│   ├── 02-COMPONENT-STATES.md
│   └── 03-PROTOTYPE-INTERACTION.md
├── 05-DEVOPS-AND-INFRA/
│   ├── 01-INFRA-TOPOLOGY.md
│   ├── 02-CICD-SPEC.md
│   └── 03-LOGGING-SECURITY-OBSERVABILITY.md
└── 06-QA-AND-MANAGEMENT/
    ├── 01-TEST-CASES.md
    ├── 02-BACKLOG-USER-STORIES.md
    └── 03-BEFORE-AFTER-DELTA.md
```

File responsibilities and coverage:

| File | Responsibility | Coverage |
|------|---------------|----------|
| 01-BUSINESS/01-PRD.md | Product, actors, scope, REQ-001..010 (all DEC resolved/deferred 2026-10-12; DEC-103..130 audit refinement 2026-10-13) | Complete; all S/N items resolved (§5/§5c) |
| 01-BUSINESS/02-FEATURE-FLOWS.md | FLOW-001..009 end-to-end (image async, no duplication; leave endpoint, closed view, graded next-render) | Complete with sequence/flow diagrams; S4b/S5 closed 2026-10-14 |
| 01-BUSINESS/03-BUSINESS-RULES.md | BR-001..055 deterministic rules (freeze hidden/show, TTL entry-only, replaceState-only, student-full list) | Complete; all S/N items resolved (§5/§5c) |
| 01-BUSINESS/04-STATE-MACHINES.md | STATE-001..007 lifecycles (leave, waiting-echo, finished-permanent) | Complete; S3 closed 2026-10-14 |
| 02-ARCHITECTURE-AND-TECHNICAL/01-ARCHITECTURE-OVERVIEW.md | Components, trust, trade-offs, ARCH-001..009 (Fiber, Cloudflare edge — NGINX removed, HTTP response caching + gzip, workers, XLSX, <2 s; MariaDB 12.3 pin, jQuery-full carrier) | Complete; S6 closed 2026-10-14 |
| 02-ARCHITECTURE-AND-TECHNICAL/02-OPENAPI-SPEC.md | API-000..070 + API-040b leave + API-039b history + API-063 XLSX operation contracts (hidden/show freeze, closed view, export ignores page) | Complete; all S/N items resolved (§5/§5c) |
| 02-ARCHITECTURE-AND-TECHNICAL/03-ERD-DATA-SCHEMA.md | Tables, indexes, transactions, seeds (R2 denormalized: 6 tables, 0 FK, 0 ENUM, 2 UNIQUE; MariaDB 12.3; no intake column, no question_started_at) | Complete; no open decisions |
| 02-ARCHITECTURE-AND-TECHNICAL/04-RBAC-MATRIX.md | 2-role matrix + enforcement (self + students + create teacher accounts per Q2, teacher-to-teacher write 403, strict no-sit, realtime revocation) | Complete; no open decisions |
| 03-RESILIENCE-AND-ERRORS/01-ERROR-MATRIX.md | ERR-001..502 contracts (hidden/show ERR-107, live-block ERR-115, cheat-exempt ERR-002, anon-conflict ERR-201/208) | Complete; all S/N items resolved (§5/§5c) |
| 03-RESILIENCE-AND-ERRORS/02-FALLBACK-STRATEGY.md | F-001..007 per-dependency resilience (generic saved-tick banner, frozen countdown, fragment-wins) | Complete; all S/N items resolved (§5/§5c) |
| 03-RESILIENCE-AND-ERRORS/03-EDGE-CASE-PLAN.md | EDGE-001..073 scenarios (EDGE-065..073 audit additions) | Complete; all S/N items resolved (§5/§5c) |
| 04-FRONTEND-UIUX/01-DESIGN-SYSTEM.md | Bootstrap full + inline CSS only + jQuery 4.0.0 full carrier R7 (DEC-114): UI-001..008, full §4 catalog mapping, closed inline-property list (Gotham + DESIGN.md palette), no-JS three items | Complete; all S/N items resolved (§5/§5c) |
| 04-FRONTEND-UIUX/02-COMPONENT-STATES.md | C-001..C-010 with exact Bootstrap bindings (student-full list, two-layer unsaved, frozen countdown, leave POST, no graded-live toast) | Complete; all S/N items resolved (§5/§5c) |
| 04-FRONTEND-UIUX/03-PROTOTYPE-INTERACTION.md | Routes + Bootstrap shell per route, nav graph, guards (closed view, two monitor surfaces, replaceState-only) | Complete; all S/N items resolved (§5/§5c) |
| 05-DEVOPS-AND-INFRA/01-INFRA-TOPOLOGY.md | Compose topology (Cloudflare edge — NGINX removed, Fiber image incl. jQuery vendor, MariaDB 12.3, deferred backup/RTO/hardware) | Complete; ops deferred per user |
| 05-DEVOPS-AND-INFRA/02-CICD-SPEC.md | Pipeline, gates (85%/100% coverage, vendored-script conformance, rollback) | Complete; all S/N items resolved (§5/§5c); scanner/review/access deferred per user |
| 05-DEVOPS-AND-INFRA/03-LOGGING-SECURITY-OBSERVABILITY.md | Logs (stdout-only), metrics, audit incl. live-block/conflict/403/leave/self-delete (no UI), runbook | Complete; no open decisions |
| 06-QA-AND-MANAGEMENT/01-TEST-CASES.md | AC-101..239 verification matrix (AC-223..232 audit additions, AC-234..239 caching ARCH-008, AC-210 next-render, AC-217 R7 carrier, AC-233 history, exit criteria A-H+J+K+N) | Complete; all S/N items resolved (§5/§5c) |
| 06-QA-AND-MANAGEMENT/02-BACKLOG-USER-STORIES.md | EPIC-1..8, US-### stories (US-404..407 audit additions, US-704 caching) | Complete; all S/N items resolved (§5/§5c) |
| 06-QA-AND-MANAGEMENT/03-BEFORE-AFTER-DELTA.md | CHG-001..008 greenfield+audit delta (CHG-008 audit refinement) | Complete; all S/N items resolved (§5/§5c) |

## 3. ID conventions

`REQ-###` requirements (PRD) · `BR-###` business rules · `FLOW-###` flows ·
`STATE-###` states · `DATA` = schema table/column names · `API-###`
operations · `ERR-###` errors · `EDGE-###` edge scenarios · `AC-###`
acceptance tests · `DEC-###` user decisions · `RISK-###` below ·
`ARCH-###`/`F-###`/`C-###`/`US-###`/`CHG-###` local to their docs.

**Letter suffixes are part of the ID** (user decision N-C5, 2026-10-14):
`API-039b`, `API-040b`, `API-03A`, `AC-126A`, `AC-126B`, `AC-134B`,
`AC-155B`, `AC-155C` and `Q1..Q12` are sanctioned IDs. A suffix must never be
parsed off, normalised away, or treated as a typo.

## 4. Traceability matrix (requirement → owner → flows → tests)

| REQ | Owner doc | Flows / Rules / States | API | Tests |
|-----|-----------|------------------------|-----|-------|
| REQ-001 authoring | 01-PRD, 03-BUSINESS-RULES BR-001..006/010 | FLOW-001 (DEC-103 hidden/show freeze, DEC-107 no echo) | API-020..024 + S1 | AC-101..109/113/116 (create/open/validate/image/timer/freeze-hidden-show/uniqueness/cancel-window/delete-last) |
| REQ-002 taking | PRD, RULES BR-007..009/011/012/030/031/053/054/055 | FLOW-002, STATE-001/003 (closed view, leave, graded next-render) | API-030..039 + API-040b + S2/S4/S5 | AC-110/111/115/116/118/123/125/126A/126B/130..134/134B/137/160/201..204/206/207/212/213/220/221/226/229/230 |
| REQ-003 grading | PRD, RULES BR-020..023 | FLOW-005/009, STATE-005 (next-render) | API-038/060/061 | AC-114/134B/205/210 (grade/audit/re-grade/queue/race-order/next-render) |
| REQ-004 recap | PRD, RULES BR-014/052/055 | FLOW-007 (two surfaces, export ignores page) | API-044/062/063/070 | AC-109/124/126A/126B/209/211/214/219/220/221/222/225 |
| REQ-005 anti-cheat | PRD, RULES BR-032 | FLOW-008 (occupant_progress, CSRF-exempt beacon) | API-03A | AC-130/140..145 |
| REQ-006 matching | PRD, RULES BR-008/013 | FLOW-003, STATE-002/007 (leave, waiting-echo, finished-permanent, guru-full per_q) | API-040/040b/041..043 | AC-112/116/134/152/153/154/155/155B/155C/156/204/208/228/229/231/232 |
| REQ-007 token/anon | PRD, RULES BR-040/041 | FLOW-004 (one-active, TTL entry-only) | API-050/051 | AC-107/108/121/150/151/156/224/227 |
| REQ-008 dashboards | PRD, UI C-001/C-006/C-008 (Daily Pulse; student-full list) | FLOW-006/007, STATE-004/006 | API-070 | AC-117/123/209/211/212/214/223 |
| REQ-009 responsive+time | PRD, ARCH, UI (Gotham + palette), FALLBACK F-002/003/004 (frozen countdown) | FLOW-009 | API-034..036 | AC-135..137/215/216/217 |
| REQ-010 deploy/scope | PRD, ARCH-001..009 (Fiber/Cloudflare edge/caching/workers/XLSX, MariaDB 12.3), INFRA, CICD (85%/100%) | — | /healthz /readyz | smoke + AC-136 |

Orphan check: every BR/FLOW/STATE/ERR/EDGE has ≥1 linked AC; every AC links
back — verified in audit §7 (2026-10-13, incl. EDGE-065..073 → AC-113/223/224/226/227/228/229/231/232 and AC-225/230 fragment/deadline legs).

## 5. Resolved decisions (DEC-001..130 decided or deferred; audit decision items S1..S6 and N1..N18 are recorded in §5b/§5c below — none remains genuinely open, the FINAL-blocking status is recorded per item in §5b)

| ID | Resolution | Trace |
|----|-----------|-------|
| DEC-001 | Coverage ≥85% overall, 100% on critical paths (grading, timers, submit idempotency, auth/session, RBAC, matching verbs). | CI gates, exit criteria. |
| DEC-002 | XLSX export in MVP (API-063, AC-219); CSV/PDF stay out. | Export scope fixed. |
| DEC-003 | Password ≥8 chars, no whitespace, ≥1 digit + ≥1 uppercase; 60-min sliding sessions (BR-053/054). | Auth fixed. |
| DEC-004 | Architect-recommended formulas decided (API-070 + US-601 + AC-214). | Dashboard fixed. |
| DEC-005 | Timer seconds teacher-typed custom 1–86400 (NULL iff none); no presets. | BR-003, API-020, AC-106. |
| DEC-006 | Zero grace for answer saves — per-question POST past deadline rejected 410 (ERR-206); late submit returns 200 + auto-receipt (DEC-086). | BR-012, AC-203. |
| DEC-010 | Quiz duplication excluded from MVP. | FLOW-001. |
| DEC-011 | Matching start with 0 occupants BLOCKED with message. | STATE-002, AC-155. |
| DEC-020 | Go Fiber + `database/sql` MariaDB driver + explicit XLSX library. | ARCH-001/002/007. |
| DEC-021 | Bounded in-process background worker pool (2/100/10-min) for images. | BR-010, F-004, AC-104/105. |
| DEC-022 | SUPERSEDED by ARCH-009 (2026-10-14): NGINX removed — the Docker `app` container is exposed through Cloudflare directly; TLS terminates at Cloudflare, and `proxy_buffering off` + ≥90 s keepalive are withdrawn (SSE long-lived-drop risk accepted). | Infra + security. |
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
| DEC-081 | Deactivate BLOCKED (409 ERR-115) while the user holds `registered`/`answering` rows on an open quiz. | BR-051, STATE-006, API-012, RBAC §5, AC-123. |
| DEC-082 | Quiz delete requires zero `registered`/`answering`/`submitted` rows (blocked → 409 ERR-111). | BR-052, API-022, schema §5, AC-109. |
| DEC-083 | Exhaustion: register is the primary gate (no row); stranded `registered` row auto-cancels at failed start (audit-logged). | BR-009, API-031/033, schema §5, STATE-003, AC-115. |
| DEC-084 | Server generates the start/submit `Idempotency-Key` (hidden field + header); echoed receipts reuse the original `request_id`; 24 h window. | API-000/033/038, schema §5, F-006, AC-110/131. |
| DEC-085 | Same control verb repeated within 10 s rejected 429 ERR-116 (in-memory per quiz); distinct verbs follow echo-vs-409. | API-043, STATE-002, AC-112. |
| DEC-086 | Past-deadline submit → `200` + auto-submitted receipt; ERR-206 covers answer saves only. | BR-012, API-038, schema §5, AC-203. |
| DEC-087 | Self-register creates the account then redirects to manual `/login` (no session at register). | FLOW-006, API-003, US-103. |
| DEC-088 | Matching start transitions only ever-present occupants (`connected=1 OR last_seen IS NOT NULL`); never-present stays `registered` no-show; presence-stale 60 s. | BR-008, STATE-002/007, API-043/044, AC-208/154/156. |
| DEC-089 | Deleting the last question of an `open` quiz BLOCKED with 422 ERR-108 (close first). | BR-005, API-024, AC-101. |
| DEC-090 | Offline answers dropped with notice (no auto-flush, no manual retry of never-received answers); only committed answers survive. | F-001, EDGE-030, AC-130, C-003. |
| DEC-091 | Anonymous quota per display name (exact match): same name shares one `max_attempts`; different name = new participant. | BR-041, API-033, schema §5, AC-150. |
| DEC-092 | Quiz delete cascades to `cancelled`/`revoked` rows (audit-logged); deactivate is REJECTED with 409 ERR-115 while ANY owned quiz is `open` — the auto-close clause is superseded per the user decision Q3 (2026-10-14) and N-A5. In-flight attempts continue normally. | BR-051/052, API-012/022, schema §5/9, observability §2. |
| DEC-093 | Empty-room exit: finish on empty `waiting` or close/hide while `waiting` → `waiting → finished` abort (SSE `room_closed`, rows no-show + auto-revoke on close); close/hide while `running`/`paused` → 409 stop-first. | STATE-002/004, API-022/031/043, EDGE-055B/C, AC-155B/C. |
| DEC-094 | Quiz close/hide straight from the list (no stop required) — EXCEPT live `running`/`paused` sessions (409 until stop). | API-022, prototype §3. |
| DEC-095 | SUPERSEDED by DEC-107 (2026-10-13): NO intake echo — editor prevents doubles via button disable + `jqXHR.abort()`; timeout-retry duplicates removed manually. | FLOW-001, API-023, AC-102. |
| DEC-096 | Entered-anon presence is memory-only: post-restart re-identify (code+name); non-returners never ever-present. | FLOW-004, AC-156. |
| DEC-097 | ERR-116 guard refined: fires ONLY on same-verb repeats with a STALE `session_version`; current-version repeats echo; failed-verb retries follow echo-vs-409, never 429. | API-043, STATE-002, F-007, AC-112, C-006. |
| DEC-098 | Realtime searchbar 300 ms debounce on every teacher/history search input; same canonical URL via jQuery AJAX partial swap + `history.replaceState` (refined by DEC-111 2026-10-13: replaceState-only, back exits list; student /student excluded by DEC-113); no-JS GET fallback. | BR-055, UI-002, API-010/022/062, C-008, EDGE-064, AC-221. |
| DEC-099 | Pagination SSR default `per_page=25` (overridable to 200); JSON/API explicit `?page/&per_page=` ≤200. | BR-055, API-060/062, C-008, AC-220. |
| DEC-100 | Canonical list keys `?q/page/per_page/window/class/status/date`; unknown ignored, invalid clamped (never 4xx); export reflects filtered view. | BR-055, API-062/063/070, AC-220/222, EDGE-063. |
| DEC-101 | Host bindmounts `data/` → `/data/uploads` + datadir (replaces named volumes; container paths unchanged). | INFRA §1, ARCH-006. |
| DEC-102 | NO polling of any kind and NO SSE auto-reconnect: dead stream → `EventSource.close()` + persistent notice + user-only refresh recovery; missed pause/stop invisible until refresh but enforced server-side. | F-002, STATE-007, ERR-502, EDGE-035/053, AC-135/153/208, C-005/006/010, PRD A1. |
| DEC-103 | Freeze+visible: draft always editable; open+hidden + registered/cancelled-only = editable; open+show + ANY row = 422 ERR-107; answering/submitted anywhere = always frozen. | BR-006, FLOW-001, API-021, ERR-107, EDGE-065, AC-113. |
| DEC-104 | Graded refresh: submit closes submitter stream; graded event only for open-stream viewers on the FINAL RESULT page `/attempts/:id/result` (Q5, 2026-10-14 — never the review page); result card updates only on next render, never in place. | FLOW-002/005, STATE-007, API-036/038/061, C-004, AC-210. |
| DEC-105 | Deactivate-live: deactivate BLOCKED 409 ERR-115 while target owns running/paused session (self incl.); peer-teacher attempt → 403 first (DEC-109). | BR-051, FLOW-006, API-012, ERR-115, EDGE-066, AC-223. |
| DEC-106 | Anon active: one taker_label = max one active row per quiz; second start while active → 409 conflict, else quota exhausted → 410 ERR-208. | BR-041, FLOW-004, API-033, schema §5, EDGE-067, AC-224. |
| DEC-107 | Intake-key removal (supersedes DEC-095): no echo promise; editor prevents doubles via disable+abort; duplicates deleted manually. | FLOW-001, API-023, AC-102. |
| DEC-108 | Cheat CSRF exemption: POST .../cheat is CSRF-exempt (cookie+membership+dedupe+2s window replace CSRF); no other POST exempt. | API-000/03A, ARCH-003, F-005, AC-140..145. |
| DEC-109 | Teacher scope: self + students + **create teacher accounts** (widened per PRD §3 + Q2, 2026-10-14 — authoritative); teacher-to-teacher WRITE (edit/deactivate/delete another teacher) → 403 ERR-003; create is exempt from that 403. The §7 audit note "narrowed to create-students-only per DEC-109" records a superseded intermediate state and is NOT the contract. | RBAC §1, API-010/011/012, AC-212/213. |
| DEC-110 | Two surfaces: GET /teacher/matching/:id + events (control, API-042) and GET /teacher/monitor/:quizId + events (monitor, API-044); both may open. | FLOW-003/007, API-042/044, C-006, AC-208/209. |
| DEC-111 | ReplaceState-only: list search uses replaceState only (back exits list); no history-restore claim. | BR-055, API-062, AC-221, US-403. |
| DEC-112 | SSR+AJAX-minimal: non-q filters + pagination = SSR GET; AJAX fragment only for search q. | BR-055, API-000, C-001/C-008, AC-225. |
| DEC-113 | Student-list-full: GET /student = full list, no search/pagination; excluded from every-list rule. | BR-055/007, API-030, C-001, AC-220. |
| DEC-114 | jQuery-full R7 (supersedes DEC-080 allow/ban): all jQuery 4.0.0 usable; native-only = SSE/EventSource, sendBeacon, SPA/router. | UI-002, ARCH, CI, AC-217. |
| DEC-115 | Closed page: hidden → 404; closed+visible → 200 closed view (sudah ditutup, no join), excluded from discover. | BR-007, STATE-004, RBAC §4, EDGE-068, AC-226. |
| DEC-116 | Export: XLSX honors window/class/q/date/status, IGNORES page/per_page (full filtered view). | BR-014/055, API-063, AC-222. |
| DEC-117 | TTL entry-only: 24 h TTL = entry gate; answering + result use row membership, never re-check TTL. | BR-041, FLOW-004, API-051, EDGE-069, AC-227. |
| DEC-118 | MariaDB 12.3.x current GA (example mariadb:12.3); exact patch pinned at build time. | ERD §1, ARCH-002, INFRA, CI. |
| DEC-119 | Guru-full per_q matching: per_question allowed, no auto-advance; deadlines recorded; movement follows teacher; single monitor index. | BR-003/011, FLOW-003, API-020/043/044, EDGE-070, AC-228. |
| DEC-120 | Cheat via progress: teacher fan-out uses occupant_progress {attempt,current_index,last_answer,cheat_count}; no cheat push to teachers. | FLOW-008/007, API-042, AC-208/209. |
| DEC-121 | Leave: POST /matching/:id/leave (API-040b), pre-start only; connected=0, last_seen kept (ever-present), broadcasts occupant_left. | FLOW-003, STATE-002, C-005, EDGE-071, AC-229. |
| DEC-122 | Fragment-wins: X-Requested-With fragment WINS for list endpoints unless Accept JSON on non-list endpoint. | API-000, BR-055, AC-225. |
| DEC-123 | Two-layer unsaved: native beforeunload when saving + Bootstrap dialog on dirty essay flag. | C-002, prototype §2, AC-202. |
| DEC-124 | Generic banner: offline banner generic + saved-tick (post-200 toast); no per-answer unsent list. | F-001, EDGE-030, C-002/C-010, AC-130. |
| DEC-125 | Freeze countdown: the countdown FREEZES under BOTH triggers — dead-stream (connection loss) and matching pause — plus the F-002 banner; timers frozen server-side (Q1 second-round decision, 2026-10-14). | F-002, FLOW-009, C-002, AC-135/153. |
| DEC-126 | Waiting verbs 200 echo: pause/stop/resume on waiting = 200 echo, never 409/429. | STATE-002, API-043, EDGE-072, AC-231. |
| DEC-127 | Finished permanent: session finished never resets on reopen; new run needs new quiz. | STATE-004, API-022, EDGE-073, AC-232. |
| DEC-128 | No-JS three items: banner lists cheat detection + SSE push + toasts. | DS §1, EDGE-045, AC-145. |
| DEC-129 | Self-delete both: both roles may self-delete via inactive+confirm; teacher self-delete BLOCKED while owning ANY quizzes. | BR-051, API-012, AC-212/213. |
| DEC-130 | Deadline-only: `attempt_deadline_utc`/`question_deadline_utc` are sole truth (canonical names per Q10); no question_started_at column. | BR-011, ERD §3/5, AC-230. |

Open decisions (2026-10-14, Q1): **none.** Every audit item whose decision is in fact
final is CLOSED and reads CLOSED; the rows below that used to be listed as open are now
recorded as closed with their decision text.

| ID | Question | Decision (2026-10-14) | Owner files |
|----|----------|----------------------|-------------|
| S2b | Closed-view page layout + button mapping + list-fragment template table for AC-220 determinism? | **CLOSED 2026-10-14:** quiz yang ditutup tidak bisa dibuka — murid yang mengklik menerima notifikasi (sudah-ditutup), no join/register/start controls, 200 page with a "sudah ditutup" message and no further action; fragment template mapping = `?q=` search only, other filters/pagination SSR GET (DEC-100/112/113/122). | BR-007/055, API-000, EDGE-063/068, AC-220/226 |
| S4-residual | C-006/C-010 UI-owned stale-spinner timing: when to clear the spinner from the heartbeat payload? | **CLOSED 2026-10-14:** stale spinner dibersihkan setelah 60s stale timeout — UI timing: spinner menghilang setelah 60s tanpa heartbeat baru. One threshold only: 60 s (Q7); no second 45 s number. | API-036, C-006/C-010, AC-135/209 |

**Closed S-items (resolved 2026-10-14 audit, removed from open table):**
- S1: `Retry-After: 1` on 429 ERR-112 only (BR-051 §S1, API-023, AC-105)
- S2a: Invalid `?page` clamped to page-1/last-page, never 4xx (DEC-100, API-000, BR-055, EDGE-063, AC-220)
- S2b: Closed-view page layout + list-fragment template table (BR-007/DEC-115; DEC-112/113/122)
- S3: ERR-116 guard is in-memory per quiz, does NOT survive restart; accept in-memory loss (STATE-002, API-043, F-007, AC-112)
- S4a: Heartbeat payload `{server_now_utc}` is FINAL — a `heartbeat` event every 30 s (API-036, STATE-007)
- S4b: Submit-vanish: answering row disappears from monitor on submit; no new SSE event name (API-044, FLOW-007, C-006, AC-209)
- S4-residual: Stale spinner cleared at 60 s — single threshold, 60 s (Q7); no second 45 s number
- S5: Cancelled/revoked row renders terminal-state card ("Percobaan dibatalkan/dicabut"), no score, no correct answers, printable (API-039, FLOW-002, AC-206)
- S6: (a) MariaDB 12.3.x CI service + sqlite-allowed locally with divergence runbook; (b) deploy-fail = rollback blip; (c) no waiver — DEC-001 gates enforced (CICD §4, ARCH §6, AC-136)

## 5c. Consolidated Unresolved Open Questions (2026-10-14 parallel audit) — now resolved; no question below is still open

The following genuinely unresolved open questions were identified by the multi-level
file audit (2 orchestrators + 5 child scouts = 7 agents, all 21 files covered,
synchronized via `wait`; a prior 5-auditor and a 10-auditor audit are separate
earlier runs and are not the source of this list). All required a decision
before FINAL; per the user decision Q1 of 2026-10-14, every row below is now
RESOLVED and is retained here as the decision record, not as an open question.
Findings that are already answered elsewhere in the suite are NOT listed here —
see §5b for the full audit trail.

|| ID | Decision | Owner files | Severity | Status |
||----|----------|-------------|----------|--------|
|| S2b | Closed-view page: quiz yang ditutup tidak bisa dibuka. Murid yang mengklik akan menerima notifikasi (sudah-ditutup). Tidak ada tombol join/register/start. Layout: halaman 200 dengan pesan "sudah ditutup" dan tidak ada aksi lanjut. | BR-007/055, API-000, EDGE-063, AC-220, 04-FRONTEND-UIUX | HIGH | RESOLVED 2026-10-14 |
|| S4-residual | Stale spinner dibersihkan setelah 60s stale timeout. Payload shape {server_now_utc} sudah diputus; UI timing: spinner menghilang setelah 60s tanpa heartbeat baru. | API-036, C-006/C-010, AC-135/209, 04-FRONTEND-UIUX | HIGH | RESOLVED 2026-10-14 |
|| N8 | PRD adalah dokumen yang otoritatif. INDEX §5b dan §5c diubah untuk mencocokkan PRD. PRD actor table: guru dapat membuat guru baru (tambahkan ke dalam daftar guru yang ada). DEC-080/108/114/118 mengikuti kata-kata PRD. | 01-PRD.md §7, 00-INDEX.md §5b | HIGH | RESOLVED 2026-10-14 |
|| N5 | Ditinggalkan apa adanya. Quiz tanpa timer (timer_mode=0): answering row tidak pernah auto-submit, tidak ada sweep, taker_label permanen 409/410-block. Guru dapat langsung menutup quiz untuk membersihkan baris yang terkunci. Tidak ada sweep otomatis. | ERD §5/§9, API-033, 02-OPENAPI-SPEC | MEDIUM | RESOLVED 2026-10-14 |
|| N12 | Banner offline dibersihkan hanya melalui user refresh. Tidak ada polling (DEC-102), tidak ada auto-reconnect. Banner persisten "Koneksi live terputus — jawaban tersimpan aman. Refresh halaman untuk menyambung ulang." | F-001, 03-RESILIENCE-AND-ERRORS/02-FALLBACK-STRATEGY, devops gates | MEDIUM | RESOLVED 2026-10-14 |
|| N13 | "Late-flow page" di-enumerasi secara eksplisit di FLOW-002 step 7 dan FLOW-005 step 3. Halaman yang dimaksud: /attempts/:id/result (halaman hasil akhir setelah submit). Event live untuk grade update hanya untuk viewers dengan stream terbuka di halaman tersebut — bukan halaman review. | FLOW-002, FLOW-005, 01-BUSINESS/02-FEATURE-FLOWS, 00-INDEX §5b | MEDIUM | RESOLVED 2026-10-14 |
|| CHG-008 | DELTA CHG-008 ditambahkan kolom "must remain unchanged" dan acceptance clause. AC-101..219 diperluas ke AC-223..233 (user decision 2026-10-14: upper bound is AC-233, and AC-233 = the API-039b student-history case is inside CHG-008's accepted scope; the earlier AC-223..232 bound is superseded). | 06-QA-AND-MANAGEMENT/03-BEFORE-AFTER-DELTA | LOW | RESOLVED 2026-10-14 |

**Note on N8 (resolved 2026-10-14):** N8 was listed as both resolved and open in §5b (the original line 227 reference no longer resolves after the §5b renumbering; see N-C2). Per user decision, PRD is authoritative and INDEX is aligned to it. N8 is now RESOLVED and appears only as resolved in §5c.

## 5d. Audit Findings Register — 2026-10-14 five-subagent parallel audit (F-01..F-72)

Durable record of the audit. **Nothing in this register may be deleted**; each
finding is closed by recording its resolution in §5e (Decision Answers) and by
the cleanup edits in the owner files, never by removing the finding.

### 5d.1 Coverage

Five parallel audit subagents (A, B, C, D, E), each permitted to spawn nested
subagents, synchronized via `wait`; 21 files, 100% coverage; 97 raw findings
consolidated to 72 unique after deduplication. Audit only — no spec file was
modified by this pass. High-impact findings were re-verified against source by
the main agent before recording.

| Agent | Files owned | Coverage |
|---|---|---|
| A | `00-INDEX.md`, `01-BUSINESS/01-PRD`, `02-FEATURE-FLOWS`, `03-BUSINESS-RULES`, `04-STATE-MACHINES` | Complete |
| B | `02-ARCHITECTURE-AND-TECHNICAL/01-ARCHITECTURE-OVERVIEW`, `02-OPENAPI-SPEC`, `03-ERD-DATA-SCHEMA`, `04-RBAC-MATRIX` | Complete |
| C | `03-RESILIENCE-AND-ERRORS/01-ERROR-MATRIX`, `02-FALLBACK-STRATEGY`, `03-EDGE-CASE-PLAN`, `04-FRONTEND-UIUX/01-DESIGN-SYSTEM`, `02-COMPONENT-STATES`, `03-PROTOTYPE-INTERACTION` | Complete |
| D | `05-DEVOPS-AND-INFRA/01-INFRA-TOPOLOGY`, `02-CICD-SPEC`, `03-LOGGING-SECURITY-OBSERVABILITY` | Complete |
| E | `06-QA-AND-MANAGEMENT/01-TEST-CASES`, `02-BACKLOG-USER-STORIES`, `03-BEFORE-AFTER-DELTA` | Complete |

### 5d.2 Summary by category and severity

| Category | HIGH | MEDIUM | LOW | Total |
|---|---:|---:|---:|---:|
| Ambiguity (missing values, undefined terms) | 3 | 11 | 3 | 17 |
| Internal inconsistency (self-contradiction) | 4 | 8 | 1 | 13 |
| Cross-document contradiction | 6 | 8 | 1 | 15 |
| Decision-marker issues (stale/open markers) | 5 | 0 | 0 | 5 |
| Answered-but-open questions | 3 | 0 | 0 | 3 |
| Unanswered open questions | 1 | 0 | 0 | 1 |
| Coverage gaps (missing definition/test) | 4 | 12 | 2 | 18 |
| **Total** | **26** | **39** | **7** | **72** |

> **Reconciliation (N-A10, resolved 2026-10-14):** the finding count of **72 is correct** — F-01..F-72 are all present in §5d.4/§5d.5, so no finding is missing and none is duplicated. Only the *breakdown* was wrong: the seven category rows previously summed to 74 overall (HIGH 26, MEDIUM 35, LOW 11), i.e. **2 findings were double-counted in the category rows** while every category total under- or over-stated its own share. The authoritative split above is a row-by-row recount of the Category column in §5d.4 (F-01..F-26) and §5d.5 (F-27..F-72): HIGH 26, MEDIUM 39, LOW 7 = 72. No finding row, severity, or ID was changed.

### 5d.3 Root cause — broken open-vs-closed status layer (DF-01..DF-08)

Eight of the ten audit agents' HIGH findings share one cause: items marked
CLOSED in one file and OPEN in another. This table is the map of every status
conflict; cleanup phase 1 resolves each one.

| ID | Item | Status conflict | Evidence |
|---|---|---|---|
| DF-01 | **S1** Retry-After | `00-INDEX.md:221` "GENUINELY OPEN" vs `:223` CLOSED; `:321` still listed open; `01-TEST-CASES.md` §M still requires a decision | line 221 *"S1 remains GENUINELY OPEN — see §5b."* vs line 223 *"### S1 — ERR-112 Retry-After (CLOSED 2026-10-14)"* |
| DF-02 | **S2b** closed-view layout | closed in INDEX/EDGE-CASE vs *"remains OPEN, owned by the frontend slice"* | `03-BUSINESS-RULES.md` line 96; API-000; AC-226 |
| DF-03 | **S4-residual** spinner timing | closed in INDEX §5c vs *"see N13 territory"* | `02-FEATURE-FLOWS.md`:204; `04-STATE-MACHINES.md`:160-161 |
| DF-04 | **S4a** heartbeat payload | decided in API-036/STATE-007 vs three UI files still DECISION REQUIRED | 02-COMPONENT-STATES; 01-DESIGN-SYSTEM L216; 03-PROTOTYPE-INTERACTION L151 |
| DF-05 | **S3, S5, S6** | decided in owner files vs still open in `01-TEST-CASES.md` §M | FLOW-002:8; API-039; CICD §4 |
| DF-06 | **§5 header vs open table vs §9** | §5 names two open items (both shown RESOLVED in §5c); §9 claims S1..S6 + N1..N18 all blocking | line 98 vs 176-180 vs 699; line 372 |
| DF-07 | **N13** late-flow page | RESOLVED in INDEX but the resolution was never written into the two owner sites | `02-FEATURE-FLOWS.md`:67 and :171 |
| DF-08 | **QA exit criteria** | *"no open DEC except S1..S6"* cannot be evaluated while the list contradicts every owner file | `01-TEST-CASES.md` §M line 433 |

### 5d.4 HIGH-severity findings (F-01..F-26)

Recommendation column gives the proposed fix direction; the binding choice is
recorded in §5e.

| ID | File · Location | Category | Finding | Evidence | Recommendation |
|---|---|---|---|---|---|
| F-01 | `01-PRD.md` L52 · `03-BUSINESS-RULES.md` L96 · `02-OPENAPI-SPEC.md` L154 | Cross-document | PRD says the seeded teacher creates further teachers; API-010/RBAC/DEC-109 forbid it; N8 claims resolution but INDEX L152 was never corrected | *"First teacher account created by database seed; can then create further teachers/students."* | Align all owner files to one verb set (see Q2) |
| F-02 | `02-OPENAPI-SPEC.md` L172-178 · `04-RBAC-MATRIX.md` L90-99 vs `03-BUSINESS-RULES.md` L284-300 | Cross-document | Deactivate is simultaneously guard-blocked (open) and force-submitting (closed) while DEC-092 auto-closes in the same transaction; precedence undefined, no AC | *"attempt rows persist (guard-blocked while live on open quizzes, force-submitted while live on closed quizzes …)"* | Specify transaction order and a dedicated AC (see Q3) |
| F-03 | `02-OPENAPI-SPEC.md` L368-371 vs `04-STATE-MACHINES.md` L143-148 | Cross-document | API-036 implements the rejected loser of N3: a 60 s-stale occupant marked never-present, while STATE-007 keeps stale occupants ever-present | *"60 s stale presence → server marks the occupant never-present (DEC-088)"* vs *"is STILL ever-present"* | Rewrite API-036 to STATE-007/N3 (see Q4) |
| F-04 | `02-FEATURE-FLOWS.md` L67, L171 | Ambiguity | `late-flow page` is the live-event trigger but is never enumerated in either owner site; N13 resolution never written back | *"fires only for other viewers still on the review/late-flow page with an open stream"* | Write the enumerated page pair into both sites (see Q5) |
| F-05 | `02-OPENAPI-SPEC.md` L402 vs `06-QA/01-TEST-CASES.md` | Unanswered / coverage gap | API-039b student-history operation has no asserting AC and is absent from INDEX §2 — the "every AC links back" gate is false | API-039b defines *"requireStudent, R1 scoping"* | Add asserting AC or delete the op and re-scope to API-062 (see Q6) |
| F-06 | `05-DEVOPS/02-CICD-SPEC.md` L41 vs `03-ERD-DATA-SCHEMA.md` §8 | Cross-document | Pipeline invokes a standalone `migrate` CLI the architecture never builds | *"`migrate up --dry` against ephemeral MariaDB 12.3.x current GA"* vs *"applied by the Go binary at boot"* | Name the runner or state it is an in-app boot subcommand |
| F-07 | `03-LOGGING-SECURITY-OBSERVABILITY.md` L44-45 | Coverage gap | A status page is asserted as a health-contract consumer but no `/status` route exists in any route inventory | *"proxy/deploy gates use `/readyz`; status page links map here."* | Create and authorise the page, or delete the sentence |
| F-08 | `03-LOGGING-SECURITY-OBSERVABILITY.md` L38-52 vs `01-INFRA-TOPOLOGY.md` L49-53 | Internal | `/metrics` (13 series) is mandated but appears in no topology/pipeline/smoke; host disk/mem alerts asserted while all host detail is out of spec | *"Dashboards/alerts: host-level (disk, mem) + app ERROR-rate spike rule."* / *"outside spec, DEC-070"* | Add a metrics scrape/exposure decision and a host-collector decision |
| F-09 | `05-DEVOPS/02-CICD-SPEC.md` L40 | Coverage gap | *"distroless-ish"* runtime asserted but no stage scans the base or built image | *"Multi-stage Dockerfile (builder → distroless-ish runtime); image `:sha`"* | Add an image-scan stage or drop the claim |
| F-10 | `05-DEVOPS/01-INFRA-TOPOLOGY.md` L59-62 | Coverage gap | A restore drill is a pre-go-live gate, yet RTO/RPO/hardware are deferred and no stage or AC implements or schedules it | *"Restore drill required before go-live once the operator defines it."* | Define RTO/RPO and a scheduled, verified drill, or remove the gate |
| F-11 | `05-DEVOPS` §3-4 (all three files) | Ambiguity | All alert thresholds defer to deploy-time config with no owner, format or example; no SLO/SLI or error budget exists | *"this spec asserts no SLO percentages"* | Pin thresholds or name the config artifact and owner (see Q11) |
| F-12 | `03-RESILIENCE/01-ERROR-MATRIX.md` ERR-115 | Internal | The trigger covers two legs; the user-facing message names only the open-quiz leg | *"OR owns a running/paused matching session"* vs *"Akun tidak dapat dinonaktifkan selama murid terdaftar/aktif di quiz yang dibuka."* | Widen the single Indonesian string to both triggers |
| F-13 | `01-ERROR-MATRIX.md` (missing row) | Coverage gap | BR-011's back-navigation 410 has no error-matrix row, component state, message, or logging action | *"No back-navigation: GET for n < current_index returns 410 with read-only snapshot."* | Add an ERR row plus a read-only component state |
| F-14 | `03-BUSINESS-RULES.md` L60-61 vs `03-ERD-DATA-SCHEMA.md` §2 / API-020 | Cross-document | The essay-only acknowledgement checkbox has no column, no transport field, no UI state, no test | *"at least one non-essay question OR teacher acknowledges essay-only (explicit checkbox)."* | Add storage/transport/state or derive it from the question set (see Q9) |
| F-15 | `02-COMPONENT-STATES.md` C-006 vs `00-INDEX.md` §5c | Internal (numeric) | Stale-spinner timing stated as 45 s in one file and 60 s in the other for the same event, against a 30 s heartbeat | *"`stale` (no heartbeat 45 s → amber banner …)"* vs *"Stale spinner dibersihkan setelah 60s stale timeout"* | Choose one threshold, or name both roles explicitly (see Q7) |
| F-16 | `06-QA/01-TEST-CASES.md` §M | Answered-but-open | S1 and S2b still listed open though decided and asserted (AC-105, AC-226) | *"**S1 Retry-After:** whether 409/429 responses carry `Retry-After` headers"* | Rewrite §M to the CLOSED text (see Q1) |
| F-17 | `00-INDEX.md` L221 vs L223 | Answered-but-open | S1 sits inside the genuinely-unresolved section while answered in that same section | *"S1 remains GENUINELY OPEN"* vs the CLOSED heading | Reconcile §5/§5b/§5c to one status list (see Q1) |
| F-18 | `04-STATE-MACHINES.md` L160-161 · `02-FEATURE-FLOWS.md` L204 | Decision marker | Residual/N13 spinner markers survive in two owner files that already carry the decided payload | *"Residual: C-006/C-010 UI-owned stale-spinner timing … see N13 territory."* | Replace with the CLOSED decision text |
| F-19 | `02-COMPONENT-STATES.md` · `01-DESIGN-SYSTEM.md` L216 · `03-PROTOTYPE-INTERACTION.md` L151 | Decision marker | S4-heartbeat still DECISION REQUIRED in three UI files; one block also self-contradicts | *"S4 (resolved 2026-10-14 per §5b …)"* followed by *"The payload half remains open and is UI-owned"* | Remove all three markers |
| F-20 | `02-OPENAPI-SPEC.md` L90-97 vs `03-BUSINESS-RULES.md` L96 | Decision marker | API-000 still describes a remaining S2 scope, then disowns the closed-view layout to the frontend slice | *"The remaining S2 scope is the closed-view layout and the list-fragment template table (below)."* | Remove the remaining-scope sentence |
| F-21 | `00-INDEX.md` §5 header vs open table vs §9 | Decision marker | Three mutually exclusive statements of what is open; the exit criterion has no verifiable truth value | line 98 vs 176-180 vs 699; line 372 | Regenerate the open list from one authoritative source (see Q1) |
| F-22 | `00-INDEX.md` L52 vs L699 | Cross-document | INDEX §2 declares FEATURE-FLOWS complete while §9 keeps S1..S6 + N1..N18 globally FINAL-blocking | *"Complete with sequence/flow diagrams"* vs *"S1..S6 + audit findings N1..N18 open and BLOCKING"* | Make §2 and §9 use one shared status vocabulary |
| F-23 | `03-EDGE-CASE-PLAN.md` L~131, L~99 | Decision marker | *"only S2b and N13 remain genuinely open"* while INDEX marks both resolved | *"only S2b and N13 remain genuinely open"* | Rewrite to the resolved status |
| F-24 | `06-QA/01-TEST-CASES.md` L411-433 | Answered-but-open | S3, S5 and S6 remain in the open-decision list though closed in owner files | *"Open decisions (DECISION REQUIRED S1..S6)"* | Rewrite §M and the exit criteria |
| F-25 | `01-INFRA-TOPOLOGY.md` L49-53 vs CICD L41-43, Observability L46-51 | Internal | Dev bypasses the proxy yet `/readyz` is declared the proxy gate, and only 80/443 may be published | *"Dev may bypass (app serves HTTP directly)"* / *"`/readyz` … is the deploy, proxy, and CI smoke gate"* | State the dev port and the dev deploy gate explicitly |
| F-26 | `01-INFRA-TOPOLOGY.md` L29/L30/L78 · `02-CICD-SPEC.md` | Ambiguity | Migration-failure-at-boot, the MariaDB internal port, NTP verification and the NGINX config artifact are all unspecified | *"internal port only (never published)"* with no port number anywhere | Pin port, boot-failure semantics, clock check and config file (see Q11) |

### 5d.5 MEDIUM and LOW findings (F-27..F-72)

| ID | Location | Sev | Category | Finding (evidence) | Recommendation |
|---|---|---|---|---|---|
| F-27 | `03-BUSINESS-RULES.md` L165 | M | Ambiguity | Names `attempt_deadline_utc`; ten other sites and BR-012's own next bullet use `deadline_utc` | Normalize to `deadline_utc` (or declare alias) — see Q10 |
| F-28 | `01-PRD.md` L61 vs `03-BUSINESS-RULES.md` L137-138 | M | Ambiguity | 10 **MB** (PRD, AC-105) vs 10 **MiB** (BR-010, F-004) — 4.8 % band undefined | State exact bytes once — see Q8 |
| F-29 | `01-PRD.md` L116 vs L141/L156 | M | Internal | §7 claims *"all DEC decided or deferred 2026-10-12"* while two rows above are 2026-10-13 supersessions | Update the freshness date and supersession policy |
| F-30 | `02-FEATURE-FLOWS.md` L173 vs `01-PRD.md` L52 | M | Cross-document | FLOW-006's blanket 403 has no bootstrap exception the PRD implies | Align with Q2 |
| F-31 | `01-PRD.md` L90 | M | Ambiguity | A1 keeps DEC-125's losing wording (pause freezes countdown) against the canonical dead-stream freeze | Rewrite to dead-stream freeze |
| F-32 | `02-FEATURE-FLOWS.md` L3 | M | Ambiguity | References a file whose title forbids REST/OpenAPI: *"does NOT expose a REST/JSON API or OpenAPI document"* | Add a "Transport Contracts, no OpenAPI" qualifier at each reference |
| F-33 | `03-BUSINESS-RULES.md` L181 | M | Cross-document | Writes `paused_at`; schema column is `session_paused_at` | Rename to match ERD — see Q10 |
| F-34 | `02-FEATURE-FLOWS.md` L36 vs `04-STATE-MACHINES.md` L112-113 | M | Ambiguity | *"Quiz may then be closed/hidden"* vs the stricter state-precedent chain and no N2 finished-session verb | State the full precedent chain |
| F-35 | `04-STATE-MACHINES.md` L40/L59/L62/L66 | M | Internal | Declared 5-state model contradicted by its own table (`waiting leave` row, `→ stopped` from running/paused) | Label leave as occupant-level; list all valid exits |
| F-36 | `03-BUSINESS-RULES.md` L346 | L | Ambiguity | Depends on a UI-002 global `ajaxComplete` handler named in no component and asserted by no AC | Name the component and asserting AC |
| F-37 | `01-ERROR-MATRIX.md` ERR-104 | M | Coverage gap | HTTP cell is not a status — *"async (surfaced on editor as `failed` + reason …)"* | Split accepted-vs-failed rows with defined statuses |
| F-38 | `02-FALLBACK-STRATEGY.md` L192 · `01-DESIGN-SYSTEM.md` L171/L206 | M | Ambiguity | Both mandate a validated requirement *"per §2.8"* — §2.8 exists in neither file | Create §2.8 or repoint the anchors |
| F-39 | `03-EDGE-CASE-PLAN.md` EDGE-045 vs `02-COMPONENT-STATES.md` C-010 | M | Cross-document | Banner body quoted in one file, listed as a three-item set in two others | Pin one copy block |
| F-40 | `02-COMPONENT-STATES.md` C-005 | M | Coverage gap | `progress` needs *"occupants vs class size"*; no `classes` table or `class_size` column exists | Add the field or change the metric |
| F-41 | `01-DESIGN-SYSTEM.md` vs vendored Bootstrap | M | Coverage gap | Toast duration, motion tokens, breakpoint pixels and dark mode unpinned (Bootstrap default 0.15 s only) | Pin a motion scale — see Q12 |
| F-42 | `docs/DESIGN.md` §3.1 vs `01-DESIGN-SYSTEM.md` | M | Cross-document | Token values live only in a non-spec file; `metric-lg` is a range while AC-215 asserts exact pairs | Resolve authority — see Q12 |
| F-43 | `01-DESIGN-SYSTEM.md` L181-183 | M | Coverage gap | Accessibility names mechanisms but pins no contrast ratio, focus-ring value, keyboard nav or screen-reader label | Pin the values AC-215 must assert |
| F-44 | `02-COMPONENT-STATES.md` L7 | L | Ambiguity | Normative pointer to *"the skill's §8.14 guidance"* — §8.14 exists in no repo file | Repoint or remove |
| F-45 | `01-ERROR-MATRIX.md` ERR-204/ERR-301 | L | Coverage gap | Entries with no logging action defined | Add logging actions |
| F-46 | `02-OPENAPI-SPEC.md` L154 | M | Cross-document | Same teacher-creation contradiction as F-01 seen from the transport layer | Resolve with F-01 — see Q2 |
| F-47 | `01-ARCHITECTURE-OVERVIEW.md` ARCH-001 | M | Coverage gap | *"grounded companions"* verified but unscoped — no template engine named, so SSR has no named engine | Name the engine and each companion's applied surface |
| F-48 | `03-ERD-DATA-SCHEMA.md` L350-353 vs L201-204 | M | Internal | §6 splits monitor (state=1) from roster (state=0) while API-043/044 claim a single `(quiz_id, state)` index the schema never declares | Declare the index or restate the API notes |
| F-49 | `04-RBAC-MATRIX.md` L25 | M | Ambiguity | The "Manage users" cell states self+students twice with a parenthetical but never the create scope | State the create scope in the matrix itself |
| F-50 | `01-ARCHITECTURE-OVERVIEW.md` L101-103 | M | Ambiguity | One 429 has two shapes (JSON envelope vs inline HTML); the precedence table never covers multipart intake | Add an image-intake row to the precedence table |
| F-51 | `01-ARCHITECTURE-OVERVIEW.md` L108-118 | L | Internal | 10-line re-statement of the S6 CI trio duplicates CICD §4's independent wording | Reduce to a pointer |
| F-52 | `01-ARCHITECTURE-OVERVIEW.md` L96-99 | M | Coverage gap | Timer respawn for a paused session undefined, so AC-136's *"past-due fire immediately"* may auto-submit paused attempts | Exclude paused sessions from the sweep |
| F-53 | `03-LOGGING-SECURITY-OBSERVABILITY.md` L8, L97-99 | M | Internal | Runbook queries `auth_invalid` WARN rate, but no field guarantees it and no component emits it | Name the emitting component and field guarantee |
| F-54 | `01-INFRA-TOPOLOGY.md` L57 | M | Ambiguity | Cites *"envelope §ARCH-006"* for capacity caps that live in ARCH §6 | Correct the pointer |
| F-55 | `01-INFRA-TOPOLOGY.md` L41-45 vs Observability §5 | M | Coverage gap | `SESSION_SECRET` rotation defined in INFRA only; no rotation period for any secret in the security spec | Pin rotation cadence and verification |
| F-56 | `03-LOGGING-SECURITY-OBSERVABILITY.md` L70-71 vs ERD §9 | M | Coverage gap | Audit retention is *"same as attempts"* and the schema says *"No TTL purge in MVP"* — unbounded PII, no delete path | Choose a retention value and a delete path |
| F-57 | `03-LOGGING-SECURITY-OBSERVABILITY.md` L76 | M | Coverage gap | No TLS version, cipher suite, HSTS or certificate-renewal requirement anywhere | Pin a minimum TLS version and renewal policy |
| F-58 | `03-LOGGING-SECURITY-OBSERVABILITY.md` L74-84 | M | Coverage gap | No CORS/CSP/X-Frame-Options policy for the app or proxy | Add a header policy |
| F-59 | `03-LOGGING-SECURITY-OBSERVABILITY.md` L60-72 vs ERD §9 | M | Internal | Observability's audit-event list is a superset of the schema's with no authoritative reconciliation | Declare one list authoritative |
| F-60 | `02-CICD-SPEC.md` L9 vs L41 | M | Internal | *"SAST/DAST tooling … REQUIRED"* but no DAST stage exists; SAST counted from the Lint stage | Correct the scope sentence or add the stage |
| F-61 | `02-CICD-SPEC.md` L61-64 vs `01-INFRA-TOPOLOGY.md` L59-62 | M | Internal | Corrected rollback-blip wording lives only in CICD; INFRA still implies zero-downtime | Merge the wording into INFRA |
| F-62 | `02-CICD-SPEC.md` L42, L49-50 | M | Ambiguity | Deploy-access method and reviewer count deferred, leaving the prod approval button undefined | Pin key custody and reviewer count |
| F-63 | `03-LOGGING-SECURITY-OBSERVABILITY.md` L44-48 | M | Coverage gap | `/healthz`, `/readyz`, `/metrics` have no operation entry in the API spec — no contract for any consumer | Add API entries with contract |
| F-64 | `03-LOGGING-SECURITY-OBSERVABILITY.md` L14-15 | L | Ambiguity | The *"explicit flag"* permitting DEBUG essay excerpts is named nowhere | Name the flag and default |
| F-65 | `06-QA/01-TEST-CASES.md` L43-44 | M | Coverage gap | AC-108 is the only case with no source-requirement link, breaking the file's own *"Each case: linked IDs"* invariant | Add the parent link |
| F-66 | `03-BEFORE-AFTER-DELTA.md` L25, L46 | M | Internal | CHG-008 cites an AC range that does not exist and omits the rows that cover its after-state | Correct the AC range |
| F-67 | `01-TEST-CASES.md` AC-115 vs `02-OPENAPI-SPEC.md` API-033 §5 | M | Cross-document | AC-115's auto-cancel contradicts the accepted N5 limitation; its cause list omits the timer-less case | Extend AC-115 to the N5 tail |
| F-68 | `01-TEST-CASES.md` AC-102 vs API-023, FLOW-001 | M | Cross-document | AC-102's *"no server duplicate claim"* contradicts API-023's server-generated intake key that FLOW-001 says does not exist | Align the three intake-key statements |
| F-69 | `01-TEST-CASES.md` AC-208 vs API-070/API-063 | M | Cross-document | AC-208 asserts a `matching-membership` payload field that no owner document defines | Define the field or remove the assertion |
| F-70 | `01-TEST-CASES.md` AC-101/AC-113 vs API-021/API-024 | M | Ambiguity | ERR-107 vs ERR-108 precedence undefined for a frozen quiz whose delete would empty it | Add the precedence to the conflict ladder |
| F-71 | `01-TEST-CASES.md` AC-221 / `US-403` vs BR-055, DEC-111 | L | Cross-document | *"browser back exits the list"* contradicts DEC-111's replaceState-only decision; two pagination defaults conflict | Align to DEC-111 |
| F-72 | `02-BACKLOG-USER-STORIES.md` US-703, US-103 · `01-INFRA-TOPOLOGY.md` L66-68 | L | Coverage gap | US-703 cites a *"<1 min"* target no owner file states; US-103 mixes actor flows; INFRA cites EDGE-030/033/051 for restart loss they do not cover | Add the target or drop the claim; re-scope US-103 |

### 5d.6 Decision-requiring findings and clarification questions (Q1..Q12)

Twelve findings cannot be resolved without a user decision. Each is answered
in §5e; the English question and options are kept here for implementers.

| # | Question (English record) | Anchors |
|---|---|---|
| Q1 | The status layer is contradictory and S1, S2b, S4-residual, S4a, S3, S5, S6 are open in one file and decided in another. Was the 2026-10-14 resolution set final, or were some re-opened? | DF-01..DF-06, F-16..F-24 |
| Q2 | Can the seeded teacher create further teacher accounts? PRD yes; API-010/RBAC/DEC-109 no. | F-01, F-30, F-46 |
| Q3 | On deactivate, what happens to live rows when DEC-092 auto-closes the target's quizzes in the same transaction? | F-02 |
| Q4 | Does a 60 s-stale occupant stay ever-present? API-036 implies no; STATE-007/N3 yes. | F-03 |
| Q5 | Which pages emit the `graded` SSE event? `late-flow page` is undefined. | F-04, F-23 |
| Q6 | Does API-039b survive as a separate operation with no asserting AC? | F-05 |
| Q7 | What is the single stale-spinner threshold (45 s vs 60 s)? | F-15, F-18, DF-03 |
| Q8 | Is the image size limit 10 MB or 10 MiB? | F-28 |
| Q9 | Does the essay-only acknowledgement checkbox exist, with storage/transport/UI? | F-14 |
| Q10 | Which names are canonical: `attempt_deadline_utc`/`paused_at` vs ERD's `deadline_utc`/`session_paused_at`? | F-27, F-33 |
| Q11 | Are the DevOps deferrals deliberate or missing (SLO/SLI, alert thresholds, TLS, CORS/CSP, backup/RTO/RPO, image scan, migrate runner, ports, boot-failure, NTP)? | F-06..F-11, F-26, F-53..F-66 |
| Q12 | Where does the authoritative token/design contract live, and must error rows carry a status and logging action? | F-37, F-41..F-43 |

## 5e. Decision Answers (user decisions, 2026-10-14 — recorded per finding ID)

Interview conducted in non-technical Indonesian with the user; each answer is
stored here in English, per finding ID, immediately upon receipt. A blank
Answer means not yet asked. **The English record below is the binding
implementation input.**

| Finding | Question (English record) | Options presented (Indonesian) | Answer given (English) | Status |
|---|---|---|---|---|
| Q1 | Was the 2026-10-14 resolution set final, or were some re-opened? | (a) semua keputusan sudah final, tulisan "masih terbuka" yang perlu diperbaiki; (b) sebagian memang masih terbuka; (c) dibuat satu daftar status utama | **No item remains open** (user, 2026-10-14, superseding the 2026-10-14 answer above): every audit item now has a decision, so the §5 statement "Open decisions (2026-10-14, Q1): none." is authoritative and no open item is carried forward. Files that still mark an item OPEN are stale and must be rewritten to CLOSED with the decided text. | RECORDED |
| Q2 | Can the seeded teacher create further teacher accounts? | (a) guru hanya boleh membuat akun murid; (b) guru boleh membuat akun guru; (c) hanya guru pertama yang boleh membuat akun guru | **Yes — a teacher may create teacher accounts** (user, 2026-10-14). Teacher-creates-teacher is permitted. The contrary sources (DEC-109 "create = students only", API-010 title "(create student only)", RBAC teacher-to-teacher create → 403, FLOW-006 blanket 403) must be widened to allow a teacher to create teacher accounts. PRD §3 ("can then create further teachers/students") is the wording to follow; API-010's parenthetical claiming a self-registration route for teachers must be removed because API-003 fixes role=student on register. | RECORDED |
| Q3 | On deactivate, what happens to live rows when quizzes auto-close in the same transaction? | (a) periksa dulu (409), lalu jawaban ditutup otomatis, baru quiz ditutup; (b) hapus skenario "jawaban ditutup otomatis"; (c) tentukan urutan transaksi lengkap | **Block (reject) the deactivation if a quiz is still open** (user, 2026-10-14): "tolak atau guard nonaktif jika masih terdapat quiz terbuka" — the deactivate action is rejected/disabled whenever an open quiz exists. This resolves the precedence question in favour of the guard: the force-submit clause for live rows is not the operative behaviour, so API-012/RBAC §5 must state that deactivate is blocked (not force-submitting) while an open quiz exists, consistent with the deactivate guard BR-051/DEC-081. | RECORDED |
| Q4 | Does a 60 s-stale occupant stay ever-present? | (a) tetap dihitung hadir dan jawaban tetap dikumpulkan; (b) hapus aturan "tidak dihitung hadir" dari kontrak API; (c) pertimbangkan ulang keputusan | **The occupant stays present** (user, 2026-10-14). A >60 s-stale occupant remains ever-present and their answers are auto-submitted on stop; only `last_seen IS NULL` rows are no-show. STATE-007 and the resolved N3 decision are authoritative; API-036's "60 s stale presence → server marks the occupant never-present (DEC-088)" must be corrected. | RECORDED |
| Q5 | Which pages emit the `graded` SSE event? | (a) halaman review dan halaman hasil; (b) hanya halaman review; (c) hapus notifikasi langsung | **Only the final result page after submit** (user, 2026-10-14): "halaman hasil akhir setelah submit". The live `graded` event targets viewers on the final result page (`/attempts/:id/result`) only — not the review page and not a card surface. FLOW-002 step 7 and FLOW-005 step 3 must enumerate that single page instead of the undefined "review/late-flow page". | RECORDED |
| Q6 | Does API-039b survive as a separate operation? | (a) tetap dengan tambahan uji; (b) hapus dan pakai daftar rekap; (c) tetap dan perbarui indeks | **Keep the operation and update the index** (user, 2026-10-14). API-039b stays as the student attempt-history operation; `00-INDEX.md` §2 must advertise it (currently omits it), and an asserting AC must be added for requireStudent 403, R1 `student_id=self`, attempt_token rejection and the submitted-only predicate. | RECORDED |
| Q7 | What is the single stale-spinner threshold? | (a) satu angka saja (45 s atau 60 s); (b) dua angka dengan peran berbeda jelas; (c) hitung dari irama heartbeat | **One number only** (user, 2026-10-14). A single threshold applies to both the "stale/slow" signal and the spinner clean-up; the second number must be deleted from `02-COMPONENT-STATES.md` C-006 and from `00-INDEX.md` §5c. Recommended single value: 60 s (aligned with the heartbeat staleness rule and the decided S4-residual text); to be applied in the cleanup phase. | RECORDED |
| Q8 | Is the image size limit 10 MB or 10 MiB? | (a) 10 MiB (10485760 byte); (b) 10 MB (10000000 byte); (c) tulis jumlah byte pasti sekali saja | **10 MiB = 10485760 bytes** (user, 2026-10-14). BR-010/F-004 keep MiB; PRD §4.1 and AC-105 must be corrected to 10 MiB (10485760 bytes). | RECORDED |
| Q9 | Does the essay-only acknowledgement checkbox exist? | (a) ya, tambahkan penyimpanan, kolom form, dan tampilan; (b) hitung otomatis dari daftar soal; (c) hapus izin tersebut | **No separate essay rule is needed** (user, 2026-10-14): "sepertinya tidak perlu aturan terpisah untuk essai, karena essai tidak ada kunci jawaban" — the essay-only acknowledgement checkbox is removed. An essay question has no answer key, so it cannot satisfy or block a "gradable question" gate through a checkbox; BR-005's "at least one non-essay question OR teacher acknowledges essay-only (explicit checkbox)" must be reworded, and FLOW-001 step 5's "explicit essay-only checkbox acknowledged" must be deleted. No storage, transport, or UI artifact is created. | RECORDED |
| Q10 | Which deadline/pause column names are canonical? | (a) ikuti nama pada skema; (b) ganti nama pada skema; (c) jadi nama alternatif yang ditulis di skema | **Rename in the schema** (user, 2026-10-14): the canonical column names are `attempt_deadline_utc` and `paused_at`. `03-ERD-DATA-SCHEMA.md` must rename `deadline_utc` → `attempt_deadline_utc` and `session_paused_at` → `paused_at`, and all other files (PRD, FLOWS, RULES, OPENAPI, DEC-130) that already use the business names become correct; BR-012/BR-013 are the reference wording. | RECORDED |
| Q11 | Are the DevOps deferrals deliberate or missing? | (a) sengaja ditunda, kumpulkan satu daftar penundaan; (b) harus ditentukan sekarang; (c) campuran — satu per satu | **Deliberately deferred to the operator at deploy time** (user, 2026-10-14). Alert thresholds, TLS/version specifics, RTO/RPO, hardware floor, deploy-access method and reviewer count remain operator-owned and are NOT fixed in the spec. The spec must state this as one consolidated deferral register (DEC-052/053/060/061/062/071 family) rather than implying values exist; every "required" claim that depends on a deferred value must be softened to an operator-runbook responsibility. | RECORDED |
| Q12 | Where does the authoritative token/design contract live? | (a) salin ke berkas spesifikasi dan tandai DESIGN.md tidak otoritatif; (b) DESIGN.md jadi acuan utama; (c) UI-007 jadi acuan utama | **Free choice — either source may be authoritative** (user, 2026-10-14): "bebas, acuan bisa dari pola design.md atau design system di spek". The cleanup must pick one authoritative token source and state it explicitly in `01-DESIGN-SYSTEM.md` (recommendation: adopt the DESIGN.md token table into the spec, choose the `metric-lg` point, and mark DESIGN.md non-authoritative), so AC-215 has exact values to assert. | RECORDED |

### 5e-bis. Second-round clarification answers (N-items, 2026-10-14)

Multi-level audit round 2 (2 orchestrators + 5 children = 7 agents) produced 36
clarification-requiring findings (N-A1..N-C9). Every item below was answered by the
user directly and is applied to the owner files. No item remains open.

| Finding | Question (short) | Answer given (user, 2026-10-14) | Owner files to align |
|---|---|---|---|
| **N-A1** | Which trigger does DEC-125 cover? | **Both triggers** — the countdown freezes on dead-stream (connection loss) AND on matching pause, plus the F-002 banner; timers stay frozen server-side. | 01-PRD.md, 02-FEATURE-FLOWS.md (FLOW-002 step 5 + FLOW-009), 01-ERROR-MATRIX.md, 02-FALLBACK-STRATEGY.md |
| **N-A2** | `leave` on a `running` matching session: error or 200 echo? | **Error** — rejected with a message; it is not a 200 echo. DEC-126's "never an error" applies to verbs issued while the session is `waiting`. | 04-STATE-MACHINES.md (STATE-002), API-040b, EDGE-072, AC-231 |
| **N-A3** | Which item is still open after Q1? | **None** — every audit item now has a decision. §5's "Open decisions: none." is authoritative and no open item is carried forward. | 00-INDEX.md §5/§5b/§5c |
| **N-A4** | Is S1..S6 still open, or already CLOSED? | **All S1..S6 are CLOSED 2026-10-14.** The "S1..S6 open (block FINAL)" line is a stale carry-over and is superseded. | 00-INDEX.md §7, 01-TEST-CASES.md §M |
| **N-A5** | Deactivate with an open quiz: auto-close or 409? | **Reject with 409 ERR-115** — no quiz is closed automatically by deactivation. The DEC-092 auto-close clause is superseded; in-flight attempts continue normally. | 00-INDEX.md (DEC-092), 03-BUSINESS-RULES.md (BR-051/052), 02-OPENAPI-SPEC.md (API-012) |
| **N-B1** | Does a hidden quiz's code still work? | **The code dies immediately** when the quiz is hidden, same as BR-040's no-enumeration rule. | 02-OPENAPI-SPEC.md (API-050) |
| **N-A7** | Stale-spinner threshold: 45 s or 60 s? | **One number, 60 s.** Delete the second number everywhere. | 02-COMPONENT-STATES.md (C-006/C-010), 00-INDEX.md §5c |
| **N-A8** | Is the S4a heartbeat payload decided or still open? | **Already decided** — delete the "remains open and is UI-owned" block; S4a CLOSED 2026-10-14 stands with the `{server_now_utc}` payload every 30 s. | 02-COMPONENT-STATES.md |
| **N-A9** | How many auditors ran, and under which partition? | **2 orchestrators + 5 children = 7 agents**, all 21 files covered, synchronized via `wait`. The 5-auditor and 10-auditor records are separate earlier runs, not the source of this register. | 00-INDEX.md §5c and §7 provenance records |
| **N-A10** | Is the register 72 or 74 findings? | **72 is the correct count.** The breakdown was wrong; recount per category so the seven rows sum to 72. | 00-INDEX.md §5d.2 |
| **N-A11** | Does DEC-109 permit teacher-creates-teacher? | **Yes, teachers may create teacher accounts.** The §7 note "narrowed to create-students-only" records a superseded intermediate state. | 00-INDEX.md §5/§7, 04-RBAC-MATRIX.md, 02-OPENAPI-SPEC.md (API-010) |
| **N-A12** | History: API-062 or separate API-039b? | **Separate API-039b operation**, advertised in §2 and asserted by AC-233; API-062 stays the teacher recap list. N10's closure is withdrawn. | 00-INDEX.md §5c (N10), 02-OPENAPI-SPEC.md |
| **N-A13** | AC range upper bound: AC-232 or AC-233? | **AC-233 is inside CHG-008's accepted scope**; the audit-addition range ends at AC-233. | 00-INDEX.md §5d/§5d.7, 03-BEFORE-AFTER-DELTA.md |
| **N-A14** | jQuery carrier label: R7 correct or mislabelled? | **R7 is correct** as DEC-114 and §2 state it. The register row calling it a defect is superseded. | 00-INDEX.md §5d register |
| **N-A15** | Browser back: exits list or restores query state? | **Back exits the list** (replaceState-only, no history-restore). The R9 wording "back/forward restores" is superseded. | 00-INDEX.md revision R9, BR-055, DEC-111 |
| **N-B5** | Is the suite 20 or 21 files? | **20 spec files + 1 index = 21 entries on disk.** "All 20 files re-read" counts spec documents; the tree lists 21 entries including the index. | 00-INDEX.md §2/§5d.1/§7 |
| **N-B6** | What scope is the "engineer" role? | **No separate engineer role.** Teacher and student only; the engineer scope equals the primary developer. Withdraw the ARCH claim that an engineer scope was named; overload behaviour is owned by the fallback strategy file. | 01-ARCHITECTURE-OVERVIEW.md, 02-FALLBACK-STRATEGY.md |
| **N-B7** | What storage marks a no-show row? | **Keep `registered` status with no extra marker.** Never-present rows stay `state=0` and receive no additional column or audit mark. | 03-ERD-DATA-SCHEMA.md |
| **N-C1** | Which AC asserts the ERR-116 restart leg? | **AC-114.** The S3 reference to AC-112 is wrong. | 01-TEST-CASES.md §M |
| **N-C5** | Are letter-suffixed IDs sanctioned? | **Yes, the suffix is part of the ID** (API-039b, API-040b, API-03A, AC-126A/B, AC-134B, AC-155B/C, Q1..Q12). Declare the convention in §3. | 00-INDEX.md §3 |
| **N-C6** | jQuery build size: 78.749 B or 78,749 B? | **78,749 bytes** — the dot is a thousands separator, not a decimal point. | 01-PRD.md (DEC-080), 00-INDEX.md DEC-080 |
| **N-C4** | DELTA acceptance date: 10-12 or 10-13? | **Follow the content: 2026-10-13.** | 03-BEFORE-AFTER-DELTA.md, 00-INDEX.md §5d.7 |
| **N-C9** | How to verify the status list is consistent? | **Re-check after all fixes are applied**, before declaring FINAL. | 00-INDEX.md §5/§5b/§5c |


### 5d.7 Cleanup execution record (2026-10-14)

Executed in five phases after the §5e answers were collected. Decisions came
only from §5e; no finding was closed by deleting its row.

| Phase | Scope | Findings closed | Files touched |
|---|---|---|---|
| 1 | Open-vs-closed status layer | DF-01..DF-08, F-16..F-24 | 00-INDEX, 01-TEST-CASES, 04-STATE-MACHINES, 02-FEATURE-FLOWS, 02-OPENAPI (API-000), 03-EDGE-CASE-PLAN |
| 2 | HIGH behavioural contradictions | F-01..F-15 (DevOps items left to phase 4) | 01-PRD, 02-FEATURE-FLOWS, 03-BUSINESS-RULES, 02-OPENAPI (API-010/012/023/036/039b), 04-RBAC-MATRIX, 01-ERROR-MATRIX, 01-TEST-CASES (AC-105/226, new AC-233) |
| 3 | MEDIUM/LOW | F-27..F-72 | PRD, ARCH-OVERVIEW, ERD, RBAC, FALLBACK, EDGE-CASE, DESIGN-SYSTEM, COMPONENT-STATES, PROTOTYPE-INTERACTION, TEST-CASES, STORIES, BEFORE-AFTER-DELTA |
| 4 | DevOps deferral honesty | F-06..F-11, F-25, F-26, F-53..F-66 | the three 05-DEVOPS-AND-INFRA files |
| 5 | Residue repair (markers + Q10 naming) | F-15/F-18/F-19 finish, Q10 stragglers, BR-007 S2b | DESIGN-SYSTEM, PROTOTYPE-INTERACTION, COMPONENT-STATES, PRD, FEATURE-FLOWS, BUSINESS-RULES, TEST-CASES, INFRA, LOGGING-OBSERVABILITY, BEFORE-AFTER-DELTA |

Verified after the final phase by grep across `docs/spec`: no live occurrence of
`DECISION REQUIRED`, `session_paused_at`, `remains OPEN`, or the bare
`deadline_utc` name survives outside this §5d register; `00-INDEX.md` §5 now
records **no open decisions (2026-10-14, Q1)** and one consistent status list
across §5, §5b, §5c and §9. The §5d register's quoted evidence is deliberately
retained as the historical audit record.

## 5b. Audit Open Questions (10-auditor parallel audit, 2026-10-14) — all resolved; nothing here remains open

Method: 10 parallel scout auditors (2 solo: 02-OPENAPI-SPEC.md, 01-TEST-CASES.md;
5 pairs: INDEX+DELTA, PRD+STORIES, FLOWS+STATES, RULES+OVERVIEW, SCHEMA+RBAC;
3 triples: resilience, frontend, devops), synchronized via wait; PRD+STORIES
agent failed to submit (empty yield ×2) and was audited by the main agent via
read+grep instead — all 21 files covered. Findings consolidated below; S1..S6
wording reconstructed verbatim from §5 above and owner-file markers (never
invented).

**Status update 2026-10-14 (consolidated audit, updated with user decisions):** S1, S2a, S2b, S3, S4a, S4b, S5, S6 and S4-residual are now CLOSED per owner-file resolutions and user decisions recorded in §5, §5b and §5c above. **No genuinely open questions remain** (Q1, 2026-10-14): every audit item below whose decision is in fact final is marked CLOSED with its decision text, and none of S1..S6 is still open. All N-items (N1..N18) are resolved.

### S1 — ERR-112 Retry-After (CLOSED 2026-10-14)
Question: do ERR-112 queue-full 429 rejections carry a `Retry-After` hint, and
does the client back off on it or show text only? Resolution: `Retry-After: 1`
on 429 ERR-112 only (time-based retry; 409s are conflicts, not time-based, so
no header there); asserted in AC-105. See BR-051 §S1, API-023, ARCH §4.

### S2 — Page mapping + closed-view layout (S2a CLOSED, S2b CLOSED 2026-10-14)
Question: explicit `?page=abc/9999/>200` mapping + closed-view layout for AC-220
determinism? Resolution: S2a — invalid `?page` clamped to page-1/last-page,
never 4xx (DEC-100, API-000, BR-055, EDGE-063, AC-220). S2b — CLOSED per user
decision 2026-10-14: quiz yang ditutup tidak bisa dibuka. Murid yang mengklik
menerima notifikasi (sudah-ditutup). Tidak ada tombol join/register/start.
Layout: halaman 200 dengan pesan "sudah ditutup" dan tidak ada aksi lanjut.

### S3 — ERR-116 restart-reset (CLOSED 2026-10-14)
Question: accept in-memory loss of the 10 s same-verb guard on restart (409
still guards) or persist it? Resolution: accept in-memory loss — the 10 s
same-verb guard is in-memory per quiz and does NOT survive process restart;
409 ERR-300 state-conflict guards persist and always apply. See STATE-002,
API-043, F-007, **AC-114 restart leg** (the earlier AC-112 reference is corrected per user decision N-C1 2026-10-14).

### S4 — Heartbeat payload + submit-vanish (S4a/S4b/S4-residual CLOSED 2026-10-14)
Question: heartbeat payload shape + monitor submit-vanish event name?
Resolution: S4a — heartbeat payload `{server_now_utc}` (DECIDED FINAL, user decision
2026-10-14): a `heartbeat` event every 30 s carries the payload. S4b — submit-vanish:
answering row disappears from monitor on submit; no new SSE event name (API-044,
FLOW-007, C-006, AC-209). S4-residual — CLOSED per user decision 2026-10-14: stale
spinner dibersihkan setelah 60s stale timeout. UI timing: spinner menghilang
setelah 60s tanpa heartbeat baru. Single threshold (Q7, 2026-10-14): ONE number,
60 s — the stale/slow signal and the spinner clean-up both fire 60 s after the
last fresh heartbeat. There is no second 45 s threshold anywhere.

### S5 — Cancelled/revoked result card (CLOSED 2026-10-14)
Question: what does a `cancelled`/`revoked`-row result view render, plus no-show
receipt? Resolution: terminal-state card ("Percobaan dibatalkan/dicabut", no
score, no correct answers, printable); no receipt for never-present no-shows
(`last_seen IS NULL`). See API-039, FLOW-002 step 8, AC-206.

### S6 — CI trio (CLOSED 2026-10-14)
(a) Integration DB: MariaDB 12.3.x current GA service in CI; sqlite allowed for
local dev only with divergence runbook. (b) Deploy-fail: rollback blip (short
outage), not continued serving of previous version. (c) No waiver — DEC-001
gates stay enforced with no waiver path. See CICD §4, ARCH §6, AC-136.

### N1 — Recap/export `?status=` semantics (RESOLVED 2026-10-14)
`?status=` appears in the canonical key set (DEC-100/BR-055) and the export
contract (DEC-116/API-063 "same order as screen") and its value set is now
defined. Owner: API-062/063 + BR-055. **RESOLVED (DECIDED N1, 2026-10-14 — no
longer open):** `?status=` values are `submitted` (default; the only
recap/history-eligible state) and `all` (treated as `submitted`); unknown values
are clamped to `submitted` per DEC-100 (never 4xx). API-062 (screen) and API-063
(export) apply the identical predicate and ordering, so screen/export parity
holds by construction and AC-220/222 are deterministic with this definition.

### N2 — Finished-session verb contract (RESOLVED 2026-10-14)
AC-232 says finished-session verbs are "rejected with current state" without a
code; EDGE-073 routes submits via "normal state checks". Implementers diverge
(409 ERR-300 vs 200 echo vs 422). Owner: API-043 + STATE-002.
**RESOLVED (Q1, 2026-10-14 — no longer open):** on a `finished` quiz, control
verbs (`start`/`pause`/`resume`/`stop`/`finish`) → 409 ERR-300 with the current
state (no state change); submit (`POST /attempts/:id/submit`) → 200 receipt
echo if the attempt was already submitted (DEC-086, idempotent), else the normal
guard applies. Recorded in API-043 (DECIDED N2) and asserted by AC-232.

### N3 — Presence-stale occupant on teacher stop (RESOLVED 2026-10-14)
AC-153 covers refresh-within-60 s, AC-154 covers present-never-returns, and the
>60 s-stale occupant at stop is now decided. Owner: STATE-007 + AC-153/154/208.
**RESOLVED (Q4, 2026-10-14 — no longer open):** a >60 s-stale occupant at stop
is STILL ever-present and auto-submits from last saved answers exactly like any
present occupant; ONLY `last_seen IS NULL` rows are no-shows. Recorded as the
presence-stale leg in STATE-007 (DECIDED per N3) and API-033/API-036 wording.

### N4 — Anon leave representation (RESOLVED 2026-10-14)
API-040b/EDGE-071/AC-229 define leave as a row flip (`connected=0`), while anon
pre-start presence is memory-only (DEC-096) — no row exists to flip.
Owner: API-040b + RBAC §1. **RESOLVED (2026-10-14 — no longer open):** account
occupants flip the row (`connected=0`, `last_seen` kept — DEC-121); anon leave
clears the in-memory intent set for that `taker_label`; the `occupant_left`
broadcast is identical for both. Recorded in API-040b (DECIDED N4), RBAC §1, and
AC-229.

### N5 — Stuck answering rows on timer-less quizzes (RESOLVED 2026-10-14)
Timer-less (`timer_mode=0`) answering rows never auto-submit (no deadline, no
sweep per ERD §9) and permanently 409/410-block their taker_label under
DEC-106/DEC-091. **RESOLVED per user decision 2026-10-14:** ditinggalkan apa
adanya. Guru dapat langsung menutup quiz untuk membersihkan baris yang terkunci.
Tidak ada sweep otomatis. Owner: ERD §5/§9 + API-033.

### N6 — Timer during outage: freeze vs counting (RESOLVED 2026-10-14)
F-002/AC-153/C-002 say the countdown FREEZES (DEC-125); the error-matrix footer
says it "keeps counting". **RESOLVED:** rewrite the footer to DEC-125 freeze
(matrix is the outlier). Owner: 01-ERROR-MATRIX.md footer + F-001 wording.
Applied in ERROR-MATRIX.md §Audit findings applied (N6).

### N7 — healthz vs readyz deploy gate (RESOLVED 2026-10-14)
INFRA §5 gates deploys on `/healthz` (503 until migrated) while observability
defines `/healthz`=liveness and `/readyz`=migrated+DB+writable gate; CICD
smoke/deploy follow INFRA. Owner: INFRA §5 + CICD + observability §3.
Recommendation: adopt observability as contract (`/readyz` = deploy/proxy/
smoke gate); rewrite INFRA §5 + CICD rows.

### N8 — PRD register drift (RESOLVED 2026-10-14)
**RESOLVED per user decision 2026-10-14:** PRD is authoritative. INDEX §5b and
§5c are aligned to PRD. PRD actor table: guru dapat membuat guru baru
(tambahkan ke dalam daftar guru yang ada). DEC-080/108/114/118 mengikuti
kata-kata PRD. Owner: 01-PRD.md §7 + 00-INDEX.md §5b.

### N9 — Anon auth after 24 h TTL (RESOLVED 2026-10-14)
DEC-117 says answering/result use row membership, but API-000's token-expiry
rule is unqualified — an anon mid-attempt at hour 24 is both authorized and
rejected. **RESOLVED per user decision 2026-10-14 (M4):** TTL 24h adalah
*hanya entry gate* — token expiry tidak eject in-flight. Membership wins
in-flight; TTL gates entry only. Asserted in AC-227. Owner: API-000/API-051.

### N10 — History endpoint gap (RESOLVED 2026-10-14, SUPERSEDED by Q6)
BR-055 + the spec closing note require a history list under the query contract,
but no API-### operation defines student attempt history (path, guards, keys,
errors) — and its relation to "no history endpoints accept attempt_token"
(API-051) is unreconciled. **RESOLVED 2026-10-14:** history list is served via the
recap endpoint (API-062) with `?status=` filter; no separate history operation.
**SUPERSEDED by user decision Q6 (2026-10-14):** student attempt history is served
by the SEPARATE **API-039b** operation (requireStudent, R1 `student_id=self`,
attempt_token rejected, submitted-only predicate). API-039b is advertised in
`00-INDEX.md` §2 and asserted by AC-233; API-062 remains the teacher recap list.
Owner: 02-OPENAPI-SPEC.md.

### N11 — Close-409 without an ERR code (RESOLVED 2026-10-14)
Close/hide-while-running-or-paused 409 (EDGE-055C/AC-155C/DEC-093) has status +
exact text but no ERR-### row; AC-155C can only assert text. **RESOLVED:**
recorded as explicit text-only exception in ERROR-MATRIX.md §ERR-3xx text-only
exception (N11). Owner: 01-ERROR-MATRIX.md.

### N12 — Banner-clear vs no-polling (RESOLVED 2026-10-14)
F-001 recovery cites "3 consecutive probes" defined nowhere; a client banner
cannot clear on server probes without the polling DEC-102 forbids.
**RESOLVED per user decision 2026-10-14:** banner dibersihkan hanya melalui
user refresh. Tidak ada polling (DEC-102), tidak ada auto-reconnect.
Owner: F-001 + devops gates.

### N13–N18 — Non-blocking polish (RESOLVED 2026-10-14; each recorded in its owner file)
N13 (`graded` viewer classes): RESOLVED — the "late-flow page" is now enumerated
explicitly as `/attempts/:id/result` (the final result page after submit) in
FLOW-002 step 7 and FLOW-005 step 3; the live graded event targets only viewers
with an open stream on that page — not the review page (Q5, 2026-10-14).
N14: RESOLVED — BR-011 is reworded to server-now + timer with DEC-130
deadline-only (no `question_started_at` column).
N15: RESOLVED — BR-051 carries the DEC-129 teacher-owns-quizzes self-delete
qualifier (both roles may self-delete; teacher self-delete BLOCKED while owning
ANY quizzes). N16: RESOLVED — student route identity aligned to `GET /student`
everywhere; `/student/quizzes` retired (API-030).
N17: RESOLVED — API-021 editable list includes `questions` (BR-006/DEC-103).
N18: RESOLVED — recorded as the DOM-text assertion exception in
01-ERROR-MATRIX.md §Invariant note (ERR-502).
Plus register hygiene found by the
audit: S-markers missing where owned (S1→ERR-112/F-004, S3→ERR-116/F-007,
S4→F-002, S5→ERR-208/EDGE-02A, S6→F-003), stale DELTA acceptance (AC-101..219
vs CHG-008, CHG-008 without acceptance clause, 2026-10-12 stamp over 10-13
material), backlog "S-items point to §M" claim (file has none),
R7 label on the jQuery carrier — **RESOLVED 2026-10-14 (user decision): the R7 label
is CORRECT as DEC-114 and §2 state it; this row recorded an audit-snapshot defect,
not the contract, and is superseded by DEC-114/§2), CSS allow-list
count vs UI-001 single file (enumerate filenames, not a count), ERR-116 audit
mislabel + unbounded audit rows, audit row schema missing level, closed-view
shell/alert-copy/ERR-204/leave-failure render gaps, AC-116 no-show exception
leg, AC-112 waiting-scope precondition, AC-133 notice string, "soal valid" vs
image-failed gate, ERR-107/108 precedence, monitor-close row behavior,
finish-vs-abort guard split, finished-surface rendering, waiting-SSE diagram
step, closed+finished auto-submit path, `room_closed` flow step, next-run
surface after finished-permanent, FLOW-002/009 diagram failure branches, anon
intent TTL sizing, restore-drill gate ownership.

Assumptions: A1 (SSE-friendly network; dead streams yield notice + refresh, never polling — DEC-102), A2 (tens of
concurrency; caps in ARCH-006). Unknowns: none hidden — the audit decision items S1..S6 in
§5b are all CLOSED as of 2026-10-14 (Q1: no genuinely open question remains), and audit findings
N1..N18 are resolved. Non-applicable (with reasons): OpenAPI YAML doc, multi-tenancy,
orchestrator/replicas, queues/circuit-breakers, offline mode, native apps,
SSO/OTP, automated essay scoring, chat in monitor, audit-log UI.

## 6. Exploration status

Mode A greenfield: workspace verified empty (`read` listing 2026-10-11);
five-agent exploration gate NOT APPLICABLE (no implementation/flow/spec to
inspect — dispatching explorers over an empty directory would fabricate
findings). Interview layers §5.2 covered progressively via `ask` (context →
shape → tech → architecture → functional → rules → flows → data →
resilience → verification) across 5 rounds; answers recorded verbatim in
per-file provenance notes. Validation: main-agent cross-check of answer
consistency (e.g. token-mode persistence model reconciled across rounds).

Modes C/D audit-driven refinement (2026-10-13): FIVE parallel explorers over
the written spec (Client/UI RipeElk, Server/API TropicalZebra, Data/Model
VisitingStingray, E2E/Integration NastyAntlion, Errors/Edge PreciousDingo) —
all completed, §4.4 barrier met (§7), findings cross-validated by the main
agent against the written files via grep + read (frozen-guard texts,
graded/SSE-close texts, intake-key absence, beacon/CSRF texts, grep-ban vs
Bootstrap usage, RBAC matrix cell). Interview: 28 decisions locked as
DEC-103..130 (ask tool, one focused question at a time with full
before/after wording; user custom inputs recorded: freeze hidden/show split,
beacon-constraint explanation, teacher scope correction self+students,
SSR-vs-CSR explanation); 6 items remain open as S1..S6 (§5).

## 7. Audit record

- Post-write reread: all 20 spec files re-read (2026-10-12), this index last.
  (Authoritative reading per user decision N-B5 2026-10-14: **20 spec files** —
  the numbered documents. `00-INDEX.md` is the register/entry point, giving
  21 entries on disk, so "all 20" and the 21-entry §2 tree are both correct.)
- Passes: A (Markdown/links — manual), B (semantic vs user answers), C
  (ambiguity sweep for "etc/properly/should" — none found), D (flow dead-ends
  traced FLOW-001..009), E (field/API/state/cross-checks), F (edge adversarial
  vs EDGE-001..062), G (Mermaid — MANUAL review, no parser available; syntax
  kept to flowchart/sequence/state/erDiagram basics), H (traceability §4).
- Corrections applied during writing: unified `visible`/`status` orthogonal
  model (was at risk of enum conflation), fixed anonymous persistence split
  (memory intent vs DB attempt), added idempotency + grace rules.
- Corrections applied during post-write audit (2026-10-12 re-read): fixed
  ERR-112 stray characters, F-004 typo fragment, infra §3 sentence damaged by
  edit (restored), `portinternal` typo, cheat-modal copy artifact,
  PRD index path, and all cross-directory relative links (verified by grep;
  only spec-root-relative path in §9 remains, explicitly annotated).
- Revision R2 (2026-10-12, user directive: fewer FK/ENUM/UNIQUE, denormalize,
  single-table hot reads): rewrote `02-ARCHITECTURE-AND-TECHNICAL/03-ERD-DATA-SCHEMA.md`
  (13→6 tables, 0 FK, 0 ENUM, 2 UNIQUE, JSON-embedded options/answers/grades/cheat/session/roster);
  aligned BR-001/002/003/006/007/008/021/032/040, FLOW-002/003, STATE-002/003/005,
  ARCH-001 ownership, API-031/032/033/040, AC-201 + new AC-122, and this index.
  Behaviour (BR/STATE/API/ERR/EDGE/AC observables) unchanged; only storage mechanics changed.
- Revision R4 Full-Bootstrap (2026-10-12, user directive: map ALL Bootstrap
  components fully + inline CSS only, supersedes R3 theming): rewrote all three
  `04-FRONTEND-UIUX/*` files (UI-001..005, full §4 catalog mapping with USED /
  NOT USED + reasons, closed inline-property list, R2 behavioural contracts
  unchanged); added AC-217/218 + CI UI-conformance stage; clarified vendored
  Bootstrap pair in ARCH-001 ownership, infra app-container row, index rows.
  Behaviour otherwise unchanged (AC-217/218 are ADDED gates).
- Re-verification after R4 (2026-10-12): re-read all three `04-FRONTEND-UIUX/*`
  files + AC-217/218 + CI UI-conformance stage + index rows; fixed utilities
  closed-list name (`borders` → catalog-exact `border`), completed examples
  NOT-USED list (carousel-demo/features/heroes), completed approach doc note,
  added section-index coverage note. Catalog-vs-§4 sweep: every
  BOOTSTRAP-V6-DOCS.md path maps to a §4 row (USED / NOT USED with reason /
  MECHANISM). No critical contradiction or undefined critical flow remains.
- Re-audit after R2 alignment (2026-10-12): re-read schema (§1–9), STATE-001/002/003/004/005,
  BR-001/008/040, FLOW-002/003, API-031/032/033/040, AC-201/122, ARCH ownership, observability
  redaction + runbook; fixed STATE-001 initial/entry (registered, same-row), STATE-003 diagram
  (`started` → `answering`), STATE-002 start side effect, STATE-004 codes wording, observability
  R1-label + runbook JSON wording. Final grep sweep: only intentional R1-history mentions remain
  (schema §2 mapping + revision notes). No critical contradiction or undefined critical flow remains.
- Revision R5 DEC-resolution (2026-10-12, all user DECs): coverage gates
  DEC-001 (85%/100%), XLSX API-063 + AC-219 (DEC-002), BR-053/054 auth
  (DEC-003/032), API-070 + US-601 + AC-214 formulas (DEC-004), custom timer
  1–86400 (DEC-005), zero grace (DEC-006), no duplication (DEC-010), blocked
  empty start (DEC-011), Fiber + Cloudflare edge + caching + workers +
  Excelize (DEC-020..024, ARCH-008/009),
  strict roles/ownership (DEC-030/031), Gotham + palette UI-006/007/008
  (DEC-040/041), button confirms (DEC-042); deferred DEC-050..062,
  indefinite logs + no audit UI + no rate limits (DEC-070..073).
- Re-audit after R5 (2026-10-12): defined all 23 previously-referenced-but-
  missing ACs (AC-108/110/111/113/114/115/123/124/125/130/131/132/133/134/137/
  141/144/152/153/154/155/156/162 — every EDGE now maps to a defined AC);
  fixed EDGE-005 column layout + stale BLOCKED footer; clarified
  `image_status` default-0 note; reconciled UI language (Indonesian product
  copy decided, DESIGN.md §12 superseded for UI); fixed AC-215 reference and
  index UI-008 row. No undefined references or open decisions remain.
- Final closure audit R5b (2026-10-12): closed every remaining orphan —
  added ERR-113/114/208 + wired to AC-155/107/115 and API-043/021/033,
  STATE-002, EDGE-007/015; fixed EDGE-021/022 mis-mapping to AC-126 (new);
  added AC-109/116/117/118/126/210 (uniqueness, cancel-window, seed, auth
  middleware, hide/close mid-attempt, essay queue); linked all ERR/BR/FLOW/
  STATE/F codes to an asserting AC (ERR-001..502, BR-001..054, FLOW-001..009,
  STATE-001..007, F-001..007 — verified by grep); decided all PROPOSED
  budgets (F-001 timeouts, F-003 batch, BR-032 debounce, runbook target);
  fixed stale DEC-051/AUDIT-§12/AC-range references; widened §4 traceability
  rows to the full asserting AC sets. Zero dangling IDs remain.
- Revision R6 jQuery-Bounded (2026-10-12, user directive: add client jQuery
  per `docs/JQUERY-DOCS.md`, approved option C — AJAX + DOM/events bounded):
  rewrote UI-002 (jQuery 4.0.0 vendored from OBSERVED
  `assets/jquery-4.0.0/jquery-4.0.0.min.js`, 78,749 bytes, full build —
  AJAX + DOM/events allow-lists, native-only SSE/beacon/animation, banned
  deprecated/plugin APIs); extended AC-217 (vendored `<script src>`
  allow-list + banned-API grep) + CI UI-conformance stage; wired ARCH-006/
  §4 carrier/§6 deploy, transport §0 + API-023/03A, C-001/shell notes,
  prototype §5 + headers, infra app row, PRD §7 + index rows (new DEC-080).
  Behaviour (BR/STATE/ERR/EDGE/AC observables) unchanged — carrier-only change.
- Re-audit after R6 (2026-10-12, skill §12 Pass A–H): reread all 12 touched
  files; Pass A fixed UI-002 self-damage + carrier wording; Pass B fixed
  ARCH-003 `POST/fetch` contradiction + runtime-diagram label + SSR trade-off
  wording; Pass E neutralized the generic `fetch/JSON` wording in error-matrix
  §0; Pass H verified DEC-080/UI-002 linkage across all 12 files + R6 headers
  (3/3) and swept zero stale `No jQuery`/raw-fetch claims (remaining `SSE +
  POST` strings are transport-pattern names, not fetch claims); honest AC-217
  grep design (method-call patterns only in JS/script blocks + reviewer
  disposition for native `bind`/`classList.toggle`, verified against the real
  bundle). Mermaid: manual review only (no parser — recorded, not claimed).
  No new orphan IDs; no open carrier decisions.
- Remaining non-critical issues: Mermaid automated validation unavailable
  (recorded, not claimed). No open DEC (all decided or deferred 2026-10-12,
  now incl. DEC-081..102); no pending palette or formula.
- Revision R10 no-poll-notice-refresh (2026-10-12, user directive: NO polling
  of any kind, no SSE auto-reconnect — dead stream yields notice + user-only
  refresh; Modes C/D over own spec, §4 five parallel explorers — Client,
  Server, Data/Metrics, E2E-Trace, Errors — all completed, barrier met,
  main-agent cross-validated): F-002 rewritten (retry NONE, `EventSource.close()`,
  persistent exact-wording notice, honest missed-update degradation, refresh =
  state GET, no `Last-Event-ID`/replay); STATE-007 lifecycle simplified to
  `connecting → open → closed` (no `reconnecting`, no ring buffer); ERR-502 →
  persistent notice + manual refresh; EDGE-035/053 + AC-135/153/208 rewritten;
  `?poll=`/`?format=` keys deleted from BR-055 (JSON is content-negotiated);
  API-044 poll variant deleted; C-005 `disconnected` + C-006 `stale` + C-010
  banner/toast + prototype SSE-drop line rewritten; DESIGN.md catalog rows
  (alert/spinner/toast) aligned; ARCH-003/004/007 wording + runtime diagram +
  disk label fixed; F-005 flush + F-007 straggler + FLOW edges + PRD A1 +
  RISK-002 + US-301 + delta + CI SSE wording + `sse_reconnects_total` →
  `sse_dead_total` (fallback + observability + runbook) fixed; PRD §7 + index
  rows (new DEC-102).
- Re-audit after R10 (2026-10-12, skill §12 Pass A–H over all 17 touched
  files): Pass A fixed ARCH runtime-diagram disk label (Volume →
  host bindmount); Pass E aligned navbar connection `badge` + schema
  `connected` cell wording with no-reconnect semantics; Pass F/H exposed and
  fixed one PRE-EXISTING contradiction the sweep surfaced (error-matrix
  footer "answers queue for retry" vs F-001/DEC-090 drop-with-notice —
  footer rewritten to the DEC-090 rule); stale-promise sweeps
  (`will reconnect`/`silent reconnect`/`queue for`/`reconnects resume`) +
  vague-language sweeps clean; DEC-102 linked in 17 files; dead endpoint
  `GET /attempts/:id/state` + metric `sse_reconnects_total` verified gone;
  R9 history note left intact (records what R9 did; R10 supersedes only the
  `?format/json&poll` keys). Mermaid: manual review only — STATE-007 holds
  no diagram (prose lifecycle), no diagram redrawn, none contradicted
  (recorded, not claimed). No open DEC (now incl. DEC-081..102).
- Revision R9 folder-search-pagination-query (2026-10-12, user directive:
  persistence via host bindmounts, realtime searchbar with debounce,
  URL query params for client-server, SSR pagination limit/25; Modes C/D):
  INFRA §1 named volumes → host bindmounts `data/` (DEC-101; container paths
  unchanged, `data/` gitignored/never in image; backup unit = both host dirs);
  new BR-055 list query contract (canonical `?q/page/per_page/window/class/
  status/date` + reserved `?format/json&poll` transport keys, DEC-100;
  SSR default `per_page=25` overridable to 200, JSON explicit ≤200, DEC-099;
  300 ms debounce on every search input via jQuery partial swap +
  `history.replaceState` **only — browser back exits the list and does NOT restore the
  previous query state, per DEC-111 (user decision 2026-10-14); the "back/forward
  restores" wording above is superseded**), no-JS GET fallback, DEC-098)
  wired through UI-002 (debounce carrier), API-000/010/022/030/060/062,
  C-008, prototype shells (quiz list/users/recap/history), schema §6
  (`LIMIT` = `per_page` ≤200), EDGE-063/064, AC-220/221/222 (§K), US-403,
  PRD §7 + index rows (new DEC-098/099/100/101).
- Re-audit after R9 (2026-10-12, skill §12 Pass A–H over all touched files):
  Pass C/F closed two gaps found mid-audit (fragment format via
  `X-Requested-With` + `jqXHR.abort()` in-flight rule — BR-055/AC-221/lifecycle
  aligned); stale-claim sweep fixed ARCH §6 + recovery + ARCH-006 volume
  wording to bindmounts and aligned the BR-055/EDGE-063 clamp example (5000);
  linkage verified both ways (BR-055, DEC-098/099/100/101, EDGE-063/064,
  AC-220/221/222, US-403 — 11 files); §K added to IN_REVIEW exit criteria;
  infra diagram manually reviewed (unique IDs, legal labels); vague-language
  sweep clean. Mermaid: manual review only (no parser — recorded, not
  claimed). No open DEC (now incl. DEC-081..101).
- Revision R8 six-parallel-audit + user decisions (2026-10-12, skill §4/§12:
  six parallel auditors — Writing, Semantic BC, Flow D, Data E, Errors F,
  Trace GH — all completed, barrier met, findings cross-validated by the main
  agent against the written files; display-truncation false positives
  (AuditorWriting T1/T2/T4 — UI-002/UI-008 tails, `closest`, NON-RENDERED)
  rejected by byte-exact grep ground truth; real defects fixed: ERR-111 guard
  widened to any live row, FLOW-003 `session_state {running}` event name,
  recap submitted-only scope (FLOW-007/API-062/063 + BR-014 anon-label
  canonical), BR-001 type-once (API-021/FLOW-001/AC-101 aligned),
  BR-011 `question_started_at` formula + ERR-205 in acceptance, cheat-txn 2 s
  server-time window + terminal-row echo (schema/API-03A/AC-142 fresh-eid
  fixture — closes P0-2), no-show revoke exception
  (API-032/STATE-003/RBAC), `image_status` + `grade_level` code-table rows,
  transport alias map, `last_cheat_at` derived wording, grade guards
  (API-061/schema/EDGE-034B + AC-134B race order + BR-054 check order),
  stop/resume race precedence (FLOW-009/EDGE-034/034B/AC-134), BR-006
  type-out (BR-001 owns immutability), ERR-300 essay-guard mapping, F-007
  guard + DB-down rule, C-002 key-scope + `expired` success framing, EDGE-005
  columns, suffix-case uppercase (EDGE-004B/034B/054B/055B/055C,
  AC-126A/126B/134B/155B/155C), UI-007 pipe-span + `exista` typo,
  conformance pipe-escape (UI-002/AC-217/CI), F-003 paused-skip + cadence +
  temp-protect, AC retags (101/102/103/113/115/116/117/122/131/136/153/156/
  203/208/214/219), seed-collision (AC-117), ERR-206 answer-only scope,
  no-rate-limit guard qualifier, card/register-guards, API-036 catalog,
  no-show monitor badge, `room_closed` catalog, mermaid counts factual
  (R5: 23 ACs; inventory: 13 diagrams), resume-state definitions, ERR-301/400
  trigger widening, C-004 expired framing, prototype §3 guard rows + §5
  framing, observability catalogue/runbook/US-703 stdout-only, EDGE-050/051
  DEC-091/096 alignment, ERR-103 volume mapping — then 8 user R8 decisions
  DEC-090..097 applied end-to-end: offline drop-with-notice (F-001/EDGE-030/
  AC-130/C-003 — no auto-flush, no answer-overflow), anon quota per name
  (BR-041/API-033/schema/FLOW-004/EDGE-050/AC-150), cascade + auto-close
  (BR-052/API-012/022/schema), waiting-abort + stop-first (STATE-002/004,
  API-022/031/043, EDGE-055B/C, AC-155B/C, `room_closed` event), list-direct
  close (API-022/DEC-094), intake key (API-023/AC-102), entered-anon
  memory-only (FLOW-004/EDGE-051/AC-151/156), ERR-116 stale-version refinement
  (API-043/STATE-002/F-007/AC-112/C-006/ERR-116 cell). Index: DEC-090..097
  rows, RISK-007/008, §4 traceability (134B/155B/155C/126A/126B legs).
  Re-audit Pass A–H after R8: all 20 files re-read; linkage verified both
  ways by grep (DEC-090..097, ERR-115/116, AC-126A/B/134B/155B/C,
  EDGE-055B/C); zero orphan IDs; zero stale `No jQuery`/raw-fetch claims.
  Final verification round (same session): all six auditor reports consumed to
  the last item — backlink pass (AC-101/109/113/156 + ERR-115/116 cells),
  DEC-tag convention (AC-104/105/106/161/208 + REQ legs), UI-007 pipe-span +
  `exista` typo, catalogue pipe-breaks (npm/examples/customize/utilities),
  conformance pipe-escape (UI-002/AC-217/CI), input-normalization rule
  (API-000), malformed-answer rule (API-035), CSRF-rotation propagation
  (BR-054), story-adverb cleanup (US-201/202/301/302/401/402), request_id +
  beacon-exception wording (§0), F-003 paused-skip, ERR-103 volume mapping,
  RISK-007/008, §4 rows widened (BR-030/031/050/051, FLOW-006, STATE-001/003/
  004/005/006/007, 134B/155B/155C legs). Vague-language sweep clean;
  stale-key-wording sweep clean. Residual: AC-134B asserts BR-054 ordering
  (no dedicated EDGE — by design, race-order rule); AC-109/154 headers omit
  redundant DEC tags already present in body (cosmetic, no behavior impact).
  Mermaid: manual review only (no parser — recorded, not claimed).
- Revision R7 audit-driven refinement (2026-10-12, skill Modes B/D over own
  spec as evidence: five parallel explorer reviews — client/UI, server/API,
  data-model, end-to-end, errors/edges — all completed; barrier met; findings
  cross-validated by the main agent against the written files, then 9 user
  case-study decisions DEC-081..089): applied deactivate guard (ERR-115,
  BR-051/STATE-006/API-012/RBAC/EDGE-028/AC-123 + closed-corner force-submit),
  live-delete guard (BR-052/API-022/schema/EDGE-029/AC-109), exhaustion
  auto-cancel (BR-009/API-031/033/schema/STATE-003/EDGE-015/AC-115),
  server-generated idempotency keys (API-000/033/schema/F-006/AC-110/131),
  10 s verb guard (ERR-116, API-043/STATE-002/AC-112), late-submit 200
  (BR-012/API-038/schema/AC-203 + ERR-206 scoped to answer saves), manual
  login (FLOW-006/API-003/US-103), no-show presence rule (BR-008/STATE-002/
  007/API-043/044/C-006/EDGE-054B/AC-208/154/156), delete-last maintain-gate
  (BR-005/API-024/EDGE-027/AC-101/109); plus objective explorer fixes: REQ-005
  narrowed to detectable events, FLOW-001 draft default, ARCH-001 vendor-note
  wording, Bootstrap alpha honesty (UI-001), TOKEN repair + lookup-deny-both,
  seed-collision abort, self-check owner/cadence, anon CSRF binding, anon
  start/enter branches, double-cancel echo, card/review field definitions,
  recap `state=2` predicate, no-show revoke exception, F-006 key
  survival/request-id/expiry, race precedence (FLOW-009), F-004 intake
  order/drain/timeouts/orphan-sweep, recap cheat columns + monitored reading,
  XLSX WIB/token vocabulary, ERR-500/F-006 split, ERR-502 INFO level,
  EDGE-012/045/053 wording, AC-104/113/153 mapping, AC-126A/126B split,
  SSE event catalog (§0 + API-036/041/042 + FLOW-008), edge footer + §4
  traceability widened. Re-audit Pass A–H: vague-language sweep clean
  (1 prose `etc.` in a quoted catalog label only), DEC/ERR/AC/EDGE linkage
  verified by grep (ERR-115/116, DEC-081..089, AC-126A/B all linked both
  ways), zero orphan IDs. Mermaid: manual review only (no parser — recorded,
  not claimed).
- Mermaid inventory: 13 diagrams (flows ×4 sequence/flowchart in FEATURE-FLOWS,
  states ×3 in STATE-MACHINES, architecture ×2 in ARCHITECTURE-OVERVIEW,
  schema erDiagram ×1, nav ×1 in PROTOTYPE-INTERACTION, infra ×1, cicd ×1)
  — all manually reviewed (no parser — recorded, not claimed).
- Revision R11 audit-driven refinement (2026-10-13, skill Modes C/D + §4 five
  parallel explorers — Client/UI, Server/API, Data/Model, E2E/Integration,
  Errors/Edge — all completed, barrier met, main-agent cross-validated via
  grep + read; interview 28 locked as DEC-103..130, 6 open as S1..S6):
  written via SIX parallel slices (Business, Arch, Resilience, Frontend,
  Devops, QA — one file-set per slice, no overlap), then independently
  re-audited by the main agent (§12 Pass A–H) BEFORE this index update.
  Pass A fixed slice damage: API-010 split sentence (create-student-only edit).
  Pass B/C fixed live stale claims the slices missed: API-062 recap
  `back/forward restores` → replaceState-only wording; `graded-live` toasts in
  the §4 catalog + C-010 → NO-claims per DEC-104 (two files). Pass B fixed one
  REAL scope defect: API-010 still allowed teacher-creates-teacher → narrowed
  to create-students-only per DEC-109 (RBAC matrix + PRD actors already
  self+students). Pass E fixed one REAL guard-order defect: AC-223 + EDGE-066
  said peer-deactivates-owner → 409, but DEC-109 demands 403 first → both
  rewritten 403-first with the 409 unreachable. Pass C vague sweep clean
  (no handle-properly class hits in the six touched areas). Pass D traced
  leave/closed-view/reopen-permanent end-to-end (endpoint + guard + event +
  shell + list-exclusion all present, no dead ends). Pass E verified 403
  before 409 order in API-012 and teacher-create narrowing. Pass F adversarial:
  peer-deactivate-live and teacher-creates-teacher probed and closed above.
  Pass G Mermaid: manual review only (STATE-002 diagram holds no
  leave/echo rows — prose table only; no diagram redrawn, none contradicted;
  recorded, not claimed). Pass H: every DEC-103..130 in ≥4 files (verified by
  count sweep); every EDGE-065..073 ↔ AC wired both ways (AC-223/224/226/227/
  228/229/231/232 in §L + AC-225/230 fragment/deadline legs); US-404..407 +
  CHG-008 present; exit criteria cite AC-223..233; S1..S6 markers present in
  owner files + §M block; zero orphan IDs. Remaining non-critical: Mermaid automated
  validation unavailable (recorded, not claimed). S1..S6 all CLOSED 2026-10-14 per the
  §5 closed S-items list and the §M section; the "S1..S6 open" note was a stale carry-over
  and is superseded by this record.
- 5-auditor and 10-auditor file audits (2026-10-14, earlier separate runs —
  superseded as the source of the consolidated list by the 2-orchestrator + 5-child
  = 7-agent audit recorded in §5c, which is the authoritative provenance for the
  findings register in §5d). Both consolidated their results into the then-new
  §5b Open Questions: S1..S6 re-verified from owner-file markers (S1 genuinely
  open with 409-scope drift; S2 overscoped — page mapping decided, layout +
  template open; S3 self-contradicted — answered yet marked open; S4 half open
  with dual API/UI ownership; S5 genuinely open; S6 trio genuinely open with
  S6b factually wrong as written) plus 12 new FINAL-blocking findings N1..N12
  and 6 non-blocking polish items N13..N18 with per-item owner files and
  recommendations. Only 00-INDEX.md was modified by this audit; all other spec
  files untouched (read-only auditors). Mermaid: no diagram changes; manual
  review only (recorded, not claimed).

## 8. Risk register

| ID | Risk | Mitigation in spec |
|----|------|--------------------|
| RISK-001 | Timer unfairness (clock, pause, restart) | Server authority + persisted deadlines + sweep (BR-013, F-003, AC-136) |
| RISK-002 | SSE blocked by school proxy | Persistent notice + user refresh + server-side guards (F-002, AC-135; no polling — DEC-102) |
| RISK-003 | Cheat false positives erode trust | Honest counting + documented limits (BR-032, EDGE-041/044/045) |
| RISK-004 | Scope creep (bank, SSO, CSV/PDF, charts) | Explicit out-of-scope + single-format XLSX gate (DEC-002); DESIGN.md §13 non-goals binding (UI-008) |
| RISK-005 | Essay bottleneck delays finals | Queue oldest-first + temporary scores (FLOW-005, BR-022) |
| RISK-006 | Single-host failure during exam | 503 honesty + read-only degradation (INFRA, F-001); backup/RTO are operator-runbook concerns (deferred DEC-050/052/053, not asserted) |
| RISK-007 | Anonymous quota spoof-trivial (`Budi` → `Budi2` new quota, DEC-091) | Documented limitation; teacher stops abuse by closing the quiz; duplicate open codes stay rejected (ERR-114) |
| RISK-008 | Offline-dropped answers need manual recovery (DEC-090: dropped with notice, no auto-flush) | Generic banner + saved-tick model (DEC-124): only post-200 tick guaranteed; no silent application of stale answers |
| RISK-009 | Peer-teacher writes and self-delete orphans (DEC-109/129: no admin role) | Teacher-to-teacher WRITE (edit/deactivate/delete another teacher) → 403; CREATE of teacher accounts is allowed per Q2 2026-10-14 (DEC-109 widened); teacher self-delete BLOCKED while owning ANY quizzes; guard order 403-before-409 (AC-223) |
| RISK-010 | Anonymous one-label concurrency (DEC-106: shared quota) | One-active guard: second concurrent same-label start → 409; quota exhausted → 410 ERR-208 |
| RISK-011 | Matching timer-mode confusion (DEC-119: per_question on teacher-paced quiz) | Guru-full: no auto-advance, deadlines recorded, teacher cues move, single monitor index |

## 9. Status lifecycle

Current: **DRAFT** — discovery complete, artifacts written and audited
internally, DEC-001..130 decided/deferred/superseded (DEC-080/095 superseded);
the audit decision items S1..S6 and audit findings N1..N18 are all resolved and
recorded in §5b/§5c (S1..S6 all CLOSED as of 2026-10-14; no genuinely open
question remains per Q1). Nothing in S1..S6 blocks the transition any longer.
Transitions to IN_REVIEW when the
user confirms the draft reads correctly; to FINAL only after explicit user
finalization with the AC-215 contrast check passed and audit §7 current.
BEFORE/AFTER delta accepted per
`06-QA-AND-MANAGEMENT/03-BEFORE-AFTER-DELTA.md` §3 (path relative to
`docs/spec/`).
