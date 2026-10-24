package attempts

// answer_test.go covers the ATT-125 transport difference at the domain
// boundary: a quiet autosave stores the slot and reports that nothing should be
// rendered, while an explicit save keeps its 303 target and flash. The write is
// the same transaction, so the autosave assertion also proves the selection
// landed.

import (
	"context"
	"strconv"
	"testing"

	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

// answerWorld is the smallest live attempt with one single-choice slot.
type answerWorld struct {
	attemptID  uint32
	questionID int64
	optionA    int64
	optionB    int64
	owner      OwnerRef
}

func seedAnswerWorld(t *testing.T, db *store.DB) answerWorld {
	t.Helper()
	teacherID := insertAccount(t, db, "Guru", "guru@answer.test", viewmodel.RoleTeacher)
	majorID := insertID(t, db, `INSERT INTO majors (name, code) VALUES ('Answer Major', 'ANS')`)
	studentAccountID := insertAccount(t, db, "Siswa", "siswa@answer.test", viewmodel.RoleStudent)
	studentID := insertID(t, db,
		`INSERT INTO students (account_id, grade_level, major_id) VALUES (?, 10, ?)`,
		studentAccountID, majorID)

	quizID := insertID(t, db, `
		INSERT INTO quizzes (owner_account_id, title, visibility, state, code, invite_token, published_at)
		VALUES (?, 'Kuis Jawaban', 'public', 'open', '654321', 'answer-invite-token', UTC_TIMESTAMP())`,
		teacherID)
	questionID := insertID(t, db,
		`INSERT INTO questions (quiz_id, position, type, prompt) VALUES (?, 1, 'single', 'Soal satu')`,
		quizID)
	optionA := insertID(t, db,
		`INSERT INTO answer_options (question_id, position, body, is_correct) VALUES (?, 1, 'A', 1)`,
		questionID)
	optionB := insertID(t, db,
		`INSERT INTO answer_options (question_id, position, body, is_correct) VALUES (?, 2, 'B', 0)`,
		questionID)
	regID := insertID(t, db,
		`INSERT INTO registrations (quiz_id, student_id) VALUES (?, ?)`, quizID, studentID)
	attemptID := insertID(t, db,
		`INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed) VALUES (?, 1, 7)`,
		regID)
	insertID(t, db,
		`INSERT INTO attempt_answers (attempt_id, question_id) VALUES (?, ?)`, attemptID, questionID)

	return answerWorld{
		attemptID:  uint32(attemptID),
		questionID: questionID,
		optionA:    optionA,
		optionB:    optionB,
		owner:      OwnerRef{Role: viewmodel.RoleStudent, AccountID: studentAccountID},
	}
}

// TestSaveAnswerAutosaveHasNoNavigation pins the quiet commit: the selection is
// written, but the result names no page and no flash, because the client is
// staying where it is (ATT-125).
func TestSaveAnswerAutosaveHasNoNavigation(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedAnswerWorld(t, db)
	svc := New(db)
	ctx := context.Background()

	res, err := svc.SaveAnswer(ctx, AnswerInput{
		AttemptID:   w.attemptID,
		QuestionRaw: strconv.FormatInt(w.questionID, 10),
		NavRaw:      "autosave",
		Options:     []string{strconv.FormatInt(w.optionB, 10)},
		HasOptions:  true,
		Owner:       w.owner,
	})
	if err != nil {
		t.Fatalf("SaveAnswer(autosave): %v", err)
	}
	if !res.NoContent {
		t.Fatalf("NoContent = false, want true (path %q flash %q)", res.Path, res.Flash)
	}
	if res.Path != "" || res.Flash != "" {
		t.Fatalf("autosave carries path %q flash %q, want neither", res.Path, res.Flash)
	}

	// The quiet path must still write the slot: exactly one live selection.
	rows, err := db.QueryContext(ctx, `
		SELECT s.option_id
		FROM attempt_answer_selections s
		JOIN attempt_answers a ON a.id = s.attempt_answer_id
		WHERE a.attempt_id = ? AND a.question_id = ? AND s.deleted_at = ?`,
		w.attemptID, w.questionID, store.LiveAt)
	if err != nil {
		t.Fatalf("read selections: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var got []int64
	for rows.Next() {
		var optionID int64
		if err := rows.Scan(&optionID); err != nil {
			t.Fatalf("scan selection: %v", err)
		}
		got = append(got, optionID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate selections: %v", err)
	}
	if len(got) != 1 || got[0] != w.optionB {
		t.Fatalf("selections = %v, want [%d]", got, w.optionB)
	}
}

// TestSaveAnswerExplicitNavKeepsRedirect pins the unchanged half: an explicit
// save still names its page and asks for the "saved" flash (ATT-125).
func TestSaveAnswerExplicitNavKeepsRedirect(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedAnswerWorld(t, db)
	svc := New(db)

	res, err := svc.SaveAnswer(context.Background(), AnswerInput{
		AttemptID:   w.attemptID,
		QuestionRaw: strconv.FormatInt(w.questionID, 10),
		NavRaw:      "save",
		Options:     []string{strconv.FormatInt(w.optionA, 10)},
		HasOptions:  true,
		Owner:       w.owner,
	})
	if err != nil {
		t.Fatalf("SaveAnswer(save): %v", err)
	}
	wantPath := "/attempt/" + strconv.FormatUint(uint64(w.attemptID), 10) + "?q=1"
	if res.Path != wantPath || res.Flash != "saved" {
		t.Fatalf("save = %q/%q, want %q/saved", res.Path, res.Flash, wantPath)
	}
}
