# Fallback Strategy — quiz-v2

Status: DRAFT | Owner: 03-RESILIENCE | Updated: 2026-10-14 (F-38 closed: dangling "per §2.8" anchor replaced with a self-contained validated-requirement rule)

Each dependency/operation states purpose, failure modes, timeout/cancel
budget, retry rule with justification, idempotency basis, degraded behaviour
with correctness limits, recovery signal, and linked IDs. No decorative
retries: every retry below is justified by safety analysis.

## F-001 — MariaDB 12 (system of record)

- **Purpose:** all persisted state. **Failure modes:** down, slow, deadlock,
  disk full. **Timeout (DECIDED):** query 5 s, transaction 10 s (statement
  timeout set per connection).
- **Retry:** NO automatic retry of committed-unknown transactions. Read-only
  GETs retry once after 500 ms. Mutations return ERR-501 with "answers safe"
  messaging; client holds state and the USER retries explicitly (button).
  Justification: blind retry of submit/grade could double-apply without the
  idempotency key path; explicit user retry + key is provably safe.
- **Offline answers (DECIDED DEC-090 R8, banner rule DEC-124):** answers that never reach the
  endpoint (offline queue) are DROPPED, never auto-flushed and never manually
  retried. Only the saved-tick (post-200 toast) is guaranteed committed; anything
  without a saved-tick MUST be treated as unsent. The offline banner is generic
  (no per-question exact list): it references the generic banner text and the
  saved-tick rule, e.g. un-ticked answers are lost. No
  answer-overflow column exists (only `cheat_overflow` for the cheat beacon);
  a queued answer that would be ERR-205/206/302 at flush time is therefore
  never silently applied. What survives offline is only what the server
  already committed.
- **Idempotency:** start/submit keyed (API-033/038); cheat keyed
  (attempt,client_event); grades are overwrites; control verbs versioned.
- **Degraded:** read-only? NO degraded grading — without DB the server
  returns 503; already-rendered pages keep client timers from last sync but
  block new saves with a banner. Correctness limit: zero writes accepted,
  explicitly.
- **Recovery (refresh-only clearing, DEC-102 + N12):** the offline banner
  clears ONLY on a user-initiated page refresh that re-renders from server
  truth — there is no health-gate probe sequence and no "3 consecutive probes"
  rule (that phrase was undefined; DEC-102 forbids polling of any kind, so a
  client banner cannot clear on server probes). On refresh the server returns
  the current state; if the DB is still unavailable the page renders the
  degraded read-only view with the banner still shown. Students resume from
  server `current_index`; no merge conflicts by design (server position wins).

## F-002 — SSE streams (live push)

- **Purpose:** start/pause/stop fan-out, monitor, cheat badges, heartbeats.
- **Failure modes:** edge connection limits (with NGINX removed per ARCH-009 the
  stream now ends at a Cloudflare connection the operator does not control —
  accepted risk, user decision 2026-10-14), buffering middleboxes,
  mobile network drop, sleep/wake. **Timeout:** heartbeat 30 s; client
  declares dead after 45 s silence.
- **Retry:** NONE — no auto-reconnect, no polling of any kind (DECIDED
  DEC-102, R10 2026-10-12). When the client declares the stream dead it calls
  `EventSource.close()` and shows a persistent notice; the ONLY recovery path
  is a user page refresh. Justification: polling invents a second data path
  with its own staleness lies; a single honest notice + server-truth refresh
  is simpler and provably fair (all guards stay server-side).
- **Notice (DECIDED DEC-102):** persistent inline `alert-warning` (never a
  toast — blocking states use `alert`, C-010) with the exact Indonesian text:
  "Koneksi live terputus — jawaban tersimpan aman. Refresh halaman untuk
  menyambung ulang." The "jawaban tersimpan aman" clause is true because only
  server-committed answers count (DEC-090); refresh is stated as the sole
  recovery so the user never waits for something that will not come.
