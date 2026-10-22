package viewmodel

import (
	"net/url"
	"strconv"
)

// PageData is the one value every template receives. Base carries chrome and
// request-scoped state; Data is exactly one screen model (LoginPage,
// StudentDashboard, …).
type PageData struct {
	Base
	Data any
}

// Base is the shared chrome contract: the document title, active nav key, the
// signed-in user (nil on public pages), the one-shot flash message, the CSRF
// placeholder, field errors for forms, and whether the demo strip is enabled.
type Base struct {
	Title  string
	Active string
	User   *CurrentUser
	Flash  *Flash
	CSRF   string
	Demo   bool
	Errors FieldErrors
}

// CurrentUser is the signed-in identity shown in the chrome. Anonymous
// participants never receive a CurrentUser.
type CurrentUser struct {
	ID         int64
	FullName   string
	Email      string
	Role       Role
	GradeLevel int
	MajorName  string
	Initials   string
}

// ClassLabel renders the user's class, when one is known.
func (u CurrentUser) ClassLabel() string {
	return ClassLabel(u.GradeLevel, u.MajorName)
}

// IsTeacher reports the teacher role.
func (u CurrentUser) IsTeacher() bool { return u.Role == RoleTeacher }

// IsStudent reports the student role.
func (u CurrentUser) IsStudent() bool { return u.Role == RoleStudent }

// Flash is a one-shot status message rendered above page content. Kind is one
// of success, info, warning or danger.
type Flash struct {
	Kind  string
	Title string
	Body  string
}

// FlashFor maps the closed ?flash= key set to copy. An unknown key is ignored,
// never reflected.
func FlashFor(key string) *Flash {
	switch key {
	case "welcome":
		return &Flash{Kind: "success", Title: "Berhasil masuk."}
	case "saved":
		return &Flash{Kind: "success", Title: "Perubahan tersimpan."}
	case "registered":
		return &Flash{Kind: "success", Title: "Anda terdaftar di kuis ini."}
	case "cancelled":
		return &Flash{Kind: "info", Title: "Pendaftaran dibatalkan."}
	case "started":
		return &Flash{Kind: "success", Title: "Percobaan dimulai.", Body: "Jawaban tersimpan saat Anda menekan Simpan."}
	case "submitted":
		return &Flash{Kind: "success", Title: "Jawaban terkirim."}
	case "graded":
		return &Flash{Kind: "success", Title: "Nilai esai tersimpan."}
	case "published":
		return &Flash{Kind: "success", Title: "Kuis dibuka."}
	case "closed":
		return &Flash{Kind: "info", Title: "Kuis ditutup."}
	case "deleted":
		return &Flash{Kind: "success", Title: "Data dihapus."}
	case "approved":
		return &Flash{Kind: "success", Title: "Akun guru disetujui."}
	case "pending":
		return &Flash{Kind: "info", Title: "Pendaftaran menunggu persetujuan guru lain.", Body: "Anda dapat masuk setelah akun disetujui."}
	case "account-created":
		return &Flash{Kind: "success", Title: "Akun siswa dibuat.", Body: "Silakan masuk dengan kata sandi yang baru dibuat."}
	case "password-reset":
		return &Flash{Kind: "success", Title: "Kata sandi diperbarui.", Body: "Silakan masuk dengan kata sandi baru."}
	case "password-changed":
		return &Flash{Kind: "success", Title: "Kata sandi diperbarui."}
	case "token":
		return &Flash{Kind: "info", Title: "Token atur ulang dibuat.", Body: "Berikan token di bawah kepada siswa. Token berlaku 24 jam."}
	case "quiz-created":
		return &Flash{Kind: "success", Title: "Draf kuis dibuat.", Body: "Tambahkan soal sebelum membuka kuis."}
	case "question-saved":
		return &Flash{Kind: "success", Title: "Soal tersimpan."}
	case "reordered":
		return &Flash{Kind: "success", Title: "Urutan soal diperbarui."}
	case "logout":
		return &Flash{Kind: "info", Title: "Anda telah keluar."}
	default:
		return nil
	}
}

