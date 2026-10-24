# Frontend Data Contract — Design

> Status: superseded in part. The fixture world this document was written for
> (`internal/httpserver/fixtures`, `APP_DEMO`, `?state=` previews, the demo
> strip) has been removed: every screen now reads MariaDB through its domain
> package. The screen inventory, markup, and chrome rules below remain the
> reference; ignore the fixture/`APP_DEMO` mechanics in §2 and §19.
>
> Date: 2026-10-23
> Supersedes, where contradicted: `2026-10-20-quiz-system-design.md` §3–§9,
> §15.7 page inventory, and the route list in `AGENTS.md`.
> Visual design authority is unchanged: `docs/DESIGN.md`.

---

## 1. Intent and why

The frontend currently renders in isolation: 29 static routes, one
`PageData{Active}` field, no SQL, and no POST routes. Every number, name, and
state on screen is a literal. When the backend arrives, each template would
have to be reverse-engineered into a data shape, and every page would drift
differently.

This document fixes that by making one thing true everywhere:

> **A screen is a pure projection of one typed view model. If a value is
> visible, it is in the model; if it is in the model, the backend can supply
> it; and nothing that is not in the model appears on screen.**

It also records the product decisions taken in the 2026-10-23 interview, which
change several rules from the 2026-10-20 spec: registration as a first-class
state, public/private access, invitation URLs, review modes, retakes, and the
removal of the question bank.

## 2. Scope

**In scope**

- Screen inventory: routes, layouts, navigation entries.
- Per-screen data requirements, including empty, waiting, error, and locked states.
- Shared vocabulary: enums, labels, score/date/number formatting.
- Form contracts: field names, validation, CSRF, post/redirect/get behaviour.
- The view-model shapes a Go handler must build.
- The contracts for live-monitor events and attempt beacons.
- A conformance test plan that keeps the contract honest.

**Out of scope**

- Table definitions, indexes, migrations, SQL.
- Handler algorithms, transactions, auth cryptography, image processing.
- Visual design, CSS, component styling (`docs/DESIGN.md` remains authoritative).
- Implementation order (that belongs to the plan that follows this document).

"UI-only" in this document means the boundary is specified, not the machinery
behind it.

### 2.1 Build phase (UI-only, no persistence)

The first build implements every screen against hardcoded fixtures. It never
writes to a database and never validates on the server.

- **Reads:** every GET renders a fixture view model. Numbers, names, and
  states are the ones in §20 and are internally consistent.
- **Writes:** every POST performs no write. It redirects (303) to the screen
  the real backend would land on, with a flash. The route sequence mirrors §8
  exactly, so wiring storage later changes handlers, not navigation.
- **Validation:** HTML5 attributes plus `app.js` handle required fields,
  patterns, lengths, password confirmation, date order, the private
  grade/major pair, and correct-answer rules. The server repeats none of it in
  this phase; §18's 422 path is exercised only by fixtures.
- **CSRF:** every form renders a fixed placeholder `csrf_token` so the markup
  is final. It is not verified in this phase.
- **Live monitor:** renders a static snapshot; the SSE stream and beacon
  endpoints are defined in §19 but deferred to the backend phase.
- **Data:** fixtures are compiled in and selectable under `APP_DEMO=1`
  (§20). No fixture is inlined in a template.

## 3. Goals, non-goals, anti-goals

**Goals**

1. Every screen has exactly one view model, defined here.
2. One word per concept. No screen invents a state or a label that is not in §5.
3. All locale-sensitive formatting happens in Go before the template sees it.
4. Every page can be rendered from a fixture before the backend exists.
5. Contradictions between the old spec and the current UI are resolved explicitly.

**Non-goals**

- Choosing an ORM, a migration tool, or a session library.
- Redesigning the visual system.
- Making the UI work without JavaScript for live updates (the monitor is the
  one deliberate exception; see §19).

**Anti-goals**

- A template printing a number, date, or status it did not receive.
- Two representations of the same state on two screens.
- A display value the model cannot produce (pass rate without a pass mark,
  countdown without a duration, question bank without a bank).
- `map[string]any` passed to a template.

## 4. Decisions taken in the interview (binding)

These override the 2026-10-20 spec wherever they differ.

### 4.1 Accounts and authentication

1. **One login page for both roles.** No role switcher, no role selector.
   The server resolves the role from the credentials.
2. **Registration forms differ by role.**
   - Teacher: full name, email, password.
   - Student: full name, email, password, grade level, major.
3. **Students are active immediately. Teachers are not.** A teacher account
   waits for another active teacher's approval. The pending page reports that
   status for a signed, single-use token.
4. **Password reset is teacher-issued.** A teacher generates a one-time reset
   token from a student's row and hands it over; the reset page accepts that
   token and a new password. There is no email delivery.
5. **Teachers may view other teachers but not edit, deactivate, or delete
   them.** Their only write is approving a pending registration.
6. **Teachers may view, edit, delete, and reassign students** (grade level and
   major). Majors and anonymous participants keep the access defined in the
   old spec: create and rename majors; view, edit, and delete anonymous
   participants.

### 4.2 Quiz lifecycle and visibility

7. **Three states:** `draft`, `open`, `closed`. Draft → open → closed. Closed
   is terminal.
8. **Start and end times are visual only.** They never open, close, or gate a
   quiz. The teacher opens and closes manually.
9. **A quiz is editable only while it is a draft.** Opening it locks questions
   and settings permanently.
10. **Every quiz has an invitation URL. Only public quizzes have a usable
    code.**
11. **Public quiz:** visible to every logged-in student regardless of grade or
    major; joinable by code; open to anonymous participants by code.
12. **Private quiz:** visible and joinable only by logged-in students whose
    profile grade level **and** major match the teacher's selection. It has no
    code and does not admit anonymous participants.
13. **Invitation URLs auto-register.** A valid URL registers the student
    immediately and lands on the quiz intro. A private URL opened by a
    non-matching account is rejected with an explanation.
14. The grade + major selector is part of the **teacher's** create form, not a
    student-side discovery form.
15. **Live monitor is a separate page** and only shows quizzes that are
    currently open ("on air").

### 4.3 Authoring

16. **Two steps.** Step 1: title, description, start and end time, visibility;
    choosing private reveals grade level and major. Step 2: questions.
