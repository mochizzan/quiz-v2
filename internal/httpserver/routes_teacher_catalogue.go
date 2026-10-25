package httpserver

import (
	"strconv"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/participation"
	"quiz-v2/internal/viewmodel"
)

// This file is the "teacher-catalogue" route group, owned by the participation
// domain: anonymous-participant records and the major catalogue. The student
// roster and teacher approval belong to identity and live in
// routes_teacher_mgmt.go.
//
// Each route carries its production Load/ActForm/Act handler. Every write
// carries the shared teacher-management rate bucket and re-renders the failing
// form at 422.

// init registers the eight catalogue routes.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/anonymous",
		Template: "pages/teacher/anonymous.html",
		Nav:      "siswa",
		Auth:     ClassTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			// PAR-163…PAR-172: search and page are fallback parsers, never a
			// 422; the list is school-wide.
			list, err := parService(x).AnonymousList(x.Context(), participation.AnonymousListInput{
				Actor: parActor(x),
				Query: x.QueryValue("q"),
				Page:  x.QueryValue("page"),
			})
			if err != nil {
				return viewmodel.PageData{}, err
			}
			return x.Page("Peserta anonim", "siswa", list), nil
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/anonymous/:id",
		Template: "pages/teacher/anonymous-edit.html",
		Nav:      "siswa",
		Auth:     ClassTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			// PAR-173…PAR-178: a live anonymous_participants.id or the shared
			// 404. The GET only reads.
			form, err := parService(x).AnonymousFormPage(x.Context(), participation.AnonymousFormInput{
				Actor: parActor(x),
				ID:    x.IntParam("id"),
			})
			if err != nil {
				return viewmodel.PageData{}, err
			}
			return x.Page("Ubah peserta anonim", "siswa", form), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/anonymous/:id",
		Auth:   ClassTeacher,
		ActForm: func(x *Ctx) (viewmodel.PageData, Redirect, error) {
			if err := catalogueRate(x); err != nil {
				return viewmodel.PageData{}, Redirect{}, err
			}
			res, err := parService(x).EditAnonymous(x.Context(), participation.AnonymousEditInput{
				Actor:      parActor(x),
				ID:         x.IntParam("id"),
				FullName:   x.Value("full_name"),
				GradeLevel: x.Value("grade_level"),
				MajorID:    x.Value("major_id"),
			})
			if err != nil {
				if _, ok := participation.FieldErrorsOf(err); ok {
					return catalogue422(x, "Ubah peserta anonim", "siswa", res.Form), Redirect{}, err
				}
				return viewmodel.PageData{}, Redirect{}, err
			}
			return viewmodel.PageData{}, x.Redirect(res.Result.Path, res.Result.Flash), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/anonymous/:id/delete",
		Auth:   ClassTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			if err := catalogueRate(x); err != nil {
				return Redirect{}, err
			}
			res, err := parService(x).DeleteAnonymous(x.Context(), participation.AnonymousDeleteInput{
				Actor: parActor(x),
				ID:    x.IntParam("id"),
			})
			if err != nil {
				return Redirect{}, err
			}
			return x.Redirect(res.Path, res.Flash), nil
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/majors",
		Template: "pages/teacher/majors.html",
		Nav:      "pengaturan",
		Auth:     ClassTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			// PAR-179…PAR-188: no query parameters are read and the page is
			// one snapshot.
			page, err := parService(x).MajorsPage(x.Context(), participation.MajorsPageInput{
				Actor: parActor(x),
			})
			if err != nil {
				return viewmodel.PageData{}, err
			}
			return x.Page("Jurusan", "pengaturan", page), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/majors",
		Auth:   ClassTeacher,
		ActForm: func(x *Ctx) (viewmodel.PageData, Redirect, error) {
			if err := catalogueRate(x); err != nil {
				return viewmodel.PageData{}, Redirect{}, err
			}
			res, err := parService(x).CreateMajor(x.Context(), participation.MajorCreateInput{
				Actor: parActor(x),
				Name:  x.Value("name"),
			})
			if err != nil {
				if _, ok := participation.FieldErrorsOf(err); ok {
					return catalogue422(x, "Jurusan", "pengaturan", res.Page), Redirect{}, err
				}
				return viewmodel.PageData{}, Redirect{}, err
			}
			return viewmodel.PageData{}, x.Redirect(res.Result.Path, res.Result.Flash), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/majors/:id",
		Auth:   ClassTeacher,
		// The rename form lives on the majors list (GET /teacher/majors), so a
		// rejected rename re-renders that page rather than a path of its own.
		Template: "pages/teacher/majors.html",
		ActForm: func(x *Ctx) (viewmodel.PageData, Redirect, error) {
			if err := catalogueRate(x); err != nil {
				return viewmodel.PageData{}, Redirect{}, err
			}
			res, err := parService(x).RenameMajor(x.Context(), participation.MajorRenameInput{
				Actor: parActor(x),
				ID:    x.IntParam("id"),
				Name:  x.Value("name"),
			})
			if err != nil {
				if _, ok := participation.FieldErrorsOf(err); ok {
					return catalogue422(x, "Jurusan", "pengaturan", res.Page), Redirect{}, err
				}
				return viewmodel.PageData{}, Redirect{}, err
			}
			return viewmodel.PageData{}, x.Redirect(res.Result.Path, res.Result.Flash), nil
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/majors/:id/delete",
		Auth:   ClassTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			if err := catalogueRate(x); err != nil {
				return Redirect{}, err
			}
			res, err := parService(x).DeleteMajor(x.Context(), participation.MajorDeleteInput{
				Actor: parActor(x),
				ID:    x.IntParam("id"),
			})
			if err != nil {
				return Redirect{}, err
			}
			return x.Redirect(res.Path, res.Flash), nil
		},
	})
}

// catalogueRate enforces the PAR teacher-management bucket before any lookup
// or write, keyed by the acting account so one teacher cannot punish another
// (§8.7.8, PAR-096, PAR-109, PAR-115, PAR-120).
func catalogueRate(x *Ctx) error {
	if x.Session == nil {
		return apperr.NotFoundf().WithRule("IDN-012")
	}
	key := participation.RateKey("manage", strconv.FormatInt(x.Session.AccountID, 10))
	return parRate(x, key, participation.RateManageLimit)
}

// catalogue422 rebuilds a rejected form's page. The templates carry no
// per-field error slots (report gap), so the re-render uses a form-level
// danger alert; finish() attaches the field map for any template that grows
// slots later.
func catalogue422(x *Ctx, title, nav string, data any) viewmodel.PageData {
	page := x.Page(title, nav, data)
	page.Base.Flash = &viewmodel.Flash{Kind: "danger", Title: apperr.MsgValidation}
	return page
}