// FieldErrors is the field-scoped validation map a 422 re-render carries.
// Keys match input names. Get makes the map safe inside templates.
type FieldErrors map[string]string

// Get returns the message for one field, or "".
func (e FieldErrors) Get(key string) string { return e[key] }

// Has reports whether any field failed.
func (e FieldErrors) Has() bool { return len(e) > 0 }

// QueryState is the search/filter/sort state echoed into forms and links.
type QueryState struct {
	Q    string
	Tab  string
	Sort SortOrder
}

// Get returns "" or the stored query value; used to keep links terse.
func (q QueryState) Get() string { return q.Q }

// Pagination is precomputed for the shared pagination partial. Handlers build
// it with NewPagination so templates never concatenate query strings.
type Pagination struct {
	Page     int
	Pages    int
	Total    int
	From     int
	To       int
	PrevHref string
	NextHref string
	Numbers  []PageNumber
}

// PageNumber is one clickable page.
type PageNumber struct {
	Number int
	Href   string
	Active bool
}

// HasPrev reports whether a previous page exists.
func (p Pagination) HasPrev() bool { return p.Page > 1 }

// HasNext reports whether a next page exists.
func (p Pagination) HasNext() bool { return p.Page < p.Pages }

// Show reports whether the rail is worth rendering.
func (p Pagination) Show() bool { return p.Pages > 1 }

