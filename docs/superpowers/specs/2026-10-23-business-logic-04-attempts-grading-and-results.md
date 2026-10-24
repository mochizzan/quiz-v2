# Attempts, Grading, and Results — Business Logic Specification

**Date:** 2026-10-23
**Status:** binding for domain 4; implements `2026-10-23-business-logic-00-contract.md` (the contract) without re-litigating its §3 rulings
**Scope:** attempt start/resume, answer persistence, deterministic shuffle, submit, objective scoring, essay grading, final score, review visibility, result/recap/read models, teacher dashboard aggregates, live monitor and SSE, attempt beacons
**Lens:** computational correctness — every score, order, count, tie-break, and payload is a total function with one derivation
**Sources:** contract §3–§7; schema spec §6.5, §6.7, §6.9–§6.12, §10; `migrations/0003`–`0005`; `internal/model/{attempt,authoring,participation}.go`; frontend contract §4.3, §4.4, §5.1–§5.2, §6, §7, §8.2–§8.3, §10.5–§10.7, §11, §13.5, §14, §15, §17–§19, §22–§24; old design §6, §8–§10, §13, §14 (not §11); current `routes_attempt.go`, `routes_teacher_grading.go`, `routes_teacher_core.go`, `routes_student.go`, `internal/viewmodel/*`, `templates/pages/{attempt,teacher}/*`
**Rule-ID prefix:** `ATT-###`; domain invariants `ATT-INV-##`; acceptance tests `AT-##`

---

## 1. Header

| Field | Value |
|---|---|
| Domain | 4 — Attempts, grading, results |
| Owned routes | 15 (contract §6): attempt GET/answer/review/submit/result/beacon; teacher dashboard, results, monitor, stream, grading queue, grade GET/POST, recap |
| Delegated routes | `POST /quiz/:id/attempts` (PAR owns the command; ATT owns the start rule it calls); `GET /student/history*` (PAR owns the read model; ATT owns the score contract) |
| Storage authority | `migrations/0003_authoring_schema.up.sql`, `0004_participation_schema.up.sql`, `0005_telemetry_schema.up.sql`; `internal/model/*`; schema spec §6.9–§6.12 (rank 1) |
| Screen authority | frontend contract §8.2–§8.3, §10.5–§10.7, §11, §13.5, §14, §15, §19 (rank 2) |
| Business authority | old design §6, §8, §9, §10, §13, §14 where rank 2 is silent (rank 3); old design §11 is forbidden |
| Time | store UTC; render WIB (UTC+7, fixed, no DST); no timers or countdowns (R-15) |

ATT-001. This document is the only authority for how an attempt is created, answered, submitted, scored, graded, reviewed, monitored, and aggregated. Where it is silent, contract §3 ranks apply.

ATT-002. Every score in this document is stored on the 0–1 scale in `DECIMAL(5,4)` and displayed as a whole percentage. No rule in this document stores 0–100 or displays 0–1.

ATT-003. Every timestamp displayed by these screens is WIB. Storage comparisons, day boundaries for "graded today", and SSE payloads use UTC. A "day" for a human-facing count is the WIB calendar day (00:00:00–23:59:59 WIB), converted to a UTC half-open interval for the query.

ATT-004. Every list, aggregate, and payload in this document is deterministic: equal inputs produce byte-identical output, including row order and tie-breaks. If a rule does not name a tie-break, contract R-21's `id` ascending applies.

ATT-005. No rule here depends on a schema change. Required schema deltas would appear only in §12; none are required. The implementation and payload deltas, and the product questions, are recorded in §12; none is a schema change.

---

## 2. Intent and boundaries

ATT-010. **Intent.** Given a live registration (PAR), a live quiz with immutable content and settings (AUT), and an authenticated participant or owner teacher (IDN), this domain decides: whether an attempt may start or must resume; what the participant sees and may save; what "submitted" means; the exact numeric score of every answer and of the attempt; what a student may review and when; how a teacher grades essays; what the teacher's queue, results, dashboards, monitor, and recaps contain; and which live signals the monitor receives.

ATT-011. **Boundary — registration and eligibility.** Registration creation, cancellation, join resolution, eligibility (grade/major), anonymous identity resolution, and the `registered`/`in_progress`/`completed` derivation are PAR's. ATT consumes `registration.id`, the participant identity, and the live/soft-deleted state. ATT never creates, cancels, or re-points a registration.

ATT-012. **Boundary — quiz content and lifecycle.** Quiz state, visibility, target audience, questions, options, correctness flags, `review_mode`, `show_score`, `randomize_questions`, `randomize_options`, `allow_retake`, `max_attempts`, publish, close, and the immutability of content/settings after publish are AUT's. ATT reads them and never writes them, except attempt-owned aggregates (`quiz_attempts.submitted_at`, `.final_score`, `attempt_answers.*`, selections, focus events).

ATT-013. **Boundary — sessions, CSRF, authorization primitives.** Cookie parsing, session lookup, the `must_change_password` gate, CSRF token issuance/verification, and the signed anonymous-identity cookie are IDN's. ATT calls them and applies its own resource-ownership check.

ATT-014. **Boundary — transactions, cascades, retention, error taxonomy.** Lock conventions, isolation level, the soft-delete sentinel, cascade scope/timestamp, and the 404/422/429/500 taxonomy are OPS's. ATT specifies the locks and rows it needs, then defers mechanism to OPS.

ATT-015. **Boundary — schema.** Table shape, column types, CHECK constraints, and unique keys are rank 1. ATT may not assume a column that does not exist: no `status`, no `started_at`, no `last_seen`, no `counted`, no `position` on answers, no `option_order`, no `focus_loss_count`, no `points` (R-07, R-25).

ATT-016. **Boundary — read models ATT does not own.** `GET /student` and `GET /student/history*` are PAR's pages. ATT supplies the attempt rows, the visibility predicate, and the best-score function defined in ATT-250–ATT-259; PAR must not invent a second score rule.

ATT-017. **Boundary — visual system.** Layout, colour, and components are `docs/DESIGN.md` (rank 4). This document names the data and copy, never the CSS.

ATT-018. **What this domain explicitly refuses to do.** No autosave (a legacy `autosave` nav key is a silent manual save, not a timer), no timers, no pause/resume clock, no countdown, no manual final-score override, no re-grade route, no question weighting, no "counted attempt" flag, no pass mark, no class average, no replay buffer for SSE, no per-question time, no current-question column, no score snapshot on the attempt.

ATT-019. **One derivation rule.** A value that appears on two screens is computed by one function named here: `DerivedQuestionOrder` (ATT-112), `AnsweredCount` (ATT-241), `BestScore` (ATT-243), `EffectiveMaxAttempts` (ATT-102), `FinalScore` (ATT-143), `DisplayPercent` (ATT-242), `ClassLabel`/`Initials` (viewmodel formatters, frontend §6). No handler, template, fixture, or query may re-implement one of these.

---

## 3. Actors, permissions, entry conditions

### 3.1 Actors

| ID | Actor | Identity source | May |
|---|---|---|---|
| A1 | Student participant | IDN session, role `student`, account `active` | Own attempt GET/answer/review/submit/result/beacon; own history via PAR |
| A2 | Anonymous participant | PAR/IDN signed identity cookie → live `anonymous_participants.cookie_token_hash` | Own attempt GET/answer/review/submit/result/beacon; no teacher pages; no student dashboard/history |
| A3 | Owner teacher | IDN session, role `teacher`, `active`, `must_change_password = 0` | Dashboard; results; monitor; stream; grading queue/detail/save; recap — for quizzes where `owner_account_id` is the session account |
| A4 | Non-owner teacher | IDN session, teacher, active | Nothing in this domain; every resource returns 404 (R-05, INV-26) |
| A5 | System | No session | Performs objective scoring at submit (ATT-142); writes no other row |
| A6 | Visitor | No session, no identity cookie | 404 on every attempt route; `/teacher*` redirects to login by IDN |

ATT-020. **404-vs-403 policy.** Missing, soft-deleted, or unauthorized resource → 404 page (R-19). CSRF failure → 403. Beacon cross-origin → 403. Rate limit → 429 with `Retry-After`. Validation failure → 422 with the same template and `FieldErrors`. State conflict → 303 with an error flash. Database failure → 500 page. There is no 409 and no 401 page in this domain.

ATT-021. A teacher is never a participant in an attempt: one account has one role (schema §6.2), so A3 can never own an attempt. If a teacher requests `/attempt/:id*`, the ownership check fails → 404.

ATT-022. A student may not request any `/teacher*` route in this domain; IDN rejects the role before ATT runs. If it reaches ATT (defect), ATT returns 404.

ATT-023. `must_change_password = 1` blocks every ATT teacher page; IDN redirects to `/change-password` before the handler (R-04). The SSE stream cannot follow a redirect usefully, so ATT-191 is the one place that re-checks the flag and returns 403. No other ATT route re-checks it.

### 3.2 Route permission matrix

| Route | A1 | A2 | A3 owner | A4 | A6 |
|---|---|---|---|---|---|
| `GET /attempt/:id` | own 200/303 | own 200/303 | 404 | 404 | 404 |
| `POST /attempt/:id/answer` | own 303/422 | own 303/422 | 404 | 404 | 404 |
| `GET /attempt/:id/review` | own 200/303 | own 200/303 | 404 | 404 | 404 |
| `POST /attempt/:id/submit` | own 303 | own 303 | 404 | 404 | 404 |
| `GET /attempt/:id/result` | own 200/303 | own 200/303 | 404 | 404 | 404 |
| `POST /attempt/:id/beacon` | own 204 | own 204 | 404 | 404 | 404 |
| `GET /teacher` | 404 | 404 | 200 | 200 (own data) | login |
| `GET /teacher/quizzes/:id/results` | 404 | 404 | 200 | 404 | login |
| `GET /teacher/quizzes/:id/monitor` | 404 | 404 | 200 (not-on-air if not open) | 404 | login |
| `GET /teacher/quizzes/:id/stream` | 404 | 404 | 200 SSE | 404 | login |
| `GET /teacher/grading` | 404 | 404 | 200 | 200 (own queue) | login |
| `GET/POST /teacher/grading/:answerID` | 404 | 404 | 200/303 | 404 | login |
| `GET /teacher/recap/:attemptID` | 404 | 404 | 200 | 404 | login |

ATT-024. "Own" for A1 means: attempt → registration → `student_id` → `students.account_id = session.account_id`, and the attempt, registration, quiz, and question rows are live. "Own" for A2 means: attempt → registration → `anonymous_participant_id`, and the live identity row's `cookie_token_hash` equals SHA-256 of the presented identity cookie. A cookie that resolves to a different participant is a 404, never a 403 (R-02, INV-27).

ATT-025. "Own data" for A4 on `/teacher` is legitimate: the dashboard shows only quizzes where `owner_account_id = session.account_id`. It is not a cross-tenant read.

ATT-026. **Entry condition — quiz state.** Attempt commands require the quiz to be live and in `open` or `closed` (R-10). No attempt can exist for a `draft` (AUT never opens registration for a draft); if one is found, every ATT route returns 404 as a corruption guard. Teacher results exist for `open` and `closed` only (a draft is 404, ATT-171); the monitor renders its not-on-air state for an owned draft or closed quiz (ATT-181); the stream is defined only while open (ATT-191).

ATT-027. **Entry condition — attempt state.** `GET /attempt/:id*` requires a live attempt. `submitted_at IS NULL` permits GET, answer, review, submit, beacon. `submitted_at IS NOT NULL` permits GET-result and beacon no-op; it redirects GET-attempt and GET-review to the result (ATT-111, ATT-131) and rejects answer with `answer_locked` (ATT-126).

ATT-028. **Entry condition — teacher grading.** Grading routes require a live essay answer whose attempt is live and submitted, whose question is live and type `essay`, and whose quiz is live and owned by the session teacher. The quiz may be `open` or `closed` (R-10 allows grading after close). A `draft` cannot have a submitted answer.

ATT-029. **Entry condition — stream.** Owner only, and only while `state = open` (R-20). A closed quiz's stream terminates with `quiz.closed`; a draft quiz's stream is 404 (there is nothing live to monitor).

---

## 4. States and transitions

### 4.1 Attempt state (derived, never stored — R-07)

| State | Definition | Entered by | Exits |
|---|---|---|---|
| `in_progress` | live row, `submitted_at IS NULL` | ATT-101 start | submit → `completed` |
| `completed` | live row, `submitted_at IS NOT NULL`, `final_score IS NULL` | ATT-141 submit (essay quiz) | last essay graded → `graded` |
| `graded` | live row, `submitted_at IS NOT NULL`, `final_score IS NOT NULL` | ATT-141 submit (objective-only) or ATT-221 last essay grade | terminal |

