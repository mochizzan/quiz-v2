package httpserver

import (
	"quiz-v2/internal/httpserver/fixtures"
)

// This file is the "teacher-grading" route group: the essay queue, the
// per-answer grading detail, the save action and the attempt recap. One
// routes_<group>.go file per group keeps parallel page tasks off each other's
// files and off server.go.

func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/grading",
		Template: "pages/teacher/grading.html",
		Nav:      "penilaian",
		Build: func(req PageRequest) PageData {
			return page(req, "Penilaian esai", "penilaian", fixtures.TeacherUser(), fixtures.GradingPage(0, req.Query))
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/grading/:answerID",
		Template: "pages/teacher/grading.html",
		Nav:      "penilaian",
		Build: func(req PageRequest) PageData {
			return page(req, "Penilaian esai", "penilaian", fixtures.TeacherUser(), fixtures.GradingPage(req.IntParam("answerID"), req.Query))
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/grading/:answerID",
		Action: action("/teacher/grading", "graded"),
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/recap/:attemptID",
		Template: "pages/teacher/recap.html",
		Nav:      "penilaian",
		Build: func(req PageRequest) PageData {
			recap := fixtures.RecapPage(req.IntParam("attemptID"))
			return page(req, "Rekap percobaan — "+recap.Attempt.StudentName, "penilaian", fixtures.TeacherUser(), recap)
		},
	})
}
