package httpserver

// This file is the "teacher-grading" route group: the essay queue and the
// attempt recap. One routes_<group>.go file per group keeps parallel page
// tasks off each other's files and off server.go.

// init registers the two grading routes; both render through
// layouts/teacher.html (the pages/teacher/ path convention).
func init() {
	registerPage(PageRoute{Method: "GET", Path: "/teacher/grading", Template: "pages/teacher/grading.html", Nav: "penilaian"})
	registerPage(PageRoute{Method: "GET", Path: "/teacher/recap", Template: "pages/teacher/recap.html", Nav: "penilaian"})
}