ATT-030. There is no stored attempt status, no `started_at`, and no "graded" column. "Start time" is `quiz_attempts.created_at`; "submit time" is `submitted_at`; "graded" means `final_score IS NOT NULL`. Every screen and query must derive from those three facts.

ATT-031. `completed` is not a terminal state for display: it means "submitted, awaiting essay grades". The participation state `completed` (R-06) is true at submit regardless of grading; the attempt's own `graded` state is separate. One is PAR vocabulary, one is ATT vocabulary; neither is stored.

ATT-032. The only permitted transition is `in_progress → completed` (submit) and `completed → graded` (finalization). No transition reverses, and a second submit is not a transition (ATT-147). A `graded` attempt cannot return to `completed`; there is no re-grade.

### 4.2 Answer-slot state

| Slot state | Objective definition | Essay definition |
|---|---|---|
| empty | zero live selections | `answer_text IS NULL` or whitespace-only |
| answered | ≥1 live selection | non-empty `answer_text` |
| ungraded | `score IS NULL` (before submit; after submit impossible for objective) | `score IS NULL` |
| scored | `score ∈ {0,1}` and `graded_at = submitted_at` | `score ∈ [0,1]`, `graded_at` set, `graded_by_account_id = teacher` |

ATT-033. A slot exists for every live question of the quiz from the moment of attempt creation (R-09, ATT-105). A missing slot is never repaired at read, answer, or submit time: it is a 500 data-integrity signal (ATT-105, ATT-123). A slot for a question that is no longer live is likewise never rendered; the question set is immutable after publish, so either state means the invariant was broken.

ATT-034. Objective slots acquire a score only at submit. Between attempt start and submit, `score` and `graded_at` remain NULL. The answer endpoint never scores (no live scoring, no partial credit, no "check answer" feature).

ATT-035. Essay slots acquire a score only through ATT-221. A blank essay is a normal ungraded essay (R-07); it does not auto-score 0 and does not block submission.

### 4.3 Registration state consumed from PAR (R-06)

| State | ATT-relevant meaning | Monitor/completed derivation |
|---|---|---|
| `registered` | no live attempt | row has no attempt, answered 0, `last_seen = registration.created_at` |
| `in_progress` | exactly one live attempt with `submitted_at IS NULL` | row uses that attempt |
| `completed` | ≥1 live attempt, none in progress | row uses the highest-numbered submitted attempt |

ATT-036. ATT never writes a registration. Start changes the *derived* registration state by inserting an attempt; submit changes it by stamping `submitted_at`. PAR's screens recompute, never cache.

ATT-037. At most one in-progress attempt per registration (INV-13, R-06). ATT enforces this under a row lock at start (ATT-104) and by resuming rather than creating a second (ATT-109).

### 4.4 Focus-event state

ATT-038. `focus_loss_events` is append-only for the attempt's life and after submit (a late beacon may still arrive if it was queued before submit; ATT-156 accepts it as a no-op after submit). Each row is one event with `kind ∈ {blur, hidden, fullscreen_exit}`. There is no counter column; every count is `COUNT(*)` over live rows (R-25, schema §6.12).

ATT-039. Focus events never change answers, scores, attempt state, or submission eligibility. They are informational for the monitor and recap (old design §6; R-20).

### 4.5 SSE connection state

| State | Entered by | Leaves to | Effects |
|---|---|---|---|
| connecting | EventSource GET, auth passed | snapshot-sent | headers written; snapshot computed |
| live | snapshot written | closed / disconnected | 2 s diff tick, 30 s heartbeat |
| closed | quiz state observed `closed` | ended | one `quiz.closed` event, then response ends |
| disconnected | client closes / write error / server shutdown | — | no writes, no cleanup rows |

ATT-040. There is no replay buffer: a reconnect is a new connection that starts at `connecting` and receives a fresh `snapshot` (ATT-196). There is no `id:` field, no `Last-Event-ID` handling, and no durable queue.

### 4.6 Transition guards summary

| Transition | Guard | Conflict behavior |
|---|---|---|
| start → `in_progress` | live registration, quiz `open`, ≥1 live question, quota not exhausted, no in-progress | in-progress → resume; closed → not_open; quota → error flash; no questions → error flash |
| answer → answered slot | attempt in progress, question in quiz, option in question | submitted → 303 error flash; invalid → 422; foreign → 404 |
| submit → `completed` | attempt in progress | already submitted → idempotent 303 to result |
| finalize → `graded` | all live slots scored, answer was last ungraded essay | cannot be forced; no route |
| close quiz | AUT only | ATT stream observes and emits `quiz.closed` |
| reconnect | owner, quiz live | closed → `quiz.closed`; draft/foreign → 404 |

---

## 5. Commands

ATT-100. **Command envelope.** A successful POST ends in 303 PRG; a validation failure re-renders the same template with 422 and `FieldErrors`; a state conflict redirects 303 with an error flash; a missing, soft-deleted, or foreign resource returns 404; a store failure returns the 500 page. There is no 409. Every POST verifies the CSRF token (R-16) except ATT-156, which is same-origin-checked instead. Every command runs at REPEATABLE READ and takes the locks named in its rule. Lock order is global: registration → quiz → attempt → answer slot → selection. No command takes a lock in the reverse order. Provenance is carried by row columns only (`created_at`, `updated_at`, `submitted_at`, `graded_at`, `graded_by_account_id`); ATT writes no audit table and no event log except `focus_loss_events`.

### 5.1 Attempt start and resume

ATT-101. **`StartOrResumeAttempt` — the rule PAR calls from `POST /quiz/:id/attempts`.**

| Field | Rule |
|---|---|
| Route | `POST /quiz/:id/attempts` — PAR's route; ATT owns this rule |
| Actor | the registration's participant: a student session or an anonymous identity cookie |
| Authorization | the caller must be the participant of `registration_id`; PAR resolves the registration, ATT re-checks ownership (INV-27). A mismatch is 404 |
| Preconditions | registration live and `registration.quiz_id = :id`; quiz live and not soft-deleted |
| Input | `registration_id`, quiz id, actor identity. No client-supplied attempt number, seed, score, or timestamp |
| Transaction | own transaction; lock the registration row for update, then shared-lock the quiz row (ATT-104) |
| Effects | exactly one of: resume the in-progress attempt with no writes, or insert one `quiz_attempts` row plus one `attempt_answers` slot per live question |
| Postconditions | at most one in-progress attempt; `attempt_no = 1 + count(live attempts)`; every live question has exactly one slot |
| Errors | `not_open`, `attempt_limit`, `no_questions`, 404, 500 (ATT-106, ATT-107) |
| Redirect | 303 to `/attempt/{id}` with no `q`; ATT-115 resumes at the first unanswered question |
| Provenance | `created_at` is the start time (R-07); `shuffle_seed` is written once and never changed |
| Idempotency | total: any retry returns the same attempt id and writes nothing |

