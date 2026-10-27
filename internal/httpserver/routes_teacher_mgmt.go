package httpserver

import (
	"errors"
	"strconv"
	"strings"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/identity"
	"quiz-v2/internal/viewmodel"
)

// This file is the "teacher-management" route group, owned by the identity
// domain: the student roster and its edit/delete/reset-token commands, teacher
// approval, and the signed-in teacher's own account (IDN-C23). Anonymous
// participants and majors belong to the participation domain and live in
// routes_teacher_catalogue.go.
//
// Each route carries its production Load/Act handler. The edit command uses
// ActForm so a rejected edit re-renders the same form with the teacher's values
// (IDN-083).

func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/students",
		Template: "pages/teacher/students.html",
		Nav:      "manajemen",
		Auth:     ClassTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			sess, serr := identitySession(x)
			if serr != nil {
				return viewmodel.PageData{}, serr
			}
			// IDN-C16, §6.1: every list input is a fallback parser, never a
			// 422; a page, sort, grade or major outside the closed set simply
			// reads as its default.
			input := identity.RosterInput{
				Query:   identity.NormalizeSearch(x.QueryValue("q")),
				Grade:   identity.ParseGradeFilter(x.QueryValue("grade")),
				Sort:    identity.ParseSort(x.QueryValue("sort")),
				Page:    identity.ParsePage(x.QueryValue("page")),
				ActorID: sess.AccountID,
			}
			if majorID, ok := identity.ParseID(x.QueryValue("major_id")); ok {
				input.MajorID = majorID
			}
			// IDN-105: the reveal cookie is one-shot. Its payload is verified
			// against a live, unused, unexpired token in the read snapshot;
			// whether the panel renders or not, the cookie is consumed.
			revealCookie := x.c.Cookies(identity.CookieReveal)
			if revealCookie != "" {
				if reveal, perr := identity.ParseRevealToken(x.Signer(), revealCookie, x.Now()); perr == nil {
					input.Reveal = reveal
				}
			}
			list, err := identityService(x).Students(x.c.Context(), input)
			if err != nil {
				return viewmodel.PageData{}, identityFailure(err)
			}
			if revealCookie != "" {
				x.ClearCookie(identity.CookieReveal, identity.RevealCookiePath)
			}
			return identityPage(x, "Siswa", "manajemen", list), nil
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/students/:id",
		Template: "pages/teacher/student-edit.html",
		Nav:      "manajemen",
		Auth:     ClassTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			if _, serr := identitySession(x); serr != nil {
				return viewmodel.PageData{}, serr
			}
			// IDN-C17: :id is a students.id; a missing, soft-deleted, or
			// teacher account id is the same 404.
			form, err := identityService(x).StudentForm(x.c.Context(), x.IntParam("id"))
			if err != nil {
				return viewmodel.PageData{}, identityFailure(err)
			}
			return identityPage(x, "Ubah siswa", "manajemen", form), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/students/:id",
		Auth:   ClassTeacher,
		ActForm: func(x *Ctx) (viewmodel.PageData, Redirect, error) {
			id := x.IntParam("id")
			err := identityService(x).UpdateStudent(x.c.Context(), id, identity.StudentInput{
				FullName: x.Value("full_name"),
				Email:    x.Value("email"),
				Grade:    x.Value("grade_level"),
				MajorID:  x.Value("major_id"),
			})
			switch {
			case err == nil:
				return viewmodel.PageData{}, Redirect{Path: "/teacher/students/" + strconv.FormatInt(id, 10), Flash: identity.FlashSaved}, nil
			case errors.Is(err, identity.ErrMajorRequired), errors.Is(err, identity.ErrEmailTaken):
				// The row pool re-read below only covers the form-validation
				// branch; these two are field-scoped too and fall through to it.
			}
			if fields, ok := identity.FieldErrorsOf(err); ok {
				return studentEdit422(x, id, fields)
			}
			return viewmodel.PageData{}, Redirect{}, identityFailure(err)
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/students/:id/delete",
		Auth:   ClassTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			sess, serr := identitySession(x)
			if serr != nil {
				return Redirect{}, serr
			}
			// IDN-C19: one timestamp stamps the account and its full cascade;
			// only the account row carries the acting teacher.
			err := identityService(x).DeleteStudent(x.c.Context(), sess.AccountID, x.IntParam("id"))
			if err != nil {
				return Redirect{}, identityFailure(err)
			}
			return identityRedirect("/teacher/students", identity.FlashDeletedStudent), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/students/:id/reset-token",
		Auth:   ClassTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			sess, serr := identitySession(x)
			if serr != nil {
				return Redirect{}, serr
			}
			// IDN-C20: the bucket is the issuing teacher, not the student, so
			// nobody can burn another teacher's issue budget.
			if err := identityRate(x, identity.ResetIssueKey(sess.AccountID), identity.ResetIssueLimit); err != nil {
				return Redirect{}, err
			}
			issued, err := identityService(x).IssueReset(x.c.Context(), sess.AccountID, x.IntParam("id"))
			if err != nil {
				return Redirect{}, identityFailure(err)
			}
			// IDN-105: the raw token reaches the teacher through the signed
			// five-minute cookie scoped to the students list, never a URL.
			revealToken, terr := identity.NewRevealToken(x.Signer(), identity.Reveal{
				Teacher:   sess.AccountID,
				Student:   issued.StudentID,
				Token:     issued.Token,
				ExpiresAt: issued.ExpiresAt.Unix(),
			}, x.Now())
			if terr != nil {
				return Redirect{}, apperr.Internalf(terr).WithRule("IDN-105")
			}
			x.SetCookie(identity.CookieReveal, revealToken, identity.RevealCookieMaxAge, identity.RevealCookiePath)
			return identityRedirect("/teacher/students", identity.FlashToken), nil
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/teachers",
		Template: "pages/teacher/teachers.html",
		Nav:      "manajemen",
		Auth:     ClassTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			if _, serr := identitySession(x); serr != nil {
				return viewmodel.PageData{}, serr
			}
			// IDN-C21: both sections from one snapshot; the approver name is
			// the current one, never a snapshot.
			teachers, err := identityService(x).Teachers(x.c.Context())
			if err != nil {
				return viewmodel.PageData{}, identityFailure(err)
			}
			return identityPage(x, "Guru", "manajemen", teachers), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/teachers/:id/approve",
		Auth:   ClassTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			sess, serr := identitySession(x)
			if serr != nil {
				return Redirect{}, serr
			}
			// IDN-C22: the monotonic update both approves a pending teacher
			// and reports the already-active target as idempotent success.
			flash, err := identityService(x).ApproveTeacher(x.c.Context(), sess.AccountID, x.IntParam("id"))
			if err != nil {
				return Redirect{}, identityFailure(err)
			}
			return identityRedirect("/teacher/teachers", flash), nil
		},
	})

	// IDN-C23/C24: the teacher's own account. R-05 kept teacher identity immutable
	// in v1; this screen removes that exception for name and email, so a
	// registration typo is correctable without an out-of-band reset. The
	// password still changes at IDN-C11, and the role itself is never writable.
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/profile",
		Template: "pages/teacher/profile.html",
		Nav:      "profil",
		Auth:     ClassTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			sess, serr := identitySession(x)
			if serr != nil {
				return viewmodel.PageData{}, serr
			}
			account, err := identityService(x).TeacherProfile(x.c.Context(), sess.AccountID)
			if err != nil {
				return viewmodel.PageData{}, identityFailure(err)
			}
			return identityPage(x, "Profil", "profil", account), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/profile",
		Auth:   ClassTeacher,
		ActForm: func(x *Ctx) (viewmodel.PageData, Redirect, error) {
			sess, serr := identitySession(x)
			if serr != nil {
				return viewmodel.PageData{}, Redirect{}, serr
			}
			err := identityService(x).SaveProfile(x.c.Context(), sess.AccountID, identity.ProfileInput{
				FullName: x.Value("full_name"),
				Email:    x.Value("email"),
			})
			switch {
			case err == nil:
				return viewmodel.PageData{}, Redirect{Path: "/teacher/profile", Flash: identity.FlashSaved}, nil
			case errors.Is(err, identity.ErrSessionGone):
				return viewmodel.PageData{}, Redirect{}, x.SeeOther("/login", identity.FlashSessionExpired)
			}
			if fields, ok := identity.FieldErrorsOf(err); ok {
				// IDN-083: re-render with the submitted pair so the field that
				// failed validation can be corrected in place.
				return x.Page("Profil", "profil", viewmodel.TeacherProfilePage{
					FullName: x.Value("full_name"),
					Email:    x.Value("email"),
				}), Redirect{}, apperr.Validationf(fields)
			}
			return viewmodel.PageData{}, Redirect{}, identityFailure(err)
		},
	})
}

