# Quiz XLSX export — design

Status: final 2026-10-25. Approved. No open product questions.
Amended 2026-10-26 by `2026-10-26-quiz-xlsx-export-layout-polish.md` for the
workbook layout (§6) only; its changes carry authority there.
Repo: `quiz-v2`. Build: `go build ./...`, `go test ./...`.

## 0. Summary

A teacher who owns a quiz downloads one `.xlsx` snapshot of that quiz. The
workbook has four visible sheets: participants, questions with the answer key,
every submitted attempt with every response and score, and a summary with a
live ranking. The sheets are linked with native formulas, so editing a score
in the grading sheet recalculates the attempt total, the participant average,
and the ranking order.

CSV was considered and dropped: a CSV is one table and cannot hold four sheets
or cross-sheet formulas. XLSX is the only format.

## 1. Intent and why

The teacher already has a results screen and a grading queue. Neither leaves
the building. This export is the copy a teacher can open in Excel, adjust an
essay score, sort a class, or hand to a colleague — without the app being
online and without losing the link between a response, its score, and the
class summary.

The workbook is a working copy, not a report: the ranking must reorder when a
score changes, and the summary must be computed from the grading sheet rather
than pasted numbers.

## 2. Goals

- One quiz per download, generated on demand, owner-only.
- Four visible sheets, Indonesian names and headers.
- Every submitted attempt retained as its own row. No best-of and no
  latest-only view; the attempt rows are never collapsed.
- A summary with total registered, total completed, the not-completed
  difference, total score, average score, and a ranking ordered high to low.
- A per-student score used for the ranking and the summary that is the mean of
  that student's scored attempts.
- The best attempt row for each participant highlighted in `Penilaian`, with
  no extra best-score column.
- Native formulas linking the sheets; the ranking reorders when a score is
  edited in the workbook.
- Question images embedded in the questions sheet, with `[IMAGE]` as the
  fallback.

## 3. Non-goals and anti-goals

Non-goals:

- CSV, ZIP of CSVs, bulk or multi-quiz export.
- Importing the workbook back into the app.
- Live refresh from the database. The file is a snapshot.
- Per-question analytics beyond the listed summary (no difficulty index, no
  histogram, no per-option distribution).
- Excel 365 dynamic arrays, macros, VBA, `.xlsm`.
- Print-ready pagination or a fixed print area.
- Essay feedback in the workbook. Feedback stays in the app; the export is for
  responses, scores, and the ranking.
- Changes to the in-app results or grading screens beyond the `Unduh XLSX`
  link in the results header.

Anti-goals:

- A file that silently drops an attempt, a participant, or a question.
- A file that only works in Excel 365.
- A file where student-entered text can execute as a formula.
- A file whose score disagrees with the app's stored score.

## 4. Decisions

### 4.1 User decisions (fixed)

| Decision | Value |
|---|---|
| Format | XLSX only. CSV is out. |
| Scope | One quiz per export; only the owner may export. |
| Attempts | All submitted attempts, exactly as they are. |
| Retake score | The mean of that student's scored attempts. |
| Best attempt | Highlight the best attempt row in `Penilaian`; no extra best-score column. |
| Summary metrics | Total registered, total completed, the not-completed difference, total score, average score, ranking. |
| Ranking | High to low, dynamic: a score edit reorders it. |
| Images | Display in the cell; `[IMAGE]` if that fails. |

### 4.2 Design decisions (settled)

- **Non-draft only.** Export is offered for open and closed quizzes. A draft
  has no attempts and the results page already 404s it; the route 404s a draft
  too.
- **Entry point.** A secondary "Unduh XLSX" link in the results page header,
  beside "Nilai esai kuis ini". The prior rule that results carries exactly one
  secondary action is amended for this page.
- **Route.** `GET /teacher/quizzes/:id/export.xlsx`, class
  `ClassOwnerTeacher`, implemented as a `HandlerCtx` download, not a page.
- **Filename.** `kuis-<id>-<slug>.xlsx`, ASCII slug from the title.
- **Language.** Sheet names, headers, and status values are Indonesian,
  sentence case, matching the rest of the UI.
