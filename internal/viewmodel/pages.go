package viewmodel

// This file is the typed screen inventory. Every page struct maps 1:1 to one
// template path; no template reads a value that is not declared here.

// ---- Auth -----------------------------------------------------------------

// LoginPage is the single sign-in screen. There is deliberately no role field:
// the backend resolves the account role from the credentials.
type LoginPage struct {
	Email            string
	DemoTeacherEmail string
	DemoStudentEmail string
}

// RegisterPage is shared by the teacher and student forms; Role selects the
// heading and the extra student fields.
type RegisterPage struct {
	Role        Role
	FullName    string
	Email       string
	GradeLevel  int
	MajorID     int64
	Majors      []MajorRef
	MinPassword int
}

// ResetPasswordPage consumes a teacher-issued token.
type ResetPasswordPage struct {
	Token string
}

// ChangePasswordPage is the signed-in password change form.
type ChangePasswordPage struct{}

// PendingPage is the teacher approval holding screen.
type PendingPage struct {
	State     PendingState
	FullName  string
	LoginHref string
}

// JoinPage is public code entry.
type JoinPage struct {
	Code  string
	Error *JoinError
}

// InvitePage is the /invite/:token landing screen.
type InvitePage struct {
	Token         string
	Outcome       *JoinError
	Quiz          *QuizRef
	Audience      string
	LoginHref     string
	DashboardHref string
	JoinHref      string
}

// IdentifyPage is the anonymous participant name screen.
type IdentifyPage struct {
	Quiz       QuizRef
	FullName   string
	GradeLevel int
	MajorID    int64
	Majors     []MajorRef
	Error      *JoinError
}

// ---- Student --------------------------------------------------------------

// StudentStats is the four-number strip on the student dashboard.
type StudentStats struct {
	Assigned   int
	Registered int
	InProgress int
	Completed  int
	BestScore  *Score
}

// RecentResult is one completed attempt on the student dashboard.
type RecentResult struct {
	Quiz        QuizRef
	AttemptID   int64
	Score       *Score
	ShowScore   bool
	Waiting     bool
	SubmittedAt *LocalTime
	Href        string
}

// StudentDashboard is the student home screen: ready-to-start, live,
// newly-available, and recent-result sections, all from the same world.
type StudentDashboard struct {
	GreetingName string
	DateLine     string
	Stats        StudentStats
	Ready        []QuizCard
	Live         []QuizCard
	Latest       []QuizCard
	Recent       []RecentResult
	Empty        EmptyState
}

// HistoryRow is one quiz with its nested attempts.
type HistoryRow struct {
	Quiz        QuizRef
	TeacherName string
	State       ParticipationState
	Attempts    []AttemptRow
	BestScore   *Score
	ShowScore   bool
	LastAttempt *LocalTime
	DetailHref  string
}

// HistoryPage groups one row per quiz.
type HistoryPage struct {
	Rows       List[HistoryRow]
	Pagination Pagination
	Empty      EmptyState
}

// HistoryDetail expands one quiz's attempts and the retake rule.
type HistoryDetail struct {
	Quiz        QuizRef
	TeacherName string
	Audience    string
	State       ParticipationState
	ShowScore   bool
	Attempts    []AttemptRow
	MaxAttempts int
	CanRetry    bool
	RetryHref   string
	BackHref    string
}

// ProfilePage is the student account screen.
type ProfilePage struct {
	FullName   string
	Email      string
	GradeLevel int
	MajorID    int64
	Majors     []MajorRef
}

// QuizIntro is /quiz/:id: the pre-flight screen for one quiz.
type QuizIntro struct {
	Quiz          QuizCard
	Description   string
	StartsAt      *LocalTime
	EndsAt        *LocalTime
	Participation ParticipationState
	Attempts      []AttemptRow
	AttemptsUsed  int
	MaxAttempts   int
	CanRegister   bool
	CanCancel     bool
	CanStart      bool
	CanRetry      bool
	Primary       *CardAction
	Secondary     *CardAction
	JoinedAt      *LocalTime
	Error         *JoinError
	BackHref      string
}

