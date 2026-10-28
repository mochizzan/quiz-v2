package httpserver_test

// join_render_test.go pins the anonymous identify form's action. The POST
// handler resolves the quiz from ?quiz= (PAR-054); when the form dropped the
// query the submit answered 404 even though every hidden field and the join
// ticket were correct. Rendering the template directly keeps this check free of
// a database; the full ticket/cookie flow is covered by join_http_test.go.

import (
	"strings"
	"testing"

	"quiz-v2/internal/viewmodel"
)

func TestJoinPageExplainsNotOpen(t *testing.T) {
	notOpen := viewmodel.JoinNotOpen
	html := renderPage(t, "pages/auth/join.html", viewmodel.PageData{
		Base: viewmodel.Base{Title: "Gabung dengan kode kuis", CSRF: "csrf-token"},
		Data: viewmodel.JoinPage{Error: &notOpen},
	})

	if !strings.Contains(html, "Kuis ini belum dibuka atau sudah ditutup.") {
		t.Fatal("join page does not explain that a not-open quiz is unavailable")
	}
}

func TestIdentifyFormPostsToItsQuiz(t *testing.T) {
	const quizID int64 = 42
	html := renderPage(t, "pages/auth/identify.html", viewmodel.PageData{
		Base: viewmodel.Base{Title: "Identitas peserta", CSRF: "csrf-token"},
		Data: viewmodel.IdentifyPage{
			Quiz: viewmodel.QuizRef{ID: quizID, Title: "Kuis Publik"},
		},
	})

	want := `action="/join/identify?quiz=42"`
	if !strings.Contains(html, want) {
		t.Fatalf("identify form action = missing %q; a submit without the quiz query is a 404 (PAR-054)", want)
	}
	if strings.Contains(html, `action="/join/identify"`) {
		t.Fatal("identify form still posts to /join/identify without the quiz query")
	}
}