- **Participants.** Anonymous participants are included; they took the quiz.
- **Participant score.** The mean of the participant's scored attempts, the
  same value the ranking uses. There is no best-of or latest-only column; the
  individual attempts stay in `Penilaian`.
- **Best-attempt highlight.** In `Penilaian`, the row(s) whose final score
  equals that participant's highest final are highlighted with a conditional
  format. There is no extra column. Ties highlight every tied row. The
  highlight is formula-driven, so editing a score moves it.
- **Response letters.** `Soal` is canonical. A response in `Penilaian` names
  the canonical option letter and body (`B. Jakarta`), not the order the
  student saw when `randomize_options` was on. The workbook does not read
  `shuffle_seed`; the option text is the authoritative response.
- **Class label.** `Kelas` is the combined label the app shows (`XII IPA`),
  via `viewmodel.ClassLabel`; `Jurusan` stays a separate column for filtering.
- **Dates.** WIB, `yyyy-mm-dd hh:mm`, matching every in-app date. Dates are
  written as real Excel date serials, not strings, so filtering and sorting
  work.
- **Feedback.** Not exported; see §3.
- **Attempts included.** Submitted only. In-progress attempts are excluded.
- **Waiting attempts.** An attempt whose essays are not all graded has no final
  score. It is shown in the grading sheet with blank score cells and status
  `Menunggu`; it does not contribute to any average or rank.
- **Deleted content.** Questions and options are frozen after publish
  (`contentDecision`), so a non-draft quiz's live set is the set every attempt
  answered. The export uses the live set.
- **Score scale.** Per-question scores are 0–100 with up to two decimals; the
  final is recomputed with the app's integer contract, not a plain `ROUND` of
  the mean (see §7.1).
- **Ranking mechanism.** `LARGE` + `INDEX`/`MATCH` over a hidden key block.
  No dynamic arrays, no macros. Works in Excel 2010+ and LibreOffice.
- **Filtering.** AutoFilter on the ranking and grading tables. Manual sorting
  is not supported: it fights the formula-driven order.
- **No sheet protection.** The teacher can edit anything; the formulas are a
  convenience, not a lock.
- **Images.** JPEG, PNG, and WebP. WebP is decoded and re-encoded as PNG
  because Excel cannot embed WebP. At most 10 MiB per image (the app's
  `authoring.MaxImageBytes`) and 50 MB total; otherwise `[IMAGE]`.
- **Empty export.** A quiz with no registrations or no submitted attempts
  still produces a valid workbook: every range reserves one blank row and the
  metric cells fall back to `0` or blank.
- **Dependencies.** `github.com/xuri/excelize/v2` for XLSX and
  `golang.org/x/image/webp` for WebP transcoding.

## 5. Data and access

### 5.1 Read model

One read transaction. No N+1: every list is one query, and answers/selections
are fetched in bulk with `IN` lists.

| Piece | Source |
|---|---|
| Quiz metadata, owner name, dates, state | `quizzes` join `accounts` |
| Questions, ordered by position | `questions` where live; includes `image_path`, `image_mime`, `image_bytes`, `image_alt` |
| Options, ordered by position | `answer_options` where live |
| Registrations and identity | `registrations` left join `students`/`accounts`, `anonymous_participants`, `majors` |
| Submitted attempts | `quiz_attempts` where live and `submitted_at` not null |
| Answers | `attempt_answers` where live |
| Selections | `attempt_answer_selections` where live |

The quiz lookup is the existing `loadOwnedQuiz` contract: `id`,
`owner_account_id`, live, else `ErrNotFound`. A draft is `ErrNotFound` after
that check. The results page's `loadResultAttempts` is the template for the
name/class/major join, but the export needs its own row type because it keeps
every attempt and every answer.

The read model deliberately does not load `quiz_attempts.shuffle_seed`,
`quizzes.randomize_options`, or `attempt_answers.feedback`: responses are
canonical, and feedback is out of scope. Image bytes are read from
`cfg.UploadsDir` after the ownership check, only for the live questions the
export already selected.

