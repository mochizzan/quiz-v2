package participation

// refusal_test.go pins where a refused participation command sends the browser.
// Since PAR-088 a student has no /quiz/:id intro once the quiz closes, so every
// closed-quiz refusal must land on a history surface instead: the flash has to
// be rendered somewhere real, never on a 404. Open-quiz refusals keep the intro,
// which still exists. Anonymous viewers keep it too, on every state.

import (
	"testing"
	"time"

	"quiz-v2/internal/viewmodel"
)

// TestClosedQuizRefusalsLandOnHistory covers each closed-quiz refusal branch of
// Register, Cancel and StartAttempt.
func TestClosedQuizRefusalsLandOnHistory(t *testing.T) {
	f := newJoinFixture(t)

	t.Run("registering again after close", func(t *testing.T) {
		seedReadRegistration(t, f.db, f.closedQuiz, f.studentID, readTestAgo(3*time.Hour))

		res, err := f.svc.Register(f.ctx, RegisterInput{QuizID: f.closedQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("Register: %v", err)
		}
		if res.Path != historyPath(uint32(f.closedQuiz)) || res.Flash != FlashAlreadyRegistered {
			t.Fatalf("result = %+v, want already_registered on the quiz history", res)
		}
	})

	t.Run("cancelling a closed quiz", func(t *testing.T) {
		quizID := closedQuizSeed(f, "Kuis Tutup Batal", "410010")
		seedReadRegistration(t, f.db, quizID, f.studentID, readTestAgo(3*time.Hour))

		res, err := f.svc.Cancel(f.ctx, CancelInput{QuizID: quizID, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		if res.Path != historyPath(uint32(quizID)) || res.Flash != FlashNotOpen {
			t.Fatalf("result = %+v, want not_open on the quiz history", res)
		}
	})

	t.Run("starting with a completed attempt", func(t *testing.T) {
		quizID := closedQuizSeed(f, "Kuis Tutup Selesai", "410011")
		regID := seedReadRegistration(t, f.db, quizID, f.studentID, readTestAgo(3*time.Hour))
		seedReadAttempt(t, f.db, regID, 1,
			readTestAgo(2*time.Hour), readTestAgo(time.Hour), readTestAgo(time.Hour), 0.5)

		res, err := f.svc.StartAttempt(f.ctx, StartQuizInput{QuizID: quizID, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("StartAttempt: %v", err)
		}
		if res.Path != historyPath(uint32(quizID)) || res.Flash != FlashNotOpen {
			t.Fatalf("result = %+v, want not_open on the quiz history", res)
		}
	})

	t.Run("starting without a registration", func(t *testing.T) {
		quizID := closedQuizSeed(f, "Kuis Tutup Tanpa Daftar", "410012")

		res, err := f.svc.StartAttempt(f.ctx, StartQuizInput{QuizID: quizID, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("StartAttempt: %v", err)
		}
		// The per-quiz history page is a 404 without a registration
		// (PAR-155), so this refusal falls back to the history list.
		if res.Path != "/student/history" || res.Flash != FlashNotRegistered {
			t.Fatalf("result = %+v, want not_registered on the history list", res)
		}
	})
}

// TestOpenQuizRefusalsKeepTheIntro guards the other half of refusalPath: an open
// quiz still has its intro page, so its refusals must not be diverted.
func TestOpenQuizRefusalsKeepTheIntro(t *testing.T) {
	f := newJoinFixture(t)

	t.Run("registering twice", func(t *testing.T) {
		first, err := f.svc.Register(f.ctx, RegisterInput{QuizID: f.publicQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("first Register: %v", err)
		}
		if first.Path != quizPath(uint32(f.publicQuiz)) || first.Flash != FlashRegistered {
			t.Fatalf("first result = %+v, want registered on the intro", first)
		}

		second, err := f.svc.Register(f.ctx, RegisterInput{QuizID: f.publicQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("second Register: %v", err)
		}
		if second.Path != quizPath(uint32(f.publicQuiz)) || second.Flash != FlashAlreadyRegistered {
			t.Fatalf("second result = %+v, want already_registered on the intro", second)
		}
	})

	t.Run("starting without a registration", func(t *testing.T) {
		res, err := f.svc.StartAttempt(f.ctx, StartQuizInput{QuizID: f.switchQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("StartAttempt: %v", err)
		}
		if res.Path != quizPath(uint32(f.switchQuiz)) || res.Flash != FlashNotRegistered {
			t.Fatalf("result = %+v, want not_registered on the intro", res)
		}
	})
}

// TestAnonymousClosedQuizIntroHasNoStudentLinks pins the deliberate asymmetry in
// the spec: anonymous participants keep the closed-quiz intro because they have
// no history page, but the intro must not offer them a /student/history link.
func TestAnonymousClosedQuizIntroHasNoStudentLinks(t *testing.T) {
	f := newJoinFixture(t)
	token := f.identityToken(4)
	identityID := f.seedIdentity(token, "Tamu Uji", 11, f.majorID)
	regID := f.seedAnonRegistration(f.closedQuiz, identityID)
	actor := ActorInput{IdentityToken: token}

	t.Run("registered, nothing submitted", func(t *testing.T) {
		out, err := f.svc.QuizIntro(f.ctx, QuizIntroInput{QuizID: f.closedQuiz, Actor: actor})
		if err != nil {
			t.Fatalf("QuizIntro: %v", err)
		}
		if out.Quiz.State != viewmodel.QuizClosed {
			t.Fatalf("state = %v, want closed", out.Quiz.State)
		}
		if out.Primary != nil || out.Secondary != nil {
			t.Fatalf("actions = %+v / %+v, want neither for a closed quiz", out.Primary, out.Secondary)
		}
	})

	t.Run("one attempt submitted", func(t *testing.T) {
		attemptID := seedReadAttempt(t, f.db, regID, 1,
			readTestAgo(2*time.Hour), readTestAgo(time.Hour), readTestAgo(time.Hour), 0.5)

		out, err := f.svc.QuizIntro(f.ctx, QuizIntroInput{QuizID: f.closedQuiz, Actor: actor})
		if err != nil {
			t.Fatalf("QuizIntro: %v", err)
		}
		want := attemptResultPath(uint32(attemptID))
		if out.Primary == nil || out.Primary.Href != want {
			t.Fatalf("primary = %+v, want Lihat hasil at %s", out.Primary, want)
		}
		if out.Secondary != nil {
			t.Fatalf("secondary = %+v, want none: /student/history is student-only", out.Secondary)
		}
	})
}
