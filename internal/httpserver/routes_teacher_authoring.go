package httpserver

import (
	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/viewmodel"
)

// This file is the "teacher-authoring" route group: the question editor and
// its create/save/delete/reorder actions. The quiz itself is created by
// routes_teacher_core.go and edited on its builder page.

// init registers the two question-editor GET screens and the four UI-only
// question actions. Every action redirects back to the quiz builder with a
// flash key; reordering posts the ordered question ids from the drag list.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/:id/questions/new",
		Template: "pages/teacher/question-editor.html",
		Nav:      "kuis",
		Build: func(req PageRequest) viewmodel.PageData {
			form := fixtures.NewQuestionFormData(req.IntParam("id"))
			return page(req, "Tambah soal", "kuis", fixtures.TeacherUser(), form)
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/questions",
		Action: teacherFlashRedirect("/teacher/quizzes/:id", "question-saved"),
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/:id/questions/:qid",
		Template: "pages/teacher/question-editor.html",
		Nav:      "kuis",
		Build: func(req PageRequest) viewmodel.PageData {
			form := fixtures.EditQuestionFormData(req.IntParam("id"), req.IntParam("qid"))
			return page(req, "Edit soal", "kuis", fixtures.TeacherUser(), form)
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/questions/:qid",
		Action: teacherFlashRedirect("/teacher/quizzes/:id", "question-saved"),
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/questions/:qid/delete",
		Action: teacherFlashRedirect("/teacher/quizzes/:id", "deleted"),
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/questions/reorder",
		Action: teacherFlashRedirect("/teacher/quizzes/:id", "reordered"),
	})
}
