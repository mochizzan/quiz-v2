package attempts

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/viewmodel"
)

// TestRecapSubmitted pins ATT-231…ATT-236 on a submitted attempt: the header
// truth (score, class, start, submit, focus), every row in derived question
// order with the derived option order, keys shown regardless of review_mode and
// show_score, essay feedback, and the derived 100/0 objective scores.
func TestRecapSubmitted(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Rekap Lengkap", "open", false, true, 3, "2026-10-20 09:00:00")
	// The owner sees the truth even for a mode that hides the key from students.
	mustExec(t, f.db, `
		UPDATE quizzes
		SET review_mode = 'questions_no_answers', randomize_questions = 1, randomize_options = 1
		WHERE id = ?`, quiz)

	const seed = uint32(4242)
	q1 := f.question(quiz, 1, "single", "Pilih satu")
	q2 := f.question(quiz, 2, "multiple", "Pilih banyak")
	q3 := f.question(quiz, 3, "essay", "Jelaskan")
	f.option(q1, 1, "A", true)
	b := f.option(q1, 2, "B", false)
	c := f.option(q2, 1, "C", true)
	d := f.option(q2, 2, "D", true)
	e := f.option(q2, 3, "E", false)

	ayu := f.newStudent("Ayu Lestari", 10)
	reg := f.register(quiz, ayu, "2026-10-23 08:00:00")
	attempt := f.attempt(reg, 2, seed, "2026-10-23 10:00:00", 0.6183,
		"2026-10-23 09:00:00", "2026-10-23 10:00:00")
	slot1 := f.answer(attempt, q1, nil, 0.0, nil, nil, "2026-10-23 10:00:00")
	f.selectOption(slot1, b)
	slot2 := f.answer(attempt, q2, nil, 0.0, nil, nil, "2026-10-23 10:00:00")
	f.selectOption(slot2, c)
	f.selectOption(slot2, e)
	f.answer(attempt, q3, "Jawaban esai", 0.8550, "Bagus sekali",
		f.teacherID, "2026-10-23 11:00:00")
	f.focus(attempt, "blur", "2026-10-23 09:10:00")
	f.focus(attempt, "hidden", "2026-10-23 09:20:00")

	recap, err := f.svc.Recap(f.ctx, RecapInput{AccountID: f.teacherID, AttemptID: attempt})
	if err != nil {
		t.Fatalf("Recap: %v", err)
	}

	header := recap.Attempt
	if header.ID != attempt || header.StudentName != "Ayu Lestari" ||
		header.ClassLabel != "X RPL" || header.QuizTitle != "Rekap Lengkap" ||
		header.QuizID != quiz || header.AttemptNo != 2 || header.MaxAttempts != 3 {
		t.Errorf("header identity = %+v", header)
	}
	if header.StartedAt == nil ||
		!header.StartedAt.Time().Equal(time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("StartedAt = %v, want 2026-10-23 09:00Z", header.StartedAt)
	}
	if header.SubmittedAt == nil ||
		!header.SubmittedAt.Time().Equal(time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("SubmittedAt = %v, want 2026-10-23 10:00Z", header.SubmittedAt)
	}
	if header.FocusLoss != 2 {
		t.Errorf("FocusLoss = %d, want 2", header.FocusLoss)
	}
	if recap.Waiting || header.Waiting {
		t.Error("submitted attempt with final_score marked waiting")
	}
	if recap.Score == nil || recap.Score.Int() != 62 || header.Score == nil || header.Score.Int() != 62 {
		t.Errorf("score = %v / %v, want 62%% in both header and recap", recap.Score, header.Score)
	}
	if recap.BackHref != resultsPath(quiz) {
		t.Errorf("BackHref = %q, want %q", recap.BackHref, resultsPath(quiz))
	}

	// Derived question order: compute the expected sequence from the same key
	// rule without asking the recap.
	wantQuestions := []int64{q1, q2, q3}
	sort.Slice(wantQuestions, func(i, j int) bool {
		ki := KeyOrder(seed, namespaceQuestion, wantQuestions[i])
		kj := KeyOrder(seed, namespaceQuestion, wantQuestions[j])
		return lessOrder(ki, wantQuestions[i], kj, wantQuestions[j])
	})
	if len(recap.Answers) != 3 {
		t.Fatalf("Answers = %d rows, want 3", len(recap.Answers))
	}
	byQuestion := map[int64]viewmodel.RecapRow{}
	for i, row := range recap.Answers {
		if row.Position != i+1 {
			t.Errorf("row %d Position = %d, want %d", i, row.Position, i+1)
		}
		byQuestion[wantQuestions[i]] = row
	}
	if byQuestion[q1].Body != "Pilih satu" || byQuestion[q1].Type != viewmodel.QuestionSingle ||
		byQuestion[q1].Essay {
		t.Errorf("q1 row = %+v", byQuestion[q1])
	}
	if byQuestion[q2].Body != "Pilih banyak" || byQuestion[q2].Type != viewmodel.QuestionMultiple {
		t.Errorf("q2 row = %+v", byQuestion[q2])
	}
	if byQuestion[q3].Body != "Jelaskan" || byQuestion[q3].Type != viewmodel.QuestionEssay ||
		!byQuestion[q3].Essay {
		t.Errorf("q3 row = %+v", byQuestion[q3])
	}

	// For q1 the only selection is B, so only the order of multiple selections
	// and the option keys can vary with the seed.
	if got := byQuestion[q1]; got.StudentAnswer != "B" || got.Key != "A" ||
		got.Score == nil || got.Score.Int() != 0 || got.Verdict != "Salah" || !got.Graded {
		t.Errorf("q1 answer = %+v, want B / key A / 0%% / Salah", got)
	}

	answerOrder := []int64{c, e}
	sort.Slice(answerOrder, func(i, j int) bool {
		ki := KeyOrder(seed, namespaceOption, answerOrder[i])
		kj := KeyOrder(seed, namespaceOption, answerOrder[j])
		return lessOrder(ki, answerOrder[i], kj, answerOrder[j])
	})
	answerBodies := map[int64]string{c: "C", d: "D", e: "E"}
	wantAnswer := make([]string, 0, len(answerOrder))
	for _, id := range answerOrder {
		wantAnswer = append(wantAnswer, answerBodies[id])
	}
	keyOrder := []int64{c, d}
	sort.Slice(keyOrder, func(i, j int) bool {
		ki := KeyOrder(seed, namespaceOption, keyOrder[i])
		kj := KeyOrder(seed, namespaceOption, keyOrder[j])
		return lessOrder(ki, keyOrder[i], kj, keyOrder[j])
	})
	wantKey := make([]string, 0, len(keyOrder))
	for _, id := range keyOrder {
		wantKey = append(wantKey, answerBodies[id])
	}
	if got := byQuestion[q2]; got.StudentAnswer != strings.Join(wantAnswer, ", ") ||
		got.Key != strings.Join(wantKey, ", ") ||
		got.Score == nil || got.Score.Int() != 0 || got.Verdict != "Salah" || !got.Graded {
		t.Errorf("q2 answer = %+v, want %q / key %q / 0%% / Salah",
			got, strings.Join(wantAnswer, ", "), strings.Join(wantKey, ", "))
	}

	// The correct option for q1 is A even though the student picked B, and the
	// essay carries its one-decimal score rounded half-up to 86%.
	if got := byQuestion[q3]; got.StudentAnswer != "Jawaban esai" || got.Key != "—" ||
		got.Feedback != "Bagus sekali" || !got.Graded ||
		got.Score == nil || got.Score.Int() != 86 || got.Verdict != "Sudah dinilai" {
		t.Errorf("q3 answer = %+v, want the graded essay", got)
	}

	// q1's unselected correct option must still be the key (A), not the picked
	// body; q2's key lists both correct bodies even though E was picked.
	if got := byQuestion[q1]; got.Key != "A" {
		t.Errorf("q1 key = %q, want A", got.Key)
	}
}

