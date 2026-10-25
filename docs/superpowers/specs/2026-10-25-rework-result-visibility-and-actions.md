# Rework — result visibility, teacher action hierarchy, student history

Status: decided 2026-10-25. This file is the single contract for the parallel
implementation waves. Where it conflicts with an older spec, this file wins and
the older spec must be corrected in the same change.

Repo: `D:\Project\Mother\quiz-v2`. Build: `go build ./...`, `go test ./...`.

## 0. Invariants

- **The route count is unchanged by this feature.** No new route path. `conformance_test.go`
  pins the surface; a scoped grading queue reuses `GET /teacher/grading?quiz=<id>`,
  which the domain already parses (`gradingQueueInput`, `routes_teacher_grading.go:24-32`).
- **Palette**: only the 24 hexes in `docs/DESIGN.md` §2.1. No new hex, ever.
- **Classes**: only the existing vocabulary. New component classes go in
  `templates/partials/ui-style.html` or the page's own `{{define "partials/page-style"}}`.
- **Banned**: `accordion-button`, `accordion-collapse`, `dropdown-*`,
  `data-bs-toggle="modal|dropdown|offcanvas"`, `window.bootstrap`,
  `window.confirm/alert/prompt`, `btn-primary`, `form-select`, `text-muted`,
  `outline: none`, icon fonts, CDN links.
- **Bootstrap 6 accordion is native `<details>`/`<summary>`.** Use that.
- **Button tiers** (DESIGN §7): primary `btn btn-solid theme-primary`,
  secondary `btn btn-outline theme-primary`, destructive
  `btn btn-outline theme-danger`. Row actions add `btn-sm`. `btn-text` is
  **not** a tier and must not be used for an action on these screens.
- **One primary button per region.** A link never submits; a button never navigates.
- **Back is navigation**: one `link-quiet` + 16px `arrow-left`, first in reading
  order, labelled with its destination. A header action pointing at the back
  link's URL is removed.
- **Copy is Indonesian, sentence case.** "Kembali ke …", "Ubah", "Simpan", "Batal", "Hapus".

## 1. Result visibility (student side)

### 1.1 Replace the enum, all four values

`viewmodel.ReviewMode` (`internal/viewmodel/enums.go:194-233`) becomes:

| Constant | Value | Label (ID) | Meaning |
|---|---|---|---|
| `ReviewQuestionsNoVerdict` | `questions_no_verdict` | Soal dan jawaban siswa | Questions + the student's own answers. No correct/incorrect mark, no key, no essay feedback, no essay score. |
| `ReviewQuestionsNoAnswers` | `questions_no_answers` | Hanya soal | Question bodies and images only. No answer box, no key, no verdict, no essay text. |
| `ReviewCorrectOnly` | `correct_only` | Hanya soal yang benar | Only questions the student answered correctly. Question + their (necessarily correct) answer. No key, no verdict badge, no colour wash. Essays excluded. |
| `ReviewQuestionsWithKey` | `questions_with_key` | Soal dan kunci jawaban | Full record: question, student answer, key, verdict badge + colour, essay feedback, essay score. Current `questions_and_key` behaviour. |

All four values are ≤20 chars, so `quizzes.review_mode VARCHAR(20)` keeps its width.

Capability predicates replace `ShowsQuestions`/`ShowsKey`/`ShowsVerdict`:

```go
func (m ReviewMode) ShowsQuestion() bool       // all four
func (m ReviewMode) ShowsStudentAnswer() bool  // no_verdict, correct_only, with_key
func (m ReviewMode) ShowsKey() bool            // with_key only
func (m ReviewMode) ShowsVerdict() bool        // with_key only
func (m ReviewMode) ShowsEssayFeedback() bool  // with_key only
func (m ReviewMode) ShowsUnansweredCount() bool// no_verdict, with_key
func (m ReviewMode) OnlyCorrect() bool         // correct_only
```

`ShowsVerdict()` returns true **only** for `questions_with_key`. This is the bug
fix: the old predicate claimed `questions_only` should show verdicts, contradicting
business contract **R-18**.

Also update `AllReviewModes` (`enums.go:349`-ish) and `Label()`.

### 1.2 Migration `migrations/0008_review_mode_v2.up.sql`

