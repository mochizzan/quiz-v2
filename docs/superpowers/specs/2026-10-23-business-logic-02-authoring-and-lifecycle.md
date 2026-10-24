# Business Logic 02 — Authoring and Quiz Lifecycle

**Date:** 2026-10-23
**Status:** Draft for integration review; normative once the five-spec merge is accepted.
**Scope:** the teacher-owned quiz aggregate: creation, metadata and settings, questions and options, question images, publish, close, draft delete, the authoring read models, and `GET /uploads/:name`.
**Lens:** product integrity and authoring. The question this domain answers is: *can a teacher build exactly the quiz they intend, and can a student be harmed by a partially authored or silently changed quiz?*
**Rule-ID prefix:** `AUT-###`; domain invariants `AUT-INV-##`; edge cases `EC-AUT-##`; concurrency `CX-AUT-##`; cross-domain `XD-AUT-##`.
**Sources:** contract §§3–7 (binding rulings R-01–R-25, INV-01–INV-30, vocabulary, route ownership, required shape); schema design §§5.2–5.7, 6.6–6.8, 7–9.4 and migration `0003`; frontend data contract §§4.2–4.3, 5, 8.3, 12–13, 17–18, 22–24; OPS §§5–7 and OPS-021/022/051/056/057/061/062/100–104/114–119/123/126/140/141/150/161/164–166/212/216/252; PAR-030/038/042/057/121; ATT-012/026/102/114.
**Precedence:** the contract's §3 rulings are binding and are not re-litigated here. Where this spec refines a mechanism, the refinement is named as such and recorded in §12. No rule in this spec requires a schema delta. The spec contains no Go, SQL, template, or test code; field names are names, not declarations.

---

## 1. Intent and boundaries

**AUT-016 — What this domain decides.** AUT decides: the quiz shell (`title`, `description`, schedule, visibility, target pair, code, invite token); the lifecycle `draft → open → closed`; the draft-only editability of metadata, settings, questions, options, and images; the exact publish gate; close and draft-delete semantics; owner-only access to every authoring screen; the list/detail/editor read models; and the byte lifecycle of question images. A quiz is a teacher's authored artifact until publish, and a frozen assessment afterward.

**AUT-017 — What this domain delegates.** Identity, session validity, role classes, CSRF, and the must-change-password guard are IDN's (IDN-003/012/016/070). Code and invite resolution, audience eligibility, registration, and the zero-question join guard are PAR's (PAR-030/038/042/057). Attempt start, question ordering at attempt time, grading, results, recap, monitor, and stream are ATT's (ATT-012/026/102/114). Transaction, lock, error, file-write, soft-delete, and SSE mechanics are OPS's (OPS-100–104/123/212). AUT owns the authoring policy and calls those mechanics; it does not restate them.

**AUT-018 — Product stance.** The draft is the only editable state. Publish is a one-way commitment: after it, the question set, option set, correctness keys, image set, audience, and settings are frozen, because attempts and grades depend on them. Close is terminal and does not reopen. Draft delete is destructive but safe because a draft can never have attempt data. Images are content: a question with a broken image is not publishable, and a replaced image's old bytes are removed after commit.

**AUT-019 — Non-goals and deliberate omissions.** No autosave or collaborative editing; no version history, restore, undelete, duplicate-quiz, import, or bulk edit; no rich text, Markdown rendering, image crop/resize/compression, or multiple images per question; no co-teachers or transfer of ownership; no question bank; no per-field concurrency token. The only write that a GET may perform is none: every AUT GET is read-only, including `GET /uploads/:name`. A query parameter is never read as a presentation switch (AUT-040).

---

## 2. Actors, permissions, entry conditions

**AUT-020 — Owner resolution and the 404 policy.** Every route with `:id` or `:qid` first resolves one live quiz row whose `owner_account_id` is the authenticated actor. Missing, soft-deleted, and non-owned quizzes are indistinguishable: the standard 404 page with status 404, no flash, no field error, no redirect. For `:qid`, the question must also be live and belong to that quiz; a live question under another quiz is 404, not 403. Authorization is re-read inside every write transaction after the quiz lock (OPS-INV-04); the pre-read used to build a form is UX, not authority. `GET /uploads/:name` is the sole capability-public route: it has no actor and no ownership check (R-17, OPS-212).

**AUT-021 — Entry classes.** The authoring routes are IDN's `owner-teacher` class (IDN-016). An anonymous request is sent to login by IDN's normal guard; a student or a non-owner teacher gets 404; a pending teacher cannot hold a session; a `must_change_password=1` session is redirected to `/change-password` before the route class (IDN-070). A CSRF failure is 403 with the contract copy `Sesi formulir tidak valid. Muat ulang halaman.` (R-16/R-19). There is no 401 and no 405: an unregistered method falls to OPS's 404 catch-all (OPS-214).

| Route | Class | Entry condition |
|---|---|---|
| `GET /teacher/quizzes` | owner-teacher | active teacher; returns only the actor's live quizzes |
| `POST /teacher/quizzes` | owner-teacher | active teacher; CSRF |
| `GET /teacher/quizzes/new` | owner-teacher | active teacher |
| `GET /teacher/quizzes/:id` | owner-teacher | live quiz owned by actor |
| `POST /teacher/quizzes/:id` | owner-teacher | live quiz owned by actor; draft |
| `POST /teacher/quizzes/:id/publish` | owner-teacher | live quiz owned by actor; draft or idempotent conflict |
| `POST /teacher/quizzes/:id/close` | owner-teacher | live quiz owned by actor; open or idempotent conflict |
| `POST /teacher/quizzes/:id/delete` | owner-teacher | live quiz owned by actor; draft |
| `GET /teacher/quizzes/:id/questions/new` | owner-teacher | live quiz owned by actor; draft |
| `POST /teacher/quizzes/:id/questions` | owner-teacher | live quiz owned by actor; draft; CSRF; multipart |
| `GET /teacher/quizzes/:id/questions/:qid` | owner-teacher | live quiz owned by actor; live question in that quiz; draft |
| `POST /teacher/quizzes/:id/questions/:qid` | owner-teacher | same as GET; CSRF; multipart |
| `POST /teacher/quizzes/:id/questions/:qid/delete` | owner-teacher | same as GET; CSRF |
| `POST /teacher/quizzes/:id/questions/reorder` | owner-teacher | live quiz owned by actor; draft; CSRF |
| `GET /uploads/:name` | public capability | syntactically valid name; live question row; file present |

**AUT-022 — Status taxonomy for authoring.** Success is 200 (GET) or 303 (every POST, including state conflicts). Validation of submitted fields is 422 and re-renders the same template with submitted values and field errors. Missing, deleted, wrong-role, and non-owner are 404. CSRF is 403. A transport body over the route limit is 413. Integrity faults, exhausted retries, and database failures are 500. There is no 409 page and no 405 page. A POST never returns 200 with HTML and never writes a success flash before the commit succeeds.

**AUT-023 — Draft guard.** `draft` is the only state in which quiz metadata, settings, questions, options, images, reorder, and question delete may be written. A write that reaches a non-draft quiz after the lock writes nothing and returns 303 to the builder with `err-not-draft` (`Soal dan pengaturan hanya dapat diubah saat kuis masih draf.`). The question-editor GETs use the same guard with the same flash, because rendering an editable form for a frozen quiz would be a lie; missing/non-owned still wins with 404. Draft delete is the only legal delete, and it has its own copy (AUT-008).

**AUT-024 — CSRF and multipart order.** Every POST requires the signed double-submit token in the form body (R-16). On the two multipart question routes, the body must be parsed before the token can be read; the CSRF middleware therefore parses multipart for those routes only, under the same 16 MiB transport guard (AUT-034), and rejects a missing/mismatched token with 403 before any field validation or file write. A failed CSRF check never creates a temp file, never opens a transaction, and never leaks field values.

---

## 3. States and transitions

**AUT-025 — Quiz state model.** The only states are `draft`, `open`, `closed`; soft-deleted is not a state, it is invisibility. `published_at` is non-NULL for open and closed; `closed_at` is non-NULL iff closed (INV-09, OPS-021). A draft has both NULL. `state` never returns to an earlier value. Visibility and the target pair are chosen at creation and are immutable; they are not part of the state machine but they constrain publish and join.

| State | Authored content | Settings | Student access | Allowed transitions |
|---|---|---|---|---|
| `draft` | editable by owner | editable by owner | none (404; owner preview only) | → `open` on publish; → deleted on draft delete |
| `open` | frozen | frozen | join/registration/attempts per PAR/ATT | → `closed` on close |
| `closed` | frozen | frozen | no new join; existing results/review/attempts remain | none; terminal |
| deleted | invisible; files removed after commit | — | none | none; no restore |

**AUT-026 — Transition table.** Every row states its guard, effect, and conflict behavior.

| Transition | Guard | Effects | Conflict |
|---|---|---|---|
| create → draft | active teacher; valid create form; live major if private | insert quiz with defaults, code/token, `state='draft'`, `published_at=NULL`, `closed_at=NULL`, `updated_by=actor` | validation → 422; code/token retry exhausted → 500 |
| draft → open | owner; `PublishGate` true | `state='open'`, `published_at=T`, `closed_at=NULL`, `updated_by=actor` | gate false → 303 with the first reason; already open → 303 info `already-published`; closed → 303 `err-closed-terminal` |
| open → closed | owner | `state='closed'`, `closed_at=T`, `updated_by=actor`; code/token kept but inert | already closed → 303 info `closed`; draft → 303 `err-not-open` |
| draft → deleted | owner; no live registrations/attempts | stamp quiz root (actor), questions/options (NULL) with one `T_del`; unlink images after commit | non-draft → 303 `err-delete-not-draft`; already deleted → 404 |
| open/closed → deleted | forbidden | none | 303 `err-delete-not-draft` for an owned row; 404 for a deleted row |
| closed → open | forbidden | none | 303 `err-closed-terminal` |
| open/closed → draft | forbidden | none | 303 `err-not-draft` for content writes |

**AUT-027 — PublishGate.** One predicate, `PublishGate(quiz)`, is used by `CanPublish` on the detail read model and by the publish command. It returns the first failing reason in this fixed order; the order is the copy priority. The predicate is evaluated under the quiz lock for publish and from one read snapshot for the detail page.

| Priority | Condition | Failure key / copy |
|---|---|---|
| 1 | `state='draft'` | handled before the gate (idempotent/terminal conflicts) |
| 2 | at least one live question | `err-no-questions` / `Tambahkan minimal satu soal sebelum membuka kuis.` |
| 3 | private: both targets present, grade 10–12, live major | `err-targets` / `Lengkapi kelas dan jurusan sasaran sebelum membuka kuis.`; deleted major → `err-target-missing` / `Jurusan sasaran tidak tersedia.` |
| 4 | every live question structurally valid and positions dense 1..N | `err-invalid-questions` / `Perbaiki soal yang belum valid sebelum membuka kuis.` |
| 5 | every image row complete and its file present with the stored byte size | `err-image-missing` / `Satu atau lebih gambar soal tidak ditemukan. Unggah ulang gambar sebelum membuka kuis.` |
| 6 | settings valid, public code present and six digits, invite token non-empty | `err-invalid-settings` / `Pengaturan kuis tidak valid.`; missing public code/token is an integrity 500, not a user error |

