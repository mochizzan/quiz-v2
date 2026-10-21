package httpserver

// This file is the "teacher-core" route group: dashboard, quiz list and the
// live monitor. One routes_<group>.go file per group keeps parallel page
// tasks off each other's files and off server.go.

// init registers the three core teacher routes; the layout
// (layouts/teacher.html) and the active-nav key come from the PageRoute.
func init() {
	registerPage(PageRoute{Method: "GET", Path: "/teacher", Template: "pages/teacher/dashboard.html", Nav: "dasbor"})
	registerPage(PageRoute{Method: "GET", Path: "/teacher/quizzes", Template: "pages/teacher/quizzes.html", Nav: "kuis"})
	registerPage(PageRoute{Method: "GET", Path: "/teacher/monitor", Template: "pages/teacher/monitor.html", Nav: "kuis"})
}
