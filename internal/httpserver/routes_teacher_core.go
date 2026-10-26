package httpserver

import (
	"strconv"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/authoring"
	"quiz-v2/internal/viewmodel"
)

// This file is the "teacher-authoring" route group, owned by the authoring
// domain: the quiz list, the builder, and the lifecycle commands. The teacher
// dashboard, results, monitor and stream belong to attempts grading and live in
// routes_teacher_dashboard.go; question editing lives in
// routes_teacher_authoring.go.
//
// Each route carries its production Load/Act handler. Auth restates the class
// in authz.go so a route file is auditable on its own.

// teacherQuizPath is the canonical builder URL.
func teacherQuizPath(id int64) string {
	return "/teacher/quizzes/" + strconv.FormatInt(id, 10)
}

// teacherQuestionPath is the canonical question editor URL.
func teacherQuestionPath(quizID, questionID int64) string {
	return teacherQuizPath(quizID) + "/questions/" + strconv.FormatInt(questionID, 10)
}

// authoringService binds the domain to this request's pool and upload root.
func authoringService(x *Ctx) *authoring.Service {
	return authoring.New(x.DB, x.UploadsDir()).WithLogger(x.server.logger)
}

// authoringActor returns the signed-in account id. The route class has already
// admitted an active teacher; a nil session is a programming fault, answered as
// the same 404 the class guard would produce.
func authoringActor(x *Ctx) (int64, error) {
	if x.Session == nil {
		return 0, apperr.NotFoundf()
	}
	return x.Session.AccountID, nil
}

// authoringPage assembles the chrome and resolves the closed query flash set.
// New AUT-033 keys are carried in the query because viewmodel.FlashFor does not
// know them; an unknown key is never reflected.
func authoringPage(x *Ctx, title, nav string, data any) viewmodel.PageData {
	page := x.Page(title, nav, data)
	if page.Base.Flash == nil {
		if key := x.QueryValue("flash"); key != "" {
			page.Base.Flash = authoring.FlashForKey(key)
		}
	}
	return page
}

// authoringRedirect chooses the transport for a closed flash key: a key the
// shared view model knows becomes a signed one-shot cookie, and a domain-only
// key rides the query for authoringPage to resolve.
func authoringRedirect(path, flash string) Redirect {
	if flash == "" {
		return Redirect{Path: path}
	}
	if viewmodel.FlashFor(flash) != nil {
		return Redirect{Path: path, Flash: flash}
	}
	return Redirect{Path: path + "?flash=" + flash}
}

// authoringConflictRedirect turns a legal-but-refused transition into a 303.
func authoringConflictRedirect(err error, path string) (Redirect, bool) {
	key, ok := authoring.ConflictKey(err)
	if !ok {
		return Redirect{}, false
	}
	return Redirect{Path: path + "?flash=" + key}, true
}

