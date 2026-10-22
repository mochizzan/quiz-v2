// Package fixtures is the in-memory demo world behind the UI-only build. Every
// page reads the same people, quizzes, questions and attempts, so numbers on
// different screens agree with each other. Nothing here is persisted.
package fixtures

import (
	"strconv"
	"strings"
	"time"

	"quiz-v2/internal/viewmodel"
)

// Stable ids used by the route builders. Keeping them named avoids six
// different pages disagreeing about which quiz is "the" open one.
const (
	MajorIPA    int64 = 1
	MajorIPS    int64 = 2
	MajorBahasa int64 = 3
)

const (
	TeacherRatna int64 = 1
	TeacherAdi   int64 = 2
	TeacherSari  int64 = 3
)

const (
	StudentAyu    int64 = 1
	StudentBima   int64 = 2
	StudentCitra  int64 = 3
	StudentDewa   int64 = 4
	StudentEka    int64 = 5
	StudentFajar  int64 = 6
	StudentGita   int64 = 7
	StudentHana   int64 = 8
	StudentIndra  int64 = 9
	StudentJoko   int64 = 10
	StudentKirana int64 = 11
	StudentLukman int64 = 12
)

const (
	QuizBiologi    int64 = 1
	QuizFisika     int64 = 2
	QuizKimia      int64 = 3
	QuizSejarah    int64 = 4
	QuizMatematika int64 = 5
	QuizBahasa     int64 = 6
	QuizIPATerpadu int64 = 7
	QuizSeni       int64 = 8
	QuizPancasila  int64 = 9
	QuizGeografi   int64 = 10
)

// Major is one academic major.
type Major struct {
	ID   int64
	Name string
}

// Teacher is one teacher account.
type Teacher struct {
	ID         int64
	FullName   string
	Email      string
	Status     viewmodel.AccountStatus
	ApprovedBy string
	ApprovedAt *viewmodel.LocalTime
}

// Student is one student account.
type Student struct {
	ID         int64
	FullName   string
	Email      string
	GradeLevel int
	MajorID    int64
}

// Option is one answer option. Index in the parent slice is the stored key.
type Option struct {
	ID   int64
	Body string
}

// Question is one question in the demo world.
type Question struct {
	ID       int64
	QuizID   int64
	Position int
	Type     viewmodel.QuestionType
	Body     string
	Image    *viewmodel.QuestionImage
	Options  []Option
	Correct  []int
}

// Quiz is one quiz. QuestionCount is derived through World.Questions.
type Quiz struct {
	ID                 int64
	Title              string
	Description        string
	TeacherID          int64
	State              viewmodel.QuizState
	Visibility         viewmodel.Visibility
	TargetGrade        int
	TargetMajorID      int64
	Code               *string
	InviteToken        string
	StartsAt           *viewmodel.LocalTime
	EndsAt             *viewmodel.LocalTime
	CreatedAt          viewmodel.LocalTime
	MaxAttempts        int
	ReviewMode         viewmodel.ReviewMode
	ShowScore          bool
	RandomizeQuestions bool
	RandomizeAnswers   bool
	AllowRetake        bool
}

// Answer is one saved answer. Selected is empty for essays.
type Answer struct {
	ID          int64
	QuestionID  int64
	Selected    []int
	Essay       string
	Score       *viewmodel.Score
	Feedback    string
	Graded      bool
	SubmittedAt *viewmodel.LocalTime
}

// Attempt is one student attempt.
type Attempt struct {
	ID             int64
	RegistrationID int64
	Number         int
	StartedAt      viewmodel.LocalTime
	SubmittedAt    *viewmodel.LocalTime
	Final          *viewmodel.Score
	Waiting        bool
	FocusLoss      int
	Answers        []*Answer
}

// Registration is one participant in one quiz. Anonymous participants have
// StudentID 0 and carry their name and class directly.
type Registration struct {
	ID         int64
	QuizID     int64
	StudentID  int64
	Anonymous  bool
	FullName   string
	GradeLevel int
	MajorID    int64
	JoinedAt   viewmodel.LocalTime
	Attempts   []*Attempt
}

// World is the whole demo dataset.
type World struct {
	Majors        []Major
	Teachers      []Teacher
	Students      []Student
	Quizzes       []Quiz
	Questions     []*Question
	Registrations []*Registration

	byQuiz   map[int64]*Quiz
	byID     map[int64]*Question
	byAnswer map[int64]*Answer
	byAtt    map[int64]*Attempt
}

// Default returns the singleton demo world. The data is immutable by
// convention; page fixtures only read it.
func Default() *World { return defaultWorld }

var defaultWorld = buildWorld()

// TeacherUser is the signed-in teacher used by every teacher screen (Ratna).
func TeacherUser() *viewmodel.CurrentUser {
	t := Default().Teacher(TeacherRatna)
	return &viewmodel.CurrentUser{
		ID:       t.ID,
		FullName: t.FullName,
		Email:    t.Email,
		Role:     viewmodel.RoleTeacher,
		Initials: viewmodel.Initials(t.FullName),
	}
}

// StudentUser is the signed-in student used by every student screen (Ayu).
func StudentUser() *viewmodel.CurrentUser {
	s := Default().Student(StudentAyu)
	return &viewmodel.CurrentUser{
		ID:         s.ID,
		FullName:   s.FullName,
		Email:      s.Email,
		Role:       viewmodel.RoleStudent,
		GradeLevel: s.GradeLevel,
		MajorName:  Default().MajorName(s.MajorID),
		Initials:   viewmodel.Initials(s.FullName),
	}
}

// ParticipationOf derives the badge state for one student in one quiz. A
// missing registration returns the zero state; pages use the registration
// pointer to decide which action to offer.
func (w *World) ParticipationOf(quizID, studentID int64) (viewmodel.ParticipationState, *Registration) {
	r := w.RegistrationFor(quizID, studentID)
	if r == nil {
		return "", nil
	}
	if len(r.Attempts) == 0 {
		return viewmodel.ParticipationRegistered, r
	}
	for _, a := range r.Attempts {
		if a.SubmittedAt == nil {
			return viewmodel.ParticipationInProgress, r
		}
	}
	return viewmodel.ParticipationCompleted, r
}

// ---- Lookups --------------------------------------------------------------

func (w *World) MajorName(id int64) string {
	for _, m := range w.Majors {
		if m.ID == id {
			return m.Name
		}
	}
	return ""
}

func (w *World) Major(id int64) *Major {
	for i := range w.Majors {
		if w.Majors[i].ID == id {
			return &w.Majors[i]
		}
	}
	return nil
}

func (w *World) Teacher(id int64) *Teacher {
	for i := range w.Teachers {
		if w.Teachers[i].ID == id {
			return &w.Teachers[i]
		}
	}
	return nil
}

func (w *World) TeacherName(id int64) string {
	if t := w.Teacher(id); t != nil {
		return t.FullName
	}
	return "—"
}

func (w *World) Student(id int64) *Student {
	for i := range w.Students {
		if w.Students[i].ID == id {
			return &w.Students[i]
		}
	}
	return nil
}

func (w *World) StudentName(id int64) string {
	if s := w.Student(id); s != nil {
		return s.FullName
	}
	return "—"
}

func (w *World) ClassOf(grade int, majorID int64) string {
	return viewmodel.ClassLabel(grade, w.MajorName(majorID))
}

func (w *World) StudentClass(id int64) string {
	s := w.Student(id)
	if s == nil {
		return "—"
	}
	return w.ClassOf(s.GradeLevel, s.MajorID)
}

func (w *World) Quiz(id int64) *Quiz { return w.byQuiz[id] }

