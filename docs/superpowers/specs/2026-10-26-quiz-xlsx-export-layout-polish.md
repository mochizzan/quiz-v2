# Quiz XLSX export — layout polish

Status: final 2026-10-26. Implements the layout changes requested as a
feedback round on the 2026-10-25 export
(`2026-10-25-quiz-xlsx-export-design.md`). That spec remains the authority
for scope, formulas, score fidelity, and the HTTP surface. This document
amends only the workbook layout (design §6) and the formula strings that
must follow the new column positions (§7.2/§7.3 shapes unchanged).

## 0. Why

The first real-world export (`kuis-392-latihan-pagi.xlsx`) was reviewed by
its user: the layout reads confusing and disorganized. Concrete complaints:

- The worksheet windows show raw sheet gridlines plus the table borders, so
  tables do not read as objects.
- The `ID` / `ID peserta` columns are internal registration numbers; a
  teacher never uses them, they only widen the frozen pane and push useful
  columns to the right.
- `Peserta` and `Penilaian` have no visible row-number column, which makes
  class discussions ("soal nomor tiga", "baris sebelas") and print-outs
  harder to reference.
- Column widths, row heights, alignment, and the body borders are one-size
  for every sheet, so wide text (essay responses, prompts, names) clips.
- Every data table looks identical to the note rows and to empty areas.

## 1. What changes

### 1.1 Sequential numbering column on every table

- `Peserta` gains a visible column A `No` with the sheet-order sequence
  1..p (the sheet sorts by name, so No is the alphabetical class list
  number).
- `Penilaian` gains a visible column A `No` with the sequence 1..a over its
  attempt rows.
- `Soal` already numbers rows with the question position in A. Unchanged.
- `Ringkasan`'s ranking table already numbers rows 1..h in A (`Peringkat`).
  Unchanged. The metadata and metric blocks are label/value pairs, not
  tables; they get no numbering column.

All `No` cells are static integers written by the builder; they are not
formulas, so re-filtering, sorting, or editing grader answers never breaks
them (the export never claims stable row identity under manual sorts,
design §4.2 keeps manual sorting unsupported).

### 1.2 IDs become hidden helper columns

The participant id stays in the workbook because five formula systems use
it as the join key (`Penilaian` name/class lookups, `Penilaian` best-attempt
highlight, `Peserta` attempt count/average, the `Ringkasan` ranking helper,
the summary counts). Removing it would force matching by student name,
which breaks on duplicate names. Instead:

- `Peserta`: the id moves from visible A to hidden trailing J. Visible
  order: `No, Nama, Jenis, Kelas, Jurusan, Terdaftar, Percobaan, Rata-rata,
  Status nilai`.
- `Penilaian`: the id moves from visible A to a hidden trailing column
  immediately after `Skor akhir` (column 7+2q+1). Visible order: `No, Nama,
  Kelas, Percobaan, Dikirim, Status, S1..Sq, N1..Nq, Skor akhir`.
- The hidden columns keep the header `ID` and body styling so unhiding them
  stays coherent.
- Every formula that read `Peserta!$A$` or `Penilaian!$A$` for the id now
  targets the hidden column. `Ringkasan!B12` counts `Peserta!$A$` — that
  column now holds the static `No` values, which count the same set, so
  the formula string is untouched.

### 1.3 Formatting contract

Builder-latitude choices (2026-10-25 design §13) are settled as follows,
using the app's own design tokens (`docs/DESIGN.md`):

| Element | Value | Token |
|---|---|---|
| Header fill | `#293681`, white bold 11, centered, wrapped | `primary` |
| Header underline | medium bottom border `#1E2864` | `primary-dark` |
| Body/table borders | hairline (style 1) `#E2E6EB` all sides | `border-light` |
| Banding | even rule fill `#F4F6F8` on odd rows (`MOD(ROW(),2)=1`) | `surface-muted` |
| Best-attempt highlight | fill `#FFF2CC` (unchanged) | latitude |
| Notes/captions | italic `#687182` | `text-secondary` |
| Skor akhir emphasis | bold `#293681` on white, integer format | `primary` |
| `Menunggu` text | conditional font `#966000` | `warning` |
| `Selesai` / `Sudah dinilai` text | conditional font `#237A57` | `success` |
| `Belum ada nilai` text | conditional font `#6B7B85` | `inactive` |

