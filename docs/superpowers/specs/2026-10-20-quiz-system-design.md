# Quiz System Design

Date: 2026-10-20
Status: Draft for review

## 1. Intent and why

A web-based quiz system for a single school. Teachers author mixed-type
quizzes (single-choice, multiple-choice, essay) and run them for students
in class, during exam weeks, or for daily/remote assessments. Students
take quizzes on their own devices; teachers monitor progress live, grade
essays, and review every attempt. The system must work without any
vendor accounts or paid services: self-hosted, single binary + MariaDB,
SSR pages, real-time updates over SSE.

## 2. Goals, non-goals, anti-goals

Goals
- Teachers can create, edit (before start), open, close quizzes.
- Students (registered or anonymous) can join an open quiz and complete
  it in one sitting, with auto-resume on reconnect.
- Auto-scoring for objective questions; manual 0-100% grading for essays.
- Live teacher view of who joined and how far along each student is.
- Configurable result display after submission.
- Every attempt retained unless its participant is deleted; teacher decides
  the final score.

Non-goals (v1)
- No native mobile app; responsive web only.
- No payment, no multi-school tenancy, no SSO.
- No plagiarism/AI detection.
- No CSV/PDF export (XLSX only if added later).

Anti-goals
- A quiz result that silently picks "best" or "latest" attempt — the
  teacher always decides.
- A hard block that prevents a student from finishing because of a
  transient network drop.

## 3. Actors and permissions

Roles: teacher, student, anonymous participant.

Registration
- Teacher: full name, email, password. Account starts inactive; another
  teacher must approve/activate it. Registration returns a signed,
  single-use `pending_token`; `GET /auth/pending?token=…` shows that
  request's status, and signing in before activation is refused with a
  "menunggu persetujuan" message that links back to that page. The first
  teacher account auto-activates when no teacher exists yet.
- Student: full name, email, password, grade level (10/11/12), field of
  study (major). Active immediately.
- Anonymous: no account. Joins via 6-digit quiz code or invitation URL;
  provides full name, grade level, major. Stored in a separate table.

Profile editing
- Everyone edits their own profile.
- Teachers may edit student profiles (including anonymous participants).
- Teachers may not edit other teachers' profiles.
- Students may not edit other students' profiles.

Account management
- Teachers manage student and anonymous accounts (view/edit/delete).
- Teacher accounts are managed only by other teachers (approval +
  edit). No self-activation.
- Teacher accounts are never deleted in v1. A peer teacher may deactivate
  one (`is_active = false`), which revokes its access and keeps its
  quizzes, attempts and grades intact. There is no delete route for
  teachers.

Fields of study (majors)
- Teachers create and edit the list of majors. Students and anonymous
  participants pick from this list.
- Renaming a major propagates to students/anonymous_participants via an
  app-level update (major_name is denormalized).

## 4. Quiz lifecycle

States: draft, open, closed.

- draft: not visible/joinable by students; teacher edits freely.
- open: students can join and take it. Once any attempt exists, structural
  edits (questions, scoring, randomization, visibility, audience, retake
  settings) are locked; cosmetic edits (title, description, dates, result
  display mode) remain allowed.
- closed: finished; no new joins; grading and review happen here.

State transitions: draft → open → closed. Closed is terminal (no reopen).

Visibility: public (any eligible student) or private (restricted to a
grade level + major).

Audience rule: `visibility = private` requires **both** `target_grade` and
`target_major_name`; `visibility = public` requires both to be NULL. The
rule is enforced on every write — quiz create, settings update, and the
draft→open transition re-checks it before the quiz is published.

Start/end date+time are shown to students as a visual indicator only;
they do not hard-gate access. The state (draft/open/closed) is the gate.

## 5. Authoring flow

1. Teacher creates quiz: title, description.
2. Teacher adds questions: text + optional image; type single/multiple/essay;
   for objective questions mark correct option(s); essay questions are
   graded 0-100% by the teacher.
3. Teacher configures advanced settings (section 7).
4. Teacher sets visibility + target audience (if private).
5. Teacher sets state to open to publish.

Images are uploaded during authoring and stored in their original, raw
form (no resize, no crop). Accepted formats: jpg/png/webp. Hard limit 10 MB;
files above 5 MB are auto-compressed. Processing (compression) runs in a
bounded background worker pool (my call: 2 workers, queue 100, 10-min
timeout).

## 6. Taking flow

1. Student/anonymous obtains a 6-digit code or invite URL.
   Join succeeds only if the quiz is open and, for private quizzes, the
   participant's grade+major match the target.
2. Anonymous provides name/grade/major; registered student logs in.
3. On join, server creates an attempt (status in_progress) and a randomized
   question order if configured; answer order randomized per attempt if
   configured.
4. Student answers in one sitting. Each answer is saved immediately via
   POST. No explicit pause; if the browser closes or network drops, the
   student resumes from the server-side attempt on reconnect.
5. On submit, attempt status becomes submitted. Auto-score computed for
   objective questions. Essay answers await grading.
6. Result display follows the quiz's configured mode (section 7).

Focus-loss anti-cheat (must include, per user): while an attempt is in
progress, the client sends a focus-loss beacon when the tab/window loses
visibility; the server records the count and the timestamp of each event
per attempt (count on `quiz_attempts`, events in `focus_loss_events`).
Teachers see the count in the live monitor and in review. No hard block
on submit (my call); it is informational for the teacher.

One active in-progress attempt per participant per quiz at a time (my
call). Re-attempts only if the teacher allowed retakes and the attempt
count is under the limit.

Retake quota (my call): the quota counts attempts in status `submitted` or
`graded`; the single `in_progress` attempt is the active one and blocks a
new attempt from being created for the same participant. `max_attempts` is
the total number of attempts allowed and is forced to `1` when
`allow_retake` is false — the field is then ignored.

Randomization is snapshotted per attempt (my call): the question order is
stored in `attempt_answers.position`, and the answer-option order is
stored in `attempt_answers.option_order` as a JSON array of option ids.
Both are written when the attempt is created, so a reload or a reconnect
reproduces the same order; options are never re-shuffled on resume.

The answer form is a real HTML form that POSTs to the server (my call):
with JavaScript disabled the student can still answer and submit. The
autosave, the focus-loss beacon and the progress beacon are enhancements
layered on that form, not its mechanism.