func (w *World) QuizByCode(code string) *Quiz {
	for i := range w.Quizzes {
		if w.Quizzes[i].Code != nil && *w.Quizzes[i].Code == code {
			return &w.Quizzes[i]
		}
	}
	return nil
}

func (w *World) QuizByInvite(token string) *Quiz {
	for i := range w.Quizzes {
		if strings.EqualFold(w.Quizzes[i].InviteToken, token) {
			return &w.Quizzes[i]
		}
	}
	return nil
}

func (w *World) Question(id int64) *Question { return w.byID[id] }

func (w *World) QuestionsFor(quizID int64) []*Question {
	out := make([]*Question, 0, 5)
	for _, q := range w.Questions {
		if q.QuizID == quizID {
			out = append(out, q)
		}
	}
	return out
}

func (w *World) QuestionCount(quizID int64) int { return len(w.QuestionsFor(quizID)) }

func (w *World) RegistrationsFor(quizID int64) []*Registration {
	out := make([]*Registration, 0, 8)
	for _, r := range w.Registrations {
		if r.QuizID == quizID {
			out = append(out, r)
		}
	}
	return out
}

func (w *World) RegistrationFor(quizID, studentID int64) *Registration {
	for _, r := range w.Registrations {
		if r.QuizID == quizID && r.StudentID == studentID {
			return r
		}
	}
	return nil
}

func (w *World) Registration(id int64) *Registration {
	for _, r := range w.Registrations {
		if r.ID == id {
			return r
		}
	}
	return nil
}

func (w *World) Attempt(id int64) *Attempt { return w.byAtt[id] }

func (w *World) Answer(id int64) *Answer { return w.byAnswer[id] }

// FindAnswer returns the attempt owning an answer, for the grading detail.
func (w *World) FindAnswer(id int64) (*Attempt, *Answer) {
	a := w.byAnswer[id]
	if a == nil {
		return nil, nil
	}
	for _, att := range w.byAtt {
		for _, candidate := range att.Answers {
			if candidate.ID == id {
				return att, a
			}
		}
	}
	return nil, a
}

func (w *World) AttemptsFor(quizID int64) []*Attempt {
	var out []*Attempt
	for _, r := range w.RegistrationsFor(quizID) {
		out = append(out, r.Attempts...)
	}
	return out
}

func (w *World) RegistrationOfAttempt(attempt *Attempt) *Registration {
	if attempt == nil {
		return nil
	}
	return w.Registration(attempt.RegistrationID)
}

func (w *World) DisplayName(r *Registration) string {
	if r == nil {
		return "—"
	}
	if r.Anonymous {
		return r.FullName
	}
	return w.StudentName(r.StudentID)
}

func (w *World) RegistrationClass(r *Registration) string {
	if r == nil {
		return "—"
	}
	return w.ClassOf(r.GradeLevel, r.MajorID)
}

func (w *World) Audience(q *Quiz) string {
	if q == nil {
		return "—"
	}
	if q.TargetGrade == 0 && q.TargetMajorID == 0 {
		return "Semua kelas"
	}
	if q.TargetGrade == 0 {
		return "Semua tingkat · " + w.MajorName(q.TargetMajorID)
	}
	return w.ClassOf(q.TargetGrade, q.TargetMajorID)
}

// Eligible reports whether a student matches the quiz audience. Public quizzes
// with no target accept everyone; private quizzes must match exactly.
func (w *World) Eligible(q *Quiz, student *Student) bool {
	if q == nil || student == nil {
		return false
	}
	if q.Visibility == viewmodel.VisibilityPublic && q.TargetGrade == 0 && q.TargetMajorID == 0 {
		return true
	}
	if q.TargetGrade != 0 && student.GradeLevel != q.TargetGrade {
		return false
	}
	if q.TargetMajorID != 0 && student.MajorID != q.TargetMajorID {
		return false
	}
	return true
}

// CorrectLabel renders the answer key: "B", "A, C, D" or "Penilaian esai".
func CorrectLabel(q *Question) string {
	if q == nil || q.Type == viewmodel.QuestionEssay {
		return "Penilaian esai"
	}
	letters := make([]string, 0, len(q.Correct))
	for _, i := range q.Correct {
		letters = append(letters, viewmodel.OptionLetter(i))
	}
	return strings.Join(letters, ", ")
}

// OptionBody returns the option text for a key index.
func OptionBody(q *Question, index int) string {
	if q == nil || index < 0 || index >= len(q.Options) {
		return "—"
	}
	return q.Options[index].Body
}

// AnswerLabel renders one saved answer as option letters, an essay excerpt, or
// "Tidak dijawab".
func AnswerLabel(q *Question, a *Answer) string {
	if q == nil || a == nil {
		return "Tidak dijawab"
	}
	if q.Type == viewmodel.QuestionEssay {
		if strings.TrimSpace(a.Essay) == "" {
			return "Tidak dijawab"
		}
		return a.Essay
	}
	if len(a.Selected) == 0 {
		return "Tidak dijawab"
	}
	letters := make([]string, 0, len(a.Selected))
	for _, i := range a.Selected {
		if i >= 0 && i < len(q.Options) {
			letters = append(letters, viewmodel.OptionLetter(i))
		}
	}
	return strings.Join(letters, ", ")
}

// AttemptState derives the attempt lifecycle from its submission time.
func AttemptState(a *Attempt) viewmodel.AttemptState {
	if a == nil || a.SubmittedAt == nil {
		return viewmodel.AttemptInProgress
	}
	return viewmodel.AttemptCompleted
}

// BestScore returns the highest final score, ignoring waiting attempts.
func BestScore(r *Registration) *viewmodel.Score {
	var best *viewmodel.Score
	for _, a := range r.Attempts {
		if a.Final == nil {
			continue
		}
		if best == nil || a.Final.Percent > best.Percent {
			best = a.Final
		}
	}
	return best
}

// LastSubmitted returns the newest submitted attempt.
func LastSubmitted(r *Registration) *Attempt {
	var last *Attempt
	for _, a := range r.Attempts {
		if a.SubmittedAt == nil {
			continue
		}
		if last == nil || a.SubmittedAt.Time().After(last.SubmittedAt.Time()) {
			last = a
		}
	}
	return last
}

func (w *World) AttemptHeader(a *Attempt) viewmodel.AttemptHeader {
	r := w.RegistrationOfAttempt(a)
	q := w.Quiz(r.QuizID)
	return viewmodel.AttemptHeader{
		ID:          a.ID,
		QuizID:      q.ID,
		QuizTitle:   q.Title,
		StudentName: w.DisplayName(r),
		ClassLabel:  w.RegistrationClass(r),
		StartedAt:   a.StartedAt,
		AttemptNo:   a.Number,
		MaxAttempts: q.MaxAttempts,
	}
}

// AttemptRows builds the history rows for one registration.
func (w *World) AttemptRows(r *Registration) []viewmodel.AttemptRow {
	out := make([]viewmodel.AttemptRow, 0, len(r.Attempts))
	for _, a := range r.Attempts {
		row := viewmodel.AttemptRow{
			ID:          a.ID,
			Number:      a.Number,
			MaxAttempts: w.Quiz(r.QuizID).MaxAttempts,
			StartedAt:   viewmodel.Ptr(a.StartedAt),
			SubmittedAt: a.SubmittedAt,
			Score:       a.Final,
			State:       AttemptState(a),
			Waiting:     a.Waiting,
			FocusLoss:   a.FocusLoss,
			ResultHref:  viewmodel.ResultHref(a.ID),
			ReviewHref:  "/attempt/" + strconv.FormatInt(a.ID, 10) + "/review",
		}
		out = append(out, row)
	}
	return out
}