**AUT-028 — Question and option state.** A question is absent, live, or soft-deleted. Live questions may be created and edited only in a draft; a live question's position changes only through reorder or delete-renumber; a live question becomes soft-deleted only in a draft. An option has no independent lifecycle or route: it exists only as part of its question's editor. Options are replaced as a set on edit (AUT-027/AUT-012). Essay questions have zero live options; objective questions have at least two (INV-11, OPS-022).

**AUT-029 — Image state.** A question has zero or one image. The row is all-or-none: path, MIME, bytes, and SHA-256 are all NULL or all non-NULL; `image_alt` is independent and may be NULL with or without an image. A file is present when a live row references it and the bytes match the stored size. Replacing an image moves `none → present` or `present → present` with a new name; removing moves `present → none`; deleting the question moves `present → none` after commit. Orphan files (present, unreferenced) are tolerated and never served (OPS-061/062/212). A live row with missing bytes is an integrity fault at publish and a 404 at serve.

**AUT-030 — Terminal states.** `closed` is terminal for the quiz; `deleted` is terminal for the aggregate; a soft-deleted question is terminal; a removed image name is never reused or resurrected. A closed quiz's code/invite token remain stored but resolve to `not_open` (R-11, OPS-115). No route reopens, restores, or undeletes. A draft delete frees its code and token for a future live row because the unique keys include `deleted_at` (INV-19, OPS-058).

---

## 4. Commands

Every command below is a complete rule. A write command names the lock set, the rows it writes, the postcondition, and the conflict result. A read command names its snapshot and derivation. `T` is the single transaction timestamp read once per transaction (OPS-103); `T_del` is the delete-event timestamp defined in AUT-031. All timestamps are stored UTC and displayed WIB (R-15). Field-error keys are the form names; `_form` is the page-level key. All copy is exact Indonesian and owned by AUT unless it cites R-19/OPS-216.

### 4.1 AUT-001 — `GET /teacher/quizzes`

- **Route and actor:** `GET /teacher/quizzes`; active teacher.
- **Authorization:** collection is owner-scoped. The query never returns another teacher's quiz, even school-wide teachers; counts and tabs are over the actor's live quizzes only.
- **Preconditions:** none. No CSRF on a GET.
- **Input:** `state` is `all|draft|open|closed`; absent or unknown normalises to `all`. `q` is trimmed, lower-cased, and matched as a substring of `title`; empty/whitespace is no filter; the first 100 runes are used and excess is ignored (a GET never returns 422 for a query parameter). `sort` is `newest|oldest|name_asc|name_desc`; absent/unknown normalises to `newest`. `page` is a positive integer; absent, non-numeric, or `<1` normalises to 1; a page beyond the last is clamped to the last.
- **Transaction and locking:** read-only, one snapshot for the page, total, and tab counts so they cannot disagree within the response (OPS-100). No locks.
- **Effects:** none.
- **Postconditions:** six cards per page (AUT-050). Tab counts are owner-scoped totals by state and are unaffected by `q` or the active tab. Cards show title, description, state, visibility/audience, schedule, question count, and code for public quizzes; the actions are `Kelola` (draft), `Monitor`/`Hasil` (open), `Hasil` (closed).
- **Errors and copy:** no domain error. A database failure is the 500 page. The empty states are exactly: no quizzes at all — `Belum ada kuis` / `Kuis yang Anda buat akan muncul di sini. Mulai dari satu draf, lalu tambahkan soal.`; filtered by `q` — `Tidak ada kuis yang cocok` / `Coba kata kunci lain atau hapus saringan yang aktif.`; filtered by tab — `Belum ada kuis di tab ini` / `Kuis dengan status ini akan muncul di sini.`
- **Status and redirect:** 200.
- **Audit and provenance:** none.
- **Idempotency:** safe and repeatable; refresh may show the same page.

### 4.2 AUT-002 — `POST /teacher/quizzes`

- **Route and actor:** `POST /teacher/quizzes`; active teacher; CSRF required.
- **Authorization:** the created row is owned by the actor; the actor cannot choose an owner.
- **Preconditions:** none; the create form is valid or the same form is re-rendered with 422. No quiz row exists before the transaction.
- **Input and validation:** `title`, `description`, `start_at`, `end_at`, `visibility`, `target_grade`, `target_major_id`. The server ignores any `state`, `code`, `invite_token`, `owner_account_id`, settings, or publish field supplied by the client.
  - `title`: required after trim and CRLF normalisation; 1–150 runes. Empty → `Judul kuis wajib diisi.`; over → `Judul kuis maksimal 150 karakter.`
  - `description`: optional; 0–1.000 runes after trim. Over → `Deskripsi maksimal 1.000 karakter.` Empty → `''`.
  - `start_at`, `end_at`: optional `datetime-local` values interpreted as WIB; optional seconds accepted and truncated to the minute; blank → NULL. Invalid → `Waktu mulai tidak valid.` / `Waktu selesai tidak valid.`. If both are present and `end_at <= start_at` → `Waktu selesai harus setelah waktu mulai.` Past dates are allowed; dates never gate state or access (R-15).
  - `visibility`: exactly `public` or `private`; missing/other → `Visibilitas tidak valid.`
  - Public: any submitted target values are ignored and both columns are written NULL. Private: `target_grade ∈ {10,11,12}`; missing → `Pilih kelas sasaran untuk kuis privat.`; value outside the set → `Kelas sasaran tidak valid.`. `target_major_id` required and must resolve to a live major → missing `Pilih jurusan sasaran untuk kuis privat.`; deleted/unknown `Jurusan yang dipilih tidak tersedia.`
- **Transaction and locking:** one transaction. Generate the public code (six crypto-random digits, leading zeros preserved) or NULL for private; generate a 128-bit invite token encoded as 22 unpadded base64url characters. For a private quiz, take a shared lock on the live major and revalidate it in the same transaction (OPS-150/PAR-121). Insert one `quizzes` row with `state='draft'`, `published_at=NULL`, `closed_at=NULL`, `updated_by_account_id=actor`, explicit settings defaults `allow_retake=0`, `max_attempts=1`, `show_score=1`, `review_mode='questions_only'`, `randomize_questions=0`, `randomize_options=0`, and the target/code pair required by INV-07. On a 1062 collision on code or invite token, roll back the insert, regenerate both values, and retry up to 10 times; a 10th collision is 500.
- **Effects:** exactly one live quiz row. No question, registration, attempt, or file is created.
- **Postconditions:** the quiz is a draft owned by the actor; public has both targets NULL and a six-digit code; private has both targets non-NULL and `code=NULL`; the invite token is non-empty and unique among live rows; `updated_by` is the actor.
- **Errors and copy:** the field errors above; code/token exhaustion is 500 with no row; a deleted major discovered under the lock is the `target_major_id` field error; database failure is 500. A double submit is not detected and may create two drafts (AUT-039).
- **Status and redirect:** success 303 to `/teacher/quizzes/:id?flash=quiz-created`; validation 422 re-renders `pages/teacher/quiz-new.html` with all submitted values except no file fields, the live major list, and the field errors.
- **Audit and provenance:** `owner_account_id=actor`, `updated_by_account_id=actor`; `created_at`/`updated_at` are database-managed. No separate audit row.
- **Idempotency:** not idempotent; there is no natural key for a quiz title. The accepted duplicate-draft behavior is recorded in OQ-AUT-08. PRG prevents the normal refresh duplicate.

### 4.3 AUT-003 — `GET /teacher/quizzes/new`

- **Route and actor:** `GET /teacher/quizzes/new`; active teacher.
- **Authorization:** owner-teacher class; no resource exists yet.
- **Preconditions:** none.
- **Input:** none. Form defaults come from the quiz row; a query parameter never changes them (AUT-040).
- **Transaction and locking:** read-only; one snapshot for the live major list.
- **Effects:** none.
- **Postconditions:** the form defaults are: empty title and description; no start/end; `visibility='public'`; no target grade/major; action `/teacher/quizzes`; cancel `/teacher/quizzes`; `IsEdit=false`; the major list is live majors ordered by name ascending then id ascending. Public is the inferred default because private reveals the target pair (R-11) and the current fixture uses public.
- **Errors and copy:** a major list that is empty renders the form with no major options; selecting private then fails server-side with `Pilih jurusan sasaran untuk kuis privat.` There is no 404/422 for this GET.
- **Status and redirect:** 200.
- **Audit and provenance:** none.
- **Idempotency:** safe.

### 4.4 AUT-004 — `GET /teacher/quizzes/:id`

- **Route and actor:** `GET /teacher/quizzes/:id`; active teacher.
- **Authorization:** live quiz owned by actor; anything else 404 (AUT-020). The 404 body is the same for missing and non-owned.
- **Preconditions:** none; no draft requirement to view.
- **Input:** `edit=1` is meaningful only in draft; it opens the metadata form. In open/closed it is ignored with no error. `flash=` is resolved by the closed key set (AUT-033); unknown keys render no flash.
- **Transaction and locking:** read-only; one snapshot for quiz, questions, option counts, settings, and image presence. `CanPublish` evaluates the same `PublishGate` as publish, including file `stat` checks; `CanClose` is `state='open'`; `Editable` is `state='draft'`.
- **Effects:** none.
- **Postconditions:** the builder shows title, description, teacher, state, visibility/audience, schedule, created date, question count, and the ordered question rows. Questions are ordered by `position` ascending, then `id` ascending. Each row shows position, type, prompt excerpt, option count, image presence, and the teacher-only correct-answer label; links are the edit and delete routes. The public code is always present from creation and is shown to the owner in every state; the invite URL is `/invite/<token>`. `PublishBlockedReason` is the first failing `PublishGate` reason (AUT-027). Settings are shown read-only when not draft.
- **Errors and copy:** missing/non-owned/deleted → 404. Database failure → 500. No 422.
- **Status and redirect:** 200; 404 for the ownership/missing case.
- **Audit and provenance:** none.
- **Idempotency:** safe.

### 4.5 AUT-005 — `POST /teacher/quizzes/:id`

