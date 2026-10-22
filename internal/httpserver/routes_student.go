package httpserver

// This file is the "student" route group: the student dashboard, quiz history
// (list and detail), the pre-flight quiz intro, and the profile screen. One
// routes_<group>.go file per group keeps parallel page tasks off each other's
// files and off server.go.

import (
	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/viewmodel"
)

// studentQuizAction redirects back to the quiz intro with the demo-only
// preview state and the PRG flash key.
func studentQuizAction(state, flash string) func(PageRequest) string {
	return func(req PageRequest) string {
		return withParam("/quiz/:id", req) + "?state=" + state + "&flash=" + flash
	}
}

// init registers the five student GET screens and the four UI-only POST
// actions. POST routes carry no Build: they validate nothing and redirect 303
// so a browser refresh never re-submits.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/student",
		Template: "pages/student/dashboard.html",
		Nav:      "dasbor",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Dasbor siswa", "dasbor", fixtures.StudentUser(), fixtures.StudentDashboard())
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/student/history",
		Template: "pages/student/history.html",
		Nav:      "riwayat",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Riwayat kuis", "riwayat", fixtures.StudentUser(), fixtures.StudentHistory())
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/student/history/:quizID",
		Template: "pages/student/history-detail.html",
		Nav:      "riwayat",
		Build: func(req PageRequest) viewmodel.PageData {
			detail := fixtures.StudentHistoryDetail(req.IntParam("quizID"))
			title := "Riwayat kuis"
			if detail.Quiz.Title != "" {
				title = detail.Quiz.Title
			}
			return page(req, title, "riwayat", fixtures.StudentUser(), detail)
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/profile",
		Template: "pages/profile.html",
		Nav:      "profil",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Profil", "profil", fixtures.StudentUser(), fixtures.StudentProfile())
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/quiz/:id",
		Template: "pages/student/quiz.html",
		Build: func(req PageRequest) viewmodel.PageData {
			intro := fixtures.StudentQuizIntroState(req.IntParam("id"), req.State())
			return page(req, intro.Quiz.Title, "", fixtures.StudentUser(), intro)
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/profile",
		Action: func(PageRequest) string { return "/profile?flash=saved" },
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/quiz/:id/register",
		Action: studentQuizAction("registered", "registered"),
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/quiz/:id/cancel",
		Action: studentQuizAction("available", "cancelled"),
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/quiz/:id/attempts",
		Action: func(req PageRequest) string {
			// The demo's two live attempts: Matematika 5001 and, for every
			// other quiz, the open Biologi attempt.
			if req.IntParam("id") == fixtures.QuizMatematika {
				return "/attempt/5001?flash=started"
			}
			return "/attempt/1009?flash=started"
		},
	})
}