// Card is the viewer-independent part of a quiz card. Page builders add the
// participation state, actions and (for students) attempts used.
func (w *World) Card(q *Quiz) viewmodel.QuizCard {
	return viewmodel.QuizCard{
		ID:            q.ID,
		Title:         q.Title,
		Description:   q.Description,
		TeacherName:   w.TeacherName(q.TeacherID),
		QuestionCount: w.QuestionCount(q.ID),
		State:         q.State,
		Visibility:    q.Visibility,
		Audience:      w.Audience(q),
		Code:          q.Code,
		StartsAt:      q.StartsAt,
		EndsAt:        q.EndsAt,
		MaxAttempts:   q.MaxAttempts,
		ReviewMode:    q.ReviewMode,
		ShowScore:     q.ShowScore,
	}
}

// JoinedCount is the registered-participant count for a quiz.
func (w *World) JoinedCount(quizID int64) int { return len(w.RegistrationsFor(quizID)) }

// TopScore returns the best submitted score and its holder, or nil.
func (w *World) TopScore(quizID int64) *viewmodel.QuizTopScore {
	var top *viewmodel.QuizTopScore
	for _, r := range w.RegistrationsFor(quizID) {
		for _, a := range r.Attempts {
			if a.Final == nil {
				continue
			}
			if top == nil || a.Final.Percent > top.Score.Percent {
				top = &viewmodel.QuizTopScore{
					Quiz:        viewmodel.QuizRef{ID: quizID, Title: w.Quiz(quizID).Title},
					StudentName: w.DisplayName(r),
					ClassLabel:  w.RegistrationClass(r),
					Score:       *a.Final,
					Href:        "/teacher/quizzes/" + strconv.FormatInt(quizID, 10) + "/results",
				}
			}
		}
	}
	return top
}

// ScorePtr is a convenience for optional model fields.
func ScorePtr(percent int) *viewmodel.Score {
	s := viewmodel.NewScore(float64(percent))
	return &s
}

// QuizRef is a convenience for headers.
func (w *World) QuizRef(id int64) viewmodel.QuizRef {
	q := w.Quiz(id)
	if q == nil {
		return viewmodel.QuizRef{}
	}
	return viewmodel.QuizRef{ID: q.ID, Title: q.Title}
}

// ---- Builders -------------------------------------------------------------

func buildWorld() *World {
	w := &World{
		Majors: []Major{
			{ID: MajorIPA, Name: "IPA"},
			{ID: MajorIPS, Name: "IPS"},
			{ID: MajorBahasa, Name: "Bahasa"},
		},
		Teachers: []Teacher{
			{ID: TeacherRatna, FullName: "Ratna Wulandari", Email: "ratna@twisty.sch.id", Status: viewmodel.AccountActive, ApprovedBy: "Kepala Sekolah", ApprovedAt: viewmodel.LocalDatePtr(2026, time.January, 12, 9, 0)},
			{ID: TeacherAdi, FullName: "Adi Nugroho", Email: "adi@twisty.sch.id", Status: viewmodel.AccountActive, ApprovedBy: "Ratna Wulandari", ApprovedAt: viewmodel.LocalDatePtr(2026, time.February, 3, 10, 30)},
			{ID: TeacherSari, FullName: "Sari Puspita", Email: "sari@twisty.sch.id", Status: viewmodel.AccountPending},
		},
		Students: buildStudents,
	}

	w.Quizzes = []Quiz{
		{
			ID: QuizBiologi, Title: "Biologi Sel — Kuis 4",
			Description: "Uji pemahaman tentang struktur sel, organel, dan proses transportasi membran.",
			TeacherID:   TeacherRatna, State: viewmodel.QuizOpen, Visibility: viewmodel.VisibilityPublic,
			Code: strPtr("482915"), InviteToken: "bio4-7f2k",
			StartsAt:    viewmodel.LocalDatePtr(2026, time.October, 23, 8, 0),
			EndsAt:      viewmodel.LocalDatePtr(2026, time.October, 23, 12, 0),
			CreatedAt:   viewmodel.LocalDate(2026, time.October, 18, 9, 15),
			MaxAttempts: 2, ReviewMode: viewmodel.ReviewQuestionsAndKey, ShowScore: true,
			RandomizeQuestions: true, RandomizeAnswers: false, AllowRetake: true,
		},
		{
			ID: QuizFisika, Title: "Fisika Kinematika — Kuis 3",
			Description: "Gerak lurus, vektor, dan gerak parabola untuk kelas XII IPA.",
			TeacherID:   TeacherRatna, State: viewmodel.QuizClosed, Visibility: viewmodel.VisibilityPrivate,
			TargetGrade: 12, TargetMajorID: MajorIPA, InviteToken: "fis3-3m9q",
			StartsAt:    viewmodel.LocalDatePtr(2026, time.October, 20, 7, 30),
			EndsAt:      viewmodel.LocalDatePtr(2026, time.October, 20, 9, 30),
			CreatedAt:   viewmodel.LocalDate(2026, time.October, 14, 13, 0),
			MaxAttempts: 2, ReviewMode: viewmodel.ReviewQuestionsOnly, ShowScore: true,
			RandomizeQuestions: false, RandomizeAnswers: true, AllowRetake: true,
		},
		{
			ID: QuizKimia, Title: "Kimia Organik — Kuis 2",
			Description: "Tata nama, gugus fungsi, dan ikatan pada senyawa karbon.",
			TeacherID:   TeacherAdi, State: viewmodel.QuizDraft, Visibility: viewmodel.VisibilityPrivate,
			TargetGrade: 11, TargetMajorID: MajorIPA, InviteToken: "kim2-1p8r",
			CreatedAt:   viewmodel.LocalDate(2026, time.October, 21, 15, 20),
			MaxAttempts: 3, ReviewMode: viewmodel.ReviewAnswersOnly, ShowScore: false,
			RandomizeQuestions: false, RandomizeAnswers: false, AllowRetake: true,
		},
		{
			ID: QuizSejarah, Title: "Sejarah Indonesia — Kuis 1",
			Description: "Proklamasi, sidang PPKI, dan peristiwa menjelang kemerdekaan.",
			TeacherID:   TeacherRatna, State: viewmodel.QuizOpen, Visibility: viewmodel.VisibilityPublic,
			Code: strPtr("671203"), InviteToken: "sej1-6c4v",
			StartsAt:    viewmodel.LocalDatePtr(2026, time.October, 22, 10, 0),
			EndsAt:      viewmodel.LocalDatePtr(2026, time.October, 24, 23, 59),
			CreatedAt:   viewmodel.LocalDate(2026, time.October, 10, 8, 45),
			MaxAttempts: 3, ReviewMode: viewmodel.ReviewKeyOnly, ShowScore: true,
			RandomizeQuestions: false, RandomizeAnswers: false, AllowRetake: true,
		},
		{
			ID: QuizMatematika, Title: "Matematika Wajib — Kuis 5",
			Description: "Persamaan linear, gradien, dan operasi aljabar dasar.",
			TeacherID:   TeacherAdi, State: viewmodel.QuizOpen, Visibility: viewmodel.VisibilityPublic,
			Code: strPtr("731204"), InviteToken: "mat5-5v2c",
			StartsAt:    viewmodel.LocalDatePtr(2026, time.October, 23, 7, 0),
			EndsAt:      viewmodel.LocalDatePtr(2026, time.October, 25, 20, 0),
			CreatedAt:   viewmodel.LocalDate(2026, time.October, 22, 11, 10),
			MaxAttempts: 1, ReviewMode: viewmodel.ReviewQuestionsOnly, ShowScore: true,
			RandomizeQuestions: true, RandomizeAnswers: true, AllowRetake: false,
		},
		{
			ID: QuizBahasa, Title: "Bahasa Indonesia — Kuis 6",
			Description: "Ide pokok, jenis teks, dan kata baku.",
			TeacherID:   TeacherRatna, State: viewmodel.QuizOpen, Visibility: viewmodel.VisibilityPrivate,
			TargetGrade: 12, TargetMajorID: MajorIPS, InviteToken: "bhs6-9x2p",
			StartsAt:    viewmodel.LocalDatePtr(2026, time.October, 21, 9, 0),
			EndsAt:      viewmodel.LocalDatePtr(2026, time.October, 24, 15, 0),
			CreatedAt:   viewmodel.LocalDate(2026, time.October, 20, 14, 0),
			MaxAttempts: 2, ReviewMode: viewmodel.ReviewQuestionsAndKey, ShowScore: true,
			RandomizeQuestions: false, RandomizeAnswers: true, AllowRetake: true,
		},
		{
			ID: QuizIPATerpadu, Title: "IPA Terpadu — Kuis 7",
			Description: "Draf kuis lintas disiplin untuk kelas X.",
			TeacherID:   TeacherAdi, State: viewmodel.QuizDraft, Visibility: viewmodel.VisibilityPublic,
			Code: strPtr("609127"), InviteToken: "ipa7-4n3b",
			CreatedAt:   viewmodel.LocalDate(2026, time.October, 23, 7, 30),
			MaxAttempts: 1, ReviewMode: viewmodel.ReviewQuestionsAndKey, ShowScore: true,
			RandomizeQuestions: false, RandomizeAnswers: false, AllowRetake: false,
		},
		{
			ID: QuizSeni, Title: "Seni Budaya — Kuis 8",
			Description: "Unsur rupa, teknik berkarya, dan seni daerah.",
			TeacherID:   TeacherRatna, State: viewmodel.QuizClosed, Visibility: viewmodel.VisibilityPublic,
			Code: strPtr("514862"), InviteToken: "sen8-6t1d",
			StartsAt:    viewmodel.LocalDatePtr(2026, time.October, 15, 8, 0),
			EndsAt:      viewmodel.LocalDatePtr(2026, time.October, 15, 11, 0),
			CreatedAt:   viewmodel.LocalDate(2026, time.October, 5, 10, 0),
			MaxAttempts: 1, ReviewMode: viewmodel.ReviewQuestionsAndKey, ShowScore: false,
			RandomizeQuestions: false, RandomizeAnswers: false, AllowRetake: false,
		},
		{
			ID: QuizPancasila, Title: "Pendidikan Pancasila — Kuis 2",
			Description: "Sejarah perumusan Pancasila dan penerapan sila dalam kehidupan sekolah.",
			TeacherID:   TeacherAdi, State: viewmodel.QuizOpen, Visibility: viewmodel.VisibilityPublic,
			Code: strPtr("905318"), InviteToken: "pan2-8j5k",
			StartsAt:    viewmodel.LocalDatePtr(2026, time.October, 22, 8, 0),
			EndsAt:      viewmodel.LocalDatePtr(2026, time.October, 26, 18, 0),
			CreatedAt:   viewmodel.LocalDate(2026, time.October, 19, 16, 0),
			MaxAttempts: 2, ReviewMode: viewmodel.ReviewQuestionsOnly, ShowScore: true,
			RandomizeQuestions: true, RandomizeAnswers: false, AllowRetake: true,
		},
		{
			ID: QuizGeografi, Title: "Geografi — Kuis 1",
			Description: "Atmosfer, cuaca, dan pengaruh letak geografis Indonesia.",
			TeacherID:   TeacherRatna, State: viewmodel.QuizOpen, Visibility: viewmodel.VisibilityPublic,
			Code: strPtr("260741"), InviteToken: "geo1-2r7m",
			StartsAt:    viewmodel.LocalDatePtr(2026, time.October, 23, 7, 0),
			EndsAt:      viewmodel.LocalDatePtr(2026, time.October, 23, 23, 59),
			CreatedAt:   viewmodel.LocalDate(2026, time.October, 23, 6, 45),
			MaxAttempts: 1, ReviewMode: viewmodel.ReviewQuestionsAndKey, ShowScore: true,
			RandomizeQuestions: true, RandomizeAnswers: true, AllowRetake: false,
		},
	}
	for i := range w.Quizzes {
		w.byQuizInit(&w.Quizzes[i])
	}

	w.Questions = buildQuestions()
	w.byID = make(map[int64]*Question, len(w.Questions))
	for _, q := range w.Questions {
		w.byID[q.ID] = q
	}

	w.Registrations = buildRegistrations()
	w.byAtt = make(map[int64]*Attempt)
	w.byAnswer = make(map[int64]*Answer)
	for _, r := range w.Registrations {
		for _, a := range r.Attempts {
			w.byAtt[a.ID] = a
			for _, ans := range a.Answers {
				w.byAnswer[ans.ID] = ans
			}
		}
	}
	return w
}

