package httpserver

// This file is the "attempt" route group: the live quiz-taking flow
// (question, review, result). One routes_<group>.go file per group keeps
// parallel page tasks off each other's files and off server.go.

// init registers the three attempt routes. These render through
// layouts/base.html (the pages/attempt/ path convention) — no teacher or
// student chrome, the attempt surfaces carry their own top bar.
func init() {
	registerPage(PageRoute{Method: "GET", Path: "/attempt", Template: "pages/attempt/question.html"})
	registerPage(PageRoute{Method: "GET", Path: "/attempt/review", Template: "pages/attempt/review.html"})
	registerPage(PageRoute{Method: "GET", Path: "/attempt/result", Template: "pages/attempt/result.html"})
}