## 7. Advanced settings (per quiz)

- Result display at end: questions only / questions + answers /
  questions + student answers + answer key / nothing.
- Randomize question order (per attempt).
- Randomize answer choice order (per attempt).
- Allow retakes: boolean + max attempts (1..N). `max_attempts` is
  validated to `>= 1` and is forced to `1` when retakes are off.
- Start/end date+time (visual indicator).

## 8. Scoring model

Simplified formula (per user): Points Earned = Score / 100.

- Objective question (single/multiple-choice): Score = 100 if the answer is
  fully correct, else 0 → 1.00 or 0.00 points. Multiple-choice requires
  an exact match of the correct option set; a partial match scores 0.
- Essay question: teacher assigns 0-100% → 0.00-1.00 points.
- Final attempt score = (sum of points earned) / (number of questions),
  yielding 0.0-1.0; displayed as 0-100.
- **Scale contract:** the columns `attempt_answers.auto_score`,
  `attempt_answers.essay_score` and `quiz_attempts.final_score` are stored
  on a **0–1** scale. A teacher-entered essay grade of 80% is stored as
  `0.8000`. The **templates** multiply by 100 and round for display, and
  display the `%` sign. No handler stores a 0–100 value and no template
  displays a 0–1 value. The attempt-level objective total is **derived**
  (`SUM(attempt_answers.auto_score)`), not stored.
- **Final score ownership (my call):** the formula above is the *default*
  the teacher is shown, pre-filled in the grading form; the teacher may
  adjust it before finalising. `quiz_attempts.final_score` is always the
  teacher-confirmed value.
- **`counted` semantics (my call):** at most one attempt per participant
  per quiz carries `counted = true`. Finalising an attempt as counted
  clears the flag on that participant's other attempts for the same quiz
  in the same transaction. The counted attempt is the one the student sees
  as their result, and the one that counts in the quiz's aggregate.
- Auto-score is computed on submit and stored; final score is set by the
  teacher after grading all essays, choosing which attempt counts.
- Every attempt is stored; the system never auto-selects best/latest.
- Per-question `points` column is retained in schema for future use but v1
  treats every question as 1.00 (per the simplified formula above). My call.

## 9. Grading flow

1. Teacher opens a closed (or open) quiz's submissions.
2. For each submitted attempt, teacher grades essay answers (0-100%).
3. Teacher confirms the final score for the attempt — pre-filled with the
   formula's value, adjustable — and marks it as the counted attempt
   among retakes.
4. Attempt status becomes graded. Student sees result per display mode.

Result visibility (my call): a quiz with no essay question is
auto-scored, so its result is released the moment the attempt reaches
`submitted`. A quiz with at least one essay question releases the result
only when the attempt reaches `graded`, so a student never sees a
partially graded score; until then the student sees a "sedang dinilai"
state. The student sees the **counted** attempt; while no attempt is
counted yet, the student sees that waiting state.

## 10. Live monitoring

While a quiz is open, the teacher sees a live list of participants with:
joined time, current question index, answered count, submitted flag.
Delivered over SSE (per-quiz channel, 30s heartbeat, fan-out <2s).
Students POST progress beacons (CSRF-exempt, rate-limited: 30 requests
per minute per attempt, my call).

## 11. Data model (MariaDB 12.x)

Design note (per user): minimize foreign keys and complex relationships.
Denormalization and data duplication are acceptable; prefer ease of
querying and migration over strict normalization. Referential integrity is
enforced in the application layer, not by DB constraints. No ENUM types
(use varchar + app validation). Fewer tables; snapshot denormalized fields
where a historical record must survive deletion.

Tables:

teachers
- id PK, full_name, email UNIQUE, password_hash, is_active, created_at,
  updated_at

students
- id PK, full_name, email UNIQUE, password_hash, grade_level, major_name,
  created_at, updated_at
  (major stored as name; no FK to majors)

majors
- id PK, name UNIQUE, created_by_teacher_id (no FK), created_at, updated_at

anonymous_participants
- id PK, full_name, grade_level, major_name, created_at
  (no FK; one row per anonymous identity — the signed cookie identifies
  the row, not the quiz, and one participant may hold attempts on
  several quizzes)

quizzes
- id PK, teacher_id (no FK), teacher_name (denormalized snapshot),
  title, description, visibility (public/private),
  target_grade (nullable), target_major_name (nullable),
  state (draft/open/closed), allow_retake bool, max_attempts int,
  randomize_questions bool, randomize_answers bool,
  result_display_mode (questions_only / questions_and_answers /
  questions_answers_key / nothing), code (6-digit join code, UNIQUE,
  nullable — generated when the quiz opens, invalidated when it closes),
  start_at, end_at, created_at, updated_at

questions
- id PK, quiz_id (no FK), position int, body text, image_ref varchar (nullable),
  type (single/multiple/essay), points decimal(3,2) default 1.00,
  created_at, updated_at

answer_options
- id PK, question_id (no FK), position int, body text, is_correct bool

quiz_attempts
- id PK, quiz_id (no FK), quiz_title (denormalized snapshot),
  participant_type (student/anonymous), participant_id (no FK),
  participant_name, participant_grade, participant_major (denormalized),
  started_at, submitted_at, status (in_progress/submitted/graded),
  final_score decimal(5,4), counted bool, focus_loss_count int default 0,
  created_at, updated_at

attempt_answers
- id PK, attempt_id (no FK), question_id (no FK), position int,
  selected_option_ids JSON (nullable), essay_text text (nullable),
  option_order JSON (nullable — the per-attempt answer-option order as a
  JSON array of option ids; written at attempt creation when
  `randomize_answers` is on),
  auto_score decimal(5,4), essay_score decimal(5,4) (nullable),
  graded_by_teacher_id (no FK, nullable), graded_at (nullable),
  created_at, updated_at
  (question order stored here via position; no separate attempt_questions
  table — one row exists per question of the attempt, created with the
  attempt)

sessions (DB-backed)
- id PK, token_hash CHAR(64) UNIQUE (the sha256 of the cookie token; the
  raw token is never stored), user_type (teacher/student), user_id (no
  FK), expires_at, created_at

