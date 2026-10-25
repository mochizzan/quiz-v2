// Package viewmodel holds the display contract between the SSR handlers and
// the templates: closed enums with Indonesian labels, preformatted scores and
// dates, and one typed struct per screen. It imports nothing from
// internal/httpserver so fixtures and handlers can both depend on it.
package viewmodel

// Role is the closed account-role set. The raw values are storage values and
// are never rendered.
type Role string

const (
	RoleTeacher   Role = "teacher"
	RoleStudent   Role = "student"
	RoleAnonymous Role = "anonymous"
)

// Label returns the Indonesian display label.
func (r Role) Label() string {
	switch r {
	case RoleTeacher:
		return "Guru"
	case RoleStudent:
		return "Siswa"
	case RoleAnonymous:
		return "Peserta"
	default:
		return labelUnknown
	}
}

// AccountStatus is the teacher approval state. Students are always active.
type AccountStatus string

const (
	AccountPending AccountStatus = "pending"
	AccountActive  AccountStatus = "active"
)

// Label returns the Indonesian display label.
func (s AccountStatus) Label() string {
	switch s {
	case AccountPending:
		return "Menunggu persetujuan"
	case AccountActive:
		return "Aktif"
	default:
		return labelUnknown
	}
}

// QuizState is the manual quiz lifecycle. Start/end times never change it.
type QuizState string

const (
	QuizDraft  QuizState = "draft"
	QuizOpen   QuizState = "open"
	QuizClosed QuizState = "closed"
)

// Label returns the Indonesian display label.
func (s QuizState) Label() string {
	switch s {
	case QuizDraft:
		return "Draf"
	case QuizOpen:
		return "Dibuka"
	case QuizClosed:
		return "Ditutup"
	default:
		return labelUnknown
	}
}

// BadgeClass returns the complete status-badge class. Open uses the single
// action colour, closed uses a neutral pill, draft uses the muted pill.
func (s QuizState) BadgeClass() string {
	switch s {
	case QuizOpen:
		return "badge theme-primary"
	case QuizDraft:
		return "badge bg-2 fg-secondary"
	case QuizClosed:
		return "badge bg-3 fg-body"
	default:
		return "badge bg-2 fg-secondary"
	}
}

// Visibility is the quiz audience rule.
type Visibility string

