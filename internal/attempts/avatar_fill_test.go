package attempts

// avatar_fill_test.go pins AVT-083 for every read surface this package owns:
// the stored photo name is joined in the query that already joins accounts,
// and it reaches the row's Avatar with the class the surface renders (row
// lists at 28 px, headers at the default 32 px, monitor at its own 36 px).
// A photo-less or anonymous participant keeps the initials fallback.

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"quiz-v2/internal/viewmodel"
)

// avatarNameOf builds a name the avatar grammar accepts: 32 lowercase hex
// characters plus .webp. The seed keeps names distinct, because the schema
// keys (avatar_name, deleted_at) unique across live rows (AVT-083).
func avatarNameOf(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:16]) + ".webp"
}

// giveAvatar stores a photo on one student's account and returns the stored
// name; it is the test's stand-in for the upload slice.
func (f *teacherFixture) giveAvatar(studentID int64, seed string) string {
	f.t.Helper()
	name := avatarNameOf(seed)
	mustExec(f.t, f.db,
		`UPDATE accounts SET avatar_name = ?
		  WHERE id = (SELECT account_id FROM students WHERE id = ?)`,
		name, studentID)
	return name
}

// accountOf resolves the acting account behind a student profile row.
func (f *teacherFixture) accountOf(studentID int64) int64 {
	f.t.Helper()
	var accountID int64
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT account_id FROM students WHERE id = ?`, studentID,
	).Scan(&accountID); err != nil {
		f.t.Fatalf("account of student %d: %v", studentID, err)
	}
	return accountID
}

// TestAvatarFillsGradingReads pins both grading surfaces: queue rows carry
// the 28 px avatar next to the name, and the detail header carries the
// student's photo at the default size on both of its models.
func TestAvatarFillsGradingReads(t *testing.T) {
	w := newGradingWorld(t)
	f := w.f
	photo := f.giveAvatar(w.ayu.studentID, "grading-ayu")
	wantURL := "/avatars/" + photo

	page, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("GradingQueue: %v", err)
	}
	byName := map[string]viewmodel.Avatar{}
	for _, row := range page.Queue.Rows.Items {
		byName[row.StudentName] = row.Avatar
	}
	if ayu := byName["Ayu Lestari"]; ayu.URL != wantURL || ayu.Class != "app-avatar--sm" {
		t.Errorf("Ayu queue avatar = %+v, want %s at app-avatar--sm", ayu, wantURL)
	}
	// Budi has no photo: the same class, but the initials fallback (AVT-082).
	if budi := byName["Budi Santoso"]; budi.URL != "" || budi.Initials != "BS" ||
		budi.Class != "app-avatar--sm" {
		t.Errorf("Budi queue avatar = %+v, want initials-only at app-avatar--sm", budi)
	}

	detail, err := f.svc.GradingDetail(f.ctx, GradingDetailInput{
		GradingQueueInput: GradingQueueInput{AccountID: f.teacherID},
		AnswerID:          w.ayu.essay1ID,
	})
	if err != nil {
		t.Fatalf("GradingDetail: %v", err)
	}
	if detail.Selected == nil {
		t.Fatal("Selected = nil, want the essay")
	}
	answer := detail.Selected.Answer.Avatar
	if answer.URL != wantURL || answer.Class != "" {
		t.Errorf("answer avatar = %+v, want %s at the default size", answer, wantURL)
	}
	attempt := detail.Selected.Attempt.Avatar
	if attempt.URL != wantURL || attempt.Class != "" {
		t.Errorf("attempt avatar = %+v, want %s at the default size", attempt, wantURL)
	}
}

// TestAvatarFillsResultsAndRecap pins the results row (28 px) and the recap
// header (default size) of the same submitted attempt.
func TestAvatarFillsResultsAndRecap(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Ulangan Avatar", "open", true, false, 3, "2026-09-20 09:00:00")
	essay := f.question(quiz, 1, "essay", "Jelaskan")

	ayu := f.newStudent("Ayu Lestari", 10)
	photo := f.giveAvatar(ayu, "results-ayu")
	wantURL := "/avatars/" + photo
	regAyu := f.register(quiz, ayu, "2026-09-21 10:00:00")
	attemptAyu := f.attempt(regAyu, 1, 11, "2026-10-01 08:00:00", 0.7250,
		"2026-10-01 07:00:00", "2026-10-01 08:00:00")
	f.answer(attemptAyu, essay, "Jawaban Ayu", 1.0, nil, f.teacherID, "2026-10-01 08:30:00")

	// Budi is submitted but still waiting for a grade, so he exercises the
	// same row without a photo.
	budi := f.newStudent("Budi Santoso", 11)
	regBudi := f.register(quiz, budi, "2026-09-21 11:00:00")
	attemptBudi := f.attempt(regBudi, 1, 12, "2026-10-02 08:00:00", nil,
		"2026-10-02 07:00:00", "2026-10-02 08:00:00")
	f.answer(attemptBudi, essay, "Jawaban Budi", nil, nil, nil, nil)

	page, err := f.svc.Results(f.ctx, ResultsInput{QuizID: quiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("Results: %v", err)
	}
	byName := map[string]viewmodel.Avatar{}
	for _, row := range page.Rows.Items {
		byName[row.StudentName] = row.Avatar
	}
	if ayu := byName["Ayu Lestari"]; ayu.URL != wantURL || ayu.Class != "app-avatar--sm" {
		t.Errorf("Ayu results avatar = %+v, want %s at app-avatar--sm", ayu, wantURL)
	}
	if budi := byName["Budi Santoso"]; budi.URL != "" || budi.Initials != "BS" ||
		budi.Class != "app-avatar--sm" {
		t.Errorf("Budi results avatar = %+v, want initials-only at app-avatar--sm", budi)
	}

	recap, err := f.svc.Recap(f.ctx, RecapInput{AccountID: f.teacherID, AttemptID: attemptAyu})
	if err != nil {
		t.Fatalf("Recap: %v", err)
	}
	if got := recap.Attempt.Avatar; got.URL != wantURL || got.Class != "" {
		t.Errorf("recap avatar = %+v, want %s at the default size", got, wantURL)
	}
}

// TestAvatarFillsTopScores pins the dashboard table: the student row carries
// the photo at 28 px and the anonymous row, which has no account at all,
// keeps the initials fallback (AVT-083).
func TestAvatarFillsTopScores(t *testing.T) {
	f := newTeacherFixture(t)

	ayu := f.newStudent("Ayu Lestari", 10)
	photo := f.giveAvatar(ayu, "top-ayu")
	wantURL := "/avatars/" + photo
	quiz := f.newQuiz(f.teacherID, "Nilai Ayu", "open", true, false, 1, "2026-10-01 09:00:00")
	regAyu := f.register(quiz, ayu, "2026-10-01 10:00:00")
	f.attempt(regAyu, 1, 1, "2026-10-02 10:00:00", 0.7250,
		"2026-10-02 09:00:00", "2026-10-02 10:00:00")

	anonymous := f.newAnonymous("Peserta Rahasia", 11)
	quiz2 := f.newQuiz(f.teacherID, "Nilai Anonim", "open", true, false, 1, "2026-10-03 09:00:00")
	regAnon := f.registerAnonymous(quiz2, anonymous, "2026-10-03 10:00:00")
	f.attempt(regAnon, 1, 2, "2026-10-04 10:00:00", 0.9000,
		"2026-10-04 09:00:00", "2026-10-04 10:00:00")

	board, err := f.svc.TeacherDashboard(f.ctx, f.teacherID, "Guru Utama")
	if err != nil {
		t.Fatalf("TeacherDashboard: %v", err)
	}
	if len(board.TopScores) != 2 {
		t.Fatalf("TopScores = %d rows, want 2", len(board.TopScores))
	}
	for _, row := range board.TopScores {
		switch row.StudentName {
		case "Ayu Lestari":
			if row.Avatar.URL != wantURL || row.Avatar.Class != "app-avatar--sm" {
				t.Errorf("Ayu top-score avatar = %+v, want %s at app-avatar--sm", row.Avatar, wantURL)
			}
		case "Peserta Rahasia":
			if row.Avatar.URL != "" || row.Avatar.Initials != "PR" ||
				row.Avatar.Class != "app-avatar--sm" {
				t.Errorf("anonymous top-score avatar = %+v, want initials-only at app-avatar--sm",
					row.Avatar)
			}
		default:
			t.Errorf("unexpected top-score row %q", row.StudentName)
		}
	}
}

// TestAvatarFillsMonitorRows pins the monitor's own 36 px class on both a
// student with a photo and an anonymous participant without one.
func TestAvatarFillsMonitorRows(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Monitor Avatar", "open", true, false, 1, "2026-10-20 09:00:00")
	f.question(quiz, 1, "essay", "Jelaskan")

	ayu := f.newStudent("Ayu Lestari", 10)
	photo := f.giveAvatar(ayu, "monitor-ayu")
	wantURL := "/avatars/" + photo
	regAyu := f.register(quiz, ayu, "2026-10-23 08:00:00")
	f.attempt(regAyu, 1, 9, nil, nil, "2026-10-23 08:30:00", "2026-10-23 09:00:00")

	anonymous := f.newAnonymous("Peserta Rahasia", 11)
	f.registerAnonymous(quiz, anonymous, "2026-10-23 08:45:00")

	page, err := f.svc.MonitorPage(f.ctx, MonitorInput{QuizID: quiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("MonitorPage: %v", err)
	}
	byName := map[string]viewmodel.Avatar{}
	for _, row := range page.Rows {
		byName[row.StudentName] = row.Avatar
	}
	if ayu := byName["Ayu Lestari"]; ayu.URL != wantURL || ayu.Class != "monitor-avatar" {
		t.Errorf("monitor avatar = %+v, want %s at monitor-avatar", ayu, wantURL)
	}
	if anon := byName["Peserta Rahasia"]; anon.URL != "" || anon.Initials != "PR" ||
		anon.Class != "monitor-avatar" {
		t.Errorf("anonymous monitor avatar = %+v, want initials-only at monitor-avatar", anon)
	}
}

// TestAvatarFillsStudentAttemptHeader pins the student's own pages: the
// header of the question screen and of the result screen carry that account's
// photo at the default size, from the join those reads already have.
func TestAvatarFillsStudentAttemptHeader(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Kuis Saya", "open", true, false, 1, "2026-10-20 09:00:00")
	single := f.question(quiz, 1, "single", "Pilih satu")
	opt := f.option(single, 1, "A", true)

	// The in-progress screen reads the header of attempt 1.
	ayu := f.newStudent("Ayu Lestari", 10)
	ayuPhoto := f.giveAvatar(ayu, "attempt-ayu")
	ayuOwner := OwnerRef{Role: viewmodel.RoleStudent, AccountID: f.accountOf(ayu)}
	regAyu := f.register(quiz, ayu, "2026-10-23 08:00:00")
	inProgress := f.attempt(regAyu, 1, 9, nil, nil, "2026-10-23 08:30:00", "2026-10-23 08:30:00")

	page, err := f.svc.QuestionPage(f.ctx, QuestionInput{AttemptID: uint32(inProgress), Owner: ayuOwner})
	if err != nil {
		t.Fatalf("QuestionPage: %v", err)
	}
	wantQuestion := "/avatars/" + ayuPhoto
	if got := page.Header.Avatar; got.URL != wantQuestion || got.Class != "" {
		t.Errorf("question header avatar = %+v, want %s at the default size", got, wantQuestion)
	}

	// The result screen reads the same fill for another student's submitted
	// attempt, with that student's own photo.
	budi := f.newStudent("Budi Santoso", 11)
	budiPhoto := f.giveAvatar(budi, "attempt-budi")
	budiOwner := OwnerRef{Role: viewmodel.RoleStudent, AccountID: f.accountOf(budi)}
	regBudi := f.register(quiz, budi, "2026-10-23 08:15:00")
	submitted := f.attempt(regBudi, 1, 10, "2026-10-23 09:00:00", 1.0,
		"2026-10-23 08:30:00", "2026-10-23 09:00:00")
	answered := f.answer(submitted, single, nil, 1.0, nil, f.teacherID, "2026-10-23 09:05:00")
	f.selectOption(answered, opt)

	result, err := f.svc.ResultPage(f.ctx, ResultInput{AttemptID: uint32(submitted), Owner: budiOwner})
	if err != nil {
		t.Fatalf("ResultPage: %v", err)
	}
	wantResult := "/avatars/" + budiPhoto
	if got := result.Header.Avatar; got.URL != wantResult || got.Class != "" {
		t.Errorf("result header avatar = %+v, want %s at the default size", got, wantResult)
	}
}