### 5.2 Ownership

Only `quizzes.owner_account_id = actor`. There is no co-teacher or admin
override in this design. `ClassOwnerTeacher` already turns a student into 404;
a foreign teacher, a missing quiz, and a draft are 404 in the handler. The
file must not reveal that a quiz exists.

## 6. Workbook layout

Sheet names are exactly `Ringkasan`, `Peserta`, `Soal`, `Penilaian`. No hidden
sheet: the helper block lives in hidden columns of `Ringkasan`, so the visible
sheet count stays four.

Every row count below (`p`, `q`, `a`, `h`) is the number of data rows actually
written. A range's last row is `MAX(2, count + 1)`, so a zero-row sheet still
has one blank row and every formula below stays valid. The extra row is blank
and never changes a count, sum, or average.

### 6.1 `Ringkasan` (summary)

Metadata block, rows 3–10:

| Cell | Label | Value |
|---|---|---|
| A3 / B3 | Judul | quiz title |
| A4 / B4 | Guru | owner full name |
| A5 / B5 | Status | `Draf`, `Dibuka`, `Ditutup` |
| A6 / B6 | Dibuat | created at |
| A7 / B7 | Diterbitkan | published at or `—` |
| A8 / B8 | Ditutup | closed at or `—` |
| A9 / B9 | Jumlah soal | `=COUNTA(Soal!$A$2:$A$<q+1>)` |
| A10 / B10 | Maks. percobaan | effective max (1 when retakes are off) |

Metrics block, rows 12–16:

| Cell | Label | Formula |
|---|---|---|
| A12 / B12 | Total terdaftar | `=COUNTA(Peserta!$A$2:$A$<p+1>)` |
| A13 / B13 | Total selesai | `=COUNTIF(Peserta!$G$2:$G$<p+1>,">0")` |
| A14 / B14 | Belum selesai | `=B12-B13` |
| A15 / B15 | Total skor | `=IFERROR(SUM($N$2:$N$<h+1>),"")` |
| A16 / B16 | Rata-rata skor | `=IFERROR(AVERAGE($N$2:$N$<h+1>),"")` |

`Total selesai` counts participants with at least one submitted attempt,
regardless of whether it is graded. The results page's `Selesai` counts
attempts; this metric counts participants, so the difference against
`Total terdaftar` is meaningful. `Belum selesai` is registered minus completed
— the drop-off, as confirmed. `Peserta!I` is a separate grading status and can
still say `Menunggu` for a participant already counted here.

`Total skor` is the sum of the per-student means used in the ranking, so
`Total skor / count(scored students)` is the average. Each student is counted
once, as confirmed. A participant who never submitted, or whose attempts are
all still waiting for essay grading, contributes nothing to either.

Ranking table, starting at A18:

| Column | Header | Content |
|---|---|---|
| A | Peringkat | 1..h, static |
| B | Nama | lookup by rank key |
| C | Kelas | lookup by rank key |
| D | Percobaan | lookup by rank key |
| E | Rata-rata | lookup by rank key |
| F | (hidden) | rank key, `LARGE(...)` |

Rows 19..(18+h), where `h` is the number of participants with at least one
submitted attempt. `C` is the combined class label (`XII IPA`). Participants
still waiting for essay grading occupy blank rows at the bottom; when their
last score is entered, their key becomes numeric and they take the rank their
score earns. A note under the table says this in one sentence.

AutoFilter on A18:E18. The table is filterable by class; its order stays
formula-driven. Filtering is a view only: the summary metrics read the full
ranges and do not change when a filter is applied.

### 6.2 `Peserta` (participants)

| Column | Header | Content |
|---|---|---|
| A | ID | registration/participant id, stable key |
| B | Nama | student full name or anonymous display name |
| C | Jenis | `Siswa` or `Anonim` |
| D | Kelas | combined class label (`XII IPA`, `XII`, `IPA`, or `—`) via `viewmodel.ClassLabel` |
| E | Jurusan | major name, or `—` |
| F | Terdaftar | registration created at |
| G | Percobaan | `=COUNTIFS(Penilaian!$A$2:$A$<a+1>,$A2)` |
| H | Rata-rata | `=IFERROR(AVERAGEIFS(Penilaian!$<final>$2:$<final>$<a+1>,Penilaian!$A$2:$A$<a+1>,$A2),"")` |
| I | Status nilai | `=IF($G2=0,"Belum ada nilai",IF($H2="","Menunggu","Sudah dinilai"))` |

