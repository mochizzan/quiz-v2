package httpserver

import (
	"errors"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/identity"
	"quiz-v2/internal/viewmodel"
)

// This file is the "profile" route group, owned by the identity domain: the
// signed-in student's own profile, read and edit. It is a student-class route,
// so a teacher gets 404 rather than a redirect (IDN-012).

func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/profile",
		Template: "pages/profile.html",
		Nav:      "profil",
		Auth:     ClassStudent,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			sess, err := identitySession(x)
			if err != nil {
				return viewmodel.PageData{}, err
			}
			// IDN-C14: grade and major are read-only here and always come from
			// the live profile, never from the session's cached values.
			profile, err := identityService(x).Profile(x.c.Context(), sess.AccountID)
			if err != nil {
				return viewmodel.PageData{}, identityFailure(err)
			}
			return identityPage(x, "Profil", "profil", profile), nil
		},
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Profil", "profil", fixtures.StudentUser(), fixtures.StudentProfile())
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/profile",
		Auth:   ClassStudent,
		ActForm: func(x *Ctx) (viewmodel.PageData, Redirect, error) {
			sess, serr := identitySession(x)
			if serr != nil {
				return viewmodel.PageData{}, Redirect{}, serr
			}
			svc := identityService(x)
			err := svc.SaveProfile(x.c.Context(), sess.AccountID, identity.ProfileInput{
				FullName: x.Value("full_name"),
				Email:    x.Value("email"),
			})
			switch {
			case err == nil:
				return viewmodel.PageData{}, Redirect{Path: "/profile", Flash: identity.FlashSaved}, nil
			case errors.Is(err, identity.ErrSessionGone):
				// The account was deleted between resolution and the write
				// (IDN-112, IDN-113); the write never commits against it.
				return viewmodel.PageData{}, Redirect{}, x.SeeOther("/login", identity.FlashSessionExpired)
			}
			if fields, ok := identity.FieldErrorsOf(err); ok {
				// IDN-083: re-render the form with the submitted values so the
				// two editable fields can be corrected in place.
				return x.Page("Profil", "profil", viewmodel.ProfilePage{
					FullName: x.Value("full_name"),
					Email:    x.Value("email"),
				}), Redirect{}, apperr.Validationf(fields)
			}
			return viewmodel.PageData{}, Redirect{}, identityFailure(err)
		},
		Action: func(PageRequest) string { return "/profile?flash=saved" },
	})
}