func (w *World) byQuizInit(q *Quiz) {
	if w.byQuiz == nil {
		w.byQuiz = make(map[int64]*Quiz)
	}
	w.byQuiz[q.ID] = q
}

func buildQuestions() []*Question {
	var out []*Question
	add := func(quizID int64, pos int, typ viewmodel.QuestionType, body string, options []string, correct []int) {
		q := &Question{ID: quizID*100 + int64(pos), QuizID: quizID, Position: pos, Type: typ, Body: body, Correct: correct}
		for i, o := range options {
			q.Options = append(q.Options, Option{ID: q.ID*10 + int64(i+1), Body: o})
		}
		out = append(out, q)
	}

	// Biologi Sel — 4 objective + 1 essay.
	add(QuizBiologi, 1, viewmodel.QuestionSingle, "Organel yang berfungsi sebagai tempat respirasi sel untuk menghasilkan energi adalah …", []string{"Nukleus", "Mitokondria", "Ribosom", "Lisosom"}, []int{1})
	add(QuizBiologi, 2, viewmodel.QuestionSingle, "Membran sel bersifat … karena hanya melewatkan molekul tertentu.", []string{"Impermeabel", "Semipermeabel", "Permeabel", "Kaku"}, []int{1})
	add(QuizBiologi, 3, viewmodel.QuestionMultiple, "Manakah yang termasuk organel bermembran?", []string{"Mitokondria", "Ribosom", "Retikulum endoplasma", "Lisosom"}, []int{0, 2, 3})
	add(QuizBiologi, 4, viewmodel.QuestionSingle, "Pembelahan sel yang menghasilkan dua sel anak identik disebut …", []string{"Mitosis", "Meiosis", "Amitosis", "Fragmentasi"}, []int{0})
	add(QuizBiologi, 5, viewmodel.QuestionEssay, "Jelaskan perbedaan respirasi aerob dan anaerob.", nil, nil)

	// Fisika Kinematika — 4 objective + 1 essay.
	add(QuizFisika, 1, viewmodel.QuestionSingle, "Besaran yang menyatakan perpindahan per satuan waktu adalah …", []string{"Kecepatan", "Percepatan", "Jarak", "Massa"}, []int{0})
	add(QuizFisika, 2, viewmodel.QuestionSingle, "Satuan SI untuk percepatan adalah …", []string{"m/s", "m/s²", "kg·m/s", "N"}, []int{1})
	add(QuizFisika, 3, viewmodel.QuestionMultiple, "Besaran berikut yang termasuk besaran vektor adalah …", []string{"Kecepatan", "Massa", "Perpindahan", "Waktu"}, []int{0, 2})
	add(QuizFisika, 4, viewmodel.QuestionSingle, "Percepatan gravitasi bumi di dekat permukaan kira-kira …", []string{"1,8 m/s²", "3,8 m/s²", "9,8 m/s²", "19,8 m/s²"}, []int{2})
	add(QuizFisika, 5, viewmodel.QuestionEssay, "Jelaskan bagaimana gerak parabola tersusun dari dua gerak yang saling bebas.", nil, nil)

	// Kimia Organik — 4 objective (draft).
	add(QuizKimia, 1, viewmodel.QuestionSingle, "Rumus umum senyawa alkana adalah …", []string{"CnH2n", "CnH2n+2", "CnH2n−2", "CnHn"}, []int{1})
	add(QuizKimia, 2, viewmodel.QuestionSingle, "Gugus fungsi aldehida adalah …", []string{"–OH", "–COOH", "–CHO", "–CO–"}, []int{2})
	add(QuizKimia, 3, viewmodel.QuestionSingle, "Ikatan karbon–karbon pada etena adalah …", []string{"Tunggal", "Rangkap dua", "Rangkap tiga", "Aromatik"}, []int{1})
	add(QuizKimia, 4, viewmodel.QuestionSingle, "Manakah yang termasuk hidrokarbon tak jenuh?", []string{"Etana", "Etena", "Propana", "Metana"}, []int{1})

	// Sejarah Indonesia — 3 objective + 2 essays.
	add(QuizSejarah, 1, viewmodel.QuestionSingle, "Proklamasi kemerdekaan Indonesia dibacakan pada …", []string{"17 Agustus 1945", "18 Agustus 1945", "1 Juni 1945", "28 Oktober 1928"}, []int{0})
	add(QuizSejarah, 2, viewmodel.QuestionSingle, "Tokoh yang membacakan teks proklamasi adalah …", []string{"Mohammad Hatta", "Soekarno", "Sutan Sjahrir", "Ahmad Subardjo"}, []int{1})
	add(QuizSejarah, 3, viewmodel.QuestionMultiple, "Hasil sidang PPKI pada 18 Agustus 1945 meliputi …", []string{"Mengesahkan UUD 1945", "Memilih presiden dan wakil presiden", "Membentuk KNIP", "Menetapkan tanggal pemilu"}, []int{0, 1, 2})
	add(QuizSejarah, 4, viewmodel.QuestionEssay, "Jelaskan makna proklamasi kemerdekaan bagi bangsa Indonesia.", nil, nil)
	add(QuizSejarah, 5, viewmodel.QuestionEssay, "Mengapa peristiwa Rengasdengklok terjadi?", nil, nil)

	// Matematika Wajib — 4 objective + 1 multiple + 1 essay.
	add(QuizMatematika, 1, viewmodel.QuestionSingle, "Nilai x dari 2x + 5 = 17 adalah …", []string{"5", "6", "7", "8"}, []int{1})
	add(QuizMatematika, 2, viewmodel.QuestionSingle, "Gradien garis y = 3x − 4 adalah …", []string{"−4", "3", "4", "−3"}, []int{1})
	add(QuizMatematika, 3, viewmodel.QuestionSingle, "Hasil dari (x + 2)(x − 3) adalah …", []string{"x² − x − 6", "x² + x − 6", "x² − 5x − 6", "x² − 6"}, []int{0})
	add(QuizMatematika, 4, viewmodel.QuestionSingle, "Himpunan penyelesaian x² − 4 = 0 adalah …", []string{"{−2, 2}", "{−4, 4}", "{2}", "{0, 4}"}, []int{0})
	add(QuizMatematika, 5, viewmodel.QuestionMultiple, "Manakah yang merupakan persamaan linear satu variabel?", []string{"2x + 5 = 11", "3y − 4 = 8", "x² + 1 = 0", "2/x = 4"}, []int{0, 1})
	add(QuizMatematika, 6, viewmodel.QuestionEssay, "Jelaskan langkah menyelesaikan sistem persamaan linear dua variabel dengan metode substitusi.", nil, nil)

	// Bahasa Indonesia — 3 objective + 1 essay.
	add(QuizBahasa, 1, viewmodel.QuestionSingle, "Ide pokok paragraf biasanya terdapat pada …", []string{"Kalimat utama", "Kalimat penjelas", "Kata hubung", "Judul"}, []int{0})
	add(QuizBahasa, 2, viewmodel.QuestionSingle, "Teks yang bertujuan memengaruhi pembaca disebut …", []string{"Narasi", "Deskripsi", "Persuasi", "Eksposisi"}, []int{2})
	add(QuizBahasa, 3, viewmodel.QuestionSingle, "Kata baku yang benar adalah …", []string{"analisa", "praktek", "apotek", "jaman"}, []int{2})
	add(QuizBahasa, 4, viewmodel.QuestionEssay, "Tuliskan satu paragraf argumentasi tentang pentingnya membaca.", nil, nil)

	// Seni Budaya — 4 objective + 1 essay.
	add(QuizSeni, 1, viewmodel.QuestionSingle, "Warna primer dalam seni rupa adalah …", []string{"Merah, kuning, biru", "Merah, hijau, biru", "Putih, hitam, abu-abu", "Jingga, hijau, ungu"}, []int{0})
	add(QuizSeni, 2, viewmodel.QuestionSingle, "Teknik melukis dengan titik-titik disebut …", []string{"Pointilis", "Aquarel", "Plakat", "Dussel"}, []int{0})
	add(QuizSeni, 3, viewmodel.QuestionSingle, "Bahan utama membuat patung teknik butsir adalah …", []string{"Tanah liat", "Kayu", "Batu", "Logam"}, []int{0})
	add(QuizSeni, 4, viewmodel.QuestionSingle, "Lagu daerah Bubuy Bulan berasal dari …", []string{"Jawa Barat", "Jawa Tengah", "Jawa Timur", "Bali"}, []int{0})
	add(QuizSeni, 5, viewmodel.QuestionEssay, "Jelaskan perbedaan seni rupa dua dimensi dan tiga dimensi.", nil, nil)

	// Pendidikan Pancasila — 4 objective + 1 essay.
	add(QuizPancasila, 1, viewmodel.QuestionSingle, "Pancasila disahkan sebagai dasar negara pada …", []string{"1 Juni 1945", "18 Agustus 1945", "17 Agustus 1945", "22 Juni 1945"}, []int{1})
	add(QuizPancasila, 2, viewmodel.QuestionSingle, "Sila pertama Pancasila berbunyi …", []string{"Ketuhanan Yang Maha Esa", "Kemanusiaan yang adil dan beradab", "Persatuan Indonesia", "Keadilan sosial bagi seluruh rakyat Indonesia"}, []int{0})
	add(QuizPancasila, 3, viewmodel.QuestionSingle, "Manakah yang mencerminkan sila ketiga?", []string{"Menghormati perbedaan agama", "Ikut kerja bakti membersihkan lingkungan", "Membantu korban bencana", "Mengikuti musyawarah kelas"}, []int{1})
	add(QuizPancasila, 4, viewmodel.QuestionSingle, "Lembaga yang mengesahkan UUD 1945 adalah …", []string{"PPKI", "BPUPKI", "KNIP", "DPR"}, []int{0})
	add(QuizPancasila, 5, viewmodel.QuestionEssay, "Berikan satu contoh penerapan sila kelima di sekolah.", nil, nil)

	// Geografi — 3 objective + 1 essay.
	add(QuizGeografi, 1, viewmodel.QuestionSingle, "Alat yang digunakan untuk mengukur suhu adalah …", []string{"Barometer", "Termometer", "Higrometer", "Anemometer"}, []int{1})
	add(QuizGeografi, 2, viewmodel.QuestionSingle, "Lapisan atmosfer tempat terjadinya cuaca adalah …", []string{"Stratosfer", "Troposfer", "Mesosfer", "Termosfer"}, []int{1})
	add(QuizGeografi, 3, viewmodel.QuestionSingle, "Indonesia terletak di antara dua samudra, yaitu …", []string{"Pasifik dan Hindia", "Atlantik dan Hindia", "Pasifik dan Atlantik", "Arktik dan Hindia"}, []int{0})
	add(QuizGeografi, 4, viewmodel.QuestionEssay, "Jelaskan pengaruh letak geografis Indonesia terhadap iklimnya.", nil, nil)

	return out
}

