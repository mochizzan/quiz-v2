package httpserver_test

// attempt_result_page_test.go pins spec §1.4 on the rendered document: the
// result page must never carry a link to nowhere, must never offer a way back
// into a closed quiz, and must render the question, the student's answer, the
// key, the verdict, and the essay feedback only when the review mode allows
// it. The domain already blanks those fields (result_visibility_test.go); this
// file proves the template does not re-introduce them from a Full record, which
// is the other half of the reported leak.

import (
	"strings"
	"testing"

	"quiz-v2/internal/viewmodel"
)

// resultRows is the same three-slot world the domain matrix uses, passed in
// fully populated on purpose: a template that gates correctly hides these
// fields even when the handler did not.
func resultRows() []viewmodel.ResultRow {
	return []viewmodel.ResultRow{
		{
			Position:      1,
			Type:          viewmodel.QuestionSingle,
			Body:          "Ibu kota Indonesia?",
			StudentAnswer: "Jakarta",
			Key:           "Bandung",
			Verdict:       "incorrect",
		},
		{
			Position:      2,
			Type:          viewmodel.QuestionEssay,
			Body:          "Jelaskan proses fotosintesis.",
			StudentAnswer: "Tumbuhan menyerap cahaya.",
			Feedback:      "Bagus sekali.",
			Verdict:       "graded",
			Score:         &viewmodel.Score{Percent: 86},
			Essay:         true,
			Graded:        true,
		},
	}
}

// resultData builds one result screen for a closed quiz: no retry, and a
// history link only for an authenticated student.
func resultData(mode viewmodel.ReviewMode, rows []viewmodel.ResultRow, historyHref string) viewmodel.PageData {
	return viewmodel.PageData{
		Base: viewmodel.Base{Title: "Hasil kuis"},
		Data: viewmodel.AttemptResult{
			Header: viewmodel.AttemptHeader{
				AttemptNo:   1,
				MaxAttempts: 1,
				QuizTitle:   "Kuis Geografi",
				StudentName: "Ayu Lestari",
				ClassLabel:  "Kelas 11 · RPL",
			},
			SubmittedAt:  nil,
			Score:        &viewmodel.Score{Percent: 50},
			ShowScore:    true,
			Graded:       true,
			Mode:         mode,
			ModeLabel:    mode.Label(),
			Rows:         rows,
			HistoryHref:  historyHref,
			AttemptsUsed: 1,
			MaxAttempts:  1,
		},
	}
}

// TestAttemptResultPageHasNoDeadLinks covers the reported defects: the empty
// `Tinjau jawaban` anchor, the duplicate `Riwayat kuis` button, the header
// `Kembali ke kuis` anchor, and the `/quiz/` fallback that 404s once the quiz
// is closed.
func TestAttemptResultPageHasNoDeadLinks(t *testing.T) {
	for _, mode := range viewmodel.AllReviewModes {
		t.Run(string(mode), func(t *testing.T) {
			html := renderPage(t, "pages/attempt/result.html",
				resultData(mode, resultRows(), "/student/history/7"))

			for _, banned := range []string{
				`href=""`,
				"Tinjau jawaban",
				"Riwayat kuis",
				"Kembali ke kuis",
				`href="/quiz/`,
			} {
				if strings.Contains(html, banned) {
					t.Errorf("result page still renders %q in %s", banned, mode)
				}
			}

			// The attempt shell has no .page-stack/.page-head chrome, so the
			// back affordance is asserted against the attempt header instead.
			head, tail, ok := strings.Cut(html, "</header>")
			if !ok {
				t.Fatal("result page has no closing </header>")
			}
			if !strings.Contains(head, `href="/student/history/7"`) {
				t.Error("history link missing from the result header")
			}
			if !strings.Contains(head, "Kembali ke riwayat") {
				t.Error("history link must be labelled with its destination")
			}
			if strings.Contains(head, "<button") {
				t.Error("the header trailing slot must be a link, never a button")
			}
			if strings.Contains(tail, "Kembali ke riwayat") {
				t.Error("the history link belongs in the header, not the body")
			}
		})
	}
}

// TestAttemptResultPageHistorySlot pins the trailing header slot: an
// authenticated student gets the history link, an anonymous participant gets
// nothing, and neither case leaves an empty href behind.
func TestAttemptResultPageHistorySlot(t *testing.T) {
	t.Run("authenticated student", func(t *testing.T) {
		html := renderPage(t, "pages/attempt/result.html",
			resultData(viewmodel.ReviewQuestionsWithKey, resultRows(), "/student/history/7"))

		head, _, ok := strings.Cut(html, "</header>")
		if !ok {
			t.Fatal("result page has no closing </header>")
		}
		if !strings.Contains(head, `href="/student/history/7"`) {
			t.Fatal("history link missing from the result header")
		}
		if !strings.Contains(head, "Kembali ke riwayat") {
			t.Fatal("history link must be labelled with its destination")
		}
		if !strings.Contains(head, "#i-arrow-left") {
			t.Fatal("history link must carry the arrow-left icon")
		}
	})

	t.Run("anonymous participant", func(t *testing.T) {
		html := renderPage(t, "pages/attempt/result.html",
			resultData(viewmodel.ReviewQuestionsWithKey, resultRows(), ""))

		if strings.Contains(html, "Kembali ke riwayat") {
			t.Error("anonymous result offered a history link")
		}
		if strings.Contains(html, `href=""`) {
			t.Error("anonymous result rendered an empty href")
		}
		// The footer is omitted entirely when nothing renders in it.
		if strings.Contains(html, `class="attempt-footer"`) {
			t.Error("anonymous one-shot result rendered an empty footer")
		}
	})
}

