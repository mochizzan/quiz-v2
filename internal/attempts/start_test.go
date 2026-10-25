package attempts

// start_test.go covers ATT-101…ATT-110 against the real schema. The
// participation package stubs this seam with a raw insert, so this file is the
// only place the real start rule runs end to end: the registration lock, the
// attempt insert, and the per-question slots. Both registration shapes are
// exercised because registrations stores the participant in one of two
// nullable columns, and a scan that cannot tolerate NULL breaks one of them.

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

// startWorld is one open quiz with three live questions and one registration.
type startWorld struct {
	quizID         int64
	registrationID int64
	studentID      int64
	questionIDs    []int64
}

// seedStartWorld builds the fixture for one registration kind. When anonymous
// is false the registration carries student_id and a NULL
// anonymous_participant_id, and the reverse for anonymous.
func seedStartWorld(t *testing.T, db *store.DB, anonymous bool) startWorld {
	t.Helper()
	teacherID := insertAccount(t, db, "Guru", "guru@start.test", viewmodel.RoleTeacher)
	majorID := insertID(t, db, `INSERT INTO majors (name, code) VALUES ('Start Major', 'STR')`)
	studentAccountID := insertAccount(t, db, "Siswa", "siswa@start.test", viewmodel.RoleStudent)
	studentID := insertID(t, db,
		`INSERT INTO students (account_id, grade_level, major_id) VALUES (?, 10, ?)`,
		studentAccountID, majorID)

	quizID := insertID(t, db, `
		INSERT INTO quizzes (owner_account_id, title, visibility, state, code, invite_token,
			allow_retake, max_attempts, randomize_questions, show_score, published_at)
		VALUES (?, 'Kuis Mulai', 'public', 'open', '654321', 'start-invite-token', 0, 1, 1, 1, UTC_TIMESTAMP())`,
		teacherID)

	var questionIDs []int64
	for i := 1; i <= 3; i++ {
		questionIDs = append(questionIDs, insertID(t, db,
			`INSERT INTO questions (quiz_id, position, type, prompt) VALUES (?, ?, 'single', ?)`,
			quizID, i, "Soal"))
	}

	var registrationID int64
	if anonymous {
		anonID := insertID(t, db, `
			INSERT INTO anonymous_participants (display_name, grade_level, major_id, cookie_token_hash)
			VALUES ('Tamu', 10, ?, ?)`, majorID, make([]byte, 32))
		registrationID = insertID(t, db,
			`INSERT INTO registrations (quiz_id, anonymous_participant_id) VALUES (?, ?)`,
			quizID, anonID)
	} else {
		registrationID = insertID(t, db,
			`INSERT INTO registrations (quiz_id, student_id) VALUES (?, ?)`,
			quizID, studentID)
	}

	return startWorld{
		quizID:         quizID,
		registrationID: registrationID,
		studentID:      studentID,
		questionIDs:    questionIDs,
	}
}

// TestStartAcceptsBothRegistrationKinds pins ATT-104 step 2: the locked
// registration read resolves a student registration and an anonymous one, then
// a fresh attempt is created with one slot per live question.
func TestStartAcceptsBothRegistrationKinds(t *testing.T) {
	for _, tc := range []struct {
		name      string
		anonymous bool
	}{
		{name: "student registration"},
		{name: "anonymous registration", anonymous: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := storetest.DB(t)
			storetest.Reset(t, db)
			w := seedStartWorld(t, db, tc.anonymous)

			in := StartInput{
				RegistrationID: uint32(w.registrationID),
				QuizID:         uint32(w.quizID),
				AttemptNo:      1,
			}
			if tc.anonymous {
				var anonID int64
				if err := db.QueryRow(
					`SELECT anonymous_participant_id FROM registrations WHERE id = ?`,
					w.registrationID).Scan(&anonID); err != nil {
					t.Fatalf("read anonymous participant: %v", err)
				}
				in.AnonymousParticipantID = uint32(anonID)
			} else {
				in.StudentID = uint32(w.studentID)
			}

			var res StartResult
			err := db.Tx(context.Background(), func(tx *sql.Tx) error {
				var err error
				res, err = Start(context.Background(), tx, in)
				return err
			})
			if err != nil {
				// apperr hides the cause from the response, so report it here:
				// the test is the only place the raw failure is visible.
				t.Fatalf("Start: %v (cause: %v)", err, errors.Unwrap(err))
			}
			if !res.Created || res.AttemptID == 0 {
				t.Fatalf("result = %+v, want a created attempt", res)
			}

			rows, err := db.Query(
				`SELECT question_id FROM attempt_answers WHERE attempt_id = ? AND deleted_at = ?`,
				res.AttemptID, store.LiveAt)
			if err != nil {
				t.Fatalf("read slots: %v", err)
			}
			defer rows.Close()
			got := map[int64]bool{}
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					t.Fatalf("scan slot: %v", err)
				}
				got[id] = true
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("slots: %v", err)
			}
			if len(got) != len(w.questionIDs) {
				t.Fatalf("slots = %d, want %d", len(got), len(w.questionIDs))
			}
			for _, id := range w.questionIDs {
				if !got[id] {
					t.Errorf("question %d has no slot", id)
				}
			}
		})
	}
}