- **Honest degradation (DECIDED DEC-102):** a student whose stream is dead
  sees NO pause/stop/monitor update until refresh — the spec asserts no
  "instant" claim in that window. Countdown FREEZE + banner (DEC-125): on
  stream-dead the countdown FREEZES at the last server sync (freeze, DEC-125) with the persistent
  notice above; it never invents ticks while disconnected. Graded results surface
  on next render (next-render, DEC-104, STATE/AC ref only): a grade committed while the stream
  is dead appears on the next full render, never via the dead stream. Fragment-wins
  error note (fragment-wins, DEC-122, STATE/AC ref only): list XHR failures keep the last rendered
  fragment with an inline error, never a blank list. Fairness is preserved because guards are
  server-side: answers sent while paused are still rejected ERR-302, locked
  questions ERR-205, past-deadline saves ERR-206, late submits resolve to the
  auto-submitted receipt (DEC-086). Assumption A1 records the SSE-friendly
  expectation; with NGINX gone (ARCH-009) there is no `proxy_buffering off` to
  misconfigure — an edge that drops the long-lived connection now yields notice
  + refresh, never polling.
- **Recovery:** the user refreshes the page → full SSR re-render from server
  truth (position, deadline, session); conflicts resolve server-wins. There is
  no `Last-Event-ID` resume across refresh and no replay buffer to consume —
  refresh IS the state GET.

## F-003 — Timer goroutines (in-process)

- **Purpose:** per-question lock + total auto-submit + pause shifting.
- **Failure modes:** process restart loses handles; goroutine leak on missed
  cleanup. **Mitigation (not retry):** deadlines persisted at start; on boot,
  recovery sweep loads all `answering` timed attempts/sessions and respawns
  timers (deadlines already past → immediate lock/submit in a bounded batch,
  100 per cycle DECIDED, cadence every 5 s while past-due rows remain —
  past-due first, so a 150-timer backlog drains in 2 cycles, ≤10 s). Paused
  sessions (`session_state=paused`, BR-013) are SKIPPED by the sweep — their
  occupants' timers were frozen at pause; on resume the server shifts deadlines
  by the full paused duration (inclusive of any restart downtime inside the
  pause), so a restart-while-paused can never auto-submit a frozen occupant.
  Goroutine registry with explicit release on
  submit/stop/finish; pprof-gated leak test in CI.
- **Correctness limit:** during the restart window, expiry may fire late by
  restart duration; late POSTs are still judged against persisted deadlines
  (fair), and auto-submit covers the gap.

## F-004 — Image intake (bounded in-process background workers)

- **Purpose:** enforce BR-010 (validate synchronously, compress/convert >10 MB
  asynchronously). **Design (DECIDED DEC-021):** bounded pool — 2 workers,
  queue 100, per-item timeout 10 min; pool created at boot, drained on
  graceful shutdown. Intake validates format/count/MIME synchronously and
  returns at once with `image_status=processing`; the quiz stays openable and
  answerable with a placeholder until `ready`. Intake checks queue depth under
  the pool semaphore BEFORE writing the question row, so concurrent fills
  cannot overshoot 100. Shutdown drain waits up to 60 s; un-drained jobs stay
  `processing` for the boot sweep. Boot order: re-queue `processing` rows
  first, then sweep temp files older than 24 h (plus unreferenced temp files —
  temp referenced by a `processing` row is NEVER swept regardless of age).
  Final-file orphans (crash between commit and old-file delete) are swept by an
  ops query comparing disk vs DB references (no auto-delete). `processing` →
  `ready`/`failed` transitions have no push — the editor re-reads on reload
  (documented).
- **Retry:** queue-full → 429 ERR-112, user retries later (answer flow
  unaffected). Job fail (unreadable/corrupt/timeout) → row `failed` with
  reason, teacher re-uploads. No server retry of failed jobs (deterministic
  outcome, CPU-bound). **Degraded:** `processing`/`failed` never blocks
  authoring, opening, answering, or grading (image optional by design).
- **Recovery:** restart drops queued/in-flight jobs; rows stuck in
  `processing` are re-queued by the boot sweep (idempotent: re-compress from
  the temp path, promote on success). Temp files older than 24 h are swept.
- **Session-state note (STATE/AC ref only):** intake is independent of matching
  session state — waiting echo (DEC-126, see EDGE-072) and finished-permanent
  (see EDGE-073) do not change intake, queue-full, or `failed` behaviour.

## F-005 — Cheat beacon (best-effort)

- **Purpose:** focus-loss evidence. **Failure modes:** beacon dropped
  (pagehide race), offline. **Retry:** exactly once via NATIVE fetch keepalive
  (never jQuery — no beacon transport, UI-002); beacon stays native `sendBeacon`,
  CSRF-exempt (DEC-108, API-03A only — never 403 ERR-002 for missing token) and
  jQuery-unbounded (DEC-114: beacon never goes through the jQuery AJAX path);
  then drop. Justification: over-retrying inflates counts; under-counting is
  the documented, teacher-visible limitation (recap shows `cheat_count` +
  `cheat_overflow` + `last_cheat_at`, API-062). Submit NEVER depends on beacon success.
