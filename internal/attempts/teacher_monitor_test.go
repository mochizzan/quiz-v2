package attempts

import (
	"errors"
	"testing"
	"time"

	"quiz-v2/internal/viewmodel"
)

// TestMonitorSnapshot pins ATT-181…ATT-187: one row per live registration,
// current-attempt selection (in-progress wins over submitted), actual answered
// counts, focus counts, last-seen, state ordering, and the progress rail.
func TestMonitorSnapshot(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Monitor Kuis", "open", true, false, 1, "2026-10-20 09:00:00")
	single := f.question(quiz, 1, "single", "Pilih satu")
	essay := f.question(quiz, 2, "essay", "Jelaskan")
	optA := f.option(single, 1, "A", true)
	optB := f.option(single, 2, "B", false)

	// Ayu has an older submitted attempt and a live in-progress attempt. The
	// in-progress one must be selected, and only its content counts.
	ayu := f.newStudent("Ayu Lestari", 10)
	ayuReg := f.register(quiz, ayu, "2026-10-20 10:00:00")
	ayuOld := f.attempt(ayuReg, 1, 1, "2026-10-21 04:00:00", 0.5,
		"2026-10-21 03:00:00", "2026-10-21 04:00:00")
	oldAns := f.answer(ayuOld, single, nil, 1.0, nil, nil, "2026-10-21 04:00:00")
	f.selectOption(oldAns, optA)
	oldEssay := f.answer(ayuOld, essay, "Jawaban lama", nil, nil, nil, nil)
	_ = oldEssay
	ayuLive := f.attempt(ayuReg, 2, 2, nil, nil,
		"2026-10-23 06:00:00", "2026-10-23 06:30:00")
	liveAns := f.answer(ayuLive, single, nil, nil, nil, nil, nil)
	f.selectOption(liveAns, optB)
	f.answer(ayuLive, essay, nil, nil, nil, nil, nil)

	// Budi has one live attempt with both slots answered and two focus losses.
	budi := f.newStudent("Budi Santoso", 11)
	budiReg := f.register(quiz, budi, "2026-10-20 11:00:00")
	budiAttempt := f.attempt(budiReg, 1, 3, nil, nil,
		"2026-10-23 05:00:00", "2026-10-23 05:30:00")
	budiAns := f.answer(budiAttempt, single, nil, nil, nil, nil, nil)
	f.selectOption(budiAns, optA)
	f.answer(budiAttempt, essay, "Jawaban Budi", nil, nil, nil, nil)
	f.focus(budiAttempt, "blur", "2026-10-23 05:10:00")
	f.focus(budiAttempt, "hidden", "2026-10-23 05:20:00")

	// Dodi submitted both answers, so he is completed, and his actual content
	// (not total) drives the answered count.
	dodi := f.newStudent("Dodi Pratama", 12)
	dodiReg := f.register(quiz, dodi, "2026-10-20 12:00:00")
	dodiAttempt := f.attempt(dodiReg, 1, 4, "2026-10-22 04:30:00", 0.8,
		"2026-10-22 03:00:00", "2026-10-22 04:35:00")
	dodiAns := f.answer(dodiAttempt, single, nil, 1.0, nil, nil, "2026-10-22 04:30:00")
	f.selectOption(dodiAns, optA)
	f.answer(dodiAttempt, essay, "Jawaban Dodi", 0.6, nil,
		f.teacherID, "2026-10-22 05:00:00")
	f.focus(dodiAttempt, "blur", "2026-10-22 04:10:00")

	// Two registrations with no attempt are still rows.
	anon := f.newAnonymous("Anonim Satu", 10)
	anonReg := f.registerAnonymous(quiz, anon, "2026-10-23 02:00:00")
	citra := f.newStudent("Citra Dewi", 12)
	citraReg := f.register(quiz, citra, "2026-10-23 03:00:00")

	page, err := f.svc.MonitorPage(f.ctx, MonitorInput{QuizID: quiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("MonitorPage: %v", err)
	}
	if page.Quiz.Title != "Monitor Kuis" || page.StreamHref != quizPath(quiz)+"/stream" {
		t.Errorf("page identity = %+v", page)
	}
	if page.GeneratedAt.IsZero() {
		t.Error("GeneratedAt is zero")
	}
	if page.Progress.Registered != 5 || page.Progress.InProgress != 2 ||
		page.Progress.Completed != 1 || page.Progress.Answered != 5 ||
		page.Progress.Total != 10 || page.Progress.PercentAnswered() != 50 {
		t.Fatalf("progress = %+v (percent %d), want 5/2/1/5/10 at 50%%",
			page.Progress, page.Progress.PercentAnswered())
	}
	if len(page.Rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(page.Rows))
	}

	want := []struct {
		name       string
		initials   string
		class      string
		state      viewmodel.ParticipationState
		answered   int
		focus      int
		lastSeen   string
		hasAttempt bool
		anonymous  bool
	}{
		{"Ayu Lestari", "AL", "X RPL", viewmodel.ParticipationInProgress, 1, 0, "2026-10-23 06:30:00", true, false},
		{"Budi Santoso", "BS", "XI RPL", viewmodel.ParticipationInProgress, 2, 2, "2026-10-23 05:30:00", true, false},
		{"Dodi Pratama", "DP", "XII RPL", viewmodel.ParticipationCompleted, 2, 1, "2026-10-22 04:35:00", true, false},
		{"Anonim Satu", "AS", "X RPL", viewmodel.ParticipationRegistered, 0, 0, "2026-10-23 02:00:00", false, true},
		{"Citra Dewi", "CD", "XII RPL", viewmodel.ParticipationRegistered, 0, 0, "2026-10-23 03:00:00", false, false},
	}
	for i, wantRow := range want {
		row := page.Rows[i]
		if row.StudentName != wantRow.name || row.Initials != wantRow.initials ||
			row.ClassLabel != wantRow.class || row.State != wantRow.state ||
			row.Answered != wantRow.answered || row.FocusLoss != wantRow.focus ||
			row.Total != 2 || row.Anonymous != wantRow.anonymous {
			t.Errorf("row %d = %+v, want %+v", i, row, wantRow)
		}
		if (row.AttemptID != nil) != wantRow.hasAttempt {
			t.Errorf("row %d AttemptID = %v, want hasAttempt=%v", i, row.AttemptID, wantRow.hasAttempt)
		}
		if row.UpdatedAt == nil {
			t.Fatalf("row %d UpdatedAt is nil", i)
		}
		wantTime, _ := time.Parse("2006-01-02 15:04:05", wantRow.lastSeen)
		if !row.UpdatedAt.Time().Equal(wantTime) {
			t.Errorf("row %d last seen = %v, want %v", i, row.UpdatedAt.Time(), wantTime)
		}
	}
	if page.Rows[0].AttemptID == nil || page.Rows[0].RegistrationID != ayuReg {
		t.Errorf("Ayu selected attempt = %v, want the in-progress attempt", page.Rows[0].AttemptID)
	}
	if page.Rows[3].RegistrationID != anonReg || page.Rows[4].RegistrationID != citraReg {
		t.Errorf("registered order = %d, %d; want %d, %d",
			page.Rows[3].RegistrationID, page.Rows[4].RegistrationID, anonReg, citraReg)
	}

	// A closed quiz is a 200 not-on-air state, not an error and not rows.
	f.setQuizState(quiz, "closed")
	closed, err := f.svc.MonitorPage(f.ctx, MonitorInput{QuizID: quiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("closed monitor: %v", err)
	}
	if len(closed.Rows) != 0 || closed.StreamHref != "" {
		t.Errorf("closed monitor has rows/stream: %+v", closed)
	}
	if closed.Empty.Title != "Kuis tidak sedang berlangsung" {
		t.Errorf("closed empty title = %q", closed.Empty.Title)
	}

	// A draft is also not-on-air when owned (ATT-181).
	draft := f.newQuiz(f.teacherID, "Draf Monitor", "draft", true, false, 1, "2026-10-20 08:00:00")
	draftPage, err := f.svc.MonitorPage(f.ctx, MonitorInput{QuizID: draft, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("draft monitor: %v", err)
	}
	if draftPage.Empty.Title != "Kuis tidak sedang berlangsung" || len(draftPage.Rows) != 0 {
		t.Errorf("draft monitor = %+v", draftPage)
	}

	for _, tc := range []struct {
		name string
		in   MonitorInput
	}{
		{"foreign", MonitorInput{QuizID: quiz, AccountID: f.otherTeacherID}},
		{"missing", MonitorInput{QuizID: 999999, AccountID: f.teacherID}},
	} {
		if _, err := f.svc.MonitorPage(f.ctx, tc.in); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s monitor error = %v, want ErrNotFound", tc.name, err)
		}
	}
}

// TestMonitorQuestionCountEmpty confirms that an open quiz with zero
// registrations still has a coherent rail (total 0, percent 0).
func TestMonitorEmptyOpenQuiz(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Belum Ada Peserta", "open", true, false, 1, "2026-10-20 09:00:00")
	f.question(quiz, 1, "essay", "Soal")

	page, err := f.svc.MonitorPage(f.ctx, MonitorInput{QuizID: quiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("MonitorPage: %v", err)
	}
	if len(page.Rows) != 0 || page.Progress.Registered != 0 ||
		page.Progress.Total != 0 || page.Progress.PercentAnswered() != 0 {
		t.Errorf("empty open monitor = rows %d, progress %+v", len(page.Rows), page.Progress)
	}
	if page.Empty.Title != "Belum ada peserta" {
		t.Errorf("empty open title = %q", page.Empty.Title)
	}
}
