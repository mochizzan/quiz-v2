# Error Matrix — quiz-v2

Status: DRAFT | Owner: 03-RESILIENCE | Updated: 2026-10-12

Machine-readable `code` is stable and asserted by tests. Page forms re-render
with the user-safe message and field errors; AJAX/JSON requests
(`Accept: application/json`, carried by jQuery per UI-002) return
`{code, message, details, request_id}`. Internal diagnostics (actor id, quiz
id, SQL, stack) go to logs ONLY, never to the client. No stack traces,
passwords, tokens, or session ids in responses.

| ID | Code | Layer / Trigger | HTTP | User-safe message (id/en) | Retry? | Terminal state |
|----|------|-----------------|------|---------------------------|--------|----------------|
| ERR-001 | `auth_invalid` | Auth: bad credentials, expired session | 401 / 302 login | "Email atau password salah / sesi berakhir." | Yes (re-login) | Unauthenticated |
| ERR-002 | `csrf_invalid` | Middleware: missing/bad CSRF; cheat exemption — cheat-beacon exemption (DEC-108, API-03A only: native `sendBeacon` cheat beacon is CSRF-exempt, never 403 for missing token) | 403 | "Sesi tidak valid, muat ulang halaman." | Yes (reload) | Same page |
| ERR-003 | `forbidden` | RBAC: role/ownership fail | 403 | "Anda tidak memiliki akses." | No | Same page |
| ERR-100 | `validation` | Generic field validation | 422 | Per-field messages | Yes (fix input) | Form preserved |
| ERR-101 | `timer_invalid` | Quiz config vs BR-003 | 422 | "Pengaturan timer tidak valid." | Yes | Draft unchanged |
| ERR-102 | `options_invalid` | Question vs BR-004 | 422 | "Opsi dan jawaban benar tidak valid." | Yes | Editor preserved |
| ERR-103 | `image_invalid` | Upload vs BR-010 (type/count); temp-write volume-full or permission-denied at intake (no row written, same family) | 422 | "Gambar harus JPG/PNG/WEBP/JPEG/GIF, 1 per soal." | Yes | Editor preserved |
| ERR-104 | `image_unprocessable` | Background job fails: incompressible, unreadable, or corrupt after intake (BR-010) | async (surfaced on editor as `failed` + reason, teacher re-uploads) | "Gambar tidak dapat diproses; unggah ulang file lain." | Yes (re-upload) | Question row `failed`, quiz flow unaffected |
| ERR-105 | `grade_out_of_range` | Essay grade vs BR-021 | 422 | "Nilai essay 0–10." | Yes | Queue unchanged |
| ERR-106 | `type_immutable` | Type change post-live BR-001 | 422 | "Tipe quiz tidak dapat diubah." | No | Unchanged |
| ERR-107 | `quiz_frozen` | Edit frozen field post-start (DEC-103 hidden/show rule, BR-006): open+show+any row → 422; answering/submitted anywhere → 422 | 422 | "Quiz sudah berjalan; tidak dapat diubah." | No | Unchanged |
| ERR-108 | `quiz_not_openable` | Open vs BR-005 | 422 | "Quiz butuh minimal 1 soal valid." | Yes (add questions) | Draft |
| ERR-109 | `email_taken` | Register/profile uniqueness | 409 | "Email sudah digunakan." | Yes (other email) | Form preserved |
| ERR-110 | `password_weak` | BR-053 fail: <8 chars, whitespace, no digit, or no uppercase | 422 | "Password minimal 8 karakter tanpa spasi, wajib ada angka dan huruf besar." | Yes | Form preserved |
| ERR-111 | `quiz_has_attempts` | Delete quiz with any row in (`registered`, `answering`, `submitted`) (BR-052 guard, DEC-082) | 409 | "Quiz memiliki nilai; tutup/sembunyikan saja." | No | Unchanged |
| ERR-112 | `worker_queue_full` | Image worker pool saturated (2 workers, queue 100) at upload intake | 429 | "Antrean pemrosesan gambar penuh; coba lagi sebentar." | Yes (retry later) | Editor preserved, no row written |
| ERR-113 | `session_empty` | Matching start with 0 occupants (DEC-011 guard) | 422 | "Belum ada peserta di ruang tunggu." | Yes (wait for occupants) | `waiting` unchanged |
| ERR-114 | `code_taken` | Token code duplicate among open+enabled quizzes (BR-040) | 409 | "Kode sudah dipakai; gunakan saran baru." (+ suggestion) | Yes (use suggestion) | Unchanged |
| ERR-115 | `deactivate_blocked_active` | Deactivate blocked (409, changing nothing — the guard, not force-submit: Q3, 2026-10-14) on either trigger leg: (a) while the user holds `registered`/`answering` rows on an open quiz (BR-051 guard, DEC-081; AC-123), OR (b) while the user owns a live-session — owns a `running`/`paused` matching session (DEC-105, including self-deactivation; same 409, terminal state unchanged: no session revoked, no auto-close, no force-submit) | 409 | "Akun tidak dapat dinonaktifkan: masih ada quiz terbuka yang diikuti pengguna, atau sesi live yang sedang berjalan." | No (stop the session / wait for submit or close the quiz) | Unchanged (no session revoked) |
| ERR-116 | `verb_too_soon` | SAME matching control verb repeated for the same quiz within 10 s of its previous acceptance, carrying a stale `session_version` (accidental-double-click shape, DEC-085 refined R8; current-version repeats echo, failed-verb retries follow echo-vs-409; waiting-verbs note per DEC-126: pause/stop/resume on a `waiting` session = 200 echo, never 409/429 (never this 429); AC-112) | 429 | "Tunggu 10 detik sebelum mengulang perintah." (+ current state) | Yes (after 10 s, or re-read + re-POST for echo) | Current state |
| ERR-201 | `already_registered` | Duplicate registration; anon active-conflict (DEC-106, STATE/AC ref only): active same-label row → 409 conflict echo; quota exhausted → 410 ERR-208, never 409 here | 409 echo | "Anda sudah terdaftar." (+ link) | No (echo) | registered |
| ERR-202 | `quiz_closed` | Register/start on closed | 410 | "Quiz sudah ditutup." | No | Same page |
| ERR-203 | `already_submitted` | Double submit | 200 echo | Receipt echo (not an error to user) | No | submitted |
| ERR-204 | `attempt_closed` | POST to submitted attempt | 410 | "Quiz sudah disubmit." | No | submitted |
| ERR-205 | `question_locked` | Answer locked question BR-011 | 422 | "Waktu soal ini habis." | No (forward) | Advanced index |
| ERR-206 | `attempt_expired` | Per-question answer POST with server time past deadline (zero grace, DEC-006; covers answer saves ONLY — late submit returns 200 + auto-receipt per DEC-086, never this code) | 410 | "Waktu quiz habis; jawaban tersimpan otomatis." | No | submitted |
| ERR-207 | `cancel_too_late` | Cancel post-start BR-008 | 422 | "Tidak dapat batal setelah mulai." | No | answering |
| ERR-208 | `attempts_exhausted` | Register/start beyond `max_attempts` (BR-002/009; register rejects pre-INSERT, start auto-cancels stranded row per DEC-083; anon active same-label quota path per DEC-106: quota exhausted → this 410, not ERR-201; AC-115) | 410 | "Kesempatan mengerjakan sudah habis." | No | Unchanged (register) or `cancelled` (stranded start row) |
| ERR-209 | `question_back_navigation` | Per-question timer: back-navigation GET for `n < current_index` (BR-011 forward-only; per-question mode, no back-navigation) renders the read-only snapshot of the locked question | 410 | "Soal ini sudah terkunci; jawaban tidak dapat diubah." | No (forward) | Read-only snapshot shown on the same page |
| ERR-300 | `state_conflict` | Control-verb race (stale version); direct `running`/`paused → finished` (finish valid only from `stopped`); essay grade on an unsubmitted attempt (`grade_not_ready` mapped here); waiting-verbs note (DEC-126: pause/stop/resume on a `waiting` session = 200 echo of `waiting`, never 409/429) | 409 | "Status berubah; memuat ulang…" + current state | Yes (re-sync) | Current state |
| ERR-301 | `room_locked` | Enter after start; register after the matching session left `waiting` (DEC-093) | 410 | "Ruang tunggu dikunci." | No | — |
| ERR-302 | `session_paused` | Answer while paused | 423 | "Quiz dijeda guru; menunggu…" | Yes (after resume) | paused |
| ERR-400 | `code_invalid` | Token lookup miss, incl. hidden-quiz codes (BR-040 visible predicate); TTL entry-only note (DEC-117, STATE/AC ref only: TTL gates entry, never ejects in-flight occupants) | 404 | "Kode salah atau quiz tidak tersedia." | Yes (retype) | Join page |
| ERR-401 | `name_required` | Anonymous identify; TTL entry-only note (DEC-117, STATE/AC ref only: TTL gates entry, never ejects in-flight occupants) | 422 | "Nama wajib diisi." | Yes | Identify page |
| ERR-500 | `internal` | Unexpected failure | 500 | "Terjadi kesalahan. Coba lagi." + request id | Yes — non-submit paths retry once explicitly (F-001); submit path retries keyed max 3 with backoff 1/2/4 s (F-006) | Preserved input |
| ERR-501 | `db_unavailable` | MariaDB down/timeout | 503 | "Layanan sibuk; jawaban tersimpan aman." | Yes (backoff) | Last saved kept |
| ERR-502 | `stream_interrupted` | SSE drop/proxy buffer | 200* (client event, NOT a server error — INFO level) | Persistent notice "Koneksi live terputus — jawaban tersimpan aman. Refresh halaman untuk menyambung ulang." (F-002, DEC-102) | Manual (user refresh) | Full re-render from server truth |

