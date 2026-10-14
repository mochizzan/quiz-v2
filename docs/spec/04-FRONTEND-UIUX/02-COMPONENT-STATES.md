# Component States — quiz-v2 (Bootstrap full + inline CSS only)

Status: DRAFT | Owner: 04-FRONTEND | Updated: 2026-10-14 (Q10/Q12 applied: F-40 waiting-room `progress` denominator pinned to `max_attempts`; F-44 §8.14 anchor repointed; stale-spinner threshold is the single 60 s per Q7 — unchanged here, C-006 owns the upstream marker)

Per-screen/component states with EXACT Bootstrap bindings + full jQuery
carrier notes (UI-002, R7, DEC-114). Only meaningful states are listed below (C-001..C-010;
states that do not apply to a component are omitted with a stated reason in
that component's block, never forced). Each state: entry/exit, allowed actions, Bootstrap
rendering (component + classes + inline-style allowance), focus. Behavioural
contracts unchanged from R2; this revision binds every element to the §4
catalog in `01-DESIGN-SYSTEM.md`. Inline `style=""` follows the closed
property list in §2 of that file; anything beyond it is a defect. Full jQuery
(UI-002, DEC-114) carries AJAX saves + DOM/event wiring; native-only are
SSE/`EventSource`, `sendBeacon`, SPA/router.

## C-001 — Student dashboard (list + analytics)

- **Purpose/inputs:** entry after login; inputs = quiz predicate result +
  analytics window aggregate. Owner of displayed state: server.
- **Shell:** `container` + `navbar` top; sections as `navs-tabs` tabs
  (Dibuka › Terdaftar › Ditutup) with server-rendered `tab` panes;
  analytics as `card` row (`row-cols-1 row-cols-md-2 row-cols-lg-4`) with
  `badge` deltas; window filter as `button-group` (3/7/30d/all) + history
  date scoping via `datepicker` on the history view (C-008 notes). Full
  quiz list per DEC-113: every eligible quiz renders on `/student`; the
  history view alone carries the query-box + chunked browsing (C-008).
- **States:**
  - `loading`: first paint server-rendered; `placeholder` skeletons ONLY on
    window-filter SSR GET swap (`aria-busy="true"`, DEC-112, UI-002). No actions.
  - `ready`: quiz `card`s (title, meta, state `badge`, primary action with
    `stretched-link`) + analytics `card`s. Actions: window-filter change
    (SSR GET reload, DEC-112), register, enter, view history.
  - `empty-open`: `card` + `alert-info` "Belum ada quiz dibuka" + guidance.
  - `error`: static `alert-danger` with retry button (ERR-501); last HTML stays.
  - `reconnecting`: NOT APPLICABLE (no stream on this page) — no component.
- First-login onboarding: `carousel` 3 slides (daftar › jawab › aturan fokus),
  shown once, dismissible, `prefers-reduced-motion` safe.
- Feedback: window-filter changes announce via `toast` + `aria-live=polite` region.
- Links: FLOW-002 step 1, BR-007, API-030/070, AC-201.

## C-002 — Question page (one per page)

- **Shell:** `container` + `card` (prompt as `h1.h4` in `card-body`,
  `img-fluid` if present) + `progress` position bar (inline
  `style="width: N%"` from server `current_index`) + `stepper` text
  ("Soal 3 dari 10") + countdown text (`role=timer`, throttled) with
  supplementary `progress` bar + sticky bottom `navbar` action bar
  (Sebelumnya/Berikutnya/Simpan, `btn-lg`, inline safe-area padding).
- **Inputs:** single = `radio` list in `list-group-item` labels; multiple =
  `checkbox` list; essay = `form-control` textarea (`maxlength=10000`,
  counter, `form-adorn` count). `validation` classes server-rendered on 422.
- **States:** `answering` (inputs enabled) → `saving` (`spinner-border`
  in button + disable, "Menyimpan…") → `saved` (`toast` success "Tersimpan ✓
  + server time", saved-tick only after HTTP 200 per DEC-124) / `save-error`
  (static `alert-danger` + retry, answer kept in DOM) → `locked` (per-Q expiry:
  `alert-warning` + inputs disabled + auto-advance) / `paused` (matching: static
  `dialog` blocker, no Esc) / `expired` (total: redirect to the result card showing
  the auto-submitted receipt — success framing per DEC-086, never an error banner;
  the attempt was submitted by the timer path) / `dead-stream-freeze` (countdown
  FREEZES on dead stream per DEC-125; F-002 banner shows; no ticking estimate).
- Unsaved guard, two layers (DEC-123): native `beforeunload` only while `saving`
  is in flight; dirty-flag dialog (Bootstrap `dialog` on the dirty-essay flag,
  typed but unsaved essay text). After successful save no confirm.
- Duplicate-submit: Save disabled while `saving`; Enter bound once;
  start/submit carry the server-generated `Idempotency-Key` (DEC-084);
  per-question saves (API-035) are plain overwrites, no key. Optimistic updates:
  NONE for answers (BR-020); only the saved-tick is post-200.
- Offline: generic offline `alert` banner (no answer queue count, DEC-124;
  cheat queue count only where F-005 applies, C-010).
- Focus: advance → new question `h1` (with `focus-ring`); lock notice →
  the `alert`; pause `dialog` traps, resume returns focus to Q1.
- Links: FLOW-002/003/009, BR-011/012/013, API-034/035, AC-202..205.

## C-003 — Review page (total/none only)

- **Shell:** `card` + `list-group` (one item per question: number, answered/
  unanswered `badge`, selected-option `chips`, jump link) + sticky bottom
  `navbar` with submit `button` opening a `dialog` confirm ("Kumpulkan
  sekarang?"). Per-question mode: read-only trail variant (items without
  jump, BR-011).
- **States:** `ready` → `submitting` (`dialog` confirm → disable +
  `spinner`) → `submitted` (redirect) / `expired` (auto-submit path → result
  card with the auto-submitted receipt, success framing per DEC-086). Uncommitted
  answers lost to offline are reported dropped (DEC-090) — the review page never
  shows phantom saved state.
- Links: BR-012, API-037/038, AC-206.

## C-004 — Result card (thank-you + visibility-filtered score)

- **Shell:** `card` (thank-you heading "Terima kasih sudah mengerjakan quiz
  <nama>") + score block (`badge` Final/Menunggu + numbers) + per-question
  breakdown as `accordion` (one item per question, open only for
  pending/incorrect) + `progress` summary + print `button` (print CSS handled
  by Bootstrap print utilities + minimal inline `print-color-adjust`).
  Anonymous viewers see the identical card (only receipt).
- **States:** `temporary` (objective subtotal + "menunggu N essay" `badge`) /
  `final` (all graded) / `hidden` (score suppressed per BR-030: thank-you +
  guidance `alert-info` only) × `correct-shown/hidden` orthogonal (BR-031).
- Re-grade: temporary shows until the next render (refresh/reopen) per DEC-104.
  The final card renders on refresh/reopen + `toast`
  "Nilai final diperbarui" on next view.
- Links: BR-022/030/031, API-039, AC-207.

## C-005 — Waiting room (matching, student)

- **Shell:** `card` (quiz title + session `badge` + `avatar` roster as
  `list-group` with `connected` dots) + roster `badge` count + leave `button`
  (pre-start → `POST /matching/:id/leave` per DEC-121; `connected=0`,
  `last_seen` kept) + `progress` (occupied slots vs the quiz's
  `max_attempts` cap — informational only, since no `classes` table and no
  `class_size` column exist in the R2 schema; the denominator is a stored
  number, not a class roster) +
  `alert-info` "menunggu guru memulai".
- **States:** `waiting` → `starting` (event → `spinner` + auto-navigate,
  inputs locked) / `locked-out` (static `alert-warning` ERR-301) /
  `disconnected` (persistent F-002 `alert-warning` notice — no spinner, no
  auto-rejoin; user refresh rebuilds presence, DEC-102) /
  `ended` (session finished/closed: `alert-secondary` + recap link).
- No answer inputs by design. `scrollspy` NOT used here (linear page).
- Links: FLOW-003, STATE-002, API-040/041, AC-208.

## C-006 — Teacher live monitor

- **Shell:** two monitor surfaces (DEC-110): waiting roster surface
  (pre-start `list-group` roster, `avatar` + `badge` connection) + live
  progress surface (post-start `table`: `.table-striped.table-hover.table-responsive`,
  sticky header, frozen name column via utilities + inline offsets) +
  control `button-group` (start/pause/stop/finish, each → `dialog`
  button-confirm with explicit consequence buttons, no typed text — DECIDED
  DEC-042) + per-row `collapse` detail on phone + `toast` stack for cheat
  increments + `stale` `alert-warning` (frozen numbers + F-002 dead-stream
  notice, never a poll — DEC-102).
- **States:** `live` (SSE rows: name, q-index/N `progress`, last-answer
  `chips`, review `badge`, cheat `badge` from the `occupant_progress` field
  (DEC-120, no cheat event), connection dot; per-question guru-full single
  index (DEC-119); never-present no-show rows render a `secondary`
  "tidak hadir" `badge`, excluded from progress math) / `stale` (no heartbeat
  60 s → amber banner with the F-002 notice text; numbers frozen at last-seen;
  user refresh rebuilds, DEC-102) / `empty` (`card` + guidance) /
  `control-confirm` (`dialog` per verb).
- `rate-limited` (same verb re-clicked within 10 s with a stale version →
  429 ERR-116, refined R8): inline
  `alert-warning` "tunggu 10 detik" + current state, no dialog, buttons stay
  enabled (the guard, not a lockout).
- Monitor-row vanish on submit is **decided, not open** (S4b closed): the row
  disappears when the submitter's attempt commits (API-044 + FLOW-007 + AC-209),
  and the recap table holds the history — so the monitor is a live view of
  *in-flight* rows only. The UI here is a **consumer** of that contract: it
  removes the row on the existing submit/occupant fan-out and never re-adds it
  on refresh (refresh shows recap, not the live surface). No new SSE event name
  is introduced for the vanish (declined per DEC-102 refresh — the event
  catalog in API-036/041/042 is closed; the removal rides the already-catalogued
  submit fan-out). S4's heartbeat payload shape is DECIDED (S4a, CLOSED
  2026-10-14): `{server_now_utc}` every 30 s, with a single 60 s stale
  threshold after which the spinner is cleared and numbers stay frozen at the
  last-seen value (no second threshold; the 45 s figure below is the separate
  dead-stream silence timeout, DEC-102). The UI consumes that contract and
  introduces no new event name.
  Actions: revoke (pre-start only, plus never-present no-show rows post-start
  per BR-008 exception); no force-submit (stop covers it); no chat.
- Links: FLOW-007, API-042/043/044, AC-209.

## C-007 — Essay grading queue

- **Shell:** `breadcrumb` (Recap › Grading) + queue as `list-group` /
  `table` rows (student `avatar`+name, question, `prose` answer, `input-group`
  grade `form-control` + `/10` `form-adorn`, save `button` with `spinner`) +
  `pagination` (page/per_page) + empty-queue celebration `alert-success` +
  link to recap.
- **States:** `queue` (oldest-first) / `saving` (per-row `spinner`) /
  `graded` (row collapses via `collapse` with score `badge` + audit note via
  `popover`) / `empty-queue`.
- Re-grade: expands graded row (`collapse` show) with old value + overwrite
  (audit-logged). Validation: `forms/validation` on 0–10 domain (ERR-105).
- Links: FLOW-005, BR-021/022, API-060/061, AC-205/114/210.

## C-008 — Recap + history tables

- **Shell:** `container-fluid` (teacher) / `container` (student history) +
  filter bar (`button-group` window + `datepicker` day filter + `combobox`
  class picker + `input-group` search) + `table-responsive` `table`
  (newest-first rows: label, per-question glyphs, subtotals, cheat `badge`,
  duration, submitted_at WIB + UTC `tooltip`) + `pagination` (BR-055: SSR
  default 25, `?per_page=` up to 200; `?q=` search-AJAX fragment +
  `replaceState`-only per DEC-112, all other filters SSR GET; no-JS GET
  fallback) + one-click
  `recap.xlsx` export `button` (API-063, DEC-002; streams workbook, same order
  filter as the table). Phone: horizontal scroll + frozen name column.
  Student history keeps `?q/&date/&page` with search + pagination; only the
  `/student` dashboard is full-list (DEC-113, C-001).
  Anon rows display `(anon) ` prefix + `taker_label` (BR-014 canonical);
  score columns use schema names (`objective_score`/`essay_score`/`final_score`,
  screen `total` = `final_score`).
- History cards (student): `card` + score/cheat `badge`s + receipt link
  (`stretched-link`); receipt reuses C-004.
- Links: BR-014, API-062/070, AC-211/214.

## C-009 — Auth and account forms (login/register/profile/users/join)

- **Shell:** `examples/sign-in` centred `card` + `floating-labels` inputs +
  `validation` (`needs-validation`, `is-invalid`, `invalid-feedback`,
  summary `alert-danger` with focus move) + `password-strength` advisory
  meter + show/hide password `input-group` + `otp-input` segmented code
  boxes on `/join` (bound to ONE hidden `form-control`, server validates
  BR-040; paste supported; no-JS = single plain input) + `combobox`
  class/grade pickers + `datepicker`-free DOB (no such requirement) +
  `breadcrumb` on management + `menu` overflow actions on user rows +
  `dialog` button-confirms for deactivate/delete (explicit buttons, no typed
  text — DEC-042).
- **States:** `idle/editing` / `field-error` (inline + summary) /
  `submitting` (button `spinner` + disable) / `registered-no-session`
  (self-register success → redirect to `/login`, no session created, DEC-087) /
  `forced-rotation` (seed login:
  `card` only, nothing else clickable until changed) /
  `deactivate-blocked` (409 ERR-115: account holds live open-quiz rows, DEC-081) /
  `confirm-destructive` (button `dialog`).
- Passwords never echoed on 422; password rule text shown inline from BR-053
  ("minimal 8 karakter tanpa spasi, wajib ada angka dan huruf besar").
  `role` never rendered as editable (RBAC R5).
- Links: BR-050/051, API-001..004/010..012/050/051, AC-212.

## C-010 — Global chrome (banner/modal/toast layer)

- Banners: connectivity `alert` stack (generic offline banner, no answer queue
  count per DEC-124; cheat queue count only where F-005 applies / persistent
  dead-stream notice F-002 with the exact refresh wording, static while dead)
  + no-JS `alert-warning` listing exactly three items (cheat detection, SSE
  push, toasts) per EDGE-045/DEC-128
  + 503 `alert-danger` with retry. Top of `container`, below `navbar`.
- Dialogs: cheat `dialog` (blocking, count `badge`, "Mengerti, kembali"),
  pause `dialog` (static, non-dismissible, `progress` frozen note), submit
  `dialog`, destructive button `dialog`s (explicit consequence buttons, no
  typed text — DEC-042). Trap + return-focus (pause →
  focus Q1 on resume).
- Toasts: saved, submitted, revoked (`toast` stack
  top-end, `close-button`, polite live region; errors stay as `alert`). No
  `graded-live` toast exists (DEC-104 — grades surface on next render) and no
  `reconnected` toast exists (DEC-102 — recovery is a user refresh, a full
  page load).
- Navbar: brand text, role links, connection `badge` (live vs dead-stream per STATE-007 — display-only, no auto action), profile `menu`,
  `toggler` for teacher `drawer` on small screens, `tooltip`s on status items,
  `popover`s for non-critical help. `scrollspy` NOT global (editor page only).

UI-owned audit notes (S1..S6, one-liners):
- S1/S2/S3/S5/S6: not UI-owned — no UI decision recorded here.
- S4 (resolved 2026-10-14 per §5b, owner: C-006/C-010 + AC-135/209): the
  **vanish half is closed** — monitor-row disappearance on submit is decided by
  API-044 + FLOW-007 + AC-209 and the UI here is a **consumer** of that contract
  (it removes the row on the existing submit fan-out; recap holds history; no
  new SSE event name is introduced — the vanish-event-name question is declined
  per the DEC-102 closed event catalog). The **payload half is also CLOSED**
  (S4a, 2026-10-14): the heartbeat payload is `{server_now_utc}` every 30 s, and
  the stale spinner is cleared after a single 60 s stale threshold. Both halves
  are decided; nothing here remains open.
- Accepted limitation (not a decision): abort-toast wording, focus return, and live-region polish stay best-effort per C-010.
