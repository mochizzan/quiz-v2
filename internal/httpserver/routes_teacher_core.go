package httpserver

import (
	"fmt"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/viewmodel"
)

// This file is the "teacher-core" route group: the dashboard, the quiz list,
// quiz detail, publish/close/delete actions, results, the live monitor and its
// SSE endpoint. Authoring routes live in routes_teacher_authoring.go.

// teacherFlashRedirect returns the PRG target for a parameterised teacher
// route, substituting :id / :qid and appending the one-shot flash key.
func teacherFlashRedirect(path, flash string) func(PageRequest) string {
	return func(req PageRequest) string {
		return withParam(path, req) + "?flash=" + flash
	}
}

// pageNumber reads ?page= and clamps it to the first page on bad input.
// quizStream emits one SSE tick. It deliberately does not loop: the UI build
// has no live database to subscribe to, so the response proves the
// text/event-stream wiring and closes.
func quizStream(c fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	tick := fmt.Sprintf("event: tick\ndata: {\"quiz_id\":%q,\"generated_at\":%q}\n\n",
		c.Params("id"), "2026-10-23T09:42:00+07:00")
	return c.SendString(tick)
}

// init registers the teacher overview, quiz list, builder actions, results,
// monitor and stream routes. GET routes carry a Build; POST routes carry only
// an Action because the UI build never validates or persists.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher",
		Template: "pages/teacher/dashboard.html",
		Nav:      "dasbor",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Dasbor guru", "dasbor", fixtures.TeacherUser(), fixtures.TeacherDashboardData())
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes",
		Template: "pages/teacher/quizzes.html",
		Nav:      "kuis",
		Build: func(req PageRequest) viewmodel.PageData {
			list := fixtures.TeacherQuizListData(
				req.Query["state"],
				req.Query["q"],
				req.Query["sort"],
				pageNumber(req),
			)
			return page(req, "Daftar kuis", "kuis", fixtures.TeacherUser(), list)
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/new",
		Template: "pages/teacher/quiz-new.html",
		Nav:      "kuis",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Buat kuis", "kuis", fixtures.TeacherUser(), fixtures.NewQuizFormData())
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes",
		Action: action("/teacher/quizzes/7", "quiz-created"),
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/:id",
		Template: "pages/teacher/quiz-detail.html",
		Nav:      "kuis",
		Build: func(req PageRequest) viewmodel.PageData {
			detail := fixtures.QuizDetailData(req.IntParam("id"))
			title := "Detail kuis"
			if detail.Quiz.Title != "" {
				title = detail.Quiz.Title
			}
			return page(req, title, "kuis", fixtures.TeacherUser(), detail)
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id",
		Action: teacherFlashRedirect("/teacher/quizzes/:id", "saved"),
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/publish",
		Action: teacherFlashRedirect("/teacher/quizzes/:id", "published"),
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/close",
		Action: teacherFlashRedirect("/teacher/quizzes/:id", "closed"),
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/delete",
		Action: action("/teacher/quizzes", "deleted"),
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/:id/results",
		Template: "pages/teacher/results.html",
		Nav:      "kuis",
		Build: func(req PageRequest) viewmodel.PageData {
			results := fixtures.QuizResultsData(
				req.IntParam("id"),
				req.Query["state"],
				req.Query["q"],
				pageNumber(req),
			)
			title := "Hasil kuis"
			if results.Quiz.Title != "" {
				title = "Hasil · " + results.Quiz.Title
			}
			return page(req, title, "kuis", fixtures.TeacherUser(), results)
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/:id/monitor",
		Template: "pages/teacher/monitor.html",
		Nav:      "kuis",
		Build: func(req PageRequest) viewmodel.PageData {
			monitor := fixtures.MonitorData(req.IntParam("id"))
			title := "Monitor kuis"
			if monitor.Quiz.Title != "" {
				title = "Monitor · " + monitor.Quiz.Title
			}
			return page(req, title, "kuis", fixtures.TeacherUser(), monitor)
		},
	})

	registerPage(PageRoute{
		Method:  "GET",
		Path:    "/teacher/quizzes/:id/stream",
		Handler: quizStream,
	})
}