One row per live registration, sorted by name then id. `Percobaan` counts
submitted attempts; `Rata-rata` is the mean of the scored attempts, the same
value the ranking uses. There is no best-of column. `Status nilai` tracks
grading only: `Belum ada nilai` means no submitted attempt, `Menunggu` means
submitted but none scored, `Sudah dinilai` means at least one scored attempt
(other attempts may still be waiting; their rows say `Menunggu`). The summary's
`Total selesai` counts submitted participants, not this grading status.

### 6.3 `Soal` (questions and key)

| Column | Header | Content |
|---|---|---|
| A | No | question position |
| B | Tipe | `Pilihan tunggal`, `Pilihan ganda`, `Esai` |
| C | Soal | prompt, wrapped |
| D | Gambar | embedded image, or `[IMAGE]` |
| E | Pilihan | `A. …` newline `B. …`, blank for essay |
| F | Kunci | `B`, `A, C`, or `—` for essay |

One row per live question, ordered by position. The key is the readable option
letter(s), not an internal id. This sheet is always canonical: when the quiz
randomizes options, the letters here are still the stored option positions,
and `Penilaian` uses the same letters. The student's shuffled view is not
reconstructed. Images are anchored to column D; row height is set to fit. The
image's alt text, when present, is set as the picture's alt text.

### 6.4 `Penilaian` (grading)

One row per submitted attempt, sorted by participant name then attempt number,
so a student's three attempts sit together.

| Column | Header | Content |
|---|---|---|
| A | ID peserta | participant id |
| B | Nama | `=INDEX(Peserta!…,MATCH(A2,…))` |
| C | Kelas | lookup of `Peserta!D`, the combined label |
| D | Percobaan | attempt number |
| E | Dikirim | submitted at |
| F | Status | `=IF(<final>="","Menunggu","Selesai")` |
| G..G+n−1 | `S1`..`Sn` | response text, one column per question |
| G+n..G+2n−1 | `N1`..`Nn` | per-question score, 0–100, blank if ungraded |
| last | Skor akhir | the final formula (§7.1) |

The `S`/`N` headers are formulas (`="S"&Soal!$A$2`) so the columns stay aligned
with the question sheet. Responses are readable text:

- single choice: `B. Jakarta`
- multiple choice: `A. …; C. …`
- essay: the answer text
- unanswered: `—`

Letters are canonical `Soal` order, not the attempt's shuffled order; the
option text is the authoritative response. There is no feedback column:
feedback is not exported. The score block is contiguous and follows the
response block, so the final formula can use `COUNT`/`SUMPRODUCT` over one
range. Freeze panes at G2; AutoFilter on the header row.

Conditional formatting on `$A$2:$<final>$<a+1>` highlights the best attempt(s)
for each participant. The rule is:

```
=AND($<final>2<>"",$<final>2=IFERROR(AGGREGATE(14,6,
  $<final>$2:$<final>$<a+1>/($A$2:$A$<a+1>=$A2),1),""))
```

`AGGREGATE(14,6,…)` is `LARGE` with errors ignored: the division leaves only
the participant's rows, and a waiting final (`""`) becomes an error and is
ignored. A light fill marks the row; the header fill is unchanged. If two
attempts tie for the best, both are highlighted. A note under the table says
this in one sentence.

### 6.5 Hidden helper block (`Ringkasan!J:O`)

One row per participant with at least one submitted attempt, sorted by name.
This is the ranking's source and the reason waiting students can join later.