17. **No question bank.** Remove the bank tile, the nav item, and any route
    that implies one.
18. **Unlimited questions and answer options.**
19. **Question types:** single-choice, multiple-choice, essay. No true/false,
    no short-answer.
20. **One optional image per question.** No gallery, no multiple images.
21. **Correct answers are marked inline** for objective questions. Essays are
    graded manually.
22. **Questions reorder by drag and drop.**
23. **Randomization:** question order and answer-option order, each per
    student, each independently toggled.
24. **Retakes:** allowed up to a teacher-set maximum. Every attempt is stored;
    all attempts carry equal weight. There is no "counted" attempt.
25. **Review modes** (post-submission): questions + key, key only, questions
    only, student answers without verdicts. Plus an independent switch for
    revealing the final score.

### 4.4 Participation

26. **Three participation states:** registered, in progress (on air), completed.
27. **Registration is self-service.** The Register button creates a
    registration; the student then chooses Start quiz or Cancel registration.
28. **Completed means submitted**, whether or not essays are graded.
29. **History groups by quiz**, one row per quiz, with its attempts expandable;
    not one row per attempt.
30. **A valid invitation URL registers automatically**, without the Register
    button.
31. **Anonymous retakes are allowed and identified by full name** on the same
    quiz. Private quizzes admit no anonymous participants at all.

### 4.5 Dashboards

31. **Student dashboard:** counts for assigned, completed, and best score, plus
    the latest available quizzes.
32. **Teacher dashboard:** top scorer per quiz, most recently created quizzes,
    and the quizzes with the highest and lowest participant counts.

## 5. Vocabulary

One word per concept, and one label per word. Storage remains varchar plus
application validation (old spec §11); this section is the closed list.

### 5.1 Enums

| Type | Values | Shown as |
|---|---|---|
| `Role` | `teacher`, `student`, `anonymous` | Guru, Siswa, Peserta |
| `AccountStatus` | `pending`, `active` | Menunggu persetujuan, Aktif |
| `QuizState` | `draft`, `open`, `closed` | Draf, Dibuka, Ditutup |
| `Visibility` | `public`, `private` | Publik, Privat |
| `QuestionType` | `single`, `multiple`, `essay` | Pilihan tunggal, Pilihan ganda, Esai |
| `ParticipationState` | `registered`, `in_progress`, `completed` | Terdaftar, Sedang berlangsung, Selesai |
| `AttemptState` | `in_progress`, `completed` | Sedang berlangsung, Selesai |
| `ReviewMode` | `questions_and_key`, `key_only`, `questions_only`, `answers_only` | see §5.2 |
| `EssayState` | `ungraded`, `graded` | Menunggu penilaian, Sudah dinilai |
| `JoinError` | `unknown_code`, `not_open`, `wrong_audience`, `already_registered`, `anonymous_not_allowed` | see §5.3 |
| `SortOrder` | `newest`, `oldest`, `name_asc`, `name_desc` | Terbaru, Terlama, Nama A–Z, Nama Z–A |

Rules:

- Storage values are lowercase snake case, never displayed.
- Labels live in Go (`Label()` methods). Templates never map a raw value.
- Unknown values render the raw value in a visible "unknown" style and fail
  the conformance test rather than being silently treated as a default.

### 5.2 Review mode labels

| Value | Label | Content |
|---|---|---|
| `questions_and_key` | Soal dan kunci jawaban | Every question, the student's answer, and the correct answer |
| `key_only` | Hanya kunci jawaban | The correct answer list, without question bodies |
| `questions_only` | Hanya soal | Question bodies and the student's answers, no key, no verdict |
| `answers_only` | Jawaban siswa tanpa penilaian | Question bodies and the student's answers, no key, no verdict color |

`answers_only` is visually distinguished from `questions_only` by suppressing
verdict badges and score marks; both are content-identical otherwise. This is
a deliberate duplication inherited from the product decision; a builder may
collapse them only with the user's agreement.

`show_score` is a separate boolean. When false it hides the score on the
result page, in history, and in the student's "best score" tile. A hidden
score is rendered as `—`, never as `0`.

### 5.3 Join rejection copy

| Value | Message |
|---|---|
| `unknown_code` | Kode kuis tidak ditemukan. |
| `not_open` | Kuis ini belum dibuka atau sudah ditutup. |
| `wrong_audience` | Kuis ini hanya untuk kelas dan jurusan tertentu. |
| `already_registered` | Anda sudah terdaftar di kuis ini. |
| `anonymous_not_allowed` | Kuis privat hanya dapat diikuti oleh siswa yang masuk. |

## 6. Formatting contract

Templates receive finished strings. No template performs arithmetic, date
formatting, pluralization, or number grouping.

### 6.1 Scores

- The backend stores scores on a 0–1 scale. **The view model exposes whole
  percentages.**
- `Score` is an integer 0–100, displayed as `84%`. It never appears as `0.84`.
- Averages (`Average`) display one decimal with an Indonesian comma: `86,2%`.
- Rounding is half-up at the view-model boundary. `84.5` → `85%`.
- An absent score is `nil`, rendered `—`. Zero is a real score, rendered `0%`.
- History, result, recap, and dashboard all use the same formatter.
- Essay input is 0–100 in the form and stored as 0–1 by the handler; the UI
  never sees the stored form.

> This supersedes old spec §8's "templates multiply by 100". The loader has no
> `FuncMap`, and the contract above puts the conversion where it is testable.

### 6.2 Dates and times

- Server stores UTC; display is WIB (UTC+7), fixed offset, no tzdata dependency.
- `Short`: `20 Okt 2026`.
- `Clock`: `09.42 WIB`.
- `Full`: `Rabu, 21 Oktober 2026, 09.42 WIB`.
- Form input: `datetime-local`, value `2026-10-21T09:42`, interpreted as WIB by
  the handler.
- No relative dates ("Besok") in v1; they are unstable in fixtures and tests.
- An unset optional date renders `—`.

### 6.3 Numbers and names

- Thousands separator `.`; decimal separator `,`.
- `Average` uses one decimal; counts and percentages use no decimals.
- Class label: roman grade + major, e.g. `XII IPA`. No rombel/section number in
  v1 (the data model has no section field).
