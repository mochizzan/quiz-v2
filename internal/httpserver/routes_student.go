package httpserver

// This file is the "student" route group: the student dashboard, quiz history
// (list and detail), the pre-flight quiz intro, and the three participation
// commands. The profile screen belongs to the identity domain and lives in
// routes_profile.go.
//
// Load/Act are the handlers (PAR, spec 03).

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/attempts"
	"quiz-v2/internal/participation"
	"quiz-v2/internal/viewmodel"
)

// parService builds the per-request participation service. The ATT start rule
// is injected here (PAR-202): PAR opens the transaction and hands it the locked
// plan; ATT inserts the attempt and its slots inside that same transaction.
func parService(x *Ctx) *participation.Service {
	svc := participation.New(x.DB, []byte(x.server.cfg.SessionSigningKey))
	svc.AttemptStart = func(ctx context.Context, tx *sql.Tx, plan participation.StartPlan) (uint32, error) {
		res, err := attempts.Start(ctx, tx, attempts.StartInput{
			RegistrationID:         plan.RegistrationID,
			QuizID:                 plan.QuizID,
			AttemptNo:              plan.AttemptNo,
			StudentID:              plan.StudentID,
			AnonymousParticipantID: plan.AnonymousParticipantID,
		})
		if err != nil {
			return 0, err
		}
		return res.AttemptID, nil
	}
	return svc
}

// parActor is the raw identity material for one participation request. The
// domain re-resolves it inside its own transaction (PAR-056, PAR-198).
func parActor(x *Ctx) participation.ActorInput {
	in := participation.ActorInput{
		IdentityToken: x.Cookie(participation.IdentityCookie),
		TicketValue:   x.Cookie(participation.TicketCookie),
	}
	if x.Session != nil {
		in.AccountID = x.Session.AccountID
		in.Role = x.Session.Role
		in.FullName = x.Session.FullName
	}
	return in
}

// parQuizID parses :id; a malformed id becomes 0, which the domain answers 404.
func parQuizID(x *Ctx) int64 {
	id, err := strconv.ParseInt(x.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// parRedirect maps a domain redirect to the transport 303.
func parRedirect(x *Ctx, err error) error {
	var r *participation.Redirect
	if errors.As(err, &r) {
		return x.SeeOther(r.Path, r.Flash)
	}
	return err
}

// studentDashboardLoad serves GET /student (PAR-126…PAR-141).
func studentDashboardLoad(x *Ctx) (viewmodel.PageData, error) {
	if !x.IsStudent() {
		return viewmodel.PageData{}, apperr.NotFoundf().WithRule("PAR-126")
	}
	data, err := parService(x).StudentDashboard(x.Context(), participation.StudentDashboardInput{
		Actor: parActor(x),
	})
	if err != nil {
		return viewmodel.PageData{}, err
	}
	return x.Page("Dasbor siswa", "dasbor", data), nil
}

// studentHistoryLoad serves GET /student/history (PAR-142…PAR-153).
func studentHistoryLoad(x *Ctx) (viewmodel.PageData, error) {
	if !x.IsStudent() {
		return viewmodel.PageData{}, apperr.NotFoundf().WithRule("PAR-142")
	}
	data, err := parService(x).StudentHistory(x.Context(), participation.StudentHistoryInput{
		Actor: parActor(x),
		Page:  x.QueryValue("page"),
	})
	if err != nil {
		return viewmodel.PageData{}, err
	}
	return x.Page("Riwayat kuis", "riwayat", data), nil
}

// studentHistoryDetailLoad serves GET /student/history/:quizID
// (PAR-154…PAR-162).
func studentHistoryDetailLoad(x *Ctx) (viewmodel.PageData, error) {
	if !x.IsStudent() {
		return viewmodel.PageData{}, apperr.NotFoundf().WithRule("PAR-154")
	}
	data, err := parService(x).StudentHistoryDetail(x.Context(), participation.StudentHistoryDetailInput{
		Actor:  parActor(x),
		QuizID: x.IntParam("quizID"),
	})
	if err != nil {
		return viewmodel.PageData{}, err
	}
	title := "Riwayat kuis"
	if data.Quiz.Title != "" {
		title = data.Quiz.Title
	}
	return x.Page(title, "riwayat", data), nil
}

// studentQuizLoad serves GET /quiz/:id (PAR-087…PAR-095).
func studentQuizLoad(x *Ctx) (viewmodel.PageData, error) {
	data, err := parService(x).QuizIntro(x.Context(), participation.QuizIntroInput{
		QuizID: parQuizID(x),
		Actor:  parActor(x),
	})
	if err != nil {
		return viewmodel.PageData{}, parRedirect(x, err)
	}
	return x.Page(data.Quiz.Title, "", data), nil
}

// studentRegisterAct serves POST /quiz/:id/register (PAR-061…PAR-068).
func studentRegisterAct(x *Ctx) (Redirect, error) {
	res, err := parService(x).Register(x.Context(), participation.RegisterInput{
		QuizID: parQuizID(x),
		Actor:  parActor(x),
	})
	if err != nil {
		return Redirect{}, err
	}
	return x.Redirect(res.Path, res.Flash), nil
}

// studentCancelAct serves POST /quiz/:id/cancel (PAR-070…PAR-077).
func studentCancelAct(x *Ctx) (Redirect, error) {
	res, err := parService(x).Cancel(x.Context(), participation.CancelInput{
		QuizID: parQuizID(x),
		Actor:  parActor(x),
	})
	if err != nil {
		return Redirect{}, err
	}
	return x.Redirect(res.Path, res.Flash), nil
}

// studentStartAct serves POST /quiz/:id/attempts (PAR-078…PAR-086).
func studentStartAct(x *Ctx) (Redirect, error) {
	res, err := parService(x).StartAttempt(x.Context(), participation.StartQuizInput{
		QuizID: parQuizID(x),
		Actor:  parActor(x),
	})
	if err != nil {
		return Redirect{}, err
	}
	return x.Redirect(res.Path, res.Flash), nil
}

// init registers the five student GET screens and the four UI-only POST
// actions. POST routes carry no Build: they validate nothing and redirect 303
// so a browser refresh never re-submits.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/student",
		Template: "pages/student/dashboard.html",
		Nav:      "dasbor",
		Load:     studentDashboardLoad,
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/student/history",
		Template: "pages/student/history.html",
		Nav:      "riwayat",
		Load:     studentHistoryLoad,
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/student/history/:quizID",
		Template: "pages/student/history-detail.html",
		Nav:      "riwayat",
		Load:     studentHistoryDetailLoad,
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/quiz/:id",
		Template: "pages/student/quiz.html",
		Load:     studentQuizLoad,
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/quiz/:id/register",
		Act:    studentRegisterAct,
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/quiz/:id/cancel",
		Act:    studentCancelAct,
	})

	registerPage(PageRoute{
		Method: "POST",
		Path:   "/quiz/:id/attempts",
		Act:    studentStartAct,
	})
}