| Column | Content |
|---|---|
| J | participant id, static |
| K | name, `INDEX`/`MATCH` into `Peserta!B` |
| L | class, `INDEX`/`MATCH` into `Peserta!D` (combined label) |
| M | attempts, `INDEX`/`MATCH` into `Peserta!G` |
| N | mean of scored attempts, `INDEX`/`MATCH` into `Peserta!H` |
| O | key: `=IF($N2="","",$N2-ROW()/1000000000000)` |

The helper is one row per participant with at least one submitted attempt,
sorted by name then id. It reads only `Peserta`, so the participant summary is
the single source of truth and the helper is just the ranking key table. The
tiny `ROW()` subtraction makes ties unique and stable: equal averages rank by
the helper order, which is name order. The divisor must be large enough that
the row term stays below the smallest possible gap between two means: with at
most 10 attempts and 4-dp units, that gap is 1e-5, so `1e12` is safe for any
realistic class size. `N` is the number displayed; `O` is only for
`LARGE`/`MATCH`. If `h` is zero, row 2 is blank and the ranges still resolve.

### 6.6 Styling

Plain and readable, not a web-page copy: bold white-on-`#293681` header row,
light borders, frozen headers, wrapped text, top alignment. Dates are WIB
serials formatted `yyyy-mm-dd hh:mm`; scores `0.##`; averages `0.0`. The
best-attempt row in `Penilaian` gets a light fill that stays readable under
the header and borders. Column widths sized to content, response columns ~28,
score columns ~8, image column ~30.

## 7. Formula contract

### 7.1 Score fidelity

The app stores scores on a 0–1 scale in 4-decimal units. `FinalScoreUnits` is
`floor((2ΣU + n) / (2n))`; `DisplayPercent` is
`floor((U + 50) / 100)` (`internal/attempts/score.go`). A plain
`ROUND(AVERAGE(percent),0)` can differ by one point because it rounds twice in
a different order. The workbook must reproduce the app's integer path.

Per-question score cells hold `U/100` (0–100, up to two decimals). The final:

```
=IF(COUNT(<scores>)<Ringkasan!$B$9,"",
  FLOOR((FLOOR((2*SUMPRODUCT(ROUND(<scores>*100,0))+Ringkasan!$B$9)
              /(2*Ringkasan!$B$9),1)+50)/100,1))
```

`ROUND(<scores>*100,0)` recovers the stored units; the two `FLOOR`s reproduce
`FinalScoreUnits` and `DisplayPercent`. A test pins the formula result against
the Go functions for a matrix that includes the case where a plain
`ROUND(AVERAGE(percent),0)` is one point low: `{0.8449, 0.8450}`. The app's
path gives 8450 units and 85%; a plain `ROUND(84.495,0)` gives 84. The formula
is written on every `Penilaian` row, and `<scores>` is that row's N block.

An attempt is `Menunggu` while any score cell is blank; the final is blank, the
attempt is excluded from every average, and it is not ranked.

### 7.2 Sheet links

- `Peserta` counts and mean read `Penilaian`.
- `Penilaian` names/classes read `Peserta`; its headers read `Soal`.
- `Ringkasan` metrics read `Peserta` and the hidden helper.
- The helper reads `Peserta` only.
- The ranking reads the helper.

Nothing is pasted twice except stable ids, question positions, and the
response/score values themselves. Editing a score in `Penilaian` flows through
the final formula, the participant average, the key, and the ranking. Editing a
name in `Peserta` flows to `Penilaian` and the ranking.

The key sheet is a reference, not a re-grader: editing a response does not
change its score. Re-grading in Excel is out of scope.

### 7.3 Ranking

For rank row `k`:

```
F19 =IFERROR(LARGE($O$2:$O$<h+1>,$A19),"")
B19 =IF($F19="","",INDEX($K$2:$K$<h+1>,MATCH($F19,$O$2:$O$<h+1>,0)))
C19 =IF($F19="","",INDEX($L$2:$L$<h+1>,MATCH($F19,$O$2:$O$<h+1>,0)))
D19 =IF($F19="","",INDEX($M$2:$M$<h+1>,MATCH($F19,$O$2:$O$<h+1>,0)))
E19 =IF($F19="","",INDEX($N$2:$N$<h+1>,MATCH($F19,$O$2:$O$<h+1>,0)))
```