- Initials: first letter of the first two words, uppercased, max 2 characters.
- File sizes: `2,4 MB`.
- Text is never truncated in the model; truncation is CSS only.

## 7. Shared view-model types

Package: `internal/viewmodel` holds shared types, formatters, and every
per-page struct. `internal/httpserver/fixtures` builds demo models from
`internal/viewmodel`; `internal/httpserver` routes import both. There is no
import cycle: `viewmodel` imports nothing from `httpserver`.

```go
type PageData struct {
    Base
    Data any // a typed per-page struct, never a map
}

type Base struct {
    Title  string      // already localized, e.g. "Dasbor"
    Active string      // nav key, e.g. "dasbor"
    User   *CurrentUser
    CSRF   string
    Flash  *Flash
    Query  QueryState  // filters/search/page for list screens
}

type CurrentUser struct {
    ID         int64
    Role       Role
    FullName   string
    Initials   string
    Email      string
    ClassLabel string // students only, e.g. "XII IPA"
    Subject    string // removed in v1; always empty (see §23)
}

type Flash struct {
    Kind  string // success | error | info
    Title string
    Body  string
}

type FieldErrors map[string]string // field name -> Indonesian message

type QueryState struct {
    Q       string
    Filter  string
    Sort    SortOrder
    Page    int
    PerPage int
}

type List[T any] struct {
    Items   []T
    Total   int
    Page    int
    PerPage int
}

func (l List[T]) Pages() int
func (l List[T]) HasPrev() bool
func (l List[T]) HasNext() bool
```

Helpers every list screen uses: `WithPage`, `Offset`, `From`, `To`, `Pages`.
The template renders pagination from these values; it never computes them.

## 8. Route and screen map

Routes carry identity in the path. The current static routes cannot express
"which quiz" or "which attempt" and are replaced. This is a deliberate break
from the 29-route conformance fixture; the test is updated in the same change.

### 8.1 Public and auth

| Method | Path | Screen | Template |
|---|---|---|---|
| GET | `/` | redirect to `/login` or the role home | — |
| GET | `/login` | Login | `pages/auth/login.html` |
| POST | `/login` | Authenticate | — |
| GET | `/register/teacher` | Teacher registration | `pages/auth/register-teacher.html` |
| POST | `/register/teacher` | Create pending teacher | — |
| GET | `/register/student` | Student registration | `pages/auth/register-student.html` |
| POST | `/register/student` | Create active student | — |
| GET | `/reset-password` | Token + new password | `pages/auth/reset-password.html` |
| POST | `/reset-password` | Consume token | — |
| GET | `/change-password` | Change password | `pages/auth/change-password.html` |
| POST | `/change-password` | Update password | — |
| GET | `/auth/pending` | Teacher pending status | `pages/auth/pending.html` |
| GET | `/join` | Code entry | `pages/auth/join.html` |
| POST | `/join` | Resolve code | — |
| GET | `/invite/:token` | Invitation landing | `pages/auth/invite.html` |
| GET | `/join/identify` | Anonymous identity form | `pages/auth/identify.html` |
| POST | `/join/identify` | Create anonymous participant | — |
| POST | `/logout` | End session | — |

### 8.2 Student

| Method | Path | Screen | Template |
|---|---|---|---|
| GET | `/student` | Dashboard | `pages/student/dashboard.html` |
| GET | `/student/history` | History | `pages/student/history.html` |
| GET | `/student/history/:quizID` | Attempts for one quiz | `pages/student/history-detail.html` |
| GET | `/profile` | Profile | `pages/profile.html` |
| POST | `/profile` | Update name/email | — |
| GET | `/quiz/:id` | Quiz intro / registration state | `pages/student/quiz.html` |
| POST | `/quiz/:id/register` | Register | — |
| POST | `/quiz/:id/cancel` | Cancel registration | — |
| POST | `/quiz/:id/attempts` | Start (or retry) | — |
| GET | `/attempt/:id` | Question page (current question) | `pages/attempt/question.html` |
| POST | `/attempt/:id/answer` | Save answer and navigate | — |
| GET | `/attempt/:id/review` | Pre-submit review | `pages/attempt/review.html` |
| POST | `/attempt/:id/submit` | Submit | — |
| GET | `/attempt/:id/result` | Post-submit result | `pages/attempt/result.html` |
| POST | `/attempt/:id/beacon` | Progress + focus-loss beacon | — |

### 8.3 Teacher

| Method | Path | Screen | Template |
|---|---|---|---|
| GET | `/teacher` | Dashboard | `pages/teacher/dashboard.html` |
| GET | `/teacher/quizzes` | Quiz list | `pages/teacher/quizzes.html` |
| GET | `/teacher/quizzes/new` | Create step 1 | `pages/teacher/quiz-new.html` |
| POST | `/teacher/quizzes` | Create draft, redirect to builder | — |
| GET | `/teacher/quizzes/:id` | Quiz detail (questions, settings, code, publish) | `pages/teacher/quiz-detail.html` |
| POST | `/teacher/quizzes/:id` | Save settings | — |
| POST | `/teacher/quizzes/:id/publish` | Draft → open | — |
| POST | `/teacher/quizzes/:id/close` | Open → closed | — |
| POST | `/teacher/quizzes/:id/delete` | Delete a draft | — |
| GET | `/teacher/quizzes/:id/results` | All attempts for the quiz | `pages/teacher/results.html` |
| GET | `/teacher/quizzes/:id/questions/new` | New question | `pages/teacher/question-editor.html` |
| POST | `/teacher/quizzes/:id/questions` | Create question | — |
| GET | `/teacher/quizzes/:id/questions/:qid` | Edit question | `pages/teacher/question-editor.html` |
| POST | `/teacher/quizzes/:id/questions/:qid` | Save question | — |
| POST | `/teacher/quizzes/:id/questions/:qid/delete` | Delete question | — |
| POST | `/teacher/quizzes/:id/questions/reorder` | Drag result | — |
| GET | `/teacher/quizzes/:id/monitor` | Live monitor | `pages/teacher/monitor.html` |
| GET | `/teacher/quizzes/:id/stream` | SSE stream | — |
| GET | `/teacher/grading` | Grading queue | `pages/teacher/grading.html` |
| GET | `/teacher/grading/:answerID` | Grade one essay | `pages/teacher/grading.html` |
| POST | `/teacher/grading/:answerID` | Save essay score + feedback | — |
| GET | `/teacher/recap/:attemptID` | Attempt recap | `pages/teacher/recap.html` |
| GET | `/teacher/students` | Students list | `pages/teacher/students.html` |
| GET | `/teacher/students/:id` | Edit student | `pages/teacher/student-edit.html` |
| POST | `/teacher/students/:id` | Save student | — |
| POST | `/teacher/students/:id/delete` | Delete student | — |
| POST | `/teacher/students/:id/reset-token` | Issue password reset token | — |
| GET | `/teacher/anonymous` | Anonymous participants | `pages/teacher/anonymous.html` |
| GET | `/teacher/anonymous/:id` | Edit anonymous participant | `pages/teacher/anonymous-edit.html` |
| POST | `/teacher/anonymous/:id` | Save anonymous participant | — |
| POST | `/teacher/anonymous/:id/delete` | Delete anonymous participant | — |
| GET | `/teacher/majors` | Majors | `pages/teacher/majors.html` |
| POST | `/teacher/majors` | Create major | — |
| POST | `/teacher/majors/:id` | Rename major | — |
| POST | `/teacher/majors/:id/delete` | Delete unused major | — |
| GET | `/teacher/teachers` | Teachers + approvals | `pages/teacher/teachers.html` |
| POST | `/teacher/teachers/:id/approve` | Approve pending teacher | — |
| GET | `/error/404` | Not found | `pages/error/404.html` |
| GET | `/error/500` | Server error | `pages/error/500.html` |

