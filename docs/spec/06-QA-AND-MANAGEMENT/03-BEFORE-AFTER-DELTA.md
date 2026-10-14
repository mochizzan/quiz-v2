# Before / After Delta — quiz-v2

Status: DRAFT | Owner: 06-QA | Updated: 2026-10-14 (F-66 closed: CHG-008 AC range corrected to AC-101..219 + AC-223..233, with the acceptance clause naming the asserting cases; stale "S1..S6 open" tail replaced by the CLOSED reference; accepted date follows content = 2026-10-13 per user decision N-C4; **CHG-009 added 2026-10-14: ARCH-008 caching/gzip (ARCH §8) + AC-234..239 + US-704, and CHG-007 revised to ARCH-001..009/Cloudflare now that NGINX is removed per ARCH-009**)

## 0. Why this file exists (Mode A greenfield)

There is no prior implementation: no BEFORE code, no migrated data, no
compatibility surface. Per skill §8.21 this file is still REQUIRED to record
the change narrative explicitly rather than leaving it implicit: the BEFORE
is the user-reported manual practice, the AFTER is this specification. No
migration or rollback-of-data applies; rollback means "stop using the system
and return to manual practice" (no data to convert back).

## 1. Change table

| Change ID | Request/source | Verified BEFORE (user-reported, NOT code-observed) | AFTER (this spec) | Must remain unchanged |
|-----------|----------------|-----------------------------------------------------|-------------------|-----------------------|
| CHG-001 | User: manual quiz from books | Teacher writes/gives quizzes manually from books | Teacher authors in-app (FLOW-001, BR-001..006) | Teacher owns content; no cross-quiz bank imposed |
| CHG-002 | User: Google Forms + manual recap | Scores recapped manually from Forms | Auto-grading + instant on-screen recap + one-click XLSX export (BR-020/022, FLOW-005/007, API-062/063) | Essay stays human-graded 0–10; XLSX is the single export format (CSV/PDF stay out) |
| CHG-003 | User: need anti-cheating | No detection | Focus-loss counter + modal + teacher visibility (BR-032, FLOW-008) | No auto-submit on cheat; no lockout |
| CHG-004 | User: daily + recap ease | Manual daily overhead | Normal async flow + repeat attempts as duplicate rows (FLOW-002, BR-009) | Each attempt immutable |
| CHG-005 | User: controlled sync quiz | No sync mechanism | Matching waiting-room + start/pause/stop/finish via SSE (FLOW-003, STATE-002) | Room locks at start; stop auto-submits |
| CHG-006 | User: token/anon join | Accounts required everywhere | Code + name, ephemeral intent, persisted attempt (FLOW-004, BR-040/041) | No account created; no history page |
| CHG-007 | User: Go SSR + MariaDB 12 + Docker | Nothing deployed | Go Fiber monolith per ARCH-001..009 (incl. Excelize XLSX streaming), Cloudflare edge + Compose per infra doc | Single instance; no invented distribution |
| CHG-009 | User 2026-10-14: SSR heavy/slow — add caching for a fast, light exam-day surface (4 user rules preserved) | No caching specification existed anywhere; SSE/proxy/header behaviour inferred from ARCH only; `/assets/`, `/media/` and any TTL value absent from all 21 files; every HTML page auth-scoped but never marked `no-store` | ARCH-008 + ARCH §8: Go Fiber owns all headers; Class A `/static/…` = `public, max-age=86400` (Rule 1's 1-year branch withheld — no build step, filenames version-stable); Class B `/uploads/:ref` = `private, max-age=86400, must-revalidate` + `ETag`/`Last-Modified` + `304` (Rule 3, new URL per replace); Class C all SSR/SSE/export/health = `no-store` (Rule 4, domain-wide Cache Everything PROHIBITED); Rule 2 `/media/` recorded as reserved-but-NOT-APPLICABLE; gzip added (§8.4); AC-234..239 + US-704 added | Must remain unchanged: `no cache tier in MVP` (ARCH-002) and "No derived caches" (ARCH §5) still binding — transport headers only; uploads stay outside web root behind the signed handler; no CDN cache tier relied upon; images still access-checked per request |
| CHG-008 | Audit: Modes C/D five-explorer audit, interview 28 locked | Pre-audit spec left DEC-103..130 implicit or untested | Audit-driven refinement (DEC-103..130 covered by AC-102/113/210/223..232, AC-130/140..145/202/208..213/217/220..222, US-403..407): before → after per cluster: intake echo promise → client-side double-submit prevention + manual duplicate delete (DEC-107); freeze blanket rule → open+hidden+registered-only editable 200, open+show+registered 422 ERR-107, answering/submitted anywhere 422 (DEC-103); live grade swap → submit closes stream, graded visible after refresh/reopen (DEC-104); deactivate without live-session guard → self/other deactivate with owned running/paused 409 ERR-115, session unchanged (DEC-105); anon repeat-start silent → same-label second start while active 409, quota exhausted 410 ERR-208 (DEC-106); cheat POST CSRF 403 → pagehide beacon 200 without CSRF (DEC-108); teacher self-only mutate → teacher-to-teacher mutate 403 (DEC-109) + both-can-self-delete (DEC-129); single monitor → two monitor surfaces (DEC-110) + cheat via occupant_progress (DEC-120) + per_q guru-full single index (DEC-119); push-history search → replaceState-only URL shareable/refreshable, back exits (DEC-111); list-everywhere pagination → student dashboard excluded, full list no q/page, history paginated (DEC-113); Bootstrap show/hide/toggle ban → vendored scripts + inline-only only, jQuery unbounded (DEC-114); hidden/closed 404 blanket → hidden 404, closed+visible 200 sudah-ditutup (DEC-115); export page-scoped → export ignores page, honors window/class/q/date (DEC-116); TTL re-check → TTL entry-only, in-flight runs, result opens (DEC-117); auto-advance per_q → no auto-advance, teacher-paced single index (DEC-119); ever-present leave gap → POST leave connected=0 last_seen kept, occupant_left, rejoin allowed (DEC-121); exact offline list → generic banner + saved-tick (DEC-124); per-answer unsent claim → no Q1-Q3 exact (DEC-124); countdown keeps ticking on dead stream → countdown freeze (DEC-125); waiting verbs 409/429 → waiting echo 200 (DEC-126); finished re-verbs → finished permanent reopen block (DEC-127); JS-only flow → no-JS three items (DEC-128); server-clock implicit → attempt_deadline_utc/question_deadline_utc sole truth (DEC-130); interview 28 locked; S1..S6 CLOSED per `00-INDEX.md` §5b (2026-10-14) | Must remain unchanged: R2/R4/R10 intact; jQuery unbounded per DEC-114; teacher owns content; essay human-graded 0–10; XLSX single export format; no auto-submit on cheat; each attempt immutable; room locks at start; no account for anon. Acceptance clause: this row is accepted only when every AC listed in its AFTER cell (AC-102/113/210/223..232, AC-130/140..145/202/208..213/217/220..222) is green in CI — the CHG-008 after-state is asserted by those cases, and the pre-existing AC-101..219 series carries the CHG-001..007 after-states. |

## 2. Impact summary

- Components: all new (no callers to migrate). Contracts: no external
  consumers; student/teacher UX is new surface (no compat). Data: fresh
  schema (§03); seeds create one teacher (BR-050). States: new machines
  (STATE-001..007) with no legacy mapping. Permissions: two-role model from
  day one (§04). Errors/retries: specified before code (no behaviour to
  preserve). Rollout: first deploy → seed → teacher creates quizzes →
  students join; rollback = previous image tag (`:stable`), data stays
  forward-migrated (CI/CD §02).
- Risks: timer fairness (server clock + pause shift), SSE proxy buffering
  (notice + refresh, never polling — DEC-102), cheat false positives (documented honesty), theme verified
  via AC-215 contrast (decided UI-007, no pending palette).

## 3. Acceptance of this delta

The delta is accepted when the user confirms: (a) BEFORE descriptions match
their practice, (b) AFTER behaviours in CHG-001..007 are desired, and (c) the
"must remain unchanged" column is correct. Until then status stays DRAFT.
Linked tests: AC-101..219 verify each AFTER row (CHG-008's audit-driven
refinement extends that series with AC-223..233 — AC-223..233 are the new
asserting cases CHG-008 added, not a renumbering of AC-101..219; the upper
bound is AC-233 per user decision N-A13 2026-10-14; the CHG-009 caching series
extends it further to **AC-239**); no BEFORE
regression tests exist (nothing to regress).