`LARGE` ignores text and blanks, so waiting participants drop out until they
have a score. `MATCH` is exact on the same key cells `LARGE` returned, so the
lookup is stable.

### 7.4 Calculation on open

Set the workbook's calc properties to automatic and full-recalc-on-load. Excel
and LibreOffice then calculate the formulas when the file opens. The builder
also writes a sensible cached value where the library allows it, but no viewer
is required to trust the cache.

## 8. Images

- Resolve `cfg.UploadsDir` + the question's stored `image_path`. The name is
  already constrained by `authoring.ValidImageName`; re-check it and reject any
  path that escapes the uploads directory.
- Only live questions of the owned quiz. No cross-quiz or cross-teacher image.
- Supported: JPEG, PNG, WebP. WebP is decoded with
  `golang.org/x/image/webp` and re-encoded as PNG in memory before
  `AddPicture`, because Excel cannot embed WebP. The app stores no GIF, so GIF
  is not a case.
- Maximum 10 MiB per image (the app's `authoring.MaxImageBytes`) and 50 MB
  total. The total is checked before each embed; once it would be exceeded,
  the remaining images fall back to `[IMAGE]`.
- Anchor at `Soal!D<row>` with `GraphicOptions{AutoFit: true, AltText: ...}`
  so the image fits the cell; set row height ~96 and column width ~30.
- Any failure — missing file, unreadable bytes, unsupported type, over budget,
  decode error, library error — writes the literal string `[IMAGE]` in D and
  continues. One bad image never fails the export.
- If the question has alt text, set it as the picture's alt text; no cell
  comment is required.

## 9. HTTP surface and UI

Route: `GET /teacher/quizzes/:id/export.xlsx`.

- Registered as a `PageRoute` with `Auth: ClassOwnerTeacher` and a
  `HandlerCtx`; no template. The pipeline still resolves identity, session,
  database, and path params.
- Adds one route to the contract: `contractRoutes` goes from 77 to 78, and
  `routeClasses` gets `ClassOwnerTeacher` for the new key.
- Response headers:
  - `Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`
  - `Content-Disposition: attachment; filename="kuis-12-ulangan-harian.xlsx"`
  - `Cache-Control: no-store`
- Errors map like the results page: `ErrNotFound` → 404, anything else → the
  normal error page. No partial file is sent: build the bytes first, then send.
- No CSRF token: it is a GET with no state change.

UI: the results page header gains a second secondary action:

```html
<a class="btn btn-outline theme-primary" href="{{.ExportHref}}">
  Unduh XLSX
</a>
```

`internal/viewmodel/pages.go` `QuizResults` gains `ExportHref string` beside
the existing `GradingHref`, so the template hardcodes neither URL. The
existing "Nilai esai kuis ini" stays. Both are secondary; the page still has
no primary. A new `i-download` symbol (mirror of `i-upload`, same 24×24
stroke style) goes in `templates/partials/icons.html` if the icon is used;
text-only is acceptable.

## 10. Security and robustness

- **Ownership first.** No query for participants, questions, or images runs
  before the owned-quiz check.
- **Formula injection.** Every untrusted string — names, titles, prompts,
  options, essay answers, alt text, class labels — is written with
  `SetCellStr` (a string cell), never `SetCellValue` (which infers a formula).
  A value beginning with `=`, `+`, `-`, or `@` stays literal. Sheet names are
  fixed constants. A test names a student `=HYPERLINK(...)` and asserts the
  cell has no formula.
- **Control characters.** Strip bytes Excel rejects (0x00–0x08, 0x0B, 0x0C,
  0x0E–0x1F); cap a cell at 32,000 characters and mark a truncation.
- **Path safety.** Images are resolved under `cfg.UploadsDir` with the existing
  name grammar; no user-controlled path reaches the filesystem.
- **No PII beyond the screen.** The file contains what the teacher already sees
  on the results and grading screens, including anonymous display names.
- **Snapshot consistency.** One read transaction; the workbook cannot mix two
  states of the quiz.
- **Size.** The workbook is built in memory. Class-sized data is small; a quiz
  with tens of thousands of attempts is out of scope and may be rejected with a
  clear error rather than exhausting the process.

## 11. Tests

1. `internal/attempts` — export read: all submitted attempts present, in-progress
   excluded, anonymous participants included, questions/options complete,
   answers/selections attached, ungraded final NULL preserved, no feedback
   loaded.
2. `internal/attempts` — ownership: foreign teacher, student, missing quiz, and
   draft all `ErrNotFound`; owner succeeds.
3. `internal/export` — workbook structure: exactly four sheets, exact names,
   headers, row counts, freeze panes, AutoFilter. An empty quiz (no
   registrations, no submitted attempts) still opens as a valid workbook.
4. `internal/export` — formula presence: participant count/mean/status, summary
   metrics, ranking `LARGE`/`INDEX`/`MATCH`, final-score formula. No best-of
   column exists.
5. `internal/export` — score fidelity: the final formula result equals
   `DisplayPercent(FinalScoreUnits(...))` for a fixture matrix including
   `{0.8449, 0.8450}` and `{1.0, 0.6667}`; for the first, a plain
   `ROUND(AVERAGE(...),0)` is asserted to be 84 while the app path is 85.
6. `internal/export` — images: a tiny PNG embeds; a WebP transcodes to PNG and
   embeds; a missing file yields `[IMAGE]`; an oversized image yields `[IMAGE]`;
   the total budget falls back cleanly.
7. `internal/export` — canonical letters: with `randomize_options` on, a
   selected option still renders its canonical `Soal` letter, and the read
   model has no `shuffle_seed`/`randomize_options` dependency.
8. `internal/export` — class and dates: a student with grade 12 and major IPA
   renders `XII IPA`; dates are WIB serials with the expected format.
9. `internal/export` — best-attempt highlight: the conditional-format rule is
   present on the `Penilaian` row range, references the final column, and uses
   `AGGREGATE(14,6,…)`; a tie highlights both rows.
10. `internal/export` — injection: a student name and an essay starting with `=`
    are literal strings, not formulas.
11. `internal/httpserver` — owner gets 200, the XLSX content type, an attachment
    disposition, and a non-empty ZIP body; foreign teacher, student, and draft
    all get 404.
12. `internal/httpserver` — conformance: the route surface is 78 and the authz
    matrix has the new key.
13. Manual — open in Excel and LibreOffice: the ranking is ordered; edit an
    essay score in `Penilaian`; the attempt final, participant average, rank
    order, and best-attempt highlight change; the image is visible.

## 12. Rollout

- `go get github.com/xuri/excelize/v2 golang.org/x/image`, `go mod tidy`.
- No migration, no config key, no new environment variable.
- Update the route contract pin and authz matrix in the same change. The
  `routeClasses` comment still says "74 pinned routes"; correct it to the new
  count while touching the table.
- The results-page rule in
  `docs/superpowers/specs/2026-10-25-rework-result-visibility-and-actions.md`
  §3.2 is already amended (2026-10-25) to allow the export action beside the
  grading action; implement the two-action header there.
- Manual Excel/LibreOffice check before the change is called done.

## 13. Builder latitude

Settled by the builder without coming back: exact column widths, header fill
shade within the palette, the best-attempt fill tint, helper-block placement
inside `Ringkasan`, note copy under the ranking, whether the button carries an
icon, whether `Peserta` also gets an AutoFilter, JPEG EXIF orientation
handling, and the internal package split between the read model and the
workbook builder.

## 14. Finalization

This spec is final as of 2026-10-25. The user confirmed:

- `Belum selesai` is registered minus completed.
- `Total skor` is the sum of each student's mean, with each student counted
  once.
- The best attempt row in `Penilaian` is highlighted; there is no extra
  best-score column.

Everything else is either a fixed decision in §4.1, a settled design decision
in §4.2, or builder latitude in §13. Acceptance is §11 passing, including the
manual Excel/LibreOffice check in §11.13. Changing a §4.1 decision, a formula
in §7, or the workbook layout in §6 requires a new spec, not an edit here.