```sql
ALTER TABLE quizzes DROP CHECK chk_quizzes_review_mode;   -- MariaDB 12 syntax
UPDATE quizzes SET review_mode = CASE review_mode
    WHEN 'questions_and_key' THEN 'questions_with_key'
    WHEN 'answers_only'     THEN 'questions_no_verdict'
    WHEN 'questions_only'   THEN 'questions_no_answers'
    WHEN 'key_only'         THEN 'questions_no_answers'
    ELSE 'questions_no_answers' END;
ALTER TABLE quizzes ALTER COLUMN review_mode SET DEFAULT 'questions_no_answers';
ALTER TABLE quizzes ADD CONSTRAINT chk_quizzes_review_mode
    CHECK (review_mode IN ('questions_no_verdict','questions_no_answers','correct_only','questions_with_key'));
```

Verify the `DROP CHECK` / `ALTER COLUMN … SET DEFAULT` syntax against the
existing migrations and the MariaDB 12.3 version in `docker-compose.yml` before
committing. `key_only` maps to `questions_no_answers`: it reveals question text
but hides the key, so it discloses strictly less than before.

Default for new rows is `questions_no_answers` (reveals nothing). Update
`internal/authoring/quiz_write.go:61`'s literal insert accordingly.

### 1.3 Domain render (`internal/attempts/read.go`)

`assembleResult` (`:656-748`): one explicit switch on the mode. Requirements:

- `StudentAnswer` is filled **only** when `ShowsStudentAnswer()`. Today it is
  filled unconditionally at `:702` before the mode check at `:706` — remove that
  first assignment.
- `Key` only when `ShowsKey()`. `Verdict` only when `ShowsVerdict()`.
- `Graded`/`Score`/`Feedback` only when `ShowsEssayFeedback()`.
- `OnlyCorrect()`: drop every row whose verdict is not `correct`, and drop every
  essay row. **Keep the original `Position`** (do not renumber) so "you got
  3, 7, 12 right" stays readable.
- `Unanswered` only when `ShowsUnansweredCount()`.
- `ReviewHref` is removed from `viewmodel.AttemptResult`. `QuizHref` is removed.
  `HistoryHref` is set only for authenticated students; **empty for anonymous**
  (the old quiz-page fallback at `:750-758` now points at a 404 — delete the
  fallback and its comment).
- `CanRetry`/`RetryHref` unchanged (already requires an open quiz).

Add a `CorrectCount int` to `AttemptResult` for the `correct_only` empty state
and heading, if useful; otherwise reuse `len(Rows)`.

### 1.4 `templates/pages/attempt/result.html`

- Heading `:160`: `correct_only` → "Soal yang dijawab benar";
  `questions_no_answers` and `questions_no_verdict` and `with_key` → "Telaah jawaban".
- Verdict badge block `:170-179`: gate on `ShowsVerdict()`.
- Answer box `:192-203`: render only when `ShowsStudentAnswer()`. Colour wash
  (`is-correct`/`is-wrong`) only when `ShowsVerdict()`.
- Key blocks `:197-202` and `:214-219`: gate on `ShowsKey()`. The second block's
  `not ShowsQuestion` condition disappears with `key_only`; delete the block if
  it becomes unreachable.
- Essay feedback `:221-231` and essay score pill `:181-183`: gate on
  `ShowsEssayFeedback()`.
- Unanswered note `:143-146`: gate on `ShowsUnansweredCount()`.
- `correct_only` with zero rows: a calm block inside the answers section,
  "Tidak ada soal yang dijawab benar." No primary action.
- **Delete** the `Tinjau jawaban` anchor (`:242-244`) — `href` was `""`, a link
  to the current document.
- **Delete** the `Riwayat kuis` button (`:238-241`).
- **Delete** the header `Kembali ke kuis` anchor (`:102`).
- Header trailing slot instead: authenticated student → `link-quiet` +
  `arrow-left` → `Kembali ke riwayat` → `.Data.HistoryHref`. Anonymous → nothing.
- Footer: only `Coba lagi` (`btn btn-solid theme-primary`), when `CanRetry`.
  Omit the `.attempt-footer` entirely when nothing renders.
- Update the page-style block (`:1-89`) for any removed selectors. Remove
  `.answer-box` variants only if `ui-style.html` no longer needs them elsewhere.

### 1.5 Authoring surfaces

- `internal/authoring/validate.go:194-201`: accept exactly the four new values.
- `internal/authoring/publish.go:210-215`: same closed set.
- `templates/pages/teacher/quiz-detail.html:702-710`: four new `<option>`s, new
  labels. Read-only recap at `:741-748` uses `Label()`.