### 8.4 Navigation

Teacher keys: `dasbor`, `kuis`, `siswa`, `penilaian`, `pengaturan`.
Student keys: `dasbor`, `riwayat`, `profil`.

Removed from the teacher nav: `Bank Soal` (no question bank), the inert global
search box, and the notification bell (no notification model). "Pengaturan"
continues to mean the teachers/majors management area.

## 9. Auth screens

### 9.1 Login

Model:

```go
type LoginPage struct {
    Email string // repopulated on error
}
```

Fields: `email`, `password`. No role field, no "remember me".
Errors: `invalid_credentials` → "Email atau kata sandi salah."
Success: teacher → `/teacher`; student → `/student`.
Pending teacher: 403-style message with a link to `/auth/pending?token=…` when
the registration token is still available.

### 9.2 Registration

One route per role; a link at the bottom switches to the other form. No
segmented toggle, no hidden role input.

Teacher fields: `full_name`, `email`, `password`, `password_confirm`.
Student fields: `full_name`, `email`, `password`, `password_confirm`,
`grade_level` (10/11/12), `major_id` (select from majors).

Student success: signed in, redirect `/student`.
Teacher success: redirect `/auth/pending?token=…` with "Menunggu persetujuan
guru lain."

Validation messages are field-scoped; the form is repopulated on failure and
never re-renders passwords.

### 9.3 Password reset

`/reset-password` asks for `token`, `new_password`, `password_confirm`.
The page explains: "Minta token kepada guru Anda." No email field.

The teacher generates the token from the student row (§16.3); the token is
single-use, shown once, and expires after 24 hours (builder call).

### 9.4 Change password

Fields: `current_password`, `new_password`, `password_confirm`.
Success flash: "Kata sandi diperbarui."

### 9.5 Join by code

`/join` asks for `code`: 6 digits, `pattern="[0-9]{6}"`, `maxlength="6"`,
`inputmode="numeric"`, no letters, no prefix.

Resolution:

- Public quiz, logged-in student → redirect to `/quiz/:id`; if eligible and not
  registered, the intro offers **Daftar**.
- Public quiz, visitor → `/join/identify?quiz=:id` with the code held in a
  signed cookie.
- Private quiz → `anonymous_not_allowed` for visitors, `unknown_code` for
  logged-in students (private codes are inert).

### 9.6 Invitation landing

`/invite/:token` resolves immediately. States:

| Outcome | Screen |
|---|---|
| Public, logged-in | Auto-register → `/quiz/:id` with flash "Anda terdaftar di kuis ini." |
| Public, visitor | `/join/identify?quiz=:id`, token held in the cookie |
| Private, logged-in, matching | Auto-register → `/quiz/:id` |
| Private, logged-in, not matching | `pages/auth/invite.html` with `wrong_audience` |
| Private, visitor | Login prompt with `anonymous_not_allowed` |
| Closed/draft | `invite.html` with `not_open` |
| Unknown token | `invite.html` with `unknown_code` |

The landing screen always offers "Ke dasbor" and "Masuk" where relevant.

### 9.7 Anonymous identity

Fields: `full_name`, `grade_level`, `major_id`. The quiz title is shown.
Submit creates the participant, registers them, and redirects to `/quiz/:id`.
Anonymous entry exists only for public quizzes; a private quiz never reaches
this page.

For retakes, the full name entered on the same quiz is what identifies the
returning participant: the same name reuses that anonymous registration and
starts the next attempt, up to the teacher's cap. Grade and major are refreshed
from the form on each join. Anonymous participants may retake a public quiz
exactly like students; there is no separate anonymous cap.

### 9.8 Pending approval

`/auth/pending?token=…` shows one of: waiting, approved (link to login),
rejected/expired. No account data beyond the full name.

## 10. Student screens

### 10.1 Dashboard

```go
type StudentDashboard struct {
    Stats   StudentStats
    Ready   []QuizCard       // registered, not started
    Live    []QuizCard       // in progress
    Latest  []QuizCard       // available, not registered
    Recent  []QuizResultRow  // completed, newest first
}

type StudentStats struct {
    Assigned   int
    Registered int
    InProgress int
    Completed  int
    BestScore  *Score // nil when no visible score
}
```

- **Assigned** counts every open quiz the student is eligible for, plus their
  own registrations that are still live.
- **BestScore** is the highest visible attempt score across all quizzes; a quiz
  with `show_score = false` never contributes.
- `QuizCard` carries: title, teacher name, question count, audience label
  (private only), code (public only), start/end labels, state, participation
  state, attempts used/max, and one primary action.
- "Latest available quizzes" is the `Latest` list, newest first, capped at 6
  (builder call).