- Gridlines are hidden on all four sheets; the tables carry their own
  borders.
- Banding, status colours, and the highlight are conditional formats
  (formula rules), so they survive grader edits the same way the highlight
  already does. Banding is a fill; status colours are fonts; the highlight
  keeps its own fill, so no rule conflicts: all matching rules apply.
- `Soal` keeps its prompt/image columns and the 96pt image rows; its body
  banding shows through behind floating images.

### 1.4 Column widths, row heights, alignment

- Numbers, dates, and status columns are centered; text columns keep left,
  top, wrapped alignment.
- `Peserta`: `No` 6, `Nama` 32, `Jenis` 10, `Kelas` 12, `Jurusan` 14,
  `Terdaftar` 18, `Percobaan` 12, `Rata-rata` 12, `Status nilai` 16.
- `Penilaian`: `No` 6, `Nama` 30, `Kelas` 12, `Percobaan` 12, `Dikirim` 18,
  `Status` 12, responses 28, scores 8, `Skor akhir` 12.
- `Soal` unchanged (6/18/50/30/45/14).
- `Penilaian` row heights adapt to the longest wrapped response text
  (24 chars/line estimate, 15pt per line, floor 24pt, cap 120pt); `Peserta`
  rows stay 24pt; `Soal` rows stay 96pt for embedded pictures.

### 1.5 Navigation

- `Peserta` gains an AutoFilter over its visible columns (latitude §13);
  `Penilaian` keeps its filter; the filter ranges exclude the hidden id
  columns.
- Freeze panes keep their anchors: `Peserta`/`Soal` at A2, `Penilaian`
  with its first six columns and header row frozen (G2), `Ringkasan`
  unfrozen (the active landing sheet).

## 2. Formula strings affected

Only the cells that referenced the moved id column change text:

```text
Penilaian!B  =IF($<id>2="","",INDEX(Peserta!$B$2:$B$<p+1>,MATCH($<id>2,Peserta!$J$2:$J$<p+1>,0)))
Penilaian!C  =IF($<id>2="","",INDEX(Peserta!$D$2:$D$<p+1>,MATCH($<id>2,Peserta!$J$2:$J$<p+1>,0)))
Peserta!G    =COUNTIFS(Penilaian!$<id>$2:$<id>$<a+1>,$J2)
Peserta!H    =IFERROR(AVERAGEIFS(Penilaian!$<final>$2:$<final>$<a+1>,Penilaian!$<id>$2:$<id>$<a+1>,$J2),"")
Helper!K..N  MATCH($J2,Peserta!$J$2:$J$$<p+1>,0)
Highlight    AND($<final>2<>"",$<final>2=IFERROR(AGGREGATE(14,6,$<final>$2:$<final>$<a+1>/($<id>$2:$<id>$<a+1>=$<id>2),1),""))
```

with `<id>` = `Peserta!J` / `Penilaian!` column 7+2q+1, `<final>` = column
7+2q, `<p+1>`/`<a+1>` per the 2026-10-25 design §6. The `S`/`N`/final-score
column positions, the header `S`/`N` formulas, and the final-score formula
(§7.1) are unchanged. The `No` insertion does not shift any other column
because the visible sequence per sheet already started at A.

## 3. Tests

The pins in `internal/export` are updated, not weakened: headers, row
numbers, hidden-column visibility, AutoFilter strings, the new formula
strings, banding + status conditional-format presence, and the existing
score-fidelity, injection, image, empty-quiz, and WIB-date coverage all
still run over the same fixtures. Everything else (route pin, HTTP
behaviour, read model) is untouched.

## 4. Out of scope (as before)

Print/page setup remains a non-goal (2026-10-25 design §3). No change to
the read model, the route, or the app screens.