- `templates/pages/teacher/results.html:128-138`: the student-view note must
  describe the new four. For `correct_only` say that only correct answers are
  listed and essays are omitted.

## 2. Closed quiz: no re-entry (student)

- `internal/participation/quiz.go` (`quizIntroModel` / the `actor.IsStudent()`
  branch ~`:303-320`): a **closed** quiz is `404` for students, regardless of
  registration. Draft stays 404. Open stays 200. Use
  `apperr.NotFoundf().WithRule("PAR-088")`.
  **The anonymous branch is deliberately exempt.** Anonymous participants have
  no `/student/history` surface, so the quiz intro is their only route back to
  an in-progress attempt and to a past result. A closed quiz stays reachable for
  a viewer with a live anonymous registration (`internal/participation/quiz.go`,
  `actor.IsAnonymous()` case). This asymmetry is the design: students have a
  history page, anonymous participants do not. Do not "fix" it for symmetry.
- In-progress attempts stay resumable: `start.go`'s deliberate "resume wins over
  closed" path (`:112-125`) is **not** touched. `Lanjutkan` on history-detail
  links to `/attempt/:id` directly and must keep working.
- `internal/participation/studentread.go`: `RetryHref` is set only when the quiz
  is open (it currently is unconditional at `:503`).
- `templates/pages/student/history-detail.html`: gate the header `Buka halaman
  kuis` (`:55`) and the empty-state action (`:103`) on `State == open`. When
  closed, the page offers only per-attempt `Lihat hasil` / `Lanjutkan`.

## 3. Teacher action hierarchy

Rule: **the quiz detail screen is the hub. Results and monitor are drill-downs
that carry a back link and one job control, nothing else.**

### 3.1 `templates/pages/teacher/quiz-detail.html`

- **Delete** the `.lifecycle-links` row (`:857-862`) entirely. That removes the
  duplicate `Lihat hasil` (`:858`), duplicate `Monitor langsung` (`:859`), the
  out-of-scope `Nilai esai` (`:860`) and the self-link `Ubah data kuis` (`:861`).
- Header keeps: `Tambah soal` (primary, draft only), `Monitor` (secondary, open
  only), `Hasil` (secondary, non-draft). Unchanged.
- Status card keeps only the lifecycle forms: `Buka kuis`, `Tutup kuis`,
  `Hapus kuis`.
- `Simpan pengaturan` (`:722`) demotes from `btn-solid theme-primary` to
  `btn btn-outline theme-primary`. The screen's primary is `Tambah soal`.
- Empty-state `Tambah soal` (`:649`) is fine — it is the same action as the
  header primary but appears only when the list is empty, so the header primary
  must be suppressed in that state, or the empty state must not repeat it.
  Choose one; do not show two primaries for one action.
- `internal/authoring/quiz_read.go`: drop the now-unused `GradingHref`,
  `EditHref` population and any field that only fed the deleted row. Keep the
  struct honest.

### 3.2 `templates/pages/teacher/results.html`

- Header: back link `Kembali ke detail kuis` (unchanged) + exactly one
  secondary action, `Nilai esai kuis ini` → `/teacher/grading?quiz={{.Quiz.ID}}`.
  Replace the hardcoded `:90`.
- Remove the empty-state action `Buka detail kuis` (`:151`) when zero attempts —
  it duplicates the back link. Keep `Tampilkan semua` for a filtered-empty list.
- `internal/viewmodel/pages.go` `QuizResults`: add a `GradingHref string` so the
  template stops hardcoding a URL.

### 3.3 `templates/pages/teacher/monitor.html`

- Header drops `Hasil` (`:74`) and `Penilaian` (`:75`). Back link only.
- Empty branch (`:191-195`): remove the `Kembali ke kuis` action that duplicates
  the back link. Message only.
- `MonitorPage.GradingHref` (`pages.go:453`) becomes unused — remove it and its
  assignment in `internal/attempts/teacher.go:581` if nothing else reads it.
  `ResultsHref` may stay if still rendered; otherwise remove too.

### 3.4 `templates/pages/teacher/grading.html`

- Add the missing back affordance: when the page is scoped to a quiz
  (`?quiz=`), a `link-quiet` + `arrow-left` `Kembali ke hasil kuis` →
  `/teacher/quizzes/{id}/results`, first in reading order. Unscoped, no back
  link (it is a top-level nav destination).