- A registered card shows **Mulai kuis** and **Batalkan pendaftaran**.
- An in-progress card shows **Lanjutkan**.
- A completed quiz with attempts remaining shows **Coba lagi (2/3)**.

### 10.2 History

One row per quiz, not per attempt:

```go
type HistoryRow struct {
    Quiz        QuizRef
    Attempts    []AttemptRow
    BestScore   *Score
    LastAttempt LocalTime
    State       ParticipationState
}
```

Columns: quiz title, teacher, attempts (`2 dari 3`), best score, last activity,
state. Expanding a row lists every attempt with start time, submit time,
score (or "Menunggu penilaian"), and a **Lihat hasil** link. With no
registrations, the empty state points to the dashboard, not to a generic CTA.

Scores respect `show_score`; hidden scores render `—`.

### 10.3 Profile

Read-only: full name, email, grade level, major, class label.
Editable: full name, email.
Grade and major are read-only with "Hubungi guru untuk mengubah kelas atau
jurusan."
Actions: change password, logout.

### 10.4 Quiz intro and registration

```go
type QuizIntro struct {
    Quiz        QuizCard
    Participation ParticipationState
    Attempts    []AttemptRow // compact
    CanRegister bool
    CanCancel   bool
    CanStart    bool
    StartsLeft  int          // 0 when exhausted
    Primary     Action
}
```

Rules:

- `CanRegister` when the quiz is open, the student is eligible, and no
  registration exists.
- `CanCancel` when registered, the quiz is open, and no attempt has started.
- `CanStart` when registered, the quiz is open, and attempts remain.
- For anonymous participants, "Cancel" also clears the identity cookie binding
  for that quiz; the registration row is deleted.
- A registered student who returns to the quiz sees the same intro with
  **Mulai kuis** and **Batalkan pendaftaran**; Start creates attempt N+1.

### 10.5 Attempt question page

```go
type AttemptQuestion struct {
    Attempt   AttemptHeader
    Question  QuestionView
    Nav       []QuestionNav
    Progress  AttemptProgress
    Action    string // POST target
    Draft     AnswerDraft
}

type AttemptHeader struct {
    ID        int64
    QuizTitle string
    StartedAt LocalTime
    AttemptNo int
    Max       int
}

type QuestionView struct {
    ID       int64
    Position int
    Total    int
    Type     QuestionType
    Body     string
    Image    *QuestionImage
    Options  []OptionView
    Essay    *EssayView
}
```

- One real `<form method="post">` wraps the answer. The Next button submits it.
  Without JavaScript the flow still works.
- `OptionView.ID` is the stable option id. The letter (A, B, C) is derived from
  the frozen per-attempt order and exists only for display.
- `EssayView` carries `Value`, `MinChars` (0 unless configured), and `MaxChars`.
- `AttemptProgress` carries `Answered`, `Total`, and computes `Percent()`
  clamped to 0–100.
- No timer, no countdown. The header shows only the start time and attempt
  number.
- The page shows the saved answer on reload, from `Draft`.

JS enhancements (allowed paths only): local character count for essays, a
before-unload warning while answers are unsaved, and the beacon on
`visibilitychange`.

### 10.6 Pre-submit review

Always shows every question and the student's current answer, and allows
navigation back to a question. It is not affected by `review_mode`, which
governs the post-submit result. **Kirim jawaban** posts to submit and asks for
confirmation. Unanswered questions are listed; submission is allowed with a
warning.

### 10.7 Result

```go
type AttemptResult struct {
    Attempt    AttemptHeader
    Submitted  LocalTime
    Score      *Score
    Graded     bool
    Mode       ReviewMode
    Rows       []ResultRow
    ShowScore  bool
}
```

- If the quiz has ungraded essays, the page shows "Menunggu penilaian" in the
  score slot. Content still follows `Mode`.
- `questions_and_key`: body + student answer + key.
- `key_only`: key list only.
- `questions_only`: body + student answer, no verdict.
- `answers_only`: body + student answer, no verdict, no color.
- `show_score = false` removes the score tile and the per-row score; verdicts
  remain as the mode allows.
- Essays have no key; a graded essay shows its score and feedback, an ungraded
  one shows "Menunggu penilaian".
- The result page is reachable from history for every completed attempt.

## 11. Teacher dashboard

```go
type TeacherDashboard struct {
    TopScorers  []QuizTopScore
    Recent      []QuizCard
    MostJoined  []QuizParticipation
    LeastJoined []QuizParticipation
}

type QuizTopScore struct {
    Quiz      QuizRef
    Student   string
    Score     Score
    AttemptID int64
}

type QuizParticipation struct {
    Quiz  QuizRef
    Count int
}
```

- `TopScorers`: the highest-scoring attempt per quiz, newest quizzes first,
  limited to quizzes with at least one graded/completed attempt. Ties show the
  earliest submission (builder call).
- `Recent`: the five newest quizzes by creation time, with state and question
  count.
- `MostJoined` / `LeastJoined`: three quizzes each, counted by distinct
  registered participants; ties by title.
- No pass rate, no class average, no question-bank tile, no "participants"
  headline that the model cannot produce. All numbers come from queries.

## 12. Quiz list

```go
type QuizList struct {
    Quizzes List[QuizCard]
    Tabs    []FilterTab // Semua | Draf | Dibuka | Ditutup
}
```

- Tabs are links with `?state=`, not inert buttons.
- Search: `?q=` over title; sort `?sort=`; pagination `?page=`.
- Row actions by state: draft → Kelola, Buka, Hapus; open → Monitor, Tutup,
  Hasil; closed → Hasil.
- **Hasil** opens `/teacher/quizzes/:id/results` (§13.5), not the monitor.
- Delete is offered only for drafts and asks for confirmation.
- Empty state: "Belum ada kuis" with a **Buat kuis** action.

## 13. Quiz authoring

### 13.1 Step 1 — create

```go
type QuizForm struct {
    Title       string
    Description string
    StartAt     *LocalTime
    EndAt       *LocalTime
    Visibility  Visibility
    TargetGrade *int      // 10/11/12
    TargetMajor *MajorRef
}
```

Fields: `title`, `description`, `start_at`, `end_at`, `visibility`,
`target_grade`, `target_major_id`.