// ---- Attempt --------------------------------------------------------------

// AttemptQuestion is the one-question-at-a-time answer form.
type AttemptQuestion struct {
	Header           AttemptHeader
	Progress         AttemptProgress
	Position         int
	Total            int
	Type             QuestionType
	Body             string
	Image            *QuestionImage
	Options          []OptionView
	Essay            *EssayView
	Answer           AnswerDraft
	Nav              []QuestionNav
	Action           string
	PrevHref         string
	NextHref         string
	SubmitHref       string
	QuizHref         string
	IsLast           bool
	RandomizeAnswers bool
}

// AttemptReview is the pre-submit summary.
type AttemptReview struct {
	Header     AttemptHeader
	Progress   AttemptProgress
	Rows       []ReviewRow
	Unanswered []int
	SubmitHref string
	BackHref   string
}

// AttemptResult is the post-submit score and review screen.
type AttemptResult struct {
	Header       AttemptHeader
	SubmittedAt  *LocalTime
	Score        *Score
	ShowScore    bool
	Graded       bool
	Waiting      bool
	Mode         ReviewMode
	ModeLabel    string
	Rows         []ResultRow
	Unanswered   []int
	QuizHref     string
	ReviewHref   string
	HistoryHref  string
	FocusLoss    int
	AttemptsUsed int
	MaxAttempts  int
	CanRetry     bool
	RetryHref    string
}

// ---- Teacher overview -----------------------------------------------------

// RecentQuiz is one row in the teacher dashboard's recent-quizzes panel.
type RecentQuiz struct {
	ID            int64
	Title         string
	State         QuizState
	CreatedAt     LocalTime
	QuestionCount int
	Joined        int
	Href          string
}

// TeacherDashboard is /teacher.
type TeacherDashboard struct {
	GreetingName string
	DateLine     string
	TopScores    []QuizTopScore
	Recent       []RecentQuiz
	MostJoined   []QuizParticipation
	LeastJoined  []QuizParticipation
}

// TeacherQuizList is /teacher/quizzes.
type TeacherQuizList struct {
	Quizzes    List[QuizCard]
	Pagination Pagination
	Tabs       []FilterTab
	Query      QueryState
	Empty      EmptyState
	CreateHref string
}

// ---- Teacher authoring ----------------------------------------------------

// QuizForm is step 1 of create/edit.
type QuizForm struct {
	Title         string
	Description   string
	StartAt       LocalTime
	EndAt         LocalTime
	Visibility    Visibility
	TargetGrade   int
	TargetMajorID int64
	Majors        []MajorRef
	Action        string
	CancelHref    string
	IsEdit        bool
	Quiz          *QuizRef
}

// QuizSettings is the read-only settings summary on the detail page.
type QuizSettings struct {
	MaxAttempts        int
	ReviewMode         ReviewMode
	ShowScore          bool
	RandomizeQuestions bool
	RandomizeAnswers   bool
	AllowRetake        bool
}

// QuestionRow is one row in the quiz detail builder.
type QuestionRow struct {
	ID           int64
	Position     int
	Type         QuestionType
	Body         string
	OptionCount  int
	HasImage     bool
	CorrectLabel string
	EditHref     string
	DeleteHref   string
}

// QuizDetail is /teacher/quizzes/:id.
type QuizDetail struct {
	Quiz                 QuizRef
	Description          string
	TeacherName          string
	State                QuizState
	Visibility           Visibility
	Audience             string
	Code                 *string
	InviteURL            string
	StartsAt             *LocalTime
	EndsAt               *LocalTime
	CreatedAt            LocalTime
	QuestionCount        int
	Questions            []QuestionRow
	Settings             QuizSettings
	Editable             bool
	CanPublish           bool
	CanClose             bool
	PublishBlockedReason string
	PublishAction        *CardAction
	CloseAction          *CardAction
	DeleteAction         *CardAction
	ResultsHref          string
	MonitorHref          string
	EditHref             string
	AddQuestionHref      string
	GradingHref          string
	Empty                EmptyState
	RandomizeQuestions   bool
	RandomizeAnswers     bool
}

