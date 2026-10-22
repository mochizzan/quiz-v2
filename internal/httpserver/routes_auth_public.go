package httpserver

import (
	"strconv"
	"strings"

	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/viewmodel"
)

// This file is the "auth-public" route group: the routes a visitor can reach
// without an account. Sign-in, both registration forms, password reset (the
// token is teacher-issued), join-by-code, the invitation landing and the
// anonymous identity form all render through layouts/auth.html, chosen by the
// pages/auth/ path.
//
// POST routes are UI-only: they validate nothing, persist nothing, and return
// the 303 target the real backend would land on. HTML5 attributes plus app.js
// carry the client-side feedback.

// init registers the fourteen public auth routes.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/login",
		Template: "pages/auth/login.html",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Masuk", "", nil, fixtures.Login())
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/login",
		Action: func(req PageRequest) string {
			// The server resolves the role from the account; this demo build
			// only has to land on the right home. "guru" and the seeded
			// teacher email both mean the teacher workspace.
			email := strings.ToLower(strings.TrimSpace(req.Form.Get("email")))
			if strings.Contains(email, "guru") || strings.Contains(email, "ratna") {
				return "/teacher?flash=welcome"
			}
			return "/student?flash=welcome"
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/register/teacher",
		Template: "pages/auth/register-teacher.html",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Daftar sebagai guru", "", nil, fixtures.RegisterTeacher())
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/register/teacher",
		Action: action("/auth/pending", "pending"),
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/register/student",
		Template: "pages/auth/register-student.html",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Daftar sebagai siswa", "", nil, fixtures.RegisterStudent())
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/register/student",
		Action: action("/student", "welcome"),
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/reset-password",
		Template: "pages/auth/reset-password.html",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Atur ulang kata sandi", "", nil, fixtures.ResetPassword(req.Query["token"]))
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/reset-password",
		Action: action("/login", "password-reset"),
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/join",
		Template: "pages/auth/join.html",
		Build: func(req PageRequest) viewmodel.PageData {
			var joinErr *viewmodel.JoinError
			if req.State() == "unknown" {
				e := viewmodel.JoinUnknownCode
				joinErr = &e
			}
			return page(req, "Gabung dengan kode kuis", "", nil, fixtures.Join("", joinErr))
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/join",
		Action: func(req PageRequest) string {
			if id, ok := fixtures.QuizForJoinCode(req.Form.Get("code")); ok {
				return "/quiz/" + strconv.FormatInt(id, 10)
			}
			return "/join?state=unknown"
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/invite/:token",
		Template: "pages/auth/invite.html",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Undangan kuis", "", nil, fixtures.Invite(req.Param("token")))
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/join/identify",
		Template: "pages/auth/identify.html",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Identitas peserta", "", nil, fixtures.Identify("", 0, 0, nil))
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/join/identify",
		Action: action("/quiz/1?state=anonymous", ""),
	})
}