- **Route and actor:** `POST /teacher/quizzes/:id`; active teacher; CSRF required.
- **Authorization:** live quiz owned by actor; else 404.
- **Preconditions:** `state='draft'` for any write. The route carries a hidden `form` discriminator: `meta` edits metadata, `settings` edits the six settings. A missing discriminator is treated as `settings` for compatibility with the current settings form; any other value is 422 with `_form` = `Formulir tidak valid. Muat ulang halaman.` and no write.
- **Input and validation — `form=meta`:** `title`, `description`, `start_at`, `end_at` with the same rules as AUT-002. `visibility`, targets, code, token, state, and settings fields are never read or written by this branch; a submitted `visibility` is ignored, not an error (R-11 immutability). No image field exists.
- **Input and validation — `form=settings`:** `randomize_questions`, `randomize_answers`, `allow_retake`, `max_attempts`, `review_mode`, `show_score`. Checkboxes are present with value `1` or absent; any other non-empty value for an AUT checkbox is 422 `Pengaturan tidak valid.` `randomize_answers` maps to the column `randomize_options`. `allow_retake=false` forces `max_attempts=1` and ignores the submitted number, including an invalid one; `allow_retake=true` accepts an integer 1–10, else `Batas percobaan harus antara 1 dan 10.` `review_mode` must be exactly `questions_and_key|key_only|questions_only|answers_only`, else `Tampilan hasil tidak valid.` `show_score` is a boolean. No partial update: any field error rejects the whole branch.
- **Transaction and locking:** one transaction; lock the quiz `FOR UPDATE`; re-read owner, live, and state. Apply the branch. If every submitted value equals the stored value, commit no update (idempotent repeat). Otherwise update only the branch's columns and set `updated_by_account_id=actor`. No other quiz column changes; in particular `state`, `published_at`, `closed_at`, code, token, visibility, and targets are untouched.
- **Effects:** metadata write changes title/description/schedule only; settings write changes the six settings only. No question, option, image, registration, or attempt is touched.
- **Postconditions:** the quiz remains draft with the same identity, code, token, visibility, and targets; the new values are stored; `updated_by` is the actor. `updated_at` changes only when a write occurred.
- **Errors and copy:** non-draft → 303 to the builder with `err-not-draft` (`Soal dan pengaturan hanya dapat diubah saat kuis masih draf.`); missing/non-owned → 404. Validation → 422, re-render the detail template with the metadata form open for `meta` (submitted raw date strings retained) or the settings form with submitted raw values, plus field errors. A 422 for `meta` does not persist a partial change; a 422 for `settings` does not persist a partial change.
- **Status and redirect:** success 303 to `/teacher/quizzes/:id?flash=saved`; validation 422; state conflict 303; missing 404.
- **Audit and provenance:** `updated_by_account_id=actor` on a real write; `updated_at` database-managed. No content history.
- **Idempotency:** repeating the same values is a logical no-op; concurrent saves serialise on the quiz lock and last committed wins (OPS-126). There is no version column or conflict warning.

### 4.6 AUT-006 — `POST /teacher/quizzes/:id/publish`

- **Route and actor:** `POST /teacher/quizzes/:id/publish`; active teacher; CSRF required.
- **Authorization:** live quiz owned by actor; else 404.
- **Preconditions:** no body fields. The command is a state transition, not a form; submitted title/settings values are ignored.
- **Input and validation:** `PublishGate` in the fixed priority of AUT-027. For the gate, the settings check covers: visibility/target XOR (INV-07), `max_attempts` 1–10 and `allow_retake` consistency, `review_mode` in the four values, booleans 0/1, public code present and exactly six digits, invite token non-empty. The question check covers: ≥1 live question; positions exactly 1..N with no gap/duplicate; every prompt 8–2.000 runes after trim; objective options 2–26 with bodies 1–500 runes; single exactly one correct; multiple at least two; essay zero live options; no option outside its question. The image check covers: row all-or-none; for each present image the file is a regular file under the uploads root, its size equals `image_bytes`, and its extension matches the stored MIME family.
- **Transaction and locking:** one transaction; lock the quiz `FOR UPDATE`; re-read owner/live/state. The lock serialises against every authoring write, so question/option rows cannot change while the gate reads them. Evaluate the gate; on failure roll back (no write) and 303 with the first reason. On success write `state='open'`, `published_at=T`, `closed_at=NULL`, `updated_by_account_id=actor`. No code is minted: R-11 creates it at quiz creation and open is the only state in which it resolves.
- **Effects:** one state transition and two timestamps; no question/option/file/attempt row changes. No SSE broadcast on publish.
- **Postconditions:** `state='open'`; `published_at` non-NULL; `closed_at` NULL; content and settings frozen by AUT-023; the public code and invite token become resolvable by PAR immediately after commit (R-11); every live question satisfies INV-11 and has a present image if it has image metadata.
- **Errors and copy:** zero questions → `err-no-questions`; private target failure → `err-targets` or `err-target-missing`; structural question failure → `err-invalid-questions`; missing/size-drifted image → `err-image-missing`; invalid settings → `err-invalid-settings`; already open → 303 with info `already-published` / `Kuis sudah dibuka.`; closed → 303 with `err-closed-terminal` / `Kuis yang sudah ditutup tidak dapat dibuka kembali.`; missing public code/token, a partial image row, or a database CHECK failure is 500 (integrity fault, OPS-215), never a 303 reason. Missing/non-owned → 404.
- **Status and redirect:** success 303 to `/teacher/quizzes/:id?flash=published`; gate failure 303 to the builder with the reason key; state conflicts 303; missing 404; integrity 500.
- **Audit and provenance:** `updated_by_account_id=actor`; `published_at=T`; `state` transition. No separate audit row and no file.
- **Idempotency:** a second publish after the first commits sees `open` and returns the informational flash without writing; a publish racing a close serialises on the quiz lock and the loser sees the winner's state (OPS-164). Publish is never retried automatically after a lock timeout or deadlock (OPS-102).

### 4.7 AUT-007 — `POST /teacher/quizzes/:id/close`

- **Route and actor:** `POST /teacher/quizzes/:id/close`; active teacher; CSRF required.
- **Authorization:** live quiz owned by actor; else 404.
- **Preconditions:** no body fields; state is `open` for a real transition.
- **Input and validation:** none beyond CSRF. A draft is not closable; a closed quiz is already terminal.
- **Transaction and locking:** one transaction; lock the quiz `FOR UPDATE`; re-read owner/live/state. `open` → write `state='closed'`, `closed_at=T`, `updated_by_account_id=actor`; `published_at` is untouched. The code and invite token remain stored; PAR's resolution state-gates them to `not_open` (R-11, OPS-115). No attempt, answer, registration, question, option, or file is touched.
- **Effects:** one state transition and `closed_at`; in-progress attempts remain answerable and submittable (R-10). After commit, broadcast `quiz.closed` and close the SSE subscribers (OPS-173); no HTTP write occurs inside the transaction.
- **Postconditions:** `state='closed'`; `closed_at` non-NULL; `published_at` unchanged; no new registration, cancellation, or attempt start is possible; results, recap, and history remain available; there is no reopen.
- **Errors and copy:** draft → 303 to the builder with `err-not-open` / `Kuis belum dibuka.`; already closed → 303 with the informational `closed` key, copy `Kuis ditutup.`; missing/non-owned → 404; database failure → 500.
- **Status and redirect:** 303 to `/teacher/quizzes/:id?flash=closed` for the real transition and for the already-closed idempotent case; 303 with `err-not-open` for draft; 404 for missing/non-owned.
- **Audit and provenance:** `updated_by_account_id=actor`; `closed_at=T`.
- **Idempotency:** double close is informational and writes nothing the second time; close racing publish serialises on the quiz lock, and publish-then-close is legal while close-then-publish refuses (OPS-164).

### 4.8 AUT-008 — `POST /teacher/quizzes/:id/delete`

- **Route and actor:** `POST /teacher/quizzes/:id/delete`; active teacher; CSRF required.
- **Authorization:** live quiz owned by actor; else 404. A non-owner teacher always gets 404, including after the owner deleted it.
- **Preconditions:** no body fields; `state='draft'` for a real delete. Draft is the only deletable state (R-10/OPS-021).
- **Input and validation:** none. The client cannot request a hard delete, a cascade override, or a file-only delete.
- **Transaction and locking:** one transaction; lock the quiz `FOR UPDATE`; re-read owner/live/state. Assert `NOT EXISTS` live registrations for the quiz and `NOT EXISTS` live attempts under any registration for it (OPS-056/OPS-121); a hit is 500, not a cascade. Collect every live question's `image_path` while the rows are readable. Choose one `T_del` (AUT-031) that avoids every live unique-key collision for this quiz's code, token, question positions, and option positions. Stamp the quiz root with `deleted_at=T_del` and `deleted_by_account_id=actor`; stamp every live question and option with the same `T_del` and NULL `deleted_by` (OPS-051/052). Commit.
- **Effects:** the quiz, all its live questions, and all their live options become invisible in one atomic cascade; no attempt data exists to delete. After commit, unlink each collected image file best effort under the uploads root (OPS-061); a failed unlink is logged as `upload_orphan` and tolerated. No file is unlinked before commit.
- **Postconditions:** no live quiz, question, or option remains; the code and invite token are freed for future live rows because the unique keys include `deleted_at` (INV-19/OPS-058); image rows are gone and their files are either removed or orphaned, never referenced. A second delete finds no live row → 404.
- **Errors and copy:** non-draft → 303 to the builder with `err-delete-not-draft` / `Hanya draf yang dapat dihapus.` (OPS-216); live registration/attempt assertion → 500; missing/non-owned/already-deleted → 404; unlink failure is not a user error.
- **Status and redirect:** success 303 to `/teacher/quizzes?flash=quiz-deleted`; non-draft 303; missing/deleted 404; integrity 500.
- **Audit and provenance:** quiz root `deleted_by_account_id=actor`; cascaded questions/options NULL; one `T_del` for the event (OPS-INV-02). No audit row.
- **Idempotency:** a repeat is a 404 with no second cascade; the commit is atomic, so a crash before commit leaves the quiz fully live, and a crash after commit leaves only possible orphan files (OPS-104). Delete never retries automatically.

### 4.9 AUT-009 — `GET /teacher/quizzes/:id/questions/new`

- **Route and actor:** `GET /teacher/quizzes/:id/questions/new`; active teacher.
- **Authorization:** live quiz owned by actor; else 404. The quiz must be `draft`; open/closed → 303 to the builder with `err-not-draft` and no form.
- **Preconditions:** live question count is below the authoring cap of 200; at the cap → 303 to the builder with `err-question-limit` / `Batas jumlah soal tercapai.`
- **Input:** none. The screen is read-only.
- **Transaction and locking:** read-only; one snapshot for the quiz and the live question count. No lock.
- **Effects:** none.
- **Postconditions:** a blank editor with type `single`, two blank option rows A and B, and **no preselected correct option**; `image_alt` empty; no image; position `count+1` for display only; action `/teacher/quizzes/:id/questions`; cancel `/teacher/quizzes/:id`; delete link empty. The no-preselection rule is a product-integrity call: a single-choice question must not accidentally ship with option A correct because the teacher never touched the key (OQ-AUT-03).
- **Errors and copy:** missing/non-owned → 404; non-draft → 303 `err-not-draft`; cap → 303 `err-question-limit`. Database failure → 500.
- **Status and redirect:** 200; 404; 303 for the two guards.
- **Audit and provenance:** none.
- **Idempotency:** safe.

