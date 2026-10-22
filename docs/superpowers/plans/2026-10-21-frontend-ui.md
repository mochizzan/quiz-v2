# Twisty Quiz Frontend (All Screens) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task, **with parallel dispatch**: tasks inside the same wave are executed concurrently by separate subagents (see "Parallel execution protocol"). Steps use checkbox (`- [ ]`) syntax for tracking.

> **Amendment 2026-10-22 — font and asset layout (user directive).** Gotham
> Rounded (`assets/gotham-rounded/`, converted to woff2 under
> `static/fonts/gotham-rounded/`) replaces Plus Jakarta Sans, and `static/` was
> reorganised: third-party files now live under `static/vendor/{bootstrap,jquery}/`.
> This supersedes the font and path instructions in the steps below, which are
> kept as the historical record. **Licensing caveat:** Task 1 Step 2 of this
> plan warned that Gotham Rounded is proprietary (Hoefler & Co., no open
> grant); that risk is now accepted by the project owner.

**Goal:** Build the complete server-rendered frontend — 25 screens + 2 error pages, shared chrome partials, the token stylesheet, and vendored Bootstrap 6 / jQuery / font assets — porting the three reference prototypes onto `docs/DESIGN.md`, with hardcoded demo data and zero backend logic.

**Architecture:** The custom view loader (`internal/httpserver/views.go`) is extended to build one `html/template` set per page (all `layouts/` + `partials/` + the page file, which defines `content`); pages choose their layout by path convention. Pages self-register their routes through a package-level registry (`internal/httpserver/routes.go` + one `routes_<group>.go` file per task), so parallel tasks never touch the same Go file. All UI data is hardcoded in the templates. One token stylesheet (`static/css/tokens.css`) maps the DESIGN palette onto Bootstrap 6's `--bs-*` theme custom properties; Bootstrap 6.0.0-alpha.1 (ESM bundle), jQuery 4.0.0 and the Plus Jakarta Sans variable font are vendored into `static/` and served by Fiber.

**Tech Stack:** Go 1.26.4, Fiber v3.6.1, `html/template`, Bootstrap 6.0.0-alpha.1 (vendored, ESM-only JS), jQuery 4.0.0 (vendored, classic), Plus Jakarta Sans variable woff2 (vendored), stdlib `testing` only.

**Specs (read before starting):**
- `docs/DESIGN.md` — sole authority for colour, type, spacing, radius, elevation, motion, states, a11y.
- `docs/superpowers/specs/2026-10-20-quiz-system-design.md` §15 (carrier, v5→v6 class map, page inventory, state contract, styling mechanism) and §15.10 (Indonesian copy).
- `docs/superpowers/plans/2026-10-21-initial-framework.md` — the landed Go skeleton this plan extends.
- Reference prototypes (READ-ONLY): `docs/template/LOGIN.html`, `docs/template/DASHBOARD.html`, `docs/template/QUIZ.html`.

## Global Constraints

Every task's requirements implicitly include this section.

**Precedence and references**
- Order of authority: `docs/DESIGN.md` > spec §15 > the three prototypes. The prototypes are normative ONLY for information architecture, screen order, and copy intent. Their markup, CDN links, icon fonts, English copy, and ad-hoc CSS are NOT normative — port, never copy.
- `docs/template/*.html` are read-only: never modify or duplicate them. They load Bootstrap 5.3.3 + FontAwesome/Bootstrap Icons from CDNs — every icon ported from them becomes an inline SVG sprite symbol (Task 5 catalog), and every v5 class becomes its v6 name.
- Bootstrap v6 source of truth is the vendored tree `assets/bootstrap-6.0.0-alpha/` (never edited). Verify any v6 class name against `assets/bootstrap-6.0.0-alpha/css/bootstrap.min.css` before use. Verified facts: `.offcanvas` does NOT exist (v6 uses `<dialog class="drawer">`); responsive prefix syntax `.md:col-6` / `.xl:row-cols-4` / `.md:table-responsive` / `.md:d-none` all exist; spacer classes `.p-7`/`.p-12`/`.g-7`/`.g-9` exist; `.container` breakpoints give max-widths 540/720/960/1200/1440px (there are NO named `.container-lg`-style variants); theme classes are `.theme-primary|secondary|accent|success|warning|danger|info|inverse|muted|subtle|border|contrast|reset`; colour utilities are `.fg-*` / `.bg-*` (no `.text-primary`, no `.text-muted`); `.placeholder-wave` and `.spinner-border` exist.

