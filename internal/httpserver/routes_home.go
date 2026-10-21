package httpserver

// This file is the "home" route group. Every route group lives in its own
// routes_<group>.go file and registers itself from init(), so parallel page
// tasks never share a Go file and never touch server.go.

// init registers the home routes: the standalone hello page stays reachable at
// GET /hello, and GET / redirects (303 See Other) to /login — the login page
// itself is registered by the auth route group (Task 7).
func init() {
	registerPage(PageRoute{Method: "GET", Path: "/hello", Template: "hello.html"})
	registerPage(PageRoute{Method: "GET", Path: "/", Redirect: "/login"})
}