### 4.10 AUT-010 — `POST /teacher/quizzes/:id/questions`

- **Route and actor:** `POST /teacher/quizzes/:id/questions`; active teacher; CSRF required.
- **Authorization:** live quiz owned by actor; else 404. Draft required; open/closed → 303 `err-not-draft`, no write.
- **Preconditions:** live question count `< 200` after the quiz lock; at the cap → 303 `err-question-limit`, no write. The route accepts `multipart/form-data` only; a non-multipart body is 422 `_form` = `Formulir tidak valid. Muat ulang halaman.` A body over 16 MiB is 413 `Badan permintaan terlalu besar.` (AUT-034).
- **Input and validation:** `type`, `body`, `image` (one optional file part), `image_alt`, `option_body[]`, `correct_index`, `correct_indexes[]`. `remove_image` is accepted but ignored on create.
  - `type`: exactly `single|multiple|essay`; else `Tipe soal tidak valid.`
  - `body`: required after trim and CRLF normalisation; 8–2.000 runes. Empty → `Teks soal wajib diisi.`; under → `Teks soal minimal 8 karakter.`; over → `Teks soal maksimal 2.000 karakter.`
  - `image_alt`: optional; 0–255 runes after trim; over → `Teks alternatif gambar maksimal 255 karakter.`. With no image it is ignored and stored NULL.
  - Options: read `option_body[]` in order; trim each; **drop trailing fully blank rows** (the dynamic form may submit unused rows) but keep an interior blank row so it can fail. After dropping, objective types require 2–26 options. Fewer than two → `Soal pilihan memerlukan minimal dua pilihan.`; more than 26 → `Pilihan jawaban maksimal 26.`; blank body at index i → `Pilihan {A} wajib diisi.`; over 500 runes → `Pilihan {A} maksimal 500 karakter.` `{A}` is the current `OptionLetter` (A, B, … Z) of the submitted position, not the option id.
  - `single`: exactly one `correct_index`, an integer index into the post-drop option list; missing/out-of-range or more than one distinct value → `Pilih tepat satu jawaban benar.`
  - `multiple`: `correct_indexes[]` deduplicated; at least two distinct valid indices → otherwise `Pilih minimal dua jawaban benar.`; any out-of-range value → `Kunci jawaban tidak valid.`
  - `essay`: option bodies and keys are ignored and not validated; the question is inserted with zero live options (INV-11). A stale objective key in a switched form is not an error.
  - Image: at most one file part; more than one → `Unggah satu gambar saja.`. Empty file → `Berkas gambar kosong.`; over 10.485.760 bytes → `Gambar maksimal 10 MB.`; sniffed type outside JPEG/PNG/WebP → `Gambar harus berupa JPEG, PNG, atau WebP.`; read failure → `Gambar tidak dapat dibaca.` The client filename, extension, and Content-Type are never trusted (OPS-123).
- **Transaction and locking:** validate every field and the image fully first; if any field error, return 422 without writing a file. If valid and an image is present, write it to a temp file in the uploads root, fsync, and atomically rename to its final random name (AUT-028/OPS-123). Then begin one transaction; lock the quiz `FOR UPDATE`; re-read owner/live/draft and the live question count. Re-check the cap. Insert the question at `position = count+1` with `updated_by_account_id=actor` and the image metadata (or all NULL); insert options at positions 1..M with `is_correct` from the submitted key. Commit. On any DB failure, roll back and unlink the new file best effort; the quiz is unchanged.
- **Effects:** one live question, its option set, and at most one new file. Quiz `updated_by` is **not** touched; the question row's `updated_by` is the actor. No attempt data exists for a draft.
- **Postconditions:** the question is live at the tail position; objective types satisfy INV-11; essay has no live options; positions remain dense 1..N because N was read under the quiz lock; the image row is all-or-none and points at a written file; the question belongs to this quiz.
- **Errors and copy:** field errors above; cap `err-question-limit` via 303; non-draft `err-not-draft`; missing/non-owned 404; body too large 413; disk write failure 500 with no row; DB failure 500 with no row and best-effort unlink; 422 keeps type, body, options, keys, and alt, but the browser cannot repopulate the file input, so the teacher must reselect the image.
- **Status and redirect:** success 303 to `/teacher/quizzes/:id?flash=question-saved`; validation 422 on `pages/teacher/question-editor.html`; non-draft/cap 303; missing 404; 413/500 as above.
- **Audit and provenance:** question `updated_by_account_id=actor`; `created_at` database-managed; image SHA-256 and byte count from the actual bytes. No quiz-level editor update and no content log.
- **Idempotency:** not idempotent; a double submit creates two questions at consecutive positions (AUT-039). A lost response followed by a retry is a duplicate; there is no client request id or natural key.

### 4.11 AUT-011 — `GET /teacher/quizzes/:id/questions/:qid`

- **Route and actor:** `GET /teacher/quizzes/:id/questions/:qid`; active teacher.
- **Authorization:** live quiz owned by actor; live question with `question.quiz_id = :id`; else 404. Draft required; open/closed → 303 `err-not-draft`.
- **Preconditions:** none beyond ownership/state.
- **Input:** none. The screen is read-only; no query parameter changes it.
- **Transaction and locking:** read-only; one snapshot for quiz, question, and live options. No lock.
- **Effects:** none.
- **Postconditions:** the editor is populated with type, body, image URL/alt/size, and options ordered by position then id. Correctness is mapped to option **indices**, never option ids, so a wholesale replacement cannot leak stale ids. The delete action is only rendered in draft. If the image row is partial or the file is missing, the editor still renders the metadata and shows the image URL; a broken preview is not a 404 for the editor.
- **Errors and copy:** missing quiz/question, cross-quiz question, non-owned → 404; non-draft → 303 `err-not-draft`; database failure → 500.
- **Status and redirect:** 200; 404; 303.
- **Audit and provenance:** none.
- **Idempotency:** safe.

### 4.12 AUT-012 — `POST /teacher/quizzes/:id/questions/:qid`

- **Route and actor:** `POST /teacher/quizzes/:id/questions/:qid`; active teacher; CSRF required.
- **Authorization:** live quiz owned by actor; live question in that quiz; else 404. Draft required; open/closed → 303 `err-not-draft`.
- **Preconditions:** multipart; 16 MiB transport guard; the question exists and is live under the quiz lock. No live `attempt_answers` may reference the question (OPS-056); a hit is 500.
- **Input and validation:** the AUT-010 field rules apply to `type`, `body`, `image`, `image_alt`, `option_body[]`, `correct_index`, and `correct_indexes[]`, plus `remove_image`. A new image and `remove_image=1` may both be present: the new file wins and `remove_image` is ignored. A new image without `image_alt` stores alt NULL; it does not inherit the old alt. With no new file and no `remove_image`, the existing image metadata is kept and `image_alt` is updated only if the field is present (blank clears to NULL). With no image at all, a submitted alt is ignored.
- **Transaction and locking:** validate all fields/image first; if valid and a new file is present, write the new file to its final random name before the transaction (AUT-028). Begin one transaction; lock the quiz `FOR UPDATE`, then the question `FOR UPDATE` (OPS-101). Re-read owner/live/draft and the question's live membership. Read the **old image path from the locked row**, never from the submitted form. Assert no live attempt answers. Update the question's `type`, `body`, `image_*`, `image_alt`, and `updated_by_account_id=actor`; `position` and `quiz_id` are unchanged. Replace options as a set: choose one `T_opt` (AUT-031), stamp every live old option with `deleted_at=T_opt`, then insert the new option rows at positions 1..M with the new keys. For essay, stamp all old options and insert none. Commit. On any DB failure, roll back, unlink the new file best effort, and leave the old row/file untouched. On success, unlink the old file after commit if it was replaced or removed (OPS-062).
- **Effects:** one question row update, a wholesale option-set replacement, and at most one file create/delete. The quiz row's `updated_by` is not touched. Old options are soft-deleted, never hard-deleted; no live `attempt_answers`/selection can reference them because the quiz is a draft.
- **Postconditions:** exactly one live option set for the question, positions 1..M dense; objective validity per INV-11; essay has zero options; image metadata all-or-none; the old image name is no longer referenced by any live row; the new file is referenced only by this question.
- **Errors and copy:** the AUT-010 field errors; state/missing/404/413/500 as there. If the quiz or question disappeared between the form render and the lock, 404. If the quiz was published/closed, 303 `err-not-draft`. If the option replacement finds a live attempt answer, 500 integrity fault, no write.
- **Status and redirect:** success 303 to `/teacher/quizzes/:id?flash=question-saved`; validation 422 on the editor with submitted values and alt (image input not repopulated); non-draft 303; missing 404; 413/500 as above.
- **Audit and provenance:** question `updated_by_account_id=actor`; option rows have only `created_at`/`deleted_at`; image SHA-256/bytes from the new bytes; old path unlinked after commit, failure logged as `upload_orphan`.
- **Idempotency:** if the normalised type/body/options/keys/image/alt are byte-identical to the live state, commit no write and 303 (idempotent retry). Otherwise the last committed edit wins whole-form; there is no per-field merge or version column. Two tabs editing the same question: the later commit overwrites the earlier one, including its option set and image choice; the earlier image may become an orphan, never a referenced-but-deleted file (CX-AUT-06).

### 4.13 AUT-013 — `POST /teacher/quizzes/:id/questions/:qid/delete`

- **Route and actor:** `POST /teacher/quizzes/:id/questions/:qid/delete`; active teacher; CSRF required.
- **Authorization:** live quiz owned by actor; live question in that quiz; else 404. Draft required; open/closed → 303 `err-not-draft`, no delete.
- **Preconditions:** no live `attempt_answers` for the question (OPS-056); a hit is 500. No body fields beyond CSRF.
- **Transaction and locking:** one transaction; lock the quiz `FOR UPDATE`, then the question `FOR UPDATE`. Re-read owner/live/draft and membership. Collect the question's `image_path`. Choose one `T_del` (AUT-031) that avoids collisions for the question's position and every option position under it. Stamp the question with `deleted_at=T_del` and `deleted_by_account_id=actor`; stamp every live option with `T_del` and NULL `deleted_by` (OPS-052). Renumber the remaining live questions to 1..N with the two-phase offset procedure (OPS-161/AUT-036); if the offset would exceed the `SMALLINT UNSIGNED` ceiling, abort 500 rather than corrupt order. Commit; unlink the image after commit best effort.
- **Effects:** one question and its options become invisible; remaining question positions become dense; one file may be removed or orphaned. No attempt data is touched. Quiz `updated_by` is not changed; `updated_at` may change via the renumber writes and is audit noise.
- **Postconditions:** the question is soft-deleted; no live option remains under it; positions of remaining live questions are exactly 1..N in the previous relative order; the image is no longer referenced by a live row; a repeat delete is 404.
- **Errors and copy:** missing/non-owned/already-deleted → 404; non-draft → 303 `err-not-draft`; live attempt answer → 500; offset overflow → 500; database failure → 500; unlink failure is logged and tolerated.
- **Status and redirect:** success 303 to `/teacher/quizzes/:id?flash=question-deleted`; non-draft 303; missing 404; 500 for integrity.
- **Audit and provenance:** question root `deleted_by_account_id=actor`; options NULL; one `T_del` for the event. No content log.
- **Idempotency:** a repeat finds no live question and returns 404; the commit is atomic. A delete racing an edit/reorder serialises on the quiz lock; the loser re-reads and aborts with 404 or `err-not-draft`, never a half-renumbered order.

