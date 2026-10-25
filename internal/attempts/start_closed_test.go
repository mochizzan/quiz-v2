package attempts

// start_closed_test.go pins ATT-104 steps 4–5 together: closing a quiz never
// aborts the attempt the student is already inside, but it does refuse a new
// one. This is the domain half of spec §2 — the participation intro page is the
// navigation half and cannot preserve a live attempt on its own.

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
)

func TestStartResumesAfterQuizCloses(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedStartWorld(t, db, false)

	in := StartInput{
		RegistrationID: uint32(w.registrationID),
		QuizID:         uint32(w.quizID),
		AttemptNo:      1,
		StudentID:      uint32(w.studentID),
	}
	start := func() (StartResult, error) {
		var res StartResult
		err := db.Tx(context.Background(), func(tx *sql.Tx) error {
			var err error
			res, err = Start(context.Background(), tx, in)
			return err
		})
		return res, err
	}
	attemptCount := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM quiz_attempts WHERE registration_id = ? AND deleted_at = ?`,
			w.registrationID, store.LiveAt).Scan(&n); err != nil {
			t.Fatalf("count attempts: %v", err)
		}
		return n
	}

	first, err := start()
	if err != nil {
		t.Fatalf("Start on the open quiz: %v", err)
	}
	if !first.Created || first.AttemptID == 0 {
		t.Fatalf("result = %+v, want a created attempt", first)
	}
	updatedAt := func() sql.NullTime {
		t.Helper()
		var at sql.NullTime
		if err := db.QueryRow(
			`SELECT updated_at FROM quiz_attempts WHERE id = ?`, first.AttemptID).Scan(&at); err != nil {
			t.Fatalf("read attempt: %v", err)
		}
		return at
	}
	beforeResume := updatedAt()

	if _, err := db.Exec(
		`UPDATE quizzes SET state = 'closed', closed_at = UTC_TIMESTAMP() WHERE id = ?`,
		w.quizID); err != nil {
		t.Fatalf("close quiz: %v", err)
	}

	// Resume wins over the closed state: same attempt, no write.
	in.AttemptNo = 2
	resumed, err := start()
	if err != nil {
		t.Fatalf("Start on the closed quiz with an in-progress attempt: %v", err)
	}
	if resumed.Created || resumed.AttemptID != first.AttemptID {
		t.Fatalf("result = %+v, want attempt %d resumed", resumed, first.AttemptID)
	}
	if n := attemptCount(); n != 1 {
		t.Fatalf("attempts = %d, want 1", n)
	}
	if afterResume := updatedAt(); afterResume != beforeResume {
		t.Fatalf("resume touched updated_at: %v -> %v", beforeResume, afterResume)
	}

	// With the attempt submitted there is nothing to resume, so the closed
	// state is the answer even though the attempt quota would also refuse.
	if _, err := db.Exec(
		`UPDATE quiz_attempts SET submitted_at = UTC_TIMESTAMP(), final_score = 0.5 WHERE id = ?`,
		first.AttemptID); err != nil {
		t.Fatalf("submit attempt: %v", err)
	}
	if _, err := start(); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("error = %v, want ErrNotOpen", err)
	}
	if n := attemptCount(); n != 1 {
		t.Fatalf("attempts = %d after the refused start, want 1", n)
	}
}
