package httpserver

import (
	"errors"
	"net"
	"net/url"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/authn"
	"quiz-v2/internal/identity"
	"quiz-v2/internal/viewmodel"
)

// This file is the "auth-public" route group: the routes a visitor can reach
// without an account. Sign-in, both registration forms and password reset (the
// token is teacher-issued) render through layouts/auth.html, chosen by the
// pages/auth/ path.
//
// Join-by-code, the invitation landing and the anonymous identity form belong
// to the participation domain and live in routes_join.go.
//
// Each route carries its production Load/Act handler. Auth restates the class
// in authz.go so a route file is auditable on its own. Post handlers that reject
// input use ActForm so the same template re-renders at 422 with the typed
// values (IDN-083).
//
// The helpers below are shared with the other identity route groups in this
// package (auth-account, profile, teacher-management): the domain adapter,
// the rate-limit call, the chrome assembly and the flash-key transport.

// identityService binds the identity domain to this request's pool, signer and
// session store. It is built per request and holds no request state. A nil
// session store is tolerated (authn.Store assertion fails only in test builds);
// the commands then answer the same 500 a nil pool answers.
func identityService(x *Ctx) *identity.Service {
	var sessions *authn.Store
	if s, ok := x.server.sessions.(*authn.Store); ok {
		sessions = s
	}
	return identity.New(x.DB, x.Signer(), sessions)
}

// identitySession returns the resolved identity on a class-protected route.
// The class guard has already admitted an authenticated session; a nil session
// is a programming fault answered with the same 404 the guard produces.
func identitySession(x *Ctx) (*authn.Session, error) {
	if x.Session == nil {
		return nil, apperr.NotFoundf().WithRule("IDN-012")
	}
	return x.Session, nil
}

// identityPage assembles the chrome and resolves this domain's closed flash
// keys. deleted_student and already_approved are carried in the query because
// viewmodel.FlashFor does not know them; an unknown key is never reflected.
func identityPage(x *Ctx, title, nav string, data any) viewmodel.PageData {
	page := x.Page(title, nav, data)
	if page.Base.Flash == nil {
		if key := x.QueryValue("flash"); key != "" {
			page.Base.Flash = identity.FlashForKey(key)
		}
	}
	return page
}

// identityRedirect chooses the transport for a closed flash key: a key the
// shared view model knows becomes a signed one-shot cookie, and a domain-only
// key rides the query for identityPage to resolve.
func identityRedirect(path, flash string) Redirect {
	if flash == "" {
		return Redirect{Path: path}
	}
	if viewmodel.FlashFor(flash) != nil {
		return Redirect{Path: path, Flash: flash}
	}
	return Redirect{Path: path + "?flash=" + flash}
}

// identityRate enforces one fixed-window bucket and maps over-limit to 429
// (IDN-063, IDN-065). Call it before any lookup, bcrypt or write.
func identityRate(x *Ctx, key string, limit int) error {
	ok, retry := x.Limiter().Allow(key, limit, identity.RateWindow)
	if ok {
		return nil
	}
	return apperr.RateLimitedf(retry).WithRule("IDN-063")
}

// identityRoleHome is the signed-in landing for one role (IDN-026).
func identityRoleHome(role viewmodel.Role) string {
	if role == viewmodel.RoleTeacher {
		return "/teacher"
	}
	return "/student"
}

// identityAgent is the client's self-reported user agent. The session store
// truncates it to the column width.
func identityAgent(x *Ctx) string {
	return string(x.c.Request().Header.UserAgent())
}

// identityFailure maps every non-form domain outcome the identity routes can
// receive to the one *apperr.Error finish() renders (IDN-012, IDN-013, §4.6).
func identityFailure(err error) *apperr.Error {
	if fields, ok := identity.FieldErrorsOf(err); ok {
		return apperr.Validationf(fields)
	}
	switch {
	case errors.Is(err, identity.ErrNotFound):
		return apperr.NotFoundf().WithRule("IDN-012")
	case errors.Is(err, identity.ErrPendingTeacher):
		return apperr.Forbiddenf(apperr.MsgPendingTeacher).WithRule("IDN-013")
	}
	return apperr.From(err)
}