### 4.14 AUT-014 — `POST /teacher/quizzes/:id/questions/reorder`

- **Route and actor:** `POST /teacher/quizzes/:id/questions/reorder`; active teacher; CSRF required.
- **Authorization:** live quiz owned by actor; else 404. Draft required; open/closed → 303 `err-not-draft`.
- **Preconditions:** no body fields beyond `order` and CSRF; the live question set is unchanged since the page render for a successful write.
- **Input and validation:** `order` is a comma-separated list of live question ids in the intended top-to-bottom order. Each token is trimmed and must be a positive integer; empty tokens, non-numeric tokens, duplicates, foreign ids, omissions, and extra ids all fail the exact-set check. There is no partial reorder and no 422: a mismatch is a stale/tampered order and returns 303 with `err-reorder` / `Daftar soal berubah. Muat ulang halaman.`
- **Transaction and locking:** one transaction; lock the quiz `FOR UPDATE`; then every live question of the quiz `FOR UPDATE` ordered by id (OPS-101). Re-read owner/live/draft. Build the live id set; compare its cardinality and membership with the submitted list. If the lists are identical to the current order, commit no write and 303 `reordered` (idempotent no-op). Otherwise write the new positions 1..N with the positive two-phase offset: first add an offset greater than `max(position)+N` to every live row, then write final 1..N (OPS-161/AUT-036). If the offset would exceed `SMALLINT UNSIGNED`, abort 500. Commit.
- **Effects:** only `position` values change; no question/option/image/quiz lifecycle field changes. Quiz and question `updated_by` are not touched; `updated_at` may move and is not the editor signal.
- **Postconditions:** live positions are dense 1..N and match the submitted order exactly; no negative, zero, temporary, or duplicate live position survives the commit; soft-deleted rows keep their old positions.
- **Errors and copy:** stale/malformed/foreign/duplicate set → rollback and 303 `err-reorder`; non-draft → 303 `err-not-draft`; missing/non-owned → 404; offset overflow or database failure → 500. No 422.
- **Status and redirect:** success 303 to `/teacher/quizzes/:id?flash=reordered`; mismatch 303 with `err-reorder`; non-draft 303; missing 404.
- **Audit and provenance:** none; reorder is presentation order, not content authorship. `updated_at` is audit noise.
- **Idempotency:** the same order is a logical no-op; a concurrent reorder of the same set serialises and the last committed order wins; a reorder against a changed set aborts with `err-reorder` (OPS-166). Two tabs reordering different valid orders both succeed in sequence, and the later one wins.

### 4.15 AUT-015 — `GET /uploads/:name`

- **Route and actor:** `GET /uploads/:name`; no actor; capability-public.
- **Authorization:** none beyond the unguessable 128-bit name. The route is not owner-scoped and does not require a session (R-17/OPS-212). It never lists, searches, or transforms.
- **Preconditions:** `name` matches exactly `^[a-f0-9]{32}\.(jpg|png|webp)$`; anything else, including uppercase hex, traversal, extra dots, directories, symlinks, and non-regular files, is 404. A `?v=` cache-buster query is ignored. No range requests and no directory listing.
- **Input:** path parameter only. `If-None-Match` is honoured as a conditional validator; `If-Modified-Since` is not.
- **Transaction and locking:** read-only; resolve one live question row whose `image_path = name`, joined to a live quiz; then read the file. No lock. No row → 404 (orphan). A row with a missing file, non-regular file, or size differing from `image_bytes` → 404 and an `upload_missing` log (OPS-061).
- **Effects:** none.
- **Postconditions:** 200 with the stored MIME (`image/jpeg|image/png|image/webp`), `Content-Length` from the actual file, a quoted `ETag` of the stored SHA-256 hex, `Last-Modified` from the file mtime in HTTP date format, `Cache-Control: private, no-cache, must-revalidate`, `X-Content-Type-Options: nosniff`, and `Content-Disposition: inline`. An `If-None-Match` equal to the ETag (or `*`) returns 304 with the ETag and cache headers and no body. The response is never cached publicly and is never served by the `/static` middleware.
- **Errors and copy:** 404 for malformed name, orphan, deleted question, deleted quiz, missing file, or size drift; no HTML body beyond the standard 404 page for real misses. A read error after headers closes the response; a read error before headers is 500. There is no 403/429 on this route.
- **Status and redirect:** 200 or 304; 404/500 otherwise.
- **Audit and provenance:** no access log of the name beyond the request line shape; `upload_missing` logs the name and question id, never image bytes. No CSRF, no cookie, no session write.
- **Idempotency:** safe; the same name and validators produce the same response. A file unlinked after a delete becomes a 404 on the next request, which is the intended tombstone.

### 4.16 Cross-cutting authoring rules (AUT-020–AUT-040)

**AUT-020 — Ownership and 404.** Defined in §2: every `:id`/`:qid` route resolves a live owned aggregate; missing, deleted, and non-owned are the same 404. A helper returns (quiz, question) only when both are live and related. No route trusts a hidden field, referrer, or list membership for authorization.

**AUT-021 — Entry classes.** Defined in §2: owner-teacher; IDN's authentication/must-change/CSRF guards run first. A pending teacher has no session; a student/non-owner gets 404; a must-change session is redirected before the route class.

**AUT-022 — Draft guard.** Defined in §2 and applied by every content command. Open/closed content GETs redirect to the builder; open/closed content POSTs write nothing and redirect with `err-not-draft`; publish/close/delete have their own state-specific outcomes.

**AUT-023 — Text normalisation and lengths.** All user text is normalised once at the boundary: CRLF/CR → LF, trim leading/trailing whitespace, strip NUL, preserve internal newlines/tabs. Lengths are Unicode code points, matching the utf8mb4 `VARCHAR` character limits. Title 1–150, description 0–1.000, prompt 8–2.000, option 1–500, alt 0–255, search 100. Empty-after-trim is empty, not a space. Templates escape `<`, `>`, `&`, and quotes; AUT never renders raw HTML from author text.

**AUT-024 — 422 rendering.** A validation failure returns status 422 and the same template, with all submitted non-file values, the live major list where relevant, and `FieldErrors` keyed by form name. `_form` carries page-level errors (malformed body, unknown discriminator, image count). Per-option keys are `option_body[0]`, `option_body[1]`, …; option letters in copy come from the submitted index. Raw invalid date/number strings are echoed exactly; a `LocalTime` zero value is not substituted. Passwords never appear here. A 422 creates no row, no file, and no flash.

**AUT-025 — PublishGate sharing.** The detail page's `CanPublish` and the publish command call the same predicate and the same reason order (AUT-027). The page may be slightly stale by the time the button is clicked; the command re-evaluates under the quiz lock and is authoritative. A button that is enabled but fails at publish produces a 303 with the reason, never a 500, for every user-fixable condition.

**AUT-026 — Question structural validity.** A question is structurally valid iff: type is one of three; prompt 8–2.000 runes; position is a positive integer and the set is dense 1..N; single has exactly one correct among 2–26 options; multiple has at least two correct among 2–26; essay has zero live options and no keys; every option body 1–500 runes; every correct index resolves to a live option of the same question. This predicate is used by the publisher and by no other writer to decide whether a write is allowed (an editor may save a temporarily invalid draft? **No**: the editor also rejects structural invalidity, so a live draft question is always structurally valid between writes). The only way a live question is invalid is manual data corruption, which publish reports as `err-invalid-questions` or 500 for impossible metadata.

**AUT-027 — Option-set normalisation.** The submitted option list is the authority for the new set. Trailing blank rows are dropped; interior blanks are errors; leading blanks are errors (A is a real option). Duplicate bodies are allowed (two options may legitimately read the same); duplicate correct indices are deduplicated; an index outside the list is invalid. Option letters are display-only and never stored. The maximum is 26 because the current formatter produces A–Z; a 27th option would render `[` and break the key column (OQ-AUT-04).

**AUT-028 — Image validation and write protocol.** Validate all non-file fields first, then the file. Sniff the first bytes; accept only JPEG/PNG/WebP; reject empty, over 10.485.760 bytes, unreadable, or multiple files. Generate 32 lowercase hex characters (128 random bits) plus an extension derived from the sniffed MIME (`.jpg`, `.png`, `.webp`). Write to a temp file in the uploads root, fsync, and rename atomically within the same filesystem (OPS-123). The DB stores only the name relative to the root, the sniffed MIME, the actual byte count, the SHA-256 of the bytes, and optional alt. The client filename, extension, and Content-Type are discarded. File creation precedes the referencing transaction; file deletion follows the commit (OPS-104).

**AUT-029 — Image metadata and alt.** The row is all-or-none (INV-24): path/MIME/bytes/SHA-256 are all NULL or all present; a partial row is a 500 integrity fault and is never rendered or published. `image_alt` is independent: it may be NULL with an image, and it is always NULL without one. Alt is author-supplied, trimmed, 0–255; it is not generated from the prompt or filename. Serving uses stored metadata, not a re-sniff on every request. No file is shared between questions; path equality is the reference test.

**AUT-030 — Image cleanup and orphan tolerance.** On replace: write new, commit the row, then unlink the old path. On remove: commit NULLs, then unlink. On question/quiz delete: collect paths under the lock, commit the soft deletes, then unlink. On DB failure: unlink the new staged file and keep the old row/file. A failed unlink is logged as `upload_orphan` and tolerated; an orphan is never served because no live row references it. A crash between commit and unlink leaves an orphan, never a broken row (OPS-061/062). Never unlink a path before the commit that stops referencing it.

**AUT-031 — Delete-event timestamp collision rule (domain refinement of OPS-051/103).** `deleted_at` is part of every authoring unique key, and DATETIME has second precision. A delete or option replacement that stamps rows with `T` can collide with an earlier soft-deleted row that shares the same key and the same second. Therefore choose `T_del = max(T, 1 second + max(deleted_at))` over **all rows that share any unique key with the rows being stamped**: for a quiz delete, the quiz code and invite token plus every live question position and option position; for a question delete, the question's position plus every live option position under it; for option replacement, every option position under the question. All rows in the event use the same `T_del`; no statement uses `NOW()`. If `T_del` exceeds the DATETIME maximum, abort 500. This keeps one timestamp per event (OPS-INV-02) and leaves a gap/future second that is harmless because `deleted_at` is an event marker, not a business clock.