- `target_grade` and `target_major_id` are enabled only when `visibility=private`.
  JavaScript reveals them; the server enforces the pair on save.
- The date fields are `datetime-local`; no validation ties them to the quiz
  state. End must be after start when both are set (builder call).
- Save creates a draft and redirects to `/teacher/quizzes/:id` (the builder).
  Cancel returns to the quiz list.

### 13.2 Step 2 — quiz detail / builder

```go
type QuizDetail struct {
    Quiz      QuizHeader
    Questions []QuestionRow
    Settings  QuizSettings
    Share     QuizShare
    Locked    bool // true once state != draft
}

type QuizHeader struct {
    ID           int64
    Title        string
    Description  string
    State        QuizState
    Visibility   Visibility
    Audience     string // "Publik" or "XII IPA"
    StartsAt     *LocalTime
    EndsAt       *LocalTime
}

type QuizSettings struct {
    RandomizeQuestions bool
    RandomizeAnswers   bool
    AllowRetake        bool
    MaxAttempts        int
    ReviewMode         ReviewMode
    ShowScore          bool
}

type QuizShare struct {
    Code        *string // nil for private
    InviteURL   string
    CanShare    bool    // draft only
}
```

- The builder lists questions with position, type, option count, image marker,
  and edit/delete actions. Drag handles reorder via
  `POST …/questions/reorder` with the ordered ids.
- Settings are on the same page, saved by `POST /teacher/quizzes/:id`.
- **Publish** is disabled when there are no questions or when a private quiz
  lacks grade/major. It asks for confirmation.
- Once open, the page is read-only except **Tutup** and **Monitor**. Once
  closed, the page is read-only and links to the results list (§13.5).
- The invitation URL and code are copyable. Private quizzes show the audience
  and the URL, never a code.
- Images: one per question, shown as a thumbnail with a remove action.

### 13.3 Question editor

```go
type QuestionForm struct {
    ID        int64
    QuizID    int64
    Type      QuestionType
    Body      string
    Image     *QuestionImage
    Options   []OptionForm
    Correct   []int // indices into Options, not database ids
    CanSave   bool
}

type OptionForm struct {
    ID    int64 // 0 for a new option
    Body  string
}
```

- `correct_index` (single) and `correct_indexes[]` (multiple) are indices into
  the submitted `option_body[]` list, on create and on edit alike. The server
  maps them to option ids; the template never handles database ids for a new
  question.

- `type` is a select with exactly three values; switching to essay hides
  options and correct-answer controls.
- Options are repeatable fields; add/remove is client-side, unlimited.
- Single-choice uses radios named `correct_option`; multiple-choice uses
  checkboxes named `correct_options`.
- Essay has no correct answer.
- Image: file input `image`, accept `image/jpeg,image/png,image/webp`, max
  10 MB, one file. Preview is local (FileReader); upload happens on save.
- Validation: body required, at least two options for objective types, exactly
  one correct for single, at least two correct for multiple, zero correct for
  essay.
- Save returns to the detail page with a flash.

### 13.4 Publish and close

- Publish (draft → open): requires at least one question; private requires a
  target pair. Generates/activates the code (public only) and keeps the invite
  token.
- Close (open → closed): sets the state, deactivates the code, and stops the
  monitor stream. In-progress attempts remain in progress; their result pages
  still work (builder call).
- Closed cannot reopen. Draft can be deleted; open and closed cannot.

### 13.5 Results

```go
type QuizResults struct {
    Quiz QuizRef
    Rows List[AttemptResultRow]
}

type AttemptResultRow struct {
    AttemptID   int64
    Student     string
    ClassLabel  string
    AttemptNo   int
    SubmittedAt LocalTime
    Score       *Score // nil while essays are ungraded
    FocusLoss   int
    RecapURL    string
}
```

- One row per attempt, grouped by student, newest submission first.
- Filters: class, search, `?state=all|waiting|graded`.
- Available for open and closed quizzes; it is how a teacher reaches
  `/teacher/recap/:attemptID` after the monitor is gone.
- Empty state: "Belum ada percobaan."

## 14. Live monitor

```go
type MonitorPage struct {
    Quiz         QuizRef
    Progress     MonitorProgress
    Participants []ParticipantRow
    StreamURL    string
    StaleAt      *LocalTime
}

type MonitorProgress struct {
    Registered int
    InProgress int
    Completed  int
    Answered   int
    Total      int
}

type ParticipantRow struct {
    AttemptID  *int64 // nil until the participant starts
    Name       string
    Initials   string
    ClassLabel string
    Answered   int
    Total      int
    State      ParticipationState
    FocusLoss  int
    LastSeen   LocalTime
}
```

- Reachable only while `state=open`; otherwise the page shows "Kuis tidak
  sedang berlangsung" with a link back.
- Shows every registered participant; those who have not started have a nil
  `AttemptID`, zero progress, and `State = registered`.
- `Percent()` clamps; the page never computes a percentage in the template.
- Live updates arrive over SSE (§19). Without JavaScript, a `<noscript>` meta
  refresh reloads every 15 seconds.
- Focus-loss count is informational and never blocks.

## 15. Grading and recap

### 15.1 Queue and detail

One template, two routes, one wrapper model. `/teacher/grading` renders the
queue with `Selected` nil; `/teacher/grading/:answerID` renders the same page
with the detail pane filled.

```go
type GradingPage struct {
    Queue    GradingQueue
    Selected *GradingDetail // nil on the queue route
}

type GradingQueue struct {
    Counts GradingCounts
    Rows   List[GradingRow]
}
```

One row per ungraded essay answer, newest submission first: student, quiz,
question excerpt, waiting time, **Nilai**. Filters: quiz, class, search.
Empty state: "Tidak ada esai yang menunggu penilaian."

### 15.2 Grade one essay

```go
type GradingDetail struct {
    Answer    EssayAnswer
    Attempt   AttemptHeader
    Suggested *Score
    Score     *Score
    Feedback  string
}
```

- Shows the question body, the student's essay, and the other answers in the
  attempt for context.
- `score` is 0–100 with one decimal allowed; stored as 0–1.
- `feedback` is optional plain text.
- Saving updates only that answer. When every essay in the attempt is graded,
  the attempt score appears immediately; there is no separate finalise step.