const (
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

// Label returns the Indonesian display label.
func (v Visibility) Label() string {
	switch v {
	case VisibilityPublic:
		return "Publik"
	case VisibilityPrivate:
		return "Privat"
	default:
		return labelUnknown
	}
}

// QuestionType is the closed three-type set.
type QuestionType string

const (
	QuestionSingle   QuestionType = "single"
	QuestionMultiple QuestionType = "multiple"
	QuestionEssay    QuestionType = "essay"
)

// Label returns the Indonesian display label.
func (q QuestionType) Label() string {
	switch q {
	case QuestionSingle:
		return "Pilihan tunggal"
	case QuestionMultiple:
		return "Pilihan ganda"
	case QuestionEssay:
		return "Esai"
	default:
		return labelUnknown
	}
}

// IsObjective reports whether the type has answer options and a key.
func (q QuestionType) IsObjective() bool {
	return q == QuestionSingle || q == QuestionMultiple
}

// ParticipationState is the student-facing lifecycle of one quiz registration.
type ParticipationState string

const (
	ParticipationRegistered ParticipationState = "registered"
	ParticipationInProgress ParticipationState = "in_progress"
	ParticipationCompleted  ParticipationState = "completed"
)

// Label returns the Indonesian display label.
func (p ParticipationState) Label() string {
	switch p {
	case ParticipationRegistered:
		return "Terdaftar"
	case ParticipationInProgress:
		return "Sedang berlangsung"
	case ParticipationCompleted:
		return "Selesai"
	default:
		return labelUnknown
	}
}

// BadgeClass returns the complete participation badge class.
func (p ParticipationState) BadgeClass() string {
	switch p {
	case ParticipationRegistered:
		return "badge theme-info"
	case ParticipationInProgress:
		return "badge theme-primary"
	case ParticipationCompleted:
		return "badge theme-success"
	default:
		return "badge bg-2 fg-secondary"
	}
}

// AttemptState is the lifecycle of one attempt.
type AttemptState string

const (
	AttemptInProgress AttemptState = "in_progress"
	AttemptCompleted  AttemptState = "completed"
)

// Label returns the Indonesian display label.
func (a AttemptState) Label() string {
	switch a {
	case AttemptInProgress:
		return "Sedang berlangsung"
	case AttemptCompleted:
		return "Selesai"
	default:
		return labelUnknown
	}
}

// ReviewMode controls what a student may see after submitting.
type ReviewMode string

const (
	ReviewQuestionsNoVerdict ReviewMode = "questions_no_verdict"
	ReviewQuestionsNoAnswers ReviewMode = "questions_no_answers"
	ReviewCorrectOnly        ReviewMode = "correct_only"
	ReviewQuestionsWithKey   ReviewMode = "questions_with_key"
)

// Label returns the Indonesian display label.
func (m ReviewMode) Label() string {
	switch m {
	case ReviewQuestionsNoVerdict:
		return "Soal dan jawaban siswa"
	case ReviewQuestionsNoAnswers:
		return "Hanya soal"
	case ReviewCorrectOnly:
		return "Hanya soal yang benar"
	case ReviewQuestionsWithKey:
		return "Soal dan kunci jawaban"
	default:
		return labelUnknown
	}
}

// Valid reports whether the value is one of the four closed review modes. The
// authoring validation and the publish re-check call this predicate so the set
// has one definition.
func (m ReviewMode) Valid() bool {
	switch m {
	case ReviewQuestionsNoVerdict, ReviewQuestionsNoAnswers,
		ReviewCorrectOnly, ReviewQuestionsWithKey:
		return true
	default:
		return false
	}
}

// ShowsQuestion reports whether question bodies and images are visible. All
// four modes show the question; the predicate exists so a template states the
// capability it renders instead of assuming it.
func (m ReviewMode) ShowsQuestion() bool { return m.Valid() }

// ShowsStudentAnswer reports whether the participant's own answer is visible:
// every mode except questions_no_answers.
func (m ReviewMode) ShowsStudentAnswer() bool {
	switch m {
	case ReviewQuestionsNoVerdict, ReviewCorrectOnly, ReviewQuestionsWithKey:
		return true
	default:
		return false
	}
}

// ShowsKey reports whether the correct answer is visible. Only the full record
// shows the key (ATT-152/R-18).
func (m ReviewMode) ShowsKey() bool { return m == ReviewQuestionsWithKey }

// ShowsVerdict reports whether correct/incorrect marks (and with them the
// answer box's colour wash) are visible. Only the full record shows a verdict;
// the old questions_only value claimed one, which leaked right/wrong to a
// screen the teacher had configured to hide it (ATT-152/R-18).
func (m ReviewMode) ShowsVerdict() bool { return m == ReviewQuestionsWithKey }

// ShowsEssayFeedback reports whether essay feedback and the essay score appear.
func (m ReviewMode) ShowsEssayFeedback() bool { return m == ReviewQuestionsWithKey }

// ShowsUnansweredCount reports whether the screen names the unanswered
// positions. Modes that hide the verdict cannot explain a position list.
func (m ReviewMode) ShowsUnansweredCount() bool {
	return m == ReviewQuestionsNoVerdict || m == ReviewQuestionsWithKey
}

// OnlyCorrect reports whether the screen lists just the correctly answered
// questions (essays excluded, positions preserved).
func (m ReviewMode) OnlyCorrect() bool { return m == ReviewCorrectOnly }

// EssayState is the grading state of one essay answer.
type EssayState string

const (
	EssayUngraded EssayState = "ungraded"
	EssayGraded   EssayState = "graded"
)

// Label returns the Indonesian display label.
func (e EssayState) Label() string {
	switch e {
	case EssayUngraded:
		return "Menunggu penilaian"
	case EssayGraded:
		return "Sudah dinilai"
	default:
		return labelUnknown
	}
}

// JoinError is the closed set of join/invite rejection reasons.
type JoinError string

const (
	JoinUnknownCode         JoinError = "unknown_code"
	JoinNotOpen             JoinError = "not_open"
	JoinWrongAudience       JoinError = "wrong_audience"
	JoinAlreadyRegistered   JoinError = "already_registered"
	JoinAnonymousNotAllowed JoinError = "anonymous_not_allowed"
)

// Label returns the Indonesian rejection copy.
func (e JoinError) Label() string {
	switch e {
	case JoinUnknownCode:
		return "Kode kuis tidak ditemukan."
	case JoinNotOpen:
		return "Kuis ini belum dibuka atau sudah ditutup."
	case JoinWrongAudience:
		return "Kuis ini hanya untuk kelas dan jurusan tertentu."
	case JoinAlreadyRegistered:
		return "Anda sudah terdaftar di kuis ini."
	case JoinAnonymousNotAllowed:
		return "Kuis privat hanya dapat diikuti oleh siswa yang masuk."
	default:
		return labelUnknown
	}
}

// SortOrder is the closed list ordering set.
type SortOrder string

const (
	SortNewest   SortOrder = "newest"
	SortOldest   SortOrder = "oldest"
	SortNameAsc  SortOrder = "name_asc"
	SortNameDesc SortOrder = "name_desc"
)

// Label returns the Indonesian display label.
func (s SortOrder) Label() string {
	switch s {
	case SortNewest:
		return "Terbaru"
	case SortOldest:
		return "Terlama"
	case SortNameAsc:
		return "Nama A–Z"
	case SortNameDesc:
		return "Nama Z–A"
	default:
		return labelUnknown
	}
}

// PendingState is the teacher-approval holding-page state.
type PendingState string

const (
	PendingWaiting  PendingState = "waiting"
	PendingApproved PendingState = "approved"
	PendingExpired  PendingState = "expired"
)

// Label returns the Indonesian display label.
func (p PendingState) Label() string {
	switch p {
	case PendingWaiting:
		return "Menunggu persetujuan"
	case PendingApproved:
		return "Disetujui"
	case PendingExpired:
		return "Kedaluwarsa"
	default:
		return labelUnknown
	}
}

const labelUnknown = "Tidak dikenal"

// LabelUnknown is exported for the conformance suite: a known enum value must
// never render it.
const LabelUnknown = labelUnknown

// The following slices are the closed sets, in display order. Tests iterate
// them; handlers never need to.
var (
	AllRoles              = []Role{RoleTeacher, RoleStudent, RoleAnonymous}
	AllAccountStatuses    = []AccountStatus{AccountPending, AccountActive}
	AllQuizStates         = []QuizState{QuizDraft, QuizOpen, QuizClosed}
	AllVisibilities       = []Visibility{VisibilityPublic, VisibilityPrivate}
	AllQuestionTypes      = []QuestionType{QuestionSingle, QuestionMultiple, QuestionEssay}
	AllParticipationState = []ParticipationState{ParticipationRegistered, ParticipationInProgress, ParticipationCompleted}
	AllAttemptStates      = []AttemptState{AttemptInProgress, AttemptCompleted}
	AllReviewModes        = []ReviewMode{ReviewQuestionsNoVerdict, ReviewQuestionsNoAnswers, ReviewCorrectOnly, ReviewQuestionsWithKey}
	AllEssayStates        = []EssayState{EssayUngraded, EssayGraded}
	AllJoinErrors         = []JoinError{JoinUnknownCode, JoinNotOpen, JoinWrongAudience, JoinAlreadyRegistered, JoinAnonymousNotAllowed}
	AllSortOrders         = []SortOrder{SortNewest, SortOldest, SortNameAsc, SortNameDesc}
	AllPendingStates      = []PendingState{PendingWaiting, PendingApproved, PendingExpired}
)