**AUT-032 — Audit columns.** `quizzes.updated_by_account_id` is set at create, metadata save, settings save, publish, and close; it is **not** changed by question/option writes or reorder. `questions.updated_by_account_id` is set at question create/update; question delete sets `deleted_by_account_id=actor` on the question root and leaves options NULL. A quiz delete sets `deleted_by_account_id=actor` on the quiz root and leaves all cascaded questions/options NULL (OPS-052). `created_at`/`updated_at` are database-managed; `published_at`/`closed_at` use `T`; `deleted_at` uses `T_del`. There is no separate audit table and no content-version history.

**AUT-033 — Flash keys and exact copy.** Existing keys reused: `quiz-created` (success, `Draf kuis dibuat.` / `Tambahkan soal sebelum membuka kuis.`), `saved` (`Perubahan tersimpan.`), `question-saved` (`Soal tersimpan.`), `published` (`Kuis dibuka.`), `closed` (info, `Kuis ditutup.`), `reordered` (`Urutan soal diperbarui.`). New success keys: `quiz-deleted` (`Draf kuis dihapus.`), `question-deleted` (`Soal dihapus.`). New info: `already-published` (`Kuis sudah dibuka.`). New danger keys: `err-no-questions`, `err-invalid-questions`, `err-targets`, `err-target-missing`, `err-image-missing`, `err-invalid-settings`, `err-closed-terminal`, `err-not-open`, `err-not-draft`, `err-delete-not-draft`, `err-reorder`, `err-question-limit`, `err-form`; their copy is fixed in AUT-027 and the command that emits them. Unknown keys render nothing, never reflect the query. A flash is consumed by the redirect target's GET (IDN-029); it never substitutes for a 404/403/422 status.

**AUT-034 — Transport and body limits.** The two question POSTs accept at most 16 MiB of request body; a declared or streamed overflow is 413 with `Badan permintaan terlalu besar.` The business image limit remains 10 MiB and is a 422 field error, so a file between 10 MiB and 16 MiB is parsed and then rejected as too large, not as a transport error. Other authoring POSTs are urlencoded and inherit Fiber's 4 MiB default; they contain no file. The limit is route-scoped so a large upload cannot exhaust memory on unrelated routes.

**AUT-035 — Multipart/CSRF bridge.** The CSRF token is in the multipart body. The middleware must parse the multipart form for the two question routes before comparing the token, under the same 16 MiB guard; a missing/mismatched token is 403 before field validation or file staging. The parsed file part is held in memory or a temp file with a hard cap of 10 MiB; it is never written to the uploads root before the token and all non-file fields pass. A malformed multipart body is 422 `_form` (or 413 if the cap is hit), not 500.

**AUT-036 — Two-phase position rewrite.** The live unique keys `(quiz_id, position, deleted_at)` and `(question_id, position, deleted_at)` make a direct swap illegal. In one transaction: (1) add an offset greater than `max(position)+N` to every live row of the parent; (2) write the final dense 1..N positions. Use the same procedure after a question delete. Soft-deleted rows keep their old positions and never collide because `deleted_at` differs; never renumber a deleted row. If the offset would exceed the `SMALLINT UNSIGNED` ceiling (65.535), abort 500 rather than corrupt order. The cap of 200 questions keeps the offset headroom large (OQ-AUT-05).

**AUT-037 — Schedule parsing.** `start_at`/`end_at` are wall-clock WIB values from `datetime-local`, parsed as UTC+7 fixed (no DST), then stored UTC. Seconds are accepted and truncated to the minute; a value with an offset/Z is invalid because the input contract is local. Blank is NULL. If both are present, `end_at > start_at`; equality is 422 on `end_at` with `Waktu selesai harus setelah waktu mulai.` A past date is allowed. The schedule is display-only and never gates join, start, close, or delete (R-15).

**AUT-038 — Code and invite generation.** The public code is six crypto-random digits, leading zeros preserved, compared as a string, never parsed as an int. The invite token is 128 crypto-random bits encoded as 22 unpadded base64url characters, stored in the 32-character ascii column. Both are generated at create, unique among live rows, and never regenerated by publish, close, or edit. On a 1062 duplicate, regenerate and retry up to 10 times, then 500; two concurrent creates may collide and the loser retries with fresh values (OPS-140/141). Private quizzes store `code=NULL`; a code submitted for a private row is ignored. A closed quiz keeps both values; PAR state-gates them to `not_open` (R-11).

**AUT-039 — Idempotency and double submit.** Natural keys and state predicates are the idempotency store; there is no request-id table (OPS-143). Publish/close are idempotent informational; delete's second request is 404; reorder with the same order is a no-op; metadata/settings with identical values is a no-op. Create and question create are **not** idempotent: a double submit can create two drafts or two questions. PRG prevents the normal refresh duplicate; the accepted residual case is recorded in OQ-AUT-08. No command retries automatically after a lock timeout/deadlock except where OPS-102 declares it idempotent; none of the file-write, publish, close, or delete flows retries.

**AUT-040 — Integrity faults and logging.** A 500 is returned for: a live public quiz with NULL code, a partial image row, a live attempt/answer under a draft, a position set that is not dense, a duplicate live option position, an offset overflow, or a database CHECK failure that validation should have prevented. Log one structured line with the rule/invariant ID and row ids, never titles, prompts, alt text, image bytes, or teacher names (OPS-219). The application has no preview switch: `?state=` is never read and a query parameter can never alter a form default, a state transition, or a persisted row.

---

## 5. Queries and read models

**AUT-050 — List query.** `GET /teacher/quizzes` reads only live quizzes with `owner_account_id = actor`. The state tab is one of `all|draft|open|closed` and filters `state` exactly; an absent/unknown tab is `all`. `q` is trimmed, lower-cased, and matched as a case-insensitive substring of `title` only; description, teacher name, and code are not searched. Tabs are `Semua`, `Draf`, `Dibuka`, `Ditutup`; their counts are owner-scoped totals by state and ignore `q` and the active tab, so a tab never lies about how many quizzes exist. A deleted quiz is absent from every count and page.

**AUT-051 — Sort, pagination, and empty states.** Sort keys and tie-breaks are fixed: `newest` = `created_at DESC, id DESC`; `oldest` = `created_at ASC, id ASC`; `name_asc` = lower(title) ASC, id ASC; `name_desc` = lower(title) DESC, id DESC. `page` is 1-based, six cards per page; a page below 1 is 1 and a page above the last is clamped to the last, so a URL never renders an empty page when rows exist. Pagination links preserve `q`, `state` (only when not `all`), and `sort` (only when not `newest`). The three empty states and their copy are fixed in AUT-001. A filtered-empty page never falls back to all rows.

**AUT-052 — Detail read model.** The detail page is built from one snapshot: the quiz row, its live questions ordered by `position, id`, each question's live option count, image presence, and the six settings. `Editable = state='draft'`. `CanPublish` = `PublishGate` (AUT-027); the blocked reason is the first failing reason and uses the same key as publish. `CanClose = state='open'`. `QuestionCount` counts live questions only. `Code` is non-NULL exactly for public quizzes and is shown to the owner in every state; the invite URL is derived from the stored token. The question rows expose the teacher-only `CorrectLabel` (e.g. `A` or `A, C`), never the student's answer, score, or another teacher's data. The page never renders a hidden or soft-deleted question.

**AUT-053 — Question-editor read model.** The editor is built from the quiz and one live question in that quiz. Options are ordered by `position, id`; correctness is mapped to indices; the image URL is `/uploads/<name>`, with alt and a formatted size. The `IsNew` form has no id, no delete action, and no preselected correct option. The edit form has the question id, the delete action, and the stored values. The page never exposes option ids as the correctness value and never includes a soft-deleted option. A missing image file still renders the metadata; the preview is broken, the editor is not 404.

**AUT-054 — Uploads read model.** The uploads route resolves one live question by `image_path` and a live quiz, then serves the file bytes. It does not consume a quiz state: a draft image (known only by its capability URL), an open image, and a closed image are all served while the question/quiz are live, because results and review may still need them (R-10/R-17). A deleted question or quiz makes the name 404 even if the file remains. The route never exposes a directory, a listing, a thumbnail, a transformed image, or a range.

**AUT-055 — Derived display values.** State labels, visibility labels, type labels, review-mode labels, option letters, `LocalTime` formatting, `Score`, and `FileSize` come from the shared view-model formatters; AUT does not re-implement them. `Position` is the stored 1-based position, not a loop index. `QuestionCount` is `len(live questions)`, never a stored counter. `CanPublish` is never computed from a template condition. The code is rendered with tabular numerals and preserved leading zeros; the invite token is rendered as a URL, never truncated.

---

## 6. Invariants owned

| ID | Invariant | Enforced by | Rules |
|---|---|---|---|
| AUT-INV-01 | Only the owner teacher can read or write a quiz's authoring surface; a non-owner gets 404, never 403 or a redacted page. | app ownership join + one helper | AUT-020, AUT-021 |
| AUT-INV-02 | Quiz lifecycle is exactly `draft → open → closed`; `published_at` is set for open/closed, `closed_at` iff closed; no reopen. | app transition guards + DB `chk_quizzes_lifecycle` (INV-09) | AUT-025, AUT-026, AUT-006, AUT-007 |
| AUT-INV-03 | Visibility and the target pair are immutable after create; public has no targets and a code, private has both targets and no code. | app write list + DB `chk_quizzes_target`/`chk_quizzes_code_visibility` (INV-07) | AUT-002, AUT-005, AUT-038 |
| AUT-INV-04 | A public code is six digits and unique among live quizzes; an invite token is non-empty and unique among live quizzes; both are created once and never rewritten. | DB unique keys + app generation/retry (INV-08) | AUT-002, AUT-038 |
| AUT-INV-05 | Questions, options, metadata, settings, images, and reorder are writable only while the quiz is a draft. | app state guard inside the quiz lock (INV-10) | AUT-022, every content command |
| AUT-INV-06 | A quiz can be opened only when `PublishGate` is true; the detail page and the command use the same predicate and reason order. | app shared predicate | AUT-025, AUT-027, AUT-006 |
| AUT-INV-07 | Live question positions are dense 1..N and unique per quiz; live option positions are dense 1..M and unique per question; reorder/delete preserve this. | DB unique keys + app two-phase rewrite | AUT-014, AUT-013, AUT-036 |
| AUT-INV-08 | Every live objective question has 2–26 options and the required key cardinality; every live essay has zero options; every live question has a valid prompt. | app validation on every write + publish gate (INV-11) | AUT-023, AUT-026, AUT-027, AUT-010, AUT-012 |
| AUT-INV-09 | A question has zero or one image; path/MIME/bytes/SHA-256 are all-or-none; the file exists for a published question; no file is shared. | DB `chk_questions_image` + app file protocol (INV-24) | AUT-028–AUT-030, AUT-015 |
| AUT-INV-10 | A draft delete stamps the quiz, all questions, and all options with one `T_del`; no live child remains under a deleted parent; no attempt data is deleted. | app one transaction + `OPS-056` assertions (INV-18) | AUT-008, AUT-031 |
| AUT-INV-11 | A deleted quiz frees its code/token; a deleted question frees its position; re-creation inserts a new row and never revives one. | DB unique keys including `deleted_at` (INV-19) | AUT-008, AUT-013, AUT-031 |
| AUT-INV-12 | Draft delete never touches registration/attempt/answer/selection rows; a live registration/attempt under a draft is an integrity 500. | app assertion inside the delete transaction | AUT-008 |
| AUT-INV-13 | `updated_by` tracks quiz content/settings/publish/close, question writes track the question editor, and cascaded rows leave `deleted_by` NULL. | app write list | AUT-032 |
| AUT-INV-14 | Uploads are served only for a live question of a live quiz, with private no-cache validators, and never list or transform. | app lookup + OPS-212 headers | AUT-015, AUT-054 |
| AUT-INV-15 | All stored timestamps are UTC; all display is WIB; schedule values never gate access or state. | DSN/connection + app formatting (INV-20) | AUT-037, AUT-055 |

