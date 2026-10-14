# Logging, Security, Observability — quiz-v2

Status: DRAFT | Owner: 05-DEVOPS | Updated: 2026-10-14

Deferral note (Q11, user decision 2026-10-14): every alert threshold, TLS
version/cipher/HSTS/renewal specific, SLO percentage, response-header value
and host-monitoring detail referenced below is deliberately deferred to the
operator at deploy time. They are consolidated once in the deferral register at
`01-INFRA-TOPOLOGY.md` §6 — this file does not repeat the list.

## 1. Logging (structured JSON to stdout)

Every line: `{ts, level, request_id, actor_id?, role?, route, method,
status, latency_ms, quiz_id?, attempt_id?, msg, err_code?}`. Levels: DEBUG
(dev only), INFO (requests 4xx incl.), WARN (auth failures, timer respawn,
SSE dead-stream notice bursts), ERROR (5xx, txn failures, migration/seed issues).
Correlation: `X-Request-ID` (generate if absent) propagated to error
responses and audit rows. Diagnostic context: include ids + latency +
`sql_state` on DB errors; NEVER log passwords, hashes, session ids, CSRF
tokens, attempt tokens, image bytes, or essay full-text at INFO (essay
excerpts only at DEBUG with explicit flag `DEBUG_ESSAY_LOG`, default off —
the flag must be set for essay excerpts to appear; without it, essay text is
omitted at every level).

Retention/access (DECIDED DEC-070, revised R8 per user: persistent logging is
unnecessary): app logs go to stdout only — no log files are written or
retained by this spec (the host/Docker may capture stdout per operator
setup, outside this spec's promises). Exam-period bump to WARN+ bounds
stdout volume. What persists in the DB indefinitely: attempts, grades,
`audit_log` rows (small, legal/banding need). DB disk-full stays an F-001
failure mode with a manual DB purge procedure (operator-owned, outside MVP);
the alert THRESHOLD that fires first is deploy-time config per DEC-071 —
operator-owned, at deploy time (consolidated with every other threshold in the
deferral register, INFRA §6).

## 2. Sensitive-data redaction

Middleware redactor strips `password, password_hash, session_id,
csrf_token, attempt_token, cookie` keys from any logged struct (allow-list
fields instead of block-list where feasible). Audit log stores actor +
entity ids, never secrets. Actor emails and client IPs appear in audit rows
ONLY (needed for manual abuse review under DEC-073); stdout INFO lines carry
ids, never emails. Recap/monitor payloads never include other
students' answers or key material to students (RBAC R1). Error responses carry `request_id` only.

## 3. Metrics and health (no invented SLOs)

`GET /metrics` is a DEPLOYED ENDPOINT of the app container (it is part of the
topology — see INFRA §1 diagram + §5) exposing counters + histograms as text:
`http_requests_total{route,status}`, `http_latency_ms`,
`db_latency_ms`, `sse_connections`, `sse_dead_total`,
`submit_retry_total`, `auto_submit_total`, `timer_respawn_total`,
`cheat_events_total`, `cheat_dropped_total`, `grading_pending`,
`image_worker_queue_depth`, `image_job_total{result}`.
Exposure: internal-only — the endpoint is bound to the container/Compose
network and is never published to the internet (with NGINX gone per ARCH-009,
the host firewall and the internal bind are the only barrier); the exact scrape address,
scrape interval and the collector/dashboard are operator-runbook items (see
the deferral register in INFRA §6). This is F-08: the series exist and are
exposed by the app, but the pipeline and the topology do not assert a scrape
or a dashboard, because those are operator-owned.

Health: `/healthz` (process up) vs `/readyz` (migrated + DB ping + volume
writable) — proxy/deploy gates use `/readyz`. (A status page is NOT asserted
here: no `/status` route exists in the app and this spec does not create one —
public status pages are out of spec / operator-runbook, F-07.)
This §3 is the **authoritative readiness contract** (N7): `/healthz` is liveness
only (Docker healthcheck signal — process up, never a deploy/proxy/smoke gate)
and `/readyz` is the deploy, proxy, and CI smoke gate (503 until migrated + DB
ping + volume writable). INFRA §5 and CICD §1/§3 consume this contract and must
never redefine it; any divergence between this file and the infra/CI wording is
a defect. Consumers and their references (F-63 — the API spec carries no
operation entries for these three, so this file is the only contract and it is
cited infra-wide): the Docker healthcheck consumes `/healthz` (INFRA §1 app
row); the proxy and the deploy/smoke path consume `/readyz` (INFRA §3/§5,
CICD §1 Smoke row); the operator's collector consumes `/metrics` (INFRA §1
diagram, §5). Adding an API-spec operation entry for any of the three is out
of scope for this folder and is not done here.

