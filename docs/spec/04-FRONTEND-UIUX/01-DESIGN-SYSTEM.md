# Design System — quiz-v2 (Bootstrap full + inline CSS only)

Status: DRAFT | Owner: 04-FRONTEND | Updated: 2026-10-14 (Q12 token authority DECIDED: §3A pins the type/motion/timing scale, §5 pins the accessibility values, `docs/DESIGN.md` marked NON-authoritative per UI-009; §2.8 anchors repointed)

Revision R7 adds DEC-114 (full jQuery 4.0.0 usable, supersedes DEC-080) and DEC-128 (no-JS banner three items) on top of R6/R5. Visual brand
values stay DECIDED (UI-006/UI-007); client carrier rule stays DECIDED
(UI-002); no open theme or carrier decisions remain.

Source catalog: `docs/BOOTSTRAP-V6-DOCS.md` (URL list, Bootstrap 6.0). Every
row in §4 names the exact catalog section. If a catalog section is not listed
in §4, the omission is a spec defect.

## 0. Library and styling decisions (DECIDED, normative)

| ID | Decision | Justification |
|----|----------|---------------|
| UI-001 | Bootstrap 6.0 full (CSS + bundled JS), VENDORED from the Go binary (`/static/bootstrap/bootstrap.min.css`, `/static/bootstrap/bootstrap.bundle.min.js` with Popper included). OBSERVED vendored source is `assets/bootstrap-6.0.0-alpha/` (`v6.0.0-alpha.1` banner in the bundle) — the spec's "6.0" means this exact vendored tree; the 6.0.x final upgrade (if ever) is a build-time record, never a silent swap. No CDN in prod, no version drift between envs. | Exam-day pages must not depend on third-party uptime; keeps the single-Docker-image promise (ARCH-001/006); school LAN keeps working when the internet does not. |
| UI-002 | Bootstrap JS bundle + jQuery 4.0.0, BOTH loaded on every page (Bootstrap first, then `/static/jquery/jquery-4.0.0.min.js` vendored from OBSERVED `assets/jquery-4.0.0/jquery-4.0.0.min.js`, 78,749 bytes, full build per `docs/JQUERY-DOCS.md` URL catalog); SSR HTML is complete without JS (EDGE-045); JS only enhances. jQuery is a FULL carrier (DECIDED DEC-114, supersedes DEC-080): full jQuery 4.0.0 usable, including Bootstrap modal/toast/collapse/tab methods, carries answer saves, filter swaps, grading saves, dialog/toast/drawer/validation wiring. NATIVE-ONLY (never jQuery): SSE (`EventSource`), cheat beacon (`navigator.sendBeacon` with `fetch keepalive:true` fallback), SPA/router (no client router). No npm/vite/webpack/parcel build step in MVP. | jQuery removes AJAX boilerplate (CSRF, idempotency keys, retry) + DOM verbosity while SSR-complete-without-JS stays; vendored files keep the single-Docker-image promise with no CDN in prod. |
| UI-003 | NO custom stylesheet. NO Sass/SCSS compilation. NO `customize/sass`, `customize/theme`, or theme-override file. The vendored `bootstrap.min.css` is NEVER edited. Exactly ONE `<style>` element is permitted per rendered page and it MUST contain ONLY `@font-face` declarations for Gotham Rounded (zero selectors, zero visual rules) — the sole exception to inline-only, because at-rules cannot be expressed in a `style` attribute. | User directive: custom style via inline CSS only. Single rule, zero tooling, zero cascade conflicts, trivial code review (every override is visible at the element). |
| UI-004 | ALL visual customization beyond Bootstrap classes uses INLINE `style=""` attributes ONLY. Allowed: per-element tweaks that Bootstrap classes cannot express (safe-area insets, frozen table column offsets, progress widths set from server data, countdown bar widths, print-only receipt tweaks). Forbidden: introducing class names (`.btn-quiz`, `.card-quiz`), `@media`/`:hover` rules outside Bootstrap utilities, `!important` anywhere, and any selector-based override of Bootstrap (`.btn-primary { ... }`). Dynamic widths from server data (progress/timers) MUST be inline `style="width: N%"` (server-computed N), never a generated class. | Inline-only keeps every deviation local, greppable (`grep 'style="'`), and incapable of breaking unrelated pages. Review rule: any PR adding a `<style>` tag or `.css` file (except the vendored Bootstrap pair) fails review. |
| UI-005 | Iconography: Bootstrap-native text + `badge` words only; no external icon font, no emoji-as-icon in MVP. | Minimal vendored footprint; semantics carried by words (screen-reader safe). |
| UI-006 | Typography: Gotham Rounded, self-hosted from the repo (`assets/gotham-rounded/*.otf`, DECIDED DEC-040/041). Vendored into the image at `/static/fonts/gotham-rounded/` with a per-page `@font-face` block as the SOLE permitted `<style>` content (UI-003 exception). Weight mapping: `gothamrnd_bold` headings/metric/display, `gothamrnd_medium` buttons/labels/body-strong, `gothamrnd_book` body/controls, `gothamrnd_light` large display only; italics (`*italic`) for citations/secondary emphasis only, never for errors or timers. Stack: `"Gotham Rounded", "Gotham Rounded Book", system-ui, sans-serif` with tabular numerals for timers/scores. No external font CDN, no Manrope/Inter webfont (DESIGN.md stacks superseded for shipped product; system stack remains the no-font fallback). | User-supplied font files; calm teacher-friendly rounded voice; offline-safe. |
| UI-007 | Palette + neutrals + semantics + surfaces (DECIDED DEC-040; adopted from `docs/DESIGN.md` §3, which is NON-authoritative since 2026-10-14 — the values HERE are the authoritative copy): `brand-900 #086288` primary actions/active nav/key headings/strong progress (white text on it); `brand-600 #4D8DBA` secondary/informational; `brand-200 #97D7FF` tracks/selected surfaces (never small white text on it); `accent-100 #FFEFC4` single Daily Pulse reminder only (never page background); canvas `#F6F9FB`, surface `#FFFFFF`, subtle `#F0F5F8`, border `#DCE6EC`, text `#172B36/#5C6F7B/#71828C` (muted never for essential instructions), scrim `rgba(13,39,53,.42)`; success `#237A57`, warning `#966000`, error `#B42318`, inactive `#6B7B85`. Applied as inline `style=""` values per UI-004 (still no theme file, no stylesheet). Contrast: white-on-`brand-900`, `text-primary`-on-surface/canvas, error/success/warning text-on-tint — verified by AC-215 contrast check before FINAL (§5 pins the ratios). | Exact user-supplied DESIGN.md tokens; calm blue workspace with rare warm counterpoint. |