focus_loss_events
- id PK, attempt_id (no FK), occurred_at datetime
  (one row per focus-loss beacon; `quiz_attempts.focus_loss_count` is the
  denormalized count of these rows)

Deletion policy (per user + my call):
- Deleting a student or anonymous participant cascades to their attempts
  (history deleted with the participant).
- Deleting a quiz is blocked if any attempt exists; teacher must archive
  (close) instead. Attempts are always preserved.
- Deleting a major is blocked if any student/anonymous references it.

Indexes: PKs, UNIQUE on emails, majors.name, and quizzes.code, and on
(quiz_id), (attempt_id), (quiz_id, state), (focus_loss_events.attempt_id).
No FK constraints.

No ORM; raw SQL via database/sql. Transactions for multi-statement writes
(attempt submit, grading, major rename propagation, attempt creation).
Migrations are plain numbered SQL files in `/migrations`; each file is
applied in order inside a transaction and recorded in `schema_migrations`.
Rollback is a manual `down` section carried in the same file, applied by
hand — there is no automatic down-migration runner.

## 12. Auth and sessions

- Passwords: bcrypt cost 12, >=8 chars, no whitespace, >=1 digit + >=1
  uppercase.
- Sessions: DB-backed, 60-min sliding expiry, HttpOnly+SameSite cookie.
  No JWT. The cookie carries a random 256-bit token and only its sha256
  hash is stored (`sessions.token_hash`). The sliding expiry is refreshed
  on every authenticated request that carries a valid session.
- The `Secure` attribute is controlled by `COOKIE_SECURE` (my call). It
  defaults to **false**, because the school LAN serves the app over plain
  HTTP and a `Secure` cookie is dropped there, which would break sign-in
  and anonymous resume; production deployments set `COOKIE_SECURE=true`
  behind TLS. `HttpOnly` and `SameSite=Lax` are always on.
- One signing key (`SESSION_SIGNING_KEY`, my call) signs the
  anonymous-participant cookie and the pending-registration token.
- Rate limits (my call): login and register 5 attempts / 15 min per IP;
  focus-loss beacon 10 requests / minute per attempt; progress beacon 30
  requests / minute per attempt.

## 13. Realtime (SSE)

- Per-quiz SSE channel for teacher live monitor.
- 30s heartbeat; no replay buffer; no auto-reconnect (app-level resume).
- Fan-out budget <2s for 100 concurrent streams.
- Students POST progress beacons (CSRF-exempt, rate-limited); the quiz id
  travels in the URL path, the progress payload in the request body.
- CSRF posture: the two beacon routes (progress and focus-loss) are the
  only CSRF-exempt routes. Every other state-changing route requires the
  CSRF token that the session middleware issues alongside the session
  cookie and that every form carries as a hidden field.

## 14. Constraints

- <=500 total users, <=100 concurrent SSE streams, fan-out <2s.
- No timers in v1 (per user); focus-loss anti-cheat included (section 6).
- Single process, no cache tier, no JWT.
- All pages server-rendered (SSR); client-server communication uses URL
  parameters (query string) where applicable.
- Frontend UI/UX built exclusively from official Bootstrap components
  (mandatory; no custom component library).
- Responsive web; Bootstrap 6.0.0-alpha.1 + jQuery 4.0.0, both vendored under
  `/static/`; Plus Jakarta Sans self-hosted under `/static/fonts/`.
- UI copy language is Indonesian; spec and design artifacts stay in English.
- Caching: /static/ public 86400; /uploads/:ref private must-revalidate with
  ETag/Last-Modified (no stale content; revalidate on every request).
  Images are cacheable but always revalidated. Everything else no-store.

## 15. UI/UX system (binding for every page)

### 15.0 Normative sources and precedence

1. `docs/DESIGN.md` — the **Twisty Quiz** design system. It is the sole
   authority for colour, typography, spacing, radius, elevation, layer
   order, component shapes, component states, responsive behaviour,
   accessibility floors, and motion. Every value in section 15 is a
   restatement of, or a pointer into, that file. If the two disagree,
   `docs/DESIGN.md` wins.
2. `docs/template/LOGIN.html`, `docs/template/DASHBOARD.html`,
   `docs/template/QUIZ.html` — the three working prototypes. They are the
   reference for information architecture, screen order, and copy intent.
   Their markup, CDN links, icon fonts, and ad-hoc CSS are **not**
   normative; they are ported onto the tokens in section 15 and
   `docs/DESIGN.md` before use.
3. This section — the build-level rules that `docs/DESIGN.md` does not
   cover: the carrier, the styling mechanism, the page inventory, and the
   per-screen state contract.

Prototype features are not automatically in scope. Explicitly out of scope
for v1: AI question generation from notes, browser lockdown or hard
tab-switch blocking, and any time-extension control.

### 15.1 Carrier

- **Bootstrap 6.0.0-alpha.1** — CSS + JS bundle, vendored at
  `/static/bootstrap/`. The vendored files are never edited; no Sass, no
  `customize/`, no build step. The source of truth is the
  `assets/bootstrap-6.0.0-alpha/` tree already in the repository.
- **ESM-only loading.** The v6 dist is ES modules. The bundle is loaded as
  `<script type="module" src="/static/bootstrap/bootstrap.bundle.min.js">`.
  There is no `window.bootstrap` global and no UMD build: component
  instances are created through explicit imports
  (`import { Dialog, Drawer, Menu } from "/static/bootstrap/bootstrap.bundle.min.js"`).
  The data API (`data-bs-toggle="dialog"`, `data-bs-dismiss="dialog"`)
  still initialises automatically once the module has executed.
- **jQuery 4.0.0** (stable, full build) — vendored at
  `/static/jquery/jquery-4.0.0.min.js`, loaded as a classic deferred
  script. jQuery carries AJAX and DOM wiring **only**. Bootstrap 6 has no
  jQuery plugin bridge, so no Bootstrap component may be driven through
  `$.fn` (`$(...).modal(...)` and every other Bootstrap alias are gone);
  components are driven by `data-bs-*` attributes or by imported ESM
  classes. SSE (`EventSource`) and the focus-loss beacon
  (`navigator.sendBeacon`) are native-only.
