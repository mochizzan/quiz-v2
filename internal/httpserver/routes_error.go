package httpserver

// This file is the "error" route group. Every route group lives in its own
// routes_<group>.go file and registers itself from init(), so parallel page
// tasks never share a Go file and never touch server.go.

// init registers the error routes: GET /error/500 renders the styled server
// error page (pages/error/500.html through layouts/error.html). The 404 page
// needs no route here — server.go's catch-all renders
// pages/error/404.html directly for every unclaimed path.
func init() {
	registerPage(PageRoute{Method: "GET", Path: "/error/500", Template: "pages/error/500.html"})
}
