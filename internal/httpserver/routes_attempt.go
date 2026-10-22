package httpserver

import (
	"strconv"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/viewmodel"
)

// This file is the "attempt" route group: the live quiz-taking flow
// (question, review, submit, result). These pages render through
// layouts/base.html (the pages/attempt/ path convention) — no teacher or
// student chrome, the attempt surfaces carry their own top bar.

// attemptQueryPosition reads ?q= (1-based). Zero means "no explicit position"
// so the fixture can resume at the first unanswered question. Invalid input
// follows the same path; non-positive values clamp to question 1.
func attemptQueryPosition(r PageRequest) int {
	raw, ok := r.Query["q"]
	if !ok || raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	if n < 1 {
		return 1
	}
	return n
}

// attemptAnswerAction is the UI-only save-and-advance action. The form posts
// the current question id and a closed nav key; the redirect target is derived
// from ?q= on the action URL. "next" on the last question opens the review.
func attemptAnswerAction(r PageRequest) string {
	id := r.IntParam("id")
	position, _ := strconv.Atoi(r.Query["q"])
	if position < 1 {
		position, _ = strconv.Atoi(r.Form.Get("position"))
	}
	if position < 1 {
		position = 1
	}

	data := fixtures.AttemptQuestionData(id, position)
	total := data.Total
	if total < 1 {
		total = 1
	}
	if position > total {
		position = total
	}

	base := "/attempt/" + r.Param("id")
	nav := r.Form.Get("nav")
	if nav == "" {
		nav = r.Form.Get("action")
	}
	switch nav {
	case "prev":
		if position <= 2 {
			return base
		}
		return base + "?q=" + strconv.Itoa(position-1)
	case "save":
		return base + "?q=" + strconv.Itoa(position) + "&flash=saved"
	case "review":
		return base + "/review"
	default: // next
		if position >= total {
			return base + "/review"
		}
		return base + "?q=" + strconv.Itoa(position+1)
	}
}

// attemptBeacon is the 204 endpoint the future focus-loss/progress beacon will
// call. The UI build accepts the request and stores nothing.
func attemptBeacon(c fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/attempt/:id",
		Template: "pages/attempt/question.html",
		Build: func(req PageRequest) viewmodel.PageData {
			data := fixtures.AttemptQuestionData(req.IntParam("id"), attemptQueryPosition(req))
			title := "Mengerjakan kuis"
			if data.Header.QuizTitle != "" {
				title = "Mengerjakan kuis · " + data.Header.QuizTitle
			}
			return page(req, title, "", nil, data)
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/attempt/:id/answer",
		Action: attemptAnswerAction,
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/attempt/:id/review",
		Template: "pages/attempt/review.html",
		Build: func(req PageRequest) viewmodel.PageData {
			data := fixtures.AttemptReviewData(req.IntParam("id"))
			title := "Tinjau jawaban"
			if data.Header.QuizTitle != "" {
				title = "Tinjau jawaban · " + data.Header.QuizTitle
			}
			return page(req, title, "", nil, data)
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/attempt/:id/submit",
		Action: func(req PageRequest) string {
			return withParam("/attempt/:id/result?flash=submitted", req)
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/attempt/:id/result",
		Template: "pages/attempt/result.html",
		Build: func(req PageRequest) viewmodel.PageData {
			data := fixtures.AttemptResultData(req.IntParam("id"), req.State())
			title := "Hasil kuis"
			if data.Header.QuizTitle != "" {
				title = "Hasil kuis · " + data.Header.QuizTitle
			}
			return page(req, title, "", nil, data)
		},
	})

	registerPage(PageRoute{
		Method:  "POST",
		Path:    "/attempt/:id/beacon",
		Handler: attemptBeacon,
	})
}
