package fixtures

import (
	"strings"

	"quiz-v2/internal/viewmodel"
)

// Auth fixtures. Every screen below is a projection of the one demo world in
// world.go: the account names, majors, quiz titles, audiences and schedules
// come from Default(), so the auth pages agree with the dashboard and result
// screens. Nothing here mutates the world.

// Demo account emails shown on the sign-in page only while APP_DEMO is on.
const (
	DemoTeacherEmail = "ratna@twisty.sch.id"
	DemoStudentEmail = "ayu@siswa.twisty.sch.id"
)

// DefaultMinPassword is the one password floor across the auth forms.
const DefaultMinPassword = 8

// The three demo invitation tokens. Each resolves to a different outcome:
// a public open quiz, a private quiz the demo student is not eligible for,
// and a draft quiz that is not open yet.
const (
	InviteTokenBiologi = "bio4-7f2k"
	InviteTokenBahasa  = "bhs6-9x2p"
	InviteTokenKimia   = "kim2-1p8r"
)

// Demo join codes accepted by POST /join.
const (
	JoinCodeBiologi    = "482915"
	JoinCodeMatematika = "731204"
	JoinCodePancasila  = "905318"
)

// Login returns the one sign-in model for both roles. Teacher and student
// demo emails are carried on the model; the template only renders them when
// the demo strip is enabled.
func Login() viewmodel.LoginPage {
	return viewmodel.LoginPage{
		DemoTeacherEmail: DemoTeacherEmail,
		DemoStudentEmail: DemoStudentEmail,
	}
}

// RegisterTeacher returns the teacher form. Teacher accounts need approval
// from another active teacher before sign-in.
func RegisterTeacher() viewmodel.RegisterPage {
	return viewmodel.RegisterPage{
		Role:        viewmodel.RoleTeacher,
		MinPassword: DefaultMinPassword,
	}
}

// RegisterStudent returns the student form. Students are active immediately,
// so the form also carries the grade and major selects.
func RegisterStudent() viewmodel.RegisterPage {
	return viewmodel.RegisterPage{
		Role:        viewmodel.RoleStudent,
		MinPassword: DefaultMinPassword,
		Majors:      Majors(),
	}
}

// Majors returns the world's majors as select options, in display order.
func Majors() []viewmodel.MajorRef {
	w := Default()
	out := make([]viewmodel.MajorRef, 0, len(w.Majors))
	for _, m := range w.Majors {
		out = append(out, viewmodel.MajorRef{ID: m.ID, Name: m.Name})
	}
	return out
}

// ResetPassword returns the teacher-issued token form. token is echoed from
// the query string so an invitation link can prefill it.
func ResetPassword(token string) viewmodel.ResetPasswordPage {
	return viewmodel.ResetPasswordPage{Token: token}
}

// ChangePassword returns the signed-in password form. It carries no fields:
// the model is intentionally empty in the UI-only build.
func ChangePassword() viewmodel.ChangePasswordPage {
	return viewmodel.ChangePasswordPage{}
}

// Pending returns the teacher-approval holding screen. The demo teacher
// waiting for approval is Sari Puspita. An empty state defaults to waiting.
func Pending(state viewmodel.PendingState) viewmodel.PendingPage {
	if state == "" {
		state = viewmodel.PendingWaiting
	}
	name := ""
	if t := Default().Teacher(TeacherSari); t != nil {
		name = t.FullName
	}
	return viewmodel.PendingPage{
		State:     state,
		FullName:  name,
		LoginHref: "/login",
	}
}

// Join returns the public code-entry model. An unknown-code outcome is
// supplied by the route, never by the page itself.
func Join(code string, joinErr *viewmodel.JoinError) viewmodel.JoinPage {
	return viewmodel.JoinPage{Code: code, Error: joinErr}
}

// Invite resolves one invitation token into the landing model. The demo
// student (Ayu, XII IPA) is the viewer:
//
//   - bio4-7f2k: public, open Biologi quiz — joinable.
//   - bhs6-9x2p: open but private XII IPS — wrong audience for Ayu.
//   - kim2-1p8r: draft private XI IPA — not open.
//
// Anything else is an unknown token. The outcome is a closed JoinError; the
// template renders the reason without recomputing eligibility.
func Invite(token string) viewmodel.InvitePage {
	w := Default()
	p := viewmodel.InvitePage{
		Token:         token,
		LoginHref:     "/login",
		DashboardHref: "/student",
		JoinHref:      "/join/identify",
	}
	q := w.QuizByInvite(token)
	if q == nil {
		p.Outcome = joinError(viewmodel.JoinUnknownCode)
		return p
	}
	ref := w.QuizRef(q.ID)
	p.Quiz = &ref
	p.Audience = w.Card(q).AudienceText()

	if q.State != viewmodel.QuizOpen {
		p.Outcome = joinError(viewmodel.JoinNotOpen)
		return p
	}
	student := w.Student(StudentAyu)
	if q.Visibility == viewmodel.VisibilityPrivate && !w.Eligible(q, student) {
		p.Outcome = joinError(viewmodel.JoinWrongAudience)
		return p
	}

	// The valid landing carries the schedule beside the audience. InvitePage
	// has no schedule field, so it is composed into Audience here; the
	// template labels the line "Audiens dan jadwal".
	p.Audience = w.Card(q).AudienceText() + " · " + w.Card(q).Schedule()
	return p
}

// InviteBiologi is the valid public invitation from the world's quiz 1.
func InviteBiologi() viewmodel.InvitePage { return Invite(InviteTokenBiologi) }

// InviteBahasa is the private XII IPS invitation Ayu cannot join.
func InviteBahasa() viewmodel.InvitePage { return Invite(InviteTokenBahasa) }

// InviteKimia is the draft invitation that is not open yet.
func InviteKimia() viewmodel.InvitePage { return Invite(InviteTokenKimia) }

// Identify returns the anonymous participant form for the world's public
// Biologi quiz (quiz 1). Submitting it registers participation by full name.
func Identify(fullName string, gradeLevel int, majorID int64, joinErr *viewmodel.JoinError) viewmodel.IdentifyPage {
	return viewmodel.IdentifyPage{
		Quiz:       Default().QuizRef(QuizBiologi),
		FullName:   fullName,
		GradeLevel: gradeLevel,
		MajorID:    majorID,
		Majors:     Majors(),
		Error:      joinErr,
	}
}

// QuizForJoinCode resolves only the three demo codes accepted by POST /join.
// Other world codes stay inert on purpose, matching the spec's code-entry
// contract.
func QuizForJoinCode(code string) (int64, bool) {
	switch strings.TrimSpace(code) {
	case JoinCodeBiologi:
		return QuizBiologi, true
	case JoinCodeMatematika:
		return QuizMatematika, true
	case JoinCodePancasila:
		return QuizPancasila, true
	default:
		return 0, false
	}
}

func joinError(e viewmodel.JoinError) *viewmodel.JoinError { return &e }