**Carrier (spec §15.1)**
- Bootstrap 6.0.0-alpha.1 vendored at `/static/bootstrap/` (files `bootstrap.min.css`, `bootstrap.bundle.min.js`), never edited, no Sass, no build step.
- Bootstrap JS is ESM-only: `<script type="module" src="/static/bootstrap/bootstrap.bundle.min.js">`. No `window.bootstrap`, no UMD, no jQuery plugin bridge (`$(...).dialog(...)` etc. forbidden). jQuery 4.0.0 is a classic deferred script at `/static/jquery/jquery-4.0.0.min.js` for DOM work only; per conformance AC-217 the Bootstrap module script is emitted BEFORE the jQuery script tag.
- Component data API names: `data-bs-toggle="dialog"|"drawer"|"menu"`, `data-bs-dismiss="dialog"|"drawer"`. v5 names `modal`/`offcanvas`/`dropdown` are defects.
- No CDN of any kind: every stylesheet, script, and font comes from `/static/`. The LAN must work with the internet down.
- Exactly one token stylesheet (`static/css/tokens.css`, Task 3) and at most one `<style>` element per page containing ONLY `@font-face` for the vendored font. No other CSS file, no new class definitions, no `!important`, no Sass.
- `<html lang="id" data-bs-theme="light">` on every page (v6 follows the OS colour scheme by default; light is forced. No dark mode is ever shipped).
- **Verified Fiber v3.6.1 facts (checked against the module source — do not use v2 idioms):** there is NO `app.Static` method and no `fiber.Static` type; static files are served by the in-module middleware `github.com/gofiber/fiber/v3/middleware/static` (same module — no new go.mod dependency): `app.Use("/static", static.New("./static", static.Config{MaxAge: 86400}))`. Redirects use the builder: `c.Redirect().Status(fiber.StatusSeeOther).To(path)` — `c.Redirect(path, code)` does not exist.
- **Verified v6 helper classes (all present in the vendored dist):** `.drawer-end`, `.drawer-bottom`, `.drawer-start`, `.drawer-top`, `.xl:d-none`, `.rounded-pill`, `.rounded-circle`, `.fixed-bottom`, `.d-none`, `.breadcrumb`/`.breadcrumb-link`/`.breadcrumb-divider`, `.check`/`.radio`/`.switch`, `.invalid-feedback`/`.invalid-tooltip`, `.placeholder`/`.placeholder-wave`, `.spinner-border`, `.toast`/`.toast-container`, `.progress`/`.progress-bar`, `.menu`/`.menu-item`, `.dialog`/`.drawer` (with `.dialog-header/-body/-footer` and `.drawer-header/-body/-footer`).
- **Verified paint facts (override only the tokens a paint rule actually consumes — each checked against the dist):** `.btn` paints `border-radius: var(--bs-btn-border-radius)`, so `.btn{--bs-btn-border-radius:9999px}` works; `.btn-solid:hover` paints `background-color: var(--bs-btn-hover-bg, var(--bs-bg-3))` and `:active` paints `--bs-btn-active-bg` — setting `--bs-btn-bg` changes the RESTING state only and has no effect on hover/active; `.card{--bs-card-bg:var(--bs-bg-body)}` (so `--bs-bg-body` must stay `#FFFFFF` or every card, toast and control renders canvas-coloured; the canvas arrives as the layouts' inline frame style); `.toast` paints `--bs-toast-bg: var(--bs-bg-body)` (no `--theme-*` consumption) and is `display:none` until it carries `.show`; `.menu-item:hover` paints `var(--bs-theme-bg-subtle, var(--bs-menu-item-hover-bg))` and `.menu-item.active` paints `var(--bs-theme-bg, var(--bs-menu-item-active-bg))` — set BOTH halves of each pair; `.progress-bar` paints `var(--bs-theme-bg, var(--bs-progress-bar-bg))`, so `theme-primary` on the bar is the sanctioned fill.

**v5 → v6 class map (spec §15.1.1 — a v5 name in authored markup is a defect)**
`.col-md-6`→`.md:col-6`; `.d-md-none`→`.md:d-none`; `.d-lg-none`→`.lg:d-none`; `.g-md-3`→`.md:g-3`; `.table-responsive-md`→`.md:table-responsive`; `.modal`+`.modal-dialog`+`.modal-content`→`<dialog class="dialog">`; `.offcanvas`→`<dialog class="drawer">`; `.dropdown-menu`/`.dropdown-item`→`.menu`/`.menu-item`; `.btn-primary`→`.btn .btn-solid .theme-primary`; `.btn-outline-primary`→`.btn .btn-outline .theme-primary`; `.alert-danger`→`.alert .theme-danger`; `.badge.bg-primary`→`.badge .theme-primary`; `.text-primary`→`.fg-primary`; `.text-muted`→`.fg-secondary`; `.form-select`→`.form-control` on the `<select>`; `.form-check-input`→`.check`/`.radio`/`.switch`; validation: `data-bs-validate` on `<form>`, `:user-invalid`, `.invalid-feedback`, `<div class="tooltip invalid-tooltip">`; `.accordion-button`→v6 accordion items (`.accordion-item/.accordion-header/.accordion-body/.accordion-icon`); `.fs-*` never used; `.placeholder-glow`→`.placeholder-wave`.
Spacer map (DESIGN px → v6 class key): 8→3, 12→4, 16→5, 24→7, 32→9. v5 `.g-3` (16px) ≠ v6 `.g-3` (8px) — recompute, never copy.

**Colour (DESIGN §2.1 — the ONLY hexes permitted in authored markup, tokens.css, or inline styles)**
`#293681` `#1E2864` `#FFFFFF` `#4274D9` `#E24A21` `#FDF0ED` `#5589C3` `#EEF4FA` `#95CCDD` `#EAF6FA` `#D0E7E6` `#EDF0F4` `#F4F6F8` `#E2E7EC` `#838EA0` `#121721` `#687182` `#94A3B8` `#E2E6EB` `#ECEFF2` `#237A57` `#966000` `#B42318` `#6B7B85`.
`rgba()` values may only use RGB components drawn from these hexes (e.g. `rgba(18, 23, 33, .04)` = `#121721`). `#000000` forbidden. State is never colour-only — pair every colour with a word.

**Typography / shape / motion (DESIGN §3, §6, §11)**
Plus Jakarta Sans everywhere. Type scale only: 32/38·700 (display-metric), 24/30·600 (h1), 20/26·600 (h2), 24/28·700 (metric), 16/22·600 (body-prominent), 14/20·400 (body), 13/18·500 (label), 11/14·600 (badge). Question text ≥16px. Every authored heading MUST carry its class — `<h1 class="h1">`, `<h2 class="h2">`, `<h3 class="h3">` — never a bare `<h1>`/`<h2>`/`<h3>` (verified: the dist has ONE heading rule `h6,.h6,…,h1,.h1{…line-height:var(--bs-heading-line-height)…}` that resolves the token on the element, so the `.h1`/`.h2` class-scoped overrides in tokens.css only reach elements carrying the class). Scores/timers/counts use tabular numerals via inline `font-variant-numeric: tabular-nums` — **the single sanctioned addition to the spec §15.11 inline closed list** (verified: the v6 dist ships no tabular-nums utility or token, so this is the only mechanism; flagged for spec ratification; use it ONLY on score/timer/count elements). Radius: container 32 / card 24 / row 18 / controls+badges pill (arrives via the tokens.css component overrides; DESIGN radii not on the v6 scale — 8/18/24/32px — may use inline `border-radius`; never rely on numeric `.rounded-*` utilities: the dist scale is 2/3/4/6/8/10/12/14/16px, which does not match DESIGN; prefer `.rounded-pill`/`.rounded-circle` where they fit). Spacing base 4px. Motion values only: 0.15s controls, 0.35s collapse, 0.3s dialog/drawer/popover, 0.6s progress fill, 0.15s toast, 1.5s spinner — all gated behind `prefers-reduced-motion: no-preference` (tokens.css handles the gate; authored markup introduces NO transition/animation).

**Inline style closed list (spec §15.11)** — inline `style=""` may only set: `padding-bottom` (safe-area), `width` (progress/countdown), `position`+`z-index` (frozen column only), `cursor`, `overflow`, `print-color-adjust`, `aspect-ratio`/`object-fit`, `white-space`, `color`/`background-color`/`border-color` (palette hex only), `font-family`, `font-size`/`line-height` (type-scale values only), `border-radius`, `box-shadow` (menus/dialogs only).

**Layout (spec §15.4)** — flexbox grid only (`.container`/`.row`/`.col-*`/`.g-*`); CSS-grid utilities (`.grid`, `.grid-cols-*`) banned. App frame = `.container` (1440px at 2xl ✓ inside the 1280–1440 band). Teacher workspace = asymmetric two-column (narrow left rail + wider content). Student attempt column: `<div class="container"><div class="row"><div class="col-12 xl:col-8 mx-auto">` (v6 responsive-prefix spelling — `col-xl-8` is a v5 name and a defect; yields ≤960px at every width: lg 960, xl ~768, 2xl ~928). Auth = split screen. Breakpoints: `md` 768 and `xl` 1280 only (there is no 1200px breakpoint). Below 768: single column, tiles stack one per row, teacher nav becomes a `<dialog class="drawer">`, the attempt action bar becomes a sticky bottom bar with `padding-bottom: env(safe-area-inset-bottom)`, tables scroll in `.table-responsive`.

**Accessibility (DESIGN §10, spec §15.5)** — WCAG 2.2 AA. Bootstrap focus ring unmodified; `outline: none` forbidden. Every input has a programmatic `<label>`; every `img` has `alt`; icon-only controls carry `aria-label`; spinners carry `role="status"` + visually-hidden text; progress bars carry `role="progressbar"` + `aria-valuenow/min/max`; the answered-count region is `aria-live="polite"`; dialogs/drawers are native `<dialog>` (platform focus trap). Copy is sentence case.

**Language (spec §15.10)** — all UI copy Indonesian; `<html lang="id">`; dates/times in WIB with "WIB" suffix. Spec/plan artifacts stay English. **Documented deviation for this frontend-only milestone:** the `internal/i18n` catalogue and `partials/strings.html` are backend work — copy is hard-coded in each page's markup now and will be extracted later; do NOT create `templates/partials/strings.html` in this plan.

**Icons (spec §15.9)** — inline SVG sprite only (`templates/partials/icons.html`, `currentColor`, stroke-based). No icon font, no emoji-as-icon (emoji are decorative-text-banned in authored templates; the conformance test flags emoji codepoints).

**Testing policy (user decision)** — **unit tests only** (`go test ./...`); no smoke tests, no headless browser, no e2e. Compile/static checks (`go build`, `go vet`, `gofmt -l .` empty, `docker build`) accompany the suite. UI conformance is verified by static analysis of templates/tokens.css plus unit tests over rendered HTML (spec §16). Every test in `internal/httpserver` inherits the existing `TestMain` chdir-to-repo-root, so `./templates` and `./static` resolve under `app.Test`.

**Skill guidance for implementers** — page/partial tasks should load `frontend-ui-engineering` (states, a11y, spacing discipline) and the `bootstrap-component` family for class reference. **Override:** `design-taste-frontend` house rules (no gradients, no resting shadows, no pill badges, no icon tiles) do NOT apply here — `docs/DESIGN.md` mandates soft shadows, pill geometry, and tinted icon badges; DESIGN.md wins.

**Canonical demo data (identical across all parallel tasks — do not invent variants)**
- Guru: **Ratna Wijaya** (inisial RW), mapel "Biologi & Genetika", sekolah "SMA Nusantara". Term: "Semester Ganjil 2026/2027". Hari ini: "Rabu, 21 Oktober 2026".
- Siswa: Ayu Lestari (XII IPA 2, AL), Bayu Prakoso (XII IPA 2, BP), Cindy Halim (XI IPA 1, CH), Dimas Ardiansyah (X IPS 3, DA), Eka Nuraini (XI IPA 1, EN). Kelas: XI IPA 1, XI IPA 2, XII IPA 2, X IPS 3.
- Kuis: "Biologi Sel — Kuis 4" (aktif, 25 soal, 45 menit, kode **QUIZ-7F2K**); "Kimia Organik — Kuis 2" (draf); "Fisika Kinematika — Kuis 3" (selesai); "Sejarah Indonesia — Kuis 1" (menunggu penilaian).
- Attempt: "Kuis Pengetahuan Umum — Sesi 1", 5 soal (nomor 1, 3, 4 pilihan ganda; 2 dan 5 esai) — reuse the exact Indonesian questions from `docs/template/QUIZ.html` lines 605–636.
- Statistik: 3 kuis aktif · 142 peserta · rata-rata kelulusan 86,2% (delta +5,4%) · 18 esai menunggu penilaian · 1.480 soal bank · 8 topik.
- Format angka Indonesia: "1.480", "86,2%", jam "09.15 WIB".
- Copy anchors: "Masuk", "Daftar akun", "Gabung dengan kode kuis", "Buat kuis baru", "Selamat datang kembali", "Menunggu persetujuan admin", "Ubah kata sandi", "Tinjau jawaban", "Kumpulkan jawaban", "Jawaban berhasil dikumpulkan", "Riwayat kuis", "Penilaian esai", "Kelola siswa", "Coba lagi".

## Parallel execution protocol

This plan is wave-structured. Tasks in the same wave are independent by construction and are executed **concurrently by separate subagents** (subagent-driven-development with parallel dispatch). Tasks in different waves run in order because later waves consume earlier waves' interfaces.

| Wave | Tasks | Mode |
| :--- | :--- | :--- |
| W0 | 1 | serial (blocks everything) |
| W1 | 2, 3, 4, 5 | **parallel ×4** |
| W2 | 6 | serial (all page tasks consume it) |
| W3 | 7, 8, 9, 10, 11, 12, 13, 14 | **parallel ×8** |
| W4 | 15 | serial (integration + conformance) |

Rules:
1. **File ownership is exclusive.** A task creates/modifies ONLY the files listed in its Files block. Never edit another task's file, `server.go` outside your task, or any file listed under another task. The registry (Task 4) exists precisely so page tasks never share a Go file.
2. **Parallel tasks never run git commands.** (Concurrent commits contend on the index lock.) A parallel task ends at: files written, its own tests green, `gofmt -w <its files>`, `go vet ./...` clean. After each parallel task's review passes, the **orchestrator** runs that task's pinned `git add … && git commit -m "…"` line itself, serially, in task order. Serial tasks (1, 6, 15) commit themselves as their final step.
3. **Each subagent receives**: this plan's Global Constraints + Canonical demo data, its own task block verbatim, the read-only reference template paths, and the instruction to load `frontend-ui-engineering` (and `bootstrap-component` for class reference) before writing markup.
4. **Concurrency-safe commands**: `go test ./...`, `go vet`, `go build`, greps. Do not run `docker build` inside parallel tasks (W4 owns it).
5. **If a task discovers a defect in an earlier wave's file**, it does NOT fix it inline: it reports the defect to the orchestrator, which dispatches a serial fix between waves.

## Review Focus

Conditions the spec implies but no single happy-path test exercises; each line is pinned by the named check.

1. **v5 class leakage from the prototypes** (a mechanical port leaves `.d-none d-lg-block`, `.btn-primary`, `.text-muted`, `data-bs-toggle="modal"` in markup) — pinned by `TestNoBootstrapV5Names` (Task 15) scanning every template, plus each page task's test asserting at least one v6-only class (`btn-solid`, `md:col-`, `theme-primary`) in its rendered page.
2. **Off-palette hex** (prototypes use `#ECFDF5`, `#FFF4EC`, `#D1FAE5`, `#059669`… which are not in DESIGN §2.1) — pinned by `TestPaletteHexOnly` (Task 15, allowlist of the 24 hexes) over `templates/**` and `static/css/*.css`.
3. **Broken templates after parallel merges** (missing `{{define}}`, a partial referencing a deleted symbol, a page defining the wrong layout's expectations) — pinned by `TestEveryRegisteredPageRenders` (Task 15: every registered route → 200 + `lang="id"` + non-empty `<title>`), which fails loudly on any parse/execute error.
4. **ESM/data-API wiring** (Bootstrap loaded as classic script → `Unexpected token 'export'`, components never initialise; or a v5 `data-bs-toggle` name) — pinned by `TestScriptWiring` (Task 15: foot emits `type="module"` bundle before jQuery; no `window.bootstrap`; no `data-bs-toggle="modal"|"offcanvas"|"dropdown"` anywhere).
5. **Icon sprite integrity + a11y minimums** (a `<use href="#i-…">` with no matching symbol; an icon-only button without `aria-label`; a progress bar without ARIA) — pinned by `TestIconRefsResolve` (Task 15) and per-page assertions in Tasks 14 (`role="progressbar"` + `aria-live`) and 9 (monitor ARIA).
6. **Font vendoring is not unit-testable** (the woff2 must be downloaded once at vendoring; the unit suite cannot detect a wrong-but-present file) — **not covered by unit tests**; the only guard is Task 1's binary-signature check plus a manual browser pass at W4.

## File Structure

```
quiz-v2/
├── Dockerfile                             # M: add COPY static/ (Task 1)
├── static/                                # served at /static (Task 1 creates tree)
│   ├── bootstrap/
│   │   ├── bootstrap.min.css              # copy of assets/bootstrap-6.0.0-alpha/css/bootstrap.min.css
│   │   └── bootstrap.bundle.min.js        # copy of assets/.../js/bootstrap.bundle.min.js (ESM)
│   ├── jquery/jquery-4.0.0.min.js         # copy of assets/jquery-4.0.0/jquery-4.0.0.min.js
│   ├── fonts/plus-jakarta-sans/
│   │   ├── PlusJakartaSans-latin.woff2    # downloaded once (fontsource variable build), OFL beside it
│   │   └── OFL.txt
│   ├── css/tokens.css                     # Task 3 — the ONLY token stylesheet
│   └── js/app.js                          # stub Task 1, attempt-page behaviour Task 14
├── internal/httpserver/
│   ├── views.go                           # M (Task 2) — per-page template sets, layout by path convention
│   ├── views_test.go                      # M (Task 2)
│   ├── server.go                          # M (Tasks 1, 4) — /static, registry wiring, 404 catch-all
│   ├── routes.go                          # Task 4 — registry: registerPage/PageRoute/PageData/RegisteredPages
│   ├── routes_home.go                     # Task 4 — GET / → /login, GET /hello
│   ├── routes_test.go                     # Task 4
│   ├── static_test.go                     # Task 1
│   ├── tokens_test.go                     # Task 3 — selector allowlist + palette check on tokens.css
│   ├── conformance_test.go                # Task 15 — whole-frontend static analysis + render-all
│   ├── routes_auth_public.go(_test)       # Task 7
│   ├── routes_auth_account.go(_test)      # Task 8
│   ├── routes_teacher_core.go(_test)      # Task 9
│   ├── routes_teacher_authoring.go(_test) # Task 10
│   ├── routes_teacher_grading.go(_test)   # Task 11
│   ├── routes_teacher_mgmt.go(_test)      # Task 12
│   ├── routes_student.go(_test)           # Task 13
│   └── routes_attempt.go(_test)           # Task 14
└── templates/
    ├── hello.html                         # existing standalone page (kept, still renders)
    ├── layouts/
    │   ├── base.html                      # Task 6 — bare document: head + content + foot
    │   ├── auth.html                      # Task 6 — split-screen shell (brand panel + auth column)
    │   ├── teacher.html                   # Task 6 — app frame + navbar-teacher + drawer-teacher
    │   ├── student.html                   # Task 6 — app frame + navbar-student
    │   └── error.html                     # Task 6 — centred card shell
    ├── partials/                          # Task 5 (except where noted)
    │   ├── icons.html                     # inline SVG sprite (symbol catalog pinned in Task 5)
    │   ├── head.html                      # <head>: meta, title slot, bootstrap.min.css, tokens.css, @font-face <style>
    │   ├── foot.html                      # scripts: bootstrap ESM bundle, jquery (defer), app.js module
    │   ├── navbar-teacher.html            # brand + 6 nav links (Active key) + search + user pill + hamburger
    │   ├── navbar-student.html            # brand + 3 nav links (Active key) + user pill
    │   ├── drawer-teacher.html            # <dialog class="drawer"> mobile nav
    │   ├── breadcrumb.html                # .breadcrumb / .breadcrumb-link
    │   ├── empty-state.html               # card + calm message + one primary action
    │   ├── banners.html                   # error / stale / locked / offline alert variants
    │   ├── toasts.html                    # fixed toast region
    │   ├── no-js-banner.html              # <noscript> alert theme-warning (included by foot.html)
    │   ├── progress-bar.html              # .progress with role="progressbar" + ARIA
    │   ├── stepper.html                   # authoring step indicator
    │   ├── dialogs.html                   # shared confirm dialog (<dialog class="dialog">)
    │   └── skeleton.html                  # placeholder-wave rows, aria-busy="true"
    └── pages/
        ├── auth/{login,register,pending,change-password,join,identify}.html   # Tasks 7, 8
        ├── profile.html                                                        # Task 13
        ├── student/{dashboard,history}.html                                    # Task 13
        ├── teacher/{dashboard,quizzes,quiz-edit,question-editor,image-upload,
        │            quiz-settings,monitor,grading,recap,students,anonymous,
        │            majors,teachers}.html                                      # Tasks 9–12
        ├── attempt/{question,review,result}.html                               # Task 14
        └── error/{404,500}.html                                                 # Task 6
```

**Template contracts (fixed by Tasks 2, 5, 6 — every later task obeys them):**
- A **page** file body is exactly `{{define "content"}} …page markup… {{end}}` (plus, rarely, extra defines named `pages/<path>#<id>`). A file WITHOUT a `content` define (e.g. `hello.html`) is executed standalone as a full document.
- A **layout** file is `{{define "layouts/<name>.html"}} … {{template "content" .}} … {{end}}`; it opens with `{{template "partials/head.html" .}}` and closes with `{{template "partials/foot.html" .}}`.
- A **partial** file is `{{define "partials/<name>.html"}} … {{end}}` (multiple defines allowed; never define `content`).
- Layout selection is by page path: `pages/auth/*`→`auth.html`, `pages/teacher/*`→`teacher.html`, `pages/student/*` and `pages/profile.html`→`student.html`, `pages/attempt/*`→`base.html`, `pages/error/*`→`error.html`.
- Template data is `httpserver.PageData{Active string}`; nav partials highlight via `{{if eq .Active "<key>"}}`.
- Icons: `<svg width="20" height="20" aria-hidden="true"><use href="#i-name"></use></svg>`; icon-only controls add `aria-label`.

---

### Task 1: Vendor static assets, serve them, ship them in the image

**Wave:** W0 (serial). **Commits itself.**

**Files:**
- Create: `static/bootstrap/bootstrap.min.css`, `static/bootstrap/bootstrap.bundle.min.js`, `static/jquery/jquery-4.0.0.min.js`, `static/fonts/plus-jakarta-sans/PlusJakartaSans-latin.woff2`, `static/fonts/plus-jakarta-sans/OFL.txt`, `static/js/app.js` (module stub)
- Modify: `internal/httpserver/server.go` (add the static route), `Dockerfile` (runtime stage copies `static/`)
- Test: `internal/httpserver/static_test.go`

**Interfaces:**
- Consumes: nothing (foundation).
- Produces: `GET /static/*` served by Fiber v3's in-module static middleware with `static.Config{MaxAge: 86400}` → `Cache-Control: public, max-age=86400` on 2xx file responses (go-web-ssr caching convention); a missing file falls through (`c.Next()`) to the 404 catch-all Task 4 adds. Later tasks link `/static/bootstrap/bootstrap.min.css`, `/static/css/tokens.css`, `/static/bootstrap/bootstrap.bundle.min.js`, `/static/jquery/jquery-4.0.0.min.js`, `/static/js/app.js`, `/static/fonts/plus-jakarta-sans/PlusJakartaSans-latin.woff2`.

- [ ] **Step 1: Copy the vendored libraries into `static/`**

```bash
mkdir -p static/bootstrap static/jquery static/fonts/plus-jakarta-sans static/css static/js
cp assets/bootstrap-6.0.0-alpha/css/bootstrap.min.css static/bootstrap/
cp assets/bootstrap-6.0.0-alpha/js/bootstrap.bundle.min.js static/bootstrap/
cp assets/jquery-4.0.0/jquery-4.0.0.min.js static/jquery/
```

- [ ] **Step 2: Download the Plus Jakarta Sans variable font (the assets tree is missing it)**

The spec names `assets/fonts/plus-jakarta-sans/PlusJakartaSans-latin.woff2` as source of truth but the file does not exist in the repo (verified: `assets/` contains only `bootstrap-6.0.0-alpha/`, `gotham-rounded/`, `jquery-4.0.0/`). Fetch the fontsource variable build (single woff2, latin subset, full weight axis) plus the OFL licence, and keep a copy under `assets/fonts/plus-jakarta-sans/` as the spec's source of truth:

```bash
curl -fsSL -o assets/fonts/plus-jakarta-sans/PlusJakartaSans-latin.woff2 \
  "https://cdn.jsdelivr.net/npm/@fontsource-variable/plus-jakarta-sans/files/plus-jakarta-sans-latin-wght-normal.woff2"
curl -fsSL -o assets/fonts/plus-jakarta-sans/OFL.txt \
  "https://cdn.jsdelivr.net/npm/@fontsource-variable/plus-jakarta-sans/LICENSE" \
  || curl -fsSL -o assets/fonts/plus-jakarta-sans/OFL.txt \
  "https://raw.githubusercontent.com/google/fonts/main/ofl/plusjakartasans/OFL.txt"
cp assets/fonts/plus-jakarta-sans/PlusJakartaSans-latin.woff2 assets/fonts/plus-jakarta-sans/OFL.txt static/fonts/plus-jakarta-sans/
```

Verify: `head -c 4 static/fonts/plus-jakarta-sans/PlusJakartaSans-latin.woff2` prints `wOF2` and `OFL.txt` is non-empty and starts with `Copyright`. (Landed reality, 2026-10-22: the fontsource latin-wght build is **27,348 bytes** — smaller than an earlier ">30 KB" heuristic; judge by the wOF2 signature and the CDN Content-Length match, not by a fixed size floor.) **If the download fails or the signature is wrong: stop and report to the user — do not substitute the proprietary `assets/gotham-rounded/` fonts and do not add a CDN link.**

- [ ] **Step 3: Write the `static/js/app.js` stub**

```js
// Twisty Quiz application behaviour (ES module). Bootstrap is imported by the
// bundle script tag; this file carries only app-specific wiring.
export {};
```

- [ ] **Step 4: Add the static route in `internal/httpserver/server.go`**

Fiber v3.6.1 has no `app.Static` (verified against the module source); use the in-module middleware (no new go.mod dependency). In `NewWithStore`, immediately after `views := NewViewLoader(defaultTemplatesDir)`:

```go
import "github.com/gofiber/fiber/v3/middleware/static"

app.Use("/static", static.New("./static", static.Config{MaxAge: 86400}))
```

(`app.Use` with the `/static` prefix only intercepts `/static/*` requests; a missing file calls `c.Next()` and reaches Task 4's 404 catch-all.)

- [ ] **Step 5: Write the failing test `internal/httpserver/static_test.go`**

Three cases in package `httpserver_test`: `TestStaticBootstrapCSSServed` (`GET /static/bootstrap/bootstrap.min.css` → 200, `Content-Type` contains `text/css`, `Cache-Control` contains `max-age=86400`, body contains `.btn-solid`), `TestStaticJQueryServed` (`GET /static/jquery/jquery-4.0.0.min.js` → 200, body contains `jQuery`), `TestStaticFontServed` (`GET /static/fonts/plus-jakarta-sans/PlusJakartaSans-latin.woff2` → 200). Use the same `app.Test` pattern as `server_test.go`.

- [ ] **Step 6: Run to verify FAIL, then implement Step 4, run to verify PASS**

Run: `go test ./internal/httpserver/ -run TestStatic -v` → FAIL (404). After Step 4 → PASS (3 tests).

- [ ] **Step 7: Ship `static/` in the container**

In `Dockerfile` runtime stage, beside the existing `COPY --chown=app:app templates /app/templates` (same style, same ownership — the server runs as the non-root `app` user):

```dockerfile
COPY --chown=app:app static /app/static
```

(`.dockerignore` already excludes only `.git`, `docs`, `assets`, `*.md`, `.env`, `bin/` — `static/` is not excluded. `templates/` must NOT be excluded.) Run `docker build -t quiz-v2:dev .` only if a Docker daemon is available; otherwise note it for Task 15.

- [ ] **Step 8: `gofmt -w internal/httpserver/ && go vet ./... && go test ./internal/httpserver/ -count=1` → green**

- [ ] **Step 9: Commit**

```bash
git add static/ internal/httpserver/server.go internal/httpserver/static_test.go Dockerfile assets/fonts/
git commit -m "feat: vendor bootstrap 6, jquery 4 and jakarta font into static/"
```

---

### Task 2: Per-page view loader (layouts + partials + page sets)

**Wave:** W1 (parallel with 3, 4, 5). **Does not commit — orchestrator commits after review.**

**Files:**
- Modify: `internal/httpserver/views.go`
- Modify: `internal/httpserver/views_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `NewViewLoader(dir string) *ViewLoader` (same signature) with new semantics; `Render(w, name, data) error` (same signature) where `name` is the slash-relative path from `dir` (e.g. `pages/teacher/dashboard.html`, `hello.html`). Every file under `dir/layouts/` and `dir/partials/` is parsed into EVERY page set, named `layouts/<rel>` / `partials/<rel>`. Every other `.html` under `dir` is a page: its file body defines `content`; the loader executes the layout chosen by the path convention (see template contracts above); files without a `content` define execute standalone. Tasks 6–15 rely on exactly this behaviour. Existing tests (`TestRenderMissingTemplateReturnsError`, `TestHelloRoute`) must stay green unchanged.

- [ ] **Step 1: Write the failing fixture tests in `internal/httpserver/views_test.go`**

Add `TestPageRendersThroughLayoutAndPartials`: build a fixture tree in `t.TempDir()` — `layouts/base.html` = `{{define "layouts/base.html"}}HEAD {{template "content" .}} FOOT{{end}}`, `partials/p.html` = `{{define "partials/p.html"}}P-TOKEN{{end}}`, `pages/x/y.html` = `{{define "content"}}BODY {{template "partials/p.html" .}}{{end}}` — then `Render(&buf, "pages/x/y.html", nil)` and assert the output contains `HEAD`, `BODY`, `P-TOKEN`, `FOOT` in that order. Add `TestStandaloneFileRendersDirectly`: a temp `solo.html` with plain full-document HTML renders as-is. Add `TestMissingLayoutReturnsError`: a page whose convention-mapped layout file is absent returns an error. Add `TestPageCanOverrideTitleDefine` (pins the per-page `<title>` mechanism every page task relies on): fixture `partials/t.html` = `{{define "partials/title"}}Judul Default{{end}}`, `layouts/base.html` emitting `<title>{{template "partials/title" .}}</title>` plus `{{template "content" .}}`, page `pages/a.html` = `{{define "partials/title"}}Judul Halaman{{end}}{{define "content"}}BODY{{end}}` — rendered title must be `Judul Halaman` (the page is parsed LAST, so its define replaces the partial's in that page's set), while a second page `pages/b.html` without the override gets `Judul Default`.

- [ ] **Step 2: Run to verify FAIL**

Run: `go test ./internal/httpserver/ -run 'TestPageRenders|TestStandalone|TestMissingLayout' -v` → FAIL (loader still globs flat).

- [ ] **Step 3: Implement the recursive loader in `views.go`**

Keep the `ViewLoader` struct and the "construction never panics, parseErr is returned by Render" contract. Construction: `filepath.WalkDir(dir)`; classify each `*.html` by its slash-relative path; parse layouts+partials into a shared per-page `template.New(relPath)` set (each file parsed with an explicit name via `t.New(name).Parse(string(content))` so nested dirs don't collide on base filenames); for each page, build its set (partials + layouts + the page parsed LAST) and record a `map[string]renderable` where `renderable` is either the page's `content`-wrapping layout name or the page itself for standalone files. Layout convention table (exact): `pages/auth/`→`layouts/auth.html`; `pages/teacher/`→`layouts/teacher.html`; `pages/student/`→`layouts/student.html`; `pages/profile.html`→`layouts/student.html`; `pages/attempt/`→`layouts/base.html`; `pages/error/`→`layouts/error.html`; anything else→standalone. `Render` looks the name up and executes; unknown name → error. Include a short doc comment stating the page/layout/partial contracts.

- [ ] **Step 4: Run ALL httpserver tests to verify PASS**

Run: `go test ./internal/httpserver/ -count=1 -v` → all green, including the two pre-existing tests (empty dir still errors; `hello.html` still renders because it is a root-level standalone page).

- [ ] **Step 5: `gofmt -w internal/httpserver/views.go internal/httpserver/views_test.go && go vet ./internal/httpserver/`**

- [ ] **Step 6: Hand off (no commit)** — report to the orchestrator: "Task 2 done; commit message: `feat: extend view loader to per-page layout+partial template sets`".

---

### Task 3: `static/css/tokens.css` — the single token stylesheet

**Wave:** W1 (parallel with 2, 4, 5). **Does not commit — orchestrator commits after review.**

**Files:**
- Create: `static/css/tokens.css`
- Test: `internal/httpserver/tokens_test.go`

**Interfaces:**
- Consumes: nothing (the palette/type/radius/elevation/motion values are pinned in Global Constraints and DESIGN.md).
- Produces: the ONLY stylesheet besides Bootstrap. Every later page gets the Twisty look through it. It contains exactly: (a) `:root, :host` custom-property declarations; (b) the closed state-rule list; (c) the closed component-override list — nothing else (spec §15.11).

- [ ] **Step 1: Write the failing selector-allowlist test `internal/httpserver/tokens_test.go`**

`TestTokensCssOnlyAllowedSelectors`: read `static/css/tokens.css`, extract every selector (text before each `{`, joined with its at-rule context), and assert each is one of: `:root`, `:host`, an entry of the closed lists — state rules `.btn-solid.theme-primary:hover`, `.btn-solid.theme-primary:active`, `.btn-solid.theme-primary:focus-visible`, `.btn-close:hover`, `.menu-item:hover`, `.menu-item.active`, `.form-control:focus-visible`, `.check:focus-visible`, `.radio:focus-visible`, `.switch:focus-within`; component overrides `.card`, `.dialog`, `.drawer`, `.alert`, `.popover`, `.list-group`, `.btn`, `.badge`, `.pagination`, `.form-control`, `.form-control:not(textarea)`, `.h1`, `.h2`, `.spinner-border` — or the single at-rule `@media (prefers-reduced-motion: reduce)` whose body contains ONLY a `:root` block (test this explicitly). Also assert: every declaration inside any selector sets a custom property (`--bs-…` or `--…`) only — no `color:`, `padding:`, etc.; and every hex literal in the file is one of the 24 palette hexes (case-insensitive). `TestTokensCssDeclaresPaletteRoles`: assert `:root` declares at least `--bs-primary-bg:`, `--bs-primary-contrast:`, `--bs-primary-fg:`, `--bs-secondary-fg:`, `--bs-accent-bg:`, `--bs-secondary-bg:`, `--bs-success-bg:`, `--bs-warning-bg:`, `--bs-danger-bg:`, `--bs-info-bg:`, `--bs-fg-body:`, `--bs-fg-3:`, `--bs-bg-body:`, `--bs-bg-1:`, `--bs-bg-2:`, `--bs-bg-3:`, `--bs-bg-4:`, `--bs-border-color:`, `--bs-link-color:`, `--bs-body-font-family:`, `--bs-body-font-size:`, `--bs-body-line-height:`, `--bs-font-size-3xl:`, `--bs-font-size-2xl:`, `--bs-radius-pill:`, `--bs-box-shadow:`.

- [ ] **Step 2: Run to verify FAIL** — `go test ./internal/httpserver/ -run TestTokens -v` → FAIL (file missing).

- [ ] **Step 3: Write `static/css/tokens.css`**

Structure (verified token names — all exist in the vendored dist):
1. `:root, :host { color-scheme: light; … }` — force light (belt-and-braces beside `data-bs-theme="light"`).
2. Palette → theme roles (verified names — every one exists in the dist's `:root`): `--bs-primary-base/-bg/-fg/-fg-emphasis/-bg-subtle/-bg-muted/-border/-contrast/-focus-ring` → `#293681` family (hover `#1E2864` arrives via the state rules), `--bs-secondary-bg/-bg-subtle/-border/-contrast/-focus-ring` → `#4274D9` (NOT `-fg`: `--bs-secondary-fg: #687182` is the `.fg-secondary` muted-text role from item 3 and must not be co-opted), `--bs-accent-*` → `#E24A21` with `-bg-subtle: #FDF0ED`, `--bs-success-*` → `#237A57`, `--bs-warning-*` → `#966000`, `--bs-danger-*` → `#B42318`, `--bs-info-*` → `#95CCDD` with `-bg-subtle: #EAF6FA`. Contrast tokens: white on the primary/accent/success/warning/danger fills per DESIGN §2.3. Do NOT invent non-`--bs` names (spec §15.11 allows Bootstrap custom properties only).
3. Foreground/surface chain (verified dependency chain — override the ROOT token, not the consumer): `--bs-fg-body: #121721` (body text), `--bs-primary-fg: #293681` (feeds `.fg-primary`), `--bs-secondary-fg: #687182` (feeds `.fg-secondary`), `--bs-fg-3: #687182` (feeds `.form-control::placeholder` through the dist's `--bs-control-placeholder-color: var(--bs-fg-3)` — this is how DESIGN's "placeholders use text-secondary" is delivered), `--bs-bg-body: #FFFFFF`, `--bs-border-color: #E2E6EB`, `--bs-border-subtle: #ECEFF2`. **`--bs-bg-body` is `#FFFFFF`, never the canvas** (verified: `.card{--bs-card-bg:var(--bs-bg-body)}` — a canvas-valued `--bs-bg-body` paints every card, toast and control grey). DESIGN surfaces arrive as utility classes by overriding the layer tokens: `--bs-bg-1: #FFFFFF` (card), `--bs-bg-2: #F4F6F8` (surface-muted), `--bs-bg-3: #E2E7EC` (surface-pill), `--bs-bg-4: #EDF0F4` (canvas) — pages use `.bg-1/.bg-2/.bg-3/.bg-4` instead of inline colours wherever they fit. `#838EA0` (outer viewport) is used only as inline `background-color` in the teacher/student layouts (legal inline property).
4. Type: `--bs-body-font-family: 'Plus Jakarta Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; --bs-body-font-size: 14px; --bs-body-line-height: 20px; --bs-font-size-3xl: 24px; --bs-font-size-2xl: 20px;` (overriding v6's fluid `clamp()` values with the DESIGN fixed scale).
5. Radius: set `--bs-radius-pill: 9999px` on `:root` (dist default is `50rem`), then the component overrides literally per spec §15.11: `.card { --bs-card-border-radius: 24px }`, `.dialog { --bs-dialog-border-radius: 24px }`, `.drawer { --bs-drawer-border-radius: 24px }`, `.alert { --bs-alert-border-radius: 24px }`, `.popover { --bs-popover-border-radius: 24px; --bs-popover-transition-duration: 0.3s }`, `.list-group { --bs-list-group-border-radius: 18px }`, `.btn { --bs-btn-border-radius: 9999px }`, `.badge { --bs-badge-border-radius: 9999px }`, `.pagination { --bs-pagination-border-radius: 9999px }`, `.form-control { --bs-control-border-radius: 18px }`, `.form-control:not(textarea) { --bs-control-border-radius: 9999px }`, `.h1 { --bs-heading-line-height: 1.25 }`, `.h2 { --bs-heading-line-height: 1.3 }`, `.spinner-border { --bs-spinner-animation-speed: 1.5s }`. Other DESIGN radii (8/32px) reach elements via legal inline `border-radius`; do NOT remap the numeric `--bs-radius-0..9` scale (its values 2–16px do not match DESIGN and pages must not use numeric `.rounded-*` utilities).
6. Elevation: override the dist's shadow tokens with DESIGN §5's four levels (flat/card/floating/tooltip). **Verified token set (the grep is already done — do not re-run it): the dist declares exactly `--bs-box-shadow`, `--bs-box-shadow-xs`, `-sm`, `-lg`, `-xl`, `-inset`; there is NO `--bs-box-shadow-card`.** Pin the mapping (and record it as a CSS comment): flat→`--bs-box-shadow-xs`, card→`--bs-box-shadow-sm`, floating→`--bs-box-shadow-lg`, tooltip→`--bs-box-shadow-xl`, and leave the base `--bs-box-shadow` = the card value so any consumer without a specific token still matches DESIGN.
7. State rules (closed list — each sets ONLY custom properties, to palette values; focus ring `rgba(66, 116, 217, .36)`): `.btn-solid.theme-primary:hover { --bs-btn-hover-bg: #1E2864 }`, `.btn-solid.theme-primary:active { --bs-btn-active-bg: #1E2864 }` (verified: hover/active paint from `--bs-btn-hover-bg`/`--bs-btn-active-bg`, never from `--bs-btn-bg`), `.btn-solid.theme-primary:focus-visible { --bs-btn-focus-ring-color: …; --bs-btn-focus-ring-width: …; --bs-btn-focus-ring-opacity: … }`, `.btn-close:hover { --bs-btn-close-hover-opacity: 1 }`, `.menu-item:hover { --bs-theme-bg-subtle: #E2E7EC; --bs-menu-item-hover-bg: #E2E7EC }`, `.menu-item.active { --bs-theme-bg: #EEF4FA; --bs-theme-fg: #293681; --bs-menu-item-active-bg: #EEF4FA; --bs-menu-item-active-color: #293681 }` (verified: `.menu-item` rules read the `--bs-theme-*` token FIRST and the `--bs-menu-item-*` token as fallback — set both halves or the override is a no-op), `.form-control:focus-visible { --bs-control-box-shadow: var(--bs-focus-ring); --bs-control-border-color: #4274D9 }`, `.check:focus-visible`/`.radio:focus-visible`/`.switch:focus-within` similarly pair `--bs-theme-focus-ring` with the generic token.
8. Motion gate: the reduced-motion collapse of the six DESIGN transition values lives here (custom properties consumed by Bootstrap's transition tokens where they exist; a single `@media (prefers-reduced-motion: reduce)` block collapsing `--bs-transition-*`/animation durations to `0.01ms` is permitted because it sets custom properties only — if the allowlist test rejects the at-rule, keep only the custom-property declarations and note that Bootstrap's own reduced-motion handling covers the rest).

- [ ] **Step 4: Run tests to verify PASS** — `go test ./internal/httpserver/ -run TestTokens -v` → PASS. Iterate the allowlist test only if a legitimate construct was over-restricted (never loosen the palette check).

- [ ] **Step 5: Hand off (no commit)** — orchestrator commits: `feat: add twisty design tokens as the single bootstrap 6 token stylesheet`.

---

### Task 4: Route registry, home redirect, 404 catch-all

**Wave:** W1 (parallel with 2, 3, 5 — it only edits `routes.go`, `routes_home.go`, `routes_test.go`, and the one registration block in `server.go`). **Does not commit — orchestrator commits after review.**

**Files:**
- Create: `internal/httpserver/routes.go`, `internal/httpserver/routes_home.go`, `internal/httpserver/routes_test.go`
- Modify: `internal/httpserver/server.go`

**Interfaces:**
- Consumes: `views.Render` (Task 2 semantics, but the registry is render-agnostic), `/static` route (Task 1).
- Produces (used by Tasks 6–14):

```go
type PageRoute struct {
    Method   string // "GET" or "POST"
    Path     string // e.g. "/login"
    Template string // e.g. "pages/auth/login.html"; must be "" when Redirect != ""
    Redirect string // e.g. "/login"; when set, the handler responds 303 See Other
    Nav      string // active-nav key exposed to templates as PageData.Active
}
type PageData struct { Active string }
func registerPage(r PageRoute)        // called from init() in routes_*.go files
func RegisteredPages() []PageRoute    // defensive copy, for the conformance test
```

- [ ] **Step 1: Write `routes.go`** — the `PageRoute`/`PageData` types, `pageRoutes` package slice, `registerPage`, `RegisteredPages`, and `renderPage(c fiber.Ctx, views *ViewLoader, r PageRoute) error` (redirect via the Fiber v3 builder — `return c.Redirect().Status(fiber.StatusSeeOther).To(r.Redirect)`; the v2 form `c.Redirect(path, code)` does not exist; else render `r.Template` with `PageData{Active: r.Nav}`; render error → 500 text). Doc comments explain the one-file-per-route-group convention that makes parallel page tasks conflict-free.

- [ ] **Step 2: Write the failing tests `routes_test.go`**

`TestRootRedirectsToLogin`: `GET /` → 303, `Location` = `/login`. `TestRegisteredRouteRendersTemplate`: call `registerPage(PageRoute{Method: "GET", Path: "/__test", Template: "hello.html", Nav: "x"})`, build a fresh app, `GET /__test` → 200 containing `Halo, Dunia!`. `TestUnknownPathReturns404`: `GET /definitely-missing` → status **404 only** (do NOT assert the body — Task 6 replaces the plain body with the styled error page). `TestRegisteredPagesIsCopy`: mutating the returned slice does not change a second call.

- [ ] **Step 3: Run to verify FAIL** — `go test ./internal/httpserver/ -run 'TestRoot|TestRegistered|TestUnknown' -v` → FAIL.

- [ ] **Step 4: Implement in `server.go`**

In `NewWithStore`: (a) replace the existing `GET /` hello handler with the loop over `RegisteredPages()` registering each route (`app.Get`/`app.Post` per `Method`, handler closes over the route value and calls `renderPage`); (b) re-add the hello page as a registered route via `routes_home.go`: `GET /hello` → `hello.html`, and `GET /` → `Redirect: "/login"`; (c) after the loop, append the 404 catch-all `app.Use(...)` that renders `pages/error/404.html` when the render succeeds and falls back to `c.Status(404).SendString("halaman tidak ditemukan")` when it does not (so W1 stays functional before Task 6 lands the template). Keep `/healthz` and its tests untouched. Update `TestHelloRoute` in `server_test.go` to hit `/hello` instead of `/` (the only permitted edit to that file).

- [ ] **Step 5: Run all httpserver tests to verify PASS** — `go test ./internal/httpserver/ -count=1` → all green (old hello test now at `/hello`; healthz untouched).

- [ ] **Step 6: `gofmt -w internal/httpserver/ && go vet ./internal/httpserver/`**

- [ ] **Step 7: Hand off (no commit)** — orchestrator commits: `feat: add self-registering page route registry with 404 catch-all`.

---

### Task 5: Icon sprite + shared partials

**Wave:** W1 (parallel with 2, 3, 4). **Does not commit — orchestrator commits after review.**

**Files:**
- Create: `templates/partials/icons.html`, `head.html`, `foot.html`, `navbar-teacher.html`, `navbar-student.html`, `drawer-teacher.html`, `breadcrumb.html`, `empty-state.html`, `banners.html`, `toasts.html`, `no-js-banner.html`, `progress-bar.html`, `stepper.html`, `dialogs.html`, `skeleton.html`
- Test: `internal/httpserver/partials_test.go`

**Interfaces:**
- Consumes: Task 3 asset paths (linked, not required to exist yet).
- Produces: every partial named exactly `partials/<file>.html` per the template contract, consumed by Task 6 layouts and Tasks 7–14 pages. Fixed sub-contracts:
  - `head.html`: `<meta charset="utf-8">`, viewport meta, then `<title>{{template "partials/title" .}}</title>`; the file also contains `{{define "partials/title"}}Twisty Quiz{{end}}` (the default). A page that needs its own title redefines `partials/title` at the top of ITS OWN file — pages are parsed last into their OWN per-page template set, so the page's define wins in that set and cannot leak into any other page (mechanism pinned by `TestPageCanOverrideTitleDefine` in Task 2). Then the two stylesheets IN THIS ORDER: `<link rel="stylesheet" href="/static/bootstrap/bootstrap.min.css">` followed by `<link rel="stylesheet" href="/static/css/tokens.css">` (spec §15.1: tokens after bootstrap; Task 15 asserts the order), then the single `<style>` element containing ONLY the `@font-face` for `/static/fonts/plus-jakarta-sans/PlusJakartaSans-latin.woff2` (`font-weight: 400 700` — one variable binary) plus a comment naming the OFL licence.
  - `foot.html`: emits ONLY, in this order (AC-217): `<noscript>` wrapping `{{template "partials/no-js-banner.html" .}}`, then `<script type="module" src="/static/bootstrap/bootstrap.bundle.min.js"></script>`, then `<script src="/static/jquery/jquery-4.0.0.min.js" defer></script>`, then `<script type="module" src="/static/js/app.js"></script>`. Foot does NOT emit `</body></html>` — layouts close the document after including foot.
  - `navbar-teacher.html`: brand ("TWISTY QUIZ" wordmark + `T` glyph + coral dot via the sprite), 6 links — Dasbor (`dasbor`→`/teacher`), Kuis (`kuis`→`/teacher/quizzes`), Bank Soal (`bank-soal`→`/teacher/questions`), Siswa (`siswa`→`/teacher/students`), Penilaian (`penilaian`→`/teacher/grading`), Pengaturan (`pengaturan`→`/teacher/teachers`) — active link gets `.active` + `aria-current="page"` (highlight driven by `{{if eq .Active "<key>"}}`), a search input (`.form-control` pill with a visually-hidden `<label>`), notification icon button (`aria-label="Notifikasi"`), user pill (avatar "RW" + "Ratna Wijaya"), hamburger button `class="d-inline-flex xl:d-none"` (`.xl:d-none` verified present) that opens the drawer with `data-bs-toggle="drawer" data-bs-target="#nav-drawer"` and carries `aria-label="Buka menu"`.
  - `drawer-teacher.html`: `<dialog class="drawer drawer-end" id="nav-drawer">` (both classes verified) with `.drawer-header`/`.drawer-body` containing the same 6 links + close button (`data-bs-dismiss="drawer"`, `aria-label="Tutup"`).
  - `progress-bar.html`: ONE parameterized partial — Go templates DO pass literal arguments (`{{template "partials/progress-bar.html" 68}}`, verified), so no markup is duplicated across pages. Define `{{define "partials/progress-bar.html"}}` as: `<div class="progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow="{{.}}"><div class="progress-bar theme-primary" style="width: {{.}}%"></div></div>`. The `theme-primary` class is the sanctioned fill (verified: `.progress-bar` paints `var(--bs-theme-bg, var(--bs-progress-bar-bg))`); the inline `width` is on the closed list. Pages supply the literal percent and place their own labels (e.g. "68%" / "17 dari 25 soal") beside it.
  - `empty-state.html`: card + `alert theme-info`? Per spec §15.8: empty = card + `alert theme-info` with one primary action — pin markup: `.card` containing `<div class="alert theme-info">` with h2 (20px) + text + one `.btn btn-solid theme-primary` action, all Indonesian placeholders ("Belum ada data." / "Buat kuis baru").
  - `banners.html`: three defines — `partials/banner-error` (`alert theme-danger` + retry `.btn btn-outline theme-danger`), `partials/banner-stale` (`alert theme-warning` naming that live updates stopped), `partials/banner-locked` (`alert theme-warning` for locked/expired, inputs disabled note). Static defines, no args; pages embed the copy they need by copying the primitive with their own text? No — defines carry fixed Indonesian copy; pages include them wholesale.
  - `toasts.html`: fixed bottom-right `.toast-container` region containing one sample success toast marked `role="status" aria-live="polite"` with a close button (`data-bs-dismiss="toast"`, `aria-label="Tutup notifikasi"`). **Verified facts (no further checking needed):** `.toast` paints `--bs-toast-bg: var(--bs-bg-body)` — it does NOT consume `--theme-*` for its background — and it is `display:none`/`opacity:0` until it carries `.show`. Ship the static demo as `<div class="toast theme-success show" style="background-color: #121721; color: #FFFFFF">` (both inline props are on the closed list; this is DESIGN §7's dark solid treatment for toasts) — `.show` is mandatory or the toast never renders.
  - `skeleton.html`: 3 `.placeholder placeholder-wave` rows inside a container with `aria-busy="true"` and visually-hidden "Memuat…".
  - `stepper.html`: ordered list of 4 authoring steps (Data kuis / Soal / Pengaturan / Terbitkan), current step styled `.badge .theme-primary`, others `.badge` neutral.
  - `dialogs.html`: one shared confirm dialog: `<dialog class="dialog" id="dialog-confirm">` with title "Konfirmasi", body slot copy (generic: "Lanjutkan?"), footer `Batal` (`data-bs-dismiss="dialog"`) + `Konfirmasi` (`.btn btn-solid theme-primary`).
  - `breadcrumb.html`: `<nav aria-label="Breadcrumb"><ol class="breadcrumb">` with `.breadcrumb-link` items and empty `.breadcrumb-divider` separators (v6 shape — verify in the vendored CSS).
  - `icons.html`: sprite root `<svg class="d-none" aria-hidden="true" xmlns="http://www.w3.org/2000/svg">` (NOT an inline `style="display:none"` — `display` is not on the closed inline list; `.d-none` is the sanctioned way and `<use>` still resolves symbol references from a `display:none` tree) containing `<symbol id="i-…" viewBox="0 0 24 24">` entries, stroke-based (`fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"` on the sprite root). **Symbol catalog (exact ids, Lucide-style geometry, drawn by hand):** `i-dashboard, i-quiz, i-bank, i-users, i-grade, i-settings, i-bell, i-search, i-menu, i-plus, i-bolt, i-trend, i-clipboard, i-boxes, i-sparkle, i-tower, i-clock, i-check, i-chev-left, i-chev-right, i-chev-down, i-chev-up, i-arrow-left, i-arrow-right, i-star, i-pencil, i-send, i-grid, i-warn, i-refresh, i-calendar, i-layers, i-cap, i-signout, i-radio, i-image, i-upload, i-trash, i-eye, i-eye-off, i-lock, i-info, i-close, i-person, i-history, i-flag, i-dots`. Every symbol id must be referenced by at least one partial or page by W4 (conformance warns otherwise).

- [ ] **Step 1: Write the failing `partials_test.go`**

`TestEveryPartialDefinesItsName`: for the pinned filename list above, assert the file exists under `templates/partials/` and contains `{{define "partials/<file>"`. `TestIconSpriteHasCatalog`: read `icons.html`, assert every id in the pinned catalog appears as `id="i-…"`, that the sprite root carries `stroke="currentColor"`, and that no `<i class="fa-`/`bi bi-`-style icon-font markup exists anywhere in `templates/partials/`. `TestFootScriptOrder`: `foot.html` contains the bundle `<script type="module"` occurrence BEFORE the jquery `<script` occurrence, and jquery's tag has `defer` and no `type="module"`.

- [ ] **Step 2: Run to verify FAIL** — `go test ./internal/httpserver/ -run TestEveryPartial -v` → FAIL.

- [ ] **Step 3: Write the 15 partials** per the contracts above. Verify every v6 class against the vendored CSS as you go; every icon usage is `<svg …><use href="#i-…"></use></svg>`; no v5 class names; Indonesian copy; no hex outside the palette (tints via inline `background-color: #FDF0ED` etc. are allowed — closed list permits `background-color` with palette hex).

- [ ] **Step 4: Run tests to verify PASS + full suite still green** — `go test ./internal/httpserver/ -count=1`.

- [ ] **Step 5: Hand off (no commit)** — orchestrator commits: `feat: add icon sprite and shared chrome partials`.

---

### Task 6: Layout shells + error pages

**Wave:** W2 (serial; every page task consumes it). **Commits itself.**

**Files:**
- Create: `templates/layouts/base.html`, `auth.html`, `teacher.html`, `student.html`, `error.html`
- Create: `templates/pages/error/404.html`, `templates/pages/error/500.html`
- Create: `internal/httpserver/routes_error.go`
- Test: `internal/httpserver/layouts_test.go`

**Interfaces:**
- Consumes: loader convention (Task 2), partials (Task 5), registry (Task 4).
- Produces: the five layout shells every page task wraps itself in; styled `/404` (catch-all) and `/error/500`. Nav keys the page tasks set: teacher `dasbor|kuis|bank-soal|siswa|penilaian|pengaturan`, student `dasbor|riwayat|profil`.

- [ ] **Step 1: Write the failing `layouts_test.go`**

`TestErrorPagesRender`: `GET /definitely-missing` → 404 with body containing `lang="id"` and "Halaman tidak ditemukan"; `GET /error/500` → 200 containing "Terjadi kesalahan". `TestLayoutWrappersArePresent`: render `/error/500` and assert the body contains `href="/static/bootstrap/bootstrap.min.css"`, `href="/static/css/tokens.css"`, `type="module" src="/static/bootstrap/bootstrap.bundle.min.js"`, `<html lang="id"`, `data-bs-theme="light"`, and exactly one `<style` element (the `@font-face` block).

- [ ] **Step 2: Run to verify FAIL.**

- [ ] **Step 3: Write the five layouts.**

`base.html`: `{{define "layouts/base.html"}}<!doctype html><html lang="id" data-bs-theme="light"> {{template "partials/head.html" .}} <body> {{template "content" .}} {{template "partials/foot.html" .}} </body></html>{{end}}` (body background comes from `--bs-bg-body` via tokens.css — no inline style needed). `auth.html`: the split screen — left brand panel, a plain `<div>` with inline `background-color: #293681` (solid, NOT a gradient: `background-image` is outside the closed inline list — document this deviation from the prototype's gradient in the task report), a `#i-star` sprite mark, wordmark, and 2–3 decorative stat chips; right column = the auth card (`.card`, 24px radius via tokens) holding `{{template "content" .}}`. Below `md`: single column, brand panel collapses to a compact header row. `teacher.html`: `<body style="background-color: #838EA0">` (outer viewport — legal inline prop), then `<div class="container">` wrapping a canvas div with inline `background-color: #EDF0F4; border-radius: 32px`, inside which: `{{template "partials/navbar-teacher.html" .}}`, `{{template "partials/drawer-teacher.html" .}}`, then `{{template "content" .}}`; closes with foot + `</body></html>`. `student.html`: identical frame with `navbar-student` instead (no teacher drawer). `error.html`: centred column (`col-12 md:col-8 mx-auto` — v6 responsive-prefix spelling) `.card` shell.

- [ ] **Step 4: Write the two error pages + `routes_error.go`**

`pages/error/404.html` (content define; title override via the `partials/title` trick; h1 24/30 "Halaman tidak ditemukan", body copy, one primary action `.btn btn-solid theme-primary` → `/`), `pages/error/500.html` ("Terjadi kesalahan pada server", secondary action → `/`). `routes_error.go`: `func init() { registerPage(PageRoute{Method: "GET", Path: "/error/500", Template: "pages/error/500.html"}) }`. The catch-all in `server.go` (Task 4) now renders `pages/error/404.html` successfully — no `server.go` edit needed.

- [ ] **Step 5: Run tests to verify PASS; full suite green; `gofmt -w internal/httpserver/ && go vet ./...`**

- [ ] **Step 6: Commit**

```bash
git add templates/layouts/ templates/pages/error/ internal/httpserver/routes_error.go internal/httpserver/layouts_test.go
git commit -m "feat: add layout shells and styled error pages"
```

---

### Task 7: Auth public — login, join by code, anonymous identify

**Wave:** W3 (parallel ×8). **No commit — orchestrator commits after review:** `feat: add login, join and identify screens`.

**Files:**
- Create: `templates/pages/auth/login.html`, `templates/pages/auth/join.html`, `templates/pages/auth/identify.html`
- Create: `internal/httpserver/routes_auth_public.go`, `internal/httpserver/routes_auth_public_test.go`

**Interfaces:**
- Consumes: `layouts/auth.html` + all Task 5 partials; `registerPage`/`PageRoute`/`PageData` (Task 4); canonical demo data.
- Produces: routes `GET /login` (Nav ""), `GET /join`, `GET /identify`. Titles via the `partials/title` override trick: "Masuk — Twisty Quiz", "Gabung dengan kode kuis — Twisty Quiz", "Identitas peserta — Twisty Quiz". These are the first `/login` renders — Task 4's root redirect starts working.

**Screen pins (prototype: `docs/template/LOGIN.html` — IA/copy intent only):**
- `login.html`: split auth card — role toggle (two `.btn` pills Guru/Siswa, Guru active, `role="group"` + `aria-pressed`), h1 "Selamat datang kembali", labelled pill email + password inputs (`type="email"`/`type="password"`, `autocomplete="email"`/`"current-password"`; placeholder colour arrives from tokens, not inline), "Ingat saya" `.check`, primary "Masuk" (full-width, ≥44px), "Lupa kata sandi?" link, divider, two register cards (Daftar sebagai Guru / Daftar sebagai Siswa → `/register`), and the join promo row → `/join`. Ships the auth family **error** state demo: a `data-state-demo="error"` container with `invalid-feedback` under the email field plus an `alert theme-danger` summary as the first focusable element in the form (`role="alert" tabindex="-1"`) — spec §15.5 form-error shape.
- `join.html`: join card — kicker "Kuis tamu", h1 "Gabung dengan kode kuis", 6-char code input (`.form-control`, uppercase, tabular, `aria-label="Kode kuis"`, placeholder "QUIZ-7F2K"), primary "Gabung", help text "Minta kode kepada guru Anda.", plus the identity note. Ships the auth family **saving** demo: `data-state-demo="saving"` — disabled submit containing `<span class="spinner-border" role="status"><span class="visually-hidden">Memuat…</span></span>`.
- `identify.html`: anonymous identity form — nama lengkap, kelas (select = `.form-control`), jurusan (select), primary "Mulai kuis", privacy note. Ships the auth family **locked/expired** demo: `data-state-demo="locked"` = `alert theme-warning` "Kuis ini sudah berakhir." with disabled inputs.

**State-demo convention (binding for every W3/W4 task):** non-ready states ship as a container `<div hidden data-state-demo="<state>">` carrying the exact Bootstrap primitives from spec §15.8 (`hidden` keeps the static page clean; the backend will unhide one). Task 15's conformance test counts them per surface family.

- [ ] **Step 1: Write the failing `routes_auth_public_test.go`** — one test per page: `TestLoginPage` (`GET /login` → 200; body contains `lang="id"`, "Selamat datang kembali", `btn-solid`, `theme-primary`, `data-state-demo="error"`, and does NOT contain `btn-primary`, `d-lg-block`, `https://`), `TestJoinPage`, `TestIdentifyPage` (same shape with their pinned strings/state demos).
- [ ] **Step 2: Run to verify FAIL** (`-run 'TestLoginPage|TestJoinPage|TestIdentifyPage'` → route not registered).
- [ ] **Step 3: Write the three pages + `routes_auth_public.go`** (`init()` with the three `registerPage` calls). Build markup from the prototypes' IA on the auth layout; every class verified against the vendored CSS; input placeholder colour comes from tokens.css (`--bs-fg-3` → `#687182`, Task 3) — never colour placeholders inline (not on the closed list).
- [ ] **Step 4: Run tests to verify PASS; full suite green; `gofmt -w` the two new Go files; `go vet ./internal/httpserver/`.**
- [ ] **Step 5: Hand off (no commit).**

---

### Task 8: Auth account — register, pending approval, change password

**Wave:** W3 (parallel). **No commit — orchestrator:** `feat: add register, pending and change-password screens`.

**Files:**
- Create: `templates/pages/auth/register.html`, `pending.html`, `change-password.html`
- Create: `internal/httpserver/routes_auth_account.go`, `internal/httpserver/routes_auth_account_test.go`

**Interfaces:**
- Consumes: same as Task 7.
- Produces: `GET /register`, `GET /auth/pending`, `GET /change-password`. Titles: "Daftar akun — Twisty Quiz", "Menunggu persetujuan — Twisty Quiz", "Ubah kata sandi — Twisty Quiz".

**Screen pins:**
- `register.html`: role toggle (Guru default), fields nama lengkap, email, sekolah, kelas/jurusan (student only, shown for the demo), kata sandi + konfirmasi (with eye-toggle icon buttons, `aria-label` "Tampilkan kata sandi"), `.check` agreeing to rules, primary "Daftar akun", link "Sudah punya akun? Masuk" → `/login`. Teacher note: "Akun guru perlu persetujuan admin sekolah." (→ `/auth/pending`).
- `pending.html`: calm centred card — sprite `i-clock` mark, h1 "Menunggu persetujuan admin", body "Akun Anda sebagai guru sedang ditinjau oleh admin SMA Nusantara. Anda akan menerima email setelah disetujui.", secondary action "Kembali ke masuk" → `/login`. (Prototype source: "new" — tone from DESIGN §8 empty state.)
- `change-password.html`: current + new + confirm pill inputs (`.form-control`), requirements list ("Minimal 8 karakter, kombinasi huruf dan angka."), primary "Simpan kata sandi"; includes the toasts partial with one visible success toast "Kata sandi berhasil diubah." (DESIGN §8 saved = brief success toast; `saved` is not a required auth-family state, so no `data-state-demo` here).
- [ ] **Step 1–2:** failing tests (`TestRegisterPage`, `TestPendingPage`, `TestChangePasswordPage` — same assertion shape as Task 7 with pinned strings).
- [ ] **Step 3:** pages + `routes_auth_account.go`.
- [ ] **Step 4:** PASS + suite green + gofmt/vet. **Step 5:** hand off (no commit).

---

### Task 9: Teacher core — dashboard, quiz list, live monitor

**Wave:** W3 (parallel). **No commit — orchestrator:** `feat: add teacher dashboard, quiz list and live monitor`.

**Files:**
- Create: `templates/pages/teacher/dashboard.html`, `quizzes.html`, `monitor.html`
- Create: `internal/httpserver/routes_teacher_core.go`, `internal/httpserver/routes_teacher_core_test.go`

**Interfaces:**
- Consumes: `layouts/teacher.html`, navbar/drawer (nav keys above), `empty-state`, `skeleton`, `banners`, `progress-bar`, `stepper`? (no — authoring owns it), sprite.
- Produces: `GET /teacher` (Nav `dasbor`), `GET /teacher/quizzes` (Nav `kuis`), `GET /teacher/monitor` (Nav `kuis`). Titles: "Dasbor guru — Twisty Quiz", "Daftar kuis — Twisty Quiz", "Monitor kuis — Twisty Quiz".

**Screen pins (prototype: `docs/template/DASHBOARD.html`):**
- `dashboard.html`: welcome header ("Selamat datang kembali, Bu Ratna" + "3 kuis aktif dengan 142 peserta sedang mengerjakan.") + "Buat kuis baru" primary (→ `/teacher/quizzes/edit`); 4 stat tiles (`.card` 18px/20px: 3 Kuis aktif + live badge word "Langsung", 86,2% rata-rata kelulusan + delta pill `#237A57` "+5,4%", 18 esai menunggu, 1.480 soal bank) — tiles in `xl:row-cols-4 md:row-cols-2 row-cols-1`; asymmetric main grid `xl:col-7` (Live Classroom Monitor: highlighted active quiz banner "Biologi Sel — Kuis 4", timer capsule, `progress-bar` (role+ARIA) 68% "17 dari 25 soal", avatar stack, "Pantau langsung" → `/teacher/monitor`) + `xl:col-5` (recent quizzes list panel: 4 quiz rows per the list-panel pattern — 40×40 sprite badge, title, meta label, status badge WITH a word, circular chevron, expandable detail via v6 accordion `.accordion-item`); below: weekly pin-chart panel (vertical stems `#E2E6EB` + `#5589C3` dots via inline `background-color`, active pin dark, day pills Sen–Min with Rab active, big metric "142" + delta pill) — the ONLY chart idiom per DESIGN §7.
- `quizzes.html`: h1 "Daftar kuis" + filter pills (Semua/Aktif/Draf/Selesai segmented `.btn` group) + search pill + "Buat kuis baru" primary; the 4 canonical quizzes as full list rows with status badges (aktif=`badge theme-primary`+word, draf=neutral+word, selesai=dark+word, menunggu penilaian=`theme-warning`+word), row actions (Pantau → `/teacher/monitor`, Kelola → `/teacher/quizzes/edit`); ships the teacher-list family **empty** demo (`data-state-demo="empty"` = `empty-state` partial, "Belum ada kuis." + "Buat kuis baru").
- `monitor.html`: h1 "Monitor langsung — Biologi Sel — Kuis 4" + `badge theme-primary` "Langsung"; progress rail (answered/unanswered counts with `aria-live="polite"` region); participant list (5 canonical students, per-row progress + status: "Mengerjakan"/"Selesai" word badges); focus-loss tally column ("Fokus hilang: 2" in `#B42318` with word); timer capsule; ships live-monitor family states: **loading** (`data-state-demo="loading"` = skeleton with `aria-busy`), **empty** (no participants), **error** (`banner-error` + "Coba lagi"), **stale** (`banner-stale` "Pembaruan langsung terhenti."), **locked/expired** (`banner-locked` "Kuis sudah ditutup.").
- [ ] **Step 1–2:** failing tests `TestTeacherDashboard` (asserts "Selamat datang kembali, Bu Ratna", `xl:row-cols-4`, `role="progressbar"`, `aria-live="polite"`, `btn-solid`), `TestQuizListPage` (asserts all four quiz titles, `data-state-demo="empty"`, a `theme-warning` badge), `TestMonitorPage` (asserts the five student names, `data-state-demo` for loading/stale/locked, `aria-busy`).
- [ ] **Step 3:** pages + `routes_teacher_core.go` (`/teacher`, `/teacher/quizzes`, `/teacher/monitor`).
- [ ] **Step 4:** PASS + suite green + gofmt/vet. **Step 5:** hand off (no commit).

---

### Task 10: Teacher authoring — quiz edit, question editor, image upload, quiz settings

**Wave:** W3 (parallel). **No commit — orchestrator:** `feat: add quiz authoring, question editor, upload and settings screens`.

**Files:**
- Create: `templates/pages/teacher/quiz-edit.html`, `question-editor.html`, `image-upload.html`, `quiz-settings.html`
- Create: `internal/httpserver/routes_teacher_authoring.go`, `internal/httpserver/routes_teacher_authoring_test.go`

**Interfaces:**
- Consumes: layouts/partials/registry; `breadcrumb`, `stepper`, `dialogs`, `toasts`.
- Produces: `GET /teacher/quizzes/edit` (Nav `kuis`), `GET /teacher/questions` (Nav `bank-soal`), `GET /teacher/questions/image` (Nav `bank-soal`), `GET /teacher/quizzes/settings` (Nav `kuis`).

**Screen pins:**
- `quiz-edit.html`: breadcrumb (Dasbor / Kuis / Biologi Sel — Kuis 4), stepper (step 1 active), form card: judul kuis, deskripsi textarea, kelas select (`.form-control` on `<select>`), durasi (menit) + jumlah percobaan inputs, question list panel (5 rows: type badge word "Pilihan Ganda"/"Esai", text excerpt, point, actions Ubah/Hapus icon buttons with `aria-label`), "Tambah soal" outline button → `/teacher/questions`, footer actions Simpan draf (outline) + Terbitkan kuis (primary, opens the shared confirm dialog via `data-bs-toggle="dialog"`); ships authoring family states: **loading** (skeleton demo), **empty** ("Belum ada soal." demo), **error** (`banner-error` demo), **saving** (disabled save + spinner demo), **saved** (toast demo), **locked/expired** (`banner-locked` "Kuis terkunci karena sudah ada pendaftar." demo).
- `question-editor.html`: breadcrumb + stepper (step 2), question-type pill group (Pilihan Ganda active / Esai / Benar-Salah / Isian), question text editor card (textarea + counter), 4 option rows (A–D letter circles, radio `.radio` "Jadikan kunci", Benar/Salah toggles hidden for PG), poin input, "Simpan soal" primary + "Kembali ke kuis" outline → `/teacher/quizzes/edit`; a right rail `.card` with tips panel.
- `image-upload.html`: breadcrumb + stepper (step 2), dropzone card (`.card` dashed? — dashed border via inline `border-color` only if legal; pin: plain bordered card with `i-upload` sprite + "Seret gambar ke sini atau pilih berkas" + `.form-control` file input with label + note "Maksimal 10 MB (JPG/PNG/WebP). Di atas 5 MB otomatis dikompres."), preview row (aspect-ratio inline legal, `object-fit` legal) with Hapus action; ships the **saving** spinner-in-button demo.
- `quiz-settings.html`: breadcrumb + stepper (step 3), advanced settings panel (spec §7 intent, UI only): acak soal (`.switch`), acak opsi, tampilkan skor langsung, kunci setelah submit, izinkan kembali? (disabled with note "Tidak tersedia untuk v1"), minimal panjang esai, jam buka/tutup inputs; footer "Simpan pengaturan" primary; ships **saved** toast + **locked** banner demos.
- [ ] **Step 1–2:** failing tests (one per page; assert pinned strings, `stepper`/`breadcrumb` presence, each owned `data-state-demo`).
- [ ] **Step 3:** pages + `routes_teacher_authoring.go`.
- [ ] **Step 4:** PASS + suite + gofmt/vet. **Step 5:** hand off (no commit).

---

### Task 11: Teacher grading — essay queue + attempt recap

**Wave:** W3 (parallel). **No commit — orchestrator:** `feat: add grading queue and attempt recap screens`.

**Files:**
- Create: `templates/pages/teacher/grading.html`, `recap.html`
- Create: `internal/httpserver/routes_teacher_grading.go`, `internal/httpserver/routes_teacher_grading_test.go`

**Interfaces:**
- Consumes: layouts/partials/registry; `dialogs`, `toasts`, `banners`.
- Produces: `GET /teacher/grading` (Nav `penilaian`), `GET /teacher/recap` (Nav `penilaian`).

**Screen pins:**
- `grading.html`: h1 "Penilaian esai" + stat strip (18 menunggu / "2 tugas jatuh tempo malam ini"); queue rows per the grading row pattern: student avatar+name+class, quiz title, question excerpt, "Menunggu penilaian" `badge theme-warning` + word; the open grading panel for Ayu Lestari: the student's essay answer in a `.card`, rubric score input (0–100), feedback textarea, "Simpan penilaian" primary (spinner demo = **saving**), **error** demo (`banner-error` "Gagal menyimpan. Jawaban tidak hilang."), **empty** demo ("Semua esai sudah dinilai."), **loading** skeleton demo, **locked** demo (`banner-locked` "Kuis dikunci — skor final terkunci."), **saved** toast demo.
- `recap.html`: h1 "Rekap percobaan — Ayu Lestari"; header `.card` (quiz, kelas, waktu mulai "09.15 WIB", durasi, skor besar 84 tabular, `badge theme-success` "Selesai"); per-question recap rows (number, question text, student answer box, key, per-question point, word badge Benar/Salah/Menunggu); focus-loss note ("Fokus hilang: 2 kali" `#B42318` word badge); actions "Kembali ke penilaian" → `/teacher/grading`; ships **error** + **locked** demos.
- [ ] **Step 1–2:** failing tests with pinned strings + state demos. [ ] **Step 3:** pages + routes file. [ ] **Step 4:** PASS + suite + gofmt/vet. [ ] **Step 5:** hand off (no commit).

---

### Task 12: Teacher management — students, anonymous participants, majors, teachers

**Wave:** W3 (parallel). **No commit — orchestrator:** `feat: add students, anonymous, majors and teacher management screens`.

**Files:**
- Create: `templates/pages/teacher/students.html`, `anonymous.html`, `majors.html`, `teachers.html`
- Create: `internal/httpserver/routes_teacher_mgmt.go`, `internal/httpserver/routes_teacher_mgmt_test.go`

**Interfaces:**
- Consumes: layouts/partials/registry; `empty-state`, `skeleton`, `banners`; `.table-responsive` wrappers.
- Produces: `GET /teacher/students` (Nav `siswa`), `GET /teacher/anonymous` (Nav `siswa`), `GET /teacher/majors` (Nav `pengaturan`), `GET /teacher/teachers` (Nav `pengaturan`).

**Screen pins (this task owns the teacher-list family state demos — loading/empty/error):**
- `students.html`: h1 "Kelola siswa" + class filter pills (Semua kelas + 4 canonical classes) + search + "Tambah siswa" primary; `.table-responsive` table (Nama, Kelas, Email, Status word badge "Aktif"/"Nonaktif", actions Ubah/Detil icon buttons `aria-label`); 5 canonical students; ships **loading** (`skeleton`, `aria-busy`), **empty** ("Belum ada siswa di kelas ini."), **error** (`banner-error` + retry) demos.
- `anonymous.html`: h1 "Peserta anonim" + explainer `alert theme-info` ("Peserta tanpa akun diidentifikasi lewat cookie peserta; baris ini tidak terhubung ke akun siswa."); table (Nama, Kuis, Bergabung "09.15 WIB", Status); 3 rows; word badges only.
- `majors.html`: h1 "Jurusan"; `.list-group` rows (IPA, IPS, Agama… keep: "IPA", "IPS", "Bahasa", "Kejuruan") with counts "32 siswa" and edit icon buttons; "Tambah jurusan" primary.
- `teachers.html`: h1 "Kelola guru"; sections: "Menunggu persetujuan" (1 pending row: nama + email + Setujui primary / Tolak outline via confirm dialog) and "Guru aktif" (Ratna Wijaya + 2 others with status word badges and a nonaktifkan outline-danger action); ships no extra states (family owner is this task's students page).
- [ ] **Step 1–2:** failing tests (pinned strings incl. all 5 student names on students page, a `table-responsive` wrapper around each table, and the three `data-state-demo` values on students). [ ] **Step 3:** pages + routes file. [ ] **Step 4:** PASS + suite + gofmt/vet. [ ] **Step 5:** hand off (no commit).

---

### Task 13: Student area — dashboard, history, profile (+ student-family states)

**Wave:** W3 (parallel). **No commit — orchestrator:** `feat: add student dashboard, history and profile screens`.

**Files:**
- Create: `templates/pages/student/dashboard.html`, `student/history.html`, `profile.html`
- Create: `internal/httpserver/routes_student.go`, `internal/httpserver/routes_student_test.go`

**Interfaces:**
- Consumes: `layouts/student.html` (profile uses it too), navbar-student (keys `dasbor|riwayat|profil`), partials.
- Produces: `GET /student` (Nav `dasbor`), `GET /student/history` (Nav `riwayat`), `GET /profile` (Nav `profil`).

**Screen pins:**
- `dashboard.html`: h1 "Halo, Ayu" + class "XII IPA 2"; active-quiz `.card` for "Kuis Pengetahuan Umum — Sesi 1" (badge "Terbuka", meta "5 soal · 20 menit · Percobaan tunggal", primary "Mulai kuis" → `/attempt`); upcoming list (2 rows "Fisika Kinematika — Kuis 3" "Besok, 09.00 WIB" + word badge "Terjadwal"); recent results (2 rows with score pills + word badges "Selesai"); ships student family **loading** (skeleton demo) + **empty** ("Tidak ada kuis aktif.") + **error** (`banner-error`) demos.
- `history.html`: h1 "Riwayat kuis"; filter pills (Semua/Bulan ini); `.table-responsive` table (Kuis, Tanggal "20 Okt 2026", Skor "84" tabular, Status word badge); 4 rows; empty/error demos not required (family owner = dashboard).
- `profile.html`: profile `.card` (avatar "AL", nama Ayu Lestari, kelas, email, sekolah), "Ubah kata sandi" outline → `/change-password`, "Keluar" outline (`i-signout`) → `/login`; edit form section (nama + kelas disabled with note "Ubah data lewat admin sekolah.").
- [ ] **Step 1–2:** failing tests (pinned strings: "Halo, Ayu", "Mulai kuis", "Riwayat kuis", the 4 history rows, `data-state-demo` values on dashboard). [ ] **Step 3:** pages + routes file. [ ] **Step 4:** PASS + suite + gofmt/vet. [ ] **Step 5:** hand off (no commit).

---

### Task 14: Attempt flow — question, review, result (+ app.js behaviour)

**Wave:** W3 (parallel). **No commit — orchestrator:** `feat: add attempt question, review and result screens with client behaviour`.

**Files:**
- Create: `templates/pages/attempt/question.html`, `review.html`, `result.html`
- Create: `internal/httpserver/routes_attempt.go`, `internal/httpserver/routes_attempt_test.go`
- Modify: `static/js/app.js` (the ONLY file in W3 that a task may modify that another task created — Task 1 shipped it as a stub)

**Interfaces:**
- Consumes: `layouts/base.html` (attempt column `col-12 xl:col-8 mx-auto`), `progress-bar`, `dialogs`, `toasts`, sprite.
- Produces: `GET /attempt` (question 1 of 5), `GET /attempt/review`, `GET /attempt/result`. Titles: "Kuis Pengetahuan Umum — Twisty Quiz", "Tinjau jawaban — Twisty Quiz", "Hasil kuis — Twisty Quiz". `app.js` behaviour: essay character counter + autosave-indicator flash (no network, no fetch, no SSE — backend wiring is a later plan).

**Screen pins (prototype: `docs/template/QUIZ.html` — its Indonesian copy is already correct and may be reused verbatim):**
- `question.html`: sticky top bar (brand glyph + quiz title + student chip "Ayu Lestari · XII IPA 2") with progress (inline progress markup: `role="progressbar"` + `aria-valuenow="20"` + fill `style="width: 20%"`, labels "Soal 1 dari 5" and the **`aria-live="polite"`** answered counter "Terjawab: 0/5"); desktop left navigator `.card` (5 question rows: number circles, type words, active/answered/unanswered states with word legend); mobile horizontal ribbon `class="d-flex xl:d-none"` + "Lihat semua" opening `<dialog class="drawer drawer-bottom" id="nav-drawer">` grid navigator (keyboard-closable — native dialog); question card: "SOAL 1 DARI 5", type badge "Pilihan Ganda", question text ≥16px (the Mars question), 4 full-width option rows (letter circles, `.radio` inputs, selected state = `#EEF4FA` surface + word check mark — never colour alone); autosave indicator ("Jawaban tersimpan otomatis"); desktop nav buttons Sebelumnya (disabled + `aria-disabled`) / Tinjau jawaban → `/attempt/review` / Selanjutnya; mobile sticky bottom bar (Bootstrap `.fixed-bottom` + inline `padding-bottom: env(safe-area-inset-bottom)`, legal). Ships attempt-family state demos: **error** (`banner-error` "Gagal menyimpan jawaban. Periksa koneksi Anda." + retry), **saving** (disabled button + spinner), **saved** (autosave toast demo), **locked/expired** (`banner-locked` "Waktu habis — kuis terkunci."). NOTE: attempt surfaces carry NO loading state (spec §15.8).
- `review.html`: h1 "Tinjau jawaban Anda"; unanswered warning (**1 kosong: nomor 5** — `alert theme-warning`: "Anda memiliki 1 soal yang belum dijawab."); 5 review rows (number, type word, question, answer preview or "—" + word badge Terjawab/Belum, "Ubah" → `/attempt`); actions "Kembali ke soal" outline + "Kumpulkan jawaban" primary → shared confirm dialog (`data-bs-toggle="dialog" data-bs-target="#dialog-confirm"`, "Ya, kumpulkan sekarang" → `/attempt/result`). Answered pattern (canonical, matches the result page): nomor 1–4 Terjawab, nomor 5 Belum.
- `result.html`: centred success card — success circle sprite `i-check`, h1 "Jawaban berhasil dikumpulkan!", body "Terima kasih, Ayu. Jawaban Anda untuk Kuis Pengetahuan Umum — Sesi 1 sudah tersimpan."; 3 summary tiles (5 Total soal / 4 Terjawab / 1 Kosong, tabular); submit time "Rabu, 21 Oktober 2026, 09.42 WIB"; note "Skor dihitung setelah guru menyelesaikan penilaian."; action "Kembali ke dasbor" → `/student`. Ships **error** + **locked** demos.
- `app.js`: `document.addEventListener("DOMContentLoaded", …)` — wire `#essay-input` (when present) to update `#char-count` ("N karakter") and toggle the `show` class on `#autosave-indicator` for 1200 ms on each input; guard everything with `if (el)`; NO network calls of any kind (`fetch`, `XMLHttpRequest`, `EventSource`, `sendBeacon` are banned — Task 15's `TestAppJsHasNoNetworkCalls` enforces this; SSE/autosave requests arrive with the backend plan). Note: the essay wiring stays DORMANT in this milestone because the static demo renders the multiple-choice question 1; it is shipped now so the backend plan only renders an essay question to activate it. Keep the file an ES module (`export {}`).
- [ ] **Step 1–2:** failing tests `TestAttemptQuestion` (asserts "Soal 1 dari 5", `role="progressbar"`, `aria-valuenow`, `aria-live="polite"`, "Pilihan Ganda", the Mars option set, `drawer`, `data-state-demo` for error/locked, and NO `data-state-demo="loading"`), `TestAttemptReview` (5 review markers, "1 soal yang belum dijawab", "Kumpulkan jawaban", one "Belum" badge and four "Terjawab" markers), `TestAttemptResult` ("Jawaban berhasil dikumpulkan", "09.42 WIB", the three summary numbers 5/4/1).
- [ ] **Step 3:** pages + `routes_attempt.go` + `app.js` behaviour.
- [ ] **Step 4:** PASS + suite green + gofmt/vet. **Step 5:** hand off (no commit).

---

### Task 15: Conformance suite, integration, image build

**Wave:** W4 (serial). **Commits itself.** Runs AFTER every W3 task's files have landed and each W3 commit has been made by the orchestrator.

**Files:**
- Create: `internal/httpserver/conformance_test.go`
- Modify: only if a conformance failure names a defect — then fix the OWNING file minimally and note it in the report.

**Interfaces:**
- Consumes: `RegisteredPages()` (Task 4), all templates, `static/css/tokens.css`, the whole W0–W3 output.
- Produces: the green whole-frontend gate every later plan must keep.

- [ ] **Step 1: `TestEveryRegisteredPageRenders`** — loop `RegisteredPages()`: for each GET route, `app.Test` → 200; body contains `<html lang="id"`, `data-bs-theme="light"`, `href="/static/bootstrap/bootstrap.min.css"`, `href="/static/css/tokens.css"` with the tokens link appearing AFTER the bootstrap link in the body (spec §15.1), `type="module" src="/static/bootstrap/bootstrap.bundle.min.js"`, and a non-empty `<title>`; for routes whose Template starts with `pages/` additionally assert the title is not the legacy `"Quiz"`. Exactly ONE `<style` element per body, whose contents contain `@font-face` and no CSS selector lines (only `@font-face{…}` blocks + comments). Skip `Redirect` routes here; assert each redirect target is itself a registered path (catches `/login` pointing nowhere).
- [ ] **Step 2: `TestNoBootstrapV5Names`** — walk every file under `templates/`, applying two ban lists. (a) CLASS-TOKEN bans — parse every `class="…"` attribute value and split on whitespace; fail on any token equal to: `btn-primary`, `btn-outline-primary`, `text-primary`, `text-muted`, `text-white`, `text-bg-primary`, `form-select`, `form-check-input`, `modal`, `modal-dialog`, `modal-content`, `offcanvas`, `offcanvas-header`, `offcanvas-body`, `dropdown-menu`, `dropdown-item`, `placeholder-glow`, `d-lg-block`, `d-md-block`, `d-md-flex`, `d-sm-block`, `d-xl-flex`, `d-md-none`, `d-lg-none`, `fs-1`, `fs-2`, `fs-3`, `fs-4`, `fs-5`, `fs-6`, `sr-only`, `text-body`, `btn-block`, `font-weight-bold`, `font-italic`, `bg-opacity-10`, `text-opacity-75`, `border-opacity-25`, `bg-body-secondary`, `bg-body-tertiary`, `accordion-button`, `accordion-collapse` (the v6 accordion is details-based — its v6 classes `.accordion-item`/`.accordion-header`/`.accordion-body`/`.accordion-icon` are all allowed), `dropdown-toggle`, `link-underline`, `ratio`, `spinner-border-sm`, or any token matching `^col-(sm|md|lg|xl)-` or `^row-cols-(sm|md|lg|xl)-` or `^d-(sm|md|lg|xl)-(block|flex|inline)` or `^(p|m|pt|pb|ps|pe|px|py|mt|mb|ms|me|mx|my|gap)-(sm|md|lg|xl)-` or `^(flex|justify-content|align-items|text|w|h)-(sm|md|lg|xl)-` (prefix-responsive v5 forms — the v6 spellings are `md:col-6`, `md:p-5`, `md:flex-row`, `md:text-center` etc., i.e. `BREAKPOINT:utility` for EVERY utility family, not just grid/display). **Do NOT ban these v6-present names (verified present in the dist — a ban would fail the plan's own markup): `nav-tabs`, `nav-pills`, `pagination`, `page-link`, `list-group`, `card-header`, `card-footer`, `alert-link`, `input-group-text`, `object-fit-cover`, `list-inline-item`, `popover-header`, `tooltip-inner`, `text-decoration-none`, `navbar-nav`, `navbar-brand`, `accordion-item`, `accordion-header`, `accordion-body`.**. (b) RAW-SUBSTRING bans over the whole file: `data-bs-toggle="modal"`, `data-bs-toggle="offcanvas"`, `data-bs-toggle="dropdown"`, `window.bootstrap`, `cdn.`, `bootstrap@`, `code.jquery`, `fonts.googleapis`, `outline: none`, `outline:none`, `fa-solid`, `fa-regular`, `bi bi-`, plus any emoji codepoint (regex U+1F000–U+1FAFF). (Token-wise matching avoids false positives such as `dialog-open` or class names merely containing "modal".)
- [ ] **Step 3: `TestPaletteHexOnly`** — regex `#[0-9A-Fa-f]{6}\b` over `templates/**` and `static/css/*.css`; every hit must be one of the 24 palette hexes (case-insensitive).
- [ ] **Step 4: `TestIconRefsResolve`** — collect every `href="#i-…"` in `templates/`; assert each referenced id exists in `partials/icons.html`. Accessibility rules on the same walk: every `<svg>` element that contains a `<use>` carries `aria-hidden="true"`, and every `<button>`/`<a>` whose only content is such an SVG carries an `aria-label`.
- [ ] **Step 5: `TestStateDemosCoverSurfaceFamilies`** — table of required `data-state-demo` values per family → owning page(s), pinned in this plan: auth→{error,saving,locked} (login/join/identify), teacher-lists→{loading,empty,error} (students + quizzes), live-monitor→{loading,empty,error,stale,locked} (monitor), grading→{loading,empty,error,saving,saved,locked} (grading), authoring→{loading,empty,error,saving,saved,locked} (quiz-edit + image-upload + quiz-settings), attempt→{error,saving,saved,locked} (question), result→{error,locked} (result), student→{loading,empty,error} (student dashboard). Assert each value appears inside a `hidden` container with `data-state-demo="<v>"` on a page of that family.
- [ ] **Step 6: `TestScriptWiring` + `TestAppJsHasNoNetworkCalls`** — from a rendered page body: bundle script tag has `type="module"` and appears before the jQuery tag; jQuery tag has `defer` and no `type="module"`; `window.bootstrap` absent. Additionally read `static/js/app.js` and fail if it contains `fetch(`, `XMLHttpRequest`, `EventSource`, `WebSocket`, or `sendBeacon` — client network calls are backend-plan work (spec §13/§15.1), not this milestone's.
- [ ] **Step 7: Run everything**

```bash
gofmt -l .                     # must print nothing
go vet ./...
go build ./...
go test ./... -count=1
docker build -t quiz-v2:dev .  # if a daemon is available; else note as pending manual gate
```

If a conformance test fails on a W3 page, fix the page minimally (the orchestrator owns cross-task fixes), rerun, and list every fix in the report.

- [ ] **Step 8: Commit**

```bash
git add internal/httpserver/conformance_test.go
git commit -m "test: add whole-frontend template conformance suite"
```

---

## Self-review notes (re-run after the correction pass)

- Spec coverage: every one of the 25 screens + 2 error pages maps to a task (inventory §15.7 ↔ Tasks 6–14); carrier/vendoring → Task 1; token stylesheet + closed override lists → Task 3; state contract §15.8 → per-family state demos pinned in Tasks 7–14 and verified in Task 15; icons §15.9 → Task 5; copy §15.10 → Global Constraints (i18n extraction explicitly deferred, documented); styling mechanism §15.11 → Global Constraints + Tasks 3/15; a11y §15.5 → pinned assertions (Task 14 ARIA, Task 15 scans).
- Facts re-verified against the repo/module cache in the correction pass: Fiber v3.6.1 has no `app.Static` (static middleware path pinned); Fiber v3 redirect is the builder API (303 pinned); `--bs-secondary-fg`/`--bs-primary-fg`/`--bs-fg-3`/`--bs-bg-1..4`/`--bs-control-placeholder-color` chains confirmed in the vendored dist; `.drawer-end`/`.drawer-bottom`/`.xl:d-none`/`.rounded-pill` confirmed; Dockerfile/.dockerignore contents matched to the pinned COPY line.
- Deliberate, documented deviations: auth brand panel uses solid `#293681` instead of a gradient (inline `background-image` is outside the closed property list); `partials/strings.html` deferred with `internal/i18n` (frontend-only milestone); attempt Prev/Next navigate between the three static attempt routes rather than question-to-question (one hardcoded question), because multi-question SSR navigation needs backend routes; inline `font-variant-numeric: tabular-nums` is the single sanctioned addition to the inline closed list (no dist mechanism exists; flagged for spec ratification); the app.js essay wiring ships dormant (static demo renders the MC question).
- Parallel safety: the only shared files (`server.go`, `app.js`) are edited in serial waves (W0/W1 for `server.go`; `app.js` stub in W0, behaviour in the single W3 task that owns it). Every W3 task owns ≥1 exclusive template, ≥1 exclusive Go route file, ≥1 exclusive test file. Every remaining "verify in the dist" note is a same-session grep of `assets/bootstrap-6.0.0-alpha/css/bootstrap.min.css`, never a guess.