// TestRecapInProgress pins ATT-234: the page exists before submit, shows saved
// answers and the key, and withholds scores, verdicts, and feedback.
func TestRecapInProgress(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Rekap Berjalan", "open", true, false, 1, "2026-10-20 09:00:00")
	q1 := f.question(quiz, 1, "single", "Pilih satu")
	q2 := f.question(quiz, 2, "essay", "Jelaskan")
	f.option(q1, 1, "A", true)
	b := f.option(q1, 2, "B", false)

	budi := f.newStudent("Budi Santoso", 11)
	reg := f.register(quiz, budi, "2026-10-23 08:00:00")
	attempt := f.attempt(reg, 1, 77, nil, nil,
		"2026-10-23 09:00:00", "2026-10-23 09:30:00")
	slot1 := f.answer(attempt, q1, nil, nil, nil, nil, nil)
	f.selectOption(slot1, b)
	f.answer(attempt, q2, "Draf jawaban", nil, nil, nil, nil)

	recap, err := f.svc.Recap(f.ctx, RecapInput{AccountID: f.teacherID, AttemptID: attempt})
	if err != nil {
		t.Fatalf("Recap: %v", err)
	}
	if !recap.Waiting || !recap.Attempt.Waiting {
		t.Error("in-progress recap is not waiting")
	}
	if recap.Score != nil || recap.Attempt.Score != nil {
		t.Errorf("in-progress score = %v / %v, want nil", recap.Score, recap.Attempt.Score)
	}
	if recap.Attempt.SubmittedAt != nil {
		t.Errorf("in-progress SubmittedAt = %v, want nil", recap.Attempt.SubmittedAt)
	}
	if len(recap.Answers) != 2 {
		t.Fatalf("Answers = %d, want 2", len(recap.Answers))
	}
	obj := recap.Answers[0]
	if obj.StudentAnswer != "B" || obj.Key != "A" || obj.Score != nil ||
		obj.Verdict != "—" || obj.Graded {
		t.Errorf("in-progress objective row = %+v", obj)
	}
	essay := recap.Answers[1]
	if essay.StudentAnswer != "Draf jawaban" || essay.Key != "—" ||
		essay.Score != nil || essay.Verdict != "—" || essay.Feedback != "" || essay.Graded {
		t.Errorf("in-progress essay row = %+v", essay)
	}
}