- The final score is the mean of question scores (objective 100/0, essay
  0–100), rounded half-up. No manual override.

### 15.3 Attempt recap

```go
type Recap struct {
    Attempt  RecapAttempt
    Answers  []RecapRow
    Mode     ReviewMode
    ShowScore bool
}
```

Header: student, class, quiz, start, submit, score or waiting, focus-loss
count, attempt number of N. Rows: position, type, body, student answer, key
when the mode permits, per-question score, essay feedback. The teacher sees the
key regardless of `Mode`; the mode governs only what students see.

## 16. Management

### 16.1 Students

```go
type StudentList struct {
    Students List[StudentRow]
    Tabs     []FilterTab // grade level
    Majors   []MajorRef
}

type StudentRow struct {
    ID         int64
    FullName   string
    Email      string
    ClassLabel string
    Attempts   int
}
```

Filters: grade, major, search, sort, page. Row actions: Ubah, Hapus, **Buat
token atur ulang**. Delete asks for confirmation and warns that attempts are
deleted with the student.

Edit form: `full_name`, `email`, `grade_level`, `major_id`.

Reset-token result: a one-time panel with the token, a copy button, and
"Berikan token ini kepada siswa." The token is not stored in the page model
after it is consumed.

### 16.2 Anonymous participants

List: name, class, quizzes joined, last activity. Actions: Ubah, Hapus. Edit
form mirrors identify: `full_name`, `grade_level`, `major_id`.

### 16.3 Majors

Create: `name`. Rename: `name`. Delete: allowed only when no student,
anonymous participant, or quiz target references the major. Rename propagates
to students, anonymous participants, and **quiz targets** (old spec omitted
the last one; this document corrects it).

### 16.4 Teachers

Two sections: pending approvals and active teachers. Teacher rows are
read-only: name, email, status, approved-by. The only action is **Setujui** on
a pending row. No edit, no deactivate, no delete, no reject action (a pending
row can simply be left alone).

## 17. State matrix and empty states

Per surface family, render exactly these states. Hidden state demos are
development-only and never ship in production HTML.

| Family | States |
|---|---|
| Auth | default, field error, form-level error, no-JS |
| Lists | populated, empty, filtered-empty, error |
| Quiz detail | draft, open (locked), closed (locked), no questions |
| Quiz intro | not registered, registered, in progress, completed, attempts exhausted, rejected invite |
| Attempt | unanswered, answered, saving (JS), locked after submit |
| Result | graded, waiting for grade, score hidden, empty |
| Monitor | live, stale, not on air, empty |
| Grading | queue, detail, saved |

Empty states take `Title`, `Body`, and one action:
`EmptyState{Title, Body, Action}`. The old single hardcoded "Buat kuis baru"
partial no longer exists; each list supplies its own action.

## 18. Forms and validation

Every state-changing action is a POST, carries `csrf_token`, and follows
post/redirect/get. A failed validation re-renders the same template with HTTP
422, the submitted values, and `FieldErrors`; passwords are never repopulated.
In the UI-only phase the 422 path is fixture-only and validation is client-side
(§2.1); the field names and markup below are already final.

Field-name contract (closed list; the backend validates against these names):

| Form | Fields |
|---|---|
| Login | `email`, `password` |
| Register teacher | `full_name`, `email`, `password`, `password_confirm` |
| Register student | + `grade_level`, `major_id` |
| Reset password | `token`, `new_password`, `password_confirm` |
| Change password | `current_password`, `new_password`, `password_confirm` |
| Join | `code` |
| Identify | `full_name`, `grade_level`, `major_id` |
| Create quiz | `title`, `description`, `start_at`, `end_at`, `visibility`, `target_grade`, `target_major_id` |
| Quiz settings | `randomize_questions`, `randomize_answers`, `allow_retake`, `max_attempts`, `review_mode`, `show_score` |
| Question | `type`, `body`, `image`, `option_body[]`, `correct_index`, `correct_indexes[]` |
| Answer | `question_id`, `selected_option_ids[]`, `essay_text`, `nav` |
| Grade essay | `score`, `feedback` |
| Student edit | `full_name`, `email`, `grade_level`, `major_id` |
| Major | `name` |

Hidden identity fields (`quiz_id`, `attempt_id`, `question_id`) belong in the
action path, not the body.

## 19. Live payload contracts

### 19.1 SSE

`GET /teacher/quizzes/:id/stream`, only while open. Events are named, and the
`data` field is JSON:

| Event | Payload |
|---|---|
| `snapshot` | `MonitorPage` without `StreamURL` |
| `participant.updated` | `{attempt_id|null, answered, total, state, focus_loss, last_seen}`; `state` is a `ParticipationState` |
| `participant.added` | full `ParticipantRow` |
| `quiz.closed` | `{quiz_id}`; the client stops and shows the not-on-air state |
| `heartbeat` | `{at}` every 30s |

The browser uses `EventSource`. `static/js/app.js` is currently forbidden from
network calls by a conformance test; that rule is narrowed to allow
`EventSource` only on the stream path and `navigator.sendBeacon` only on the
beacon path.

### 19.2 Beacon

`POST /attempt/:id/beacon`, `text/plain` body or JSON:

```json
{"kind":"focus_loss|progress","question_id":12,"position":3,"at":"2026-10-23T09:42:00Z"}
```

Server-side rules: CSRF-exempt (the only exempt route besides the stream), rate
limited, and idempotent for progress. Focus loss increments the attempt count
and appends an event; progress stores the current position for the monitor.

In the UI-only phase neither the stream nor the beacon is served. The monitor
renders its snapshot from a fixture and `app.js` does not open a connection.
The payloads above are the backend-phase contract.

## 20. Demo data and fixtures

- Each page's view model has a fixture constructor in
  `internal/httpserver/fixtures` (one function per page group).
- Fixtures use one coherent world: the same teachers, students, majors, and
  quizzes across pages. Numbers agree: a quiz card claiming 5 questions
  matches the detail page's 5 rows; a 2-of-3 history row matches the attempts.
- Production builders replace fixtures page by page; the templates never change.
- No fixture data is inlined in templates. A literal name, score, or date in a
  template is a test failure (see §21).