---

## 7. Edge cases and failure modes

**EC-AUT-01 — Create form is stale after the major is deleted.** The private create POST takes the shared lock, sees no live major, and re-renders 422 with `Jurusan yang dipilih tidak tersedia.` on `target_major_id`; no quiz row is written. A public create ignores targets and succeeds even if the form carried a stale major.

**EC-AUT-02 — Detail page is stale after a delete.** A refresh of `/teacher/quizzes/:id` after the draft delete returns 404; a `POST` from the stale page returns 404 for publish/close/delete/settings because the quiz is no longer live. No flash is shown, because there is no live resource to show it on.

**EC-AUT-03 — Edit form is stale after publish/close.** The GET editor redirects to the builder with `err-not-draft`; a POST editor, reorder, or question delete from the stale tab re-reads the state under the lock and returns 303 `err-not-draft` without a write. The teacher's typed content is lost, which is the accepted cost of a stale form; the quiz is frozen and cannot be silently changed.

**EC-AUT-04 — Question editor is stale after the question is deleted.** The GET returns 404 because the question is no longer live. A POST edit/delete returns 404. The builder reload shows the remaining dense positions.

**EC-AUT-05 — Publish with zero live questions.** The gate fails at priority 2 and returns 303 `err-no-questions`; no state/timestamp changes. The button is disabled on the detail page for the same reason. A stale button click is safe.

**EC-AUT-06 — Publish with invalid questions.** A manually corrupted question (one option, two correct singles, an essay with options, a blank prompt, a gap in positions) fails at priority 4 with `err-invalid-questions`. The quiz stays draft; the builder shows the offending rows by position. There is no auto-fix.

**EC-AUT-07 — Publish with a missing image file.** The gate stats each present image and fails at priority 5 with `err-image-missing`; the quiz stays draft. The teacher must re-upload the image (which creates a new name) or remove it. A DB row with partial image metadata is a 500 integrity fault, not this message.

**EC-AUT-08 — Publish with an invalid private target.** A deleted target major fails at priority 3 with `err-target-missing`; a missing grade/major (possible only by manual data) fails with `err-targets`. Both leave the quiz draft. The create/edit forms never write a partial private pair (AUT-INV-03).

**EC-AUT-09 — Double publish or publish after close.** The first commit opens the quiz; the second sees `open` and returns info `Kuis sudah dibuka.` or sees a concurrent close and refuses with `Kuis yang sudah ditutup tidak dapat dibuka kembali.` The final state is whatever the serialised commits produced, never a mixed timestamp pair.

**EC-AUT-10 — Close a draft or a closed quiz.** Draft → 303 `err-not-open` / `Kuis belum dibuka.`; closed → 303 info `Kuis ditutup.`. Neither writes a timestamp. A stale close button on a draft is harmless.

**EC-AUT-11 — Delete an open/closed quiz.** The delete re-reads the state under the lock and returns 303 `err-delete-not-draft` / `Hanya draf yang dapat dihapus.` No cascade, no file unlink. Only a draft can be deleted.

**EC-AUT-12 — Double delete.** The first commit stamps the aggregate; the second request finds no live quiz and returns 404. The 404 may appear once after a double-click that beat PRG. No second cascade and no second unlink run.

**EC-AUT-13 — Two tabs reorder a changed set.** Tab A opens with questions 1,2,3; tab B deletes question 2; tab A posts `1,2,3`. The reorder locks the quiz, sees the live set `1,3`, fails the exact-set check, rolls back, and 303s with `Daftar soal berubah. Muat ulang halaman.` No positions change.

**EC-AUT-14 — Two tabs reorder the same set differently.** Both submit a valid permutation of the same live set. They serialise on the quiz lock; the later commit wins. The earlier tab's page is stale until reload; no error is shown, because both requests were valid. This is the expected drag-and-drop semantic (OPS-166).

**EC-AUT-15 — Question edit races question delete or quiz delete.** Both lock the quiz first. If delete wins, the edit's current read sees no live question/quiz and returns 404; if edit wins, the delete sees the new content and deletes it. No live child is written under a deleted parent. The same ordering protects reorder and publish.

**EC-AUT-16 — New image plus `remove_image=1`.** The new file wins; the checkbox is ignored. The row points at the new name, the old file is unlinked after commit, and the new alt is stored (or NULL if absent). A client that meant to remove but also attached a file gets the safer, non-destructive interpretation.

**EC-AUT-17 — Image replacement fails in the DB.** The new file was written before the transaction. The transaction rolls back; the old row still points at the old file; the new file is unlinked best effort. If the unlink fails, it is an orphan, never referenced. A retry writes a fresh name.

**EC-AUT-18 — Process dies after the file write or after the commit.** Before commit: the staged file is an orphan; the DB is unchanged. After commit but before unlink: the old file is an orphan; the DB points at the new file. Neither case leaves a live row pointing at a missing file. Orphans are tolerated and never served (OPS-104).

**EC-AUT-19 — Uploads requested for a deleted question, deleted quiz, missing file, or orphan name.** All return 404. A name that never existed, an uppercase/traversal name, a directory, and a symlink are also 404. A file whose size no longer matches the stored byte count is 404 + `upload_missing`; it is never served partially.

**EC-AUT-20 — Oversized or malformed multipart.** A body over 16 MiB is 413 before field validation. A file over 10 MiB but under 16 MiB is parsed and rejected as 422 `Gambar maksimal 10 MB.` A malformed multipart, multiple files in the one file field, or a missing boundary is 422 `_form` `Formulir tidak valid. Muat ulang halaman.` No file is written for any of these. A browser that cannot repopulate the file input after a 422 requires the teacher to reselect the image.

---

## 8. Concurrency and idempotency

**CX-AUT-01 — The quiz row is the authoring mutex.** Every authoring write locks the quiz `FOR UPDATE` first, then questions/options in ascending id order (OPS-101). Publish, close, settings, metadata, question create/update/delete, reorder, and quiz delete therefore serialise on one row. A transaction that starts after a competing commit re-reads the current state and either applies its write to the new state or returns the conflict. No authoring write relies on a snapshot read for authorization or state.

**CX-AUT-02 — Code/token collision.** Two concurrent creates generate values independently; the unique keys decide. The loser gets 1062, regenerates both values, and retries up to 10 times. A loser never reuses the winner's value and never commits a partial row. Exhaustion is 500, and the teacher can retry the form; no duplicate quiz is created by the failed transaction.

**CX-AUT-03 — Publish versus content edit/reorder.** Publish holds the quiz lock; edits/reorders queue. If publish commits first, the edit re-reads `open` and returns `err-not-draft`; if the edit commits first, publish's gate sees the new content. A publish can never freeze a half-applied option replacement or a half-written reorder, because both are one transaction under the same lock.

**CX-AUT-04 — Publish versus close.** Both lock the quiz. Publish-then-close is legal and ends closed; close-then-publish sees closed and returns `err-closed-terminal`; double publish sees open and returns info; double close sees closed and returns info. The final row always satisfies AUT-INV-02/OPS-164.

**CX-AUT-05 — Delete versus any child write.** Quiz delete locks the quiz first, so it serialises with every child write. If delete wins, the child write's current read fails and returns 404 (missing quiz/question); if the child wins, delete cascades the new row. A child write never creates a live row under a deleted quiz because it revalidates the live parent chain inside its transaction (OPS-INV-01/OPS-178).

**CX-AUT-06 — Concurrent image replacement.** Both edits write their new files before locking, then serialise on the quiz/question lock. The second reads the old path **from the locked row**, not from its form, so it unlinks the first edit's file only after replacing it; the first edit's file may become an orphan if the second had already read the pre-first path. The final row points at exactly one existing file; no live row points at a file another transaction unlinked.

**CX-AUT-07 — Image delete versus serve.** A serve may read a row just before the delete commits and then fail to open the file after the unlink, returning 404; that is the intended tombstone. The serve never reads a row after the delete commit because the row is no longer live. No lock is taken on the serve path, and a transient 404 on an image that was just deleted is acceptable.

**CX-AUT-08 — Double publish/close.** The state predicate under the lock is the compare-and-set. The winner writes the transition; the loser sees the new state and returns the informational flash. No second timestamp is written, and no SSE event is emitted twice for one close.

**CX-AUT-09 — Double delete.** The first commit stamps all rows; the second finds no live quiz and returns 404. If two deletes race before either commits, the quiz lock serialises them; the loser's current read sees the deletion and returns 404. The cascade runs once; file unlinks are best effort and idempotent at the path level.

**CX-AUT-10 — Reorder serialisation.** Two reorders of the same set serialise and the last commit wins. A reorder against a changed set aborts before writing any position. The two-phase offset is inside the same transaction, so no other transaction can observe the temporary offset positions; the committed positions are dense 1..N (OPS-161).

**CX-AUT-11 — Double create / double question create.** There is no natural key, so both commits succeed and create duplicates at consecutive positions (questions) or two drafts (quizzes). The question lock makes the positions dense, not unique by content. The UI should disable the submit button on submit, but the server does not deduplicate (OQ-AUT-08).

**CX-AUT-12 — Lock timeouts and retries.** A 1205/1213 aborts the transaction. AUT retries **none** of its writes automatically: publish, close, delete, image writes, reorder, settings, metadata, and question writes are not in OPS-102's idempotent retry list. The handler returns 500 and the teacher retries manually from a reloaded page. Read pages have no lock and no retry. A retry after a lost response is covered by the state predicates (publish/close/delete/reorder) or is a documented duplicate (create/question create).

---

## 9. Cross-domain contracts

**XD-AUT-01 — Identity and access (IDN).** AUT requires: an authenticated active teacher from IDN's `CurrentUser`; the `owner-teacher` route class; the must-change guard before the route class; the CSRF check on every POST with the form-body token; the 404 policy for wrong role; and a 403 for CSRF only. AUT guarantees: it never authenticates by itself, never reads a session row, never issues a CSRF token, and never returns 403 for a non-owned quiz. A student or non-owner probing an authoring URL learns nothing: 404 (IDN-012/016).

