package httpserver

// This file is the "home" route group. Every route group lives in its own
// routes_<group>.go file and registers itself from init(), so parallel page
// tasks never share a Go file and never touch server.go.

// init registers the root route: the bare host is the front door, so it sends
// every visitor to the single sign-in screen. The role home is resolved after
// credentials, never from the URL.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/",
		Redirect: "/login",
	})
}
