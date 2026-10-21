package httpserver

// This file is the "student" route group: the student dashboard, quiz history
// and profile. One routes_<group>.go file per group keeps parallel page tasks
// off each other's files and off server.go.

// init registers the three student routes. Dashboard and history render
// through layouts/student.html (the pages/student/ path convention);
// /profile is a root-level route but reuses the same student layout via its
// own page set (registered here, in the student group, so there is a single
// entry point for the student area).
func init() {
	registerPage(PageRoute{Method: "GET", Path: "/student", Template: "pages/student/dashboard.html", Nav: "dasbor"})
	registerPage(PageRoute{Method: "GET", Path: "/student/history", Template: "pages/student/history.html", Nav: "riwayat"})
	registerPage(PageRoute{Method: "GET", Path: "/profile", Template: "pages/profile.html", Nav: "profil"})
}
