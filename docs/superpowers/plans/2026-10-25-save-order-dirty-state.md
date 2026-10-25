# Show “Simpan urutan” only when the reorder is unsaved

**Date:** 2026-10-25
**Status:** proposed
**Surface:** teacher quiz detail, question reorder footer

## Problem Statement

On the teacher quiz detail screen (`/teacher/quizzes/:id`, for example quiz 2),
the **Simpan urutan** button is rendered whenever the quiz is an editable draft
with at least one question. It is visible before the teacher changes anything,
and it stays visible after the teacher manually restores the original order.
Pressing it without a change still posts the form; the server treats that as an
idempotent no-op but redirects with the “Urutan soal diperbarui.” success flash,
so the interface reports a save that did not happen.

The reorder is already a client-side, pre-save state: dragging a row or pressing
the up/down controls changes the DOM and rewrites the hidden `order` input, but
nothing is written until the teacher submits. The missing piece is a comparison
between the current order and the order the server rendered, plus a visible
state derived from that comparison.

### Current route and render conditions

- The detail page marks the question list editable only while the quiz is a
  draft, and renders the reorder form only when at least one live question
  exists. Open and closed quizzes show the question list read-only and replace
  the settings form with read-only values; no reorder form is rendered, so the
  button is not rendered there.
- The reorder POST (`/teacher/quizzes/:id/questions/reorder`) is owner-teacher
  only, requires CSRF, re-checks draft state under the quiz lock, and requires
  the submitted `order` to be the exact live question-ID set. A different
  sequence rewrites positions 1..N; an identical sequence commits no write but
  still returns the same success flash.
- The button sits inside the reorder form. The per-question delete forms sit
  outside it and are associated with their row buttons through the `form`
  attribute; that structure must not change.

### Similar pages audited

- The student profile and the teacher profile in the current tree render one
  always-visible “Simpan perubahan”. Their inputs naturally hold edits in the
  browser until submit, as any form does, but they do not derive a dirty state
  or conditionally reveal the save control.
- The quiz settings card renders an always-visible “Simpan pengaturan”; the
  question editor, quiz creation, student edit, anonymous edit, majors, and
  grading screens likewise render their save control whenever their form is
  present, without a dirty-state condition.
- There is no shared dirty-form helper in the application JavaScript. The only
  existing unsaved-change guard is the `beforeunload` warning on the student
  attempt form.
- There is therefore no existing pattern to copy. This refactor introduces a
  new, deliberately narrow pattern for the reorder control only; it does not
  retrofit profile or any other form.

### Why a leave guard

Without a leave guard, a teacher who reorders questions and then follows any
other link silently loses the new order. The same page also carries settings,
publish, close, delete, and sign-out forms, so a dirty reorder can be lost
through many different navigations.

## Solution

Keep the save-order control in the markup but render it hidden on first paint.
Capture the server-rendered question-ID sequence when the reorder block starts.
On the initial sync, every drag end, and every up/down click, compute the
current sequence and compare it with the captured sequence:

- different sequence → reveal **Simpan urutan**;
- identical sequence → hide it again.

Arm one page-level `beforeunload` guard when the reorder block initialises; it
acts only while the sequence differs, so leaving the page asks for confirmation.
Clear it when the reorder form itself is submitted. Keep the existing
explicit-save flow: no autosave, no new transport, no server change.

Update the builder footer copy so the conditional button is explained rather
than merely missing: the save-order button appears after the order changes, and
reordering still requires JavaScript.

Add a template-contract test that pins the initial hidden state, the JavaScript
hook, the placement inside the reorder form, the explanatory copy, and the
absence of the control on locked and empty pages. Verify the interactive
behaviour manually in a browser, because this repository has no JavaScript test
runner and its CI only runs Go.

## Commits

### Commit 1 — `feat(teacher): reveal save-order only when the order changes`

This is the core slice and must land as one commit: hiding the button without
the JavaScript that can reveal it would remove the only way to save a reorder.

Template work:

1. In the reorder form footer, keep the existing secondary submit button, icon,
   label, and position. Add the `data-save-order` hook to the button and render
   it with the `hidden` attribute so it is not visible on first paint.
2. Keep the button inside the reorder form; do not move it to the page header or
   the settings card, and do not turn it into a link or a JavaScript-only click
   target.
3. Leave the per-question delete forms and their `form` associations exactly as
   they are; they must stay outside the reorder form.
