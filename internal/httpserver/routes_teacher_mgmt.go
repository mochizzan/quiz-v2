package httpserver

// This file is the "teacher-management" route group: students, anonymous
// participants, majors and teacher approval. One routes_<group>.go file per
// group keeps parallel page tasks off each other's files and off server.go.

// init registers the four management routes; all render through
// layouts/teacher.html (the pages/teacher/ path convention).
func init() {
	registerPage(PageRoute{Method: "GET", Path: "/teacher/students", Template: "pages/teacher/students.html", Nav: "siswa"})
	registerPage(PageRoute{Method: "GET", Path: "/teacher/anonymous", Template: "pages/teacher/anonymous.html", Nav: "siswa"})
	registerPage(PageRoute{Method: "GET", Path: "/teacher/majors", Template: "pages/teacher/majors.html", Nav: "pengaturan"})
	registerPage(PageRoute{Method: "GET", Path: "/teacher/teachers", Template: "pages/teacher/teachers.html", Nav: "pengaturan"})
}