- **Application JavaScript is allowed** — `/static/js/app.js` is an ES
  module. It carries the client behaviour the product requires: the SSE
  subscription for the live monitor, the autosave request, the focus-loss
  beacon, and the progress/toast wiring. What is banned is custom
  *styling*, not custom *behaviour*.
- **Plus Jakarta Sans** — self-hosted `woff2`. The vendored source of truth
  is `assets/fonts/plus-jakarta-sans/PlusJakartaSans-latin.woff2` — one
  **variable** file carrying the whole 200–800 weight axis, SIL OFL, with
  the licence text beside it as `OFL.txt`; the file served to the browser
  is copied to `static/fonts/plus-jakarta-sans/` at vendoring time, the
  same way jQuery is handled. It is declared once with `font-weight:
  400 700`: there is one binary, not one per weight, and the weights
  400/500/600/700 are selected from its axis.
- **No CDN in production.** Every stylesheet, script, and font is served
  from `/static/`. The school LAN must work with the internet down.
- **Exactly one token stylesheet is permitted** —
  `static/css/tokens.css`, linked from every page after
  `bootstrap.min.css`. It contains **only** custom-property declarations
  on `:root` (and `:host`), plus the two closed sets named in §15.11 (the
  state rules and the component token overrides). It defines no layout, no
  typography, and no component structure;
  it only maps the `docs/DESIGN.md` tokens onto Bootstrap 6's theme custom
  properties. It is the single place where the palette, radius scale, and
  elevation enter the cascade. The per-page `<style>` element remains
  limited to `@font-face` declarations for the vendored font files, because
  at-rules cannot be expressed as a `style` attribute.

#### 15.1.1 The v5 → v6 class map

The v6 class system is not the v5 class system. The renames below are
normative; a v5 name in any template is a defect.

| v5 | v6 |
| :--- | :--- |
| `.col-md-6` | `.md:col-6` |
| `.row-cols-md-3` | `.md:row-cols-3` |
| `.d-md-none` | `.md:d-none` |
| `.g-md-3` | `.md:g-3` |
| `.navbar-expand-md` | `.md:navbar-expand` |
| `.table-responsive-md` | `.md:table-responsive` |
| `.modal` + `.modal-dialog` + `.modal-content` | `<dialog class="dialog">` — both wrapper divs are removed |
| `.offcanvas` | `<dialog class="drawer">` — a `<div class="drawer">` does not work |
| `.dropdown-menu`, `.dropdown-item` | `.menu`, `.menu-item` — the `.dropdown` wrapper is removed |
| `.btn-primary` | `.btn-solid` + `.theme-primary` |
| `.btn-outline-primary` | `.btn-outline` + `.theme-primary` |
| `.alert-danger` | `.alert` + `.theme-danger` |
| `.badge.bg-primary` | `.badge` + `.theme-primary` |
| `.text-primary` | `.fg-primary` |
| `.text-muted` | `.fg-secondary` |
| `.text-bg-primary` | `.bg-primary` + `.fg-contrast-primary` |
| `.form-select` | `.form-control` on the `<select>` |
| `.form-check-input` | `.check` (checkbox), `.radio`, `.switch` |
| `.form-range` on the input | `.form-range` wrapper + `.form-range-input` on the input |
| `.form-switch` | `.switch` wrapper around the input |
| `.needs-validation` / `.was-validated` | `data-bs-validate` on the `<form>`; `:user-invalid` replaces `.was-validated` |
| `.invalid-tooltip` | `<div class="tooltip invalid-tooltip">` |
| `.breadcrumb` links | `.breadcrumb-link` on the `<a>`, plus empty `.breadcrumb-divider` separators |
| `.accordion-button` / `.accordion-collapse` | `.accordion-item` / `.accordion-header` / `.accordion-body` / `.accordion-icon` — v6 has no `.accordion-button` |
| `.placeholder-glow` | `.placeholder-wave` (standard) or `.placeholder-pulse` — `.placeholder-glow` still exists in v6 as a wrapper, but authored markup uses the new names |
| `.fs-1` … `.fs-6` | not used — the v6 `.fs-*` sizes are fluid `clamp()` values that do not match the type scale; sizes come from the token layer (§15.3) |
| `.p-2`, `.p-3`, `.p-4`, `.p-5` | `.p-3`, `.p-5`, `.p-7`, `.p-12` — the v6 spacer scale differs |
| `data-bs-toggle="modal"` | `data-bs-toggle="dialog"` |
| `data-bs-toggle="offcanvas"` | `data-bs-toggle="drawer"` |
| `data-bs-toggle="dropdown"` | `data-bs-toggle="menu"` |

The v6 spacer scale is `0 / 4 / 6 / 8 / 12 / 16 / 20 / 24 / 28 / 32 / 36 /
40 / 48px` for keys `0`–`12`. A mechanically ported v5 spacing class is
therefore silently wrong: v5 `.g-3` is 16px, v6 `.g-3` is 8px. The
`docs/DESIGN.md` spacing steps map to v6 as `8px → 3`, `12px → 4`,
`16px → 5`, `24px → 7`, `32px → 9`.

#### 15.1.2 Why v6, and what the alpha costs

`6.0.0-alpha.1` was published to npm on 2026-10-08 and is the only
Bootstrap tree vendored in this repository. 5.3.8 remains the published
stable line; v6 has no stable release date.

The alpha is accepted for three reasons:

1. **The design system maps onto v6's token model.** v6 is built on CSS
   custom properties: `:root` defines `--bs-primary-bg`,
   `--bs-primary-contrast`, `--bs-primary-border`, `--bs-primary-focus-ring`,
   `--bs-radius-*` and `--bs-box-shadow-*`; `.theme-primary` maps them onto
   `--bs-theme-*`, which `.btn-solid`, `.alert`, `.badge`,
   `.list-group-item.active` and the rest consume. The
   `docs/DESIGN.md` palette therefore needs exactly one token stylesheet
   (§15.11) and no Sass build. Under 5.3 the same palette needs
   per-component overrides and `!important`.
2. **The migration cost is bounded and already known.** The renames in
   §15.1.1 are mechanical, and the 25 screens do not exist yet — there is
   no v5 application markup to port except the three reference prototypes,
   which are not normative.
