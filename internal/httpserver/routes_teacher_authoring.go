package httpserver

// This file is the "teacher-authoring" route group: quiz editing, the
// question editor, image upload and quiz settings. One routes_<group>.go file
// per group keeps parallel page tasks off each other's files and off server.go.

// init registers the four authoring routes; all render through
// layouts/teacher.html (the pages/teacher/ path convention).
func init() {
	registerPage(PageRoute{Method: "GET", Path: "/teacher/quizzes/edit", Template: "pages/teacher/quiz-edit.html", Nav: "kuis"})
	registerPage(PageRoute{Method: "GET", Path: "/teacher/questions", Template: "pages/teacher/question-editor.html", Nav: "bank-soal"})
	registerPage(PageRoute{Method: "GET", Path: "/teacher/questions/image", Template: "pages/teacher/image-upload.html", Nav: "bank-soal"})
	registerPage(PageRoute{Method: "GET", Path: "/teacher/quizzes/settings", Template: "pages/teacher/quiz-settings.html", Nav: "kuis"})
}
