package httpserver

import (
	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/viewmodel"
)

// This file is the "auth-account" route group: the screens that belong to an
// existing account (password change, the teacher-approval holding page) plus
// sign-out. All three pages render through layouts/auth.html, chosen by the
// pages/auth/ path.

// init registers the four account routes.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/change-password",
		Template: "pages/auth/change-password.html",
		Nav:      "profil",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Ubah kata sandi", "profil", fixtures.StudentUser(), fixtures.ChangePassword())
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/change-password",
		Action: action("/profile", "password-changed"),
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/auth/pending",
		Template: "pages/auth/pending.html",
		Build: func(req PageRequest) viewmodel.PageData {
			state := viewmodel.PendingWaiting
			title := "Menunggu persetujuan"
			switch req.State() {
			case "approved":
				state = viewmodel.PendingApproved
				title = "Akun disetujui"
			case "expired":
				state = viewmodel.PendingExpired
				title = "Persetujuan kedaluwarsa"
			}
			return page(req, title, "", nil, fixtures.Pending(state))
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/logout",
		Action: action("/login", "logout"),
	})
}