Dashboards/alerts (all operator-owned, F-08/F-11): the app may raise an
ERROR-rate spike signal, and HOST-level (disk, mem) signals belong to the host
monitor, not to the app. All host detail (disk/mem collection, agent, retention)
is deliberately outside this spec — the app owns its own signals and the host
collector is an operator-runbook item. Disk-full is an F-001 failure mode;
every threshold (disk, mem, app ERROR-rate) and its accumulation window is
deploy-time config per DEC-071 — the operator sets and owns them, and no
value, percentage, or window is asserted here. Owner: operator, at deploy time;
the consolidated list is INFRA §6.

## 4. Audit events (business-critical, append-only)

Authority (F-59): **this §4 is the authoritative audit-event list.** The
`audit_log` record list in ERD §3 is a short summary of the historical subset
(grade/re-grade, deactivate/delete, open/close, visibility/token toggles,
question mutations pre-start, matching start/pause/stop/finish); where the two
disagree, this list governs. The additions below are events the behaviour
specs require to be auditable (DEC-105/106/109/116/121/129); they map onto the
same `audit_log` columns, so none needs a schema change — the ERD column set
(`action`, `entity`, `old_value`/`new_value`, `ip`, `created_at`) already holds
them.

Recorded: login failures (WARN), forced-rotation completion, user
create/deactivate/delete, self-delete (INFO, DEC-129), blocked deactivates (INFO, ERR-115 — incl. live-deactivate blocked self/other deactivate on a live matching session, DEC-105) and unblocked
deactivates, owned-quiz auto-closes on deactivate (INFO, DEC-092),
authorization denials (403 + 404-per-BR-007, INFO, actor+resource+action — the RBAC §4 "all denials are audit-logged" rule; incl. teacher-403 teacher-to-teacher 403, DEC-109),
anon active-conflict 409 echoes (anon-conflict, INFO, DEC-106),
quiz open/close/visibility/token toggles, empty-room aborts (`waiting → finished`,
DEC-093) and stop-first 409s, exhaustion auto-cancels (INFO, DEC-083),
stranded-row 429 guard hits (INFO, ERR-116 — guard, not abuse),
question mutations pre-start, matching start/pause/stop/finish, matching leave + `occupant_left` broadcasts (INFO, DEC-121), essay
grade/re-grade (old→new). NOT audited: recap views (read-noise; explicitly excluded — a recap page view is read-only traffic) and closed-view 200 renders (DEC-115, read-only view, no state change).
Each row: actor, action, entity, old/new JSON, ip, request_id, ts.
Retention (= data retention, same as attempts): there is NO TTL purge in MVP —
attempts are never hard-deleted except quiz delete with zero
`registered`/`answering`/`submitted` rows (BR-052 guard, DEC-082, app-checked),
and account delete anonymises in place rather than deleting (ERD §9), so the
audit trail keeps balancing without a purge job. The consequence is stated
honestly: audit rows persist for the life of the deployment; a retention VALUE
and an explicit delete path are deliberately not invented here (F-56 — see the
deferral principle: values that would be numeric/ownership decisions belong to
the operator's runbook, INFRA §6; nothing in MVP needs them to be correct to
behave correctly). There is deliberately NO audit viewer UI in MVP (DECIDED
DEC-072); audit rows are ops-queried on the host only.

## 5. Transport and storage security

