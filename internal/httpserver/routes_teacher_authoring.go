package httpserver

import (
	"io"
	"strings"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/authoring"
	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/viewmodel"
)

// This file is the "teacher-authoring" route group: the question editor and
// its create/save/delete/reorder actions. The quiz itself is created by
// routes_teacher_core.go and edited on its builder page.
//
// The two question POSTs are multipart-only. The pipeline has already parsed the
// multipart body (so CSRF and the text fields are available) and enforced its
// transport cap; this layer only hands the image parts to the domain, which
// sniffs and stages them.

// questionInput reads the editor fields. Alt presence is distinct from an empty
// alt: a submitted blank clears the stored alt, an absent field keeps it.
func questionInput(x *Ctx, uploads []authoring.Upload) authoring.QuestionInput {
	_, altPresent := x.Form["image_alt"]
	return authoring.QuestionInput{
		Type:           x.Value("type"),
		Body:           x.RawValue("body"),
		ImageAlt:       x.RawValue("image_alt"),
		AltPresent:     altPresent,
		Options:        x.Form["option_body[]"],
		CorrectIndex:   x.Form["correct_index"],
		CorrectIndexes: x.Form["correct_indexes"],
		RemoveImage:    x.Checked("remove_image"),
		Images:         uploads,
	}
}

// questionUploads reads the one optional image part. An entirely empty part is
// the browser's "no file chosen" placeholder, not an uploaded empty file; a
// chosen zero-byte file keeps its filename and is rejected by validation.
func questionUploads(x *Ctx) ([]authoring.Upload, error) {
	if !strings.HasPrefix(string(x.c.Request().Header.ContentType()), "multipart/") {
		return nil, apperr.Field("_form", authoring.MsgFormInvalid)
	}
	form, err := x.c.MultipartForm()
	if err != nil || form == nil {
		return nil, apperr.Field("_form", authoring.MsgFormInvalid)
	}

	var out []authoring.Upload
	for _, header := range form.File["image"] {
		if header.Filename == "" && header.Size == 0 {
			continue
		}
		file, openErr := header.Open()
		if openErr != nil {
			out = append(out, authoring.Upload{Filename: header.Filename, ReadErr: openErr})
			continue
		}
		data, readErr := io.ReadAll(file)
		_ = file.Close()
		out = append(out, authoring.Upload{
			Filename:    header.Filename,
			ContentType: header.Header.Get("Content-Type"),
			Data:        data,
			ReadErr:     readErr,
		})
	}
	return out, nil
}

// init registers the two question-editor GET screens and the four question
// actions. Every action redirects back to the quiz builder with a flash key;
// reordering posts the ordered question ids from the drag list.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/:id/questions/new",
		Template: "pages/teacher/question-editor.html",
		Nav:      "kuis",
		Auth:     ClassOwnerTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return viewmodel.PageData{}, err
			}
			quizID := x.IntParam("id")
			form, err := authoringService(x).NewQuestionForm(x.c.Context(), actor, quizID)
			if err != nil {
				if key, ok := authoring.ConflictKey(err); ok {
					return viewmodel.PageData{}, x.SeeOther(teacherQuizPath(quizID)+"?flash="+key, "")
				}
				return viewmodel.PageData{}, err
			}
			return authoringPage(x, "Tambah soal", "kuis", form), nil
		},
		Build: func(req PageRequest) viewmodel.PageData {
			form := fixtures.NewQuestionFormData(req.IntParam("id"))
			return page(req, "Tambah soal", "kuis", fixtures.TeacherUser(), form)
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/questions",
		Auth:   ClassOwnerTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return Redirect{}, err
			}
			uploads, err := questionUploads(x)
			if err != nil {
				return Redirect{}, err
			}
			quizID := x.IntParam("id")
			flash, err := authoringService(x).CreateQuestion(
				x.c.Context(), actor, quizID, questionInput(x, uploads))
			if err != nil {
				if redirect, ok := authoringConflictRedirect(err, teacherQuizPath(quizID)); ok {
					return redirect, nil
				}
				return Redirect{}, err
			}
			return authoringRedirect(teacherQuizPath(quizID), flash), nil
		},
		Action: teacherFlashRedirect("/teacher/quizzes/:id", "question-saved"),
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/:id/questions/:qid",
		Template: "pages/teacher/question-editor.html",
		Nav:      "kuis",
		Auth:     ClassOwnerTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return viewmodel.PageData{}, err
			}
			quizID := x.IntParam("id")
			form, err := authoringService(x).EditQuestionForm(
				x.c.Context(), actor, quizID, x.IntParam("qid"))
			if err != nil {
				if key, ok := authoring.ConflictKey(err); ok {
					return viewmodel.PageData{}, x.SeeOther(teacherQuizPath(quizID)+"?flash="+key, "")
				}
				return viewmodel.PageData{}, err
			}
			return authoringPage(x, "Edit soal", "kuis", form), nil
		},
		Build: func(req PageRequest) viewmodel.PageData {
			form := fixtures.EditQuestionFormData(req.IntParam("id"), req.IntParam("qid"))
			return page(req, "Edit soal", "kuis", fixtures.TeacherUser(), form)
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/questions/:qid",
		Auth:   ClassOwnerTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return Redirect{}, err
			}
			uploads, err := questionUploads(x)
			if err != nil {
				return Redirect{}, err
			}
			quizID := x.IntParam("id")
			flash, err := authoringService(x).UpdateQuestion(
				x.c.Context(), actor, quizID, x.IntParam("qid"), questionInput(x, uploads))
			if err != nil {
				if redirect, ok := authoringConflictRedirect(err, teacherQuizPath(quizID)); ok {
					return redirect, nil
				}
				return Redirect{}, err
			}
			return authoringRedirect(teacherQuizPath(quizID), flash), nil
		},
		Action: teacherFlashRedirect("/teacher/quizzes/:id", "question-saved"),
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/questions/:qid/delete",
		Auth:   ClassOwnerTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return Redirect{}, err
			}
			quizID := x.IntParam("id")
			flash, err := authoringService(x).DeleteQuestion(
				x.c.Context(), actor, quizID, x.IntParam("qid"))
			if err != nil {
				if redirect, ok := authoringConflictRedirect(err, teacherQuizPath(quizID)); ok {
					return redirect, nil
				}
				return Redirect{}, err
			}
			return authoringRedirect(teacherQuizPath(quizID), flash), nil
		},
		Action: teacherFlashRedirect("/teacher/quizzes/:id", "deleted"),
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/questions/reorder",
		Auth:   ClassOwnerTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return Redirect{}, err
			}
			quizID := x.IntParam("id")
			flash, err := authoringService(x).ReorderQuestions(
				x.c.Context(), actor, quizID, x.Value("order"))
			if err != nil {
				if redirect, ok := authoringConflictRedirect(err, teacherQuizPath(quizID)); ok {
					return redirect, nil
				}
				return Redirect{}, err
			}
			return authoringRedirect(teacherQuizPath(quizID), flash), nil
		},
		Action: teacherFlashRedirect("/teacher/quizzes/:id", "reordered"),
	})
}
