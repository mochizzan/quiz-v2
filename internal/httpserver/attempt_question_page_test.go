package httpserver_test

// attempt_question_page_test.go pins the live question screen's navigation
// contract. Leaving an attempt is navigation, never an action (DESIGN §7): the
// exit link stays a destination-labelled .link-quiet inside the question
// navigator, and out of the action footer. That footer holds the prev/next
// submit buttons and goes sticky at the thumb on phones, where an accidental
// press beside "Sebelumnya" used to leave the attempt and drop an answer the
// no-JS fallback would otherwise have committed.

import (
	"strings"
	"testing"

	"quiz-v2/internal/attempts"
	"quiz-v2/internal/viewmodel"
)

func questionPage(t *testing.T) string {
	t.Helper()
	page := attempts.QuestionPage{
		AttemptQuestion: viewmodel.AttemptQuestion{
			Header: viewmodel.AttemptHeader{
				ID: 12, QuizID: 3, QuizTitle: "Kuis Fokus", StudentName: "Ayu Lestari",
				ClassLabel: "XII-1", AttemptNo: 1, MaxAttempts: 2,
				Avatar: viewmodel.Avatar{Initials: "AL"},
			},
			Progress: viewmodel.AttemptProgress{Answered: 1, Total: 3},
			Position: 2, Total: 3,
			Type: viewmodel.QuestionSingle,
			Body: "Soal kedua?",
			Nav: []viewmodel.QuestionNav{
				{Position: 1, Href: "/attempt/12/1", Answered: true},
				{Position: 2, Href: "/attempt/12/2", Answered: true, Current: true},
				{Position: 3, Href: "/attempt/12/3"},
			},
			QuizHref:   "/quiz/3",
			IsLast:     false,
			PrevHref:   "/attempt/12/1",
			NextHref:   "/attempt/12/3",
			BeaconHref: "/attempt/12/beacon",
		},
		QuestionID: 8,
		AnswerHref: "/attempt/12/answer",
	}
	return renderPage(t, "pages/attempt/question.html", viewmodel.PageData{
		Base: viewmodel.Base{Title: "Kuis Fokus", CSRF: "csrf-token"},
		Data: page,
	})
}

// TestQuestionPageKeepsExitInTheNavigator pins the relocation: the exit link is
// inside the navigator card, is still navigation (link-quiet + arrow-left +
// destination label), and is absent from the sticky action footer.
func TestQuestionPageKeepsExitInTheNavigator(t *testing.T) {
	html := questionPage(t)

	if got := strings.Count(html, "Kembali ke kuis"); got != 1 {
		t.Fatalf("Kembali ke kuis rendered %d times, want exactly once", got)
	}

	rail := between(t, html, `aria-label="Navigasi soal"`, "</nav>")
	exit := between(t, rail, `class="link-quiet attempt-exit"`, "</a>")
	for _, want := range []string{"Kembali ke kuis", "#i-arrow-left", `href="/quiz/3"`} {
		if !strings.Contains(exit, want) {
			t.Errorf("navigator exit link missing %q: %s", want, exit)
		}
	}
	if strings.Contains(rail, "<button") {
		t.Error("the navigator carries a button; leaving must stay a link")
	}

	footer := between(t, html, `class="attempt-footer`, "</form>")
	if strings.Contains(footer, "Kembali ke kuis") {
		t.Error("the sticky action footer must not carry the exit link (DESIGN §7)")
	}
	if strings.Contains(footer, `href="/quiz/`) {
		t.Error("the sticky action footer must not link out of the attempt")
	}

	// Prev/next commit the in-flight answer, so they stay submit buttons.
	for _, want := range []string{`name="nav" value="prev"`, `name="nav" value="next"`} {
		if !strings.Contains(footer, want) {
			t.Errorf("action footer missing the %q submit button", want)
		}
	}
	if strings.Contains(footer, "<a ") {
		t.Error("the action footer must hold only prev/next controls")
	}
}