3. **The alpha's own risks are absorbed by this stack.** The bundle is
   ESM-only, so it is loaded as a module and component classes are
   imported explicitly. There is no jQuery plugin bridge, so jQuery is
   used for AJAX and DOM work only and never for Bootstrap components.

Two alpha-specific consequences are binding:

- **Only the bundle is vendored.** The standalone `bootstrap.js` build
  leaves Floating UI and Vanilla Calendar Pro external and needs an
  import map; the bundle includes both, so no import map is required.
- **v6 requires `oklch()` and `color-mix()` support.** The supported
  browser floor is stated in §15.4.

**Not** carried over from v5: the jQuery plugin bridge, the UMD build,
`window.bootstrap`, the `--bs-*-rgb` custom properties (use
`color-mix(in oklab, var(--bs-primary-bg), transparent 50%)`), and the
`util/backdrop`, `util/focustrap` and `util/scrollbar` modules — v6's
`Dialog` and `Drawer` use the native `<dialog>` element, which supplies
the backdrop, the focus trap, and the top layer.

### 15.2 Colour

The closed palette from `docs/DESIGN.md` §2.1. In summary:

The names below are **design-token names**, not Bootstrap class names. In
Bootstrap 6 a foreground colour is applied with `.fg-*` (`.fg-primary`,
`.fg-secondary`), a surface with `.bg-*`, and a component theme with
`.theme-*` — there is no `.text-primary` and no `.text-muted`.

- `primary` `#293681` — the single high-emphasis action fill: primary
  buttons, active nav marker, key headings, strong progress fill. White
  text on it (10.83:1).
- `primary-dark` `#1E2864` — primary button hover/active.
- `secondary` `#4274D9` — secondary actions, gradients, progress fills.
- `accent` `#E24A21` — sparse warm accent: role badges, live/attention
  markers, critical stat deltas. Never a page background, never body text
  on white.
- `accent-light` `#FDF0ED`, `accent-blue` `#5589C3`,
  `accent-blue-light` `#EEF4FA` — tint and data-mark roles.
- `info` `#95CCDD`, `info-soft` `#EAF6FA`, `tint` `#D0E7E6` — tracks,
  selected surfaces, calm secondary surfaces.
- `canvas` `#EDF0F4` — app frame background. `card` `#FFFFFF` — every
  elevated card, input, and popover. `surface-muted` `#F4F6F8`,
  `surface-pill` `#E2E7EC`. `app-bg` `#838EA0` — outer viewport.
- `text-primary` `#121721`, `text-secondary` `#687182`,
  `text-muted` `#94A3B8` (decorative marks and disabled indicators only).
- `border-light` `#E2E6EB`, `border-subtle` `#ECEFF2`.
- `success` `#237A57`, `warning` `#966000`, `danger` `#B42318`,
  `inactive` `#6B7B85`.

No hex outside this palette may appear in **authored** markup, in
`static/css/tokens.css`, or in any inline `style=""` attribute. (The
vendored Bootstrap CSS carries its own internal palette and is never
edited.) The contrast floors and the explicit restricted-pair list are
normative — see `docs/DESIGN.md` §2.3.

### 15.3 Typography, shape, spacing, elevation

Restated from `docs/DESIGN.md` §3, §4, §5, §6:

- **Family**: Plus Jakarta Sans, one family, no serif/sans pairing. Stack
  `'Plus Jakarta Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI',
  Roboto, sans-serif`.
- **Type scale** (size / line-height px): display-metric 32/38 (700),
  h1 24/30 (600), h2 20/26 (600), metric 24/28 (700), body-prominent
  16/22 (600), body 14/20 (400), label 13/18 (500), badge 11/14 (600).
  No other size or line-height value may appear. Question body text is at
  least 16px. Scores, timers, and counts use tabular numerals.
  **Delivery:** the carrier's display sizes are fluid and its heading
  line-height is unitless, so the scale is applied by the token layer
  only — `--bs-body-font-size`, `--bs-body-line-height`,
  `--bs-font-size-3xl` (h1) and `--bs-font-size-2xl` (h2) in
  `static/css/tokens.css`, plus the two heading line-heights from the
  closed component-override list (§15.11) — and by inline `font-size` /
  `line-height` for display-metric, metric, body-prominent, label and
  badge. `.fs-*` utilities and headings below `h2` are not used.
- **Spacing**: 4px base unit; common steps 8 / 12 / 16 / 24 / 32 px. Card
  padding 24px; panel gaps 24–32px.
- **Radius**: container 32px, card 24px, row/sub-card 18px, controls and
  badges 9999px. No sharp corners anywhere.
- **Elevation**: flat `0 1px 2px rgba(18,23,33,0.02)`, card
  `0 10px 25px -4px rgba(18,23,33,0.04)`, floating
  `0 14px 35px -6px rgba(18,23,33,0.08)`, tooltip
  `0 4px 12px rgba(18,23,33,0.12)`. Cards are separated by whitespace and
  a 1px border first; a shadow is the exception.
- **Layer order**: canvas (z-0) → cards (z-1) → floating cards, drawers,
  sticky bars (z-2) → tooltips, popovers, dialogs (z-3). Inline `z-index` is
  permitted only for a frozen name column.

### 15.4 Layout and responsive

- Bootstrap `container`, `row` / `col-*` / `row-cols-*` and the `g-*`
  gutter utilities only — the flexbox grid. The carrier also ships a CSS
  **grid** utility family (`.grid`, `.grid-cols-*`, `.grid-auto-flow-*`),
  and there is no `.columns`, `.gutters` or `.stack` class in it: the
  grid utilities are banned, and one grid mental model (flexbox) applies.
- App frame: centred container, max width 1280–1440px, minimum outer
  margin 24px.
- Teacher workspace: asymmetric two-column grid, narrow left rail beside a
  wider content column. Student attempt: a single focused column capped at
  960px. Auth: split screen, atmospheric brand panel beside a centred card.