Logging: every ERR-5xx logs `request_id, actor, route, latency, sql_state`
at ERROR; ERR-4xx at INFO with actor+quiz ids (auth failures at
WARN). Alerting thresholds live in
`../05-DEVOPS-AND-INFRA/03-LOGGING-SECURITY-OBSERVABILITY.md`. Client behaviour:
timer FREEZES at the last server sync during outages (DEC-125 freeze, not counting); read-only GETs
retry once, mutations retry only via explicit user action (button) or the
keyed submit path (F-001/F-006) — offline answers that never reached the
endpoint are DROPPED with notice, never retried (DEC-090); cheat beacon retries once then drops (count gap
documented, never blocks submit).

## Audit findings applied (§5b, 2026-10-14)

- **N6** — Timer during outage: freeze vs counting. Footer corrected from
  "keeps counting" to DEC-125 freeze (matrix was the outlier; F-002/AC-153/C-002
  and DEC-125 all state FREEZE). F-001 wording already consistent with freeze.
- **N11** — Close-409 without an ERR code. The close/hide-while-running-or-paused
  409 (EDGE-055C/AC-155C/DEC-093) previously had status + exact text but no ERR-### row.
  Recorded as an explicit text-only exception below.
- **N18** — ERR-502/client-event vs "every code test-asserted" invariant.
  ERR-502 is a DOM-text assertion (persistent notice), not a code test.
  Exception recorded in the invariant note below.