// init registers the eight public identity routes.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/login",
		Template: "pages/auth/login.html",
		Auth:     ClassGuestOnly,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			// IDN-C02: an empty form; the class guard has already sent any
			// authenticated visitor home.
			return x.Page("Masuk", "", viewmodel.LoginPage{}), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/login",
		Auth:   ClassGuestOnly,
		ActForm: func(x *Ctx) (viewmodel.PageData, Redirect, error) {
			// Rate limits run before validation, bcrypt and any write, and
			// answer 429 with no form (IDN-063, IDN-065).
			if err := identityRate(x, identity.LoginIPKey(x.IP), identity.LoginIPLimit); err != nil {
				return viewmodel.PageData{}, Redirect{}, err
			}
			emailKey := identity.LoginEmailKey(identity.LoginEmailBucket(x.Value("email")), x.IP)
			if err := identityRate(x, emailKey, identity.LoginEmailLimit); err != nil {
				return viewmodel.PageData{}, Redirect{}, err
			}

			if fields, ok := identity.FieldErrorsOf(identity.ValidateLogin(x.Value("email"), x.RawValue("password"))); ok {
				return x.Page("Masuk", "", viewmodel.LoginPage{Email: x.Value("email")}),
					Redirect{}, apperr.Validationf(fields)
			}

			result, err := identityService(x).Login(x.c.Context(), x.Value("email"),
				x.RawValue("password"), net.ParseIP(x.IP), identityAgent(x))
			switch {
			case err == nil:
				// A success clears the (email, IP) failure bucket only; the IP
				// bucket always expires by window (IDN-064).
				x.Limiter().Clear(emailKey)
				x.SetSessionCookie(result.Token)
				if _, cerr := x.RotateCSRF(); cerr != nil {
					return viewmodel.PageData{}, Redirect{}, apperr.Internalf(cerr).WithRule("IDN-049")
				}
				x.ClearCookie(identity.CookiePending, identity.PendingCookiePath)
				target, flash := identityRoleHome(result.Role), identity.FlashWelcome
				if result.MustChangePassword {
					target, flash = "/change-password", identity.FlashMustChange
				}
				return viewmodel.PageData{}, identityRedirect(target, flash), nil
			case errors.Is(err, identity.ErrPendingTeacher):
				// A correct password on a pending teacher is 403, never a form
				// answer; the error page's home link is shared chrome (§4.3).
				return viewmodel.PageData{}, Redirect{},
					apperr.Forbiddenf(apperr.MsgPendingTeacher).WithRule("IDN-013")
			}
			if fields, ok := identity.FieldErrorsOf(err); ok {
				return x.Page("Masuk", "", viewmodel.LoginPage{Email: x.Value("email")}),
					Redirect{}, apperr.Validationf(fields)
			}
			// The single credential failure is a form-level alert, not a field
			// error, so it cannot point at which half was wrong (IDN-083).
			// "error" is not one of the closed alert themes, so the same copy
			// rides the danger theme.
			failure := x.Page("Masuk", "", viewmodel.LoginPage{Email: x.Value("email")})
			failure.Base.Flash = &viewmodel.Flash{Kind: "danger", Title: apperr.MsgInvalidCredentials}
			return failure, Redirect{}, apperr.Validationf(nil).WithRule("IDN-083")
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/register/teacher",
		Template: "pages/auth/register-teacher.html",
		Auth:     ClassGuestOnly,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			// IDN-C04: the teacher form never loads majors; the account does
			// not join a school class.
			model := viewmodel.RegisterPage{Role: viewmodel.RoleTeacher, MinPassword: identity.MinPasswordLength}
			return x.Page("Daftar sebagai guru", "", model), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/register/teacher",
		Auth:   ClassGuestOnly,
		ActForm: func(x *Ctx) (viewmodel.PageData, Redirect, error) {
			if err := identityRate(x, identity.RegisterTeacherKey(x.IP), identity.RegisterLimit); err != nil {
				return viewmodel.PageData{}, Redirect{}, err
			}
			token, err := identityService(x).RegisterTeacher(x.c.Context(), x.Value("full_name"),
				x.Value("email"), x.RawValue("password"), x.RawValue("password_confirm"))
			if err != nil {
				if fields, ok := identity.FieldErrorsOf(err); ok {
					model := viewmodel.RegisterPage{
						Role:        viewmodel.RoleTeacher,
						FullName:    x.Value("full_name"),
						Email:       x.Value("email"),
						MinPassword: identity.MinPasswordLength,
					}
					return x.Page("Daftar sebagai guru", "", model), Redirect{}, apperr.Validationf(fields)
				}
				return viewmodel.PageData{}, Redirect{}, identityFailure(err)
			}
			// The pending token rides both channels on purpose: the 24-hour
			// cookie keeps the holder on the approval page (IDN-067), the query
			// string is the link they can revisit after the browser restarts.
			x.SetCookie(identity.CookiePending, token, identity.PendingCookieMaxAge, identity.PendingCookiePath)
			return viewmodel.PageData{}, Redirect{Path: "/auth/pending?token=" + url.QueryEscape(token)}, nil
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/register/student",
		Template: "pages/auth/register-student.html",
		Auth:     ClassGuestOnly,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			majors, err := identityService(x).Majors(x.c.Context())
			if err != nil {
				return viewmodel.PageData{}, identityFailure(err)
			}
			model := viewmodel.RegisterPage{
				Role:        viewmodel.RoleStudent,
				MinPassword: identity.MinPasswordLength,
				Majors:      majors,
			}
			data := x.Page("Daftar sebagai siswa", "", model)
			if len(majors) == 0 {
				// IDN-C06, IDN-122: no live major yet is an expected state, so
				// the page explains it instead of failing.
				data.Base.Flash = &viewmodel.Flash{Kind: "info", Title: identity.MsgNoMajors}
			}
			return data, nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/register/student",
		Auth:   ClassGuestOnly,
		ActForm: func(x *Ctx) (viewmodel.PageData, Redirect, error) {
			if err := identityRate(x, identity.RegisterStudentKey(x.IP), identity.RegisterLimit); err != nil {
				return viewmodel.PageData{}, Redirect{}, err
			}
			svc := identityService(x)
			result, err := svc.RegisterStudent(x.c.Context(), x.Value("full_name"), x.Value("email"),
				x.RawValue("password"), x.RawValue("password_confirm"),
				x.Value("grade_level"), x.Value("major_id"), net.ParseIP(x.IP), identityAgent(x))
			if err != nil {
				if fields, ok := identity.FieldErrorsOf(err); ok {
					return studentRegister422(x, svc, fields)
				}
				return viewmodel.PageData{}, Redirect{}, identityFailure(err)
			}
			// IDN-C07: the registration is itself a login event; the account
			// is signed in exactly like IDN-C03.
			x.SetSessionCookie(result.Token)
			if _, cerr := x.RotateCSRF(); cerr != nil {
				return viewmodel.PageData{}, Redirect{}, apperr.Internalf(cerr).WithRule("IDN-049")
			}
			return viewmodel.PageData{}, identityRedirect("/student", identity.FlashWelcome), nil
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/reset-password",
		Template: "pages/auth/reset-password.html",
		Auth:     ClassGuestOnly,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			// IDN-C08: the query token is prefill only. It is never looked up
			// or consumed on GET, so a link scanner cannot probe it.
			model := viewmodel.ResetPasswordPage{Token: x.QueryValue("token")}
			return x.Page("Atur ulang kata sandi", "", model), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/reset-password",
		Auth:   ClassGuestOnly,
		ActForm: func(x *Ctx) (viewmodel.PageData, Redirect, error) {
			if err := identityRate(x, identity.ResetConsumeKey(x.IP), identity.ResetConsumeLimit); err != nil {
				return viewmodel.PageData{}, Redirect{}, err
			}
			err := identityService(x).ResetPassword(x.c.Context(), x.Value("token"),
				x.RawValue("password"), x.RawValue("password_confirm"))
			if err != nil {
				if fields, ok := identity.FieldErrorsOf(err); ok {
					model := viewmodel.ResetPasswordPage{Token: x.Value("token")}
					return x.Page("Atur ulang kata sandi", "", model), Redirect{}, apperr.Validationf(fields)
				}
				return viewmodel.PageData{}, Redirect{}, identityFailure(err)
			}
			return viewmodel.PageData{}, identityRedirect("/login", identity.FlashPasswordReset), nil
		},
	})
}

// studentRegister422 re-renders the student form: the submitted values echoed,
// passwords blank, and the live majors re-read because a major may have been
// deleted since the page was loaded (IDN-083).
func studentRegister422(x *Ctx, svc *identity.Service, fields map[string]string) (viewmodel.PageData, Redirect, error) {
	majors, merr := svc.Majors(x.c.Context())
	if merr != nil {
		return viewmodel.PageData{}, Redirect{}, identityFailure(merr)
	}
	majorID, _ := identity.ParseID(x.Value("major_id"))
	grade, _ := identity.ParseGradeLevel(x.Value("grade_level"))
	model := viewmodel.RegisterPage{
		Role:        viewmodel.RoleStudent,
		FullName:    x.Value("full_name"),
		Email:       x.Value("email"),
		GradeLevel:  grade,
		MajorID:     majorID,
		Majors:      majors,
		MinPassword: identity.MinPasswordLength,
	}
	return x.Page("Daftar sebagai siswa", "", model), Redirect{}, apperr.Validationf(fields)
}