func buildRegistrations() []*Registration {
	var out []*Registration
	nextReg := int64(0)
	reg := func(quizID int64, studentID int64, joined viewmodel.LocalTime) *Registration {
		nextReg++
		s := defaultWorldStudent(studentID)
		r := &Registration{ID: nextReg, QuizID: quizID, StudentID: studentID, JoinedAt: joined}
		if s != nil {
			r.GradeLevel, r.MajorID = s.GradeLevel, s.MajorID
		}
		out = append(out, r)
		return r
	}
	anon := func(quizID int64, name string, grade int, majorID int64, joined viewmodel.LocalTime) *Registration {
		nextReg++
		r := &Registration{ID: nextReg, QuizID: quizID, Anonymous: true, FullName: name, GradeLevel: grade, MajorID: majorID, JoinedAt: joined}
		out = append(out, r)
		return r
	}
	attempt := func(r *Registration, id int64, number int, started viewmodel.LocalTime, submitted *viewmodel.LocalTime, final *viewmodel.Score, waiting bool, focus int, answers ...*Answer) *Attempt {
		a := &Attempt{ID: id, RegistrationID: r.ID, Number: number, StartedAt: started, SubmittedAt: submitted, Final: final, Waiting: waiting, FocusLoss: focus, Answers: answers}
		r.Attempts = append(r.Attempts, a)
		return a
	}
	ans := func(id, questionID int64, selected []int) *Answer {
		return &Answer{ID: id, QuestionID: questionID, Selected: selected}
	}
	essay := func(id, questionID int64, text string, score *viewmodel.Score, feedback string) *Answer {
		graded := score != nil
		return &Answer{ID: id, QuestionID: questionID, Essay: text, Score: score, Feedback: feedback, Graded: graded}
	}

	// --- Biologi Sel (open, public): 10 participants ----------------------
	rAyuBio := reg(QuizBiologi, StudentAyu, viewmodel.LocalDate(2026, time.October, 23, 8, 2))
	attempt(rAyuBio, 1001, 1, viewmodel.LocalDate(2026, time.October, 23, 8, 3), viewmodel.LocalDatePtr(2026, time.October, 23, 8, 28), ScorePtr(84), false, 1,
		ans(100101, 101, []int{1}),
		ans(100102, 102, []int{1}),
		ans(100103, 103, []int{0, 2, 3}),
		ans(100104, 104, []int{0}),
		essay(100105, 105, "Respirasi aerob membutuhkan oksigen dan menghasilkan energi jauh lebih besar melalui glikolisis, siklus Krebs, dan rantai transpor elektron. Anaerob tidak membutuhkan oksigen dan hanya menghasilkan sedikit energi melalui fermentasi.", ScorePtr(20), "Konsep dasar sudah disebutkan, tetapi mekanisme pembentukan ATP belum dijelaskan."),
	)
	rBima := reg(QuizBiologi, StudentBima, viewmodel.LocalDate(2026, time.October, 23, 8, 5))
	attempt(rBima, 1002, 1, viewmodel.LocalDate(2026, time.October, 23, 8, 6), nil, nil, false, 1,
		ans(100201, 101, []int{1}),
		ans(100202, 102, []int{0}),
		ans(100203, 103, []int{0, 2}),
	)
	rCitra := reg(QuizBiologi, StudentCitra, viewmodel.LocalDate(2026, time.October, 23, 8, 8))
	_ = rCitra
	rDewa := reg(QuizBiologi, StudentDewa, viewmodel.LocalDate(2026, time.October, 23, 8, 10))
	attempt(rDewa, 1003, 1, viewmodel.LocalDate(2026, time.October, 23, 8, 11), nil, nil, false, 0,
		ans(100301, 101, []int{1}),
		ans(100302, 102, []int{1}),
		ans(100303, 103, []int{0, 2, 3}),
		ans(100304, 104, []int{0}),
	)
	rEka := reg(QuizBiologi, StudentEka, viewmodel.LocalDate(2026, time.October, 23, 8, 12))
	attempt(rEka, 1004, 1, viewmodel.LocalDate(2026, time.October, 23, 8, 13), viewmodel.LocalDatePtr(2026, time.October, 23, 8, 40), ScorePtr(92), false, 0)
	rFajar := reg(QuizBiologi, StudentFajar, viewmodel.LocalDate(2026, time.October, 23, 8, 14))
	_ = rFajar
	rGita := reg(QuizBiologi, StudentGita, viewmodel.LocalDate(2026, time.October, 23, 8, 15))
	attempt(rGita, 1005, 1, viewmodel.LocalDate(2026, time.October, 23, 8, 16), nil, nil, false, 2,
		ans(100501, 101, []int{1}),
		ans(100502, 102, []int{0}),
	)
	rHana := reg(QuizBiologi, StudentHana, viewmodel.LocalDate(2026, time.October, 23, 8, 18))
	attempt(rHana, 1006, 1, viewmodel.LocalDate(2026, time.October, 23, 8, 19), viewmodel.LocalDatePtr(2026, time.October, 23, 8, 51), ScorePtr(76), false, 0)
	rRizky := anon(QuizBiologi, "Rizky Maulana", 12, MajorIPA, viewmodel.LocalDate(2026, time.October, 23, 8, 20))
	attempt(rRizky, 1007, 1, viewmodel.LocalDate(2026, time.October, 23, 8, 21), viewmodel.LocalDatePtr(2026, time.October, 23, 8, 58), ScorePtr(68), false, 1)
	rSinta := anon(QuizBiologi, "Sinta Dewi", 11, MajorIPS, viewmodel.LocalDate(2026, time.October, 23, 8, 22))
	attempt(rSinta, 1008, 1, viewmodel.LocalDate(2026, time.October, 23, 8, 23), nil, nil, false, 0,
		ans(100801, 101, []int{1}),
	)
	attempt(rAyuBio, 1009, 2, viewmodel.LocalDate(2026, time.October, 23, 9, 5), nil, nil, false, 1,
		ans(100901, 101, []int{1}),
		ans(100903, 103, []int{0, 2}),
		essay(100905, 105, "Respirasi aerob memakai oksigen dan terjadi di mitokondria, sedangkan anaerob tidak memakai oksigen dan terjadi di sitoplasma.", nil, ""),
	)

	// --- Fisika Kinematika (closed, private XII IPA): 4 participants ------
	rAyu := reg(QuizFisika, StudentAyu, viewmodel.LocalDate(2026, time.October, 20, 7, 31))
	attempt(rAyu, 2001, 1, viewmodel.LocalDate(2026, time.October, 20, 7, 32), viewmodel.LocalDatePtr(2026, time.October, 20, 8, 2), ScorePtr(86), false, 0,
		ans(200101, 201, []int{0}),
		ans(200102, 202, []int{1}),
		ans(200103, 203, []int{0, 2}),
		ans(200104, 204, []int{2}),
		essay(200105, 205, "Gerak parabola tersusun dari gerak lurus beraturan pada arah horizontal dan gerak lurus berubah beraturan pada arah vertikal. Keduanya saling bebas sehingga waktu tempuh ditentukan oleh komponen vertikal.", ScorePtr(30), "Konsep dua gerak bebas sudah tepat; perjelas pengaruh sudut elevasi."),
	)
	attempt(rAyu, 2002, 2, viewmodel.LocalDate(2026, time.October, 20, 8, 10), viewmodel.LocalDatePtr(2026, time.October, 20, 8, 44), ScorePtr(72), false, 2,
		ans(200201, 201, []int{0}),
		ans(200202, 202, []int{2}),
		ans(200203, 203, []int{0}),
		ans(200204, 204, []int{0}),
		essay(200205, 205, "Parabola terjadi karena benda bergerak horizontal sekaligus jatuh karena gravitasi.", ScorePtr(20), "Jawaban terlalu singkat; sebutkan bahwa kedua gerak tidak saling memengaruhi."),
	)
	rBima = reg(QuizFisika, StudentBima, viewmodel.LocalDate(2026, time.October, 20, 7, 32))
	attempt(rBima, 2003, 1, viewmodel.LocalDate(2026, time.October, 20, 7, 33), viewmodel.LocalDatePtr(2026, time.October, 20, 8, 5), ScorePtr(83), false, 0)
	rEka = reg(QuizFisika, StudentEka, viewmodel.LocalDate(2026, time.October, 20, 7, 35))
	attempt(rEka, 2006, 1, viewmodel.LocalDate(2026, time.October, 20, 7, 36), viewmodel.LocalDatePtr(2026, time.October, 20, 8, 12), ScorePtr(67), false, 0)
	rGita = reg(QuizFisika, StudentGita, viewmodel.LocalDate(2026, time.October, 20, 7, 36))
	attempt(rGita, 2004, 1, viewmodel.LocalDate(2026, time.October, 20, 7, 37), viewmodel.LocalDatePtr(2026, time.October, 20, 8, 9), ScorePtr(77), false, 1)
	attempt(rGita, 2005, 2, viewmodel.LocalDate(2026, time.October, 20, 8, 15), viewmodel.LocalDatePtr(2026, time.October, 20, 8, 52), ScorePtr(91), false, 0)

	// --- Sejarah Indonesia (open, public): 6 participants, 4 in queue -----
	rAyu = reg(QuizSejarah, StudentAyu, viewmodel.LocalDate(2026, time.October, 22, 10, 3))
	attempt(rAyu, 4001, 1, viewmodel.LocalDate(2026, time.October, 22, 10, 4), viewmodel.LocalDatePtr(2026, time.October, 22, 10, 42), nil, true, 0,
		ans(400101, 401, []int{0}),
		ans(400102, 402, []int{1}),
		ans(400103, 403, []int{0, 1, 2}),
		essay(400104, 404, "Proklamasi menjadi tanda bahwa bangsa Indonesia mengambil nasibnya sendiri, lepas dari penjajahan, dan berdiri sebagai negara merdeka yang setara dengan bangsa lain.", ScorePtr(70), "Makna sudah tepat; tambahkan kaitan dengan pengakuan internasional."),
		essay(400105, 405, "Peristiwa Rengasdengklok terjadi karena golongan muda ingin Soekarno dan Hatta segera memproklamasikan kemerdekaan, sementara golongan tua masih menunggu kepastian dari Jepang.", nil, ""),
	)
	rBima = reg(QuizSejarah, StudentBima, viewmodel.LocalDate(2026, time.October, 22, 10, 5))
	attempt(rBima, 4002, 1, viewmodel.LocalDate(2026, time.October, 22, 10, 6), viewmodel.LocalDatePtr(2026, time.October, 22, 10, 55), ScorePtr(88), false, 0)
	rCitra = reg(QuizSejarah, StudentCitra, viewmodel.LocalDate(2026, time.October, 22, 10, 7))
	attempt(rCitra, 4003, 1, viewmodel.LocalDate(2026, time.October, 22, 10, 8), viewmodel.LocalDatePtr(2026, time.October, 22, 11, 1), nil, true, 1,
		ans(400301, 401, []int{0}),
		ans(400302, 402, []int{1}),
		ans(400303, 403, []int{0, 1, 2}),
		essay(400304, 404, "Proklamasi berarti Indonesia sudah merdeka dan berhak menentukan pemerintahan sendiri tanpa campur tangan negara lain.", nil, ""),
		essay(400305, 405, "Karena ada perbedaan pendapat antara golongan muda dan tua tentang waktu proklamasi.", ScorePtr(60), "Jawaban singkat; perlu menyebut peran Jepang yang mulai terdesak."),
	)
	rDewa = reg(QuizSejarah, StudentDewa, viewmodel.LocalDate(2026, time.October, 22, 10, 10))
	attempt(rDewa, 4004, 1, viewmodel.LocalDate(2026, time.October, 22, 10, 11), viewmodel.LocalDatePtr(2026, time.October, 22, 11, 12), ScorePtr(79), false, 0)
	rEka = reg(QuizSejarah, StudentEka, viewmodel.LocalDate(2026, time.October, 22, 10, 12))
	attempt(rEka, 4005, 1, viewmodel.LocalDate(2026, time.October, 22, 10, 13), viewmodel.LocalDatePtr(2026, time.October, 22, 11, 20), nil, true, 0,
		ans(400501, 401, []int{0}),
		ans(400502, 402, []int{1}),
		ans(400503, 403, []int{0, 1, 2}),
		essay(400504, 404, "Proklamasi adalah puncak perjuangan bangsa dan awal berdirinya negara Indonesia yang merdeka dan berdaulat.", ScorePtr(80), "Baik; hubungkan dengan pembentukan pemerintahan."),
		essay(400505, 405, "Golongan muda mendesak proklamasi segera dilakukan karena Jepang sudah menyerah kepada Sekutu, sehingga ada kekosongan kekuasaan yang harus segera diisi.", nil, ""),
	)
	rFajar = reg(QuizSejarah, StudentFajar, viewmodel.LocalDate(2026, time.October, 22, 10, 15))
	attempt(rFajar, 4006, 1, viewmodel.LocalDate(2026, time.October, 22, 10, 16), viewmodel.LocalDatePtr(2026, time.October, 22, 11, 30), nil, true, 1,
		ans(400601, 401, []int{0}),
		ans(400602, 402, []int{1}),
		ans(400603, 403, []int{0, 1, 2}),
		essay(400604, 404, "Proklamasi memberi bangsa Indonesia kebebasan untuk mengatur negara sendiri dan menjadi langkah awal pembangunan.", nil, ""),
		essay(400605, 405, "Karena Jepang menyerah tanpa syarat kepada Sekutu pada 14 Agustus 1945.", ScorePtr(50), "Sebutkan juga peran golongan muda dalam peristiwa ini."),
	)

	// --- Matematika Wajib (open, public): 5 registered, one in progress ----
	rAyu = reg(QuizMatematika, StudentAyu, viewmodel.LocalDate(2026, time.October, 23, 7, 5))
	attempt(rAyu, 5001, 1, viewmodel.LocalDate(2026, time.October, 23, 7, 20), nil, nil, false, 1,
		ans(500101, 501, []int{1}),
		ans(500102, 502, []int{1}),
		ans(500105, 505, []int{0, 1}),
		essay(500106, 506, "Pertama, nyatakan salah satu variabel dari persamaan pertama, lalu substitusikan ke persamaan kedua untuk memperoleh satu variabel saja.", nil, ""),
	)
	reg(QuizMatematika, StudentBima, viewmodel.LocalDate(2026, time.October, 23, 7, 8))
	reg(QuizMatematika, StudentCitra, viewmodel.LocalDate(2026, time.October, 23, 7, 10))
	reg(QuizMatematika, StudentDewa, viewmodel.LocalDate(2026, time.October, 23, 7, 12))
	reg(QuizMatematika, StudentEka, viewmodel.LocalDate(2026, time.October, 23, 7, 15))

	// --- Bahasa Indonesia (open, private XII IPS): 3 participants ---------
	rCitra = reg(QuizBahasa, StudentCitra, viewmodel.LocalDate(2026, time.October, 21, 9, 4))
	attempt(rCitra, 6001, 1, viewmodel.LocalDate(2026, time.October, 21, 9, 5), viewmodel.LocalDatePtr(2026, time.October, 21, 9, 45), ScorePtr(90), false, 0)
	rJoko := reg(QuizBahasa, StudentJoko, viewmodel.LocalDate(2026, time.October, 21, 9, 8))
	attempt(rJoko, 6002, 1, viewmodel.LocalDate(2026, time.October, 21, 9, 9), viewmodel.LocalDatePtr(2026, time.October, 21, 10, 2), ScorePtr(82), false, 1)
	reg(QuizBahasa, StudentHana, viewmodel.LocalDate(2026, time.October, 21, 9, 12))

	// --- Seni Budaya (closed, public): 5 attempts, score hidden -----------
	rAyu = reg(QuizSeni, StudentAyu, viewmodel.LocalDate(2026, time.October, 15, 8, 2))
	attempt(rAyu, 8001, 1, viewmodel.LocalDate(2026, time.October, 15, 8, 3), viewmodel.LocalDatePtr(2026, time.October, 15, 8, 39), ScorePtr(88), false, 0,
		ans(800101, 801, []int{0}),
		ans(800102, 802, []int{0}),
		ans(800103, 803, []int{0}),
		ans(800104, 804, []int{0}),
		essay(800105, 805, "Seni rupa dua dimensi hanya memiliki panjang dan lebar, sedangkan tiga dimensi memiliki volume dan bisa dilihat dari berbagai sisi.", ScorePtr(40), "Perbedaan dasar sudah disebutkan; berikan contoh karya untuk masing-masing."),
	)
	rBima = reg(QuizSeni, StudentBima, viewmodel.LocalDate(2026, time.October, 15, 8, 4))
	attempt(rBima, 8002, 1, viewmodel.LocalDate(2026, time.October, 15, 8, 5), viewmodel.LocalDatePtr(2026, time.October, 15, 8, 44), ScorePtr(73), false, 0)
	rCitra = reg(QuizSeni, StudentCitra, viewmodel.LocalDate(2026, time.October, 15, 8, 6))
	attempt(rCitra, 8003, 1, viewmodel.LocalDate(2026, time.October, 15, 8, 7), viewmodel.LocalDatePtr(2026, time.October, 15, 8, 47), ScorePtr(95), false, 0)
	rDewa = reg(QuizSeni, StudentDewa, viewmodel.LocalDate(2026, time.October, 15, 8, 8))
	attempt(rDewa, 8004, 1, viewmodel.LocalDate(2026, time.October, 15, 8, 9), viewmodel.LocalDatePtr(2026, time.October, 15, 8, 52), ScorePtr(60), false, 0)
	rEka = reg(QuizSeni, StudentEka, viewmodel.LocalDate(2026, time.October, 15, 8, 10))
	attempt(rEka, 8005, 1, viewmodel.LocalDate(2026, time.October, 15, 8, 11), viewmodel.LocalDatePtr(2026, time.October, 15, 8, 58), ScorePtr(81), false, 0)

	// --- Pendidikan Pancasila (open, public): 5 participants -------------
	reg(QuizPancasila, StudentAyu, viewmodel.LocalDate(2026, time.October, 22, 8, 3))
	rBima = reg(QuizPancasila, StudentBima, viewmodel.LocalDate(2026, time.October, 22, 8, 4))
	attempt(rBima, 9001, 1, viewmodel.LocalDate(2026, time.October, 22, 8, 5), viewmodel.LocalDatePtr(2026, time.October, 22, 8, 35), ScorePtr(80), false, 0)
	attempt(rBima, 9002, 2, viewmodel.LocalDate(2026, time.October, 22, 9, 0), viewmodel.LocalDatePtr(2026, time.October, 22, 9, 32), ScorePtr(90), false, 0)
	rFajar = reg(QuizPancasila, StudentFajar, viewmodel.LocalDate(2026, time.October, 22, 8, 6))
	attempt(rFajar, 9003, 1, viewmodel.LocalDate(2026, time.October, 22, 8, 7), viewmodel.LocalDatePtr(2026, time.October, 22, 8, 41), ScorePtr(70), false, 0)
	reg(QuizPancasila, StudentDewa, viewmodel.LocalDate(2026, time.October, 22, 8, 8))
	rHana = reg(QuizPancasila, StudentHana, viewmodel.LocalDate(2026, time.October, 22, 8, 9))
	attempt(rHana, 9004, 1, viewmodel.LocalDate(2026, time.October, 22, 8, 10), viewmodel.LocalDatePtr(2026, time.October, 22, 8, 48), ScorePtr(85), false, 0)

	// --- Geografi (open, public): 2 participants --------------------------
	rCitra = reg(QuizGeografi, StudentCitra, viewmodel.LocalDate(2026, time.October, 23, 7, 2))
	attempt(rCitra, 10001, 1, viewmodel.LocalDate(2026, time.October, 23, 7, 3), viewmodel.LocalDatePtr(2026, time.October, 23, 7, 31), ScorePtr(90), false, 0)
	reg(QuizGeografi, StudentJoko, viewmodel.LocalDate(2026, time.October, 23, 7, 5))

	return out
}