// OptionDraft is one editable option row.
type OptionDraft struct {
	ID      int64
	Letter  string
	Body    string
	Correct bool
}

// QuestionForm is /teacher/quizzes/:id/questions/new and …/:questionID/edit.
type QuestionForm struct {
	Quiz             QuizRef
	ID               int64
	Position         int
	Type             QuestionType
	Body             string
	Image            *QuestionImage
	Options          []OptionDraft
	EssayNote        string
	RandomizeAnswers bool
	Action           string
	CancelHref       string
	DeleteHref       string
	IsNew            bool
}

// ResultsCounts is the summary strip above the results table.
type ResultsCounts struct {
	Submitted  int
	InProgress int
	// Average is the quiz-wide mean over graded attempts, rendered with one
	// decimal and an Indonesian comma (ATT-174). Nil means "no graded attempt
	// yet" and the template renders an em dash.
	Average *Average
}

// ResultsRow is one attempt in the teacher results table.
type ResultsRow struct {
	AttemptID   int64
	StudentName string
	ClassLabel  string
	AttemptNo   int
	MaxAttempts int
	SubmittedAt *LocalTime
	Score       *Score
	Waiting     bool
	FocusLoss   int
	RecapHref   string
}

// QuizResults is /teacher/quizzes/:id/results.
type QuizResults struct {
	Quiz       QuizRef
	Counts     ResultsCounts
	Rows       List[ResultsRow]
	Pagination Pagination
	Tabs       []FilterTab
	Query      QueryState
	Empty      EmptyState
	BackHref   string
	ReviewMode ReviewMode
}

// ---- Monitor --------------------------------------------------------------

// MonitorProgress is the live participation rail.
type MonitorProgress struct {
	Registered int
	InProgress int
	Completed  int
	Answered   int
	Total      int
}

// PercentAnswered returns the answered share for the rail.
func (m MonitorProgress) PercentAnswered() int {
	if m.Total <= 0 {
		return 0
	}
	n := m.Answered * 100 / m.Total
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}

// ParticipantRow is one live participant.
type ParticipantRow struct {
	RegistrationID int64
	AttemptID      *int64
	StudentName    string
	Initials       string
	ClassLabel     string
	State          ParticipationState
	Answered       int
	Total          int
	Score          *Score
	FocusLoss      int
	StartedAt      *LocalTime
	UpdatedAt      *LocalTime
	Anonymous      bool
}

// MonitorPage is /teacher/quizzes/:id/monitor, open quizzes only.
type MonitorPage struct {
	Quiz        QuizRef
	Progress    MonitorProgress
	Rows        []ParticipantRow
	StreamHref  string
	QuizHref    string
	ResultsHref string
	GradingHref string
	GeneratedAt LocalTime
	Empty       EmptyState
}

// ---- Grading --------------------------------------------------------------

// GradingRow is one ungraded essay in the queue.
type GradingRow struct {
	AnswerID        int64
	StudentName     string
	ClassLabel      string
	QuizTitle       string
	QuestionExcerpt string
	WaitingSince    LocalTime
	WaitingLabel    string
	Href            string
}

// GradingCounts is the queue header strip.
type GradingCounts struct {
	Pending     int
	GradedToday int
}

// GradingQueue is the left pane of the grading screen.
type GradingQueue struct {
	Counts     GradingCounts
	Rows       List[GradingRow]
	Pagination Pagination
	Tabs       []FilterTab
	Query      QueryState
	Empty      EmptyState
}

// GradingContextRow is another answer in the same attempt, shown for context.
type GradingContextRow struct {
	Position      int
	Type          QuestionType
	Body          string
	StudentAnswer string
	Key           string
	Score         *Score
	Verdict       string
}

// EssayAnswer is the full essay under review.
type EssayAnswer struct {
	ID            int64
	QuestionID    int64
	StudentName   string
	ClassLabel    string
	QuizTitle     string
	QuestionBody  string
	QuestionImage *QuestionImage
	Text          string
	SubmittedAt   LocalTime
	WaitingLabel  string
	State         EssayState
	Score         *Score
	Feedback      string
}