- **Degraded:** offline cheat events queue in memory (max 20) and flush on
  the next beacon POST that reaches the server (next focus-loss event or
  pagehide); a page refresh before flush drops the queue (memory-only,
  documented undercount consistent with DEC-090 honesty); overflow drops
  oldest with a client flag `cheat_overflow=true`
  shown to the teacher as "≥N".

## F-006 — Submit path (the one operation that must not lose data)

- **Timeout:** 15 s end-to-end (transaction ≤10 s). **Retry:** client retries
  with the SAME server-generated `Idempotency-Key` (DEC-084: rendered into the
  form at GET time, returned verbatim) on timeout/5xx (max 3, backoff 1/2/4 s);
  server dedupes to a single submission. Crash between commit and response →
  retry returns the committed receipt (key lookup), never a duplicate row.
  Key survival across browser close: the key lives on the `attempts` row
  (set at start); after relogin the server re-issues the same key for the
  taker's open row, so the 24 h replay window (API-000) holds across process
  death. Echoed receipts reuse the ORIGINAL `request_id` (byte-identical
  receipt, AC-110). Key lifetime is tied to the attempt, not the wall clock:
  the 24 h replay window runs from the attempt reaching terminal (submit), so
  a 24 h-max timer (BR-003) + pause shifts can never strand a live attempt
  with an expired key; after the window the key is inert — a retry becomes a
  normal state-checked submit (echo if submitted, guard error otherwise). The
  `UNIQUE(idempotency_key)` row persists as the dedupe record (never reused
  for a new attempt). Post-exhaustion (3 keyed retries failed): the client
  surfaces 500 ERR-500 with `request_id`; a later retry still resolves via
  key lookup to the committed receipt with the ORIGINAL `request_id`.
- **Degraded:** if the browser is closed mid-submit-unknown, relogin shows
  "submission uncertain — checking…" then resolves to submitted or resumes
  answering based on server truth. No user-visible fork.
- **Session-state note (STATE/AC ref only):** finished permanent (see EDGE-073)
  — a submit against a `finished` session resolves via normal state checks
  (echo if submitted, guard error otherwise), never a reopen.

## F-007 — Matching control verbs

- **Timeout:** 10 s (transaction + fan-out). **Retry:** teacher clicks again
  (idempotent verbs; version conflict → ERR-300 echo + re-sync, not a blind
  state flip). Same-verb re-click within 10 s carrying a STALE version →
  429 ERR-116 (DEC-085 guard, refined R8); a re-read + re-POST with the current
  version → 200 echo, never 429. Retry-after-failure (503/409 — never accepted,
  no guard timestamp) re-reads state first and follows echo-vs-409, never 429. Waiting echo (DEC-126, STATE/AC ref only): pause/stop/resume on a `waiting` session = 200 echo of `waiting`, never 409 ERR-300 / 429 ERR-116. Finished permanent (STATE/AC ref only): `finished` is terminal — no verb reopens it (see EDGE-073). Leave broadcast (STATE/AC ref only): occupant leave via the leave endpoint broadcasts presence to the roster (see EDGE-071). **DB-down during a control verb:** the version check cannot run —
  the txn fails closed with 503 ERR-501 (no state change, no fan-out); the
  teacher retry follows the same re-read rule above. **Degraded:** if SSE fan-out partially fails, stragglers
  see NO update until they refresh (F-002, DEC-102 — no reconnect, no poll);
  session truth stays in DB, so no split
  brain: a student who missed `pause` still gets ERR-302 on answer attempts.

## What is deliberately absent

No circuit breakers (single DB, no downstream calls), no queues/dead-letter
(single process), no multi-region failover, no offline answering mode
(answers require server timestamps for fairness). Each absence is justified by
MVP scope; adding any of them requires a documented, user-validated requirement
recorded as a new DEC entry — none exists today, so none may be added.

Metrics/logs/alerts per fallback: `db_latency_ms`, `sse_dead_total`,
`submit_retry_total`, `timer_respawn_total`, `cheat_dropped_total`,
`image_worker_queue_depth`, `image_job_total{result}` — defined
in `../05-DEVOPS-AND-INFRA/03-LOGGING-SECURITY-OBSERVABILITY.md`.
