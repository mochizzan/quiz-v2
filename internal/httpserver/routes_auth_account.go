package httpserver

import (
	"errors"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/identity"
	"quiz-v2/internal/viewmodel"
)

// This file is the "auth-account" route group: the screens that belong to an
// existing account (password change, the teacher-approval holding page) plus
// sign-out. All three pages render through layouts/auth.html, chosen by the
// pages/auth/ path.

func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/change-password",
		Template: "pages/auth/change-password.html",
		Nav:      "profil",
		Auth:     ClassAnyAuth,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			// IDN-C10: the form is reachable during the must-change guard and
			// carries no fields; every input renders blank.
			return identityPage(x, "Ubah kata sandi", "profil", viewmodel.ChangePasswordPage{}), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/change-password",
		Auth:   ClassAnyAuth,
		ActForm: func(x *Ctx) (viewmodel.PageData, Redirect, error) {
			sess, serr := identitySession(x)
			if serr != nil {
				return viewmodel.PageData{}, Redirect{}, serr
			}
			svc := identityService(x)
			// The per-account bucket is keyed by the signed-in account and
			// cleared on success only (IDN-063, IDN-064). A 422 therefore
			// spends a slot too — the accepted over-count, never the reverse.
			bucket := identity.ChangePasswordKey(sess.AccountID)
			if err := identityRate(x, bucket, identity.ChangePasswordLimit); err != nil {
				return viewmodel.PageData{}, Redirect{}, err
			}
			err := svc.ChangePassword(x.c.Context(), sess.AccountID, sess.ID, identity.ChangePasswordInput{
				Current: x.RawValue("current_password"),
				New:     x.RawValue("new_password"),
				Confirm: x.RawValue("password_confirm"),
			})
			switch {
			case err == nil:
				x.Limiter().Clear(bucket)
				target := "/profile"
				if sess.IsTeacher() {
					// IDN-C11: a teacher has no profile page; the command's
					// shared copy rides both role homes.
					target = "/teacher"
				}
				return viewmodel.PageData{}, identityRedirect(target, identity.FlashPasswordReset), nil
			case errors.Is(err, identity.ErrSessionGone):
				// The session was revoked between resolution and the write
				// (IDN-112): the honest answer is the expired-session 303.
				return viewmodel.PageData{}, Redirect{}, x.SeeOther("/login", identity.FlashSessionExpired)
			}
			if fields, ok := identity.FieldErrorsOf(err); ok {
				return x.Page("Ubah kata sandi", "profil", viewmodel.ChangePasswordPage{}),
					Redirect{}, apperr.Validationf(fields)
			}
			return viewmodel.PageData{}, Redirect{}, identityFailure(err)
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/auth/pending",
		Template: "pages/auth/pending.html",
		Auth:     ClassPublic,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			// IDN-C12: the query token is preferred; otherwise the holder of
			// the 24-hour registration cookie is recognised. Both absent is an
			// expired page, never a 422, and the state resolution never
			// consumes or rotates anything.
			token := x.QueryValue("token")
			if token == "" {
				token = x.c.Cookies(identity.CookiePending)
			}
			info, err := identityService(x).Pending(x.c.Context(), token)
			if err != nil {
				return viewmodel.PageData{}, identityFailure(err)
			}
			title := "Menunggu persetujuan"
			switch info.State {
			case viewmodel.PendingApproved:
				title = "Akun disetujui"
			case viewmodel.PendingExpired:
				title = "Persetujuan kedaluwarsa"
			}
			model := viewmodel.PendingPage{State: info.State, FullName: info.FullName, LoginHref: "/login"}
			return identityPage(x, title, "", model), nil
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/logout",
		Auth:   ClassPublic,
		Act: func(x *Ctx) (Redirect, error) {
			// IDN-C13: logout is public and idempotent. A missing or already
			// revoked session is still a success; only CSRF can stop the POST.
			if x.Session != nil {
				if err := identityService(x).Logout(x.c.Context(), x.Session.ID); err != nil {
					return Redirect{}, identityFailure(err)
				}
			}
			x.ClearSessionCookie()
			if _, err := x.RotateCSRF(); err != nil {
				return Redirect{}, apperr.Internalf(err).WithRule("IDN-049")
			}
			x.ClearCookie(identity.CookiePending, identity.PendingCookiePath)
			return Redirect{Path: "/login", Flash: identity.FlashLoggedOut}, nil
		},
	})
}
