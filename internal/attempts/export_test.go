package attempts

// export_test.go pins the XLSX read model: one snapshot of every live
// registration and submitted attempt, canonical response text, and the
// owner-only 404 contract. The fixture uses the real MariaDB schema through
// storetest; without TEST_DB_DSN these tests skip.

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"quiz-v2/internal/viewmodel"
)

// exportAnswerFor finds one question's entry in an attempt's answer list.
func exportAnswerFor(t *testing.T, attempt ExportAttempt, questionID int64) ExportAnswer {
	t.Helper()
	for _, answer := range attempt.Answers {
		if answer.QuestionID == questionID {
			return answer
		}
	}
	t.Fatalf("attempt %d has no answer for question %d", attempt.ID, questionID)
	return ExportAnswer{}
}

// TestExportReadModel covers the complete snapshot: metadata, ordered live
// questions/options, every registration including the anonymous one, every
// submitted attempt including a retake, the excluded in-progress attempt, and
// the bulk answers/selections with canonical response text and score units.
func TestExportReadModel(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Ulangan Harian", "open", true, true, 3, "2026-10-20 09:00:00")

	q1 := f.question(quiz, 1, "single", "Ibu kota Indonesia?")
	bandung := f.option(q1, 1, "Bandung", false)
	jakarta := f.option(q1, 2, "Jakarta", true)
	surabaya := f.option(q1, 3, "Surabaya", false)

	q2 := f.question(quiz, 2, "multiple", "Pilih bilangan ganjil")
	satu := f.option(q2, 1, "Satu", true)
	dua := f.option(q2, 2, "Dua", false)
	tiga := f.option(q2, 3, "Tiga", true)

	q3 := f.question(quiz, 3, "essay", "Jelaskan prosesnya")

	q4 := f.question(quiz, 4, "single", "Soal bergambar")
	benar := f.option(q4, 1, "Benar", true)
	f.option(q4, 2, "Salah", false)
	mustExec(t, f.db, `
		UPDATE questions
		SET image_path = ?, image_mime = ?, image_alt = ?, image_bytes = ?, image_sha256 = ?
		WHERE id = ?`,
		"gambar.png", "image/png", "Diagram", 1234, make([]byte, 32), q4)

	ayu := f.newStudent("Ayu Lestari", 10)
	regAyu := f.register(quiz, ayu, "2026-10-21 08:00:00")
	budi := f.newStudent("Budi Santoso", 11)
	regBudi := f.register(quiz, budi, "2026-10-21 09:00:00")
	anon := f.newAnonymous("Citra Anonim", 12)
	regAnon := f.registerAnonymous(quiz, anon, "2026-10-21 10:00:00")

	// Ayu's first attempt is fully graded.
	a1 := f.attempt(regAyu, 1, 111, "2026-10-22 08:00:00", 0.7250,
		"2026-10-22 07:00:00", "2026-10-22 08:00:00")
	slot := f.answer(a1, q1, nil, 1.0, nil, f.teacherID, "2026-10-22 08:05:00")
	f.selectOption(slot, jakarta)
	slot = f.answer(a1, q2, nil, 1.0, nil, f.teacherID, "2026-10-22 08:05:00")
	f.selectOption(slot, satu)
	f.selectOption(slot, tiga)
	f.answer(a1, q3, "  Jawaban esai Ayu  ", 0.8550, "Bagus",
		f.teacherID, "2026-10-22 08:10:00")

	// Ayu's retake is waiting for the essay; the objective answers are graded.
	a2 := f.attempt(regAyu, 2, 222, "2026-10-22 09:00:00", nil,
		"2026-10-22 08:30:00", "2026-10-22 09:00:00")
	slot = f.answer(a2, q1, nil, 1.0, nil, f.teacherID, "2026-10-22 09:05:00")
	f.selectOption(slot, surabaya)
	slot = f.answer(a2, q2, nil, 0.0, nil, f.teacherID, "2026-10-22 09:05:00")
	f.selectOption(slot, dua)
	f.answer(a2, q3, "   ", nil, nil, nil, nil)

	// Budi is still working: submitted_at NULL, so no row and no answers.
	a3 := f.attempt(regBudi, 1, 333, nil, nil, "2026-10-22 09:00:00", "2026-10-22 09:10:00")

	// The anonymous participant's attempt is complete.
	a4 := f.attempt(regAnon, 1, 444, "2026-10-22 10:00:00", 0.5000,
		"2026-10-22 09:30:00", "2026-10-22 10:00:00")
	slot = f.answer(a4, q1, nil, 0.0, nil, f.teacherID, "2026-10-22 10:05:00")
	f.selectOption(slot, bandung)
	slot = f.answer(a4, q2, nil, 0.0, nil, f.teacherID, "2026-10-22 10:05:00")
	f.selectOption(slot, dua)
	f.answer(a4, q3, "Jawaban esai Citra", 1.0, nil, f.teacherID, "2026-10-22 10:05:00")
	slot = f.answer(a4, q4, nil, 1.0, nil, f.teacherID, "2026-10-22 10:05:00")
	f.selectOption(slot, benar)

	data, err := f.svc.Export(f.ctx, ExportInput{QuizID: quiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Metadata comes from the owned quiz plus the live owner account.
	if data.Quiz.ID != quiz || data.Quiz.Title != "Ulangan Harian" ||
		data.Quiz.State != viewmodel.QuizOpen || data.Quiz.OwnerName != "Guru Utama" {
		t.Errorf("quiz identity = %+v", data.Quiz)
	}
	if !data.Quiz.CreatedAt.Equal(time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("CreatedAt = %v", data.Quiz.CreatedAt)
	}
	if data.Quiz.PublishedAt == nil ||
		!data.Quiz.PublishedAt.Equal(time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("PublishedAt = %v, want the publish instant", data.Quiz.PublishedAt)
	}
	if data.Quiz.ClosedAt != nil {
		t.Errorf("ClosedAt = %v, want nil for an open quiz", data.Quiz.ClosedAt)
	}
	if data.Quiz.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", data.Quiz.MaxAttempts)
	}

	// Questions are the live set in position order, options canonical.
	if len(data.Questions) != 4 {
		t.Fatalf("Questions = %d, want 4", len(data.Questions))
	}
	wantQuestions := []int64{q1, q2, q3, q4}
	for i := range data.Questions {
		if data.Questions[i].ID != wantQuestions[i] || data.Questions[i].Position != i+1 {
			t.Errorf("question %d = id %d position %d, want id %d position %d",
				i, data.Questions[i].ID, data.Questions[i].Position, wantQuestions[i], i+1)
		}
	}
	if got := data.Questions[0].Options; len(got) != 3 ||
		got[0].Body != "Bandung" || got[1].Body != "Jakarta" || got[2].Body != "Surabaya" ||
		got[0].Correct || !got[1].Correct || got[2].Correct {
		t.Errorf("q1 options = %+v, want canonical order with Jakarta correct", got)
	}
	if got := data.Questions[3]; got.ImagePath != "gambar.png" || got.ImageMIME != "image/png" ||
		got.ImageAlt != "Diagram" || got.ImageBytes != 1234 {
		t.Errorf("q4 image = %+v", got)
	}
	if got := data.Questions[0]; got.ImagePath != "" || got.ImageMIME != "" ||
		got.ImageAlt != "" || got.ImageBytes != 0 {
		t.Errorf("q1 image = %+v, want zero values", got)
	}

	// Every live registration, sorted by lower(name), then id.
	if len(data.Participants) != 3 {
		t.Fatalf("Participants = %d, want 3", len(data.Participants))
	}
	wantParticipants := []struct {
		id    int64
		name  string
		anon  bool
		grade int
	}{
		{regAyu, "Ayu Lestari", false, 10},
		{regBudi, "Budi Santoso", false, 11},
		{regAnon, "Citra Anonim", true, 12},
	}
	for i, want := range wantParticipants {
		got := data.Participants[i]
		if got.RegistrationID != want.id || got.Name != want.name ||
			got.Anonymous != want.anon || got.Grade != want.grade || got.Major != "RPL" {
			t.Errorf("participant %d = %+v, want %+v", i, got, want)
		}
	}
	if !data.Participants[0].CreatedAt.Equal(time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("participant CreatedAt = %v", data.Participants[0].CreatedAt)
	}

	// Submitted attempts only, one row per attempt, retakes kept.
	if len(data.Attempts) != 3 {
		t.Fatalf("Attempts = %d, want 3", len(data.Attempts))
	}
	if got := data.Attempts[0]; got.ID != a1 || got.RegistrationID != regAyu || got.Number != 1 {
		t.Errorf("attempt 0 = %+v, want Ayu attempt 1", got)
	}
	if got := data.Attempts[0]; !got.SubmittedAt.Equal(time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("attempt 0 SubmittedAt = %v", got.SubmittedAt)
	}
	if got := data.Attempts[0].FinalUnits; got == nil || *got != 7250 {
		t.Errorf("attempt 0 FinalUnits = %v, want 7250", got)
	}
	if got := data.Attempts[1]; got.ID != a2 || got.RegistrationID != regAyu || got.Number != 2 {
		t.Errorf("attempt 1 = %+v, want Ayu attempt 2", got)
	}
	if data.Attempts[1].FinalUnits != nil {
		t.Errorf("attempt 1 FinalUnits = %d, want nil while waiting", *data.Attempts[1].FinalUnits)
	}
	if got := data.Attempts[2]; got.ID != a4 || got.RegistrationID != regAnon || got.Number != 1 {
		t.Errorf("attempt 2 = %+v, want the anonymous attempt", got)
	}
	if got := data.Attempts[2].FinalUnits; got == nil || *got != 5000 {
		t.Errorf("attempt 2 FinalUnits = %v, want 5000", got)
	}
	for _, attempt := range data.Attempts {
		if attempt.ID == a3 {
			t.Errorf("in-progress attempt %d was exported", a3)
		}
	}

	// Answers follow the question order for every attempt.
	for i := range data.Attempts {
		if len(data.Attempts[i].Answers) != len(data.Questions) {
			t.Fatalf("attempt %d Answers = %d, want %d",
				i, len(data.Attempts[i].Answers), len(data.Questions))
		}
		for j := range data.Questions {
			if data.Attempts[i].Answers[j].QuestionID != data.Questions[j].ID {
				t.Errorf("attempt %d answer %d question = %d, want %d",
					i, j, data.Attempts[i].Answers[j].QuestionID, data.Questions[j].ID)
			}
		}
	}

	if got := exportAnswerFor(t, data.Attempts[0], q1); got.Text != "B. Jakarta" ||
		got.ScoreUnits == nil || *got.ScoreUnits != 10000 {
		t.Errorf("attempt 0 q1 = %+v, want B. Jakarta / 10000", got)
	}
	if got := exportAnswerFor(t, data.Attempts[0], q2); got.Text != "A. Satu; C. Tiga" ||
		got.ScoreUnits == nil || *got.ScoreUnits != 10000 {
		t.Errorf("attempt 0 q2 = %+v, want A. Satu; C. Tiga / 10000", got)
	}
	if got := exportAnswerFor(t, data.Attempts[0], q3); got.Text != "Jawaban esai Ayu" ||
		got.ScoreUnits == nil || *got.ScoreUnits != 8550 {
		t.Errorf("attempt 0 q3 = %+v, want the trimmed essay / 8550", got)
	}
	if got := exportAnswerFor(t, data.Attempts[0], q4); got.Text != "—" || got.ScoreUnits != nil {
		t.Errorf("attempt 0 q4 = %+v, want unanswered", got)
	}

	if got := exportAnswerFor(t, data.Attempts[1], q1); got.Text != "C. Surabaya" ||
		got.ScoreUnits == nil || *got.ScoreUnits != 10000 {
		t.Errorf("attempt 1 q1 = %+v, want C. Surabaya / 10000", got)
	}
	if got := exportAnswerFor(t, data.Attempts[1], q2); got.Text != "B. Dua" ||
		got.ScoreUnits == nil || *got.ScoreUnits != 0 {
		t.Errorf("attempt 1 q2 = %+v, want B. Dua / 0", got)
	}
	if got := exportAnswerFor(t, data.Attempts[1], q3); got.Text != "—" || got.ScoreUnits != nil {
		t.Errorf("attempt 1 q3 = %+v, want blank essay / no score", got)
	}
	if got := exportAnswerFor(t, data.Attempts[1], q4); got.Text != "—" || got.ScoreUnits != nil {
		t.Errorf("attempt 1 q4 = %+v, want unanswered", got)
	}

	if got := exportAnswerFor(t, data.Attempts[2], q1); got.Text != "A. Bandung" ||
		got.ScoreUnits == nil || *got.ScoreUnits != 0 {
		t.Errorf("attempt 2 q1 = %+v, want A. Bandung / 0", got)
	}
	if got := exportAnswerFor(t, data.Attempts[2], q2); got.Text != "B. Dua" ||
		got.ScoreUnits == nil || *got.ScoreUnits != 0 {
		t.Errorf("attempt 2 q2 = %+v, want B. Dua / 0", got)
	}
	if got := exportAnswerFor(t, data.Attempts[2], q3); got.Text != "Jawaban esai Citra" ||
		got.ScoreUnits == nil || *got.ScoreUnits != 10000 {
		t.Errorf("attempt 2 q3 = %+v, want the essay / 10000", got)
	}
	if got := exportAnswerFor(t, data.Attempts[2], q4); got.Text != "A. Benar" ||
		got.ScoreUnits == nil || *got.ScoreUnits != 10000 {
		t.Errorf("attempt 2 q4 = %+v, want A. Benar / 10000", got)
	}
}

// TestExportOwnership pins the 404 contract: a foreign teacher, a student, a
// missing quiz, and a draft are all ErrNotFound, while the owner and a closed
// quiz succeed.
func TestExportOwnership(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Kepemilikan", "open", true, false, 5, "2026-10-20 09:00:00")
	draft := f.newQuiz(f.teacherID, "Draf", "draft", true, false, 1, "2026-10-20 10:00:00")
	closed := f.newQuiz(f.teacherID, "Ditutup", "closed", true, false, 1, "2026-10-20 11:00:00")
	studentAccount := insertAccount(t, f.db, "Siswa Aktor", "siswa-aktor@test", viewmodel.RoleStudent)

	owner, err := f.svc.Export(f.ctx, ExportInput{QuizID: quiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("owner export: %v", err)
	}
	if owner.Quiz.ID != quiz || owner.Quiz.MaxAttempts != 1 {
		t.Errorf("owner export = %+v, want quiz %d with effective max 1", owner.Quiz, quiz)
	}

	closedData, err := f.svc.Export(f.ctx, ExportInput{QuizID: closed, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("closed export: %v", err)
	}
	if closedData.Quiz.State != viewmodel.QuizClosed || closedData.Quiz.ClosedAt == nil ||
		closedData.Quiz.PublishedAt == nil {
		t.Errorf("closed export metadata = %+v, want closed and both dates", closedData.Quiz)
	}

	for _, tc := range []struct {
		name string
		in   ExportInput
	}{
		{"foreign teacher", ExportInput{QuizID: quiz, AccountID: f.otherTeacherID}},
		{"student actor", ExportInput{QuizID: quiz, AccountID: studentAccount}},
		{"missing quiz", ExportInput{QuizID: 999999, AccountID: f.teacherID}},
		{"draft", ExportInput{QuizID: draft, AccountID: f.teacherID}},
		{"foreign draft", ExportInput{QuizID: draft, AccountID: f.otherTeacherID}},
	} {
		if _, err := f.svc.Export(f.ctx, tc.in); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s Export error = %v, want ErrNotFound", tc.name, err)
		}
	}
}

// TestExportCanonicalResponsesIgnoreShuffle turns randomize_options on and uses
// two attempts with different shuffle seeds. The response must still name the
// stored option letter and body, never a shuffled display position.
func TestExportCanonicalResponsesIgnoreShuffle(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Opsi Acak", "open", true, true, 3, "2026-10-20 09:00:00")
	mustExec(t, f.db, `UPDATE quizzes SET randomize_options = 1 WHERE id = ?`, quiz)

	q := f.question(quiz, 1, "single", "Ibu kota Indonesia?")
	f.option(q, 1, "Bandung", false)
	jakarta := f.option(q, 2, "Jakarta", true)
	f.option(q, 3, "Surabaya", false)

	student := f.newStudent("Ayu Lestari", 10)
	reg := f.register(quiz, student, "2026-10-21 08:00:00")

	for i, seed := range []uint32{1, 987654} {
		attempt := f.attempt(reg, i+1, seed, "2026-10-22 08:00:00", 1.0,
			"2026-10-22 07:00:00", "2026-10-22 08:00:00")
		slot := f.answer(attempt, q, nil, 1.0, nil, f.teacherID, "2026-10-22 08:05:00")
		f.selectOption(slot, jakarta)
	}

	data, err := f.svc.Export(f.ctx, ExportInput{QuizID: quiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(data.Attempts) != 2 {
		t.Fatalf("Attempts = %d, want 2", len(data.Attempts))
	}
	options := data.Questions[0].Options
	if len(options) != 3 || options[0].Body != "Bandung" || options[1].Body != "Jakarta" ||
		options[2].Body != "Surabaya" {
		t.Fatalf("options = %+v, want canonical stored order", options)
	}
	for _, attempt := range data.Attempts {
		if got := exportAnswerFor(t, attempt, q); got.Text != "B. Jakarta" {
			t.Errorf("attempt %d response = %q, want canonical B. Jakarta", attempt.ID, got.Text)
		}
	}
}

// TestExportZeroScoreIsNotNil pins the nullable-score distinction: a stored
// final or answer score of 0 is a scored value, not a waiting row.
func TestExportZeroScoreIsNotNil(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Nilai Nol", "open", true, false, 1, "2026-10-20 09:00:00")
	q := f.question(quiz, 1, "single", "Pilih satu")
	f.option(q, 1, "Satu", true)

	student := f.newStudent("Ayu Lestari", 10)
	reg := f.register(quiz, student, "2026-10-21 08:00:00")
	attempt := f.attempt(reg, 1, 1, "2026-10-22 08:00:00", 0.0,
		"2026-10-22 07:00:00", "2026-10-22 08:00:00")
	f.answer(attempt, q, nil, 0.0, nil, f.teacherID, "2026-10-22 08:05:00")

	data, err := f.svc.Export(f.ctx, ExportInput{QuizID: quiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(data.Attempts) != 1 {
		t.Fatalf("Attempts = %d, want 1", len(data.Attempts))
	}
	if got := data.Attempts[0].FinalUnits; got == nil || *got != 0 {
		t.Errorf("FinalUnits = %v, want a non-nil 0", got)
	}
	if got := exportAnswerFor(t, data.Attempts[0], q); got.ScoreUnits == nil || *got.ScoreUnits != 0 {
		t.Errorf("ScoreUnits = %v, want a non-nil 0", got.ScoreUnits)
	}
}

// TestExportEmptyResponses pins the readable placeholder for an objective row
// with no selection and an essay whose stored text is blank.
func TestExportEmptyResponses(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Kosong", "open", true, false, 1, "2026-10-20 09:00:00")
	q1 := f.question(quiz, 1, "single", "Pilih satu")
	f.option(q1, 1, "Satu", true)
	q2 := f.question(quiz, 2, "essay", "Tulis jawaban")

	student := f.newStudent("Ayu Lestari", 10)
	reg := f.register(quiz, student, "2026-10-21 08:00:00")
	attempt := f.attempt(reg, 1, 1, "2026-10-22 08:00:00", nil,
		"2026-10-22 07:00:00", "2026-10-22 08:00:00")
	f.answer(attempt, q1, nil, 0.0, nil, f.teacherID, "2026-10-22 08:05:00")
	f.answer(attempt, q2, "   ", nil, nil, nil, nil)

	data, err := f.svc.Export(f.ctx, ExportInput{QuizID: quiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(data.Attempts) != 1 {
		t.Fatalf("Attempts = %d, want 1", len(data.Attempts))
	}
	for _, qid := range []int64{q1, q2} {
		if got := exportAnswerFor(t, data.Attempts[0], qid); got.Text != "—" {
			t.Errorf("question %d response = %q, want —", qid, got.Text)
		}
	}
}

// TestExportResponseFormatter is the pure formatter contract: canonical
// letters, semicolon-joined multiple selections, trimmed essays, and the
// placeholder for missing or stale selections.
func TestExportResponseFormatter(t *testing.T) {
	objective := ExportQuestion{
		Type: viewmodel.QuestionSingle,
		Options: []ExportOption{
			{ID: 10, Position: 1, Body: "Satu"},
			{ID: 20, Position: 2, Body: "Dua"},
			{ID: 30, Position: 3, Body: "Tiga"},
		},
	}
	essay := ExportQuestion{Type: viewmodel.QuestionEssay}
	cases := []struct {
		name     string
		question ExportQuestion
		selected []int64
		text     sql.NullString
		want     string
	}{
		{"single canonical", objective, []int64{20}, sql.NullString{}, "B. Dua"},
		{"multiple canonical order", objective, []int64{30, 10}, sql.NullString{}, "A. Satu; C. Tiga"},
		{"no selection", objective, nil, sql.NullString{}, "—"},
		{"stale selection", objective, []int64{999}, sql.NullString{}, "—"},
		{"essay trimmed", essay, nil, sql.NullString{String: "  jawaban  ", Valid: true}, "jawaban"},
		{"essay blank", essay, nil, sql.NullString{String: "   ", Valid: true}, "—"},
		{"essay null", essay, nil, sql.NullString{}, "—"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exportResponse(tc.question, tc.selected, tc.text); got != tc.want {
				t.Errorf("exportResponse = %q, want %q", got, tc.want)
			}
		})
	}
}
