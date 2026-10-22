package httpserver

import (
	"strconv"
	"strings"

	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/viewmodel"
)

// This file is the "teacher-management" route group: students, anonymous
// participants, majors and teacher approval. One routes_<group>.go file per
// group keeps parallel page tasks off each other's files and off server.go.
//
// POST routes are UI-only: they validate nothing, persist nothing, and return
// the 303 target the real backend would land on. HTML5 attributes plus app.js
// carry the client-side feedback.

// init registers the fifteen management routes; every page renders through
// layouts/teacher.html (the pages/teacher/ path convention).
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/students",
		Template: "pages/teacher/students.html",
		Nav:      "siswa",
		Build: func(req PageRequest) viewmodel.PageData {
			list := fixtures.StudentListPage(studentFilter(req))
			if req.Query["token"] != "" {
				list.ResetToken = fixtures.ResetToken()
			}
			return page(req, "Siswa", "siswa", fixtures.TeacherUser(), list)
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/students/:id",
		Template: "pages/teacher/student-edit.html",
		Nav:      "siswa",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Ubah siswa", "siswa", fixtures.TeacherUser(), fixtures.StudentFormPage(req.IntParam("id")))
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/students/:id",
		Action: func(req PageRequest) string {
			return withParam("/teacher/students/:id", req) + "?flash=saved"
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/students/:id/delete",
		Action: action("/teacher/students", "deleted"),
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/students/:id/reset-token",
		Action: func(PageRequest) string {
			return "/teacher/students?token=" + fixtures.DemoResetToken + "&flash=token"
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/anonymous",
		Template: "pages/teacher/anonymous.html",
		Nav:      "siswa",
		Build: func(req PageRequest) viewmodel.PageData {
			filter := fixtures.AnonymousFilter{Query: req.Query["q"], Page: pageNumber(req)}
			return page(req, "Peserta anonim", "siswa", fixtures.TeacherUser(), fixtures.AnonymousListPage(filter))
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/anonymous/:id",
		Template: "pages/teacher/anonymous-edit.html",
		Nav:      "siswa",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Ubah peserta anonim", "siswa", fixtures.TeacherUser(), fixtures.AnonymousFormPage(req.IntParam("id")))
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/anonymous/:id",
		Action: action("/teacher/anonymous", "saved"),
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/anonymous/:id/delete",
		Action: action("/teacher/anonymous", "deleted"),
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/majors",
		Template: "pages/teacher/majors.html",
		Nav:      "pengaturan",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Jurusan", "pengaturan", fixtures.TeacherUser(), fixtures.MajorsPage())
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/majors",
		Action: action("/teacher/majors", "saved"),
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/majors/:id",
		Action: action("/teacher/majors", "saved"),
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/majors/:id/delete",
		Action: action("/teacher/majors", "deleted"),
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/teachers",
		Template: "pages/teacher/teachers.html",
		Nav:      "pengaturan",
		Build: func(req PageRequest) viewmodel.PageData {
			return page(req, "Guru", "pengaturan", fixtures.TeacherUser(), fixtures.TeachersPage())
		},
	})
	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/teachers/:id/approve",
		Action: action("/teacher/teachers", "approved"),
	})
}

// studentFilter parses the list query into the neutral fixture filter. The
// grade tabs may echo their label ("Kelas XI") into the form, so both the
// numeric grade and its label are accepted.
func studentFilter(req PageRequest) fixtures.StudentFilter {
	majorID, _ := strconv.ParseInt(req.Query["major_id"], 10, 64)
	return fixtures.StudentFilter{
		Query:   strings.TrimSpace(req.Query["q"]),
		Grade:   parseGrade(req.Query["grade"]),
		MajorID: majorID,
		Sort:    viewmodel.SortOrder(req.Query["sort"]),
		Page:    pageNumber(req),
	}
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