4. Replace the footer help sentence with the exact copy recorded under
   **Behavioural decisions → Copy**. It must retain the note that reordering
   requires JavaScript. Do not promise an autosave.
5. Do not change the empty, locked, or no-question branches: the button is not
   rendered there today and must remain absent.
6. Do not add CSS for the hidden state. Bootstrap’s reboot already declares
   `[hidden] { display: none !important }`, so the attribute is sufficient.

Application JavaScript work, inside the existing drag-reorder block only:

1. Resolve the reorder form with `sortable.closest("form")`, not a global form
   lookup, find the save button through `data-save-order` inside that form, and
   keep the existing `#question-order` hidden input lookup. Null-guard the form,
   button, and input; the block must stay inert on pages that do not carry them.
   This page has one reorder list; do not generalise the selector to multiple
   lists.
2. Before the first order sync, capture the server-rendered sequence of question
   IDs from each `[data-sortable-item]`’s `data-id`, in DOM order. This is the
   clean baseline, not the hidden input’s value, because the hidden input is
   written by this same JavaScript.
3. Refactor the existing sync routine so it computes the current ID sequence
   once, writes that sequence to the hidden order input, and then performs the
   existing index-label and up/down-disabled updates.
4. Compare the current sequence with the baseline. Set the save button’s
   `hidden` property to the inverse of “changed”. Do not use a CSS class or an
   inline style for this state.
5. Keep the initial sync call: it populates the hidden input on load and
   enforces the clean state, even if the server-rendered `hidden` attribute is
   accidentally removed.
6. Preserve the existing behaviour for a single question: no move is possible,
   the sequence never changes, and the button stays hidden.
7. Do not change the hidden input’s format or the POST contract: it remains a
   comma-separated list of the full live question set.

Verification for this commit:

- Load a draft quiz with at least three questions; **Simpan urutan** is not
  visible.
- Drag a middle row; the button appears.
- Move the same row back with the up/down controls; the button disappears.
- Repeat with keyboard-activated up/down controls; the same visibility rule
  holds and focus stays in the moved row, on its remaining enabled move button,
  as it does today.
- Check the browser console for errors.
- Disable JavaScript; the save-order button stays hidden. The move buttons and
  drag handles remain inert exactly as they are today; this refactor adds no new
  dead control.
- Before the leave guard lands, confirm the settings, publish, close, and delete
  forms still render and submit exactly as before.
- Confirm there is no UI path to submit a clean order, so the false success
  flash is no longer reachable from the reorder footer.

### Commit 2 — `test(teacher): pin the hidden save-order default`

Add one table-driven template-contract test to the teacher quiz drill-down
suite. Render the real quiz-detail template in four cases: an editable draft
with two questions, an open quiz with questions, a closed quiz with questions,
and a draft with no questions.

Assertions for the editable draft with questions:

1. The reorder form is present and contains exactly one `data-save-order` hook.
2. The hooked button sits inside that form, not in the settings form or the
   Status kuis card.
3. The element that carries `data-save-order` is the save button itself and
   also carries the `hidden` attribute on first render. Parse the rendered
   markup with `golang.org/x/net/html` and assert on the parsed node, so a hook
   on a hidden descendant cannot satisfy the requirement and `hidden` is a real
   attribute rather than a substring of another attribute such as
   `aria-hidden`.
4. The footer copy contains `muncul setelah urutan berubah` and
   `memerlukan JavaScript`. Assert those clauses, not incidental punctuation or
   line wrapping.
5. The button remains a secondary outline submit control labelled “Simpan
   urutan”; it is not promoted to the solid primary style.
6. The settings save button remains visible and does not carry the reorder hook,
   so the change has not leaked to another form.

Assertions for the non-editable and empty cases:

7. No reorder form, no `data-save-order` hook, and no “Simpan urutan” button is
   rendered.

Do not assert internal variable names or the implementation of the comparison.
The test should fail if the reorder button is visible on first paint, if it
loses its hook, if it moves outside the form, if the explanatory copy is
removed, or if the non-editable and empty branches start rendering the control.
The test pins the server-rendered attribute; the manual browser pass is what
confirms the vendored Bootstrap rule actually computes to `display: none`.

Verification:

- Run the focused teacher-detail tests, including
  `TestQuizDetailSaveOrderVisibilityContract` and `TestQuizDetailIsTheOnlyHub`.