- Breakpoints 768 / 1280 — the v6 `md` and `xl` breakpoints — written in
  v6 prefix syntax (`.md:col-6`, `.xl:row-cols-4`). Below 768px: single
  column, metric tiles stack
  one per row, teacher navigation becomes a drawer, the student action bar
  becomes a sticky bottom bar with
  `padding-bottom: env(safe-area-inset-bottom)` that never covers an open
  input, and wide tables scroll inside `.table-responsive`.

  The v6 breakpoint scale is `sm` 576px, `md` 768px, `lg` 1024px, `xl`
  1280px, `2xl` 1536px. There is no 1200px breakpoint, so the design
  system's 1200px "wide workspace" threshold is served by `xl` (1280px);
  `docs/DESIGN.md` §9 states the same two thresholds.
- **Dark mode is not shipped.** The app is light-only: no
  `data-bs-theme="dark"`, no `color-scheme: dark`, no `bg-dark` /
  `bg-black` / `text-bg-dark`, no authored `light-dark()`. Adding a dark
  theme is a new decision, not a builder's choice.

### 15.5 Accessibility

Target **WCAG 2.2 AA** — not AAA. AAA requires 7:1 for normal text, which
the palette cannot deliver, so claiming it would be an unsatisfiable
acceptance criterion.

- Contrast floors per `docs/DESIGN.md` §2.3, including the explicit
  restricted-pair list.
- Focus ring: the vendored Bootstrap focus ring, unmodified (3px outline,
  1px offset). `outline: none` is forbidden. The ring colour is derived
  from the palette primary and must measure ≥ 3:1 against the canvas.
- Keyboard: every control is reachable in DOM order with a visible focus
  indicator; dialogs trap Tab and restore focus to the invoking control;
  the question drawer is closable by keyboard. In v6 both are native
  `<dialog>` elements, so the top layer, the `::backdrop`, the focus trap,
  and the scroll lock come from the platform rather than from JavaScript.
- Form errors: a rejected form renders a summary alert as the **first
  focusable element inside the form** (`role="alert"`, `tabindex="-1"`),
  followed by the per-field messages. It is announced without JavaScript
  and needs no scripted focus move.
- Screen readers: every `img` has an `alt`; every input has a programmatic
  `<label>`; spinners carry `role="status"` with visually hidden text;
  progress bars carry `role="progressbar"` with `aria-valuenow` /
  `aria-valuemin` / `aria-valuemax`; the answered-count region is
  `aria-live="polite"`.
- Touch targets: primary actions on a phone are at least 44px tall and full
  width.
- Copy is sentence case.

### 15.6 Motion

Only the values in `docs/DESIGN.md` §11 are permitted: control colour-state
0.15s, collapse 0.35s, dialog/drawer/popover 0.3s, progress width 0.6s,
toast 0.15s, spinner 1.5s. Every transition is gated behind
`prefers-reduced-motion: no-preference`; under `reduce` no inline
transition or animation may re-enable motion. No ambient motion, no
entrance choreography, no decorative animation.

**Delivery.** The carrier already matches that table for control colour
states (`0.15s`), dialogs and drawers (`0.3s`), toasts (`0.15s`) and the
progress fill (`width 0.6s ease`). Two values differ — the popover is
`0.15s` in the carrier against `0.3s` here, and the spinner is `1s`
against `1.5s` here — and both tokens are declared on the component
rather than at `:root`, so they are set by the closed component-override
list in §15.11. No other transition or animation value may be introduced.

### 15.7 Page inventory

25 screens. Each maps to a template file; each template file derives its
structure from the prototype named in the last column.

| # | Screen | Template | Prototype source |
| :-- | :--- | :--- | :--- |
| 1 | Sign in | `pages/auth/login.html` | `LOGIN.html` split screen |
| 2 | Register (teacher / student) | `pages/auth/register.html` | `LOGIN.html` register form |
| 3 | Teacher pending approval | `pages/auth/pending.html` | new |
| 4 | Change password | `pages/auth/change-password.html` | new |
| 5 | Join by quiz code | `pages/auth/join.html` | `LOGIN.html` join form |
| 6 | Anonymous identity (name / grade / major) | `pages/auth/identify.html` | new |
| 7 | Profile | `pages/profile.html` | new |
| 8 | Student dashboard | `pages/student/dashboard.html` | new |
| 9 | Student history | `pages/student/history.html` | new |
| 10 | Teacher dashboard | `pages/teacher/dashboard.html` | `DASHBOARD.html` |
| 11 | Quiz list | `pages/teacher/quizzes.html` | `DASHBOARD.html` recent quizzes |
| 12 | Quiz authoring | `pages/teacher/quiz-edit.html` | new |
| 13 | Question editor | `pages/teacher/question-editor.html` | new |
| 14 | Image upload | `pages/teacher/image-upload.html` | new |
| 15 | Advanced quiz settings | `pages/teacher/quiz-settings.html` | new |
| 16 | Live monitor | `pages/teacher/monitor.html` | `DASHBOARD.html` live monitor |
| 17 | Essay grading queue | `pages/teacher/grading.html` | `DASHBOARD.html` grading rows |
| 18 | Attempt review / recap | `pages/teacher/recap.html` | `DASHBOARD.html` |
| 19 | Student management | `pages/teacher/students.html` | new |
| 20 | Anonymous participants | `pages/teacher/anonymous.html` | new |
| 21 | Majors | `pages/teacher/majors.html` | new |
| 22 | Teacher management (approve / deactivate) | `pages/teacher/teachers.html` | new |
| 23 | Quiz taking (one question) | `pages/attempt/question.html` | `QUIZ.html` |
| 24 | Review and submit | `pages/attempt/review.html` | `QUIZ.html` review view |
| 25 | Result | `pages/attempt/result.html` | `QUIZ.html` submitted view |

Plus `pages/error/404.html` and `pages/error/500.html`.

Shared chrome lives in `templates/partials/`: `head.html`, `foot.html`,
`navbar-teacher.html`, `navbar-student.html`, `drawer-teacher.html`,
`breadcrumb.html`, `empty-state.html`, `dialogs.html`, `toasts.html`,
`banners.html`, `no-js-banner.html`, `progress-bar.html`, `stepper.html`,
`icons.html`, `strings.html`.

Page shells live in `templates/layouts/`: `base.html`, `auth.html`,
`student.html`, `teacher.html`, `error.html`. A layout owns the document
skeleton and wraps the page body; a partial is a reusable fragment
rendered inside a layout. The two directories never overlap.

### 15.8 Component states

Every list, form, and monitor surface ships these as distinct Bootstrap
primitives. Silent failure is a defect.

