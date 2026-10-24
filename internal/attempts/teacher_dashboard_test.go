package attempts

import (
	"testing"

	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// TestTeacherDashboardCards pins ATT-161…ATT-164: one snapshot scoped to the
// owner, the top-score tie-break and cap, the recent cap, and the two
// participation cards that exclude drafts.
func TestTeacherDashboardCards(t *testing.T) {
	f := newTeacherFixture(t)

	ayu := f.newStudent("Ayu Lestari", 10)
	budi := f.newStudent("Budi Santoso", 11)
	citra := f.newStudent("Citra Dewi", 12)
	dodi := f.newStudent("Dodi Pratama", 10)

	// Created order is deliberate: it drives both Recent and TopScores.
	ancient := f.newQuiz(f.teacherID, "Sejarah", "open", true, false, 1, "2026-09-01 09:00:00")
	c1 := f.newQuiz(f.teacherID, "Ekonomi", "open", true, false, 1, "2026-09-10 09:00:00")
	c2 := f.newQuiz(f.teacherID, "Fisika", "open", true, false, 1, "2026-09-11 09:00:00")
	old := f.newQuiz(f.teacherID, "Aljabar", "open", true, false, 1, "2026-10-01 09:00:00")
	extra := f.newQuiz(f.teacherID, "Kimia", "open", true, false, 1, "2026-10-02 09:00:00")
	hidden := f.newQuiz(f.teacherID, "Tersembunyi", "closed", false, false, 1, "2026-10-04 09:00:00")
	newest := f.newQuiz(f.teacherID, "Biologi", "open", true, false, 1, "2026-10-05 09:00:00")
	draft := f.newQuiz(f.teacherID, "Draf", "draft", true, false, 1, "2026-10-07 09:00:00")
	other := f.newQuiz(f.otherTeacherID, "Punyanya Lain", "open", true, false, 1, "2026-10-08 09:00:00")

	// Every quiz gets a registration so the participation cards have real
	// counts. The draft registration proves drafts are excluded, not just
	// empty.
	regOldAyu := f.register(old, ayu, "2026-10-01 10:00:00")
	regOldBudi := f.register(old, budi, "2026-10-01 10:00:00")
	f.register(newest, citra, "2026-10-05 10:00:00")
	f.register(extra, dodi, "2026-10-02 10:00:00")
	f.register(hidden, ayu, "2026-10-04 10:00:00")
	f.register(ancient, ayu, "2026-09-01 10:00:00")
	f.register(c1, ayu, "2026-09-10 10:00:00")
	f.register(c2, ayu, "2026-09-11 10:00:00")
	f.register(draft, ayu, "2026-10-07 10:00:00")
	regOther := f.register(other, ayu, "2026-10-08 10:00:00")

	// qOld tie at 0.8000: Budi's earlier submission must beat Ayu's later one.
	f.attempt(regOldAyu, 1, 1, "2026-10-02 10:00:00", 0.7250,
		"2026-10-02 09:00:00", "2026-10-02 10:00:00")
	f.attempt(regOldAyu, 2, 2, "2026-10-03 10:00:00", 0.8000,
		"2026-10-03 09:00:00", "2026-10-03 10:00:00")
	f.attempt(regOldBudi, 1, 3, "2026-10-01 10:00:00", 0.8000,
		"2026-10-01 09:00:00", "2026-10-01 10:00:00")

	for _, q := range []struct {
		quiz  int64
		reg   int64
		when  string
		score float64
	}{
		{newest, f.registrationID(t, newest, citra), "2026-10-06 10:00:00", 0.9500},
		{extra, f.registrationID(t, extra, dodi), "2026-10-02 10:00:00", 0.9000},
		{hidden, f.registrationID(t, hidden, ayu), "2026-10-05 10:00:00", 0.9900},
		{ancient, f.registrationID(t, ancient, ayu), "2026-09-02 10:00:00", 0.9900},
		{c1, f.registrationID(t, c1, ayu), "2026-09-12 10:00:00", 0.6000},
		{c2, f.registrationID(t, c2, ayu), "2026-09-13 10:00:00", 0.7000},
		{other, regOther, "2026-10-09 10:00:00", 0.8800},
	} {
		f.attempt(q.reg, 1, 7, q.when, q.score, q.when, q.when)
	}

	board, err := f.svc.TeacherDashboard(f.ctx, f.teacherID, "Guru Utama")
	if err != nil {
		t.Fatalf("TeacherDashboard: %v", err)
	}
	if board.GreetingName != "Guru Utama" || board.DateLine == "" {
		t.Fatalf("greeting/date = (%q, %q), want the session name and a date",
			board.GreetingName, board.DateLine)
	}

	// ATT-162: owned, live, show_score; best attempt with earliest tie; order
	// by quiz creation, cap 5. The hidden quiz is excluded, and the ancient
	// quiz's 99% loses to the cap rather than to a lower score.
	wantTop := []struct {
		title   string
		student string
		class   string
		score   int
	}{
		{"Biologi", "Citra Dewi", "XII RPL", 95},
		{"Kimia", "Dodi Pratama", "X RPL", 90},
		{"Aljabar", "Budi Santoso", "XI RPL", 80},
		{"Fisika", "Ayu Lestari", "X RPL", 70},
		{"Ekonomi", "Ayu Lestari", "X RPL", 60},
	}
	if len(board.TopScores) != len(wantTop) {
		t.Fatalf("TopScores = %d rows, want %d: %+v", len(board.TopScores), len(wantTop), board.TopScores)
	}
	for i, want := range wantTop {
		got := board.TopScores[i]
		if got.Quiz.Title != want.title || got.StudentName != want.student ||
			got.ClassLabel != want.class || got.Score.Int() != want.score {
			t.Errorf("TopScores[%d] = %+v, want %+v", i, got, want)
		}
		if got.Href != resultsPath(got.Quiz.ID) {
			t.Errorf("TopScores[%d].Href = %q, want %q", i, got.Href, resultsPath(got.Quiz.ID))
		}
	}

	// ATT-163: five newest owned live quizzes, drafts included.
	wantRecent := []string{"Draf", "Biologi", "Tersembunyi", "Kimia", "Aljabar"}
	if len(board.Recent) != len(wantRecent) {
		t.Fatalf("Recent = %d rows, want %d: %+v", len(board.Recent), len(wantRecent), board.Recent)
	}
	for i, want := range wantRecent {
		if board.Recent[i].Title != want {
			t.Errorf("Recent[%d] = %q, want %q", i, board.Recent[i].Title, want)
		}
	}
	if board.Recent[0].State != viewmodel.QuizDraft {
		t.Errorf("Recent[0].State = %q, want draft", board.Recent[0].State)
	}
	if board.Recent[0].QuestionCount != 0 || board.Recent[0].Joined != 1 {
		t.Errorf("Recent[0] counts = (%d, %d), want (0, 1)",
			board.Recent[0].QuestionCount, board.Recent[0].Joined)
	}

	// ATT-164: most and least over open/closed only; ties by LOWER(title).
	if got := participationTitles(board.MostJoined); !equalStrings(got, []string{"Aljabar", "Biologi", "Ekonomi"}) {
		t.Errorf("MostJoined = %v, want [Aljabar Biologi Ekonomi]", got)
	}
	if got := participationTitles(board.LeastJoined); !equalStrings(got, []string{"Biologi", "Ekonomi", "Fisika"}) {
		t.Errorf("LeastJoined = %v, want [Biologi Ekonomi Fisika]", got)
	}

	// A different teacher sees only their own quiz, never another teacher's
	// data (ATT-161).
	otherBoard, err := f.svc.TeacherDashboard(f.ctx, f.otherTeacherID, "Guru Lain")
	if err != nil {
		t.Fatalf("other TeacherDashboard: %v", err)
	}
	if len(otherBoard.TopScores) != 1 || otherBoard.TopScores[0].Quiz.Title != "Punyanya Lain" ||
		otherBoard.TopScores[0].StudentName != "Ayu Lestari" {
		t.Errorf("foreign TopScores = %+v, want only the other teacher's quiz", otherBoard.TopScores)
	}
	if len(otherBoard.Recent) != 1 || otherBoard.Recent[0].Title != "Punyanya Lain" {
		t.Errorf("foreign Recent = %+v, want only the other teacher's quiz", otherBoard.Recent)
	}
	if len(otherBoard.MostJoined) != 1 || otherBoard.MostJoined[0].Quiz.Title != "Punyanya Lain" {
		t.Errorf("foreign MostJoined = %+v", otherBoard.MostJoined)
	}
}

func participationTitles(rows []viewmodel.QuizParticipation) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Quiz.Title)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// registrationID finds the live registration of one student in one quiz.
func (f *teacherFixture) registrationID(t *testing.T, quizID, studentID int64) int64 {
	t.Helper()
	var id int64
	if err := f.db.QueryRowContext(f.ctx, `
		SELECT id FROM registrations
		WHERE quiz_id = ? AND student_id = ? AND deleted_at = ?`,
		quizID, studentID, store.LiveAt).Scan(&id); err != nil {
		t.Fatalf("registration (%d, %d): %v", quizID, studentID, err)
	}
	return id
}
