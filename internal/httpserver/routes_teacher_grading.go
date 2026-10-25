package httpserver

import (
	"errors"
	"net/url"
	"strconv"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/attempts"
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

// gradingQueueInput reads the shared filter set (grading-history spec §2).
// Invalid values fall back to the defaults inside the domain; a query never
// 422s.
func gradingQueueInput(x *Ctx, accountID int64) attempts.GradingQueueInput {
	return attempts.GradingQueueInput{
		AccountID: accountID,
		StateRaw:  x.QueryValue("state"),
		QuizRaw:   x.QueryValue("quiz"),
		ClassRaw:  x.QueryValue("class"),
		QueryRaw:  x.QueryValue("q"),
		PageRaw:   x.QueryValue("page"),
	}
}

// gradingQuery preserves the active tab and filters across a redirect and the
// detail action, so an edit returns the teacher to the screen they left.
func gradingQuery(x *Ctx) string {
	values := url.Values{}
	if x.QueryValue("state") == "graded" {
		values.Set("state", "graded")
	}
	for _, key := range []string{"q", "class", "quiz"} {
		if v := x.QueryValue(key); v != "" {
			values.Set(key, v)
		}
	}
	return values.Encode()
}

// gradingWithQuery appends the preserved query to a grading path.
func gradingWithQuery(path string, x *Ctx) string {
	if enc := gradingQuery(x); enc != "" {
		return path + "?" + enc
	}
	return path
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
// stored values stay everywhere else; an editable graded answer keeps its
// stored dates and version. The raw score string is echoed so a one-decimal
// value is not rounded on the way back.
func gradingDraft(selected *viewmodel.GradingDetail, in attempts.GradeInput) *viewmodel.GradingDetail {
	if selected == nil {
		return selected
	}
	selected.ScoreInput = in.ScoreRaw
	selected.Feedback = in.FeedbackRaw
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

// gradingAct serves POST /teacher/grading/:answerID (ATT-221…ATT-228 and the
// re-grade rules of the grading-history spec §4).
//
// A validation failure re-renders the detail at 422 with the submitted values;
// the resource must still resolve first, so a foreign or missing answer wins as
// 404 even when the input was also invalid. A stale form (or a lost first-grade
// race) is a 303 back to the detail with the closed `grade_conflict` flash.
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
		VersionRaw:  x.RawValue("version"),
	}
	err = attemptService(x).GradeEssay(x.Context(), in)
	if err == nil {
		return viewmodel.PageData{}, x.Redirect(gradingWithQuery("/teacher/grading", x), "graded"), nil
	}
	if flash, ok := attempts.ConflictKey(err); ok {
		return viewmodel.PageData{}, x.Redirect(gradingWithQuery(gradingAnswerHref(answerID), x), flash), nil
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
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/grading/:answerID",
		Template: "pages/teacher/grading.html",
		Nav:      "penilaian",
		Auth:     ClassOwnerTeacher,
		Load:     gradingDetailLoad,
	})
	registerPage(PageRoute{
		Method:   "POST",
		Path:     "/teacher/grading/:answerID",
		Template: "pages/teacher/grading.html",
		Nav:      "penilaian",
		Auth:     ClassOwnerTeacher,
		ActForm:  gradingAct,
	})
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/recap/:attemptID",
		Template: "pages/teacher/recap.html",
		Nav:      "penilaian",
		Auth:     ClassOwnerTeacher,
		Load:     recapLoad,
	})
}