// NewPagination builds page links from the current query, replacing only the
// page key. page is 1-based.
func NewPagination(base string, query url.Values, page, perPage, total int) Pagination {
	if perPage < 1 {
		perPage = 10
	}
	pages := (total + perPage - 1) / perPage
	if pages < 1 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	href := func(n int) string {
		q := url.Values{}
		for k, vs := range query {
			if k == "page" {
				continue
			}
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		if n > 1 {
			q.Set("page", strconv.Itoa(n))
		}
		if enc := q.Encode(); enc != "" {
			return base + "?" + enc
		}
		return base
	}
	from, to := (page-1)*perPage+1, page*perPage
	if total == 0 {
		from, to = 0, 0
	} else if to > total {
		to = total
	}
	p := Pagination{Page: page, Pages: pages, Total: total, From: from, To: to}
	if page > 1 {
		p.PrevHref = href(page - 1)
	}
	if page < pages {
		p.NextHref = href(page + 1)
	}
	start, end := page-2, page+2
	if start < 1 {
		start = 1
	}
	if end > pages {
		end = pages
	}
	for n := start; n <= end; n++ {
		p.Numbers = append(p.Numbers, PageNumber{Number: n, Href: href(n), Active: n == page})
	}
	return p
}

// List is a page of one row type plus its total. Templates render Items and
// the shared pagination partial; they never slice.
type List[T any] struct {
	Items   []T
	Total   int
	Page    int
	PerPage int
}

// NewList clamps the page into range and returns the slice for that page.
func NewList[T any](items []T, page, perPage int) List[T] {
	if perPage < 1 {
		perPage = 10
	}
	pages := (len(items) + perPage - 1) / perPage
	if pages < 1 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	start := (page - 1) * perPage
	end := start + perPage
	if start > len(items) {
		start = len(items)
	}
	if end > len(items) {
		end = len(items)
	}
	return List[T]{Items: items[start:end], Total: len(items), Page: page, PerPage: perPage}
}

// Empty reports whether the list has no rows.
func (l List[T]) Empty() bool { return len(l.Items) == 0 }

// Pages returns the page count.
func (l List[T]) Pages() int {
	if l.Total == 0 {
		return 1
	}
	return (l.Total + l.PerPage - 1) / l.PerPage
}

// FilterTab is one link in a segmented filter row. Count is omitted when zero.
type FilterTab struct {
	Label  string
	Href   string
	Active bool
	Count  int
}

// CardAction is a pre-labelled, pre-linked action. Style is primary, outline
// or danger; templates translate it to the one button vocabulary.
type CardAction struct {
	Label string
	Href  string
	Style string
}

// Primary, Outline and Danger build the three action styles.
func Primary(label, href string) CardAction {
	return CardAction{Label: label, Href: href, Style: "primary"}
}
func Outline(label, href string) CardAction {
	return CardAction{Label: label, Href: href, Style: "outline"}
}
func Danger(label, href string) CardAction {
	return CardAction{Label: label, Href: href, Style: "danger"}
}

// EmptyState is the one empty-state copy used across lists.
type EmptyState struct {
	Title  string
	Body   string
	Icon   string
	Action *CardAction
}

// MajorRef is a major id/name pair for selects and metadata.
type MajorRef struct {
	ID   int64
	Name string
}

// QuizRef is the minimal quiz identity used in headers and breadcrumbs.
type QuizRef struct {
	ID    int64
	Title string
}

// QuizCard is one row/card in any quiz list (teacher or student).
type QuizCard struct {
	ID            int64
	Href          string
	Title         string
	Description   string
	TeacherName   string
	QuestionCount int
	State         QuizState
	Visibility    Visibility
	Audience      string
	Code          *string
	StartsAt      *LocalTime
	EndsAt        *LocalTime
	Participation ParticipationState
	AttemptsUsed  int
	MaxAttempts   int
	ReviewMode    ReviewMode
	ShowScore     bool
	JoinedAt      *LocalTime
	Primary       CardAction
	Secondary     *CardAction
}

// Schedule renders the start–end pair, or "Tanpa batas waktu".
func (q QuizCard) Schedule() string {
	if q.StartsAt == nil && q.EndsAt == nil {
		return "Tanpa batas waktu"
	}
	if q.StartsAt == nil {
		return "Hingga " + q.EndsAt.DateTime()
	}
	if q.EndsAt == nil {
		return "Mulai " + q.StartsAt.DateTime()
	}
	return q.StartsAt.DateTimeRange(*q.EndsAt)
}

// AudienceText renders the visibility and target audience.
func (q QuizCard) AudienceText() string {
	if q.Visibility == VisibilityPrivate {
		return "Privat · " + q.Audience
	}
	return "Publik · Semua kelas"
}

// CodeText renders the join code or a dash.
func (q QuizCard) CodeText() string {
	if q.Code == nil || *q.Code == "" {
		return "Tanpa kode"
	}
	return *q.Code
}

// AttemptText renders "1 dari 2 percobaan".
func (q QuizCard) AttemptText() string {
	if q.MaxAttempts <= 0 {
		return strconv.Itoa(q.AttemptsUsed) + " percobaan"
	}
	return strconv.Itoa(q.AttemptsUsed) + " dari " + strconv.Itoa(q.MaxAttempts) + " percobaan"
}

// StatusLabel renders the student-facing participation badge.
func (q QuizCard) StatusLabel() string { return q.Participation.Label() }

// MetaText composes the compact metadata line.
func (q QuizCard) MetaText() string {
	s := q.TeacherName
	if q.QuestionCount > 0 {
		s += " · " + strconv.Itoa(q.QuestionCount) + " soal"
	}
	return s
}

// AttemptRow is one attempt inside a history or quiz-intro row.
type AttemptRow struct {
	ID          int64
	Number      int
	MaxAttempts int
	StartedAt   *LocalTime
	SubmittedAt *LocalTime
	Score       *Score
	State       AttemptState
	Waiting     bool
	FocusLoss   int
	ResultHref  string
	ReviewHref  string
}

// StatusLabel renders "Menunggu nilai" for ungraded essays, otherwise the
// attempt state.
func (a AttemptRow) StatusLabel() string {
	if a.Waiting {
		return "Menunggu nilai"
	}
	return a.State.Label()
}

// StatusTone renders the badge tone key for the shared status partial.
func (a AttemptRow) StatusTone() string {
	switch {
	case a.Waiting:
		return "warning"
	case a.State == AttemptInProgress:
		return "primary"
	default:
		return "success"
	}
}

// QuizTopScore is the teacher dashboard "top scorer per quiz" row.
type QuizTopScore struct {
	Quiz        QuizRef
	StudentName string
	ClassLabel  string
	Score       Score
	Href        string
}

// QuizParticipation is one most/least-joined quiz.
type QuizParticipation struct {
	Quiz   QuizRef
	Joined int
	Href   string
}

// QuestionImage is the single optional image attached to a question.
type QuestionImage struct {
	URL       string
	Alt       string
	SizeLabel string
}

// QuestionNav is one numbered question button in the attempt navigator.
type QuestionNav struct {
	Position int
	Href     string
	Answered bool
	Current  bool
}

// Class renders the nav button's state class.
func (n QuestionNav) Class() string {
	switch {
	case n.Current:
		return "is-current"
	case n.Answered:
		return "is-answered"
	default:
		return "is-unanswered"
	}
}

// AttemptHeader is the shared attempt identity block.
type AttemptHeader struct {
	ID          int64
	QuizID      int64
	QuizTitle   string
	StudentName string
	ClassLabel  string
	StartedAt   LocalTime
	AttemptNo   int
	MaxAttempts int
}

// AttemptProgress is the answered/total rail.
type AttemptProgress struct {
	Answered int
	Total    int
}

// Percent returns a clamped integer for the ARIA progressbar.
func (p AttemptProgress) Percent() int {
	if p.Total <= 0 {
		return 0
	}
	n := p.Answered * 100 / p.Total
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}

// Unanswered returns the remaining count.
func (p AttemptProgress) Unanswered() int {
	if p.Total < p.Answered {
		return 0
	}
	return p.Total - p.Answered
}

// OptionView is one radio/checkbox row in the attempt form.
type OptionView struct {
	ID      int64
	Letter  string
	Body    string
	Checked bool
}

// EssayView is the essay textarea model.
type EssayView struct {
	Value    string
	MinChars int
	MaxChars int
	Rows     int
}

// AnswerDraft is the saved answer for one question.
type AnswerDraft struct {
	SelectedIDs []int64
	EssayText   string
}

// ReviewRow is one question summary on the pre-submit review screen. Answer is
// the student's current answer rendered for display (option text or essay).
type ReviewRow struct {
	Position   int
	Type       QuestionType
	Body       string
	Answer     string
	Unanswered bool
	Href       string
}

// ResultRow is one question in a student result/review or teacher recap.
type ResultRow struct {
	Position      int
	Type          QuestionType
	Body          string
	Image         *QuestionImage
	StudentAnswer string
	Key           string
	Feedback      string
	Verdict       string // correct, incorrect, unanswered, ungraded
	Score         *Score
	Essay         bool
	Graded        bool
}

// VerdictLabel renders the Indonesian verdict word.
func (r ResultRow) VerdictLabel() string {
	switch r.Verdict {
	case "correct":
		return "Benar"
	case "incorrect":
		return "Salah"
	case "unanswered":
		return "Tidak dijawab"
	case "ungraded":
		return "Menunggu nilai"
	default:
		return "—"
	}
}

// VerdictTone maps the verdict to a badge tone.
func (r ResultRow) VerdictTone() string {
	switch r.Verdict {
	case "correct":
		return "success"
	case "incorrect":
		return "danger"
	case "unanswered":
		return "secondary"
	case "ungraded":
		return "warning"
	default:
		return "secondary"
	}
}

// ResultHref is the canonical attempt result URL.
func ResultHref(attemptID int64) string {
	return "/attempt/" + strconv.FormatInt(attemptID, 10) + "/result"
}

// RecapHref is the teacher recap URL for one attempt.
func RecapHref(attemptID int64) string {
	return "/teacher/recap/" + strconv.FormatInt(attemptID, 10)
}
