---
version: alpha
name: Twisty Quiz
description: Soft, calm dashboard aesthetic for a school quiz system — rounded geometry, a muted cool-gray canvas, pure white floating cards, and one deep-navy action colour with coral used sparingly as a warm accent.
colors:
  primary: "#293681"
  primary-dark: "#1E2864"
  on-primary: "#FFFFFF"
  secondary: "#4274D9"
  on-secondary: "#FFFFFF"
  accent: "#E24A21"
  accent-light: "#FDF0ED"
  accent-blue: "#5589C3"
  accent-blue-light: "#EEF4FA"
  info: "#95CCDD"
  info-soft: "#EAF6FA"
  tint: "#D0E7E6"
  canvas: "#EDF0F4"
  card: "#FFFFFF"
  surface-muted: "#F4F6F8"
  surface-pill: "#E2E7EC"
  app-bg: "#838EA0"
  text-primary: "#121721"
  text-secondary: "#687182"
  text-muted: "#94A3B8"
  border-light: "#E2E6EB"
  border-subtle: "#ECEFF2"
  success: "#237A57"
  warning: "#966000"
  danger: "#B42318"
  inactive: "#6B7B85"
typography:
  display-metric:
    fontFamily: Gotham Rounded
    fontSize: 32px
    fontWeight: 700
    lineHeight: 38px
  h1:
    fontFamily: Gotham Rounded
    fontSize: 24px
    fontWeight: 600
    lineHeight: 30px
  h2:
    fontFamily: Gotham Rounded
    fontSize: 20px
    fontWeight: 600
    lineHeight: 26px
  metric:
    fontFamily: Gotham Rounded
    fontSize: 24px
    fontWeight: 700
    lineHeight: 28px
  body-prominent:
    fontFamily: Gotham Rounded
    fontSize: 16px
    fontWeight: 600
    lineHeight: 22px
  body:
    fontFamily: Gotham Rounded
    fontSize: 14px
    fontWeight: 400
    lineHeight: 20px
  label:
    fontFamily: Gotham Rounded
    fontSize: 13px
    fontWeight: 500
    lineHeight: 18px
  badge:
    fontFamily: Gotham Rounded
    fontSize: 11px
    fontWeight: 600
    lineHeight: 14px
rounded:
  sm: 8px
  md: 12px
  lg: 18px
  card: 24px
  container: 32px
  full: 9999px
