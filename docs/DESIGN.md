---
title: "quiz-v2 — UI Design Direction"
product: "Daily Quiz for Teachers"
status: "Draft"
updated: "2026-10-12"
source: "PRD — quiz-v2 (Daily Quiz for Teachers)"
---

# quiz-v2 — UI Design Direction

## 1. Design Intent

### Product definition

**quiz-v2 is a single-school daily assessment workspace for teachers and students.** Teachers use it to build and run quizzes, monitor attempts, grade essays, and review recaps. Students use it to discover an available quiz, complete it with minimal friction, and understand their results when the teacher permits them to do so. Anonymous participants enter through a quiz code and follow the same quiz-taking flow without receiving an account or personal dashboard.

### Primary design goal

Make the next meaningful action obvious at every point in the assessment lifecycle: **create → open → run → review → grade → recap**. The interface should feel calm before a quiz, focused during a quiz, and precise when presenting scores or incidents.

### Visual reference interpretation

Use the supplied TalentPro dashboard as a **layout reference, not as a content or branding template**:

- A compact global header for product identity, date/context, and one prominent action.
- An asymmetric workspace: a narrow left rail for navigation and useful at-a-glance information, with a larger content area for the current task.
- A clear page title followed by large summary metrics.
- Short filter tabs, a search field, and a grid/list of scannable cards.
- Soft surfaces, restrained borders, generous spacing, and compact secondary metadata.

Adapt the recruiting dashboard's job cards and hiring insights into quiz cards, assessment activity, session status, and academic recaps. Do not reproduce the orange recruiting accent, recruiting copy, or recruitment-specific gauges.

### Signature design decision: the Daily Pulse rail

The memorable element is a **Daily Pulse rail**: a compact panel in the left workspace column that summarizes today's quiz activity and, when relevant, the currently running matching session. It uses a deep-blue heading, a pale-yellow attention surface for one timely reminder, and a short status line rather than a decorative gauge. It should help a teacher answer “What needs my attention today?” without competing with the primary workspace.

This is a visual organization of existing quiz/session/recap information, not a new scheduling, recommendation, or notification feature. Do not invent activity data when the underlying product has none.

## 2. Design Principles

1. **Task before decoration.** Each screen has one primary action. Secondary actions remain visually quieter and do not compete with it.
2. **Calm, high-contrast workspace.** Use the blue palette for hierarchy and actions, pale yellow only for attention or context, and neutral surfaces for most content.
3. **Assessment state is always explicit.** Distinguish Draft, Open, Closed, Waiting, Live, Paused, Stopped, Submitted, Awaiting essay grading, and Final where applicable. Color is never the only indicator; pair it with text and/or an icon.
4. **Trust the server.** Display server-controlled quiz state, remaining-time information, saved/submitted status, shuffled order, and final scores as authoritative. A local countdown is only a visual projection of server time.
5. **One question per page means one question per page.** During a quiz, remove dashboard chrome and unrelated actions. Keep question content, answer controls, progress, timer (if enabled), and navigation clearly separated.
6. **Responsive by design.** Important flows must work in a phone browser without requiring hover, a wide table, or precise pointer control.
7. **Reveal complexity progressively.** Keep cards concise; put detailed settings, question editing, attempt answers, and incident timelines on detail screens or focused panels.
8. **Respect score visibility settings.** Never reveal scores or correct answers from a UI component unless the quiz's server-provided visibility rules allow it.

## 3. Visual Tokens

### 3.1 Core palette

The following four colors are the required brand palette from the supplied palette reference.

| Token | Hex | Intended use | Usage guidance |
|---|---|---|---|
| `brand-900` | `#086288` | Primary brand blue | Primary buttons, active navigation marker, key headings/links, strong progress indicators. Use white text on this color. |
| `brand-600` | `#4D8DBA` | Secondary blue | Secondary actions, selected non-primary controls, informational iconography, subdued data emphasis. |
| `brand-200` | `#97D7FF` | Light blue | Soft selected surfaces, progress tracks, subtle highlights, focus-adjacent accents. Avoid small white text on this background. |
| `accent-100` | `#FFEFC4` | Warm pale-yellow | Daily Pulse reminder, due/attention context, quiet callout backgrounds. Do not use as a general page background or for large areas of body text. |

**Palette intent:** blue carries the product identity and interaction hierarchy; pale yellow is a rare, warm counterpoint. Use the light blue as a supporting tint rather than a competing primary accent.

### 3.2 Neutral palette

These neutral values complement the required palette; they are not additional brand colors.

