package attempts

// teacher_fixture_test.go builds the database world the teacher-facing tests
// need: accounts, a major, quizzes, questions/options, registrations,
// attempts, answers, selections, and focus events. It uses the real MariaDB
// schema through storetest, because ordering, CHECK constraints, and
// transaction semantics are part of the rules under test.

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

// teacherFixture is one reset database plus the two teachers every ownership
// test needs.
type teacherFixture struct {
	t              *testing.T
	db             *store.DB
	svc            *Service
	ctx            context.Context
	teacherID      int64
	otherTeacherID int64
	majorID        int64
	seq            int
}

func newTeacherFixture(t *testing.T) *teacherFixture {
	t.Helper()
	db := storetest.DB(t)
	storetest.Reset(t, db)
	f := &teacherFixture{
		t:   t,
		db:  db,
		svc: New(db),
		ctx: context.Background(),
	}
	f.teacherID = insertAccount(t, db, "Guru Utama", "guru@teacher.test", viewmodel.RoleTeacher)
	f.otherTeacherID = insertAccount(t, db, "Guru Lain", "lain@teacher.test", viewmodel.RoleTeacher)
	f.majorID = insertID(t, db,
		`INSERT INTO majors (name, code) VALUES ('RPL', 'RPL')`)
	return f
}

// newQuiz inserts a public quiz. The state decides the lifecycle columns the
// schema's CHECK constraint requires: an open/closed quiz has published_at and
// a closed one also has closed_at.
func (f *teacherFixture) newQuiz(owner int64, title, state string, showScore, allowRetake bool, maxAttempts int, createdAt string) int64 {
	f.t.Helper()
	f.seq++
	code := fmt.Sprintf("%06d", 100000+f.seq)
	token := fmt.Sprintf("invite-%06d", f.seq)
	var published, closed any
	if state != string(viewmodel.QuizDraft) {
		published = createdAt
	}
	if state == string(viewmodel.QuizClosed) {
		closed = createdAt
	}
	show, allow := 0, 0
	if showScore {
		show = 1
	}
	if allowRetake {
		allow = 1
	}
	return insertID(f.t, f.db, `
		INSERT INTO quizzes (owner_account_id, title, visibility, state, code, invite_token,
		                     published_at, closed_at, show_score, allow_retake, max_attempts,
		                     review_mode, created_at)
		VALUES (?, ?, 'public', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		owner, title, state, code, token, published, closed, show, allow, maxAttempts,
		string(viewmodel.ReviewQuestionsAndKey), createdAt)
}

// newStudent inserts an active student account and its profile row.
func (f *teacherFixture) newStudent(name string, grade int) int64 {
	f.t.Helper()
	f.seq++
	accountID := insertAccount(f.t, f.db, name,
		fmt.Sprintf("student-%03d@teacher.test", f.seq), viewmodel.RoleStudent)
	return insertID(f.t, f.db,
		`INSERT INTO students (account_id, grade_level, major_id) VALUES (?, ?, ?)`,
		accountID, grade, f.majorID)
}

// newAnonymous inserts an anonymous participant with a deterministic token
// hash. The participant is only distinguishable from a student by the
// anonymous flag in the monitor payload.
func (f *teacherFixture) newAnonymous(name string, grade int) int64 {
	f.t.Helper()
	f.seq++
	hash := sha256.Sum256([]byte(fmt.Sprintf("anon-%03d", f.seq)))
	return insertID(f.t, f.db, `
		INSERT INTO anonymous_participants (display_name, grade_level, major_id, cookie_token_hash)
		VALUES (?, ?, ?, ?)`,
		name, grade, f.majorID, hash[:])
}

func (f *teacherFixture) register(quizID, studentID int64, createdAt string) int64 {
	f.t.Helper()
	return insertID(f.t, f.db, `
		INSERT INTO registrations (quiz_id, student_id, created_at) VALUES (?, ?, ?)`,
		quizID, studentID, createdAt)
}

func (f *teacherFixture) registerAnonymous(quizID, anonymousID int64, createdAt string) int64 {
	f.t.Helper()
	return insertID(f.t, f.db, `
		INSERT INTO registrations (quiz_id, anonymous_participant_id, created_at) VALUES (?, ?, ?)`,
		quizID, anonymousID, createdAt)
}

func (f *teacherFixture) question(quizID int64, position int, typ, prompt string) int64 {
	f.t.Helper()
	return insertID(f.t, f.db, `
		INSERT INTO questions (quiz_id, position, type, prompt) VALUES (?, ?, ?, ?)`,
		quizID, position, typ, prompt)
}

func (f *teacherFixture) option(questionID int64, position int, body string, correct bool) int64 {
	f.t.Helper()
	isCorrect := 0
	if correct {
		isCorrect = 1
	}
	return insertID(f.t, f.db, `
		INSERT INTO answer_options (question_id, position, body, is_correct) VALUES (?, ?, ?, ?)`,
		questionID, position, body, isCorrect)
}

// attempt inserts one attempt; submittedAt/final are nil for an in-progress
// attempt. created_at and updated_at are explicit because the monitor's
// last_seen and the recap's start line read them.
func (f *teacherFixture) attempt(regID int64, number int, seed uint32,
	submittedAt, final any, createdAt, updatedAt string) int64 {
	f.t.Helper()
	return insertID(f.t, f.db, `
		INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed,
		                           submitted_at, final_score, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		regID, number, seed, submittedAt, final, createdAt, updatedAt)
}

// answer inserts one slot. gradedBy/gradedAt are NULL for an ungraded slot;
// an objective slot must carry a graded_at whenever its score is non-null
// (chk_answers_grading).
func (f *teacherFixture) answer(attemptID, questionID int64,
	text, score, feedback, gradedBy, gradedAt any) int64 {
	f.t.Helper()
	return insertID(f.t, f.db, `
		INSERT INTO attempt_answers (attempt_id, question_id, answer_text, score,
		                             feedback, graded_by_account_id, graded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		attemptID, questionID, text, score, feedback, gradedBy, gradedAt)
}

func (f *teacherFixture) selectOption(answerID, optionID int64) {
	f.t.Helper()
	insertID(f.t, f.db, `
		INSERT INTO attempt_answer_selections (attempt_answer_id, option_id) VALUES (?, ?)`,
		answerID, optionID)
}

func (f *teacherFixture) focus(attemptID int64, kind, occurredAt string) {
	f.t.Helper()
	insertID(f.t, f.db, `
		INSERT INTO focus_loss_events (attempt_id, kind, occurred_at) VALUES (?, ?, ?)`,
		attemptID, kind, occurredAt)
}

// setQuizState closes or reopens a quiz with the lifecycle columns the schema
// requires.
func (f *teacherFixture) setQuizState(quizID int64, state string) {
	f.t.Helper()
	if state == string(viewmodel.QuizClosed) {
		mustExec(f.t, f.db,
			`UPDATE quizzes SET state = 'closed', closed_at = UTC_TIMESTAMP() WHERE id = ?`, quizID)
		return
	}
	mustExec(f.t, f.db, `UPDATE quizzes SET state = ?, closed_at = NULL WHERE id = ?`, state, quizID)
}