// TestRecapClosedQuizAndNotFound pins ATT-235: a closed quiz stays reachable,
// while foreign/missing/deleted resources are 404.
func TestRecapClosedQuizAndNotFound(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Rekap Ditutup", "open", true, false, 1, "2026-10-20 09:00:00")
	essay := f.question(quiz, 1, "essay", "Jelaskan")
	citra := f.newStudent("Citra Dewi", 12)
	reg := f.register(quiz, citra, "2026-10-23 08:00:00")
	attempt := f.attempt(reg, 1, 88, "2026-10-23 10:00:00", 0.8000,
		"2026-10-23 09:00:00", "2026-10-23 10:00:00")
	f.answer(attempt, essay, "Jawaban", 0.8000, nil, f.teacherID, "2026-10-23 11:00:00")
	f.focus(attempt, "fullscreen_exit", "2026-10-23 09:30:00")

	f.setQuizState(quiz, "closed")
	recap, err := f.svc.Recap(f.ctx, RecapInput{AccountID: f.teacherID, AttemptID: attempt})
	if err != nil {
		t.Fatalf("closed-quiz recap: %v", err)
	}
	if recap.Score == nil || recap.Score.Int() != 80 || len(recap.Answers) != 1 {
		t.Errorf("closed-quiz recap = %+v", recap)
	}
	if recap.Attempt.FocusLoss != 1 {
		t.Errorf("closed-quiz focus = %d, want 1", recap.Attempt.FocusLoss)
	}

	if _, err := f.svc.Recap(f.ctx, RecapInput{
		AccountID: f.otherTeacherID, AttemptID: attempt,
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign recap error = %v, want ErrNotFound", err)
	}
	if _, err := f.svc.Recap(f.ctx, RecapInput{
		AccountID: f.teacherID, AttemptID: 999999,
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing recap error = %v, want ErrNotFound", err)
	}

	// A soft-deleted quiz makes its recap a 404 for the owner too.
	f.setQuizState(quiz, "open")
	mustExec(t, f.db, `UPDATE quizzes SET deleted_at = UTC_TIMESTAMP() WHERE id = ?`, quiz)
	if _, err := f.svc.Recap(f.ctx, RecapInput{
		AccountID: f.teacherID, AttemptID: attempt,
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted-quiz recap error = %v, want ErrNotFound", err)
	}

	// A cancelled registration takes its attempts with it (PAR cascade), so the
	// recap is 404 because both rows are gone.
	mustExec(t, f.db, `UPDATE quizzes SET deleted_at = '1970-01-01 00:00:00' WHERE id = ?`, quiz)
	mustExec(t, f.db, `UPDATE quiz_attempts SET deleted_at = UTC_TIMESTAMP() WHERE id = ?`, attempt)
	mustExec(t, f.db, `UPDATE registrations SET deleted_at = UTC_TIMESTAMP() WHERE id = ?`, reg)
	if _, err := f.svc.Recap(f.ctx, RecapInput{
		AccountID: f.teacherID, AttemptID: attempt,
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cancelled-registration recap error = %v, want ErrNotFound", err)
	}
}