| Token | Suggested value | Intended use |
|---|---|---|
| `canvas` | `#F6F9FB` | Main page background; slightly cool and very light. |
| `surface` | `#FFFFFF` | Cards, form panels, dialogs, question surfaces. |
| `surface-subtle` | `#F0F5F8` | Secondary panels, inactive segmented controls, table headers. |
| `border` | `#DCE6EC` | Card outlines, input borders, dividers. Keep contrast perceptible but quiet. |
| `text-primary` | `#172B36` | Main headings, question text, labels, important values. |
| `text-secondary` | `#5C6F7B` | Metadata, hints, supporting descriptions. |
| `text-muted` | `#71828C` | Low-priority timestamps and tertiary metadata; never use for essential instructions. |
| `text-on-brand` | `#FFFFFF` | Text/icons on `brand-900` or other sufficiently dark surfaces. |
| `scrim` | `rgba(13, 39, 53, 0.42)` | Modal backdrops. |

### 3.3 Semantic colors

Keep semantic states distinct from the brand palette. They are functional status colors, not new brand accents.

| State | Suggested token | Use |
|---|---|---|
| Success / complete | `#237A57` | Submitted, saved, grading complete. Pair with a label or icon. |
| Warning / attention | `#966000` | Focus-loss warning, unresolved essay grading, attention needed. Use a pale neutral/yellow surface behind it. |
| Error / destructive | `#B42318` | Validation errors, failed save/submit, destructive confirmation. |
| Informational | `brand-600` | Non-blocking details and server/session information. |
| Neutral / inactive | `#6B7B85` | Draft, disabled/expired context, inactive states. |

For semantic badges, use a pale tinted background with a darker text/icon. Do not set body text in pale `brand-200` or pale yellow.

### 3.4 Typography

Use a clean sans-serif system with clear distinction between display values, task text, and utilities. Typography should feel contemporary and teacher-friendly rather than playful or overly corporate.

| Role | Preferred stack | Guidance |
|---|---|---|
| Display / page title | `Manrope`, `Inter`, system sans-serif | Use for page titles and large metric values. Prefer medium/semibold weights, not oversized marketing typography. |
| Body / controls | `Inter`, system sans-serif | Use for navigation, forms, question content, buttons, helper text. |
| Numeric / time utility | `Inter`, system sans-serif with tabular numerals | Use tabular figures for timers, counts, scores, and timestamps to prevent shifting. |

No external font dependency is required. If custom fonts are not shipped with the app, the interface must retain its intended hierarchy with system fallbacks.

#### Type scale

| Token | Size / line-height | Use |
|---|---|---|
| `text-xs` | 12 / 16 px | Compact metadata, timestamps, card tags. Never use for critical instructions. |
| `text-sm` | 14 / 20 px | Secondary descriptions, labels, helper text. |
| `text-md` | 16 / 24 px | Standard body text and controls. |
| `text-lg` | 18 / 26 px | Card titles, question titles where needed. |
| `heading-sm` | 20 / 28 px | Section headings. |
| `heading-md` | 24 / 32 px | Page headings. |
| `metric-lg` | 32–40 / 1.1 | Key dashboard metrics, only when the value is genuinely important. |

For question content, prioritize reading comfort. Use at least 16 px body text on mobile, with additional separation between paragraphs, answer options, and images.

## 4. Layout and Spacing

### 4.1 Desktop workspace