// GradingDetail is the right pane when an essay is selected.
type GradingDetail struct {
	Answer    EssayAnswer
	Attempt   AttemptHeader
	Suggested *Score
	Score     *Score
	Feedback  string
	Context   []GradingContextRow
	Action    string
	BackHref  string
	NextHref  string
}

// GradingPage is one template serving both /teacher/grading and the detail.
type GradingPage struct {
	Queue    GradingQueue
	Selected *GradingDetail
}

// RecapAttempt is the attempt identity on the teacher recap.
type RecapAttempt struct {
	ID          int64
	StudentName string
	ClassLabel  string
	QuizTitle   string
	QuizID      int64
	AttemptNo   int
	MaxAttempts int
	StartedAt   *LocalTime
	SubmittedAt *LocalTime
	FocusLoss   int
	Score       *Score
	Waiting     bool
}

// RecapRow is one answer in the teacher recap.
type RecapRow struct {
	Position      int
	Type          QuestionType
	Body          string
	StudentAnswer string
	Key           string
	Feedback      string
	Score         *Score
	Verdict       string
	Essay         bool
	Graded        bool
}

// Recap is /teacher/recap/:id; it ignores the quiz review mode.
type Recap struct {
	Attempt  RecapAttempt
	Answers  []RecapRow
	Score    *Score
	Waiting  bool
	BackHref string
}

// ---- Management -----------------------------------------------------------

// ResetToken is the one-shot token panel appended to the students list.
type ResetToken struct {
	Token     string
	ExpiresAt LocalTime
}

// StudentRow is one row in the teacher's student table.
type StudentRow struct {
	ID         int64
	FullName   string
	Email      string
	ClassLabel string
	GradeLevel int
	MajorID    int64
	Attempts   int
	EditHref   string
	ResetHref  string
	DeleteHref string
}

// StudentList is /teacher/students.
type StudentList struct {
	Students   List[StudentRow]
	Pagination Pagination
	Tabs       []FilterTab
	Majors     []MajorRef
	Query      QueryState
	Empty      EmptyState
	ResetToken *ResetToken
}

// StudentForm is /teacher/students/new and /teacher/students/:id.
type StudentForm struct {
	ID         int64
	FullName   string
	Email      string
	GradeLevel int
	MajorID    int64
	Majors     []MajorRef
	Action     string
	CancelHref string
	IsNew      bool
}

// AnonymousRow is one anonymous participant identity.
type AnonymousRow struct {
	ID            int64
	FullName      string
	ClassLabel    string
	GradeLevel    int
	MajorName     string
	QuizzesJoined int
	LastActivity  *LocalTime
	EditHref      string
	DeleteHref    string
}

// AnonymousList is /teacher/anonymous.
type AnonymousList struct {
	Rows       List[AnonymousRow]
	Pagination Pagination
	Query      QueryState
	Empty      EmptyState
}

// AnonymousForm is /teacher/anonymous/:id.
type AnonymousForm struct {
	ID         int64
	FullName   string
	GradeLevel int
	MajorID    int64
	Majors     []MajorRef
	Action     string
	CancelHref string
	IsNew      bool
}

// MajorRow is one managed major.
type MajorRow struct {
	ID           int64
	Name         string
	StudentCount int
	QuizCount    int
	CanDelete    bool
	RenameAction string
	DeleteAction string
}

// MajorsPage is /teacher/majors.
type MajorsPage struct {
	Majors       []MajorRow
	Empty        EmptyState
	CreateAction string
}

// TeacherRow is one teacher in the approval table.
type TeacherRow struct {
	ID          int64
	FullName    string
	Email       string
	Initials    string
	Status      AccountStatus
	ApprovedBy  string
	ApprovedAt  *LocalTime
	ApproveHref string
	CanApprove  bool
}

// TeachersPage is /teacher/teachers.
type TeachersPage struct {
	Pending []TeacherRow
	Active  []TeacherRow
	Empty   EmptyState
}

// ---- Error ----------------------------------------------------------------

// ErrorPage is the branded 404/500 screen.
type ErrorPage struct {
	Code     int
	Title    string
	Message  string
	HomeHref string
}
