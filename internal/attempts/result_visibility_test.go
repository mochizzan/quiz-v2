package attempts

// result_visibility_test.go pins ATT-152/R-18: the review mode alone decides
// which fields of a result row exist. The missing test here is what let the
// "show questions only" mode render a right/wrong badge in production, so each
// mode asserts the four disclosures directly — StudentAnswer, Key, Verdict,
// and essay Feedback — instead of only checking that the page rendered.

import (
	"database/sql"
	"testing"

	"quiz-v2/internal/viewmodel"
)

// matrixWorld is three slots in one attempt: a correctly answered objective
// question, an incorrectly answered one, and a graded essay. Every mode sees
// the same stored record; only the rendering differs.
func matrixWorld() ([]takeSlot, map[int64][]optionRow, map[int64][]int64) {
	slots := []takeSlot{
		{
			questionID: 1, typ: viewmodel.QuestionSingle, prompt: "Soal benar",
			score:      sql.NullFloat64{Float64: 1, Valid: true},
			selections: []int64{11},
		},
		{
			questionID: 2, typ: viewmodel.QuestionSingle, prompt: "Soal salah",
			score:      sql.NullFloat64{Float64: 0, Valid: true},
			selections: []int64{22},
		},
		{
			questionID: 3, typ: viewmodel.QuestionEssay, prompt: "Soal esai",
			answerText: sql.NullString{String: "Jawaban esai", Valid: true},
			score:      sql.NullFloat64{Float64: 0.86, Valid: true},
			feedback:   sql.NullString{String: "Bagus sekali", Valid: true},
		},
	}
	opts := map[int64][]optionRow{
		1: {
			{id: 11, position: 1, body: "A", isCorrect: true},
			{id: 12, position: 2, body: "B"},
		},
		2: {
			{id: 21, position: 1, body: "C", isCorrect: true},
			{id: 22, position: 2, body: "D"},
		},
		3: {},
	}
	sels := map[int64][]int64{1: {11}, 2: {22}, 3: nil}
	return slots, opts, sels
}

// TestResultVisibilityMatrix is the one-case-per-mode matrix: for each mode,
// the student answer, key, verdict, and essay feedback are either populated
// exactly as the mode promises or blank.
func TestResultVisibilityMatrix(t *testing.T) {
	slots, opts, sels := matrixWorld()

	cases := []struct {
		mode          viewmodel.ReviewMode
		wantRows      int
		studentAnswer string
		key           string
		verdict       string
		essayFeedback string
	}{
		{
			mode: viewmodel.ReviewQuestionsNoVerdict, wantRows: 3,
			studentAnswer: "A", key: "", verdict: "", essayFeedback: "",
		},
		{
			mode: viewmodel.ReviewQuestionsNoAnswers, wantRows: 3,
			studentAnswer: "", key: "", verdict: "", essayFeedback: "",
		},
		{
			mode: viewmodel.ReviewCorrectOnly, wantRows: 1,
			studentAnswer: "A", key: "", verdict: "", essayFeedback: "",
		},
		{
			mode: viewmodel.ReviewQuestionsWithKey, wantRows: 3,
			studentAnswer: "A", key: "A", verdict: "correct", essayFeedback: "Bagus sekali",
		},
	}

	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			rows := buildResultRows(slots, opts, sels, 0, false, tc.mode, true)
			if len(rows) != tc.wantRows {
				t.Fatalf("rows = %d, want %d", len(rows), tc.wantRows)
			}

			first := rows[0]
			if first.Position != 1 || first.Body != "Soal benar" {
				t.Errorf("first row identity = position %d body %q", first.Position, first.Body)
			}
			if first.StudentAnswer != tc.studentAnswer {
				t.Errorf("StudentAnswer = %q, want %q", first.StudentAnswer, tc.studentAnswer)
			}
			if first.Key != tc.key {
				t.Errorf("Key = %q, want %q", first.Key, tc.key)
			}
			if first.Verdict != tc.verdict {
				t.Errorf("Verdict = %q, want %q", first.Verdict, tc.verdict)
			}

			// The essay is the last slot; correct_only drops it entirely.
			var essay *viewmodel.ResultRow
			for i := range rows {
				if rows[i].Essay {
					essay = &rows[i]
					break
				}
			}
			if tc.mode == viewmodel.ReviewCorrectOnly {
				if essay != nil {
					t.Fatalf("correct_only kept an essay row: %+v", *essay)
				}
				return
			}
			if essay == nil {
				t.Fatal("mode dropped the essay row")
			}
			if essay.Feedback != tc.essayFeedback {
				t.Errorf("essay Feedback = %q, want %q", essay.Feedback, tc.essayFeedback)
			}
			if essay.Position != 3 {
				t.Errorf("essay Position = %d, want the original 3", essay.Position)
			}
			wantEssayAnswer := ""
			if tc.mode.ShowsStudentAnswer() {
				wantEssayAnswer = "Jawaban esai"
			}
			if essay.StudentAnswer != wantEssayAnswer {
				t.Errorf("essay StudentAnswer = %q, want %q", essay.StudentAnswer, wantEssayAnswer)
			}
			if tc.essayFeedback == "" {
				if essay.Graded || essay.Score != nil {
					t.Errorf("essay grade leaked in %s: graded=%v score=%v", tc.mode, essay.Graded, essay.Score)
				}
				return
			}
			if !essay.Graded || essay.Score == nil || essay.Score.Int() != 86 {
				t.Errorf("graded essay = graded %v score %v, want 86", essay.Graded, essay.Score)
			}
		})
	}
}

