package httpserver

import (
	"errors"
	"strconv"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/attempts"
	"quiz-v2/internal/httpserver/fixtures"
	"quiz-v2/internal/viewmodel"
)

// This file is the "teacher-grading" route group: the essay queue, the
// per-answer grading detail, the save action and the attempt recap. One
// routes_<group>.go file per group keeps parallel page tasks off each other's
// files and off server.go.

// gradingAnswerHref is the detail/conflict target for one essay answer.
func gradingAnswerHref(answerID int64) string {
	return "/teacher/grading/" + strconv.FormatInt(answerID, 10)
}

// gradingQueueInput reads the shared filter set (ATT-201, ATT-211). Invalid
// values fall back to the defaults inside the domain; a query never 422s.
func gradingQueueInput(x *Ctx, accountID int64) attempts.GradingQueueInput {
	return attempts.GradingQueueInput{
		AccountID: accountID,
		QuizRaw:   x.QueryValue("quiz"),
		ClassRaw:  x.QueryValue("class"),
		QueryRaw:  x.QueryValue("q"),
		PageRaw:   x.QueryValue("page"),
	}
}

// gradingDetail loads the queue and one selected essay in a single read-only
// transaction. The answer id is the row key; a missing, foreign, objective, or
// in-progress answer is ErrNotFound (ATT-211).
func gradingDetail(x *Ctx, accountID, answerID int64) (viewmodel.GradingPage, error) {
	page, err := attemptService(x).GradingDetail(x.Context(), attempts.GradingDetailInput{
		GradingQueueInput: gradingQueueInput(x, accountID),
		AnswerID:          answerID,
	})
	if err != nil {
		return viewmodel.GradingPage{}, attemptTeacherError(err)
	}
	return page, nil
}

// gradingDraft overlays the submitted values on a 422 re-render (ATT-222). The
// stored values stay everywhere else: a graded answer keeps its stored score
// and feedback because there is no re-grade path (ATT-214). The frozen template
// renders the input through Score.Int, so a one-decimal submission can only
// echo as its whole display percent (template delta).
func gradingDraft(selected *viewmodel.GradingDetail, in attempts.GradeInput) *viewmodel.GradingDetail {
	if selected == nil || selected.Answer.State == viewmodel.EssayGraded {
		return selected
	}
	selected.Feedback = in.FeedbackRaw
	if units, err := attempts.GradeScore(in.ScoreRaw); err == nil {
		selected.Score = &viewmodel.Score{Percent: attempts.DisplayPercent(units)}
	} else {
		selected.Score = nil
	}
	return selected
}

// gradingQueueLoad serves GET /teacher/grading (ATT-201).
func gradingQueueLoad(x *Ctx) (viewmodel.PageData, error) {
	accountID, err := teacherActor(x)
	if err != nil {
		return viewmodel.PageData{}, err
	}
	page, err := attemptService(x).GradingQueue(x.Context(), gradingQueueInput(x, accountID))
	if err != nil {
		return viewmodel.PageData{}, attemptTeacherError(err)
	}
	return x.Page("Penilaian esai", "penilaian", page), nil
}

// gradingDetailLoad serves GET /teacher/grading/:answerID (ATT-211).
func gradingDetailLoad(x *Ctx) (viewmodel.PageData, error) {
	accountID, err := teacherActor(x)
	if err != nil {
		return viewmodel.PageData{}, err
	}
	page, err := gradingDetail(x, accountID, x.IntParam("answerID"))
	if err != nil {
		return viewmodel.PageData{}, err
	}
	return x.Page("Penilaian esai", "penilaian", page), nil
}

// gradingAct serves POST /teacher/grading/:answerID (ATT-221…ATT-228).
//
// A validation failure re-renders the detail at 422 with the submitted values;
// the resource must still resolve first, so a foreign or missing answer wins as
// 404 even when the input was also invalid. An already-graded answer is a 303
// to its read-only detail with the closed `already_graded` flash.
func gradingAct(x *Ctx) (viewmodel.PageData, Redirect, error) {
	accountID, err := teacherActor(x)
	if err != nil {
		return viewmodel.PageData{}, Redirect{}, err
	}
	answerID := x.IntParam("answerID")
	in := attempts.GradeInput{
		AccountID:   accountID,
		AnswerID:    answerID,
		ScoreRaw:    x.RawValue("score"),
		FeedbackRaw: x.RawValue("feedback"),
	}
	err = attemptService(x).GradeEssay(x.Context(), in)
	if err == nil {
		return viewmodel.PageData{}, x.Redirect("/teacher/grading", "graded"), nil
	}
	if flash, ok := attempts.ConflictKey(err); ok {
		return viewmodel.PageData{}, x.Redirect(gradingAnswerHref(answerID), flash), nil
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind() != apperr.Validation {
		return viewmodel.PageData{}, Redirect{}, attemptTeacherError(err)
	}
	page, loadErr := gradingDetail(x, accountID, answerID)
	if loadErr != nil {
		return viewmodel.PageData{}, Redirect{}, loadErr
	}
	page.Selected = gradingDraft(page.Selected, in)
	return x.Page("Penilaian esai", "penilaian", page), Redirect{}, err
}

// recapLoad serves GET /teacher/recap/:attemptID (ATT-231…ATT-236).
func recapLoad(x *Ctx) (viewmodel.PageData, error) {
	accountID, err := teacherActor(x)
	if err != nil {
		return viewmodel.PageData{}, err
	}
	recap, err := attemptService(x).Recap(x.Context(), attempts.RecapInput{
		AccountID: accountID,
		AttemptID: x.IntParam("attemptID"),
	})
	if err != nil {
		return viewmodel.PageData{}, attemptTeacherError(err)
	}
	return x.Page("Rekap percobaan — "+recap.Attempt.StudentName, "penilaian", recap), nil
}

func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/grading",
		Template: "pages/teacher/grading.html",
		Nav:      "penilaian",
		Auth:     ClassOwnerTeacher,
		Load:     gradingQueueLoad,
		Build: func(req PageRequest) PageData {
			return page(req, "Penilaian esai", "penilaian", fixtures.TeacherUser(), fixtures.GradingPage(0, req.Query))
		},
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/grading/:answerID",
		Template: "pages/teacher/grading.html",
		Nav:      "penilaian",
		Auth:     ClassOwnerTeacher,
		Load:     gradingDetailLoad,
		Build: func(req PageRequest) PageData {
			return page(req, "Penilaian esai", "penilaian", fixtures.TeacherUser(), fixtures.GradingPage(req.IntParam("answerID"), req.Query))
		},
	})
	registerPage(PageRoute{
		Method:   "POST",
		Path:     "/teacher/grading/:answerID",
		Template: "pages/teacher/grading.html",
		Nav:      "penilaian",
		Auth:     ClassOwnerTeacher,
		ActForm:  gradingAct,
		Action:   action("/teacher/grading", "graded"),
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/recap/:attemptID",
		Template: "pages/teacher/recap.html",
		Nav:      "penilaian",
		Auth:     ClassOwnerTeacher,
		Load:     recapLoad,
		Build: func(req PageRequest) PageData {
			recap := fixtures.RecapPage(req.IntParam("attemptID"))
			return page(req, "Rekap percobaan — "+recap.Attempt.StudentName, "penilaian", fixtures.TeacherUser(), recap)
		},
	})
}