spacing:
  2xs: 4px
  xs: 8px
  sm: 12px
  md: 16px
  lg: 20px
  xl: 24px
  2xl: 32px
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.full}"
    padding: 12px
  button-primary-hover:
    backgroundColor: "{colors.primary-dark}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.full}"
  button-secondary:
    backgroundColor: "{colors.card}"
    textColor: "{colors.primary}"
    rounded: "{rounded.full}"
    padding: 12px
  button-accent:
    backgroundColor: "{colors.accent-light}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    padding: 12px
  panel-card:
    backgroundColor: "{colors.card}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.card}"
    padding: 24px
  panel-card-muted:
    backgroundColor: "{colors.surface-muted}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.lg}"
    padding: 16px
  stat-tile:
    backgroundColor: "{colors.card}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.lg}"
    padding: 20px
  stat-tile-metric:
    backgroundColor: "{colors.card}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.lg}"
    typography:
      fontFamily: Gotham Rounded
      fontSize: 32px
      fontWeight: 700
      lineHeight: 38px
  badge-neutral:
    backgroundColor: "{colors.surface-pill}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    padding: 4px
  badge-accent:
    backgroundColor: "{colors.accent-light}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    padding: 4px
  badge-dark:
    backgroundColor: "{colors.text-primary}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.full}"
    padding: 4px
  badge-success:
    backgroundColor: "{colors.success}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.full}"
    padding: 4px
  badge-warning:
    backgroundColor: "{colors.warning}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.full}"
    padding: 4px
  badge-danger:
    backgroundColor: "{colors.danger}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.full}"
    padding: 4px
  badge-inactive:
    backgroundColor: "{colors.surface-muted}"
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.full}"
    padding: 4px
  badge-info:
    backgroundColor: "{colors.info}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    padding: 4px
  badge-info-soft:
    backgroundColor: "{colors.info-soft}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    padding: 4px
  badge-tint:
    backgroundColor: "{colors.tint}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    padding: 4px
  badge-blue:
    backgroundColor: "{colors.accent-blue-light}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    padding: 4px
  input-field:
    backgroundColor: "{colors.card}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    padding: 12px
  input-placeholder:
    backgroundColor: "{colors.card}"
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.full}"
    padding: 12px
  muted-marker:
    backgroundColor: "{colors.text-muted}"
    size: 8px
  input-shell-muted:
    backgroundColor: "{colors.surface-muted}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    padding: 12px
  nav-link-active:
    backgroundColor: "{colors.surface-pill}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    padding: 8px
  nav-link-idle:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    padding: 8px
  accent-marker:
    backgroundColor: "{colors.accent}"
    height: 3px
  inactive-marker:
    backgroundColor: "{colors.inactive}"
    size: 8px
  divider-light:
    backgroundColor: "{colors.border-light}"
    height: 1px
  divider-subtle:
    backgroundColor: "{colors.border-subtle}"
    height: 1px
  tooltip-dark:
    backgroundColor: "{colors.text-primary}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.md}"
    padding: 8px
  progress-fill:
    backgroundColor: "{colors.secondary}"
    rounded: "{rounded.full}"
    height: 6px
  chart-pin:
    backgroundColor: "{colors.accent-blue}"
    rounded: "{rounded.full}"
    size: 10px
  chart-pin-active:
    backgroundColor: "{colors.text-primary}"
    rounded: "{rounded.full}"
    size: 12px
  app-frame:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.text-primary}"
  app-viewport:
    backgroundColor: "{colors.app-bg}"
---

# Twisty Quiz Design System

## 1. Overview

Twisty Quiz is the visual system for a single-school quiz system. Teachers
author and run mixed-type quizzes, monitor attempts live, grade essays, and
review every attempt; students take a quiz on their own device, one question
at a time.

It is a calm, soft-surfaced dashboard aesthetic: high-radius geometry, a
muted cool-gray canvas, pure white floating cards, and one deep-navy action
colour with coral used sparingly as a warm accent.

- **Visual attributes**: soft-depth influence, pill-shaped interactive
  elements, generous negative space, refined data density.
- **Mood**: calm before a quiz, focused during a quiz, precise when
  presenting scores.

The three working prototypes in `docs/template/` are the visual reference:
`LOGIN.html` (auth, join, register), `DASHBOARD.html` (teacher workspace),
`QUIZ.html` (student attempt flow). Their information architecture, screen
order, and copy intent are normative; their markup, CDN links, and ad-hoc CSS
are not — they are ported onto the tokens in this file.

## 2. Colors

### 2.1 Palette roles