| State | Treatment |
| :--- | :--- |
| loading | SSR first paint; `placeholder` + `placeholder-wave` skeletons only on a filter swap, with `aria-busy="true"` |
| ready | the populated surface |
| empty | `card` + `alert theme-info` with one primary action |
| error | static `alert theme-danger` + retry; the previous content stays visible |
| saving | `spinner-border` inside the disabled control, plus `role="status"` |
| saved | brief success `toast`; never a permanent marker |
| locked / expired | static `alert theme-warning`, inputs disabled |
| stale | persistent `alert theme-warning` naming that live updates stopped |
| no-JS | `noscript` `alert theme-warning` naming exactly what is unavailable |

The states a surface must ship are fixed per surface family. A state not
listed for a family is not required of it; every listed state must be a
distinct Bootstrap primitive, and a silent failure is a defect.

| Surface family | Required states |
| :--- | :--- |
| Teacher lists (quizzes, students, anonymous, majors, teachers) | loading, ready, empty, error, no-JS |
| Live monitor | loading, ready, empty, error, stale, locked / expired, no-JS |
| Grading queue | loading, ready, empty, error, saving, saved, locked / expired, no-JS |
| Authoring and settings (quiz edit, question editor, upload, settings) | loading, ready, empty, error, saving, saved, locked / expired, no-JS |
| Attempt surfaces (question, review) | ready, error, saving, saved, locked / expired, no-JS |
| Result | ready, error, locked / expired, no-JS |
| Auth and identity forms (login, register, pending, change password, join, identify) | ready, error, saving, locked / expired, no-JS |
| Student dashboard and history | loading, ready, empty, error, no-JS |

The attempt surfaces carry no `loading` state: the attempt page is
server-rendered one question at a time, so there is no filter swap to
skeleton.

### 15.9 Iconography

No external icon font and no emoji-as-icon. Icons are a self-contained
inline SVG sprite (`partials/icons.html`) using `currentColor`. Every
icon-only control carries an accessible name. No state — selected answer,
correctness, focus-loss, attempt state — is signalled by colour or icon
alone.

### 15.10 Language and copy

UI copy is Indonesian; spec and design artifacts stay in English. Every
rendered page sets `<html lang="id">` — the `lang` attribute is part of the
accessibility contract, not a detail. One string catalogue is the single
source: it lives in Go (`internal/i18n`), handlers and templates read it
by key, and `partials/strings.html` is the rendered view of that same
catalogue rather than a second copy — no copy is hard-coded in a handler
and none is duplicated in this spec. Dates and times render in WIB with a
"WIB" suffix (UTC in tooltips); payloads stay ISO-8601 UTC. The
prototypes' English copy is a layout reference, not a copy source.

### 15.11 Styling mechanism

Bootstrap classes plus inline `style=""` attributes, plus exactly one
token stylesheet. Exactly one `<style>` element per page, containing only
the `@font-face` declarations for the vendored font files. No Sass/SCSS,
no `!important`, no new class definitions, and the vendored Bootstrap CSS
is never edited. The only selectors `tokens.css` may contain are the
`:root` block and the two closed lists below; a selector may set custom
properties and nothing else, and it may never change a component's
structure, layout, or appearance beyond the token it overrides.

`static/css/tokens.css` is the only other stylesheet. It is a token
file, not a component stylesheet, and it may contain:

- `:root` (and `:host`) blocks declaring Bootstrap custom properties
  only — the `docs/DESIGN.md` palette mapped onto `--bs-primary-bg`,
  `--bs-primary-contrast`, `--bs-primary-border`, `--bs-primary-focus-ring`,
  the `--bs-accent-*` / `--bs-success-*` / `--bs-warning-*` /
  `--bs-danger-*` / `--bs-info-*` role tokens, `--bs-fg-body`,
  `--bs-bg-body`, `--bs-border-color`, `--bs-link-color`, the
  `--bs-radius-*` scale, `--bs-box-shadow-*`, `--bs-body-font-family`,
  `--bs-body-font-size`, `--bs-body-line-height`, `--bs-font-size-3xl`
  (h1) and `--bs-font-size-2xl` (h2).
- The closed set of **state rules** below, and nothing else:
  `.btn-solid.theme-primary:hover`, `.btn-solid.theme-primary:active`,
  `.btn-solid.theme-primary:focus-visible`, `.btn-close:hover`,
  `.menu-item:hover`, `.menu-item.active`, `.form-control:focus-visible`,
  `.check:focus-visible`, `.radio:focus-visible`, `.switch:focus-within`.
  Each may set only custom properties, and only to values drawn from
  `docs/DESIGN.md` §2.1.
- The closed set of **component token overrides** below, and nothing else.
  These exist because the carrier binds several design-system values to
  element-level custom properties that a `:root` override cannot reach:
  `.card { --bs-card-border-radius: 24px }`,
  `.dialog { --bs-dialog-border-radius: 24px }`,
  `.drawer { --bs-drawer-border-radius: 24px }`,
  `.alert { --bs-alert-border-radius: 24px }`,
  `.popover { --bs-popover-border-radius: 24px; --bs-popover-transition-duration: 0.3s }`,
  `.list-group { --bs-list-group-border-radius: 18px }`,
  `.btn { --bs-btn-border-radius: 9999px }`,
  `.badge { --bs-badge-border-radius: 9999px }`,
  `.pagination { --bs-pagination-border-radius: 9999px }`,
  `.form-control { --bs-control-border-radius: 18px }`,
  `.form-control:not(textarea) { --bs-control-border-radius: 9999px }`,
  `.h1 { --bs-heading-line-height: 1.25 }`,
  `.h2 { --bs-heading-line-height: 1.3 }`,
  `.spinner-border { --bs-spinner-animation-speed: 1.5s }`.
  Each may set only custom properties, and only to values drawn from
  `docs/DESIGN.md` §3, §6 and §11.

Everything else is Bootstrap classes plus inline `style=""` attributes
against the closed property list below. The token stylesheet exists
because Bootstrap 6 is driven entirely by custom properties: without it
the shipped UI would render Bootstrap's own blue (#0d6efd) and 6px radii,
which contradicts `docs/DESIGN.md` §2.1, §6, and §12.

