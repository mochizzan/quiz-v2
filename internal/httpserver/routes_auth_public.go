package httpserver

// This file is the "auth-public" route group. Every route group lives in its
// own routes_<group>.go file and registers itself from init(), so parallel
// page tasks never share a Go file and never touch server.go.

// init registers the public auth routes: sign-in, join-by-code (guest entry)
// and the anonymous participant identity form. All three render through
// layouts/auth.html (the split-screen shell chosen by the pages/auth/ path).
func init() {
	registerPage(PageRoute{Method: "GET", Path: "/login", Template: "pages/auth/login.html"})
	registerPage(PageRoute{Method: "GET", Path: "/join", Template: "pages/auth/join.html"})
	registerPage(PageRoute{Method: "GET", Path: "/identify", Template: "pages/auth/identify.html"})
}