**XD-AUT-02 — Code and invite resolution (PAR).** AUT guarantees that a public open quiz has a stored six-digit code and that a private quiz stores NULL; every quiz has a stored invite token; a draft/closed code or token resolves to `not_open`, never to a join; a deleted quiz's values no longer resolve. AUT requires PAR-030 (code lookup), PAR-038/042 (invite resolution/audience), and PAR-057 (zero-question join guard) to read the current row and state-gate before any registration. AUT never generates a code at publish and never nulls a code at close (R-11/OPS-115).

**XD-AUT-03 — Majors and audience (PAR).** AUT requires a live major under a shared lock when it writes `target_major_id` (OPS-150/PAR-121) and refuses to write a target it cannot validate. AUT guarantees it never deletes a major, never cascades into majors, and that a private target pair is both non-NULL and within grade 10–12. PAR's major delete refuses while a live quiz targets the major, including drafts (PAR-121). The audience label on the detail/list is derived from visibility + grade + current major name; AUT stores only the id.

**XD-AUT-04 — Attempts, grading, and results (ATT).** AUT guarantees: a draft can never have an attempt (no join, no start); a publish freezes questions, options, keys, images, and settings; a close freezes new joins but not in-progress answers; a closed quiz is never reopened; a question delete never touches attempt data (only drafts, with an assertion); a deleted draft never had attempt data. ATT requires ATT-026 (state `open`/`closed` for attempts; draft 404), ATT-114 (content immutability after draft), and ATT-102 (`EffectiveMaxAttempts` reads `allow_retake`/`max_attempts` and never trusts a form). AUT requires ATT to read the quiz row inside its transaction and to treat a draft-with-attempt as a 500/404 corruption guard, not a normal state.

**XD-AUT-05 — Operations (OPS).** AUT delegates: one transaction per write; the lock order `quizzes → questions → options`; `T` once per transaction; the two-phase position rewrite (OPS-161); the file write protocol (OPS-123); post-commit unlink/orphan tolerance (OPS-061/062); the uploads headers/validators (OPS-212); the 404/500 pages and error mapping (OPS-214/215/217); SSE broadcast after close (OPS-173). AUT supplies the domain copy and honors OPS-216: `Hanya draf yang dapat dihapus.` and `Daftar soal berubah. Muat ulang halaman.` are reused verbatim.

**XD-AUT-06 — Guarantees AUT provides.** (a) A live quiz has a legal state/timestamp pair. (b) A public quiz has a six-digit code; a private quiz has both targets and no code. (c) A published quiz has ≥1 structurally valid question, dense positions, and present images. (d) No content/settings write occurs after publish. (e) Draft delete leaves no live child and no attempt data. (f) Question/option positions are dense. (g) Every uploads URL resolves to a live question's file or 404. (h) The list/detail/editor never leak another teacher's quiz. (i) The authoring cap is 200 questions/26 options per question.

**XD-AUT-07 — Requirements AUT places on others.** IDN must give AUT the actor id and role class, and must not let a must-change session reach authoring. PAR must not create a registration for a draft/closed/zero-question quiz and must resolve a closed code as `not_open`. ATT must not start an attempt on a draft and must treat content as immutable after draft. OPS must provide the uploads root, atomic rename, post-commit unlink, 404 for missing files, and the one-timestamp cascade. No other domain may write `quizzes`, `questions`, `answer_options`, or their image columns.

---

## 10. Traceability

| Owned route | Command | Invariants | Acceptance |
|---|---|---|---|
| `GET /teacher/quizzes` | AUT-001 | AUT-INV-01/02 | AC-01–AC-04, AC-55 |
| `POST /teacher/quizzes` | AUT-002 | AUT-INV-03/04 | AC-05–AC-12 |
| `GET /teacher/quizzes/new` | AUT-003 | AUT-INV-03 | AC-13 |
| `GET /teacher/quizzes/:id` | AUT-004 | AUT-INV-01/02/06 | AC-14–AC-16, AC-55 |
| `POST /teacher/quizzes/:id` | AUT-005 | AUT-INV-03/05/13 | AC-17–AC-21 |
| `POST /teacher/quizzes/:id/publish` | AUT-006 | AUT-INV-02/06/08/09 | AC-22–AC-28 |
| `POST /teacher/quizzes/:id/close` | AUT-007 | AUT-INV-02 | AC-29–AC-30 |
| `POST /teacher/quizzes/:id/delete` | AUT-008 | AUT-INV-10/11/12 | AC-31–AC-35 |
| `GET /teacher/quizzes/:id/questions/new` | AUT-009 | AUT-INV-05/08 | AC-36 |
| `POST /teacher/quizzes/:id/questions` | AUT-010 | AUT-INV-05/07/08/09 | AC-37–AC-42 |
| `GET /teacher/quizzes/:id/questions/:qid` | AUT-011 | AUT-INV-05/08 | AC-43 |
| `POST /teacher/quizzes/:id/questions/:qid` | AUT-012 | AUT-INV-05/07/08/09 | AC-44–AC-47 |
| `POST /teacher/quizzes/:id/questions/:qid/delete` | AUT-013 | AUT-INV-07/10/11 | AC-48 |
| `POST /teacher/quizzes/:id/questions/reorder` | AUT-014 | AUT-INV-07 | AC-49–AC-51 |
| `GET /uploads/:name` | AUT-015 | AUT-INV-09/14 | AC-52–AC-54 |

**Cross-cutting coverage.** AUT-020/021 → AC-55/56; AUT-022 → AC-21/46; AUT-023 → AC-07/39; AUT-024 → AC-19/39; AUT-025 → AC-22–AC-28; AUT-026 → AC-25/37; AUT-027 → AC-39; AUT-028 → AC-40/41; AUT-029 → AC-40/45; AUT-030 → AC-31/48; AUT-031 → AC-35; AUT-032 → AC-59; AUT-033 → AC-28/30/33; AUT-034 → AC-57; AUT-035 → AC-56; AUT-036 → AC-49/50; AUT-037 → AC-08; AUT-038 → AC-11; AUT-039 → AC-12/28/51; AUT-040 → AC-58–AC-60. Every route in contract §6's AUT row appears exactly once above. The route surface is 73 method+path pairs: the 72 currently pinned plus `GET /uploads/:name`; `/healthz` remains OPS's operational route and is not counted here.

---

## 11. Open questions and schema deltas

| ID | Question / conflict | Recommended default | Impact |
|---|---|---|---|
| OQ-AUT-01 | The current builder's “Ubah data kuis” link is a self-link and there is no metadata form. | Add `?edit=1` on the detail GET and a hidden `form=meta` branch on `POST /teacher/quizzes/:id`; keep `form=settings`/missing for the existing settings form. | View-model/template/route extension; no schema. |
| OQ-AUT-02 | R-11 makes visibility immutable; a wrong target major cannot be fixed without deleting the draft and losing its questions. | Allow **target** edits in draft while visibility, code, and token remain immutable; INV-07 still holds. If rejected, keep delete/recreate as the only correction. | Adds two fields to the metadata form; no schema; a contract clarification, not a weakening of R-11. |
| OQ-AUT-03 | The fixture preselects option A as correct on a new single-choice question; this spec does not. | No preselection: a new question must not ship with an accidental key. The fixture is a demo, not a rule. | Fixture/template change; no schema. |
| OQ-AUT-04 | `OptionLetter` renders `[` after Z, so a 27th option breaks the key column. | Cap the authored set at 26 now; later render two-letter labels (AA, AB) and lift the cap. | Copy/validation cap; no schema. |
| OQ-AUT-05 | No product cap exists for questions/options; the two-phase offset and builder layout need one. | 200 live questions/quiz and 26 live options/question; reject create at the cap with `Batas jumlah soal tercapai.` | New validation/copy; no schema; offset headroom stays large. |
| OQ-AUT-06 | Contract §6 assigns `GET /uploads/:name` to AUT, while IDN-016 lists it as OPS's public route. | Contract wins: AUT owns the authoring behavior; OPS owns the headers/error family; IDN's class is `public`. | Ownership bookkeeping only; no schema. |
| OQ-AUT-07 | `deleted_at` is DATETIME (second precision) and part of every unique key; a same-second re-delete of a reused code/token/position can collide. | Adopt AUT-031's one-second bump `T_del = max(T, 1 + max(deleted_at))` over the colliding key family; abort 500 past DATETIME max. | Refines OPS-051/103; no schema. |
| OQ-AUT-08 | Create and question create have no natural key, so a double submit creates duplicates. | Accept; disable the submit button client-side; no request-id table. Document the duplicate as a user-visible consequence, not a 500. | UX only; no schema. |
| OQ-AUT-09 | `UPLOADS_DIR` is absent from config, compose, and the Dockerfile; the image spec needs a writable root. | Add `UPLOADS_DIR` (default `./uploads`), create it at boot, mount a named volume at `/app/uploads` in compose, and ensure the app user can write it. | Deployment/config; no schema. |
| OQ-AUT-10 | `GET /uploads/:name` is not in the current 72-pair conformance pin. | Add it deliberately: 73 pairs (72 + uploads); `/healthz` remains OPS's 74th audited pair only in the OPS matrix. | Test pin; no schema. |
| OQ-AUT-11 | Templates use title `maxlength=120`, option `maxlength=300`, no description max, no alt field, and copy “Kode dibuat saat kuis dibuka.” even though the code exists at creation. | Update to title 150, option 500, description 1.000, add the alt input, and render the existing public code from creation with “Kode aktif setelah kuis dibuka.” | UI only; no schema. |
| OQ-AUT-12 | `quizzes.updated_by_account_id` is not touched by question/option writes; a teacher may expect the quiz's “last edited by” to move. | Keep AUT-032: the question row tracks its own editor; the quiz editor tracks quiz-level content/settings/lifecycle. | Semantics only; no schema. |
| OQ-AUT-13 | Fiber's default 4 MiB body limit is below the 10 MiB image limit. | Route-scope 16 MiB on the two question POSTs; map >16 MiB to 413 and 10–16 MiB to the 422 image error (AUT-034). | Server config; no schema. |

**Schema deltas: none.** Every rule fits migration `0003`: `quizzes` has the lifecycle, target, code, token, settings, image, and audit columns; `questions` has position, image metadata, alt, and the all-or-none check; `answer_options` has the position/body/key. No new table, column, index, CHECK, or migration is required. Three DB checks are intentionally looser than the app and must not be “fixed” by app assumptions: `chk_quizzes_attempts` allows 0 but AUT never writes it; `chk_quizzes_lifecycle` permits a draft with `published_at` but AUT never writes that pair; `chk_questions_image` does not constrain `image_alt` and cannot express “essay has no options”, so AUT enforces both.

**Objection recorded, not re-litigated.** OQ-AUT-02 is the only place this spec disagrees with a §3 ruling's product consequence (R-11's immutability of the target pair). The ruling stands until the contract changes; the spec implements delete/recreate as the correction path.

<!-- AUT-APPEND -->
