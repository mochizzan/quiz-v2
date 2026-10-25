package participation

// quizintro_test.go pins PAR-088's closed-quiz rule (spec §2): once a quiz
// closes, a student has no intro page even with a live registration, because
// the history detail screen is the only way back in. The same fixture proves
// the exception: an attempt that was already in progress still resumes
// (PAR-080, ATT-104 step 4) and the intro stays closed either way.

import (
	"testing"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// closedQuizSeed is a closed quiz of its own per subtest, so a write in one
// case cannot change what another case reads.
func closedQuizSeed(f *joinFixture, title, code string) int64 {
	f.t.Helper()
	id := seedReadQuiz(f.t, f.db, readQuizSeed{
		owner: f.teacher, title: title, visibility: "public", state: "closed",
		code: code, invite: "invite-" + code, published: readTestAgo(48 * time.Hour),
		closed: readTestAgo(2 * time.Hour), allowRetake: true, maxAttempts: 3, showScore: true,
	})
	seedReadQuestions(f.t, f.db, id, 1)
	return id
}

func TestQuizIntroClosedQuizIntegration(t *testing.T) {
	f := newJoinFixture(t)

	t.Run("closed quiz is 404 for a registered student", func(t *testing.T) {
		quizID := closedQuizSeed(f, "Kuis Tutup Terdaftar", "410001")
		seedReadRegistration(t, f.db, quizID, f.studentID, readTestAgo(3*time.Hour))

		_, err := f.svc.QuizIntro(f.ctx, QuizIntroInput{QuizID: quizID, Actor: f.studentActor()})
		if !apperr.IsNotFound(err) {
			t.Fatalf("error = %v, want 404", err)
		}
		if rule := apperr.From(err).Rule(); rule != "PAR-088" {
			t.Fatalf("rule = %q, want PAR-088", rule)
		}
	})

	t.Run("closed quiz is 404 for an unregistered student", func(t *testing.T) {
		quizID := closedQuizSeed(f, "Kuis Tutup Baru", "410002")

		_, err := f.svc.QuizIntro(f.ctx, QuizIntroInput{QuizID: quizID, Actor: f.studentActor()})
		if !apperr.IsNotFound(err) {
			t.Fatalf("error = %v, want 404", err)
		}
		if rule := apperr.From(err).Rule(); rule != "PAR-088" {
			t.Fatalf("rule = %q, want PAR-088", rule)
		}
		if n := f.count("SELECT COUNT(*) FROM registrations WHERE quiz_id = ?", quizID); n != 0 {
			t.Fatalf("intro created %d registrations", n)
		}
	})

	t.Run("draft quiz is still 404", func(t *testing.T) {
		_, err := f.svc.QuizIntro(f.ctx, QuizIntroInput{QuizID: f.draftQuiz, Actor: f.studentActor()})
		if !apperr.IsNotFound(err) {
			t.Fatalf("error = %v, want 404", err)
		}
		if rule := apperr.From(err).Rule(); rule != "PAR-088" {
			t.Fatalf("rule = %q, want PAR-088", rule)
		}
	})

	t.Run("open quiz is 200 for an unregistered student", func(t *testing.T) {
		out, err := f.svc.QuizIntro(f.ctx, QuizIntroInput{QuizID: f.publicQuiz, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("QuizIntro: %v", err)
		}
		if out.Quiz.ID != f.publicQuiz || out.Quiz.State != viewmodel.QuizOpen {
			t.Fatalf("quiz = %+v, want the open quiz", out.Quiz)
		}
		if !out.CanRegister || out.Primary == nil || out.Primary.Label != "Daftar" {
			t.Fatalf("actions = can_register %v primary %+v, want Daftar", out.CanRegister, out.Primary)
		}
		if out.BackHref != "/student" {
			t.Fatalf("BackHref = %q, want /student", out.BackHref)
		}
	})

	t.Run("open quiz is 200 for a registered student", func(t *testing.T) {
		quizID := seedReadQuiz(t, f.db, readQuizSeed{
			owner: f.teacher, title: "Kuis Buka Terdaftar", visibility: "public", state: "open",
			code: "410003", invite: "invite-410003", published: readTestAgo(24 * time.Hour),
			allowRetake: true, maxAttempts: 2, showScore: true,
		})
		seedReadQuestions(t, f.db, quizID, 1)
		seedReadRegistration(t, f.db, quizID, f.studentID, readTestAgo(2*time.Hour))

		out, err := f.svc.QuizIntro(f.ctx, QuizIntroInput{QuizID: quizID, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("QuizIntro: %v", err)
		}
		if out.Participation != viewmodel.ParticipationRegistered || out.AttemptsUsed != 0 {
			t.Fatalf("participation = %v used = %d, want registered with no attempts",
				out.Participation, out.AttemptsUsed)
		}
		if out.Primary == nil || out.Primary.Label != "Mulai" {
			t.Fatalf("primary = %+v, want Mulai", out.Primary)
		}
	})
}

func TestStartAttemptResumesOnClosedQuizIntegration(t *testing.T) {
	f := newJoinFixture(t)

	t.Run("in-progress attempt resumes without a flash", func(t *testing.T) {
		quizID := closedQuizSeed(f, "Kuis Tutup Lanjut", "410004")
		regID := seedReadRegistration(t, f.db, quizID, f.studentID, readTestAgo(3*time.Hour))
		attemptID := seedReadAttempt(t, f.db, regID, 1,
			readTestAgo(90*time.Minute), readTestAgo(10*time.Minute), nil, nil)

		res, err := f.svc.StartAttempt(f.ctx, StartQuizInput{QuizID: quizID, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("StartAttempt: %v", err)
		}
		if res.Path != attemptPath(uint32(attemptID)) || res.Flash != "" {
			t.Fatalf("result = %+v, want the in-progress attempt without a flash", res)
		}
		if n := f.count("SELECT COUNT(*) FROM quiz_attempts WHERE registration_id = ?", regID); n != 1 {
			t.Fatalf("resume created an attempt: %d rows", n)
		}

		// The intro stays closed: the resume is reachable from the history
		// detail page, never from /quiz/:id (PAR-088).
		if _, err := f.svc.QuizIntro(f.ctx, QuizIntroInput{QuizID: quizID, Actor: f.studentActor()}); !apperr.IsNotFound(err) {
			t.Fatalf("QuizIntro error = %v, want 404", err)
		}
	})

	t.Run("a submitted attempt on a closed quiz is not open", func(t *testing.T) {
		quizID := closedQuizSeed(f, "Kuis Tutup Selesai", "410005")
		regID := seedReadRegistration(t, f.db, quizID, f.studentID, readTestAgo(3*time.Hour))
		seedReadAttempt(t, f.db, regID, 1,
			readTestAgo(2*time.Hour), readTestAgo(time.Hour), readTestAgo(time.Hour), 0.5)

		res, err := f.svc.StartAttempt(f.ctx, StartQuizInput{QuizID: quizID, Actor: f.studentActor()})
		if err != nil {
			t.Fatalf("StartAttempt: %v", err)
		}
		if res.Path != historyPath(uint32(quizID)) || res.Flash != FlashNotOpen {
			t.Fatalf("result = %+v, want not_open landing on the quiz history", res)
		}
		if n := f.count("SELECT COUNT(*) FROM quiz_attempts WHERE registration_id = ? AND deleted_at = ?",
			regID, store.LiveAt); n != 1 {
			t.Fatalf("attempts = %d, want the single seeded row", n)
		}
	})
}