// defaultWorldStudent exists because buildWorld needs student lookups while it
// is still assembling the struct.
var buildStudents = []Student{
	{ID: StudentAyu, FullName: "Ayu Lestari", Email: "ayu@siswa.twisty.sch.id", GradeLevel: 12, MajorID: MajorIPA},
	{ID: StudentBima, FullName: "Bima Pratama", Email: "bima@siswa.twisty.sch.id", GradeLevel: 11, MajorID: MajorIPA},
	{ID: StudentCitra, FullName: "Citra Halim", Email: "citra@siswa.twisty.sch.id", GradeLevel: 12, MajorID: MajorIPS},
	{ID: StudentDewa, FullName: "Dewa Ananta", Email: "dewa@siswa.twisty.sch.id", GradeLevel: 10, MajorID: MajorIPA},
	{ID: StudentEka, FullName: "Eka Nuraini", Email: "eka@siswa.twisty.sch.id", GradeLevel: 12, MajorID: MajorIPA},
	{ID: StudentFajar, FullName: "Fajar Ramadhan", Email: "fajar@siswa.twisty.sch.id", GradeLevel: 11, MajorID: MajorIPS},
	{ID: StudentGita, FullName: "Gita Maharani", Email: "gita@siswa.twisty.sch.id", GradeLevel: 12, MajorID: MajorIPA},
	{ID: StudentHana, FullName: "Hana Salsabila", Email: "hana@siswa.twisty.sch.id", GradeLevel: 10, MajorID: MajorIPS},
	{ID: StudentIndra, FullName: "Indra Kusuma", Email: "indra@siswa.twisty.sch.id", GradeLevel: 11, MajorID: MajorIPA},
	{ID: StudentJoko, FullName: "Joko Santoso", Email: "joko@siswa.twisty.sch.id", GradeLevel: 12, MajorID: MajorIPS},
	{ID: StudentKirana, FullName: "Kirana Dewi", Email: "kirana@siswa.twisty.sch.id", GradeLevel: 10, MajorID: MajorBahasa},
	{ID: StudentLukman, FullName: "Lukman Hakim", Email: "lukman@siswa.twisty.sch.id", GradeLevel: 11, MajorID: MajorBahasa},
}

func defaultWorldStudent(id int64) *Student {
	for i := range buildStudents {
		if buildStudents[i].ID == id {
			return &buildStudents[i]
		}
	}
	return nil
}

func strPtr(s string) *string { return &s }