- Run the touched-file formatting check, the full Go suite, vet, and build.
- Confirm the template test passes in the repository’s normal no-database path.

### Commit 3 — `feat(teacher): guard against losing an unsaved order`

Extend the same drag-reorder block with a derived dirty state; do not introduce
a second source of truth.

1. Set the dirty flag from the same sequence comparison used for visibility.
2. Add one `beforeunload` listener inside the guarded sortable block. When
   dirty, call `preventDefault()` and set the legacy `returnValue` to the empty
   string, matching the existing attempt-form guard. The listener must not be
   registered when the sortable block is absent.
3. Add a submit listener on the reorder form that clears the dirty flag before
   navigation. Listen for `submit`, not a button click, so any submission that
   fires the event, including keyboard and `requestSubmit()`, clears the flag.
   The page has no bare `form.submit()` call.
4. Do not clear the flag from the settings, publish, close, delete, or sign-out
   forms. Those navigations should warn while the order is unsaved.
5. Keep the listener registration once per page load; do not add a new listener
   on every drag.
6. Clear the flag when the teacher restores the original order, so reverting and
   then leaving does not warn.
7. Do not add a `data-confirm` attribute or a second confirmation dialog to the
   reorder form; the browser guard is the only leave prompt, and saving the
   order should not ask again.

Verification for this commit:

- Reorder a question, then activate the back link; the browser asks before
  leaving. Cancelling keeps the page and the reordered rows intact.
- Repeat with a full page reload; the same prompt appears.
- Reorder, restore the original order, then activate the back link; no prompt.
- Reorder, press **Simpan urutan**; no prompt, the request completes, the
  success flash appears, and the reloaded page shows the button hidden.
- Reorder, then submit valid settings; the leave prompt appears before the
  settings request. Cancelling leaves the page and the reordered rows intact.
- Reorder, then publish, close, or delete the quiz, or delete a question: the
  shared confirmation dialog appears first, then the browser’s leave prompt
  after accepting. Reorder, then sign out: the browser’s leave prompt appears
  directly. That double confirmation for the destructive actions is accepted.
- Run the touched-file formatting check and the full Go suite, vet, and build.
- Re-run the local JavaScript syntax check.

## Decision Document

### Modules changed

- **Teacher quiz detail template** — only the reorder form footer. The button
  keeps its secondary style, label, icon, and placement; it gains a hidden
  initial state and one data hook. The footer copy changes. The per-question
  delete forms and their `form` associations are untouched.
- **Shared application behaviour module** — only the existing drag-reorder
  block. No new module, no new runtime dependency, no network API.
- **Teacher quiz drill-down template test suite** — one new rendered-output
  test. It parses the rendered DOM with `golang.org/x/net/html`, which was
  already in the module graph as an indirect dependency and is now a direct
  test-only requirement.

### Interfaces

- One new DOM contract: the submit button carries `data-save-order`.
- No new form field, query parameter, route, view-model field, or response
  header.
- The hidden `order` input and the comma-separated ID list remain the POST
  contract.
- The reorder form is located from the sortable list through `closest("form")`,
  not through a new form-level hook.

### Behavioural decisions

- **Clean baseline:** the server-rendered DOM order at page load. This is the
  order the teacher sees and the order the server currently stores.
- **Dirty definition:** the current full ID sequence differs from the baseline.
  Reordering and then restoring the original sequence is clean again.
- **Visibility:** the button is hidden on first paint and whenever clean. It is
  revealed by JavaScript on the first actual difference. Because the `hidden`
  attribute is server-rendered, there is no flash of the button before the
  script runs. It is hidden rather than disabled because the intended treatment
  is a control that is not shown until it can do something; a disabled button
  would still occupy the footer and imply that a pending action exists.
- **No autosave:** the teacher still chooses when to commit the new order. The
  existing POST, redirect, flash, lock, exact-set validation, and idempotent
  no-op behaviour are untouched.
- **Leave guard scope:** page-scoped. Any navigation while the order is dirty
  warns, including navigation initiated by other forms. Only the reorder form’s
  own submit clears the flag. The browser controls the prompt’s wording and may
  suppress it in some situations; this matches the existing attempt guard rather
  than promising a guaranteed dialog.