Use a compact top header and a two-column workspace. At wide desktop sizes, the left workspace rail should be approximately **248–272 px**, with the primary content taking the remaining width. Avoid forcing three card columns when content becomes cramped.

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ quiz-v2 / school context           Date or session context   [Create quiz]  │
├──────────────────────┬───────────────────────────────────────────────────────┤
│ Workspace navigation │ Breadcrumb / page title          Primary page action  │
│                      │                                                       │
│ Dashboard            │ Metric A      Metric B      Metric C      Metric D     │
│ Quizzes              │                                                       │
│ Students             │ Filters / search / sort                               │
│ Essay grading        │                                                       │
│ Recaps               │ Quiz cards, table, question editor, or live monitor    │
│                      │                                                       │
│ ─ Daily Pulse ─      │                                                       │
│ Today's activity     │                                                       │
│ Live session status  │                                                       │
│ Attention reminder   │                                                       │
└──────────────────────┴───────────────────────────────────────────────────────┘
```

The precise sidebar items depend on the actor and current route. Do not display teacher-only navigation for students or anonymous participants.

### 4.2 Page composition

Typical management pages follow this order:

1. Breadcrumb when useful, then a clear page title and concise supporting line.
2. A primary action aligned with the title (for example, **Create quiz** or **Save question**).
3. Summary metrics only when relevant to the page.
4. Filters/search immediately above the content they control.
5. Main content: cards for discovery and high-level quiz management; tables or structured lists for recaps and grading queues.
6. Contextual empty/loading/error state in the content region, not an unrelated global banner.

Do not force metric cards onto every screen. The quiz-taking, question-editing, essay-grading, and anonymous-entry flows should favor task focus over dashboard density.

### 4.3 Grid, spacing, and shape

- Use a 4 px base spacing unit. Common gaps: 8, 12, 16, 24, and 32 px.
- Desktop content padding: 24–32 px; tablet: 20–24 px; mobile: 16 px.
- Card grid: 3 columns on spacious desktop, 2 on medium screens, 1 on mobile. Cards should remain readable before adding another column.
- Card radius: 14–16 px. Inputs/buttons: 10–12 px. Pills: fully rounded only where they represent compact filters or status labels.
- Use a soft shadow sparingly, mainly for layered menus and dialogs. Cards should usually be separated by whitespace and a subtle border, not heavy shadows.
- Border widths: 1 px for standard surfaces; 2 px for clear selected/focus states.
- Avoid glassmorphism, strong gradients, large decorative blobs, or multiple competing accent colors.

### 4.4 Signature component — Daily Pulse

On the teacher workspace, the lower sidebar contains a compact **Daily Pulse** panel:

- A clear title and a short summary of today's relevant quiz activity, using only available data.
- A small live-session status when a matching quiz is waiting, live, paused, or otherwise needs the teacher's attention.
- At most one pale-yellow reminder at a time, such as an unresolved essay-grading queue or an active focus-loss incident count, when that information is already available from the relevant feature.
- A direct text link to the relevant destination; no decorative gauge or unexplained score.

On short screens, the panel may move below the main navigation or become a dismissible/collapsible section. Do not let it obscure navigation or quiz content.

## 5. Navigation and Global Chrome

### 5.1 Teacher navigation

Recommended primary navigation:

- **Dashboard** — overview and Daily Pulse context.
- **Quizzes** — create, edit, open/close, and manage quiz settings.
- **Students** — teacher-managed student accounts.
- **Essay grading** — essays awaiting manual grading and grading history where needed.
- **Recaps** — quiz attempts and score/answer/cheat-count/timing details.

The profile menu contains account/session actions that are actually implemented. Do not add navigation for out-of-scope features such as exports, classes, reusable question banks, or notifications.

### 5.2 Student navigation

Use a simpler navigation model:

- **Available quizzes** — visible quizzes that the student may inspect, including closed or not-open quizzes only when the server marks them visible.
- **My results** — attempt history and personal analytics with the defined 3-day, 7-day, 30-day, and all-time filters.
- **Profile** — supported account/profile actions only.

On mobile, reduce navigation to a compact menu or bottom navigation only if all destinations remain clear and keyboard/screen-reader accessible. Anonymous participants do not get account navigation or a history/profile area.

### 5.3 Header

The header is low-height and quiet. It may contain the product mark, current school context, an optional date/session indicator, the principal action, and a profile menu for authenticated users. Do not fill the header with decorative icon-only shortcuts. Every icon-only control needs an accessible name and a visible focus state.

Use **Create quiz** as the teacher's prominent action when on dashboard or quiz management pages. On other routes, the primary action should match the task (for example, **Save question**, **Submit quiz**, or **Finish session**).

## 6. Core Screens and Component Direction

### 6.1 Authentication and first access

**Purpose:** Let teachers and students sign in or register without distraction.

- Use a centered, moderate-width form surface on a light canvas rather than the full management dashboard layout.
- Show the `quiz-v2` identity and a short task-oriented heading.
- Use explicit labels for email and password, reveal-password control if implemented, field-level validation, and a single prominent submit button.
- Student self-registration must not expose teacher account creation. The seeded teacher is a deployment/bootstrap concern, not a public registration option.
- Preserve form values after recoverable errors where safe; provide a specific error message rather than a generic failure.
- Avoid promising email verification, OTP, SSO, or password policy behavior that is out of scope.

### 6.2 Teacher dashboard

**Purpose:** Surface current quiz activity and provide quick access to core teacher tasks.

Recommended hierarchy:

- Page title: **Dashboard** with a concise context line such as “Your quiz activity at a glance.”
- Four small-to-medium metric cards for the PRD's teacher analytics: **Students**, **Scores given**, **Questions**, and **Quizzes**. Values and definitions must come from documented product rules; do not invent formulas for ambiguous metrics.
- A primary content area with recent/relevant quizzes represented as quiz cards.
- A visible entry point to essay grading when there are essays awaiting review.
- The Daily Pulse rail for relevant activity and a live matching-session state.

Each metric card uses one strong number, a short label, and optional compact supporting context. Avoid decorative charts or interpreting trends until the analytics formulas are defined.

**Quiz card content:** quiz title; question count; quiz mode (Normal / Matching); availability status (Draft / Open / Closed); relevant timer or question-shuffle summary only when useful; and one clear primary destination such as **Manage quiz** or **View recap**. Do not expose participant answer details on the card.

### 6.3 Quiz list and management

**Purpose:** Find and manage a teacher-owned quiz quickly.

- Title: **Quizzes**; primary action: **Create quiz**.
- Provide concise status filters such as **All**, **Draft**, **Open**, and **Closed**. These labels filter the relevant quiz records and must not be confused with **Show / Hide**, which controls student-dashboard visibility.
- Add search when the list can reasonably become long. Search is not a substitute for status filters.
- Use a three-column card grid on wide screens, reducing to two and then one column as space narrows.
- Keep destructive actions (delete) in a menu or separate confirmation flow. Do not make the entire card's click behavior conflict with nested actions.
- Show an empty state with a next step, e.g. “No quizzes yet” and **Create your first quiz**. Include a separate filtered-empty message when there are quizzes but none match the selected filter.

### 6.4 Quiz creation and settings

**Purpose:** Let teachers create a quiz and configure the assessment rules without missing consequential settings.

Use a focused multi-section form or step-by-step page, but do not introduce extra wizard stages that make creation unnecessarily slow. Group settings in this order:

1. **Basics** — quiz title and the supported descriptive fields, if present in the data model.
2. **Questions** — add/edit/reorder questions and their options.
3. **Quiz behavior** — Normal / Matching mode; attempt setting; shuffle questions; shuffle answers; timer mode (None / Per question / Total) and seconds; result visibility flags; token/code mode and anonymous participation toggle.
4. **Availability** — Open / Closed and Show / Hide, presented as distinct controls with clear explanations.
5. **Review** — a compact read-only summary before opening or starting where it helps prevent configuration mistakes.

Keep related controls together, and explain the practical effect of less-obvious settings in plain language. Avoid presenting internal implementation terms such as SSE or server authority to the user.

The UI must distinguish **Save draft** from **Open quiz** where the underlying behavior supports both. Changing a quiz's state should show the resulting state and any relevant constraint. Do not imply that opening a quiz starts a matching session; matching-session start is a separate teacher-controlled action.

### 6.5 Question editor

**Purpose:** Build single-choice, multiple-choice, and essay questions accurately.

- Present questions as individually numbered editor panels, with clear question type labels—not unexplained color-only markers.
- Provide a type selector for **Single choice**, **Multiple choice**, and **Essay**.
- Single-choice: allow exactly one correct option.
- Multiple-choice: allow one or more correct options, with an explanatory hint that scoring is all-or-nothing.
- Essay: make it clear that the teacher will assign a score from 0 to 10 later.
- Support one optional image per question with a visible preview, replacement/removal action, upload progress, and readable failure state. Show file size/format constraints near the upload control; accepted formats are JPG/JPEG, PNG, WEBP, and GIF. The server handles compression for files over 10 MB, so do not imply that the browser must perform that compression.
- Use explicit add-option, remove-option, and move/reorder controls. On mobile, ensure reorder actions are operable without drag-and-drop.
- Validate before save and display errors next to the relevant field. Preserve valid entered content when a validation error occurs.
- Offer a clear preview of how students will see the question, including the optional image and answer options.

### 6.6 Student quiz discovery

**Purpose:** Help students identify what they can take now, without confusing visibility and availability.

- Separate whether a quiz is **visible** from whether it is **open for new attempts**.
- Quiz cards show title, question count, mode, and a text status such as **Open**, **Closed**, or **Not available yet** when that state exists in the product model.
- Use a clear **Start quiz** or **Register** action based on the actual flow and state. Do not show an enabled start action for a closed quiz.
- Avoid teacher-only metadata, cheat counters belonging to other students, or aggregate answer information.
- Include the entry to join with a code for the token/code flow, without making anonymous participation look like account registration.

### 6.7 Student registration and anonymous code entry

**Purpose:** Reduce friction before a quiz begins and make identity requirements clear.

For registered students:

- Show the quiz context and the required registration/start action.
- Explain that registration can be cancelled before the quiz starts, where that action is available.

For anonymous participants:

- Use a dedicated compact entry screen: quiz code, entered name, and class only if the flow requires it as defined by the application.
- Explain that the name is attached to this quiz attempt and does not create an account or history page.
- Validate an invalid/expired code with a specific, actionable error.
- Do not display login, profile, or personal-history navigation in the anonymous flow.

### 6.8 Waiting room for matching quizzes

**Purpose:** Show that the participant has joined and is waiting for the teacher to control the session.

- Display the quiz title, the current state (**Waiting for teacher**, **Live**, **Paused**, or terminal state), and clear next-step text.
- Before start, show a waiting indicator and only the participant information appropriate to the user's role. Avoid exposing unnecessary personal data about other participants.
- When the teacher starts the session, switch to the quiz-taking view based on the server's state update.
- If the connection drops, make the connection state visible and explain whether the participant should wait or retry. Reconcile the screen with the server state after reconnecting.
- The waiting room becomes locked after the session starts as defined by the PRD. Do not present registration-cancellation controls once they are no longer allowed.

### 6.9 Quiz-taking view — one question per page

**Purpose:** Maximize focus, readability, and confidence during answering.

This view intentionally abandons the full dashboard layout. Do not show the sidebar, general analytics, unrelated links, or promotional elements.

```text
┌───────────────────────────────────────────────────────────────┐
│ Quiz title                            Question 3 of 10   Timer │
├───────────────────────────────────────────────────────────────┤
│ Progress:  ● ● ● ○ ○ ○ ○ ○ ○ ○                                │
│                                                               │
│ Question 3                                                    │
│ Question text and optional image                              │
│                                                               │
│  ○ / □  Answer option A                                       │
│  ○ / □  Answer option B                                       │
│  ○ / □  Answer option C                                       │
│                                                               │
├───────────────────────────────────────────────────────────────┤
│ [Previous]                              [Save & next]          │
└───────────────────────────────────────────────────────────────┘
```

- Show question position (for example, “Question 3 of 10”) and a compact progress indicator.
- Use radio controls for single-choice and checkboxes for multiple-choice. Each option's entire row should be tappable, with a visible selected state.
- Make the timer prominent only when a timer is enabled. If no timer is configured, do not show a fake countdown.
- Keep previous/next controls in a stable location. Ensure long question text or images do not push the primary navigation below an unreasonable distance on mobile.
- Save answers through the server flow; display a subtle saving/saved/error state without interrupting typing or selection unnecessarily.
- Focus-loss detection warnings should be direct and unambiguous: explain that the quiz detected the page losing focus and that the incident is recorded. Do not use shaming language or claim that every focus change proves deliberate cheating.
- A detected focus-loss event may trigger the required immediate warning modal. The modal must have a clear acknowledgement action and must not reveal internal detection implementation details.
- The browser timer and displayed option order are not authoritative; server state defines remaining time and stored shuffle order.
- Provide an explicit **Review answers** action before final submission.

### 6.10 Review and submit

**Purpose:** Give students a final opportunity to check their work before submitting.

- Show an ordered question list with answered/unanswered status and a link to return to each question.
- Do not expose correct answers during review unless the product rules explicitly permit it; in this MVP, correct-answer visibility is controlled by the quiz setting and should usually be evaluated after submission.
- Make **Submit quiz** the clear primary action and require a confirmation step if submission is irreversible.
- State the consequence of submitting in plain language. Avoid alarmist copy.
- Disable repeat clicks while a submission is processing. If submission outcome is uncertain because of a disconnect, query/reconcile with the server before asking the student to submit again; do not create duplicate attempts by blindly retrying.

### 6.11 Thank-you card and result visibility

**Purpose:** Confirm that the attempt was submitted and communicate only the result information allowed by the teacher.

- Always make submission confirmation clear when the server confirms it.
- If score visibility is enabled, show the temporary score only according to the PRD's rule; make it clear that essays may still be awaiting grading.
- If the teacher hides scores, do not show score placeholders, correct-answer cues, or metadata that leaks the score indirectly.
- If correct-answer visibility is enabled, show answers only when the relevant visibility rule permits them.
- Keep the thank-you card compact and calm, with the next available action clearly named.
- Do not promise a final score time or status if the server has not confirmed grading completion.

### 6.12 Live monitor — normal and matching modes

**Purpose:** Let the teacher understand attempt progress and handle session controls without confusing two different quiz modes.

**Normal quiz monitor:**

- Focus on attempts and their current states, submitted status, timing, and recorded focus-loss count.
- Provide a clear route from a participant row to that attempt's detail/recap.
- Do not imply that the teacher's start/pause controls broadcast to all normal attempts unless that behavior is explicitly specified elsewhere.

**Matching quiz monitor:**

- Put session state and session controls at the top: **Start**, **Pause**, **Resume** if supported by the state machine, **Stop**, and **Finish** as appropriate to the current state.
- Make destructive or terminal actions visually distinct and require confirmation when the action ends a session or prevents further answers.
- Show participant/attempt progress and focus-loss counts in a scannable table or structured list.
- Display connection/realtime freshness information when useful, but never make an unsourced claim that the live data is current if the event stream has disconnected.
- Use SSE updates to refresh visible session state; on stream failure, show a non-blocking connection indicator and follow the polling fallback defined in the resilience specification.
- After reconnecting, fetch the current server state before presenting the session as live/paused/finished.

### 6.13 Essay grading queue

**Purpose:** Help teachers assign consistent scores to essay answers.

- Start with a queue summary (for example, essays awaiting grading) and a filter only if it provides real value.
- Use a list/table that identifies the quiz, participant, question, and grading state. Keep student/attempt identifiers visible enough to prevent grading the wrong submission.
- The grading detail should show the question text, the student's response, and a score control bounded from **0 to 10**.
- Include concise guidance that each essay's 0–10 grade is scaled to 0–1 point in the deterministic total score. The application should follow the scoring rules; the UI must not ask the teacher to calculate the scaled point manually.
- Provide a clear **Save grade** action, validation for out-of-range values, and an explicit saved/failed state.
- Distinguish **Awaiting grading** from **Graded**. Explain that the attempt's final score is available only after all required essays are graded, according to the business rules.
- Avoid auto-grading suggestions or plagiarism/proctoring panels; those features are out of scope.

### 6.14 Recaps and attempt detail

**Purpose:** Replace manual score recaps with a reliable view of attempts and their evidence.

- The recap list should make quiz, participant/name, score status, submission/attempt timing, and focus-loss count easy to scan.
- Include anonymous attempts with their entered-name label and do not represent them as registered user accounts.
- Attempt detail contains the score, recorded answers, relevant timing, focus-loss count, and essay-grading state required by the PRD.
- Distinguish temporary score from final score. Show labels, not just different colors.
- Do not add CSV/PDF export, print actions, or unrequested charting to the MVP interface.
- If score formulas or analytics definitions remain unresolved by DEC-004, label ambiguous metrics clearly or leave them out until the rule is decided. Do not silently invent a formula.

### 6.15 Student dashboard and personal analytics

**Purpose:** Help students find available quizzes and understand their own history.

- Lead with **Available quizzes** and relevant quiz cards, not teacher-wide school metrics.
- Present **My results** as a separate section or destination.
- Provide exactly the defined filter options: **3 days**, **7 days**, **30 days**, and **All time**. Clearly indicate the active filter and ensure changing it refreshes the metrics/results.
- Limit analytics to the PRD-defined personal scope. Do not reveal other students' scores or introduce unapproved comparisons, leaderboards, or rankings.
- Empty state text should explain when no attempts exist within the selected time range and offer a relevant route back to available quizzes.

## 7. Reusable Component Specifications

### 7.1 Buttons

| Variant | Appearance | Use |
|---|---|---|
| Primary | `brand-900` fill, white text | The single highest-priority action for the current view. |
| Secondary | White/surface fill, `brand-900` text, neutral border | Important but non-primary actions. |
| Tertiary / text | No heavy container, readable brand text | Low-priority navigation or inline actions. |
| Destructive | Semantic error fill or outlined treatment | Delete, stop/finish where terminal, and other consequential actions. |
| Disabled | Muted fill/text plus non-interactive behavior | Only when the action truly cannot be used; include a nearby reason when unclear. |

Buttons use explicit action labels: **Create quiz**, **Save draft**, **Open quiz**, **Save question**, **Review answers**, **Submit quiz**, **Save grade**. Avoid vague labels such as “Submit” or “OK” where a more precise label is possible.

### 7.2 Cards

- White surface, 1 px neutral border, 14–16 px radius, 16–20 px padding.
- Content hierarchy: title → state/metadata → relevant summary → action.
- Keep the card's title and primary action visible without requiring hover.
- Do not make important status information available only on hover.
- Avoid placing several unrelated actions side by side. Use a compact overflow menu for low-frequency actions.

### 7.3 Status badges

Use short text labels and, where helpful, a small icon. Color is supplemental and never the only signal. Examples include **Draft**, **Open**, **Closed**, **Waiting**, **Live**, **Paused**, **Submitted**, **Awaiting grading**, **Final**.

Do not overload one state label to represent multiple independent flags. **Show / Hide** and **Open / Closed** are different controls and require different labels.

### 7.4 Inputs and validation

- Persistent visible labels; placeholders are examples, not labels.
- Minimum comfortable control height: 44 px for primary touch targets; text fields can be taller when multiline.
- Use field-level validation and a concise form-level summary when multiple errors prevent saving.
- On errors, preserve entered values wherever possible and focus/announce the first invalid field.
- For numeric grade and timer controls, display allowed ranges and reject invalid values with a specific message.
- Do not rely on browser-native validation alone for critical business rules; server validation remains authoritative.

### 7.5 Tables and structured lists

Use tables for teacher recaps, student account management, and dense monitoring data on large screens. On mobile, convert each row to a stacked record with named fields or an intentionally designed horizontal overflow region; never crop the participant name, score status, or focus-loss count without an alternate way to inspect it.

Use stable column labels and do not rely on sort icons without accessible labels. Where sorting is not implemented, do not imply that a column is sortable.

### 7.6 Dialogs and modals

Reserve dialogs for focused decisions: confirmation of irreversible actions, the required focus-loss warning, and compact success/error acknowledgements only when inline feedback is insufficient.

- Include a descriptive title, specific explanation, and action labels that state consequences.
- Keep keyboard focus inside modal dialogs while open and return focus to the triggering control when closed.
- Do not use a modal for ordinary status information that can be displayed inline.
- For a focus-loss warning, keep language factual and calm; acknowledge the incident and explain that it has been recorded.

### 7.7 Loading, empty, error, and offline/disconnect states

Every data-driven screen must define these states:

- **Loading:** preserve layout shape where practical; use a short status label or skeleton, not a blank page.
- **Empty:** explain what is absent and provide one relevant next action where available.
- **Filtered empty:** explain that no items match the current filters and make clearing filters easy.
- **Recoverable error:** identify what failed and provide a retry or correction path.
- **Permission/state conflict:** explain why the requested action is unavailable, then reflect the latest server state.
- **Connection lost:** indicate that live updates or saving may be delayed; do not claim an answer, grade, or state transition was saved until the server confirms it.
- **Reconnect:** reconcile against server state before restoring controls or declaring a quiz/session active.

## 8. Interaction and State Rules

### 8.1 Server-authoritative behavior

The UI must treat the server as the authority for time, score calculation, shuffle order, attempt state, session transitions, and persisted focus-loss counts. Browser-side state is for interaction and display, not for deciding the final result.

### 8.2 Quiz visibility and availability

Represent these as separate concepts in both data presentation and controls:

- **Show / Hide** controls whether a quiz appears on the student dashboard.
- **Open / Closed** controls whether new registrations/starts are accepted.

A quiz can be visible but closed. The UI must not imply that visibility means a student can start it.

### 8.3 Attempt lifecycle

The normal quiz path should communicate the supported journey without adding hidden steps:

`Discover → Register → Start → Answer → Review → Submit → Thank-you / permitted result`

Repeated attempts create distinct attempt records as defined in the PRD. The UI should not imply that an old attempt will be overwritten. State and retry behavior must be reconciled with the server, particularly after a disconnect around submission.

### 8.4 Matching-session lifecycle

The waiting room, active quiz view, and terminal state must visually reflect the server's state machine. Controls shown to the teacher depend on the current state. Do not leave stale actions visible after a session transition. When SSE events are unavailable, show degraded live status and use the resilience fallback defined elsewhere in the specs.

### 8.5 Essay grading and score visibility

- Objective questions are auto-graded according to the defined deterministic rules.
- Essays are manually graded 0–10, scaled to 0–1 point each.
- A temporary score excludes ungraded essays; the final score follows the specified completion rules.
- Results and correct answers must follow the configured visibility flags on both desktop and mobile, including direct URLs and refreshed pages.

### 8.6 Focus-loss warning

Use an immediate, prominent but calm warning when the product detects a focus-loss event. Record/aggregate incident counts on the server as specified, and display the count to teachers in the appropriate monitor/recap views. The UI should report what was detected, not assert intent or guilt.

## 9. Responsive Behavior

### Wide desktop — approximately 1200 px and above

- Show full teacher navigation and Daily Pulse rail.
- Use up to three columns for quiz cards when each card has enough space for title, metadata, status, and actions.
- Keep metric cards in one row when widths permit.
- Use tables for dense recaps and live monitoring.

### Tablet / compact desktop — approximately 768–1199 px

- Reduce the workspace rail width or collapse it to icon-plus-label navigation depending on available width.
- Reduce card grids to two columns.
- Allow metric cards to wrap without compressing values and labels.
- Keep the primary action visible near the page title.

### Mobile — below approximately 768 px

- Use a compact header and a navigation pattern that keeps destinations accessible without consuming excessive vertical space.
- Stack cards and metric summaries; use short labels and preserve adequate touch targets.
- Move Daily Pulse beneath navigation or into a compact expandable region.
- Convert management tables to labelled stacked rows or a carefully bounded overflow table.
- Keep quiz-taking mode distraction-free with stable previous/next controls and readable question content.
- Ensure review, submission confirmation, focus-loss warning, and essay grading are fully operable without hover.
- Avoid fixed bottom controls that cover answer content or the mobile browser's safe area.

Breakpoints are implementation guidance, not a requirement to match one device exactly. Components should respond to available space and content, not only named device categories.

## 10. Accessibility and Usability Requirements

- Meet WCAG 2.2 AA contrast expectations for text and essential interface components. Validate actual contrast for the palette combinations used; do not assume that every palette color works with white text.
- Every interactive control is keyboard-operable and has a visible focus indicator. Prefer a 2 px focus outline with sufficient separation from the component border.
- Give icon-only controls accessible names. Do not use an icon as the only means of communicating quiz state.
- Associate inputs, validation messages, and helper text programmatically with their fields.
- Do not communicate selected answers, correctness, focus-loss incidents, or session state with color alone.
- Respect reduced-motion preferences. Transitions should be brief and functional; do not animate during timed assessments in ways that distract or delay action.
- Keep touch targets comfortable on phones, with 44 × 44 px as the preferred target size for primary controls.
- Announce important asynchronous updates accessibly: saved status, submission confirmation, connection loss/recovery, new session state, and validation errors. Avoid continuously announcing a rapidly changing countdown every second.
- Make images attached to questions readable and zoomable when necessary, with suitable alternative text support if the product's image workflow provides it. Do not invent descriptive alt text for an image when the author has not supplied meaningful information.

## 11. Motion and Feedback

Use restrained, purposeful motion only:

- Short transitions for opening a menu, switching a tab, or revealing a validation message.
- Subtle state change when a quiz transitions between Draft/Open/Closed or a matching session changes state.
- Clear progress feedback for question-image upload and save/submit actions.
- No ambient floating blobs, parallax, decorative count-up effects, or motion that competes with timers and answer entry.

Honor reduced motion. In assessment flows, reliability and clarity take priority over visual animation.

## 12. Content and Microcopy

Write interface copy in concise international English. Preserve and define Indonesian domain terms only when they appear in the product language or glossary; do not mix languages arbitrarily across controls.

Use active, concrete action labels. Examples:

| Prefer | Avoid |
|---|---|
| `Create quiz` | `New` |
| `Save draft` | `Submit` |
| `Open quiz` | `Activate` when the user expects student access |
| `Review answers` | `Review` without context |
| `Submit quiz` | `Finish` when submission is the actual action |
| `Save grade` | `Confirm` |
| `No quizzes match these filters` | `No data` |
| `Connection lost. Reconnecting…` | `Something went wrong` |
| `This quiz is closed to new attempts` | `Unavailable` |

Errors should state what failed and how to recover. Empty states should guide the next action without adding marketing language. Success messages should use the same verb as the action, for example **Grade saved** after **Save grade**.

## 13. Do Not Add in This Design

The following are outside the stated MVP and must not appear as implied capabilities:

- Multi-school or per-class teacher tenancy.
- Reusable cross-quiz question bank or question version history.
- Rich-text/LaTeX/audio/video questions or student image uploads.
- SSO/OAuth, OTP, email verification, or unapproved password policy rules.
- CSV/PDF export, print workflows, or unrequested complex charts.
- Native mobile applications, offline mode, push notifications, or notification center features.
- Automated essay scoring, plagiarism detection, or proctoring beyond focus-loss detection.
- Leaderboards, public rankings, or analytics that expose students' information to other students.
- Invented metric formulas or thresholds while DEC-001, DEC-002, DEC-003, or DEC-004 remains unresolved.

## 14. Open Decisions and Design Dependencies

Design behavior must remain aligned with the PRD's unresolved decisions:

- **DEC-001 — Success thresholds:** Do not turn the proposed 15-minute authoring/instant recap measures into visible promises until accepted.
- **DEC-002 — CSV/PDF export:** Keep export and print actions out of MVP screens unless scope changes.
- **DEC-003 — Password length and session lifetime:** Show rules only after values are decided; avoid inventing authentication policy copy.
- **DEC-004 — Analytics formulas:** Define metric labels/tooltips only after formulas are agreed. Until then, do not create misleading totals or trend indicators.

The frontend must also follow the canonical feature flows, business rules, state machines, RBAC matrix, resilience documentation, architecture overview, and test cases referenced by the PRD. If those specifications disagree with this design document, surface the mismatch rather than silently creating a new behavior.

## 15. Implementation Notes for SSR

- Render useful page structure and core content with server-side rendering. Do not make essential navigation or question content depend on client-only rendering.
- Use progressive enhancement for interactive controls where practical, while respecting the application's Go monolith and chosen templating approach.
- Treat SSE as the delivery mechanism for matching-session updates, not as the source of truth. Reconcile state from the server when the connection drops or the page reloads.
- Use POST for state-changing actions according to the application routing/security rules. Show a clear pending state while waiting for server confirmation.
- Avoid optimistic UI for irreversible operations unless there is a reliable reconciliation strategy. In particular, quiz submission, session stop/finish, and grade saves must distinguish pending from confirmed success.
- Ensure direct route access, refresh, and browser back/forward behavior do not bypass authorization or quiz visibility rules.
- Keep all teacher-only controls and data behind server-side authorization. Hiding a control in the template is not an authorization check.

## 16. Design Acceptance Checklist

Before considering a screen design complete, verify:

- [ ] The user can identify the screen's purpose and primary action quickly.
- [ ] The four provided brand colors are used according to the palette rules, with tested contrast and restrained use of pale yellow.
- [ ] Desktop layout follows the compact-header / left-rail / main-content direction where appropriate, without forcing dashboard chrome onto the quiz-taking flow.
- [ ] Quiz cards expose the right state without conflating Show/Hide with Open/Closed.
- [ ] Student and anonymous views do not expose teacher-only navigation or analytics.
- [ ] Question type rules and optional-image behavior are clear in the editor.
- [ ] One-question-per-page answering, review, submission, and visibility-controlled results work on mobile.
- [ ] Matching session controls accurately reflect the server state, with connection-loss and reconnect states defined.
- [ ] Focus-loss warnings are immediate, factual, accessible, and non-accusatory.
- [ ] Essay scoring accepts 0–10 and explains the score scaling without manual arithmetic.
- [ ] Recaps clearly distinguish temporary/final score, anonymous attempts, timing, recorded answers, and focus-loss counts.
- [ ] Loading, empty, filtered-empty, validation, permission-conflict, failed-save, connection-loss, and reconnect states have been designed.
- [ ] Keyboard focus, form labels, screen-reader announcements, touch targets, contrast, and reduced motion have been considered.
- [ ] No out-of-scope feature or unresolved metric formula has been presented as decided product behavior.

---

**Design north star:** a calm blue assessment workspace that helps teachers manage the whole quiz lifecycle and lets students focus on one question at a time—while making states, scores, and server-confirmed outcomes unmistakable.