| UI-008 | DESIGN.md non-goals are binding exclusions (DECIDED DEC-040/041 context): no leaderboards/public rankings, no decorative charts, no notification center, no ambient motion (DESIGN.md §13). Analytics stay personal-scope only (teacher sees own quizzes; students never see other students' scores — RBAC R1/R2, API-070). Pulse is the single permitted attention surface (at most one reminder). | Prevents scope creep from design inspiration; keeps fairness and privacy guarantees intact. |
| UI-009 | **Token authority (DECIDED Q12, user 2026-10-14 — "either source may be authoritative"; the recommendation is adopted): THIS FILE (`04-FRONTEND-UIUX/01-DESIGN-SYSTEM.md`) is the SOLE authoritative design-token contract for spec, templates, and tests.** `docs/DESIGN.md` is a non-authoritative inspiration source: its §3 palette and §3.4 type-scale values are adopted verbatim into UI-006/UI-007 and §3A below, and NO token, motion, timing, or accessibility value may be read from `docs/DESIGN.md` anywhere in the spec, in templates, or in tests. AC-215 asserts the exact hex set in UI-007, the type scale and motion/timing values in §3A, and the accessibility values in §5 of this file — never a value cited from `docs/DESIGN.md`. Any future token change is a DEC entry in THIS file; `docs/DESIGN.md` carries no normative force. | One authority removes the two-source contradiction (F-42): `metric-lg` was a range in DESIGN.md while AC-215 needs exact pairs — §3A pins the single point, so every asserting test reads one file. |

Conformance tests: AC-215 (contrast + type/motion/accessibility audit: exact UI-007 hex pairs, §3A type scale + motion/timing values, §5 accessibility values). AC-217 (inline-only audit: at most one `<style>` per page containing ONLY `@font-face` for Gotham Rounded; `grep -r '\.css' templates/` matches only the two vendored Bootstrap files; `<script src>` vendored-only set = `bootstrap.bundle.min.js` + `/static/jquery/jquery-4.0.0.min.js` ONLY (no CDN script, no other local JS); full jQuery 4.0.0 usable per UI-002 (DEC-114, no method list); every `style=""` property in the §2 closed list; every colour hex in `style=""` from the UI-007 exact set; `font-family` only the UI-006 stack). AC-218 (full-mapping audit: every §4 row has at least one template reference or an explicit NOT-RENDERED justification in the same row).

## 1. Principles (unchanged, Bootstrap-bound)

- Mobile-first responsive SSR: every page is a Bootstrap `container` + `grid`
  layout usable at 360 px without horizontal scroll (except the explicitly
  scrollable `.table-responsive` monitor/recap wrapper). Phone question
  actions use `btn-lg`; option rows are full-width tappable labels.
- Server-rendered truth: HTML complete without JS; Bootstrap bundle + full
  jQuery (UI-002, DEC-114) enhance only; native-only are SSE/`EventSource`,
  `sendBeacon`, and SPA/router. The no-JS banner (EDGE-045, `alert-warning`)
  states exactly what is unavailable (cheat detection, SSE push, toasts) per EDGE-045/DEC-128.
- Honest states via Bootstrap primitives: `alert` (banners/errors), `spinner`
  / `placeholder` (loading/skeleton), `toast` (transient confirmations),
  `dialog` static (blocking modals), `badge` (counts/chips), `progress`
  (position/time supplement). Silent failure is a defect.
- Bahasa Indonesia UI strings (DECIDED: product language is Indonesian; DESIGN.md
  §12 "concise international English" is superseded for shipped UI copy —
  spec artifacts stay in English). String
  catalogue lives with Go templates (single source), never duplicated here.

## 2. Inline-CSS rule (normative: what may and may not be inline)

Allowed inline properties (closed list; anything else needs a DEC update):
`padding-bottom` (safe-area bars only), `width` (progress/countdown bars from
server data only), `position:left/right` + `z-index` (frozen name column only),
`cursor`, `overflow`, `print-color-adjust` (receipt print only),
`aspect-ratio`/`object-fit` (question images only), `white-space` (chips/labels
only), `color`/`background-color`/`border-color` (UI-007 exact hex ONLY —
no other hex anywhere), `font-family` (Gotham Rounded stack, UI-006, set once
on `body` plus metric/display overrides), `font-size`/`line-height`
(type scale in §3A — text-xs 12/16 through metric-lg 36/39.6), `border-radius`
(cards 15 px, inputs/buttons 11 px per §3A), `box-shadow` (menus/dialogs
only, sparing). Values MUST be literal, server-computed numbers, or UI-007 hex;
no other colour hex, no new font names. Example allowed:
`<div class="progress-bar" role="progressbar" style="width: 30%; background-color: #086288">`.
Example forbidden:
`<style>.btn-primary{background:#123456}}</style>` or a new
`quiz.css` file — both fail AC-217.

Forbidden globally: `<style>` elements except the single `@font-face`-only
block (UI-003/UI-006), `<link>` to any CSS except the two
vendored Bootstrap files, new class definitions, ID selectors for styling,
`!important`, overriding Bootstrap variables via `customize/css-variables`
(explicitly NOT used — UI-007 values apply inline per element).

## 3. Breakpoints and layout (Bootstrap `layout/*` only)

`layout/containers` + `layout/grid` + `layout/columns` + `layout/gutters` +
`helpers/stacks` are the only layout mechanism (`layout/css-grid` is
available but the 12-column `grid` is canonical; `css-grid` utilities are
NOT used to avoid two competing grid mental models — documented choice).

| Viewport | Container / grid | Notes |
|----------|------------------|-------|
| Phone (xs/sm) | `.container` single column; question action bar = sticky bottom bar (`navbar` + `position-sticky bottom-0` + inline `style="padding-bottom: env(safe-area-inset-bottom)"`); analytics `vstack gap-3`; review single column. | No horizontal scroll except `.table-responsive`. |
| Tablet (md) | `.container-md` centred; review `row-cols-1 row-cols-md-2`; dashboards 2-col stat cards. | — |
| Desktop (lg+) | `.container-lg` max ~960 px for quiz flow; `.container-fluid` ONLY for teacher monitor/recap/user tables; dashboards `row-cols-1 row-cols-md-2 row-cols-lg-4`; teacher shell = sidebar `drawer` on lg, top `navbar` + `drawer` on smaller (`examples/sidebars`, `examples/drawer-navbar`, `examples/navbars-drawer`). | — |

`layout/breakpoints` govern every responsive class; `layout/z-index` order
is Bootstrap default (dialog > toast > tooltip/popover > sticky bar);
`layout/utilities` + `utilities/display|flex|gap|margin|padding|position|
overflow|visibility|float|width|height|shadows|border|text-*|vertical-align`
carry all remaining layout needs (exact closed-world list in §4).
Sticky bars MUST NOT cover inputs when the
keyboard opens (implementation keeps the R2 visual-viewport note).

## 3A. Pinned design tokens (DECIDED Q12 — sole authority, adopted from `docs/DESIGN.md` which is now NON-authoritative per UI-009)

Every value below is the exact, testable number AC-215 asserts. No range, no
"or default": the DESIGN.md ranges were collapsed to one point so a grep can
verify a template against a single literal. `docs/DESIGN.md` may no longer be
cited as a token source.

Type scale (font-size / line-height, px; inline `font-size` + `line-height` only):

| Token | Size / line-height | Use |
|-------|--------------------|-----|
| `text-xs` | 12 / 16 | Compact metadata, timestamps, card tags. Never for critical instructions. |
| `text-sm` | 14 / 20 | Secondary descriptions, labels, helper text. |
| `text-md` | 16 / 24 | Standard body text and controls (question text, options). |
| `text-lg` | 18 / 26 | Card titles, question titles where needed. |
| `heading-sm` | 20 / 28 | Section headings. |
| `heading-md` | 24 / 32 | Page headings. |
| `metric-lg` | **36 / 39.6** | Key dashboard metrics ONLY — the single pinned point replacing DESIGN.md's "32–40 / 1.1" range. Exactly one `font-size` literal (`36`) and one `line-height` literal (`39.6`) may appear on a metric element; nothing else in this role. |

Motion and timing scale — the ONLY animation values permitted anywhere (inline
or Bootstrap class). Values marked **OBSERVED** are the vendored Bootstrap
6.0.0-alpha tokens in `assets/bootstrap-6.0.0-alpha/css/bootstrap.css`, read
directly, so no invented duration is carried here; every transition in the
bundle is already gated behind `@media (prefers-reduced-motion: no-preference)`.
A new motion value requires a DEC update — AC-217 fails the build on any inline
`transition`/`animation` not in this table and not already in the bundle:

| Purpose | Value | Source |
|---------|-------|--------|
| Generic fade (`.fade`) | 0.15 s `linear`, opacity | OBSERVED `--bs-transition-fade-duration/timing` |
| Toast show/hide | 0.15 s `linear`, opacity + display | OBSERVED `--bs-toast-transition-*` |
| Form-control / check / radio / switch colour-state transition | 0.15 s `cubic-bezier(0.34, 1.56, 0.64, 1)` | OBSERVED `--bs-control-transition-duration/timing` |
| Collapse expand/collapse (grading row, editor fieldset, monitor row detail) | 0.35 s `ease`, block-size + display | OBSERVED `--bs-transition-collapse-*` |
| Accordion panel (result per-question breakdown) | 0.2 s `ease`, content-visibility + block-size | OBSERVED `--bs-accordion-panel-transition-*` |
| Dialog / drawer / popover open-close | 0.3 s `cubic-bezier(0.22, 1, 0.36, 1)`, opacity + transform + visibility | OBSERVED `--bs-dialog-transition-*` + `--bs-transition-timing-overlay` |
| `progress`-bar width fill | 0.6 s `ease`, width | OBSERVED `--bs-progress-bar-transition` |
| `hover-lift` card lift | 0.2 s `ease`, transform + box-shadow | OBSERVED `--bs-hover-lift-transition-*` |
| Carousel onboarding fade | 0.6 s `ease`, opacity | OBSERVED `--bs-carousel-fade-*` |
| Placeholder pulse/glow + spinner under reduced-motion | animation → `none`; spinner speed 1.5 s | OBSERVED `@media (prefers-reduced-motion: reduce)` blocks |
| Toast visible duration (autohide; errors excepted) | **5000 ms** | SPEC PIN (not a Bootstrap value) — `close-button` stays visible; error toasts stay until dismissed |
| Countdown / `role=timer` announce interval | **5000 ms** | SPEC PIN (not a Bootstrap value) — throttled live region per §5 |

Radii (SPEC PINS — the DESIGN.md ranges "14–16"/"10–12" collapsed to one point
each so AC-215 has an exact literal): cards **15 px**; inputs/buttons **11 px**;
progress bars **11 px**. The vendored Bootstrap radius scale is NOT applied to
cards/inputs because UI-007/UI-006 drive those via inline `border-radius`.

Breakpoints (Bootstrap `layout/breakpoints` — OBSERVED px values from the
vendored `sm/md/lg/xl/xxl` media queries: `576/768/992/1200/1400`; the drawer
breakpoints are `576/768/1024/1280/1536`, matching the same scale): the quiz flow
uses `container` / `container-md` / `container-lg`; `.container-lg` is capped at
~960 px for the quiz flow; `.container-fluid` ONLY for teacher monitor/recap/user
tables. Phone widths under 360 px are out of the supported envelope (A2 + REQ-009).

Dark mode: **NOT SHIPPED** (UI-008/§7 — no `color-modes`, no dark palette, no
`color-scheme` beyond Bootstrap's own default). Note: the vendored bundle
already contains a `light-dark()` token layer (OBSERVED), but the spec does not
activate or ship a dark mode; dark tokens are not pinned because the mode does
not exist, and a dark theme would be a new DEC entry.

## 4. FULL component catalog mapping (every catalog section → quiz-v2 usage)

Notation: USED = rendered in MVP with the stated quiz usage; MECHANISM =
supporting layer (no direct render). No catalog section is left unmapped.

### 4.1 Components (`components/*`)

| Catalog | Status | quiz-v2 usage (concrete screens) |
|---------|--------|----------------------------------|
| `components/accordion` | USED | Result per-question breakdown (`C-004`), grading-row detail (`C-007`), help/FAQ blocks on login/join. One `accordion-item` per question/row; server-rendered open state only for the failed/pending item. |
| `components/alert`, `components/alerts` | USED | 422 field summary, 503 banner, offline banner, persistent dead-stream notice (F-002 — static, never dismissible while the stream is dead), no-JS banner, empty-queue celebration, per-question locked/expired notices (`C-002`), waiting-room ended state. Dismissible (`close-button`) ONLY where non-blocking; blocking states are static. |
| `components/avatar` | USED | Roster/monitor/history initials (`C-005`, `C-006`, history cards). Decorative (`aria-hidden="true"`); the name text beside it is the accessible label. Inline `style` limited to `width/height` square lock where the grid needs it. |
| `components/badge` | USED | ALL chips: Dibuka/Ditutup, Terlihat/Disembunyikan, Menunggu dinilai/Final, Curang N, Otomatis tersubmit, `attempt_no`, roster count, pending-essay count. Semantic colours only (`text-bg-primary`, `text-bg-success`, `text-bg-warning`, `text-bg-danger`, `text-bg-secondary`). |
| `components/breadcrumb` | USED | Teacher management trail (Quizzes › Edit › Question 4; Users › Edit; Recap › Grading). Student quiz flow does NOT use breadcrumbs (linear flow, back-button safe) — explicit scope. |
| `components/button`, `components/buttons`, `components/button-group` | USED | Every action: sticky question bar (Sebelumnya/Berikutnya/Simpan, `btn-lg` on phone), monitor controls (start/pause/stop/finish, each opening a `dialog` confirm), `btn-group` for timer-mode radio-style segmented choice + dashboard window filter (3/7/30d/all). Destructive = `btn-danger`, never custom red. |
| `components/card` | USED | Quiz card, stat card, history card, result card, waiting-room card, auth card (`examples/sign-in` shape), empty-state cards. Clickable cards use `helpers/stretched-link` on the primary action link only. |
| `components/carousel` | USED | Onboarding/tutorial slides ONLY: student first-login 3-slide how-to (daftar › jawab › aturan fokus) and teacher help slides. NEVER for question images (BR-010 = single static `img-fluid`) and never for quiz content — this bounds the carousel to non-exam surfaces so it cannot interfere with timers/focus. Indicators + prev/next native buttons; `prefers-reduced-motion` disables autoplay (no autoplay in MVP at all). |
| `components/close-button` | USED | Dismiss control inside dismissible `alert`s and `toast`s only. NEVER on blocking `dialog`s (cheat/pause/submit-confirm have explicit buttons). |
| `components/collapse` | USED | Grading-row expand, monitor row detail on phone, question-editor per-question fieldsets, recap per-attempt detail rows. `aria-expanded`/`aria-controls` server-rendered correctly on first paint. |
| `components/dialog` | USED | THE blocking primitive: cheat warning (count N, single "Mengerti, kembali"), pause blocker (static, no Esc, no dismiss), submit confirm ("Kumpulkan sekarang?" — consequence stated), two-step button destructive confirms for delete user/quiz and stop/finish (explicit consequence buttons, no typed text, DEC-042). Focus trap + return-focus per C-010. |
| `components/drawer` | USED | Teacher sidebar navigation (Quizzes, Monitor, Grading, Recap, Users, Profile) — `drawer` on lg as sidebar, off-canvas `drawer` via `navbar` `toggler` on smaller (`examples/drawer`, `examples/drawer-navbar`, `examples/sidebars`). Student flow has NO drawer (top `navbar` only, linear flow). |
| `components/list-group` | USED | Review rows (`C-003`, one item per question with answered/unanswered `badge` + jump link), waiting roster (`C-005`), grading queue (`C-007`), history list, notification-ish event lists (`examples/list-groups`). Actionable rows use real `<button>`/`<a>` inside items. |
| `components/menu`, `components/menus` | USED | Profile/account dropdown in `navbar` (Profile, Change password, Logout), quiz-card overflow actions (Edit, Open/Close, Show/Hide, Token, Delete) with server-rendered items per role/ownership. Keyboard navigable; never the only path (every menu item has a page equivalent). |
| `components/nav`, `components/navs`, `components/navs-tabs`, `components/tab`, `components/nav-overflow` | USED | Dashboard section tabs (student: Dibuka › Terdaftar › Ditutup; teacher: monitor tabs per quiz), `nav-overflow` scrollable tab bar on phone. `tab` panes are server-rendered (no JS-required tab state for content availability; JS only enhances switching). |
| `components/navbar` | USED | Global top bar on every page: brand wordmark (text until DEC-041), role links, connection badge (`C-010`), profile `menu`. Bottom sticky question bar reuses `navbar` bottom pattern (`examples/navbar-bottom`, `examples/sticky-footer-navbar`). |
| `components/pagination` | USED | Recap, grading queue, history, user management (`page/per_page`, API-062/070). Server-rendered links (`examples` pagination shape); no JS paging. Current page `aria-current="page"`. |
| `components/placeholder`, `components/placeholders` | USED | Skeleton swaps: dashboard window-filter aggregates, monitor stale shimmer, grading save row, question `saving` shimmer. Skeletons mirror final card/list/table shapes; `aria-busy="true"` during swap. |
| `components/popover`, `components/popovers` | USED | Non-critical explanations: timer-mode help, shuffle explanation, code-format hint, grade-range hint, destructive-action explanation. NEVER the only carrier of critical info (touch/no-JS safe — the same text exists inline or in `tooltip` title). |
| `components/progress` | USED | Question position bar (Q3/10), total-timer bar, matching session progress, upload/compress progress. Widths are server-computed inline `style="width: N%"`. The countdown TEXT (`role=timer`) is always present — the bar is supplementary, never the only time signal. |
| `components/scrollspy` | USED | Teacher question-editor long-page section nav (metadata › questions › settings) ONLY. Never for the student quiz flow (linear pages, no scroll sections). |
| `components/spinner`, `components/spinners` | USED | `saving` buttons, SSE initial-connect indicator (first load only — no reconnect exists, DEC-102), grading per-row save, monitor stale indicator, dashboard filter reload. `role="status"` + visually-hidden text ("Menyimpan…", "Memuat…"). |
| `components/stepper` | USED | Question progress indicator on the question page (`C-002`): reflects server `current_index` (Step 3 of 10), supplementary to the `progress` bar; display-only, never drives navigation. |
| `components/toasts` | USED | Transient confirmations ONLY: saved ✓, submitted, revoked (`C-010`, polite live region, autohide except errors). There is NO `graded-live` toast and NO `reconnected` toast — grades surface on next render (DEC-104) and stream recovery is a user refresh (full page load, DEC-102), never events worth toasting. Blocking states use `alert`/`dialog`, never toasts. Stacked top-end; `close-button` included. |
| `components/toggler` | USED | `navbar`/`drawer` collapse control on small viewports (teacher shell). Single purpose; not reused as a generic toggle (toggles are `forms/switch`). |
| `components/tooltip`, `components/tooltips` | USED | UTC title on WIB times, `badge`/chip explanations, icon-less action clarifications, code/grade hints. Hover/focus only; critical info is always visible text elsewhere. |

### 4.2 Forms (`forms/*`)

| Catalog | Status | quiz-v2 usage |
|---------|--------|---------------|
| `forms/overview`, `forms/field`, `forms/form-control`, `forms/layout` | USED | Every input in the system. `field` + `form-control` + `form/layout` grid + `gutters` on all auth/profile/quiz/question/grade/code/name forms. |
| `forms/radio` | USED | Single-choice answers (native radios in `list-group-item` labels), timer-mode select, visibility select where radio-group fits. |
| `forms/checkbox` | USED | Multiple-choice answers (native checkboxes in labels). |
| `forms/switch` | USED | Boolean quiz settings (shuffle_q/a, visible, show_score, show_correct, token_enabled) + confirm-style toggles. Native checkbox semantics. |
| `forms/range` | USED | Quiz-settings sliders: `timer_seconds` (bounded per BR-003, with numeric `form-control` mirror) and `max_attempts` 1–10 stepper-slider. Server validates; slider is convenience, the number input is truth. Essay grading does NOT use range (exact 0–10 integer input required). |
| `forms/input-group` | USED | Code entry (`/join` with `form-adorn` + submit), grade input (0–10 + `/10` adornment), search/filter inputs on user/recap pages, password show/hide. |
| `forms/form-adorn` | USED | Adornments: seconds (`dtk`), `/10` grade, `+62`-style neutral prefixes where needed, code-length hint. |
| `forms/floating-labels` | USED | Auth (login/register), profile, join-identify (name/class) — compact phone-friendly labels. Quiz answer inputs do NOT float (options must stay fully visible). |
| `forms/chips` | USED | Selected-option summary chips on the review page (`C-003`) + monitor last-answer summary chips. Display + remove-to-jump (review) semantics. |
| `forms/combobox` | USED | Class picker (`class_label` datalist) + grade-level picker (10/11/12) on register/profile/user-create; quiz pickers where bounded choice fits. Native `<select>`/datalist fallback preserved no-JS. |
| `forms/validation` | USED | THE error style: `needs-validation` + `is-invalid` + `invalid-feedback` server-rendered on 422, `aria-describedby` wiring, summary `alert` with focus move (C-009). Client constraint validation is supplementary; server is truth (ERR-100 series). |
| `forms/password-strength` | USED | Advisory meter on register + password change + forced rotation. Advisory ONLY — never blocks beyond ERR-110; server minimum is the gate. |
| `forms/datepicker` | USED | Date filter for teacher recap/history exists as: `submitted_at` day picker on recap + history (supplements, never replaces, the 3/7/30d/all window buttons). No new business concept (quiz scheduling is still manual open/close — datepicker filters existing rows only). |
| `forms/otp-input` | USED | Join-code entry UX (`/join?code=`, 6–12 alnum): segmented code boxes bound to ONE hidden `form-control` submitted value (server validates BR-040; paste/type full code supported no-JS via the single input). NOT used for auth OTP (no OTP requirement exists — this is code-entry UX reuse, documented to prevent misreading). |
| `forms/combobox` date/time variants | MECHANISM | Time values render as text + `tooltip` (no time-picker widget; server owns time). |

### 4.3 Content, layout, helpers, utilities (`content/*`, `layout/*`, `helpers/*`, `utilities/*`)

| Catalog | Status | quiz-v2 usage |
|---------|--------|---------------|
| `content/reboot` | MECHANISM | Bootstrap base reset ships with the vendored CSS; no action, no override (UI-003 forbids editing it). |
| `content/typography`, `content/prose` | USED | All headings/body; question prompts (`h1.h4` scale, focus target on advance); essay `prose` rendering with wrapping. |
| `content/images` | USED | Question images (`.img-fluid`, full-width phone, inline `style` for `aspect-ratio/object-fit` only), avatar-adjacent figures. |
| `content/tables` | USED | Monitor, recap, users, grading tables (`.table .table-striped .table-hover .table-responsive` + sticky header + frozen name column via utilities + minimal inline offsets). |
| `layout/*` (all eight) | USED/MECHANISM | §3 is the binding; `css-grid` listed but NOT used (single grid model decision, §3). |
| `helpers/focus-ring` | USED | The ONLY focus style (banning bare `outline:none`). |
| `helpers/stretched-link`, `helpers/stacks`, `helpers/position`, `helpers/visually-hidden`, `helpers/screen-readers`, `helpers/text-truncation`, `helpers/icon-link`, `helpers/vertical-rule`, `helpers/hover-lift` | USED | Cards (`stretched-link`), card rows (`stacks`, `vertical-rule` dividers), sticky/frozen (`position`), live regions/counters (`visually-hidden`, `screen-readers`), long titles (`text-truncation` + full-title tooltip), dashboard card affordance (`hover-lift`, reduced-motion off), text links (`icon-link` without icon font — text + arrow char). |
| `utilities/*` | USED (closed-world) | The §3 set (`display`, `flex`, `gap`, `margin`, `padding`, `position`, `overflow`, `visibility`, `float`, `width`, `height`, `shadows`, `border`, `text-*`, `vertical-align`) plus `align-*`, `justify-*`, `place-items`, `grid`, `colors`, `background`, `border-color`, `border-radius`, `opacity`, `object-fit`, `aspect-ratio`, `text-alignment`, `text-decoration`, `text-transform`, `text-wrapping`, `font-*`, `line-height`, `link`, `pointer-events`, `user-select`, `z-index`, `container-queries` (container-responsive cards only), `space`, `divide` (stack dividers only), `theme`, `color-scheme` (light only), `api`/`rtl`/`motion` (mechanisms, not renders). Any utility not named here requires a DEC note before use. |

### 4.4 Guides, examples, customize, migration (`getting-started/*`, `guides/*`, `examples/*`, `customize/*`, `migration`, `docsref`)

| Catalog | Status | quiz-v2 treatment |
|---------|--------|-------------------|
| `getting-started/install`, `guides/npm`, `guides/parcel`, `guides/vite`, `guides/webpack`, `guides/build-tools` | NOT USED as tooling (no build step, vendored files) — install = copy two Bootstrap files into `/static/bootstrap/` + `jquery-4.0.0.min.js` into `/static/jquery/` from OBSERVED `assets/jquery-4.0.0/`. jQuery API surface used is the full UI-002 set (DEC-114); the full API catalog in `docs/JQUERY-DOCS.md` is reference-only (native-only SSE/`EventSource`, `sendBeacon`, SPA/router stay native). |
| `getting-started/javascript`, `getting-started/css-variables`, `getting-started/accessibility`, `getting-started/browsers-devices`, `getting-started/approach`, `getting-started/ai` | MECHANISM | Rules followed (bundle loading, a11y §5, browser matrix = evergreen mobile/desktop, mobile-first approach); `ai` docs carry no runtime effect. |
| `examples/sign-in` | USED | Auth pages shape (login/register/forced rotation/join). |
| `examples/dashboard`, `examples/sidebars`, `examples/drawer`, `examples/drawer-navbar`, `examples/navbars*`, `examples/navbar-*`, `examples/list-groups`, `examples/breadcrumbs`, `examples/buttons`, `examples/dialogs`, `examples/headers`, `examples/footers`, `examples/sticky-footer*`, `examples/grid`, `examples/cheatsheet`, `examples/jumbotron(s)`, `examples/checkout`, `examples/starter-template` | USED as shell references | Teacher shell (`dashboard`+`sidebars`+`drawer`), student shell (compact `jumbotron`-restraint hero + `headers`/`footers`), forms pages (`checkout` field grouping reference for long settings forms), `starter-template` page skeleton, `cheatsheet`/`grid` spacing QA. |
| `examples/album`, `examples/blog`, `examples/pricing`, `examples/product`, `examples/cover`, `examples/carousel`, `examples/masonry`, `examples/features`, `examples/heroes`, `examples/menus(nav-marketing)` | NOT USED | Marketing/showcase/demo surfaces (incl. the `carousel` demo page — the `components/carousel` onboarding usage above is separate and bounded); a school tool has no marketing page. Revisit needs a documented, user-validated requirement recorded as a new DEC entry — none exists today. |
| `customize/*` (`overview`, `color`, `color-modes`, `components`, `css-variables`, `optimize`, `options`, `rtl`, `sass`, `theme`, `transitions`) | NOT USED as build inputs (UI-003) | Shipped defaults apply; `transitions`/`options` behaviours respected at runtime; `rtl` ships passively. Revisit (palette via DEC-040) MUST still respect UI-003/004 — i.e. even then, overrides land as inline `style=""`, not a theme file. |
| `guides/migration`, `migration`, `docsref`, `guides/contribute`, `guides/quickstart` | MECHANISM/OFF | No runtime effect; `migration` noted for the Bootstrap version-pin record at build time. Section-index pages (`components`, `forms`, `content`, `layout`, `helpers`, `utilities`, `examples`, `customize`, `getting-started`, `guides` roots) carry no component semantics of their own — coverage is via their children above. |

## 5. Accessibility contract (Bootstrap-aided, WCAG 2.2 AA target, testable)

Pinned values (AC-215 asserts each one — these are the exact, greppable
assertions; F-43 closed by giving every named mechanism a number):

- **Contrast ratios (WCAG 2.2 AA):** normal text ≥ **4.5:1**; large text
  (`metric-lg` 36 px, `heading-md` 24 px+ at any weight, `heading-sm` 20 px
  bold) ≥ **3:1**; non-text UI (focus rings, borders, progress-bar track vs
  fill) ≥ **3:1**. Verified pairs (exact hex from UI-007; ratios computed with
  the WCAG relative-luminance formula, rounded to one decimal):

  | Text | Surface | Ratio | AA normal (4.5:1) | AA large (3:1) |
  |------|---------|-------|-------------------|----------------|
  | `#FFFFFF` | `#086288` | 6.7:1 | PASS | PASS |
  | `#172B36` | `#FFFFFF` | 14.6:1 | PASS | PASS |
  | `#172B36` | `#F6F9FB` | 13.8:1 | PASS | PASS |
  | `#5C6F7B` | `#FFFFFF` | 5.2:1 | PASS | PASS |
  | `#5C6F7B` | `#F6F9FB` | 4.9:1 | PASS | PASS |
  | `#B42318` | `#FFFFFF` | 6.6:1 | PASS | PASS |
  | `#237A57` | `#FFFFFF` | 5.3:1 | PASS | PASS |
  | `#966000` | `#FFFFFF` | 5.3:1 | PASS | PASS |
  | `#71828C` | `#FFFFFF` | 4.0:1 | **FAIL** | PASS |
  | `#6B7B85` | `#FFFFFF` | 4.4:1 | **FAIL** | PASS |

  Consequences (binding, asserted by AC-215): `#71828C` (4.0:1) and `#6B7B85`
  (4.4:1) are the ONLY two UI-007 text colours below 4.5:1, so they are
  permitted for **large text (≥ 3:1) and non-text UI only** — metadata
  timestamps, tertiary labels, and inactive badges. They MUST NEVER render
  normal-size text, an instruction, an error message, or a control label; those
  use `#172B36`/`#5C6F7B`. Every other pair in the table above passes AA at
  normal size. Any pair not in this table MUST be measured before use.
- **Focus ring:** exactly the vendored Bootstrap `helpers/focus-ring` value —
  `.focus-ring:focus-visible { outline: var(--bs-focus-ring-width) solid
  var(--bs-theme-focus-ring, var(--bs-focus-ring-color)); }` with OBSERVED
  tokens `--bs-focus-ring-width: 3px`, `--bs-focus-ring-offset: 1px`, and
  `--bs-focus-ring-color: var(--bs-primary-focus-ring)` = `color-mix(in oklch,
  var(--bs-blue-500) 50%, var(--bs-bg-body))` (i.e. `blue-500` mixed 50 % with
  the body background; the colour resolves at render time and is never an
  inline `style=""` value — the focus ring ships with the vendored bundle and is
  NOT overridden by UI-007). The width (`3px`) is the pinned, greppable value;
  form controls apply the same ring with an inverted offset (`-1px`, OBSERVED).
  A bare `outline:none` anywhere fails AC-217. The ring's computed colour is a
  ≥3:1 non-text contrast requirement against the adjacent surface (WCAG 2.2 AA
  non-text rule); it is not one of the ten text pairs in the table above.
- **Keyboard navigation:** every interactive element is reachable by
  `Tab` in DOM order; visible focus on all of the above; dialogs trap `Tab`
  inside and restore focus to the invoking control on close (`Enter`/`Esc`
  per `components/dialog` — the pause blocker deliberately has no Esc);
  menus open with `Enter`/`Space`/`↓` and close on `Esc`; no keyboard trap
  outside a modal; `Skip to content` link is NOT shipped (single-page
  sections, no header nav repeat — documented scope).
- **Screen-reader labels:** every `img` carries an `alt` (teacher-supplied
  or empty for decorative, per BR-010); every input has a programmatic
  `<label>` (or `aria-label` on the code boxes); `spinner`s carry
  `role="status"` + visually-hidden text; countdown is `role="timer"` with a
  throttled (5 s) `aria-live="polite"` region; `progress` bars carry
  `role="progressbar"` + `aria-valuenow/min/max`; avatar initials are
  `aria-hidden="true"` with the name text as the label; `badge` counts are
  announced by their accompanying text (no icon-only meaning).
- **Reduced motion:** `prefers-reduced-motion: reduce` — the vendored bundle
  already gates every transition behind `@media (prefers-reduced-motion:
  no-preference)` (OBSERVED), so the default reduced-motion experience is
  static. The quiz-v2 contract on top of it: no inline `transition`/`animation`
  may re-enable motion under reduce (AC-217 fails the build); `hover-lift` stays
  static; `.fade`/toast/collapse/dialog/`progress` values have no effect; the
  `progress`-bar and `hover-lift` animations in the §3A table are inert; and the
  countdown TEXT (`role=timer`) plus the progress width remain the sole time
  signal, so a reduced-motion user loses no information.

- `getting-started/accessibility` applies verbatim: labelled inputs
(`field`/`floating-labels`), `validation` errors with `aria-describedby`,
focus to the `alert` summary on 422 and to the new question `h1` on advance,
`dialog` trap + return-focus (pause blocker: no Esc by design, C-010).
- `helpers/focus-ring` only; countdown `role=timer` throttled (5 s) +
supplementary `progress` bar (never animation-only); `stepper` text always
present ("Soal 3 dari 10").
- Contrast 4.5:1/3:1 verified against the decided UI-007 palette (AC-215
asserts the exact pairs and ratios listed above; any failing pair blocks
FINAL). `utilities/motion` + `prefers-reduced-motion`
disables smoothing, `hover-lift`, toast slide, `progress` animation,
`carousel` motion.
- Touch: option rows `<label class="list-group-item">` + NATIVE inputs;
  `switch` native; `range` paired with number input (either works); `otp-input`
  degrades to the single plain input no-JS; `carousel` controls are native
  buttons with labels.

## 6. Localisation and formatting (unchanged)

WIB + "WIB" suffix + UTC `tooltip`; Indonesian number grouping in UI,
invariant decimals in payloads; "12 mnt 30 dtk"; wrapping via
`utilities/text-wrapping`; `datepicker` output localized id-ID while payloads
stay ISO UTC.

## 7. What is out of scope (Bootstrap-honest)

New stylesheets (beyond the single `@font-face` block), Sass builds, extra
theme files, icon fonts,
illustrations, PWA/offline shell (breaks server-time fairness), dark
`color-modes`, marketing examples, auth OTP (the `otp-input` reuse is
code-entry UX only). Anything here needs a documented, user-validated requirement
recorded as a new DEC entry — none exists today.

Open: none on theme — UI-006/UI-007 are DECIDED (DEC-040/041 resolved
2026-10-12); remaining open is only the wordmark file question (text
"quiz-v2" in Gotham Bold ships; a logo FILE is supplied by nobody, so none
ships). Bootstrap 6.0.x exact patch + vendored SRI
hashes are build-time records, not spec decisions.

UI-owned audit notes (S1..S6, one-liners; BR-055/API/EDGE/AC traces stay inline):
- S1 Retry-After, S2 page mapping, S3 restart window, S5 cancelled card, S6 CI: not UI-owned — no UI decision recorded here.
- S4-heartbeat (CLOSED, DECIDED S4a 2026-10-14): payload is `{server_now_utc}`, emitted every 30 s; the single stale threshold is 60 s — after 60 s without a heartbeat the stale banner shows, numbers stay frozen at the last-seen value and the spinner is cleared (C-006/C-010, API-036, STATE-007). The separate dead-stream silence timeout (45 s, DEC-102) is a different mechanism and is unchanged.
- Accepted limitation (not a decision): abort-toast wording stays generic; focus return and live-region polish stay best-effort per C-010.
