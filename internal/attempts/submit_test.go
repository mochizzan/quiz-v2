package attempts

// submit_test.go covers ATT-141/ATT-144 at the service boundary. Submit is a
// write command: a successful call must commit its stamp and report success as
// a nil error. The 303 target belongs to the adapter (routes_attempt.go), which
// already builds the success redirect itself. Returning a redirect signal from
// the transaction body instead makes store.Tx treat the commit as a failure and
// roll it back — the attempt stays unsubmitted, GET /attempt/:id/result answers
// ATT-151's not_submitted bounce, and the student lands back on the first
// question with every answer still saved. That is the "submit does nothing"
// defect this file pins, read from the pool so a rollback cannot hide.

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"quiz-v2/internal/store/storetest"
)

// TestSubmitCommitsTheAttempt pins the whole submit contract against the real
// schema: the stamp and the final score persist, and the result page renders.
func TestSubmitCommitsTheAttempt(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedAnswerWorld(t, db)
	svc := New(db)
	ctx := context.Background()

	// One correct answer, saved through the ordinary command (ATT-121).
	if _, err := svc.SaveAnswer(ctx, AnswerInput{
		AttemptID:   w.attemptID,
		QuestionRaw: strconv.FormatInt(w.questionID, 10),
		NavRaw:      "save",
		Options:     []string{strconv.FormatInt(w.optionA, 10)},
		HasOptions:  true,
		Owner:       w.owner,
	}); err != nil {
		t.Fatalf("SaveAnswer: %v", err)
	}

	if err := svc.Submit(ctx, SubmitInput{AttemptID: w.attemptID, Owner: w.owner}); err != nil {
		if sig, ok := AsRedirect(err); ok {
			t.Fatalf("Submit returned the redirect signal %q instead of nil: the adapter builds that redirect from a nil return (ATT-141), and store.Tx rolls back on any error", sig.Path)
		}
		t.Fatalf("Submit: %v", err)
	}

	// Read outside the command's transaction: this is what the next request
	// sees, so it proves the commit rather than the in-transaction state.
	var submitted sql.NullTime
	var final sql.NullFloat64
	if err := db.QueryRowContext(ctx, `
		SELECT submitted_at, final_score
		FROM quiz_attempts
		WHERE id = ?`,
		w.attemptID,
	).Scan(&submitted, &final); err != nil {
		t.Fatalf("read attempt: %v", err)
	}
	if !submitted.Valid {
		t.Fatal("submitted_at is NULL after a successful Submit: the transaction rolled back (ATT-144 step 7)")
	}
	if !final.Valid || final.Float64 != 1 {
		t.Fatalf("final_score = %+v, want 1 (the single correct answer, ATT-143)", final)
	}

	// ATT-151: a submitted attempt's result renders; an unsubmitted one is the
	// redirect that sends the student back to the question form.
	if _, err := svc.ResultPage(ctx, ResultInput{AttemptID: w.attemptID, Owner: w.owner}); err != nil {
		if sig, ok := AsRedirect(err); ok {
			t.Fatalf("ResultPage redirected to %q: the attempt is still unsubmitted", sig.Path)
		}
		t.Fatalf("ResultPage: %v", err)
	}
}

// TestSubmitIsIdempotent pins ATT-147: a second submit writes nothing, keeps
// the original stamp, and answers the result redirect the adapter maps.
func TestSubmitIsIdempotent(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedAnswerWorld(t, db)
	svc := New(db)
	ctx := context.Background()

	if err := svc.Submit(ctx, SubmitInput{AttemptID: w.attemptID, Owner: w.owner}); err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	var first sql.NullTime
	if err := db.QueryRowContext(ctx,
		`SELECT submitted_at FROM quiz_attempts WHERE id = ?`, w.attemptID,
	).Scan(&first); err != nil {
		t.Fatalf("read stamp: %v", err)
	}

	err := svc.Submit(ctx, SubmitInput{AttemptID: w.attemptID, Owner: w.owner})
	sig, ok := AsRedirect(err)
	if !ok {
		t.Fatalf("second Submit = %v, want the result redirect signal", err)
	}
	if want := attemptResultPath(w.attemptID); sig.Path != want {
		t.Fatalf("second Submit path = %q, want %q", sig.Path, want)
	}
	if sig.Flash != "" {
		t.Fatalf("second Submit flash = %q, want none (ATT-147)", sig.Flash)
	}

	var second sql.NullTime
	if err := db.QueryRowContext(ctx,
		`SELECT submitted_at FROM quiz_attempts WHERE id = ?`, w.attemptID,
	).Scan(&second); err != nil {
		t.Fatalf("re-read stamp: %v", err)
	}
	if !first.Valid || !second.Valid || !first.Time.Equal(second.Time) {
		t.Fatalf("submitted_at moved on the repeat: %v -> %v", first, second)
	}
}