// studentEdit422 re-renders the edit form at 422: the live row supplies its
// majors and coherent current values, and the submitted values overlay them
// where the submission parsed (IDN-083). A row deleted meanwhile is the same
// 404 the load answers.
func studentEdit422(x *Ctx, id int64, fields map[string]string) (viewmodel.PageData, Redirect, error) {
	form, err := identityService(x).StudentForm(x.c.Context(), id)
	if err != nil {
		return viewmodel.PageData{}, Redirect{}, identityFailure(err)
	}
	form.FullName = x.Value("full_name")
	form.Email = x.Value("email")
	if grade, err := identity.ParseGradeLevel(x.Value("grade_level")); err == nil && grade != 0 {
		form.GradeLevel = grade
	}
	if submitted, ok := identity.ParseID(x.Value("major_id")); ok {
		for _, m := range form.Majors {
			if m.ID == submitted {
				form.MajorID = submitted
				break
			}
		}
	}
	data := x.Page("Ubah siswa", "manajemen", form)
	// The template carries no per-field error slots, so the re-render keeps a
	// form-level alert; per-field copy is a template delta recorded in the
	// report (§12.2).
	data.Base.Flash = &viewmodel.Flash{Kind: "danger", Title: apperr.MsgValidation}
	return data, Redirect{}, apperr.Validationf(fields)
}

func parseGrade(raw string) int {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.TrimSpace(strings.TrimPrefix(v, "kelas"))
	switch v {
	case "10", "x":
		return 10
	case "11", "xi":
		return 11
	case "12", "xii":
		return 12
	default:
		return 0
	}
}