| Token | Hex | Role |
| :--- | :--- | :--- |
| `primary` | `#293681` | The single action colour: primary buttons, active nav marker, key headings, strong progress fill. |
| `primary-dark` | `#1E2864` | Primary button hover and active state. |
| `secondary` | `#4274D9` | Secondary actions, gradients, progress fills, informational emphasis. |
| `accent` | `#E24A21` | Sparse warm accent: role badges, live/attention markers, critical stat deltas. Never a page background. |
| `accent-light` | `#FDF0ED` | Tint fill behind `accent` content. |
| `accent-blue` | `#5589C3` | Data marks: chart pins and dots, intermediate badges. |
| `accent-blue-light` | `#EEF4FA` | Tint fill behind `accent-blue` content. |
| `info` | `#95CCDD` | Progress tracks, selected surfaces, soft highlights. Never small white text on it. |
| `info-soft` | `#EAF6FA` | Quietest information surface. |
| `tint` | `#D0E7E6` | Calm secondary surface: quiz cards, empty-state panels. |
| `canvas` | `#EDF0F4` | App frame background — the surface cards float on. |
| `card` | `#FFFFFF` | Every elevated card, input, and popover. |
| `surface-muted` | `#F4F6F8` | Sub-card rows and nested panels. |
| `surface-pill` | `#E2E7EC` | Tag pills, inactive segmented controls. |
| `app-bg` | `#838EA0` | Outer viewport surrounding the app shell. |
| `text-primary` | `#121721` | Headings, question text, figures. Never `#000000`. |
| `text-secondary` | `#687182` | Descriptions, dates, metadata, helper text, input placeholders. |
| `text-muted` | `#94A3B8` | Decorative marks and disabled indicators only — never text on a light surface (2.56:1 on white). |
| `border-light` | `#E2E6EB` | Card borders, dividers, separators. |
| `border-subtle` | `#ECEFF2` | Hairline dividers inside a card. |
| `success` | `#237A57` | Submitted, saved, grading complete. |
| `warning` | `#966000` | Awaiting grading, attention needed. |
| `danger` | `#B42318` | Validation errors, failed save, focus-loss count. |
| `inactive` | `#6B7B85` | Draft, disabled, expired context. |

### 2.2 Color rules

- The palette reaches the UI through Bootstrap 6's theme custom properties
  in `static/css/tokens.css` — the single token stylesheet, and the only
  place a hex from this table is **defined**. Authored markup may repeat a
  hex from this table in an inline `style` attribute (spec §15.11's closed
  inline property list); it may never introduce a hex that is not in this
  table. See spec §15.11.
- Cards resting on the canvas are always `#FFFFFF`.
- `#293681` is the only high-emphasis action fill. One primary button per
  screen region.
- `#E24A21` is an accent, not a fill for large areas and not a body-text
  colour on white.
- Dark solid backgrounds (`#121721`) are reserved for high-contrast pills,
  tooltips, and the active chart marker.
- State is never carried by colour alone — every badge pairs the colour with
  a word.

### 2.3 Contrast floors (measured, WCAG 2.2)

Normal text ≥ 4.5:1; large text (≥ 20px bold / 24px) ≥ 3:1; non-text UI
(focus ring, borders, progress track versus fill) ≥ 3:1.

| Foreground on background | Ratio | Use |
| :--- | :--- | :--- |
| `#121721` on `#FFFFFF` | 17.95 | Body and headings — AAA |
| `#121721` on `#EDF0F4` | 15.70 | Body on canvas — AAA |
| `#687182` on `#FFFFFF` | 4.91 | Secondary text — AA |
| `#687182` on `#F4F6F8` | 4.54 | Secondary text on muted — AA |
| `#293681` on `#FFFFFF` | 10.83 | Primary text on card — AAA |
| `#FFFFFF` on `#293681` | 10.83 | Text on primary button — AAA |
| `#121721` on `#E2E7EC` | 14.42 | Text on pill — AAA |
| `#121721` on `#95CCDD` | 10.22 | Text on info — AAA |
| `#121721` on `#D0E7E6` | 13.89 | Text on tint — AAA |
| `#121721` on `#FDF0ED` | 16.12 | Text on accent tint — AAA |
| `#237A57` / `#966000` / `#B42318` on `#FFFFFF` | 5.26 / 5.29 / 6.57 | Semantic text — AA |

Restricted pairs — **never** body text at normal size: `#94A3B8` on white
(2.56), `#FFFFFF` on `#95CCDD` (1.76), `#FFFFFF` on `#D0E7E6` (1.29),
`#E24A21` on `#FFFFFF` (4.01 — large text only), `#FFFFFF` on `#5589C3`
(3.65 — large text only). Input placeholders use `text-secondary`, not
`text-muted` — a placeholder carries instruction the reader needs.

## 3. Typography

**Gotham Rounded** — a geometric sans with rounded terminals — for everything.
One family, no serif/sans pairing.

Stack: `'Gotham Rounded', -apple-system, BlinkMacSystemFont, 'Segoe UI',
Roboto, sans-serif`.