// TestAttemptResultPageModeVisibility is the template half of the visibility
// matrix, with every field populated in the input.
func TestAttemptResultPageModeVisibility(t *testing.T) {
	cases := []struct {
		mode            viewmodel.ReviewMode
		heading         string
		studentAnswer   bool
		key             bool
		verdict         bool
		essayFeedback   bool
		unansweredCount bool
	}{
		{mode: viewmodel.ReviewQuestionsNoVerdict, heading: "Telaah jawaban", studentAnswer: true, unansweredCount: true},
		{mode: viewmodel.ReviewQuestionsNoAnswers, heading: "Telaah jawaban"},
		{mode: viewmodel.ReviewCorrectOnly, heading: "Soal yang dijawab benar", studentAnswer: true},
		{
			mode: viewmodel.ReviewQuestionsWithKey, heading: "Telaah jawaban",
			studentAnswer: true, key: true, verdict: true, essayFeedback: true, unansweredCount: true,
		},
	}

	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			html := renderPage(t, "pages/attempt/result.html",
				resultData(tc.mode, resultRows(), "/student/history/7"))

			checks := []struct {
				name  string
				text  string
				shown bool
			}{
				{"student answer", "Jawaban Anda", tc.studentAnswer},
				{"answer key", "Kunci jawaban", tc.key},
				{"verdict badge", `theme-danger">Salah<`, tc.verdict},
				{"wrong-answer wash", "answer-box is-wrong", tc.verdict},
				{"essay feedback", "Umpan balik guru", tc.essayFeedback},
				{"essay score pill", `class="score-pill`, tc.essayFeedback},
				{"unanswered count", "Semua dijawab", tc.unansweredCount},
			}
			for _, c := range checks {
				got := strings.Contains(html, c.text)
				if got != c.shown {
					t.Errorf("%s rendered = %v, want %v (%q)", c.name, got, c.shown, c.text)
				}
			}

			if !strings.Contains(html, ">"+tc.heading+"<") {
				t.Errorf("answers heading missing %q", tc.heading)
			}
			if other := "Soal yang dijawab benar"; tc.heading != other && strings.Contains(html, ">"+other+"<") {
				t.Errorf("non-correct_only screen rendered the correct_only heading")
			}
			if !strings.Contains(html, "Tampilan hasil: "+tc.mode.Label()) {
				t.Errorf("mode label missing for %s", tc.mode)
			}
		})
	}
}

// TestAttemptResultPageCorrectOnlyEmptyState pins the calm zero-row block: it
// appears for correct_only alone and carries no action.
func TestAttemptResultPageCorrectOnlyEmptyState(t *testing.T) {
	const empty = "Tidak ada soal yang dijawab benar."

	t.Run("correct_only", func(t *testing.T) {
		html := renderPage(t, "pages/attempt/result.html",
			resultData(viewmodel.ReviewCorrectOnly, nil, "/student/history/7"))
		if !strings.Contains(html, empty) {
			t.Fatal("correct_only with no correct row is missing its empty message")
		}
		if !strings.Contains(html, "Soal yang dijawab benar") {
			t.Fatal("correct_only with no correct row lost its heading")
		}
	})

	t.Run("every other mode", func(t *testing.T) {
		for _, mode := range viewmodel.AllReviewModes {
			if mode.OnlyCorrect() {
				continue
			}
			html := renderPage(t, "pages/attempt/result.html",
				resultData(mode, nil, "/student/history/7"))
			if strings.Contains(html, empty) {
				t.Errorf("%s rendered the correct_only empty message", mode)
			}
		}
	})
}

// TestAttemptResultPageRetryFooter pins the one remaining footer action: a
// retry is a solid button, it is the only control, and it disappears with
// CanRetry (because a closed quiz cannot be retried).
func TestAttemptResultPageRetryFooter(t *testing.T) {
	data := resultData(viewmodel.ReviewQuestionsNoVerdict, resultRows(), "/student/history/7")
	result, ok := data.Data.(viewmodel.AttemptResult)
	if !ok {
		t.Fatalf("Data = %T, want viewmodel.AttemptResult", data.Data)
	}
	result.CanRetry = true
	result.RetryHref = "/quiz/7/attempts"
	data.Data = result

	html := renderPage(t, "pages/attempt/result.html", data)
	footer := html[strings.Index(html, `class="attempt-footer"`):]
	if !strings.Contains(footer, `class="btn btn-solid theme-primary" href="/quiz/7/attempts"`) {
		t.Fatalf("retry footer missing the solid Coba lagi button:\n%s", footer)
	}
	if !strings.Contains(footer, ">Coba lagi<") {
		t.Fatal("retry footer must be labelled Coba lagi")
	}
	if strings.Contains(footer, "Riwayat kuis") || strings.Contains(footer, "Tinjau jawaban") {
		t.Fatal("retry footer carries a removed action")
	}
	if n := strings.Count(footer, "<a "); n != 1 {
		t.Fatalf("retry footer has %d links, want only Coba lagi", n)
	}
}
