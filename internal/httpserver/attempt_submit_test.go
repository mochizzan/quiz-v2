package httpserver

// attempt_submit_test.go pins the transport pair a student actually walks:
// POST /attempt/:id/submit answers the ATT-141 result redirect with the
// success flash, and the GET that follows renders the result instead of
// ATT-151's not_submitted bounce back to the question form. The domain commit
// is covered by attempts.TestSubmitCommitsTheAttempt; this file covers the
// adapter, whose success path was unreachable while Submit returned its
// redirect as an error.

import (
	"strconv"
	"testing"

	"quiz-v2/internal/attempts"
	"quiz-v2/internal/authn"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

// submitActCtx builds the request-scoped values the attempt submit/result
// adapters read.
func submitActCtx(db *store.DB, attemptID, accountID int64) *Ctx {
	return &Ctx{
		DB:      db,
		Params:  map[string]string{"id": strconv.FormatInt(attemptID, 10)},
		Session: &authn.Session{Role: viewmodel.RoleStudent, AccountID: accountID},
	}
}

// TestAttemptSubmitActRedirectsToAResultThatRenders is the regression for the
// "submit bounces back to the first question" defect: the 303 and the page it
// points at must agree about whether the attempt is submitted.
func TestAttemptSubmitActRedirectsToAResultThatRenders(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	attemptID, questionID, optionID, accountID := seedAnswerActWorld(t, db)

	// Answer the single question so the submit has something to score (ATT-142).
	if _, _, err := attemptAnswerAct(
		answerActCtx(db, attemptID, questionID, optionID, accountID, "save"),
	); err != nil {
		t.Fatalf("attemptAnswerAct: %v", err)
	}

	redirect, err := attemptSubmitAct(submitActCtx(db, attemptID, accountID))
	if err != nil {
		if sig, ok := attempts.AsRedirect(err); ok {
			t.Fatalf("attemptSubmitAct returned the redirect as an error (path %q): the adapter's success path never runs", sig.Path)
		}
		t.Fatalf("attemptSubmitAct: %v", err)
	}
	want := "/attempt/" + strconv.FormatInt(attemptID, 10) + "/result"
	if redirect.Path != want || redirect.Flash != attempts.FlashSubmitted {
		t.Fatalf("redirect = %q/%q, want %q/%q",
			redirect.Path, redirect.Flash, want, attempts.FlashSubmitted)
	}

	// The browser follows the 303: this must render, not bounce.
	data, err := attemptResultLoad(submitActCtx(db, attemptID, accountID))
	if err != nil {
		if sig, ok := attempts.AsRedirect(err); ok {
			t.Fatalf("GET result redirected to %q: the submission did not commit", sig.Path)
		}
		t.Fatalf("attemptResultLoad: %v", err)
	}
	res, ok := data.Data.(viewmodel.AttemptResult)
	if !ok {
		t.Fatalf("result data = %T, want viewmodel.AttemptResult", data.Data)
	}
	if res.Waiting || !res.Graded {
		t.Fatalf("result waiting/graded = %v/%v, want false/true for an all-objective attempt",
			res.Waiting, res.Graded)
	}
}
