# Rework — teacher grading history and editable grades

Status: decided 2026-10-25. This file is the contract for the grading-history
change. Where it conflicts with an older spec, this file wins and the older spec
must be corrected in the same change.

Repo: `D:\Project\Mother\quiz-v2`. Build: `go build ./...`, `go test ./...`.

## 0. Decisions

1. **History is a tab, not a route.** `GET /teacher/grading?state=graded` lists
   graded essays; the default state is `pending`. This change adds no route;
   `conformance_test.go` pins the surface.
2. **Graded essays are editable by the owning teacher, at any time.** There is no
   same-day window and no original-grader restriction: a quiz has one owner
   account (INV-26), so the owner is the only possible editor.
3. **No revision log.** The list is the history of assessments. An edit
   overwrites score/feedback; `graded_at` keeps the first-grade instant and
   `updated_at` records the last change, so the detail can show both.
4. **Optimistic concurrency.** A new `attempt_answers.grading_version` counter
   detects a stale edit form and turns it into a conflict instead of a silent
   overwrite.

## 1. Migration `migrations/0009_grading_version.up.sql`

```sql
ALTER TABLE attempt_answers
  ADD COLUMN IF NOT EXISTS grading_version SMALLINT UNSIGNED NOT NULL DEFAULT 0;
```

Forward-only and idempotent. Existing rows start at 0, which is also the value a
first-grade form submits. Every grade and re-grade increments the column.

The migration-test fixture that pins "the eight good files" becomes nine
(`internal/migrate/integration_test.go`, `TestBrokenFileLeavesNoLedgerRow`).

## 2. Read model (`internal/attempts/teacher.go`)

### 2.1 State

`GradingQueueInput` gains `StateRaw string` (query key `state`).
`gradingState(raw)` returns `"graded"` for exactly `graded`; everything else is
`"pending"`. Invalid values never 422 (ATT-240).

### 2.2 Predicates

- Pending: live essay answer, `score IS NULL AND graded_at IS NULL`, submitted
  live attempt, live registration, live quiz owned by the session teacher.
- Graded: the same joins with `score IS NOT NULL AND graded_at IS NOT NULL`.
- Filters (`quiz`, `class`, `q`) apply to both states, before pagination. The
  search haystack for the graded tab additionally includes the stored feedback.
- Order: pending keeps `submitted_at DESC, aa.id ASC`; graded uses
  `aa.updated_at DESC, aa.id ASC` (most recently assessed or edited first).

### 2.3 Counts

`GradingCounts` gains `Graded int` (all graded essays across owned quizzes,
ignoring filters). `Pending` becomes a COUNT query rather than `len(rows)` so it
is available while the graded tab is active. `GradedToday` keeps its ATT-203
meaning: first-grade instants only; a re-grade does **not** inflate it because
`graded_at` is preserved on edit.

### 2.4 Rows

`GradingRow` gains:

| Field | Pending | Graded |
|---|---|---|
| `Score *Score` | nil | whole-percent score, for `Tone()` |
| `ScoreLabel string` | "" | one-decimal value, e.g. `85,5` |
| `GradedLabel string` | "" | `Dinilai 23 Okt 2026` |
| `EditedLabel string` | "" | `Diubah 25 Okt 2026`, empty when never edited |
| `WaitingLabel string` | ATT-205 | "" |

The row href carries the active state and filters:
`/teacher/grading/{id}?state=graded&q=…&class=…&quiz=…` when any are present;
the pending, unfiltered href stays `/teacher/grading/{id}` (existing tests).

### 2.5 Empty states

- Pending empty: unchanged (ATT-204).
- Graded empty: `Belum ada esai yang dinilai.` /
  `Esai yang sudah dinilai akan muncul di sini.`
- Filtered empty in either tab: existing copy, with the reset action pointing at
  the active tab.

## 3. Detail (`GradingDetail`)

`EssayAnswer` gains `GradedAt *LocalTime`, `UpdatedAt *LocalTime`,
`Edited bool`, `Version int`. `GradingDetail` gains `ScoreInput string` for the
score field so a one-decimal stored score round-trips (`85,5`, not `86`).

- Graded detail: the form is enabled and pre-filled, the badge stays
  `Sudah dinilai`, and the header shows the stored score. Meta shows
  `Dinilai <DateTime>` and, when edited, `Diubah <DateTime>`.
- `NextHref` follows the active queue: next pending essay in the pending tab,
  next graded essay in the graded tab. It preserves the active query.
- `Action` is the current detail path plus the preserved query; the POST can
  therefore return the teacher to the same tab.
- No suggested score (ATT-213 unchanged).

## 4. Write path (`GradeEssay`)

`GradeInput` gains `VersionRaw string` (form key `version`).

Validation is unchanged (ATT-222). Inside the transaction, after the existing
ownership/essay/submitted checks and the answer lock:

1. Parse `VersionRaw` as a non-negative integer; a malformed value is a stale
   conflict (never a 422).
2. **First grade** (`score IS NULL`): the submitted version must be 0. Update
   `score`, `feedback`, `graded_by_account_id`, `graded_at = now`,
   `grading_version = grading_version + 1`, guarded by
   `score IS NULL`; zero rows → conflict.
3. **Re-grade** (`score IS NOT NULL`): the submitted version must equal the
   stored `grading_version`. Update `score`, `feedback`,
   `graded_by_account_id = actor`, `grading_version = grading_version + 1`;
   `graded_at` is **not** changed. Guard on
   `grading_version = ? AND score IS NOT NULL`; zero rows → conflict.
4. Recompute `final_score` in the same transaction (ATT-223/ATT-224). A changed
   score changes the result; a changed feedback does not.

Conflict (stale form or lost race): rollback, 303 to
`/teacher/grading/{answerID}` with error flash `grade_conflict`:
"Nilai sudah diubah di tempat lain." / "Muat ulang halaman untuk melihat nilai
terbaru." `already_graded` and `ErrAlreadyGraded` are removed.

Success: 303 to the queue named by the POST query (`?state=graded` returns to
the history tab, otherwise the pending queue) with flash `graded`:
"Nilai esai tersimpan."

`gradingDraft` overlays submitted values for a 422 on graded answers too, and
echoes the raw score string, not a rounded percent.

## 5. Template (`templates/pages/teacher/grading.html`)

- Render `.Queue.Tabs` (already wired): `Menunggu` → `/teacher/grading` plus the
  active filters, `Sudah dinilai` → the same with `state=graded`. Counts come
  from `Pending`/`Graded`.
- History rows: score pill (`ScoreLabel` + tone), `GradedLabel` on the right,
  `EditedLabel` when present. Pending rows keep the waiting label.
- Search form keeps `state` (its existing hidden input) and the active tab when
  no score field is touched.
- Detail form:
  - the score field is `type="text"` with `inputmode="decimal"`; a `number`
    input would silently discard the comma decimal and render the field empty.
    `value="{{.ScoreInput}}"`, not `.Score.Int` (fixes the one-decimal prefill).
  - hidden `version` when graded.
  - submit label `Simpan perubahan` when graded, `Simpan nilai` otherwise.
  - the skip link reads `Berikutnya` when graded, `Lewati` otherwise.
  - inline `.field-error` blocks for `score` and `feedback`, with
    `aria-invalid`/`aria-describedby` when `.Errors.Get` returns a message.
  - a graded note: "Perubahan menggantikan nilai sebelumnya. Waktu penilaian
    pertama tetap tercatat."
- No new hex, class vocabulary, or Bootstrap JS. Score pill uses the existing
  `partials/score.html` classes.

## 6. Flash keys (`internal/viewmodel/types.go`)

| Key | Kind | Copy |
|---|---|---|
| `graded` | success | "Nilai esai tersimpan." (unchanged) |
| `grade_conflict` | error | "Nilai sudah diubah di tempat lain." / "Muat ulang halaman untuk melihat nilai terbaru." |

Remove `already_graded`.

## 7. Tests

1. `internal/attempts` — graded tab lists only graded essays, ordered by
   `updated_at DESC`, with one-decimal labels; counts ignore filters; the
   pending tab is unchanged.
2. Filters (`quiz`, `class`, `q`) apply to the graded tab; feedback is searchable.
3. Detail of a graded essay exposes score input `85,5`, `Version`, `GradedAt`,
   and `Edited=false`; after an edit `Edited=true`, `GradedAt` unchanged,
   `UpdatedAt` later.
4. Re-grade changes the score and `final_score`; keeps `graded_at`, bumps
   `grading_version`, records the actor.
5. Stale version → `ErrGradeConflict` with `grade_conflict` and no write.
   First grade with a non-zero version → conflict. Malformed version → conflict.
6. A missing version on an already-graded POST (old link/bookmark) → conflict,
   not an overwrite.
7. HTTP: `?state=graded` renders the history tab and the editable form; a
   `state=invalid` falls back to pending; a re-grade redirects to the graded tab.
8. Migration: 0009 adds the column with default 0 and the nine-file runner test
   passes.

## 8. Docs corrected in the same change

- `docs/superpowers/specs/2026-10-23-business-logic-04-attempts-grading-and-results.md`
  ATT-201, ATT-202, ATT-203, ATT-204, ATT-206, ATT-207, ATT-211, ATT-212,
  ATT-214, ATT-221, ATT-223, ATT-224, ATT-225, ATT-226, ATT-227, ATT-240,
  ATT-275, ATT-295 (D3), AT-32, AT-37.
- `docs/superpowers/specs/2026-10-23-schema-and-migrations-design.md` §6.10 and
  the migration note list: add `grading_version`, 0009.