- Requires a new `GradingPage.BackHref string` (or `Quiz *QuizRef`), populated in
  `internal/attempts/teacher.go` only when `QuizRaw` resolves to an owned quiz.
- Keep `Simpan nilai` as the primary, `Lewati` as secondary.
- The queue filter `Cari` stays secondary.

### 3.5 Tier normalisation (same pass)

- `quizzes.html:98` `Terapkan` primary → secondary (it is a filter submit).
- `anonymous.html:34` `Cari` primary → secondary.
- `question-editor.html:225` `Batal` duplicates the back link → delete the link,
  keep the back link.
- `quiz-new.html:109` `Batal` duplicates the back link → delete, keep the back link.
- The same duplicate exists on `student-edit.html`, `anonymous-edit.html`, and
  both `profile.html` pages (teacher and student): delete each `Batal` anchor and
  keep the back link. `conformance_test.go` pins this with
  `TestNoCancelLinkDuplicatesBackLink`.
- `dashboard.html:197` and `:201` link to the same URL in one row → keep exactly
  one. Keep the action-column `Lihat hasil`; make the row title plain text.

## 4. Student history: expandable attempts

`internal/viewmodel/pages.go` `HistoryRow` already carries `Attempts []AttemptRow`
(ordered by `attempt_no`) and `DetailHref`. No SQL change.

`templates/pages/student/history.html`: replace the one-`<tr>`-per-quiz table with
a `<details>`-per-quiz list.

- Each quiz is a `<details class="history-quiz">` whose `<summary>` is the row:
  quiz title, teacher, `{{len .Attempts}} percobaan`, best score, last activity,
  state badge, and a chevron. The summary is the disclosure control — no extra
  "Buka detail" button beside it.
- The expanded body lists every attempt as a row: `Percobaan n`,
  `Dimulai`, `Dikirim`, score (or `—`), status badge, and one link —
  `Lihat hasil` for a submitted attempt, `Lanjutkan` for an in-progress one
  (`AttemptRow.ResultHref` already resolves both, `quizgate.go:349-352`).
- Keep `/student/history/:quizID` as the destination for "all attempts of this
  quiz"; put a single `link-quiet` at the end of the expanded body:
  `Lihat semua percobaan` → `.DetailHref`. Do not repeat it in the summary.
- Empty state (`:12-13`) unchanged.
- Pagination (`:50`) unchanged.
- `<summary>` needs a visible focus ring and a chevron that rotates on `[open]`.
  Motion only under `prefers-reduced-motion: no-preference`, per DESIGN §11.
- Styles go in the page's `partials/page-style` block, not `ui-style.html`, so
  the shared partial set and `pinnedPartials` stay untouched.

## 5. Docs to correct in the same change

- `docs/superpowers/specs/2026-10-23-business-logic-04-attempts-grading-and-results.md`:
  rewrite ATT-152's matrix to the four new modes; delete the `questions_only`
  "verdict = yes" clause; update ATT-155's result-page action set; update ATT-297.
- `docs/superpowers/specs/2026-10-23-business-logic-00-contract.md:295-300` (R-18)
  already matches the new behaviour — align its wording with the new value names.
- `docs/DESIGN.md` §8: an empty state carries one primary action **when there is
  one**; where the only available destination is the page's own back link, the
  action is omitted.

## 6. Tests that must exist after this change

1. `internal/attempts` — a visibility matrix test, one case per mode, asserting
   `ResultRow.StudentAnswer` / `Key` / `Verdict` / `Feedback` are populated or
   blank as §1.3 requires. This is the test that was missing and let the leak ship.
2. `correct_only` filtering: only correct rows survive, positions are not
   renumbered, essays are absent, zero-correct yields an empty row set.
3. `internal/authoring` — validation accepts the four new values and rejects
   `key_only`, `questions_only`, `answers_only`, `questions_and_key`.
4. Migration test (or `cmd/migrate` integration test) proving each old value maps
   to its new value and the CHECK constraint rejects an old value afterwards.
5. `internal/participation` — closed quiz is 404 for a student with a live
   registration; open quiz is still 200; an in-progress attempt on a closed quiz
   still resumes.
6. `internal/httpserver` — the result page renders no `href=""`, no `Tinjau
   jawaban`, no `Riwayat kuis`; a closed quiz's result page contains no link to
   `/quiz/`.