// TestReviewCorrectOnlyFiltering pins the substitution rule: only correct rows
// survive, positions are not renumbered, essays are absent, and a zero-correct
// attempt yields an empty row set (the template renders its calm message).
func TestReviewCorrectOnlyFiltering(t *testing.T) {
	opts := map[int64][]optionRow{
		1: {{id: 11, position: 1, body: "A", isCorrect: true}},
		2: {{id: 21, position: 1, body: "C", isCorrect: true}},
		3: {{id: 31, position: 1, body: "E", isCorrect: true}},
		4: {{id: 41, position: 1, body: "G", isCorrect: true}},
	}
	slot := func(questionID int64, optionID int64, score float64) takeSlot {
		return takeSlot{
			questionID: questionID, typ: viewmodel.QuestionSingle, prompt: "Soal",
			score:      sql.NullFloat64{Float64: score, Valid: true},
			selections: []int64{optionID},
		}
	}
	sels := map[int64][]int64{1: {11}, 2: {21}, 3: {31}, 4: {41}}

	t.Run("positions are preserved", func(t *testing.T) {
		slots := []takeSlot{
			slot(1, 11, 1), // correct, position 1
			slot(2, 21, 0), // wrong, position 2
			slot(3, 31, 1), // correct, position 3
			slot(4, 41, 0), // wrong, position 4
		}
		rows := buildResultRows(slots, opts, sels, 0, false, viewmodel.ReviewCorrectOnly, true)
		if len(rows) != 2 {
			t.Fatalf("rows = %d, want the two correct rows", len(rows))
		}
		if rows[0].Position != 1 || rows[1].Position != 3 {
			t.Fatalf("positions = %d, %d; want 1, 3 (never renumbered)", rows[0].Position, rows[1].Position)
		}
		for _, row := range rows {
			if row.Verdict != "" || row.Key != "" {
				t.Errorf("correct_only row carries a verdict/key: %+v", row)
			}
			if row.StudentAnswer == "" {
				t.Error("correct_only row lost the student's answer")
			}
		}
	})

	t.Run("zero correct yields no rows", func(t *testing.T) {
		slots := []takeSlot{slot(1, 11, 0), slot(2, 21, 0)}
		if rows := buildResultRows(slots, opts, sels, 0, false, viewmodel.ReviewCorrectOnly, true); len(rows) != 0 {
			t.Fatalf("rows = %d, want an empty set", len(rows))
		}
	})

	t.Run("essays never survive", func(t *testing.T) {
		slots := []takeSlot{{
			questionID: 3, typ: viewmodel.QuestionEssay, prompt: "Esai",
			answerText: sql.NullString{String: "Jawaban", Valid: true},
			score:      sql.NullFloat64{Float64: 1, Valid: true},
		}}
		if rows := buildResultRows(slots, opts, sels, 0, false, viewmodel.ReviewCorrectOnly, true); len(rows) != 0 {
			t.Fatalf("rows = %d, want the essay dropped", len(rows))
		}
	})
}

// TestReviewModeVerdictPredicate is the reported defect itself: the verdict is
// a full-record capability only.
func TestReviewModeVerdictPredicate(t *testing.T) {
	for _, mode := range viewmodel.AllReviewModes {
		if got, want := mode.ShowsVerdict(), mode == viewmodel.ReviewQuestionsWithKey; got != want {
			t.Errorf("%s.ShowsVerdict() = %v, want %v", mode, got, want)
		}
		if got, want := mode.ShowsKey(), mode == viewmodel.ReviewQuestionsWithKey; got != want {
			t.Errorf("%s.ShowsKey() = %v, want %v", mode, got, want)
		}
	}
	if viewmodel.ReviewQuestionsNoVerdict.ShowsVerdict() {
		t.Fatal("questions_no_verdict still shows the verdict: the original leak")
	}
	if viewmodel.ReviewQuestionsNoAnswers.ShowsStudentAnswer() {
		t.Fatal("questions_no_answers shows the student's answer")
	}
	if viewmodel.ReviewQuestionsWithKey.OnlyCorrect() || !viewmodel.ReviewCorrectOnly.OnlyCorrect() {
		t.Fatal("OnlyCorrect does not select correct_only alone")
	}
}