// queryPage reads ?page= and clamps bad or missing input to the first page.
func queryPage(x *Ctx) int {
	n, err := strconv.Atoi(x.QueryValue("page"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// quizInput reads the create/metadata form. The metadata branch never reads
// visibility or the target pair, so carrying them is harmless.
func quizInput(x *Ctx) authoring.QuizInput {
	return authoring.QuizInput{
		Title:         x.RawValue("title"),
		Description:   x.RawValue("description"),
		StartAt:       x.RawValue("start_at"),
		EndAt:         x.RawValue("end_at"),
		Visibility:    x.Value("visibility"),
		TargetGrade:   x.Value("target_grade"),
		TargetMajorID: x.Value("target_major_id"),
	}
}

// init registers the eight quiz-authoring routes.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes",
		Template: "pages/teacher/quizzes.html",
		Nav:      "kuis",
		Auth:     ClassTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return viewmodel.PageData{}, err
			}
			list, err := authoringService(x).List(x.c.Context(), actor, authoring.ListInput{
				State: x.QueryValue("state"),
				Query: x.QueryValue("q"),
				Sort:  x.QueryValue("sort"),
				Page:  queryPage(x),
			})
			if err != nil {
				return viewmodel.PageData{}, err
			}
			return authoringPage(x, "Daftar kuis", "kuis", list), nil
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/new",
		Template: "pages/teacher/quiz-new.html",
		Nav:      "kuis",
		Auth:     ClassTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			form, err := authoringService(x).NewQuizForm(x.c.Context())
			if err != nil {
				return viewmodel.PageData{}, err
			}
			return authoringPage(x, "Buat kuis", "kuis", form), nil
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes",
		Auth:   ClassTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return Redirect{}, err
			}
			id, flash, err := authoringService(x).CreateQuiz(x.c.Context(), actor, quizInput(x))
			if err != nil {
				return Redirect{}, err
			}
			return authoringRedirect(teacherQuizPath(id), flash), nil
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/:id",
		Template: "pages/teacher/quiz-detail.html",
		Nav:      "kuis",
		Auth:     ClassOwnerTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return viewmodel.PageData{}, err
			}
			quizID := x.IntParam("id")
			detail, err := authoringService(x).Detail(x.c.Context(), actor, quizID, x.Session.FullName)
			if err != nil {
				return viewmodel.PageData{}, err
			}
			// The share card copies this link into messages students receive,
			// so it must carry the request's own origin, not just the path.
			if detail.InviteURL != "" {
				detail.InviteURL = x.c.BaseURL() + detail.InviteURL
			}
			title := "Detail kuis"
			if detail.Quiz.Title != "" {
				title = detail.Quiz.Title
			}
			return authoringPage(x, title, "kuis", detail), nil
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id",
		Auth:   ClassOwnerTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return Redirect{}, err
			}
			quizID := x.IntParam("id")
			target := teacherQuizPath(quizID)
			svc := authoringService(x)

			var flash string
			switch x.Value("form") {
			case "meta":
				flash, err = svc.SaveMeta(x.c.Context(), actor, quizID, quizInput(x))
			case "", "settings":
				flash, err = svc.SaveSettings(x.c.Context(), actor, quizID, x.Form)
			default:
				return Redirect{}, apperr.Field("_form", authoring.MsgFormInvalid)
			}
			if err != nil {
				if redirect, ok := authoringConflictRedirect(err, target); ok {
					return redirect, nil
				}
				return Redirect{}, err
			}
			return authoringRedirect(target, flash), nil
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/publish",
		Auth:   ClassOwnerTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return Redirect{}, err
			}
			quizID := x.IntParam("id")
			target := teacherQuizPath(quizID)
			flash, err := authoringService(x).Publish(x.c.Context(), actor, quizID)
			if err != nil {
				if redirect, ok := authoringConflictRedirect(err, target); ok {
					return redirect, nil
				}
				return Redirect{}, err
			}
			return authoringRedirect(target, flash), nil
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/close",
		Auth:   ClassOwnerTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return Redirect{}, err
			}
			quizID := x.IntParam("id")
			target := teacherQuizPath(quizID)
			flash, err := authoringService(x).Close(x.c.Context(), actor, quizID)
			if err != nil {
				if redirect, ok := authoringConflictRedirect(err, target); ok {
					return redirect, nil
				}
				return Redirect{}, err
			}
			return authoringRedirect(target, flash), nil
		},
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/teacher/quizzes/:id/delete",
		Auth:   ClassOwnerTeacher,
		Act: func(x *Ctx) (Redirect, error) {
			actor, err := authoringActor(x)
			if err != nil {
				return Redirect{}, err
			}
			quizID := x.IntParam("id")
			flash, err := authoringService(x).DeleteQuiz(x.c.Context(), actor, quizID)
			if err != nil {
				if redirect, ok := authoringConflictRedirect(err, teacherQuizPath(quizID)); ok {
					return redirect, nil
				}
				return Redirect{}, err
			}
			return authoringRedirect("/teacher/quizzes", flash), nil
		},
	})
}