### ERR-3xx text-only exception (N11)

The following 409 response is asserted by EDGE-055C / AC-155C / DEC-093
(close/hide while `running`/`paused` → stop-first) and carries **no ERR-### row**:
it is a **text-only exception** — the acceptance test asserts the exact
Indonesian message and HTTP status, not a machine-readable `code`. This is
deliberate: the response is a UI guard (teacher must stop the live session
before closing), not a client-recoverable error with a retry path.

| Scenario | HTTP | Exact text (asserted by AC-155C) | ERR code | Retry? |
|----------|------|----------------------------------|----------|--------|
| Close/hide while `running` or `paused` (DEC-093, EDGE-055C) | 409 | "Hentikan sesi terlebih dahulu sebelum menutup/ menyembunyikan quiz." | **None — text-only exception (N11)** | No (stop first, then close) |

### Invariant note: ERR-502 DOM-text assertion (N18)

The invariant "every ERR-### code is test-asserted" holds for all machine-readable
`code` values returned in JSON error envelopes. **ERR-502 is the sole documented
exception**: it is a **DOM-text assertion**, not a code test. ERR-502 (`stream_interrupted`)
is a client-side event (200*, INFO level, DEC-102) surfaced as a persistent
inline `alert-warning` with the exact Indonesian notice
"Koneksi live terputus — jawaban tersimpan aman. Refresh halaman untuk menyambung ulang."
(F-002, DEC-102). The acceptance test (AC-135/153/208) asserts the rendered DOM text
and the `EventSource.close()` behaviour, not a JSON `code` field — because the
server never emits ERR-502 as a response body. This exception is recorded here
so the invariant is stated honestly rather than silently violated.
