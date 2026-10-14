# RBAC Matrix — quiz-v2

Status: DRAFT | Owner: 02-ARCHITECTURE | Updated: 2026-10-14 (N4/N9 applied; F-49 closed: "Manage users" cell now states the create scope)

## 0. Model statement

quiz-v2 uses exactly two roles: `teacher` and `student` (DECIDED). There is no
admin, operator, auditor, or per-class role in MVP. Anonymous token
participants are NOT a role — they hold a signed `attempt_token` valid for one
quiz only, with capabilities strictly below student. Enforcement is
server-side on every handler (middleware loads actor; handlers check
role + ownership); hidden UI buttons are NEVER the enforcement layer.

Unauthenticated ≠ forbidden: unauthenticated requests get `302 → /login`
(page) or `401 ERR-001` (fetch); authenticated-but-forbidden get `403
ERR-003` (no redirect, no existence leak beyond BR-007's 404 rule for
hidden/closed quizzes).

## 1. Permission matrix

Legend: ✅ allowed · ❌ denied (403/404 per rule) · — not applicable.

| Resource / Action | Teacher | Student | Anonymous (attempt_token) |
|-------------------|---------|---------|---------------------------|
| Manage users (create/edit/deactivate/delete) | ✅ self + all students + create new TEACHER accounts (BR-051, DEC-109 + Q2 decision 2026-10-14: a teacher may create teacher accounts); teacher-to-teacher mutate (edit/deactivate/delete of another teacher) → 403 ERR-003 | ❌ (own profile only) | ❌ |
| Edit own profile (name/email/class/grade/password) | ✅ own | ✅ own (never role/status) | — (no profile) |
| Create quiz / add-edit-delete questions (pre-start) | ✅ own quizzes (BR-006 freeze) | ❌ | ❌ |
| Open/close quiz, toggle visible/score/correct/token | ✅ own quizzes | ❌ | ❌ |
| View teacher recap, all attempts, cheat counts | ✅ own quizzes | ❌ (own attempts only) | ❌ (own card only) |
| Grade / re-grade essays | ✅ own quizzes (audit-logged) | ❌ | ❌ |
| Run matching controls (start/pause/stop/finish) | ✅ own quizzes | ❌ | ❌ |
| View live monitor (normal + matching) | ✅ own quizzes | ❌ | ❌ |
| List open+visible quizzes | ✅ (management view incl. hidden) | ✅ (predicate BR-007) | ❌ (code entry only) |
| Register / cancel (pre-start) / start / answer / review / submit | ❌ (teachers never sit quizzes — DECIDED DEC-030 strict; `requireStudent` rejects teacher role on all attempt endpoints) | ✅ own | ✅ own attempt (no cancel — anon holds no `registered` row; abandoning = discarding the token) |
| View own history + result card per flags BR-030/031 | ✅ (sees all as teacher) | ✅ own, flags applied | ✅ own card once (printable) |
| Join waiting room via code (token quizzes) | ❌ | ✅ with account | ✅ with name |
| Revoke registration (pre-start; plus DEC-088 never-present no-show post-start) | ✅ own quizzes | ❌ | ❌ |
| Leave waiting room (API-040b, pre-start) | — | ✅ (account: flips `connected=0`, `last_seen` kept) | ✅ (anon: clears the in-memory intent set — DEC-096 means no row exists to flip; `occupant_left` broadcast identical, DECIDED N4, 2026-10-14) |
| Change own password / forced rotation | ✅ | ✅ | — |

Ownership rule (DECIDED DEC-031 strict): teachers manage ONLY quizzes they own
(`owner_id = self`). No shared staff-room editing in MVP; any cross-owner
access is 403. The matrix cell "own quizzes" reflects this.

## 2. Record-level and trust rules

- R1: `student_id` scoping — every student query includes
  `AND (student_id = :self)`; forgetting the predicate is a P0 defect class
  covered by AC tests (list/detail/history/recap-denied).
- R2: IDOR — sequential integer ids are acceptable ONLY with R1 + BR-007.
  Tests MUST probe `/attempts/:otherId/...` expecting 403/404.
- R3: Anonymous token is single-quiz, expiry 24 h, no lateral movement:
  presenting it on another quiz's endpoints → 403 ERR-003. TTL 24 h is the
  entry gate (DEC-117): it gates identify-to-start only; answering/result legs
  use membership, not the clock. Qualified (DECIDED N9, 2026-10-14, per API-000):
  the TTL check applies ONLY at entry (API-050→051→033). Once the attempt row
  exists (`answering`/`submitted`), membership wins in-flight — answering,
  submit, result, and cheat endpoints re-check row ownership, never the TTL —
  so an anon mid-attempt at hour 24 is authorized, not rejected. AC-227 asserts
  this split.
- R4: Teacher-as-student is denied (DECIDED DEC-030 strict: no sitting any quiz,
  own or another teacher's) to keep grading and cheat semantics clean.
- R5: Role change is impossible via any endpoint (no `role` mass-assignment;
  struct binding allow-lists fields explicitly).

## 3. Enforcement points

| Layer | Check |
|-------|-------|
| Middleware | Session load, 60-min sliding expiry (BR-054), `must_change_password` gate, CSRF on POST. |
| Handler head | Role gate (`requireTeacher` / `requireStudent` / `requireAttemptToken`). |
| Service | Ownership (`owner_id`), visibility predicate, freeze guard, timer/state guards. |
| Templates | Conditional buttons (UX only, mirrors server truth). |
| Files | Image handler re-checks quiz access before serving bytes. |
| SSE | Stream URL carries attempt/session id; hub verifies membership per frame subscription; deactivation closes streams. |

## 4. Failure behaviour

- 401 unauthenticated (fetch) / 302 login (page): ERR-001.
- 403 forbidden: ERR-003 with generic message (no "you are a student"
  disclosure beyond necessity).
- 404 for hidden student access; closed+visible student GET → 200 closed view
  (no join; existence leak accepted as decided): BR-007 (DEC-115).
- 409 on control-verb races: ERR-300 with current state for re-sync.
- All denials are audit-logged (actor, resource, action) without logging
  passwords or tokens.

## 5. Revocation

Deactivate → sessions deleted and SSE connections dropped at once in the same
transaction (one definition for every revocation path — logical close is
immediate, no heartbeat split), attempt rows persist — deactivate is BLOCKED
(409 ERR-115, changing nothing), never force-submitting, while the target user
holds any `registered`/`answering` row on an open quiz (BR-051 guard, DECIDED
DEC-081; Q3, 2026-10-14 — the guard is the operative behaviour, so no sessionless
submit is ever created by this path). Live-session block (DEC-105): the same 409
ERR-115 guard covers a target who owns a `running`/`paused` matching session,
including self-deactivate. The guard runs inside the deactivate transaction
before any auto-close, so a rejected action changes nothing. Password change revokes ALL other
sessions immediately and closes their SSE streams in realtime (DECIDED
DEC-032, BR-054); the changing session survives with a rotated csrf_token.
Role set is append-only in MVP: no demote/promote endpoint (new account +
delete old instead — explicit, auditable).

Links: roles from `../../01-BUSINESS/01-PRD.md`; guards from `../../01-BUSINESS/03-BUSINESS-RULES.md`; states
from `../../01-BUSINESS/04-STATE-MACHINES.md`; tests in `../../06-QA-AND-MANAGEMENT/01-TEST-CASES.md`
(RBAC section mandatory).