Self-hosted from `assets/gotham-rounded/` and served from
`static/fonts/gotham-rounded/` as eight woff2 faces: Light 300, Book 400,
Medium 500 and Bold 700, each with an italic. Every face is declared in the
single `<style>` block in `partials/head.html`. No font CDN in production.

The four weights this system uses (400/500/600/700) map onto Book, Medium and
Bold; 600 has no dedicated file and is synthesised between Medium and Bold.
These are licensed commercial fonts (Hoefler & Co.); the copyright notice
ships beside them as `COPYRIGHT.txt` and grants no redistribution right — see
the licensing note in the product spec §"Rejected alternatives".

**Delivery.** Bootstrap 6.0.0-alpha.1 ships fluid display sizes
(`clamp()`) and a unitless `1.2` heading line-height, so the exact pairs
below are **not** reachable through Bootstrap's own typography classes. The
scale is delivered by the token layer only: `--bs-body-font-size`,
`--bs-body-line-height`, `--bs-font-size-3xl` (h1) and `--bs-font-size-2xl`
(h2) in `static/css/tokens.css`, plus the closed component-override list
in spec §15.11 for the two heading line-heights. The remaining steps —
display-metric, metric, body-prominent, label, badge — are applied inline
(`font-size` / `line-height`), which spec §15.11 permits. `.fs-*`
utilities and headings below `h2` are not used anywhere in this system.

| Style | Size / line-height | Weight | Use |
| :--- | :--- | :--- | :--- |
| Display / metric | 32 / 38 px | 700 | Large percentage metrics (`+20%`). |
| H1 / card title | 24 / 30 px | 600 | Page and panel titles. |
| H2 / section title | 20 / 26 px | 600 | Section titles. |
| Metric value | 24 / 28 px | 700 | Stat counters (`64`, `12`). |
| Body prominent | 16 / 22 px | 600 | Question text, participant names. |
| Body regular | 14 / 20 px | 400 | Descriptions, option text, excerpts. |
| Label / nav | 13 / 18 px | 500 | Navigation links, input labels, rates. |
| Badge / caption | 11 / 14 px | 600 | State tags, counts, day markers. |

Question body text is at least 16px so a student can read it on a phone.
The attempt question line is set at 20px/600 (mirroring the h2 step) — kept
deliberately above the 16px body-prominent so the one text a student must
read dominates the card.
Scores, timers, and counts use tabular numerals so digits do not shift.

## 4. Layout

- **App frame** — centred container, max width `1280px`–`1440px`, minimum
  outer margin `24px`. The canvas (`#EDF0F4`) fills the frame; the viewport
  behind it is `#838EA0`.
- **Teacher workspace** — asymmetric two-column grid: a narrow left rail
  (navigation plus at-a-glance status) beside a wider content column. The
  content column stacks a row of stat tiles, then a feature panel, then a
  list panel.