- The fixture set covers every state in §17, not just the happy path.
- Fixtures are compiled into the binary and selected by `APP_DEMO=1`. When it
  is set, routes accept `?state=` and render a small preview strip for manual
  review. When it is unset (production), `?state=` is ignored, no strip is
  rendered, and the normal builders run. The tests construct fixtures directly
  and never depend on the environment variable.

## 21. Test plan

Cheap, server-side tests that keep this document honest. New file:
`internal/httpserver/contract_test.go`, plus focused tests per group.

| # | Test | Pins |
|---|---|---|
| C1 | `TestRouteSurface` | The exact method+path set in §8; additions are deliberate |
| C2 | `TestFormsPostWithCSRF` | Every `<form method="post">` targets a registered POST route and contains `csrf_token` |
| C3 | `TestViewModelsRenderZeroAndPopulated` | Every page renders from zero values and from fixtures; no panic, no `<no value>` |
| C4 | `TestNoLiteralData` | No hardcoded score/date/name patterns in `templates/pages/**` |
| C5 | `TestScoreFormatting` | 0, 0.005, 0.8, 1.0, nil → `0%`, `1%`, `80%`, `100%`, `—` |
| C6 | `TestDateFormatting` | WIB short/clock/full strings with a fixed clock |
| C7 | `TestEnumLabels` | Every enum value has exactly one Indonesian label; unknown values fail |
| C8 | `TestReviewModeMatrix` | Each mode renders exactly the permitted fields; `show_score=false` hides every score |
| C9 | `TestParticipationStates` | Intro renders the right action set for each state |
| C10 | `TestFieldNameContract` | Form names match §18; no `class`, `major`, `duration`, `attempts`, `openAt`, `closeAt` |
| C11 | `TestJoinCodeShape` | 6 digits, numeric pattern, no prefix |
| C12 | `TestPagination` | List helpers clamp page 0, page > pages, and empty lists |
| C13 | `TestNavActive` | Exactly one `aria-current="page"` on the matching section |
| C14 | `TestUniqueIDs` | No duplicate `id` in a rendered page |
| C15 | `TestNoHiddenStateDemos` | Production pages contain no `data-state-demo` |
| C16 | `TestMonitorPayloads` | SSE event names and JSON shapes match §19 |

Phase applicability:

- **UI-only phase:** C1, C3–C15 run. C2 checks that each form targets a
  registered POST route and contains the placeholder `csrf_token`; token
  validity is not tested. C5/C6/C7 run against the view-model formatters. C8
  and C9 render every fixture state. C16 is deferred.
- **Backend phase:** C2 becomes strict (real token verification), C16 runs,
  and the 422 field-error path is tested with server validation.

Existing tests to amend deliberately:

- `conformance_test.go`'s route count (29) becomes the §8 set.
- The "no network calls in app.js" test is narrowed to the two allowed paths.
- `routes_test.go`'s registry pollution is fixed with `t.Cleanup`; a page
  builder map keyed by `method + " " + path` keeps `PageRoute` comparable.
- The 27 page templates gain `.Data` field access; zero-value rendering is
  tested before fixtures are wired.

## 22. Backend implications (conceptual, not schema)

The following records must exist for the view models to be buildable. Names are
conceptual; the schema plan decides columns.

1. **Registration** — one per participant per quiz, with state and timestamps.
   Attempts belong to a registration. Without it, "registered" cannot be
   distinguished from "never joined".
2. **Attempt** — one per start, numbered within the registration, with start,
   submit, and score state.
3. **Invitation token** — per quiz, unique, resolvable before the quiz opens.
4. **Quiz settings** — retake cap, randomization, review mode, score visibility.
5. **Question image** — a single image reference per question.
6. **Password reset token** — one-time, issued by a teacher to a student.
7. **Major references** — students, anonymous participants, and quiz targets.

Explicitly gone from the old model: the `counted` attempt flag, the
`duration`/countdown field, the question bank, the four old result modes, and
teacher deactivation.

## 23. What is left to the builder

These are calls I made that the builder may settle without changing the
product. They are marked here so they can be checked during review.

1. Route paths and parameter names in §8.
2. Invite token format (`/invite/:token`, unguessable, generated at creation).
3. Quiz code generated at creation for every quiz but surfaced and accepted
   only for public quizzes.
4. `answers_only` renders question bodies plus the student's answers, without
   key or verdict.
5. Default review mode `questions_only`, default score visibility on.
6. Averages display one decimal; per-attempt scores are whole percentages.
7. No relative dates; no timer; no rombel/section; no pass mark; no class
   average.
8. Profile edits limited to name and email; grade/major are teacher-managed.
9. Registration cancellation allowed only before the first attempt starts.
10. Attempts remain playable after the quiz closes; the result page still
    works, but no new attempt can start.
11. Reset tokens expire after 24 hours and are single-use.
12. Removed UI: question bank, notification bell, global search, remember-me,
    school field, terms checkbox, teacher subject line, teacher deactivation.
13. List page sizes: 10 for students and quizzes, 20 for grading.
14. Dashboard caps: 5 recent quizzes, 6 latest student quizzes, 3 top/bottom
    participation lists.
15. Focus-loss tracking is retained (confirmed in review).
16. Per-quiz results page (`/teacher/quizzes/:id/results`) lists every attempt
    and is the post-close path to a recap.
17. Grading is one template with a nullable `Selected` detail, not two screens.
18. Correct-answer form values are indices into the submitted option list,
    identical for create and edit.
19. `APP_DEMO=1` enables the state preview strip and `?state=`; production
    ignores both.

## 24. Resolved in review

The reviewer's remaining questions were answered on 2026-10-23:

1. **Focus-loss tracking is retained.** It stays in the attempt page, the
   monitor count, and the recap.
2. **Essay grading has no finalise step.** The computed score is final. A
   student sees the full score in history and on the result page whenever the
   teacher enabled `show_score`; otherwise the score is hidden, never zero.
3. **Anonymous retakes are allowed**, identified by full name on the same
   quiz. Private quizzes still admit no anonymous participants, so the
   question does not arise there.
4. **The first build is UI-only.** Validations live in the forms, no data is
   persisted, and every value is hardcoded; workflows still follow the real
   backend sequence (§2.1).
