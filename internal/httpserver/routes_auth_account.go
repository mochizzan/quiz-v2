package httpserver

// This file is the "auth-account" route group: everything about owning an
// account (registration, the pending-approval holding page, password
// changes). One routes_<group>.go file per group keeps parallel page tasks
// off each other's files and off server.go.

// init registers the account routes. All three render through
// layouts/auth.html (chosen by the pages/auth/ path).
func init() {
	registerPage(PageRoute{Method: "GET", Path: "/register", Template: "pages/auth/register.html"})
	registerPage(PageRoute{Method: "GET", Path: "/auth/pending", Template: "pages/auth/pending.html"})
	registerPage(PageRoute{Method: "GET", Path: "/change-password", Template: "pages/auth/change-password.html"})
}