- **No-JS behaviour:** the button remains hidden. The page already states that
  reordering requires JavaScript, and the hidden input cannot be populated
  without it, so there is no usable no-JS path to preserve.
- **Layout:** revealing the button may reflow the footer at narrow widths
  because it uses `flex-wrap`. That is accepted; no blank placeholder space is
  reserved, because reserving it would keep a phantom control in the layout.
- **Focus:** no focus is moved when the button appears. It sits after the list
  in DOM order, so keyboard users reach it after the move controls without
  losing their place.
- **Single list:** this page has one reorder list and one save-order control.
  The change does not attempt to support multiple sortable lists.
- **No new CSS:** the `hidden` attribute is enough because Bootstrap’s reboot
  declares `[hidden] { display: none !important }`. No stylesheet change is part
  of this refactor.
- **Button hierarchy:** unchanged. The reorder save remains a secondary outline
  button; the screen’s primary action stays where it is.
- **Submit feedback:** no spinner and no disabled state are added. The browser’s
  native navigation remains the only feedback, exactly as today.
- **Copy:** the footer explains the conditional appearance and retains the
  JavaScript requirement. Exact copy: “Seret baris atau gunakan tombol
  naik/turun. Tombol Simpan urutan muncul setelah urutan berubah. Pengurutan
  memerlukan JavaScript.” No toast or permanent “saved” marker is added.
- **Documentation impact:** no business-logic or API spec change is required;
  the POST contract and the server rules are untouched. If the team wants the
  UI rule recorded in the design guidance, add a one-line note in a separate
  docs change rather than mixing it into the code commits.
- **Error paths:** a stale or malformed reorder still redirects with the
  existing error flash. The reloaded editable page starts clean, so the button
  is hidden until the teacher changes the order again. A non-draft error
  reloads a locked page where the reorder form is absent entirely. Missing or
  non-owned quizzes remain 404s.
- **Drag cancellation:** if the DOM was already reordered during a drag and the
  drag ends without a drop, the sequence is still compared to the baseline.
  The button reflects the DOM state the teacher can see; this preserves the
  current drag semantics rather than inventing a rollback.

### Alternatives considered

- **Visible but disabled:** rejected in favour of hiding the control. A disabled
  button would still occupy the footer and imply that a pending action exists.
- **Autosave on every reorder:** rejected. It would need a non-navigating
  transport, a new failure/recovery model, and a change to the explicit-save
  contract; it is much larger than the requested visibility fix.
- **Generic dirty-form helper applied now:** rejected. Profile, settings, and
  the question editor have different input types, autofill behaviour, and
  validation-error paths; the request is scoped to the reorder route.
- **Apply the rule to every save form:** rejected. Hiding save buttons across
  the product would harm discoverability and is not part of this request.
- **No leave guard:** rejected. The reorder is held only in the DOM, so an
  accidental navigation would still discard it silently.

### Schema and API

- No schema change.
- No migration.
- No API contract change.
- No change to the server-side idempotent no-op; the UI simply stops offering
  the no-op submission.

## Testing Decisions

### What makes a good test here

The test should describe the page’s external contract: on first paint, the
teacher cannot see a save control for an order that has not changed. It should
not reach into JavaScript variables or assert a particular implementation of
the comparison. It should also protect the placement and copy that make the
control understandable, because those are the parts most likely to regress when
the footer is edited.

### Modules covered

- The teacher quiz detail template, through the existing direct-render test
  helper.
- The teacher drill-down test suite, which already pins the page’s action
  hierarchy.

### Prior art

- `TestQuizDetailIsTheOnlyHub` renders the same template and counts solid
  primaries plus the settings button’s exact class. The new save-order control
  is a hidden outline button, so those expectations remain valid.
- The profile-page tests render page templates directly to pin navigation and
  action contracts.
- The conformance suite scans templates and application JavaScript for banned
  patterns; it is not a behavioural test and is not the right place for this
  feature.

### Automated coverage added

One table-driven rendered-output test covering four cases: the editable draft
with questions (single hidden hook inside the reorder form, explanatory copy,
secondary styling, untouched settings save), an open quiz (no control), a
closed quiz (no control), and a draft with no questions (no control).

### Manual coverage required

Because there is no JavaScript test runner in the repository and CI only runs
Go, the toggle and the leave guard are verified manually in a browser with a
draft quiz containing at least three questions, so a middle row can be moved
past a non-adjacent neighbour:

1. Initial load: button hidden, list in server order, settings save still
   visible.
2. Drag a middle row to a non-adjacent position: button visible, hidden input
   order matches the DOM.
3. Drag back to the original position: button hidden, guard disarmed.
4. Keyboard up/down: same visibility rule; focus stays in the moved row, on its
   remaining enabled move button, as it does today.
5. Save while dirty: no leave prompt, success flash, button hidden after reload.
6. Back link while dirty: leave prompt; cancel preserves the reorder.
7. Reload while dirty: leave prompt.
8. Revert then leave: no prompt.
9. Valid settings submit while dirty: leave prompt; cancel preserves the order.
10. Publish, close, delete a question, delete the quiz, or sign out while
    dirty: the leave prompt appears, after the shared confirmation dialog where
    one exists.
11. No-JS load: the save-order button stays hidden; the pre-existing move and
    drag controls are inert, as before.

### Verification commands

- `gofmt -l internal/httpserver/teacher_hierarchy_pages_test.go` (must print
  nothing). The repo-wide `gofmt -l .` is the CI gate, but the working tree
  already contains unrelated edits, so do not attribute pre-existing output to
  this change.
- `go test ./internal/httpserver -run 'TestQuizDetailSaveOrderVisibilityContract|TestQuizDetailIsTheOnlyHub' -count=1`
- `go build ./...`
- `go vet ./...`
- `go test ./...`
- Local JavaScript syntax check with Node’s module input mode, as used before
  this plan. Pipe the application behaviour script into
  `node --input-type=module --check`. This is a developer check, not a CI gate.

### Coverage gap accepted

The dirty comparison and leave guard have no automated regression test. If this
conditional-save pattern spreads to other forms, that is the point to introduce
a browser test harness or a small JavaScript test setup. Adding Node or
Playwright for this single control would be disproportionate to the change.

## Out of Scope

- Changing the student profile, teacher profile, quiz settings, question editor,
  grading form, or any other always-visible save button.
- Building a generic dirty-form helper or a shared conditional-save component.
- Autosaving reorder changes or introducing a non-navigating transport.
- Any server-side change: route, authorization, locking, exact-set validation,
  idempotency, flash keys, or redirects.
- A no-JavaScript reorder fallback, such as numbered position inputs.
- Visual redesign, button-tier changes, a status line, a toast, or a submit
  spinner. The leave guard is the only supporting behaviour in scope.
- Adding `data-confirm` or a second confirmation dialog to the reorder form.
- Reserving footer space with CSS to eliminate the reveal reflow.
- Changing the no-JavaScript banner.
- Adding Node, Playwright, or another test framework, and changing CI.
- Fixing the profile pages’ unconditional save buttons.
- Reworking the delete, publish, close, or sign-out confirmation dialogs
  themselves; the added browser leave prompt is in scope, the dialogs are not.
- Data migration or cleanup of existing question positions.

## Further Notes

- The working tree already contains extensive uncommitted backend, template,
  and design changes, and the drill-down test file is itself untracked. This
  refactor changes three application files, but file-level staging will not
  isolate it: the template and application script already carry unrelated edits,
  and appending a test to the untracked suite carries the whole suite. Use
  hunk-level staging, or land the change after the current working set is
  committed. If a clean standalone commit matters more than suite proximity,
  put the new test in its own new test file and use that path in the formatting
  command. Until that file is committed, CI will not run the new test, so local
  `go test` is the only automated check.
- No business-logic or frontend data-contract spec change is required: the
  reorder route, payload, validation, locking, and flash keys stay identical.
- The repository currently has no git remote and no GitHub CLI available, so
  this plan could not be published as a GitHub issue. This document is the
  issue-ready body.
- The profile page is not prior art for this behaviour. If a reviewer asks for
  consistency, the honest answer is that this is a new pattern scoped to the
  reorder control; profile and settings remain explicit-save forms with
  permanently visible buttons.
- The server’s “same order is a no-op” rule is correct and should stay. Hiding
  the button removes the misleading success flash without weakening that rule.
- If accessibility review later asks for an announcement when the button
  appears, add a polite status region in a separate change. It is intentionally
  not bundled here.
- If product later wants the same rule on the settings form, treat it as a
  separate refactor: settings has multiple input types, autofill, validation
  errors, and a different save affordance, so the reorder comparison does not
  generalise cleanly.