- **Student attempt** — a single focused column, max `960px`. One question
  per page; dashboard chrome and unrelated actions are removed. The
  question navigator is a card of numbered pills — inline above the question
  in portrait, a sticky left rail from `xl` — not a drawer (decided against
  the prototype's drawer during the type-data port).
- **Auth** — a split screen: an atmospheric brand panel beside a centred
  auth card.
- **Spacing scale** — base unit `4px`. Common steps `8` / `12` / `16` /
  `24` / `32` px. Cards use `24px` internal padding; panel gaps are
  `24px`–`32px`.

## 5. Elevation & Depth

| Level | Value | Use |
| :--- | :--- | :--- |
| Flat | `0 1px 2px rgba(18, 23, 33, 0.02)` | Resting surfaces. |
| Card | `0 10px 25px -4px rgba(18, 23, 33, 0.04)` | Panels and tiles. |
| Floating | `0 14px 35px -6px rgba(18, 23, 33, 0.08)` | Prominent cards, drawers, sticky bars. |
| Tooltip | `0 4px 12px rgba(18, 23, 33, 0.12)` | Chart tooltips, popovers. |

Cards are separated by whitespace and a 1px `#E2E6EB` border first; a shadow
is the exception, not the default separator.

**Layer order:** canvas (`z-0`) → cards (`z-1`) → floating cards, drawers and
sticky bars (`z-2`) → tooltips, popovers and modals (`z-3`).

## 6. Shapes

- **Main container**: `32px`.
- **Primary content cards**: `24px`.
- **Interactive rows / sub-cards**: `18px`.
- **Buttons, badges, search fields, status pills**: `9999px` (fully round).

**Delivery.** The carrier's own radius scale is `0 2 3 4 6 8 10 12 14 16px`
plus a pill token, and each component binds its radius to an element-level
custom property (`.card { --bs-card-border-radius: var(--bs-radius-7) }`,
and likewise for button, badge, alert, dialog, drawer, list-group,
popover). The values above are therefore applied by the closed
component-override list in spec §15.11, which sets those custom properties
per component; a single `:root` radius override cannot express a `24px`
card and a pill button at the same time.

## 7. Components

### Top navigation bar
Brand group left (navy logo glyph, uppercase bold wordmark `TWISTY QUIZ`),
nav links centre-left, search and utilities right. The active nav item shows
`#121721` text on a `#E2E7EC` pill with a coral underline marker; inactive
items are `#687182`.

The teacher task nav has four destinations: Dasbor, Kuis, Penilaian, and
Manajemen. Manajemen is the single entry point to the management section; its
tab bar (Siswa, Peserta anonim, Jurusan, Guru) is the only sub-navigation, so
no management tab is repeated as a task-nav item.

Account access is a utility, not a task destination, and exactly one control
owns it per width: the avatar pill is the only route to the profile screen
from `lg` (1024px) up, and the drawer's account row — avatar, name, class or
role, chevron — is the only route below `lg`. The task nav never carries a
"Profil" item beside it. The pill always names itself: `Profil` until `xl`
(1280px), then the person's name and role.

Sign-out obeys the same rule through a swap: the drawer's labelled button
below `md` (768px), the top-bar icon from `md` up — the drawer is the only
copy reachable while it covers the bar on a phone.

### Page navigation and action hierarchy

Every screen answers "where am I going back to?" and "what is the one action
here?" in the same places.

**Back is navigation, never a button.** One idiom: `link-quiet` with a 16px
`arrow-left`, first element in the page's reading order and above the `h1`,
labelled with its destination ("Kembali ke daftar kuis", never a bare
"Kembali"). Attempt screens have no page header: the result screen puts its
quiet link in the attempt top bar's trailing slot, while the live question and
review screens keep it in the question-navigation row, so leaving during an
attempt is never a permanently visible header action. An in-flow contextual
link that sits beside a directional button keeps `link-quiet` but drops the
arrow, so it does not read as a second "previous". A header action that points
at the same URL as the back link is removed: one destination, one affordance.

**A detail screen is the hub for its drill-downs.** A quiz's detail screen is the hub for results and monitor: each drill-down opens from it, carries one quiet back link to it, and offers at most one job control of its own.

**Acting is a button, in three tiers.**

| Tier | Class | Use |
| :--- | :--- | :--- |
| Primary | `btn btn-solid theme-primary` | The one action the screen region exists for: save, submit, create, next. |
| Secondary | `btn btn-outline theme-primary` | Every other action: cancel, filter submit, secondary header actions, row actions. |
| Destructive | `btn btn-outline theme-danger` | Delete, sign-out, destructive confirmation. |

Row actions add `btn-sm`; an inline copy button beside a code or URL field is
always `btn-sm`, so the same job never changes size between screens. A button
never navigates and a link never submits. Copy is sentence case and uses the
product's Indonesian vocabulary consistently: "Ubah" (not "Edit"), "Simpan",
"Batal", "Hapus", "Kembali ke …".

### Stat tile
White card, `18px` radius, `20px` padding. A `32px` large figure in
`text-primary`, a `13px` label in `text-secondary`, an icon badge on a muted
tint, and an optional delta pill (coral for negative, `success` for
positive).

### Panel card
White card, `24px` radius, `24px` padding. The header carries a `32px`
tinted icon badge, a bold title, and an optional pill dropdown. Content sits
directly on the card surface.

### Chart card (data graphic)
Vertical pin chart: thin `#E2E6EB` stems, `#5589C3` dots. The active pin sits
in a vertical capsule with a `#121721` dot and a dark floating tooltip above
it. This is the only chart idiom in the system; decorative charts do not
exist.

### Status badges
| State | Treatment |
| :--- | :--- |
| Open | `primary` fill, white text. |
| Closed | `surface-pill` fill, `text-primary` text. |
| Draft | `surface-pill` fill, `text-secondary` text. |
| Submitted | `success` fill, white text. |
| Awaiting grading | `warning` fill, white text. |
| Focus-loss recorded | `danger` fill, white text, with the count. |
| Role / level accent | `accent-light` fill with `text-primary` text. |
| Metadata tag | `surface-pill` fill, `text-primary` text, pill shape, `11px`. |

### List panel
Flat rows separated by `#ECEFF2` dividers. Each row: a `40×40px` rounded icon
badge, the title, a metadata label, a status badge, and a circular chevron
that expands the row to reveal tags, a description, and a timestamp.

### Forms
Inputs and selects are fully round (`9999px`), white, with `13px` medium
labels in `text-secondary` and a visible focus ring. Validation errors are
`danger` text with `invalid-feedback` copy directly under the field.

**Act only when there is something to act on.** A save row is not furniture:
the profile screen hides `Simpan perubahan` until a field differs from the
server-rendered value, and hides `Unggah foto` until a file is selected. A
rejected submit re-renders with the row visible; no-JS keeps it visible. The
rule is progressive enhancement, not a security boundary.

**Profile photo editor.** The photo card merges the identity anchor and the
picker: the current avatar and account line sit beside a drag-and-drop target.
A chosen file opens a square crop stage with a dark surround and a
rule-of-thirds grid; pointer drag and arrow keys pan, the slider (plus wheel
and `+`/`-`) zooms, and a thumbnail shows the exported result. The crop is
exported into the same `photo` field, so the server's decode, crop, and encode
remain authoritative and the no-JS multipart path still works. An explanation
that the screen does not need on every visit lives behind one info icon beside
the page title, not in a permanent note above the form; the dialog that opens
uses the shared `dialog` component.

### Split auth panel
A brand panel (navy gradient, decorative stat chips, product promise) beside
a centred auth card (white, `24px` radius) containing the role toggle,
sign-in form, join-by-code form, and register form.

### Student attempt page
A sticky top bar with the quiz title and live answered count; the question
card with question text, optional image, and full-width option rows; a
progress rail showing answered / total / unanswered; a question-navigator
card with numbered pills and no status legend (fill tone carries the state;
`aria-current` / `aria-label` carry it for assistive tech); and an action bar
carrying Previous / Next, sticky bottom on phones. Answers commit
automatically when a choice changes or typing pauses — Save exists only as
the no-JS fallback. From `xl` the navigator becomes a sticky left rail beside
the question.

## 8. Component states

Every surface ships these states as distinct treatments. Silent failure is a
defect.

| State | Treatment |
| :--- | :--- |
| Loading | Server-rendered first paint; a skeleton placeholder only on a filter swap, with `aria-busy="true"`. |
| Ready | The populated surface. |
| Empty | A white card with a calm message and, when there is one, a single primary action; where the only available destination is the page's own back link, the action is omitted rather than duplicated. |
| Error | A `danger` alert with a retry action; the previous content stays visible. |
| Saving | A spinner inside the control, which is disabled, plus `role="status"`. |
| Saved | Automatic answers save silently — the live answered count is the confirmation; an explicit fallback save shows a brief success toast. Never a permanent state marker. |
| Stale | A persistent `warning` banner explaining that live updates stopped. |
| Offline / no-JS | A `warning` banner naming exactly what is unavailable. |

The monitor and results screens apply this rule to their empty states: the
message stays, and no action is rendered when the only destination would repeat
the back link above it.

## 9. Responsive behavior

The two thresholds are Bootstrap 6's `md` (768px) and `xl` (1280px).
Bootstrap 6 has no 1200px breakpoint; `xl` is the wide-workspace
threshold.

- **Below 768px** — single column. Metric tiles stack one per row. The teacher
  navigation becomes a drawer. The student action bar becomes a sticky
  bottom bar with `padding-bottom: env(safe-area-inset-bottom)` and never
  covers an open input. Wide tables scroll inside a `table-responsive`
  wrapper.
- **768px–1279px** — two-column metric grids; content column centred.
- **1280px and above** — four-column metric grids; full asymmetric workspace;
  the attempt sets its navigator as a sticky left rail beside the question and
  may use its full `960px` width.

## 10. Accessibility

Target: **WCAG 2.2 AA**, verified by an automated contrast check plus a manual
keyboard pass.

- The contrast floors and restricted-pair list in §2.3 are normative.
- Focus ring: the Bootstrap focus ring, unmodified. `outline: none` is
  forbidden.
- Keyboard: every control is reachable in DOM order with a visible focus
  indicator. Dialogs trap Tab and return focus to the invoking control. The
  question drawer is closable by keyboard.
- Screen readers: every image carries an `alt`; every input has a
  programmatic `<label>`; spinners use `role="status"` with visually hidden
  text; progress bars carry `role="progressbar"` with `aria-valuenow`,
  `aria-valuemin`, `aria-valuemax`; the answered-count region is
  `aria-live="polite"`.
- Touch targets: primary actions on a phone are at least 44px tall and full
  width.
- Copy is sentence case.

## 11. Motion

Restrained and short. Motion is only ever feedback for a state change.

| Purpose | Value |
| :--- | :--- |
| Control colour-state transition | `0.15s cubic-bezier(0.34, 1.56, 0.64, 1)` |
| Collapse expand / collapse | `0.35s ease` |
| Dialog / drawer / popover | `0.3s cubic-bezier(0.22, 1, 0.36, 1)` |
| Progress width fill | `0.6s ease` |
| Toast show / hide | `0.15s linear` |
| Spinner | `1.5s linear` |

Every transition is gated behind `prefers-reduced-motion: no-preference`;
under `reduce`, animation and transition duration collapse to `0.01ms` and
iteration count to 1. No ambient motion, no entrance choreography, no
decorative animation.

**Delivery.** The carrier already matches this table for control colour
states (`0.15s`), dialogs and drawers (`0.3s`), toasts (`0.15s`) and the
progress fill (`width 0.6s ease`). Two values differ: the popover is
`0.15s` in the carrier against `0.3s` here, and the spinner is `1s` against
`1.5s` here. Both tokens are declared on the component rather than at
`:root`, so they are set by the closed component-override list in spec
§15.11. No other transition or animation value may be introduced.

## 12. Do's and Don'ts

### Do's
- **Do** maintain extreme corner rounding: pills for tags and buttons,
  `24px`+ for cards.
- **Do** keep canvas backgrounds strictly in cool slate-gray (`#EDF0F4`) so
  white cards pop.
- **Do** use minimal, high-contrast badges for status markers instead of
  long text strings.
- **Do** preserve generous whitespace between sections (minimum `24px`
  gutter).
- **Do** pair every status colour with a word, and an icon where one exists.

### Don'ts
- **Don't** use pure black (`#000000`) for text; use `#121721`.
- **Don't** apply harsh, dark, or saturated box-shadows. Shadows must remain
  soft, diffuse, and ambient.
- **Don't** use sharp corners (`0px` or `4px` radius) on buttons, containers,
  or tags.
- **Don't** introduce a second warm accent; the warm accent is `#E24A21` and
  the cool data accent is `#5589C3`.
- **Don't** ship a dark mode. This system is light-only; adding one is a new
  decision entry, not a builder's choice.
- **Don't** load any stylesheet, script, or font from a CDN in production.