ATT-102. **`EffectiveMaxAttempts(allow_retake, max_attempts)`.** `allow_retake = false` → 1. `allow_retake = true` and `max_attempts = 0` → unlimited (defensive; AUT never writes 0, R-06). `max_attempts > 10` → 10 (defensive; AUT accepts 1–10). Otherwise the stored number. The function reads the quiz row inside the transaction; it never trusts a form value. A changed setting after publish is impossible (AUT's immutability rule), so the result cannot drift between attempts.

ATT-103. **Seed.** `shuffle_seed` is 4 bytes from `crypto/rand`, interpreted big-endian as an unsigned 32-bit integer; a read failure is 500 and rolls the start back. Zero is a legal seed; there is no sentinel. It is generated only at insert, never regenerated on resume, reload, retry, or device change, and never rendered to any actor. It is not derived from time, user id, quiz id, or attempt number, so two attempts of the same quiz do not share an order unless their seeds collide by chance.

ATT-104. **Start transaction.** In order: (1) begin; (2) select the registration live row for update; if absent, soft-deleted, or `quiz_id ≠ :id`, roll back with 404; verify participant ownership; (3) select the quiz live row with a shared lock; if absent or soft-deleted, 404; (4) select live attempts ordered by `attempt_no`; if one has `submitted_at IS NULL`, commit and return it — **the quiz state is not checked on this path**, because a closed quiz still resumes and accepts answers (R-10); (5) require `state = 'open'`, else `not_open`; (6) count live questions, require ≥ 1, else `no_questions`; (7) evaluate ATT-102 against `count(live attempts)`, else `attempt_limit`; (8) insert the attempt; (9) insert the slots; (10) commit.

ATT-105. **Slot creation.** One `attempt_answers` row per live question, in `DerivedQuestionOrder` (ATT-112). Every slot starts with no selections, `answer_text NULL`, `score NULL`, `feedback NULL`, `graded_by_account_id NULL`, `graded_at NULL`. Slots are never created or deleted after start.

ATT-106. **No-questions and quota.** `no_questions` copy: "Kuis ini belum memiliki soal." `attempt_limit` copy: "Batas percobaan untuk kuis ini sudah tercapai." Neither writes anything; the registration survives, because a failed start must not undo a successful join (PAR's command owns the registration). The retry is a fresh transaction; errors are never cached.

ATT-107. **Start error mapping.** `ErrNotFound` → the 404 page. `ErrNotOpen` → PAR redirects 303 to `/quiz/{id}` with error flash key `not_open` ("Kuis ini belum dibuka atau sudah ditutup."). `ErrQuota` → flash key `attempt_limit`. `ErrNoQuestions` → flash key `no_questions`. Any store error → 500 page and full rollback. PAR must not create a second registration to work around a start error.

ATT-108. **Start idempotency and races.** The registration row lock serializes concurrent starts: the second transaction sees the first attempt and resumes it. `uq_attempt_number` is a backstop; a duplicate-key error rolls back and retries the rule once, and a second collision is 500 (it signals a writer that bypassed the lock). A lost response is recovered by repeating the POST; the answer is the same attempt id.

ATT-109. **Resume is not a state change.** It does not touch `updated_at`, does not generate a seed, does not re-evaluate the quota, and does not consume an attempt. `attempt_no` counts rows; `max_attempts` bounds rows, not resumes.

ATT-110. **No implicit start.** No GET creates an attempt. A missing attempt on `GET /attempt/:id` is 404; only PAR's POST reaches ATT-101.

### 5.2 Taking an attempt

ATT-111. **`GET /attempt/:id`.**

| Field | Rule |
|---|---|
| Actor | the attempt's participant (student session or the anonymous cookie identity) |
| Authorization | ownership through the registration; mismatch, soft-deleted attempt, registration, or quiz → 404 |
| Preconditions | attempt live. If `submitted_at IS NOT NULL` → 303 to `/attempt/{id}/result` with no flash |
| Input | optional `q` = display position (ATT-115); no other query value changes state |
| Transaction | one read-only transaction for header, slots, selections, and focus count; no locks |
| Effects | none |
| Postconditions | renders exactly one question; no key, score, verdict, or `is_correct` value reaches the template |
| Errors | 404 only; a closed quiz is not an error |
| Redirect | 303 to result for a submitted attempt |
| Provenance | start time = `created_at`; no per-view write |
| Idempotency | GET |

ATT-112. **`DerivedQuestionOrder(seed, questions, randomize)`.** With randomization off: `position` ascending, then `id` ascending. With it on, compute for every live question:

```
key(seed, ns, id) = SHA-256( ascii(decimal(seed)) || "|" || ns || "|" || ascii(decimal(id)) )
```

`decimal` is base 10 with no sign and no leading zeros (`0` renders `"0"`); `|` is the single byte `0x7C`; `ns` is `q` here. Compare the 32 digest bytes as unsigned lexicographic bytes; ties break by `id` ascending. The display position is the 1-based index in the result. The function is total and pure: it never mutates and never reads the database beyond the passed rows.

ATT-113. **`DerivedOptionOrder(seed, options, randomize)`.** Same function with `ns = "o"`, applied to the live options of one question. With randomization off: `position` ascending, then `id` ascending. The option letter is `OptionLetter(index)` (A, B, C…); it is derived per request and never stored. The form posts `option_id`, never a letter or an index.

ATT-114. **Stability proof.** The order is a pure function of `(seed, namespace, id)` over a fixed set: the seed is written once (ATT-103) and has no update path; question and option rows are immutable once the quiz leaves draft (AUT INV-10/R-01), and a published quiz cannot gain or lose items (AUT has no edit/delete after publish). Therefore reload, back/forward, a second tab, a retry, a server restart, and a different device with the same attempt all produce the same sequence. Removing an item (which cannot happen after publish) would not permute the remaining items, because each key depends only on its own id. A resume never re-seeds (ATT-109).

ATT-115. **`q` semantics.** `q` is a display position, never a question id. Missing, empty, non-numeric, or `0` means "first unanswered position": the lowest display position whose slot is unanswered (ATT-116); if every slot is answered, position 1. A parsed value `< 1` clamps to 1; `> total` clamps to total. The redirect after a save always carries `?q=` as a display position, so a stale link cannot address a question that has left the attempt.

ATT-116. **`AnsweredSlot`.** True iff the question is objective and the slot has at least one live selection, or the question is essay and `answer_text IS NOT NULL AND TRIM(answer_text) <> ''`. Blank selections count as unanswered. `AnsweredCount(attempt)` is the `COUNT(*)` of live slots satisfying this predicate (ATT-241); it is computed in SQL once per render, not from the current page and not from a stored counter.

ATT-117. **Attempt header.** Quiz title, `attempt_no` label, start time in WIB, answered/total, and a progress percentage `floor(answered * 100 / total)` clamped to 0–100 with 0 when `total = 0`. The attempt number label is `Percobaan {n} dari {max}` when ATT-102 is finite and `Percobaan {n}` when unlimited. No timer, countdown, duration, score, key, or correctness flag appears.

ATT-118. **Leak guard.** The attempt and review builders select option bodies and ids only. `is_correct`, `score`, `feedback`, and `final_score` are not loaded into any pre-submit view model, so a template cannot leak them even by accident. The option order is the only per-attempt variation the client can observe.

ATT-119. **Closed quiz notice.** If the quiz is closed while the attempt is in progress, the page renders with the static notice `quiz_closed_notice`: "Kuis ini sudah ditutup. Jawaban masih dapat diubah dan dikirim." Saving and submitting remain enabled (R-10). A cancelled registration or a soft-deleted quiz is 404, because the attempt is no longer reachable (INV-18).

### 5.3 Saving an answer

ATT-121. **`POST /attempt/:id/answer`.**

| Field | Rule |
|---|---|
| Actor | the attempt's participant |
| Authorization | ownership through the registration; mismatch or missing attempt → 404 |
| CSRF | required; failure is 403 with the contract `csrf` copy |
| Preconditions | attempt live and `submitted_at IS NULL`; registration live. Quiz state is irrelevant (ATT-119) |
| Input | `question_id`, `nav`, and exactly one of `selected_option_ids[]` (objective) or `essay_text` (essay) |
| Transaction | lock attempt for update → lock the slot row for update → replace selections/text (ATT-123) |
| Effects | the slot's saved answer becomes the posted answer; no score is computed; `attempt.updated_at` is stamped only if the answer changed |
| Postconditions | the answer survives reload and is the one used at submit; no other slot is touched |
| Errors | 404, 403, 303 conflict (`answer_locked`), 422 validation (ATT-126) |
| Redirect | 303 per `nav` (ATT-125) |
| Provenance | `attempt_answers.updated_at` and `quiz_attempts.updated_at`; no actor column exists |
| Idempotency | content-equal repeats are no-ops with the same redirect (ATT-127) |

ATT-122. **Validation.** Parse `question_id` as an unsigned integer; missing or malformed → 422 field `question_id` with "Soal tidak ditemukan." The id must name a live slot of this attempt; a question of another quiz or another attempt is 404. Then, by the question's type:

| Input | Rule | 422 field copy |
|---|---|---|
| single, zero ids | valid blank | — |
| single, one distinct id | must be a live option of the question | "Pilihan jawaban tidak valid. Muat ulang halaman." |
| single, two or more distinct ids | invalid | "Pilih satu jawaban." |
| multiple, zero ids | valid blank | — |
| multiple, one or more ids | each must be a live option of the question; duplicates collapse | "Pilihan jawaban tidak valid. Muat ulang halaman." |
| essay, text absent or whitespace-only | valid blank, stored `NULL` | — |
| essay, text present | stored verbatim; byte length ≤ 65,535 | "Jawaban terlalu panjang." |
| wrong field for the type | invalid | "Jawaban tidak sesuai jenis soal." |

The form field for an essay is `essay_text`; storage is `answer_text`. `nav` accepts `prev`, `save`, `next`, `review`, and the legacy `autosave`; a missing or unknown value is treated as `save` (save and stay), never as a state error.

ATT-123. **Answer transaction.** (1) Lock the attempt row for update; re-check ownership, `submitted_at IS NULL`, and live registration. If submitted, roll back to the conflict path. (2) Lock the slot by `(attempt_id, question_id, deleted_at = sentinel)`; a missing slot is 500 data integrity, because ATT-105 guarantees it. (3) Compare the posted content with the current content. If equal, commit with no writes and return the redirect — this is the idempotent path. (4) Soft-delete every live selection of the slot with one timestamp. (5) For an objective answer, insert one selection per accepted option id in `DerivedOptionOrder`, with the sentinel `deleted_at`; set `answer_text = NULL`. For an essay, set `answer_text` to the text or `NULL`; no selections are inserted. (6) Set `quiz_attempts.updated_at = now` and `attempt_answers.updated_at` via the write. (7) Commit.

ATT-124. **Effects.** Only the addressed slot changes. Objective answers store ids, never letters, indices, or correctness; essay answers store text, never a trimmed copy (blank is `NULL`). Existing scores are not touched while in progress; submit is the only objective scorer (ATT-142). The transaction is all-or-nothing, so a failed selection insert cannot leave a half-replaced set.

ATT-125. **Navigation and redirect.** Let `p` be the display position of the saved question and `n` the question count:

| `nav` | Redirect |
|---|---|
| `prev` | `/attempt/{id}?q={max(1, p-1)}` |
| `save` | `/attempt/{id}?q={p}&flash=saved` |
| `autosave` | `/attempt/{id}?q={p}` (silent; legacy clients only) |
| `next` | `p < n` ? `/attempt/{id}?q={p+1}` : `/attempt/{id}/review` |
| `review` | `/attempt/{id}/review` |

All are 303. `flash=saved` renders the contract key `saved` ("Perubahan tersimpan."). No redirect carries a question id.

ATT-126. **Answer errors.** A submitted attempt → rollback, 303 to `/attempt/{id}/result` with error flash `answer_locked`: "Jawaban tidak dapat diubah setelah dikirim." A foreign or missing attempt → 404 page. A validation failure → 422, the same question template, the posted values preserved in the form, the stored values used for every other field, and `FieldErrors` from ATT-122. A store failure → 500 page with no partial write. A stale form pointing at a removed option is 422, never a silent drop.

ATT-127. **Idempotency and stale tabs.** The same content posted twice produces the same slot content and redirect; the second request takes the no-write path, so `updated_at` does not move. When two tabs save different content to the same slot, commit order decides: the later commit wins, no merge is attempted, and each request's redirect reflects its own `nav`. The monitor sees the later save. There is no version token, because the schema has none (R-25); the before-unload warning is the only stale-tab mitigation.

### 5.4 Pre-submit review

ATT-131. **`GET /attempt/:id/review`.**

| Field | Rule |
|---|---|
| Actor | the attempt's participant |
| Authorization | ownership; missing or foreign → 404 |
| Preconditions | attempt live and `submitted_at IS NULL`; if submitted → 303 to `/attempt/{id}/result` with no flash |
| Input | none |
| Transaction | one read-only transaction for all slots, selections, and the unanswered list |
| Effects | none |
| Postconditions | every question is listed in `DerivedQuestionOrder`, each with the saved answer or a blank marker |
| Errors | 404 only; a closed quiz still renders |
| Redirect | 303 to result when the attempt is already submitted |
| Provenance | none |
| Idempotency | GET |

ATT-132. **Review content.** Position, type label, question body, optional image, and the saved answer (selected option bodies in `DerivedOptionOrder`, joined with ", "; essay text; `—` for blank). The pre-submit review is independent of `review_mode` (R-18): it never shows a key, a verdict, a score, feedback, or `is_correct`. It is a read of the same slots the answer page writes, not a second store.

ATT-133. **Unanswered list.** Positions where ATT-116 is false, rendered as "Belum dijawab: 3, 7, 12." in ascending position order; if none, the warning block is absent. The list does not block submission. A blank objective answer will score 0 (ATT-142); a blank essay is gradeable (R-07).

ATT-134. **Submit form.** One POST to `/attempt/{id}/submit` with only `csrf_token`, no answer payload and no "confirmed" flag. The confirmation ("Kumpulkan jawaban sekarang? Jawaban tidak dapat diubah setelah dikirim.") is a UI enhancement; the server never requires it. Navigation back to a question is `GET /attempt/{id}?q={p}`.

ATT-135. **Closed quiz.** If the quiz closes while the review is open, the page renders `quiz_closed_notice` (ATT-119) and the submit form stays active. A stale review of an attempt that was submitted in another tab redirects to the result on the next GET (ATT-131); the submit POST is handled by ATT-141's idempotent path.

### 5.5 Submit and objective scoring

ATT-141. **`POST /attempt/:id/submit`.**

| Field | Rule |
|---|---|
| Actor | the attempt's participant |
| Authorization | ownership; missing or foreign → 404 |
| CSRF | required |
| Preconditions | attempt live, registration live, quiz live. Quiz state may be open or closed (R-10) |
| Input | none beyond `csrf_token` |
| Transaction | lock the attempt for update, then lock every live slot for update ordered by `id` (ATT-144) |
| Effects | objective scores are computed, every objective slot is stamped graded, `submitted_at` is set once, and `final_score` is set iff every slot is scored |
| Postconditions | the attempt is completed; no answer write is possible afterwards; essay grading may still change `final_score` |
| Errors | 404; a store failure rolls back to an unsubmitted attempt and returns 500; there is no validation error |
| Redirect | 303 to `/attempt/{id}/result?flash=submitted` (key `submitted`, "Jawaban terkirim.") |
| Provenance | `submitted_at`; objective `graded_at = submitted_at`; objective `graded_by_account_id = NULL` (system) |
| Idempotency | a second submit writes nothing and redirects to the result without a flash |

ATT-142. **Objective scoring.** For every live objective slot, use the live selections and the live options only:

| Type | Score 1 iff | Otherwise |
|---|---|---|
| single | exactly one live selection, and that option is live and `is_correct` | 0 |
| multiple | the set of live selected ids equals the set of live correct ids, with no extra and no missing id | 0 |
| essay | — (never scored here) | `score` stays `NULL` |

Blank objectives score 0. Wrong objectives score 0. There is no partial credit, no negative marking, and no weighting. Every objective slot receives `score ∈ {0, 1}`, `graded_at = submitted_at`, and `graded_by_account_id = NULL`; this satisfies `chk_answers_grading` because a non-null score always has a non-null `graded_at`. Pre-existing scores, if any, are overwritten from the selections; the stored score of an in-progress attempt is never trusted.

ATT-143. **`FinalScore`.** Let `U_i = round(score_i × 10000)` as an integer (objective scores are 0 or 10000; essay scores are 0–10000), and `n` the number of live slots. `final_score` is defined iff `n > 0` and every slot has a non-null score. Then:

```
units = floor( (2 × ΣU_i + n) / (2 × n) )      // mean, rounded half-up to 4 dp
final_score = units / 10000                     // DECIMAL(5,4), 0..1
```

Otherwise `final_score = NULL`. The mean is unweighted: every question counts once regardless of type or option count. The result is clamped to 0–10000 defensively. Worked example: scores 1, 0, 0 over three questions → `ΣU = 10000`, `n = 3` → `floor(20003 / 6) = 3333` → `0.3333`, displayed 33%. Two questions scored 1 and 0.6667 → `ΣU = 16667`, `n = 2` → `floor(33336 / 4) = 8334` → `0.8334`, displayed 83%.

ATT-144. **Submit transaction.** (1) Lock the attempt row; re-check ownership and liveness. If `submitted_at IS NOT NULL`, commit and return the idempotent redirect. (2) Lock all live slots ordered by `id`; this is the same relative order used by grading (attempt → answer), so a concurrent grade or answer save either waits or is already impossible. (3) Compute every objective score from the locked selections. (4) Take one `now` and use it for `submitted_at` and every objective `graded_at`. (5) Update objective slots; leave essay scores, feedback, and grading columns untouched. (6) Compute ATT-143. (7) Update the attempt once with `submitted_at = now` and `final_score = computed-or-NULL`. (8) Commit. Any failure rolls back both the scores and the submission: a submitted attempt never has a half-scored objective slot.

ATT-145. **Blanks and confirmation.** Submission is allowed with unanswered questions; they are not rejected. The server does not require a minimum answered count, a confirmation token, or a client timestamp. A blank objective is scored 0 at submit; a blank essay is a normal ungraded essay and can later receive 0–1.

ATT-146. **After submit.** `answer` POSTs hit ATT-126's `answer_locked` conflict; `review` GETs redirect to the result; the result page renders the post-submit review. Objective scores and `submitted_at` are immutable. Only essay grading (ATT-221) may change a slot score and therefore `final_score`; nothing can change `submitted_at` or clear it.

ATT-147. **Submit idempotency and races.** Two concurrent submits serialize on the attempt row: the first commits, the second observes `submitted_at` and returns the same redirect with no flash and no recomputation. A retry after a lost response behaves the same. The endpoint is therefore safe for double-clicks and network retries. A quiz close racing with submit does not block it (R-10).

### 5.6 Result and post-submit review

ATT-151. **`GET /attempt/:id/result`.**

| Field | Rule |
|---|---|
| Actor | the attempt's participant |
| Authorization | ownership; a teacher, another student, or an unknown id → 404 (INV-26) |
| Preconditions | attempt live, registration live, quiz live |
| Not submitted | 303 to `/attempt/{id}` with info flash `not_submitted`: "Percobaan ini belum dikirim." |
| Transaction | one read-only transaction for header, slots, selections, and focus count |
| Effects | none |
| Postconditions | the page is the post-submit review; `review_mode` and `show_score` fully determine the rows |
| Errors | 404, or the 303 above |
| Redirect | 303 for an in-progress attempt; none otherwise |
| Provenance | `submitted_at`, `graded_at`, `graded_by_account_id` are read, never written |
| Idempotency | GET |

ATT-152. **Review matrix.** The result page implements R-18 exactly; `show_score = false` is applied after the mode:

| Mode | Question body | Student answer | Key | Verdict | Essay score + feedback |
|---|---|---|---|---|---|
| `questions_and_key` | yes | yes | yes | yes, with colour | yes, when graded |
| `key_only` | no | no | objective key list only | no | no; essay rows omitted |
| `questions_only` | yes | yes | no | yes, with colour | no |
| `answers_only` | yes | yes | no | no, and no verdict colour | no |

ATT-153. **Row derivation.** Position, type label, body, image. The student answer is the selected bodies in `DerivedOptionOrder` joined with ", ", the essay text, or `—`. The key is the correct option bodies in `DerivedOptionOrder` joined with ", "; essays have no key. The verdict is derived from the stored `score`, never recomputed in the view: objective `score = 1` → correct; `score = 0` and answered → incorrect; no content → unanswered; essay `score IS NULL` → ungraded; otherwise graded. For objective rows the verdict is the per-row score (benar = 1, salah = 0); no duplicate numeral is printed. For essays the 0–100 numeral is printed only when graded and `show_score` is true. Feedback is printed only in `questions_and_key`, because it is teacher commentary attached to an answer that the other modes hide.

ATT-154. **Waiting and hidden scores.** A submitted attempt with `final_score IS NULL` shows "Menunggu penilaian" in the score slot and on every ungraded essay row; no percentage is shown and no average exists. With `show_score = false`, the score tile is omitted and every per-row essay numeral is suppressed, while verdicts permitted by the mode remain. A hidden score is never rendered as 0; where a student-facing score cell is structurally required (history), it renders `—` (R-18, INV-17). The teacher recap and results always show the true score.

ATT-155. **Result chrome and actions.** Header: quiz title, attempt number label, submitted time in WIB, focus-loss count if greater than zero ("Tercatat N kali meninggalkan layar kuis."), mode label, and the score tile when permitted. Actions: `Kembali ke kuis` always; `Riwayat kuis` only for an authenticated student (anonymous participants have no history); `Tinjau jawaban` is omitted because this page is the review (`ReviewHref` is empty); `Coba lagi` is a POST form to PAR's start route, rendered only when the quiz is open, `allow_retake` is true, and ATT-102 leaves a remaining attempt. The button is a convenience; ATT-101 re-checks every condition. No duration, countdown, pass mark, or class average appears.

### 5.7 Attempt beacons

ATT-156. **`POST /attempt/:id/beacon`.**

| Field | Rule |
|---|---|
| Actor | the attempt's participant |
| Authorization | ownership; missing or foreign → 404 |
| CSRF | exempt (R-20, INV-28); same-origin required (ATT-159) |
| Preconditions | attempt live and registration live. Quiz state is irrelevant; a closed quiz accepts beacons from in-progress attempts |
| Input | `text/plain` whose trimmed body is the kind, or `application/json` with a `kind` field |
| Transaction | one statement: progress updates the attempt row; focus inserts one event row |
| Effects | progress touches `updated_at`; focus appends one `focus_loss_events` row with `occurred_at = now` |
| Postconditions | focus never changes answers, scores, state, or eligibility (ATT-039) |
| Errors | 400 malformed/unknown kind; 404 foreign; 403 bad origin; 429 rate limit; 500 store failure |
| Redirect | none; 204 with an empty body on success |
| Provenance | `occurred_at` = server clock; the client `at` value is ignored |
| Idempotency | progress is idempotent; focus is intentionally not (ATT-160) |

ATT-157. **Kinds.** `progress`, `blur`, `hidden`, `fullscreen_exit`. JSON may also carry `question_id`, `position`, and `at`; all three are accepted and ignored, because the schema has no current-question column and no trusted client clock (R-25). Unknown fields are ignored. A missing or unknown `kind`, malformed JSON, an empty text body, or a non-`text/plain`/`application/json` content type is 400 with an empty body. No HTML is rendered, so 400 replaces 422 for this route only.

ATT-158. **Rate limits.** Progress: 30 requests per minute per attempt. Focus: 10 requests per minute per attempt across the three kinds combined. The limiter is an in-memory sliding 60-second window keyed by `(attempt_id, bucket)` where bucket is progress or focus; over-limit returns 429 with `Retry-After: 60` and an empty body. The limit is per process; OPS owns whether a shared limiter is required for multi-instance deployments (it is not for the current single-binary deployment). A limited focus event is dropped, never queued, and never blocks submit.

ATT-159. **Ownership and same-origin.** The attempt must belong to the current student account or anonymous cookie hash; otherwise 404, not 403, so attempt ids are not enumerable (INV-27). The `Origin` header must be present and equal the request's effective scheme + host + port; a mismatch or absence is 403 with an empty body. The CSRF token is deliberately not required, because `navigator.sendBeacon` cannot set one (R-20). Cookies are sent by the browser on this same-origin POST; the handler still resolves the session/identity itself.

ATT-160. **After submit and idempotency.** If `submitted_at IS NOT NULL`, the route returns 204 without writing and without consuming rate-limit budget: a queued beacon cannot mutate a completed attempt. `progress` is a single `UPDATE ... SET updated_at = now` and is idempotent in effect. A focus event is a real occurrence: a retried request appends a second row and may over-count by one; the count is informational only, is never part of a score, and never blocks submission (ATT-039). The server never backdates an event from the client `at`.

### 5.8 Teacher dashboard

ATT-161. **`GET /teacher`.**

| Field | Rule |
|---|---|
| Actor | an active teacher (`role = teacher`, `status = active`, no forced password change) |
| Authorization | every card is scoped to `quizzes.owner_account_id = session account`; no cross-teacher data |
| Preconditions | none; a teacher with no quizzes sees four empty cards |
| Input | none |
| Transaction | one read-only transaction for all four cards, so counts agree with each other |
| Effects | none |
| Postconditions | deterministic card order and tie-breaks (ATT-162–ATT-164) |
| Errors | 500 page only |
| Redirect | none |
| Provenance | reads `created_at`, `submitted_at`, `final_score`; writes nothing |
| Idempotency | GET |

ATT-162. **`TopScores`.** A quiz is eligible iff it is owned, live, `show_score = true`, and has at least one live submitted attempt with `final_score IS NOT NULL`. Hidden scores are excluded entirely, never rendered as zero (INV-17, R-21). For each eligible quiz choose the attempt with the greatest `final_score`; tie → earliest `submitted_at`; tie → smallest attempt id. Render quiz title, student display name, class label, `DisplayPercent(final)`, and a link to that quiz's results. Order the quizzes by `created_at` descending, then id descending; cap 5. A quiz whose only attempts are in progress is not eligible. Empty copy: "Belum ada nilai."

ATT-163. **`Recent`.** The five newest owned live quizzes by `created_at` descending, then id descending. Each row carries title, state, live question count, distinct live registrations, creation date in WIB, and the quiz detail href. Drafts are included, because this is the teacher's own workspace. Empty copy: "Belum ada kuis."

ATT-164. **`MostJoined` and `LeastJoined`.** Candidates are owned live quizzes in state `open` or `closed`; drafts are excluded because they cannot be joined and would occupy every "least joined" slot (recommended default, §12 Q1). The count is distinct live registrations, never attempts and never participants across quizzes (INV-06 makes these equal per quiz). Most: count descending, `LOWER(title)` ascending, id ascending, limit 3. Least: count ascending, `LOWER(title)` ascending, id ascending, limit 3. A quiz can appear in both lists when fewer than six candidates exist; there is no de-duplication across the two cards. Empty copy for either: "Belum ada kuis yang dibuka."

ATT-165. **No invented aggregates.** The dashboard does not show a pass rate, class average, question-bank tile, total participant headline, or score distribution: no rule in this contract derives them, and a placeholder would violate ATT-004. The only numbers are the four card sets above.

ATT-166. **Consistency.** The four cards are read in one snapshot; a quiz created between cards cannot appear in `Recent` but be missing from `MostJoined`. Titles are compared case-insensitively with the database collation, so `Ulangan` and `ulangan` tie and the id decides. No card is paginated.

### 5.9 Quiz results

ATT-171. **`GET /teacher/quizzes/:id/results`.**

| Field | Rule |
|---|---|
| Actor | the quiz's owner teacher |
| Authorization | foreign quiz, missing quiz, or soft-deleted quiz → 404 (INV-26) |
| Preconditions | quiz live and in state `open` or `closed`; a draft is 404 because it cannot have attempts |
| Input | `state`, `class`, `q`, `page` |
| Transaction | one read-only transaction for rows, counts, and the average |
| Effects | none |
| Postconditions | one row per submitted live attempt; in-progress attempts are not rows (ATT-176) |
| Errors | 404; invalid query values fall back to defaults, never 422 (R-21) |
| Redirect | none |
| Provenance | reads `submitted_at`, `final_score`, focus counts |
| Idempotency | GET |

ATT-172. **Filters.** `state` ∈ `all | waiting | graded` (default `all`; invalid → `all`). `waiting` = submitted and `final_score IS NULL`; `graded` = submitted and non-null. `class` ∈ `X | XI | XII` (default all; invalid → all), matched against the participant's grade (student grade or anonymous grade) through `RomanGrade`; the major is not part of the filter. `q` is trimmed, lowercased, capped at 100 runes, and matched as a case-insensitive substring of the display name; `%` and `_` are escaped so they are literal. Filters combine with AND. A filter is applied before pagination. Invalid values are dropped from generated links rather than echoed, so a garbage URL cannot be reflected into pagination.

ATT-173. **Tabs and counts.** Tabs: "Semua", "Menunggu", "Sudah dinilai". Their counts apply the current `q` and `class` filters but not `state`, so the numbers describe what the other filters leave; the active tab is `state`. The header also carries two quiz-wide metrics independent of all filters: `Submitted` (all submitted live attempts) and `InProgress` (live attempts with `submitted_at IS NULL`). This is the only place in-progress attempts appear on this page.

ATT-174. **Average.** Quiz-wide over **all** submitted attempts with a non-null `final_score`, regardless of `show_score` because `show_score` governs the student's view, not the owner's. With `ΣU` the sum of stored score units and `n` the attempt count:

```
tenths = floor( (2 × ΣU + 10 × n) / (20 × n) )   // one decimal, half-up
average = tenths / 10                              // percent, rendered with a comma
```

Nil when `n = 0`, rendered `—`. Examples: 7250 units over one attempt → 72,5%; 7250 over two → 36,3%. The average is not the mean of rounded per-row percentages and is not affected by the active tab or search.

ATT-175. **Empty states.** No submitted attempts at all: title "Belum ada percobaan.", body "Percobaan siswa akan muncul di sini setelah mereka mengirim jawaban.", action "Buka detail kuis". Filters or search active with no match: title "Tidak ada percobaan yang cocok.", body "Coba kata kunci lain atau tampilkan semua percobaan.", action "Tampilkan semua" linking to the unfiltered page. The two states are distinct and are never shown together.

ATT-176. **Rows and order.** Flat list, one row per submitted attempt: student name, class label, attempt number, submitted time in WIB, score, focus count, and `Rekap` link to `/teacher/recap/{attemptID}`. Order: `submitted_at` descending, then attempt id ascending. There is no group header and no clustering by student; "grouped by student" in the frontend contract is satisfied by the repeated student column, not by a second sort key. In-progress attempts are excluded because a results row has a submission time; the monitor is the live view for them. Page size 10; `page` clamps to 1..pages; an empty result is page 1.

ATT-177. **Score cell.** `final_score IS NULL` → "Menunggu"; otherwise the whole percent, including `0%`. `show_score = false` does not hide a score from the owner teacher; it only governs the student pages (R-18). Scores use tabular numerals and `DisplayPercent` (ATT-242), never a raw 0–1 value.

ATT-178. **Implementation delta.** The current fixture exposes tabs `in_progress|completed` and a 10-row page; this rule replaces the tabs with `waiting|graded`, removes in-progress rows, and keeps 10. The fixture's average over displayed percentages is replaced by ATT-174. Recorded in §12 D2.

### 5.10 Live monitor

ATT-181. **`GET /teacher/quizzes/:id/monitor`.**

| Field | Rule |
|---|---|
| Actor | the quiz's owner teacher |
| Authorization | foreign or missing quiz → 404 |
| Preconditions | owned quiz in state `open` renders rows; owned `draft` or `closed` renders the not-on-air empty state with HTTP 200 |
| Input | none |
| Transaction | one read-only transaction for rows, progress, and the generated timestamp |
| Effects | none |
| Postconditions | one row per live registration, including participants who have not started |
| Errors | 404 for foreign/unknown; 500 only for store failure |
| Redirect | none; not-on-air is a state, not an error |
| Provenance | `generated_at` = server now in UTC; rows read only |
| Idempotency | GET |

ATT-182. **Row derivation.** For each live registration, select its current attempt: the in-progress attempt if one exists (at most one), else the highest `attempt_no` submitted attempt, else none. Then:

| Field | Derivation |
|---|---|
| `registration_id` | the registration row id; the stable row key (R-02, INV-27) |
| `attempt_id` | selected attempt id, or null |
| `name`, `initials` | PAR's display name; `viewmodel.Initials(name)` |
| `class_label` | `ClassLabel(grade, major)` from PAR; `—` when unknown |
| `state` | `registered` when no attempt, `in_progress` when the selected attempt is unsubmitted, else `completed` (R-06) |
| `answered` | ATT-241 on the selected attempt; 0 when none; actual content, never `total` for a completed attempt |
| `total` | live question count of the quiz |
| `focus_loss` | `COUNT(*)` of live focus events for the selected attempt; 0 when none |
| `last_seen` | selected attempt's `updated_at`, else the registration's `created_at`; never null (R-20) |

ATT-183. **Order.** In-progress rows first, then completed, then registered; within a group `LOWER(name)` ascending, then `registration_id` ascending. Names are display labels, not identities; the id is the only key. The order is stable across reloads and SSE snapshots.

ATT-184. **Progress rail.** `registered` = row count; `in_progress` and `completed` = row state counts; `answered` = sum of row `answered`; `total` = `registered × question count`. Percent = `floor(answered × 100 / total)`, clamped, 0 when total is 0. The rail is derived from the same rows, so it cannot disagree with the table.

ATT-185. **Columns.** Registration identity (name, initials, class), state, answered/total, focus count, last seen, and the attempt link when an attempt exists. There is **no score column, no start-time column, and no current-question column**: score is not part of the SSE participant payload, a start time would be a second timestamp with no monitor meaning, and no current-question column exists (R-25). The student's name is not clickable to a recap; the row links to the recap only through the attempt action.

ATT-186. **Empty states.** Zero registrations on an open quiz: "Belum ada peserta" / "Peserta yang bergabung akan muncul di sini secara langsung." with "Kembali ke kuis". Owned draft/closed: "Kuis tidak sedang berlangsung" / "Monitor hanya tersedia selama kuis dibuka. Setelah ditutup, gunakan halaman hasil untuk melihat rekap." with "Kembali ke kuis" and a results link. A `<noscript>` meta refresh every 15 seconds is the no-JS fallback; with JS the page uses ATT-191.

ATT-187. **One builder.** The monitor page and the SSE snapshot call the same derivation (ATT-182–ATT-184). The stream's snapshot is the monitor model minus `StreamHref`; if the two ever disagree, the stream is wrong, not the page.

### 5.11 SSE stream

ATT-191. **`GET /teacher/quizzes/:id/stream`.**

| Field | Rule |
|---|---|
| Actor | the quiz's owner teacher |
| Authorization | not signed in, non-teacher, foreign, or unknown → 404 with an empty body; owner with `must_change_password` → 403 |
| Preconditions | owned quiz in state `open`. Owned `closed` at connect → 200, `quiz.closed`, end. Owned `draft` → 404 |
| Input | none; `Last-Event-ID` is ignored |
| Transaction | per tick, one read-only transaction; no locks held across ticks |
| Effects | writes nothing; no connection row, no replay buffer |
| Postconditions | the client has a complete snapshot or a terminal close |
| Errors | DB error during a tick closes the stream; no invented event |
| Redirect | none |
| Provenance | every payload carries UTC RFC 3339 timestamps |
| Idempotency | a reconnect is a new connection with a fresh snapshot |

ATT-192. **Handshake.** Respond 200 with `Content-Type: text/event-stream; charset=utf-8`, `Cache-Control: no-cache, no-store, no-transform`, `Connection: keep-alive`, and `X-Accel-Buffering: no`. For an open quiz, send the snapshot immediately, before the first diff tick; for a closed quiz at connect, send `quiz.closed` instead (ATT-191). No CORS headers are emitted, so a cross-origin `EventSource` cannot read the stream. The response is not compressed.

ATT-193. **Snapshot payload.** `event: snapshot` and one line of JSON:

```json
{"quiz":{"id":12,"title":"Ulangan Bab 3","state":"open"},"progress":{"registered":31,"in_progress":4,"completed":27,"answered":512,"total":930},"participants":[{"registration_id":501,"attempt_id":9001,"name":"Ayu Lestari","initials":"AL","class_label":"XII IPA","answered":7,"total":30,"state":"in_progress","focus_loss":1,"last_seen":"2026-10-23T02:42:10Z"}],"generated_at":"2026-10-23T02:42:11Z"}
```

`attempt_id` is `null` for a participant who has not started. `total` is `registered × question count`. `last_seen` and `generated_at` are UTC with `Z`. Keys are exactly these snake_case names; no extra keys and no HTML. The client replaces its entire table and rail from this event.

ATT-194. **Event payloads.** `participant.added` carries one full participant object, exactly the snapshot element shape. `participant.updated` carries the fixed subset, always all seven fields:

```json
{"registration_id":501,"attempt_id":9001,"answered":8,"total":30,"state":"in_progress","focus_loss":1,"last_seen":"2026-10-23T02:42:31Z"}
```

`quiz.closed` carries `{"quiz_id":12}`. `heartbeat` carries `{"at":"2026-10-23T02:42:41Z"}`. Framing is `event: <name>` then `data: <json>` then a blank line; there is no `id:`, no `retry:`, and no comment line. Payloads contain no newline, so each `data:` is one line.

ATT-195. **Diff loop.** Every 2 seconds (measured from the end of the previous tick, never overlapping), read the live state in one snapshot. Compare by `registration_id`:

| Change | Event |
|---|---|
| new registration | `participant.added` with the full row |
| removed/soft-deleted registration | one fresh `snapshot` (there is no `removed` event; the client replaces) |
| `name`, `initials`, or `class_label` changed | one fresh `snapshot` (identity labels are not in the update subset) |
| any of `attempt_id`, `answered`, `total`, `state`, `focus_loss`, `last_seen` changed | `participant.updated` with all seven fields |
| none | nothing |
| quiz state no longer `open` | `quiz.closed`, then end |

The progress rail is not re-sent; the client recomputes it from the rows after every add/update/snapshot. A mid-stream snapshot is always complete and replaces state, so it is safe at any time.

ATT-196. **Heartbeat and reconnect.** Emit `heartbeat` when 30 seconds have passed since the last event of any kind; a busy stream may therefore go longer without one because data is already flowing. There is no replay buffer and no `Last-Event-ID` handling: a reconnect starts at ATT-192 and receives a fresh snapshot, which is the only correct recovery. The client must call `close()` on `quiz.closed`, otherwise `EventSource` would reconnect to a closed quiz and receive another `quiz.closed` (the server never keeps the response open after that event).

ATT-197. **Termination.** End the response after `quiz.closed`. Also end on a client disconnect (write error), on server shutdown, or on a DB error during a tick; in the latter two cases no event is sent. No cleanup rows exist, so termination has no side effect. Multiple tabs may hold independent connections; each has its own ticker and snapshot.

ATT-198. **Security.** The stream is owner-only and read-only. It never includes student ids, account ids, emails, scores, answer content, or correctness. `registration_id` is a per-quiz row key, not a person id. A teacher who loses ownership (impossible in v1 because ownership is immutable) or becomes inactive is rejected on the next connection, not mid-stream; the current connection ends only when the quiz closes or the client disconnects.

### 5.12 Grading queue

ATT-201. **`GET /teacher/grading`.**

| Field | Rule |
|---|---|
| Actor | an active teacher |
| Authorization | rows are ungraded essays of quizzes owned by the session account only (INV-26) |
| Preconditions | none |
| Input | `quiz`, `class`, `q`, `page` |
| Transaction | one read-only transaction for rows, counts, and the today count |
| Effects | none |
| Postconditions | one row per ungraded essay of a submitted attempt; never an objective answer |
| Errors | invalid filters fall back to defaults; no 404 for an empty queue |
| Redirect | none |
| Provenance | reads `submitted_at`, `graded_at`, `graded_by_account_id` |
| Idempotency | GET |

ATT-202. **Row predicate and search.** An answer is in the queue iff: the question is live and `type = 'essay'`, the answer is live with `score IS NULL` and `graded_at IS NULL`, its attempt is live with `submitted_at IS NOT NULL`, its registration is live, and its quiz is live and owned by the session teacher. `quiz` filters by owned quiz id; `class` uses ATT-172's Roman grade; `q` (trimmed, lowercased, ≤100 runes) matches the student display name, the quiz title, or the question prompt as a substring with escaped wildcards. Filters combine with AND before pagination.

ATT-203. **Counts.** `Pending` = all ungraded essays across owned quizzes, ignoring the queue filters. `GradedToday` = answers with `graded_by_account_id = session teacher` and `graded_at` in the current WIB calendar day, converted to a half-open UTC interval, also ignoring filters. Both are stable while paging and are not tab counts. Objective auto-scores have `graded_by_account_id = NULL` and are therefore never counted as teacher grading.

ATT-204. **Order, pagination, empty.** Order: attempt `submitted_at` descending, then answer id ascending. Page size 20 (R-21); `page` clamps to 1..pages. Nothing pending at all: "Tidak ada esai yang menunggu penilaian." / "Esai yang dikirim siswa akan muncul di sini." Filters active with no match: "Tidak ada esai yang cocok dengan filter." / "Coba kata kunci lain atau tampilkan semua esai." with "Tampilkan semua".

ATT-205. **Waiting label.** `RelativeShort(now - submitted_at)`; when it returns "baru saja", render "Baru dikirim"; otherwise "Menunggu " + the value ("Menunggu 2 jam", "Menunggu 3 hari"). WIB is used for the day boundary only; the label is a duration, not a clock time.

ATT-206. **Row content.** Student name, class label, quiz title, question excerpt (first 120 runes plus "…" when longer), waiting label, and a link to `/teacher/grading/{answerID}`. There is no score column because the row is by definition ungraded. The row is keyed by answer id.

ATT-207. **No graded tab.** Graded essays are not listed, not counted in `Pending`, and have no filter. A graded answer remains reachable by direct URL for read-only review (ATT-214). The current fixture's `graded` tab and 10-per-page size are replaced by this rule (§12 D3).

### 5.13 Grade one essay — read

ATT-211. **`GET /teacher/grading/:answerID`.**

| Field | Rule |
|---|---|
| Actor | the quiz owner teacher |
| Authorization | foreign answer, foreign quiz, or missing → 404 |
| Preconditions | live essay answer; its attempt is submitted and live; registration and quiz live. An objective answer or an in-progress attempt is 404 (not a gradable resource) |
| Input | none |
| Transaction | one read-only transaction for the queue page, the selected detail, and context |
| Effects | none |
| Postconditions | the same template as the queue with `Selected` filled; a graded answer renders read-only |
| Errors | 404 only |
| Redirect | none |
| Provenance | reads `graded_at`, `graded_by_account_id`; never writes |
| Idempotency | GET |

ATT-212. **Detail content.** Question body and image, the student's essay text, attempt header (student, class, quiz, attempt number, submitted time), current score and feedback when graded, and the essay state (`ungraded`/`graded`). The score value is shown at up to one decimal (`85,5`), never rounded to a whole percent. The queue is rendered with the selected row highlighted; `NextHref` is the next ungraded answer in ATT-204 order, or empty when none.

ATT-213. **No suggested score.** `Suggested` is always nil. There is no essay auto-grader, no keyword model, and no model-derived number; showing a suggestion would be an invented rule (ATT-004). The current fixture's fixed `suggestedEssayScore` is demo data and must not ship (§12 D10).

ATT-214. **Read-only graded state.** When `score IS NOT NULL`, the page shows the stored score and feedback, disables the form, and shows "Sudah dinilai". There is no edit or re-grade control (R-07). The answer remains visible so a teacher can audit a score they or a colleague entered; it is never re-queued.

### 5.14 Grade one essay — write

ATT-221. **`POST /teacher/grading/:answerID`.**

| Field | Rule |
|---|---|
| Actor | the quiz owner teacher |
| Authorization | foreign/missing/objective/in-progress → 404 |
| CSRF | required |
| Preconditions | live essay answer of a submitted live attempt; `score IS NULL` |
| Input | `score` (0–100, one decimal, comma or dot) and optional `feedback` |
| Transaction | lock the attempt for update → lock the answer for update → re-check ungraded → update → recompute final (ATT-223) |
| Effects | the answer gets a 0–1 score, feedback, `graded_by_account_id`, `graded_at`; `final_score` is recomputed from all slots |
| Postconditions | the answer leaves the queue; the result page and results row show the score; the student sees it only if `show_score` |
| Errors | 422 validation; 303 conflict when already graded; 404; 500 |
| Redirect | 303 to `/teacher/grading?flash=graded` (key `graded`, "Nilai esai tersimpan.") |
| Provenance | `graded_by_account_id = session teacher`, `graded_at = now`; feedback is stored verbatim |
| Idempotency | first-write-wins: a retry is a conflict, not a silent success (ATT-227) |

ATT-222. **Validation.** `score` is required: trim, accept one optional decimal separator `,` or `.`, at most one digit after it, numeric value 0–100 inclusive. Normalize the comma to a dot, convert `percent × 100` to 4-dp units (`85,5 → 0.8550`), never store 0–100. Empty → field `score` "Nilai wajib diisi." Out of range or malformed → "Nilai harus antara 0 dan 100." `feedback` is trimmed for emptiness, stored verbatim when non-empty, and must be ≤ 1000 characters → "Umpan balik maksimal 1000 karakter." A failure re-renders the detail with 422, the submitted score string and feedback, and the stored values everywhere else; it writes nothing.

ATT-223. **Transaction.** (1) Lock the attempt row; verify submitted, live, owned, and registration live. (2) Lock the answer row; verify live, essay, and `score IS NULL`. (3) `UPDATE attempt_answers SET score=?, feedback=?, graded_by_account_id=?, graded_at=? WHERE id=? AND score IS NULL`; zero rows affected → conflict. (4) Read all live slots' scores and units in the same transaction and compute ATT-143; if every slot is scored, update `quiz_attempts.final_score`; otherwise leave it NULL. (5) Commit. The attempt lock serializes two essays of the same attempt, so they cannot race the final recompute.

ATT-224. **Final-score effect.** The first grade that completes the last ungraded essay sets `final_score` in the same transaction; there is no separate finalize step and no route that sets it directly. A grade on an attempt whose final is already set is impossible (all slots were scored); if it happens, the recompute still runs and is idempotent. Objective slots already carry 0/1 from submit and are not re-evaluated during grading.

ATT-225. **Conflict copy.** Already graded → rollback, 303 to `/teacher/grading/{answerID}` with error flash `already_graded`: "Esai ini sudah dinilai dan tidak dapat diubah." The page then shows the stored score read-only. There is no override path, no "force" parameter, and no second grader identity change (R-07).

ATT-226. **Grading provenance.** `graded_by_account_id` is the acting teacher (INV-03: a teacher account), `graded_at` is server now UTC. The pair is written once. `chk_answers_grading` is satisfied because score and graded_at are both non-null. `feedback` is optional and may be empty; an empty feedback is stored `NULL`, not `""`, so "no feedback" has one representation.

ATT-227. **First-write-wins and retries.** Two concurrent posts: the attempt lock serializes them; the first commits; the second observes a non-null score and takes the conflict path. A network retry after a lost response therefore shows "already graded" rather than silently succeeding; this is deliberate, because a silent second write could mask a different score from another tab. The queue reflects the first commit only.

ATT-228. **Score edge cases.** 100 → 1.0000; 0 → 0.0000 (a valid grade, not "missing"); 85,5 → 0.8550; "85.55" → 422 (two decimals); "-1" → 422; "1e2" → 422; " 85 " → 85. An essay scored 0 still counts as graded and can complete the final score. A blank essay is gradeable like any other (R-07).

### 5.15 Attempt recap

ATT-231. **`GET /teacher/recap/:attemptID`.**

| Field | Rule |
|---|---|
| Actor | the owner teacher of the attempt's quiz |
| Authorization | foreign attempt, missing attempt, foreign/missing quiz, or missing registration → 404 |
| Preconditions | attempt live, registration live, quiz live; the attempt may be in progress or submitted |
| Input | none |
| Transaction | one read-only transaction for the header, all slots, selections, and focus count |
| Effects | none |
| Postconditions | every question is listed in the student's derived order, with key, answer, score when known, and feedback |
| Errors | 404 only |
| Redirect | none |
| Provenance | reads `submitted_at`, `graded_at`, `graded_by_account_id` |
| Idempotency | GET |

ATT-232. **Header.** Student name and class, quiz title, attempt label (ATT-117), start (`created_at`), submitted (`submitted_at` or "Belum dikumpulkan"), score, and focus count. Score: `—`/`Sedang dikerjakan` when unsubmitted; "Menunggu penilaian" when submitted with `final_score NULL`; otherwise the whole percent. The recap always shows the key regardless of `review_mode` (R-18); the mode governs only students.

ATT-233. **Rows.** Position in `DerivedQuestionOrder`, type label, body, optional image, student answer, key, per-question score, feedback. Student answer: selected option bodies in `DerivedOptionOrder` joined with ", ", the essay text, or `—`. Key: correct option bodies in `DerivedOptionOrder` joined with ", ", or "—" for an essay. Per-question score: objective 100 or 0 once submitted, essay whole percent once graded, `—` before submit or while ungraded. Feedback: essay only, when non-empty. No `show_score` gating: the teacher sees the truth.

ATT-234. **In-progress recap.** Available so the monitor can link to a participant who has not submitted. Answers are the currently saved ones; objective scores and verdicts are `—` (no submit score exists); the key is shown because the teacher is the owner; feedback is absent. The header says "Sedang dikerjakan". A later submit changes the same page to scored rows; there is no caching.

ATT-235. **Access and deletion.** A cancelled registration takes its attempts with it (PAR must cascade, INV-18), so the recap becomes 404; the rows are retained but not reachable through this route. A soft-deleted quiz is 404. A teacher who does not own the quiz is 404, never 403 (INV-26). An in-progress attempt whose quiz closed remains reachable (R-10).

ATT-236. **Order and pagination.** The recap is not paginated; question counts are bounded by the quiz. Rows follow the student's display order, so the teacher sees the same numbering the student saw, including shuffled options. There is no "original order" toggle, because the original order is not stored per attempt (R-08).

## 6. Queries and read models

ATT-240. **Shared list rules.** Every list clamps `page` to `1..max(1, ceil(total/perPage))`; an empty list is page 1. Invalid filter, sort, or search values fall back to the documented default and never produce 422 (R-21). Search values are trimmed, lowercased, capped at 100 runes, and matched with escaped `%`/`_`. Pagination links preserve valid filters, omit `page=1`, and never echo a value that was rejected. Empty and filtered-empty are separate states with separate copy (ATT-175, ATT-204). Page sizes: results 10, grading 20; the monitor, recap, attempt, and dashboard are unpaginated, with dashboard caps 5/5/3/3.

ATT-241. **`AnsweredCount(attempt)`.** `COUNT(*)` of live slots where either the question is an essay and `answer_text` is non-null with a non-empty trim, or the question is objective and the slot has at least one live selection. It is one SQL aggregate per render, never a stored counter and never derived from the current page. It is used by the attempt header, the monitor row and rail, and `Progress.PercentAnswered` (floor, clamped). A selection whose option is soft-deleted still counts as a selection for "answered" but cannot be correct at submit, because scoring uses live options only.

ATT-242. **`DisplayPercent(units)`.** `p = floor((units + 50) / 100)`, clamped to 0–100, where `units ∈ [0, 10000]` is the stored 4-dp score. This is half-up at the whole-percent boundary and is the only 0–1 → 0–100 conversion: `0 → 0`, `49 → 0`, `50 → 1`, `8450 → 85`, `8449 → 84`, `9950 → 100`, `10000 → 100`. The tone is `viewmodel.Score.Tone` (≥80 success, ≥60 warning, else danger) and the numeral is always rendered beside the colour. A nil score is `—` or "Menunggu penilaian"; it is never 0.

ATT-243. **`BestScore(quiz, participant)`.** If `quiz.show_score = false`, return nil. Otherwise the maximum stored `final_score` over the participant's live submitted attempts; nil when none. A stored 0 is a score, not nil. Where the caller needs the attempt behind the best score (history badge), ties break by earliest `submitted_at`, then smallest attempt id. PAR's history and dashboard must call this function; they may not compute a second "best" from percentages, which would round before comparing.

ATT-244. **Formatting contract.** One formatter per type, used by every page: percent = whole number plus `%`; average = one decimal, half-up, with a comma (`72,5%`); counts = `Number` with dot thousands (`1.240`); dates = WIB (`23 Okt 2026`), clock = `08.00`, combined = `23 Okt 2026 · 08.00`; missing = `—`; waiting = "Menunggu penilaian"; unsubmitted = "Belum dikumpulkan". Durations never appear. Attempt label = "Percobaan n dari max" or "Percobaan n" when unlimited. A raw 0–1 value and a raw Go time are never passed to a template.

ATT-245. **List matrix.** One row per list, matching the command rules:

| List | Rows | Filters / sort | Page | Empty |
|---|---|---|---|---|
| Dashboard | top scores (≤5), recent (≤5), most/least (3 each) | ATT-162–ATT-164 | — | per card (ATT-162–ATT-164) |
| Results | submitted attempts | state/class/q; submitted desc, id asc | 10 | ATT-175 |
| Grading queue | ungraded essays | quiz/class/q; submitted desc, answer id asc | 20 | ATT-204 |
| Monitor | live registrations | none; state rank, name, registration id | — | ATT-186 |
| Recap | all slots | derived question order | — | n/a |
| Attempt nav | all slots | derived question order | — | n/a |

ATT-246. **Visibility.** A student sees only their own attempt; an anonymous participant sees only the attempt reached through their identity cookie; a teacher sees only owned quizzes. `show_score` and `review_mode` change the student's result and history only; the owner's results, recap, grading detail, and dashboard always show the true score. The monitor and stream never carry scores or answer content. There is no route that returns another participant's answers, and no list is public.

ATT-247. **Read-model provenance.** Results rows read `quiz_attempts` + `registrations` + participant identity; grading rows read `attempt_answers` + `questions` + `quiz_attempts`; the monitor reads registrations + the selected attempt + `focus_loss_events`; the recap reads slots + selections + questions/options. No read model copies a stored aggregate that a write does not maintain: `final_score` is the only stored aggregate and ATT-143/ATT-223 maintain it. Counts and averages are computed per request.

ATT-248. **Search fields.** Results searches only the display name. Grading searches display name, quiz title, and question prompt, because a grader looks for any of the three. Neither searches class labels, because class is a structured filter. Search is case-insensitive under `utf8mb4_unicode_ci`; the escaped wildcard rule is shared. A one-character search is allowed; an empty search is no filter.

ATT-249. **No N+1.** Results and grading load their rows with joins for student, class, quiz title, and question excerpt; the monitor loads all registrations and selected attempts in two queries (rows, focus counts) or one joined query; the recap loads all slots and selections in two queries. A page may not issue one query per row.

## 7. Invariants owned

ATT-INV-01. At most one in-progress attempt exists per registration; start resumes rather than creating a second (INV-13, R-06). A start failure creates nothing.

ATT-INV-02. `attempt_no` is `1 + count(live attempts)` and is unique per registration; attempts are never deleted in v1.

ATT-INV-03. Every live question of the quiz has exactly one live slot in every attempt; no slot exists for a non-question or another quiz's question (INV-12, INV-14).

ATT-INV-04. `shuffle_seed` is written once and never changes; display order, option letters, and navigation are pure functions of it and the immutable item ids (R-08).

ATT-INV-05. An objective score is exactly 0 or 1 and is computed at submit from live selections; an essay score is 0–1 and is written only by a teacher grade (INV-16, R-07).

ATT-INV-06. `final_score` is null until every live slot is scored, then equals ATT-143; it is never stored on 0–100 and never manually overridden (INV-15, R-07).

ATT-INV-07. A graded answer is never re-graded; `score IS NULL` is the write guard and a second grade is a state conflict (first-write-wins).

ATT-INV-08. A submitted attempt accepts no answer write, no score re-computation at submit, and no beacon write; only essay grading may change its `final_score` (R-10).

ATT-INV-09. Review output is a pure function of `review_mode`, `show_score`, grading state, and ownership (INV-30, R-18). Hidden scores are never 0 (INV-17).

ATT-INV-10. The monitor row's identity is `registration_id`; names, initials, and classes are labels (INV-27, R-02). `last_seen` is never null (R-20).

ATT-INV-11. The SSE stream is owner-only, open-only, read-only, and replay-free; no payload contains a score, an email, an account id, or answer content (R-20).

ATT-INV-12. Every score, count, order, and timestamp in this domain is produced by one named function (ATT-019, ATT-004), and no rule depends on a schema delta (R-25).

## 8. Edge cases and failure modes

ATT-250. Stale answer form after submit → ATT-126: 303 to result, error flash `answer_locked`; no write. The student sees the result, not a half-saved answer.

ATT-251. Double-click submit → two 303s, one write, one score computation; the second may land without the flash, which is harmless. A refresh of the POST repeats the idempotent path.

ATT-252. Two tabs saving the same slot → last commit wins; the earlier tab's next GET shows the later value, and its redirect may point to a position that no longer matches what it just saved. No merge is attempted (ATT-127).

ATT-253. Option id from another question or another quiz → 422 "Pilihan jawaban tidak valid. Muat ulang halaman."; the attempt is owned, so this is tampering, not a 404. A question id from another attempt → 404, because it is not a slot of this attempt.

ATT-254. Deleted question or option after publish is impossible (AUT's immutability rule). If the database nevertheless contains one, reads ignore it and a missing slot is 500 data integrity; the page never silently drops a question and renumbers the rest.

ATT-255. Session expires mid-attempt → the POST redirects to login with `session_expired`; the saved answer is whatever was committed before expiry, and the GET after login resumes at the first unanswered position. Anonymous identity cookie cleared → 404 until the participant rejoins; ATT cannot re-identify them (INV-27).

ATT-256. Quiz closes mid-attempt → GET, answer, review, submit, and beacons keep working (R-10); the page shows `quiz_closed_notice`. Start after close → `not_open` unless an in-progress attempt exists, in which case it resumes. No new attempt, registration, or cancellation after close (PAR/AUT).

ATT-257. Registration cancelled while an attempt is in progress → PAR soft-deletes the registration and cascades attempts (INV-18); ATT's next read is 404, the monitor row disappears (ATT-195 snapshot), and the teacher's recap is 404. The score is not recomputed.

ATT-258. Essay score at the boundary → `100` and `0` are valid; `100,0` is valid; `100,1`, `-0,1`, `NaN`, `Inf`, and a blank are 422. A comma and a dot are equivalent; no thousands separator is accepted. `0,0` is a grade and completes the final score.

ATT-259. Final score with one question → that question's score, rounded to 4 dp. With zero questions → NULL, but start forbids it (ATT-106). With essays ungraded → NULL. With objective-only → computed at submit. With one blank objective → that slot is 0 and the mean includes it (R-07).

ATT-260. `max_attempts = 0` with `allow_retake = true` (defensive) → unlimited. `allow_retake = false` with a stored 5 → 1. A participant at the limit sees `attempt_limit`, even if the button was rendered (the server re-checks).

ATT-261. SSE DB error mid-stream → close the response with no event; the browser reconnects and gets a fresh snapshot. Do not send a partial snapshot, a fabricated `quiz.closed`, or an error event (the contract has none).

ATT-262. Beacon clock skew → `at` is ignored; `occurred_at` is server now. A focus event with a future client timestamp is still stored at server now; a progress beacon cannot move `updated_at` backwards.

## 9. Concurrency and idempotency

ATT-270. **Lock map.** Start: registration row for update, quiz row shared. Answer save: attempt row, then slot row. Submit: attempt row, then all slots ordered by id. Grade: attempt row, then answer row. Beacon progress: attempt row via a single update. Beacon focus: no attempt lock (append-only insert). No command locks quiz after attempt or registration after quiz.

ATT-271. **Deadlock avoidance.** The global order is registration → quiz → attempt → slot → selection (ATT-100). The only two-lock command that crosses domains is start (registration then quiz); AUT's close takes quiz only and PAR's cancel takes registration then attempts, so no cycle exists. Start never waits for an attempt row it does not hold; it reads attempts under the registration lock.

ATT-272. **Unique keys as backstops.** `uq_attempt_number` catches a start that bypassed the registration lock; `uq_answers_slot` catches a duplicate slot (never written after start); `uq_selection` is satisfied by replace-all because old rows are soft-deleted with a distinct timestamp. A duplicate-key error is retried only where ATT-108 says so; elsewhere it is 500 with rollback, because it signals a bug, not a user conflict.

ATT-273. **Idempotency table.** Start: same attempt id, no new row. Answer: same content, no write; different content, last commit wins. Submit: first write wins, later calls redirect without writes. Beacon progress: same end state. Beacon focus: not idempotent by design. Grade: first write wins, later calls conflict. All GETs are idempotent.

ATT-274. **Answer vs submit.** Both lock the attempt row first. If submit wins, the answer sees `submitted_at` and returns the conflict redirect; if the answer wins, submit's slot locks see the new selection and score it. There is no window in which a submitted attempt has an unscored objective slot, because scoring and `submitted_at` commit together (ATT-144).

ATT-275. **Grade vs grade.** Two teachers cannot grade the same answer concurrently because ownership is single and the attempt lock serializes; even the same teacher in two tabs produces one success and one `already_graded` conflict. The final score is recomputed from the locked slots, so two different essays of one attempt cannot lose a score.

ATT-276. **Beacon vs submit.** A progress update after submit returns 204 without writing (ATT-160); a focus insert after submit also returns 204 without inserting. A beacon that commits just before submit is preserved; submit does not clear focus events.

ATT-277. **Rate limiter races.** The sliding window is updated atomically per request; two simultaneous focus beacons at the boundary may both pass or one may be limited, but the count is informational and the outcome never affects an answer or a score. The limiter is per attempt, not per IP, so shared school NAT does not limit a whole class.

ATT-278. **SSE isolation.** Each connection owns its ticker and reads in its own snapshot; it never writes and never blocks a command. A slow client cannot slow the quiz, because the server writes to that response only and drops the connection on a write error.

## 10. Cross-domain contracts

ATT-280. **PAR — start handoff.** PAR owns `POST /quiz/:id/attempts` and the registration. It must commit the registration before calling ATT-101, pass only `registration_id` + actor, map ATT's typed errors to the copy in ATT-106/ATT-107, and issue the 303 to `/attempt/{id}`. PAR must never insert `quiz_attempts` or `attempt_answers`, never choose a seed, and never trust a client attempt number. ATT guarantees total idempotency and that a failed start writes no attempt.

ATT-281. **PAR — cancellation.** When PAR cancels a registration it must soft-delete the registration and every attempt, slot, selection, and focus event of that registration in one transaction with one timestamp (INV-18). ATT relies on this: a live attempt under a dead registration is a 500 data-integrity signal, not a page. After cancellation the monitor row disappears (ATT-195) and recap/result are 404.

ATT-282. **PAR — identity labels.** ATT calls PAR's `DisplayName(registration)` and class derivation for the monitor, results, grading, and recap. Names are current, not snapshots (R-25); `registration_id` is the key. ATT never joins `students.full_name` or `anonymous_participants.name` directly, because PAR owns the anonymous display-name rule and the student/anonymous union.

ATT-283. **PAR — history and best score.** `GET /student/history*` must use `BestScore` (ATT-243), the visibility rules (ATT-246), the score labels (ATT-244), and the attempt state derivation (ATT-037). A hidden score renders `—`; a waiting attempt renders "Menunggu penilaian"; an attempt score of 0 renders `0%`. PAR may not compute a best score from rounded percentages or from `show_score = false` rows.

ATT-284. **AUT — publish prerequisites.** ATT assumes every open quiz has at least one live question, dense positions, at least two options for objective questions, exactly one correct option for single, at least two for multiple, none for essay (INV-11), at most 26 options per question so `OptionLetter` covers A–Z, and immutable content/settings after publish (R-01, INV-10). If any is violated, ATT's defensive path is `no_questions` at start or 500 at read; it never renumbers or repairs content.

ATT-285. **AUT — close.** Close sets `state`, `closed_at`, makes the code inert, and ends the monitor (R-10). It must not touch attempts or scores. ATT's stream observes the state change and emits `quiz.closed`; ATT's start resumes an in-progress attempt but refuses a new one; ATT's result, recap, results, and grading remain available.

ATT-286. **AUT — settings.** `review_mode`, `show_score`, `randomize_questions`, `randomize_options`, `allow_retake`, and `max_attempts` are read from the quiz row on every request. They cannot change after publish, so an attempt never sees a different mode between its review and its result. AUT must not "helpfully" rewrite `max_attempts` for an existing quiz.

ATT-287. **IDN — sessions and CSRF.** IDN resolves the student/teacher/anonymous actor, enforces `must_change_password`, issues and verifies the CSRF token, and owns the anonymous identity cookie. ATT uses the resolved actor and re-checks resource ownership against the registration. ATT's 404 policy overrides any instinct to return 403: a foreign attempt, answer, quiz, or stream is 404 (INV-26). The beacon is the only route where ATT accepts a request without a CSRF token, and it enforces same-origin itself (ATT-159).

ATT-288. **OPS — transaction and transport.** OPS supplies REPEATABLE READ, the soft-delete sentinel, cascade stamps, the 404/422/429/500 pages, proxy buffering off for SSE, and the limiter's process model. ATT specifies the locks, statements, statuses, and copy; OPS implements them and must not add a 409, a hard delete, or an audit table. The rate limiter may be in-process for the current single binary; a multi-instance deployment needs a shared window, which OPS owns.

ATT-289. **No telemetry export.** Focus events are read only by the monitor and recap. There is no analytics endpoint, export job, or third-party beacon in this domain; a future export is a new rule, not an inference from `focus_loss_events`.

## 11. Traceability

| Route / action | Command | Invariants | Acceptance |
|---|---|---|---|
| ATT-101 start rule (called by `POST /quiz/:id/attempts`) | ATT-101–ATT-110 | ATT-INV-01–04, INV-13 | AT-01–AT-03 |
| `GET /attempt/:id` | ATT-111–ATT-119 | ATT-INV-03, 04, 09 | AT-04, AT-05 |
| `POST /attempt/:id/answer` | ATT-121–ATT-127 | ATT-INV-03, 05 | AT-06–AT-08 |
| `GET /attempt/:id/review` | ATT-131–ATT-135 | ATT-INV-09 | AT-09 |
| `POST /attempt/:id/submit` | ATT-141–ATT-147 | ATT-INV-05, 06, 08 | AT-10–AT-13 |
| `GET /attempt/:id/result` | ATT-151–ATT-155 | ATT-INV-06, 09 | AT-14–AT-16 |
| `POST /attempt/:id/beacon` | ATT-156–ATT-160 | ATT-INV-08, 10 | AT-17–AT-19 |
| `GET /teacher` | ATT-161–ATT-166 | ATT-INV-10, 12 | AT-20, AT-21 |
| `GET /teacher/quizzes/:id/results` | ATT-171–ATT-178 | ATT-INV-06, 09 | AT-22, AT-23 |
| `GET /teacher/quizzes/:id/monitor` | ATT-181–ATT-187 | ATT-INV-10 | AT-24, AT-25 |
| `GET /teacher/quizzes/:id/stream` | ATT-191–ATT-198 | ATT-INV-10, 11 | AT-26–AT-28 |
| `GET /teacher/grading` | ATT-201–ATT-207 | ATT-INV-07 | AT-29 |
| `GET /teacher/grading/:answerID` | ATT-211–ATT-214 | ATT-INV-07, 09 | AT-30 |
| `POST /teacher/grading/:answerID` | ATT-221–ATT-228 | ATT-INV-05–07 | AT-31–AT-33 |
| `GET /teacher/recap/:attemptID` | ATT-231–ATT-236 | ATT-INV-09 | AT-34, AT-35 |

ATT-290. **Cross-cutting tests.** AT-36 pins determinism, AT-37 pins copy, AT-38 pins the no-schema-delta rule, and AT-39 pins the 404-vs-403 policy. Every command row above has at least one executable acceptance criterion; the traceability table is the checklist for the conformance suite.

## 12. Open questions and schema deltas

ATT-291. **Numbering and schema deltas.** The spawn brief called this section §11; contract §7 mandates open questions as §12 and traceability as §11, and this document follows the contract. No schema delta is required: ATT needs no column, table, index, or constraint beyond migrations 0003–0005. `graded_by_account_id = NULL` already means "system-graded", so no auto-graded flag is needed, and no `status`, `started_at`, `last_seen`, `option_order`, `position` on answers, `counted`, `focus_loss_count`, or `points` is assumed (R-25, INV-25).

ATT-293. **D1 — client autosave.** `static/js/app.js` still contains a 450 ms autosave block, which R-09 forbids. Recommendation: delete it; keep the form POST and the before-unload warning; narrow the conformance rule to allow `EventSource` only on the stream path and `navigator.sendBeacon` only on the beacon path. Impact: client only; no server rule.

ATT-294. **D2 — results tabs.** The fixture exposes `in_progress|completed` and includes in-progress rows. Recommendation: implement `waiting|graded`, exclude in-progress from rows, keep the two header metrics (ATT-171–ATT-178). Impact: results page and fixture; monitor remains the live view.

ATT-295. **D3 — grading queue.** The fixture has a `graded` tab and 10 rows per page. Recommendation: ungraded-only, 20 per page, graded answers reachable only by direct read-only URL (ATT-201–ATT-214). Impact: queue template, fixture, and page size.

ATT-296. **D4 — monitor columns and counts.** The fixture shows score and start time and sets a completed row's answered count to the total. Recommendation: drop both columns, use ATT-241 for every state, and show `last_seen` (ATT-182–ATT-185). Impact: monitor template and fixture; SSE cannot keep dropped fields truthful.

ATT-297. **D5 — result review link.** The result template renders `ReviewHref` to the pre-submit review, which redirects back to the result, a loop. Recommendation: `ReviewHref` is empty on the result page and the review route redirects to the result when submitted (ATT-131, ATT-155). Impact: template only.

ATT-298. **D6 — 422 on answer POST.** The route is registered with an `Action` (PRG only), so it cannot re-render a 422. Recommendation: convert it to a full handler that validates, writes, or re-renders the question page with `FieldErrors` (ATT-122, ATT-126). Impact: route registration and handler; no schema.

ATT-299. **D7 — score and average rounding.** `viewmodel.NewAverage` uses `%.1f`, which rounds ties to even (`84,25 → 84,2`), while ATT-244 requires half-up (`84,25 → 84,3`). `NewScoreFromRatio` multiplies a `float64`, so `0.8450` can compute as 84 rather than 85. Recommendation: format both from integer units per ATT-242 and ATT-174. Impact: display only; stored scores unchanged.

ATT-300. **D8 — SSE row key.** Frontend §19.1's `participant.updated` has no `registration_id`, so the only key would be the name, which R-02 forbids. Recommendation: include `registration_id` in `added` and `updated` payloads (ATT-193, ATT-194). Impact: payload extension, not a schema change.

ATT-301. **D9 — beacon position.** Frontend §19.2 says progress stores the current position, but the schema has no current-question column and R-25 says not to display one. Recommendation: accept and ignore `position`/`question_id`; progress only touches `updated_at` (ATT-157). Impact: beacon contract only.

ATT-302. **D10 — suggested essay score.** The fixture supplies a fixed `suggestedEssayScore`. Recommendation: `Suggested` is always nil; there is no essay auto-grader (ATT-213). Impact: grading fixture and placeholder; no rule.

ATT-303. **Q1 — most/least joined and drafts.** Should drafts appear? Recommendation: exclude them (ATT-164), because a draft cannot be joined and would occupy every "least joined" slot. Impact: if product wants drafts, the least-joined card becomes the three newest drafts; no scoring changes.

ATT-304. **Q2 — option count.** AUT has no explicit cap. Recommendation: cap the form at 10 options and hard-reject >26 at publish, so A–Z is total (ATT-284). Impact: authoring validation; ATT needs no change.

ATT-305. **Q3 — unlimited attempt label.** Recommendation: "Percobaan n" when ATT-102 is unlimited, "Percobaan n dari max" otherwise (ATT-117). Impact: copy only; `max_attempts = 0` is defensive and should not be writable.

ATT-306. **Q4 — duplicate focus events.** Recommendation: accept the over-count; focus is informational and never blocks (ATT-160). A dedupe key would need a schema change, which R-25 forbids; client-side debouncing is the only mitigation.

ATT-307. **Q5 — results grouping.** Frontend §13.5 says "grouped by student" and "newest submission first". Recommendation: flat list ordered by submission (ATT-176); a grouped UI would need a second sort key and would violate the stated order. Impact: presentation only.

ATT-308. **Q6 — teacher average and hidden scores.** Recommendation: include `show_score = false` attempts in the teacher's average (ATT-174); the flag is student-facing. If product disagrees, only ATT-174 changes; no stored value or student page changes.

## 13. Acceptance criteria

ATT-310. Each criterion is executable server-side with a fixed clock and seeded rows; "renders" means the response body and status, not a browser assertion. Scores in examples are exact, not illustrative.

AT-01. Start: a live registration for an open quiz with 3 questions → 303 `/attempt/{id}`; exactly one attempt with `attempt_no = 1`, a non-null seed, 3 slots, and 0 selections; a second POST returns the same id and the row count is unchanged.

AT-02. Resume after close: an in-progress attempt on a quiz that is then closed → POST start returns the same attempt id, adds no row, and `GET /attempt/{id}` renders `quiz_closed_notice` and remains answerable.

AT-03. Quota: `allow_retake = false`, one submitted attempt → start returns `attempt_limit` ("Batas percobaan untuk kuis ini sudah tercapai."), writes no attempt, and leaves the registration live. The same holds at `max_attempts = 2` after two rows.

AT-04. Shuffle: with `randomize_questions` on, seed 42, and question ids 3, 7, 9, two calls return the same order; with the toggle off the order is 1, 2, 3 by position; a reload, resume, and second device return the identical order and identical option letters.

AT-05. Attempt GET: `?q=999` clamps to the last position, `?q=0` opens the first unanswered position, a submitted attempt returns 303 to the result, and the HTML contains no key, score, verdict, or `is_correct` value.

AT-06. Answer single: posting one valid option id creates exactly one live selection, sets `answer_text` to NULL, and moves `attempt.updated_at`; reposting the same option creates no new row and does not move `updated_at`; posting no option soft-deletes the selection.

AT-07. Answer multiple: selecting 2 of 3 options stores exactly 2 live selections; posting a duplicate id stores 1; posting an option from another question returns 422 "Pilihan jawaban tidak valid. Muat ulang halaman." and writes nothing.

AT-08. Answer after submit: a POST to a submitted attempt returns 303 to the result with `answer_locked` ("Jawaban tidak dapat diubah setelah dikirim.") and the stored selections are byte-identical.

AT-09. Review: 3 questions with 2 answered renders all 3, the unanswered line "Belum dijawab: 3.", the submit form, and no key/score/verdict; changing `review_mode` does not change this page.

AT-10. Submit objective: scores 1, 0, 0 over 3 questions → every objective slot has `graded_at = submitted_at` and `graded_by_account_id = NULL`, `final_score = 0.3333`, the result shows 33%, and the redirect carries `flash=submitted`.

AT-11. Submit mixed: 4 questions scored 1, 0, 0, and one ungraded essay → `final_score IS NULL`, the result shows "Menunggu penilaian", and no percentage is rendered.

AT-12. Grade completes final: grading the essay in AT-11 at 50 → stored `0.5000`; the final becomes `0.3750` (mean of 1, 0, 0, 0.5) and the result shows 38% when `show_score` is true.

AT-13. Double submit: two POSTs in parallel → one `submitted_at`, one set of scores, both responses 303 to the result; the second produces no flash and no additional write.

AT-14. Result access: the owner student gets 200; the owner teacher, another student, and an unknown id get 404; an anonymous owner gets 200 and no "Riwayat kuis" link.

AT-15. Review matrix: `questions_and_key` renders body, answer, key, verdict, and a graded essay's score/feedback; `key_only` renders the objective key list and no essay row; `questions_only` renders body/answer and no key; `answers_only` is identical to `questions_only` with no verdict badge and no verdict colour.

AT-16. Hidden and waiting scores: `show_score = false` removes the score tile and every essay numeral while keeping permitted verdicts; a hidden score renders `—` where a cell exists and never `0%`; a waiting attempt renders "Menunggu penilaian".

AT-17. Best score: attempts with finals 0.8000 and 0.6000 → 80%; a final of 0.0000 → 0%; `show_score = false` → nil (`—`), never the numeric best.

AT-18. Beacon: `text/plain` body `progress` → 204 and `updated_at` moves; the 31st progress request in a minute → 429 with `Retry-After: 60`; `blur` inserts exactly one event and does not move `updated_at`; any beacon after submit → 204 with no row and no timestamp change.

AT-19. Beacon security: no `Origin` → 403; a foreign attempt → 404; a request with no CSRF token but a valid same-origin `Origin` → 204; a malformed kind → 400.

AT-20. Dashboard top scores: a quiz with `show_score = false` is absent, never shown as 0; a tie at 0.8500 chooses the earlier `submitted_at`, then the smaller attempt id; at most 5 quizzes; a quiz with only in-progress attempts is absent.

AT-21. Dashboard joined: counts are distinct live registrations; a title tie breaks `LOWER(title)` then id; drafts are absent from most/least; each list has at most 3; a quiz may appear in both when fewer than six candidates exist.

AT-22. Results filters: 2 waiting + 1 graded submitted attempts → tab counts 3/2/1 for all/waiting/graded; `class=XII` keeps only that grade; `q` matches a name substring; `page=99` clamps; page size is 10; in-progress attempts are not rows.

AT-23. Results average: finals 0.8000 and 0.6500 → "72,5%"; one attempt at 0.7250 → "72,5%"; no graded attempts → `—`; `show_score = false` does not remove the teacher's average.

AT-24. Monitor counts: one registered, one in-progress with 2 answered, one completed with 1 blank → completed row answered = 2 (not 3), progress answered = 4, total = 9, and every `last_seen` is non-null.

AT-25. Monitor states: an owned closed quiz returns 200 with "Kuis tidak sedang berlangsung" and a results link; a foreign quiz returns 404; an owned open quiz with zero registrations returns the "Belum ada peserta" state.

AT-26. SSE snapshot: the first event is `snapshot`; its JSON has exactly `quiz`, `progress`, `participants`, `generated_at`; a participant's `attempt_id` is null before start; no participant object has a score or answer field; `total` equals registered × question count.

AT-27. SSE diff: a new registration emits `participant.added` with the full row; an answer save emits `participant.updated` with all seven fields; closing the quiz emits `quiz.closed` then EOF; the stream contains no `id:` or `retry:` line.

AT-28. SSE reconnect: disconnect and reconnect → a fresh `snapshot`, no replay of missed events; sending `Last-Event-ID` changes nothing; a mid-stream registration cancellation produces a fresh `snapshot`.

AT-29. Grading queue: 3 ungraded essays → 3 rows, newest submission first, answer id ascending on ties; page size 20; no graded tab; `Pending` ignores filters; `GradedToday` counts only this teacher's grades in the WIB day; objective answers never appear.

AT-30. Grade read: an objective answer → 404; an in-progress essay → 404; a graded essay → 200 with the stored score, read-only form, and "Sudah dinilai"; `Suggested` is nil.

AT-31. Grade write: posting `85,5` stores `0.8550`, sets `graded_by_account_id` to the teacher and `graded_at` to now, recomputes the final if it completes the set, and redirects 303 to the queue with `flash=graded`.

AT-32. First write wins: a second POST for the same answer with `90` returns 303 to the detail with `already_graded`, keeps `0.8550`, and leaves `graded_by_account_id` unchanged.

AT-33. Grade validation: blank, `101`, `85.55`, `NaN`, and a 1001-character feedback each return 422, preserve the entered values, and write nothing; `0` is accepted and stored `0.0000`.

AT-34. Recap: an in-progress attempt renders saved answers, the key, no per-question scores, and "Sedang dikerjakan"; a submitted attempt renders 100/0 for objective rows, the essay score when graded, and "Menunggu penilaian" otherwise; the key is present in every mode.

AT-35. Recap order: with shuffle on, the recap positions match the student's rendered positions; the recap is unpaginated; a foreign attempt returns 404.

AT-36. Determinism: with a frozen clock and identical rows, two calls to each list (results, grading, monitor, dashboard) produce byte-identical order and content; no tie is left to the database's default order.

AT-37. Copy: every new key in this document matches exactly, including `answer_locked`, `attempt_limit`, `no_questions`, `not_submitted`, `already_graded`, the field errors, and the two empty-state pairs; no bare TBD remains.

AT-38. No schema delta: `migrations/0003`–`0005` are unchanged; a grep of ATT rules finds no column outside those files; `graded_by_account_id = NULL` is the only system-grader marker.

AT-39. 404 policy: every foreign or soft-deleted attempt, answer, quiz, stream, and recap returns 404, never 403; the only 403s in this domain are CSRF failure and beacon same-origin failure (and the owner's `must_change_password` stream gate).

ATT-311. **Definition of done.** This file owns all 15 ATT routes, the start rule, every score/order/count derivation, and every copy string it introduces. It cites R-01–R-25 and INV-01–INV-30 where they bind; it introduces no schema change; and every open item carries a recommended default and an impact, so an implementer never has to ask a question.

