package httpserver

import (
	"quiz-v2/internal/identity"
	"quiz-v2/internal/viewmodel"
)

// This file is the "home" route group. Every route group lives in its own
// routes_<group>.go file and registers itself from init(), so parallel page
// tasks never share a Go file and never touch server.go.

// init registers the root route: the bare host is the front door, so it sends
// every visitor to the single sign-in screen. The role home is resolved after
// credentials, never from the URL.
//
// IDN-C01: the bare host never renders — it is always 303 to the session's
// role home (/login without a session, /change-password + must_change for a
// must-change session, /teacher or /student otherwise). The must-change flash
// is the reason this is a handler rather than a static redirect.
func init() {
	registerPage(PageRoute{
		Method: "GET",
		Path:   "/",
		Auth:   ClassPublic,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			if x.Session == nil {
				return viewmodel.PageData{}, x.SeeOther("/login", "")
			}
			flash := ""
			if x.Session.MustChangePassword {
				flash = identity.FlashMustChange
			}
			return viewmodel.PageData{}, x.SeeOther(homeFor(x.Session), flash)
		},
	})
}