TLS at the Cloudflare edge in prod (REQUIRED per ARCH-009, which supersedes
DEC-022 by removing NGINX; `Secure`/HttpOnly/SameSite=Lax cookies depend on it).
TLS VERSION, cipher suite, HSTS and certificate-renewal
specifics are NOT pinned by this spec — they are operator-runbook items at
deploy time (Q11, recorded 2026-10-14; consolidated in the deferral register,
INFRA §6), and the renewal process belongs to whoever owns the certificate.
What this spec asserts is the shape (terminate TLS at the proxy in prod) and
the consequence (Secure cookies require it); the values are the operator's.
DB not network-exposed (internal port only — INFRA §1); uploads served only
via signed handler with access check; bcrypt cost 12 (DECIDED DEC-024);
sessions 60-min sliding idle with realtime revocation on password change
(BR-054, DEC-032). No rate limiting in MVP (DECIDED DEC-073) — abuse handling
is manual (deactivate account, close quiz) until a validated requirement
exists. Sole exception: the DEC-085 same-verb 10 s re-click guard
(429 ERR-116) is a server-side double-click guard, not a rate limit — it
rejects only the SAME control verb repeated within 10 s, changing nothing.
Response headers (F-58): this app is SSR + same-origin with no cross-origin
API surface — every route is served from one origin behind the proxy, so CORS
is not a mechanism the app needs (a cross-origin request has no legitimate
consumer here; there are no third-party origins in scope). Therefore NO CORS
policy is asserted and none is needed for correctness. App-level
`Content-Security-Policy`, `X-Frame-Options`/frame-ancestors and
`Strict-Transport-Security` directives are proxy/app response-header policy —
operator-runbook items with no values pinned here (same deferral as the TLS
specifics above; consolidated in INFRA §6). The two headers this spec DOES
assert are `X-Request-ID` correlation (§1) and the session cookie flags, which
are fixed by ARCH-005/API-000 (HttpOnly + SameSite=Lax, Secure in prod) and
are not deferrable. **Cache headers are asserted by ARCH §8 (ARCH-008) and are
NOT part of this file's deferral:** Go Fiber owns `Cache-Control`, `ETag` and
`Last-Modified` per URL class, so observability here cross-references ARCH §8
rather than duplicating the values.

## 6. Incident diagnosis runbook (MVP-short)

Symptom → check: wrong scores → `audit_log` grades + attempt `question_order` /
`question_snapshots` vs current questions (freeze violations impossible by BR-006; look
for seed/manual DB edits). Missing push → `sse_dead_total` (DEC-102 kept) + stream
error/45 s-silence evidence (no `Last-Event-ID` — DEC-102; with NGINX gone per
ARCH-009 there are no proxy buffering flags left to check, and a Cloudflare
connection drop is a known accepted risk — the client reconnect + DEC-102 refresh
rebuild is the recovery); closed-view renders without a stream (DEC-115, read-only, no audit); stream-dead countdown stays frozen at the last sync and never ticks while disconnected (freeze-countdown, DEC-125). Lost answers → attempt `answers_json` entries
+ `submit_retry_total` + idempotency keys. Slow pages → `db_latency_ms`
+ per-route histogram + missing-index check against the schema's index list
(ERD §7), plus a header check that the caching classes match ARCH §8 (assets
`public, max-age=86400`, everything else `no-store`) — a mismatched header is a
caching bug, not a DB bug. Stale assets → a vendored tree swap inside the 1-day
Class A window (ARCH-008) clearing on its own, or a build-time record mismatch.
Clock skew claims → server `attempt_deadline_utc` vs `submitted_at` (client clock
irrelevant by design). Blocked deactivate (409 ERR-115) → wait for
submit/auto-submit, or close the quiz first (the guard, not an error).
Login/code abuse (no rate limits, DEC-073) →
the app's auth middleware emits one WARN line per failed credential check
carrying `err_code: "auth_invalid"` (ERR-001, bad credentials/expired
session) at 401/302 — grep the stdout stream for
`err_code=auth_invalid` (or the JSON field `err_code`) to see the rate. There
is no separate auth-failure metric; this field IS the signal, and the emitting
component is the auth middleware (F-53). The rate reading is an
operator judgement (no threshold is asserted — thresholds are operator-runbook,
INFRA §6) → manual deactivate / quiz close (no auto-lockout in MVP).

No encryption-at-rest/compliance/certification claims are made (uninvented).
Secrets handling per infra doc; incident ownership: deploying teacher/admin.
The deploy-time alerting layer (host disk/mem, app ERROR-rate) and the
operator runbook are consolidated once in INFRA §6.