Inline properties are limited to a closed list: `padding-bottom`
(safe-area), `width` (server-computed progress and countdown),
`position:left/right` + `z-index` (frozen name column only), `cursor`,
`overflow`, `print-color-adjust`, `aspect-ratio` / `object-fit` (question
images), `white-space`, `color` / `background-color` / `border-color`
(palette hex only), `font-family`, `font-size` / `line-height`,
`border-radius`, `box-shadow` (menus and dialogs only).

## 16. What's left to the builder

- Exact SQL migrations and index tuning (the plan fixes the schema and the
  index set; the file bodies are the builder's).
- Error matrix and edge cases (derive from flows).
- Test cases and acceptance criteria beyond the ones the plan pins.
- A Cloudflare edge. **Out of scope for v1** — the school LAN serves the
  app directly, and the plan's Task 18 ships Compose and CI gates only.
- UI conformance is verified by static analysis of the templates and the
  token stylesheet, plus unit tests over rendered HTML. There is no
  headless browser and no browser-driven e2e suite in v1, so a responsive
  claim is a claim about the classes in the markup, not about pixels.

## 17. My calls (decisions I made, not agreed by the user)

- Normalization of final score (sum / question count), displayed 0-100.
- One active in-progress attempt per participant per quiz.
- Structural edits lock once an attempt exists.
- Image worker pool size 2/100/10-min; images stored raw, no resize/crop;
  auto-compress above 5 MB, hard limit 10 MB.
- Login rate limit 5/15min.
- Anonymous participant rows are not linked to any account; identified by a
  signed httpOnly cookie that identifies the participant rather than the
  quiz, so one participant may hold attempts on several quizzes.
- First teacher auto-activates when none exist; peer approval after.
- Private quiz requires both grade level and major.
- Quiz code: random 6-digit, regenerated on collision, invalidated on close.
- Quiz deletion blocked if attempts exist (attempts preserved).
- Student/anonymous deletion cascades to attempts.
- Major deletion blocked if referenced.
- Focus-loss anti-cheat is informational (no hard block on submit).
- Per-question `points` column retained but v1 scores every question as
  1.00 (per the simplified formula).
- All pages SSR; client-server communication via URL parameters.
- Frontend uses only official Bootstrap components.
- Image caching: private must-revalidate with ETag/Last-Modified (no stale).
- **Bootstrap 6.0.0-alpha.1 rather than 5.3.8.** The alpha is published on
  npm (2026-10-08) and is the only Bootstrap tree vendored in this
  repository. Its renamed class system is a bounded, known cost
  (section 15.1.1), and its custom-property token model is what lets the
  `docs/DESIGN.md` palette ship without a Sass build (section 15.11). The
  ESM-only bundle and the missing jQuery plugin bridge are absorbed by
  loading the bundle as a module and by restricting jQuery to AJAX and
  DOM work.
- **Plus Jakarta Sans rather than Gotham Rounded.** The vendored Gotham
  files are commercially licensed (Hoefler & Co., no open grant); Plus
  Jakarta Sans is SIL OFL, matches the prototypes' actual usage, and is
  what `docs/DESIGN.md` specifies. Weights 400/500/600/700 only — the
  type scale uses no 800. The woff2 files are vendored once into
  `assets/fonts/plus-jakarta-sans/` and served from
  `static/fonts/plus-jakarta-sans/`.
  > **Superseded 2026-10-22 (user directive).** The project owner has chosen
  > Gotham Rounded anyway. The licensing risk recorded above is unresolved
  > and is accepted knowingly: the faces are shipped as woff2 under
  > `static/fonts/gotham-rounded/` with `COPYRIGHT.txt` beside them, and
  > `docs/DESIGN.md` §3 now specifies Gotham Rounded.
- **The join code lives on `quizzes.code`.** It is generated when the quiz
  opens, regenerated on collision, and nulled when the quiz closes, so a
  closed quiz's invite URL stops working.
- **Focus-loss timestamps live in `focus_loss_events`.** The denormalized
  `quiz_attempts.focus_loss_count` stays as the fast read for the live
  monitor.
- **Scores are stored on a 0–1 scale and displayed as 0–100.** The
  conversion happens in the template, never in a handler.
- **`docs/DESIGN.md` is the single design authority** for tokens, states,
  accessibility, and motion (section 15.0). The education palette is
  adopted as its `primary`; the Twisty neutrals, geometry, and type scale
  are retained; the freelance-product components are replaced.
- **UI copy is Indonesian**; spec and design artifacts stay in English.
- **Dark mode is not shipped** (section 15.4).
- **One token stylesheet, no component stylesheet.** All visual styling is
  Bootstrap classes plus inline `style=""` against a closed property list,
  plus `static/css/tokens.css`, which carries only custom-property
  declarations and a closed set of state rules (section 15.11).
- **Teacher accounts are deactivated, never deleted** (section 3): a peer
  teacher may set `is_active = false`, which revokes access and keeps that
  teacher's quizzes, attempts and grades intact.
- **Result release depends on grading** (section 9): an all-objective quiz
  releases its result at `submitted`; a quiz with essays releases it at
  `graded`, and the student sees the counted attempt.
- **The formula is the default, the teacher decides** (section 8): the
  grading form pre-fills the computed final score and the teacher may
  adjust it. `counted` marks at most one attempt per participant per
  quiz.
- **`COOKIE_SECURE` defaults to false** (section 12) because the school
  LAN serves the app over plain HTTP; production sets it true behind TLS.
- **One signing key** (`SESSION_SIGNING_KEY`) signs the
  anonymous-participant cookie and the pending-registration token.
- **Rate limits are pinned** (section 12): login and register 5 per 15
  min per IP, focus-loss 10 per minute per attempt, progress 30 per
  minute per attempt.
- **The token stylesheet carries a closed component-override list**
  (section 15.11) because the carrier binds radius, heading line-height
  and two motion durations to element-level custom properties that a
  `:root` override cannot reach.
- **The typeface ships as one variable file** (section 15.1):
  `PlusJakartaSans-latin.woff2`, declared with `font-weight: 400 700`.
- **The string catalogue lives in Go** (section 15.10);
  `partials/strings.html` is its rendered view.

---

What's wrong or missing in this?