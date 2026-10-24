package httpserver

// attempt_answer_test.go pins the wiring between the attempt answer command and
// the transport: the domain says "nothing to render" for a quiet autosave, and
// attemptAnswerAct must carry that through instead of inventing an empty
// redirect (ATT-125).

import (
	"net/url"
	"strconv"
	"testing"

	"quiz-v2/internal/authn"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

// seedAnswerActWorld creates one live student attempt with one single-choice
// question and returns the ids attemptAnswerAct needs plus the owning account.
func seedAnswerActWorld(t *testing.T, db *store.DB) (attemptID, questionID, optionID, accountID int64) {
	t.Helper()
	exec := func(query string, args ...any) int64 {
		t.Helper()
		res, err := db.Exec(query, args...)
		if err != nil {
			t.Fatalf("seed: %v\nquery: %s", err, query)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("seed last id: %v", err)
		}
		return id
	}

	teacherID := exec(`
		INSERT INTO accounts (full_name, email, password_hash, role, status, approved_at)
		VALUES ('Guru Jawab', 'guru@answer-act.test', 'test', 'teacher', 'active', UTC_TIMESTAMP())`)
	majorID := exec(`INSERT INTO majors (name, code) VALUES ('Jawab Major', 'JWB')`)
	accountID = exec(`
		INSERT INTO accounts (full_name, email, password_hash, role, status)
		VALUES ('Siswa Jawab', 'siswa@answer-act.test', 'test', 'student', 'active')`)
	studentID := exec(`INSERT INTO students (account_id, grade_level, major_id) VALUES (?, 10, ?)`, accountID, majorID)

	quizID := exec(`
		INSERT INTO quizzes (owner_account_id, title, visibility, state, code, invite_token, published_at)
		VALUES (?, 'Kuis Jawab', 'public', 'open', '654321', 'answer-act-invite', UTC_TIMESTAMP())`, teacherID)
	questionID = exec(`INSERT INTO questions (quiz_id, position, type, prompt) VALUES (?, 1, 'single', 'Soal satu')`, quizID)
	optionID = exec(`INSERT INTO answer_options (question_id, position, body, is_correct) VALUES (?, 1, 'A', 1)`, questionID)
	registrationID := exec(`INSERT INTO registrations (quiz_id, student_id) VALUES (?, ?)`, quizID, studentID)
	attemptID = exec(`INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed) VALUES (?, 1, 7)`, registrationID)
	exec(`INSERT INTO attempt_answers (attempt_id, question_id) VALUES (?, ?)`, attemptID, questionID)
	return attemptID, questionID, optionID, accountID
}

// answerActCtx builds the request-scoped values attemptAnswerAct reads.
func answerActCtx(db *store.DB, attemptID, questionID, optionID, accountID int64, nav string) *Ctx {
	return &Ctx{
		DB:     db,
		Params: map[string]string{"id": strconv.FormatInt(attemptID, 10)},
		Form: url.Values{
			"question_id":           {strconv.FormatInt(questionID, 10)},
			"selected_option_ids[]": {strconv.FormatInt(optionID, 10)},
			"nav":                   {nav},
		},
		Session: &authn.Session{Role: viewmodel.RoleStudent, AccountID: accountID},
	}
}

// TestAttemptAnswerActAutosaveIsNoContent is the regression the transport split
// exists for: dropping the flag would answer 303 with an empty Location.
func TestAttemptAnswerActAutosaveIsNoContent(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	attemptID, questionID, optionID, accountID := seedAnswerActWorld(t, db)

	x := answerActCtx(db, attemptID, questionID, optionID, accountID, "autosave")
	_, redirect, err := attemptAnswerAct(x)
	if err != nil {
		t.Fatalf("attemptAnswerAct(autosave): %v", err)
	}
	if !redirect.NoContent {
		t.Fatalf("redirect = %+v, want NoContent", redirect)
	}
	if redirect.Path != "" || redirect.Flash != "" {
		t.Fatalf("redirect path %q flash %q, want neither", redirect.Path, redirect.Flash)
	}
}

// TestAttemptAnswerActExplicitSaveRedirects keeps the guard on the other side:
// an explicit save still names its page and asks for the flash.
func TestAttemptAnswerActExplicitSaveRedirects(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	attemptID, questionID, optionID, accountID := seedAnswerActWorld(t, db)

	x := answerActCtx(db, attemptID, questionID, optionID, accountID, "save")
	_, redirect, err := attemptAnswerAct(x)
	if err != nil {
		t.Fatalf("attemptAnswerAct(save): %v", err)
	}
	wantPath := "/attempt/" + strconv.FormatInt(attemptID, 10) + "?q=1"
	if redirect.NoContent {
		t.Fatal("NoContent = true, want a redirect")
	}
	if redirect.Path != wantPath || redirect.Flash != "saved" {
		t.Fatalf("redirect = %q/%q, want %q/saved", redirect.Path, redirect.Flash, wantPath)
	}
}
