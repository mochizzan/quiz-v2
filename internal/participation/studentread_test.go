package participation

// studentread_test.go covers the PAR read-model projections without a database
// (builders are pure) plus one integration pass over the three service methods.
// The builders take a snapshot value, so list order, caps, score visibility and
// the empty-state copy are asserted exactly; the integration test proves the
// loaders compose against the real schema.

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

var readTestNow = time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)

func readTestService() *Service {
	svc := New(nil, []byte("student-read-test-key"))
	svc.nowFn = func() time.Time { return readTestNow }
	return svc
}

func readTestAgo(d time.Duration) time.Time { return readTestNow.Add(-d) }

func readNullTime(t time.Time) sql.NullTime { return sql.NullTime{Time: t, Valid: true} }

func readNullScore(v float64) sql.NullFloat64 { return sql.NullFloat64{Float64: v, Valid: true} }

func readQuiz(id int64, title, state string, showScore bool) quizRow {
	return quizRow{
		ID:          id,
		Title:       title,
		TeacherName: "Bu Guru",
		Visibility:  "public",
		State:       state,
		AllowRetake: true,
		MaxAttempts: 3,
		ShowScore:   showScore,
		ReviewMode:  "questions_no_answers",
	}
}

func readRegistration(id, quizID int64, created time.Time) registrationRow {
	return registrationRow{
		ID:        id,
		QuizID:    quizID,
		StudentID: sql.NullInt64{Int64: 11, Valid: true},
		CreatedAt: created,
	}
}

func readAttempt(id int64, number int, submitted sql.NullTime, score sql.NullFloat64) attemptRecord {
	at := readTestNow.Add(-time.Hour)
	if submitted.Valid {
		at = submitted.Time
	}
	return attemptRecord{
		ID:         id,
		Number:     number,
		CreatedAt:  at.Add(-time.Hour),
		UpdatedAt:  at,
		Submitted:  submitted,
		FinalScore: score,
	}
}

func requireScore(t *testing.T, got *viewmodel.Score, want int) {
	t.Helper()
	if got == nil {
		t.Fatalf("score = nil, want %d%%", want)
	}
	if got.Percent != want {
		t.Fatalf("score = %d%%, want %d%%", got.Percent, want)
	}
}

func TestBuildStudentDashboard(t *testing.T) {
	svc := readTestService()

	quizzes := map[int64]quizRow{
		2:  readQuiz(2, "Live Dua", "open", true),
		3:  readQuiz(3, "Selesai Tiga", "closed", true),
		4:  readQuiz(4, "Tutup Empat", "closed", false),
		5:  readQuiz(5, "Selesai Lima", "open", true),
		7:  readQuiz(7, "Baru Tujuh", "open", true),
		8:  readQuiz(8, "Baru Delapan", "open", true),
		9:  readQuiz(9, "Baru Sembilan", "open", true),
		10: readQuiz(10, "Baru Sepuluh", "open", true),
		11: readQuiz(11, "Baru Sebelas", "open", true),
		12: readQuiz(12, "Baru Dua Belas", "open", true),
		13: readQuiz(13, "Baru Tiga Belas", "open", true),
		15: readQuiz(15, "Live Lima Belas", "open", true),
	}
	siap := readQuiz(1, "Siap Satu", "open", true)
	siap.MaxAttempts = 1
	quizzes[1] = siap
	siapLain := readQuiz(14, "Siap Empat Belas", "open", false)
	siapLain.MaxAttempts = 1
	quizzes[14] = siapLain

	registrations := []registrationRow{
		readRegistration(1, 1, readTestAgo(2*time.Hour)),
		readRegistration(2, 2, readTestAgo(3*time.Hour)),
		readRegistration(3, 3, readTestAgo(4*time.Hour)),
		readRegistration(4, 4, readTestAgo(5*time.Hour)),
		readRegistration(5, 5, readTestAgo(6*time.Hour)),
		readRegistration(7, 14, readTestAgo(30*time.Minute)),
		readRegistration(8, 15, readTestAgo(45*time.Minute)),
		readRegistration(9, 999, readTestAgo(7*time.Hour)), // soft-deleted quiz: skipped
	}

	closedAttempts := make([]attemptRecord, 0, 6)
	for i := 0; i < 6; i++ {
		hours := 6 - i
		closedAttempts = append(closedAttempts, readAttempt(
			int64(301+i), i+1, readNullTime(readTestAgo(time.Duration(hours)*time.Hour)), readNullScore(0.1+float64(i)*0.1)))
	}
	attempts := map[int64][]attemptRecord{
		2: {{
			ID:        201,
			Number:    1,
			CreatedAt: readTestAgo(40 * time.Minute),
			UpdatedAt: readTestAgo(10 * time.Minute),
		}},
		3: closedAttempts,
		5: {readAttempt(501, 1, readNullTime(readTestAgo(30*time.Minute)), readNullScore(1))},
		8: {{
			ID:        801,
			Number:    1,
			CreatedAt: readTestAgo(50 * time.Minute),
			UpdatedAt: readTestAgo(5 * time.Minute),
		}},
	}

	assigned := []quizRow{quizzes[1], quizzes[13], quizzes[7], quizzes[8], quizzes[9], quizzes[10], quizzes[11], quizzes[12]}
	counts := map[int64]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 7: 2, 8: 2, 9: 2, 10: 2, 11: 2, 12: 2, 13: 0, 14: 2, 15: 1}

	dash := svc.buildStudentDashboard(studentSnapshot{
		fullName:       "Ayu Lestari",
		assigned:       assigned,
		registrations:  registrations,
		quizzes:        quizzes,
		attempts:       attempts,
		questionCounts: counts,
	})

	if dash.GreetingName != "Ayu" {
		t.Errorf("GreetingName = %q, want Ayu", dash.GreetingName)
	}
	if dash.DateLine == "" {
		t.Error("DateLine is empty")
	}
	if dash.Stats.Assigned != 14 {
		t.Errorf("Assigned = %d, want 14", dash.Stats.Assigned)
	}
	if dash.Stats.Registered != 2 || dash.Stats.InProgress != 2 || dash.Stats.Completed != 2 {
		t.Errorf("stats = registered %d / in-progress %d / completed %d, want 2/2/2",
			dash.Stats.Registered, dash.Stats.InProgress, dash.Stats.Completed)
	}
	requireScore(t, dash.Stats.BestScore, 100)

	if len(dash.Ready) != 2 {
		t.Fatalf("Ready len = %d, want 2", len(dash.Ready))
	}
	if dash.Ready[0].ID != 14 || dash.Ready[1].ID != 1 {
		t.Errorf("Ready order = [%d %d], want [14 1]", dash.Ready[0].ID, dash.Ready[1].ID)
	}
	if dash.Ready[0].Primary.Label != "Mulai kuis" || dash.Ready[0].Primary.Href != "/quiz/14" {
		t.Errorf("Ready[0] primary = %+v", dash.Ready[0].Primary)
	}
	if dash.Ready[0].Participation != viewmodel.ParticipationRegistered || dash.Ready[0].AttemptsUsed != 0 {
		t.Errorf("Ready[0] participation = %v attempts = %d", dash.Ready[0].Participation, dash.Ready[0].AttemptsUsed)
	}
	if dash.Ready[0].JoinedAt == nil {
		t.Error("Ready[0].JoinedAt = nil")
	}

	if len(dash.Live) != 2 {
		t.Fatalf("Live len = %d, want 2", len(dash.Live))
	}
	if dash.Live[0].ID != 15 || dash.Live[1].ID != 2 {
		t.Errorf("Live order = [%d %d], want [15 2]", dash.Live[0].ID, dash.Live[1].ID)
	}
	if dash.Live[0].Primary.Href != "/attempt/801" || dash.Live[1].Primary.Href != "/attempt/201" {
		t.Errorf("Live primary hrefs = %q, %q", dash.Live[0].Primary.Href, dash.Live[1].Primary.Href)
	}
	if dash.Live[0].Participation != viewmodel.ParticipationInProgress || dash.Live[0].AttemptsUsed != 1 {
		t.Errorf("Live[0] participation = %v attempts = %d", dash.Live[0].Participation, dash.Live[0].AttemptsUsed)
	}

	if len(dash.Latest) != 6 {
		t.Fatalf("Latest len = %d, want 6 (cap)", len(dash.Latest))
	}
	for i, want := range []int64{7, 8, 9, 10, 11, 12} {
		if dash.Latest[i].ID != want {
			t.Errorf("Latest[%d].ID = %d, want %d", i, dash.Latest[i].ID, want)
		}
	}
	if dash.Latest[0].QuestionCount != 2 {
		t.Errorf("Latest[0].QuestionCount = %d, want 2", dash.Latest[0].QuestionCount)
	}
	if dash.Latest[0].Primary.Label != "Lihat kuis" || dash.Latest[0].Primary.Href != "/quiz/7" {
		t.Errorf("Latest[0] primary = %+v", dash.Latest[0].Primary)
	}

	if len(dash.Recent) != 5 {
		t.Fatalf("Recent len = %d, want 5 (cap)", len(dash.Recent))
	}
	for i, want := range []int64{501, 306, 305, 304, 303} {
		if dash.Recent[i].AttemptID != want {
			t.Errorf("Recent[%d].AttemptID = %d, want %d", i, dash.Recent[i].AttemptID, want)
		}
	}
	requireScore(t, dash.Recent[0].Score, 100)
	requireScore(t, dash.Recent[1].Score, 60)
	if dash.Recent[0].Href != "/attempt/501/result" {
		t.Errorf("Recent[0].Href = %q", dash.Recent[0].Href)
	}
	if dash.Recent[0].SubmittedAt == nil {
		t.Error("Recent[0].SubmittedAt = nil")
	}
	if dash.Empty.Title != "" {
		t.Errorf("Empty = %q, want zero value", dash.Empty.Title)
	}
}

func TestBuildStudentDashboardEmptyStates(t *testing.T) {
	svc := readTestService()

	empty := svc.buildStudentDashboard(studentSnapshot{})
	if empty.Empty.Title != "Belum ada kuis untuk Anda." {
		t.Errorf("empty title = %q", empty.Empty.Title)
	}
	if empty.Empty.Body != "Kuis baru akan muncul di sini setelah guru membukanya untuk kelas Anda." {
		t.Errorf("empty body = %q", empty.Empty.Body)
	}
	if empty.Empty.Icon != "quiz" || empty.Empty.Action == nil {
		t.Fatalf("empty icon/action = %q/%v", empty.Empty.Icon, empty.Empty.Action)
	}
	if empty.Empty.Action.Label != "Gabung dengan kode" || empty.Empty.Action.Href != "/join" ||
		empty.Empty.Action.Style != "primary" {
		t.Errorf("empty action = %+v", *empty.Empty.Action)
	}

	// PAR-137: a dead closed registration keeps Assigned at one, so the page
	// renders stats even though every list is empty.
	dead := svc.buildStudentDashboard(studentSnapshot{
		registrations: []registrationRow{readRegistration(1, 4, readTestAgo(time.Hour))},
		quizzes:       map[int64]quizRow{4: readQuiz(4, "Tutup", "closed", true)},
		attempts:      map[int64][]attemptRecord{},
	})
	if dead.Stats.Assigned != 1 {
		t.Errorf("dead Assigned = %d, want 1", dead.Stats.Assigned)
	}
	if dead.Empty.Title != "" {
		t.Errorf("dead Empty = %q, want no empty state", dead.Empty.Title)
	}
}

func TestBuildStudentDashboardScoreVisibility(t *testing.T) {
	svc := readTestService()

	hidden := readQuiz(1, "Tersembunyi", "closed", false)
	waiting := readQuiz(2, "Menunggu", "closed", true)
	visible := readQuiz(3, "Terlihat", "closed", true)

	dash := svc.buildStudentDashboard(studentSnapshot{
		registrations: []registrationRow{
			readRegistration(1, 1, readTestAgo(3*time.Hour)),
			readRegistration(2, 2, readTestAgo(2*time.Hour)),
			readRegistration(3, 3, readTestAgo(time.Hour)),
		},
		quizzes: map[int64]quizRow{1: hidden, 2: waiting, 3: visible},
		attempts: map[int64][]attemptRecord{
			1: {readAttempt(101, 1, readNullTime(readTestAgo(50*time.Minute)), readNullScore(0.9))},
			2: {readAttempt(201, 1, readNullTime(readTestAgo(40*time.Minute)), sql.NullFloat64{})},
			3: {
				readAttempt(301, 1, readNullTime(readTestAgo(30*time.Minute)), readNullScore(1.5)),
				readAttempt(302, 2, readNullTime(readTestAgo(20*time.Minute)), readNullScore(0.25)),
			},
		},
	})

	requireScore(t, dash.Stats.BestScore, 25)
	if len(dash.Recent) != 4 {
		t.Fatalf("Recent len = %d, want 4", len(dash.Recent))
	}
	// Newest first: visible 0.25, visible out-of-range, waiting, hidden.
	if dash.Recent[0].AttemptID != 302 || dash.Recent[1].AttemptID != 301 ||
		dash.Recent[2].AttemptID != 201 || dash.Recent[3].AttemptID != 101 {
		t.Errorf("Recent order = %d %d %d %d", dash.Recent[0].AttemptID, dash.Recent[1].AttemptID,
			dash.Recent[2].AttemptID, dash.Recent[3].AttemptID)
	}
	if dash.Recent[0].Score == nil || dash.Recent[0].Score.Percent != 25 {
		t.Errorf("visible score = %+v, want 25%%", dash.Recent[0].Score)
	}
	if dash.Recent[1].Score != nil {
		t.Errorf("out-of-range score = %+v, want nil", *dash.Recent[1].Score)
	}
	if !dash.Recent[2].Waiting || dash.Recent[2].Score != nil {
		t.Errorf("waiting row = waiting %v score %+v", dash.Recent[2].Waiting, dash.Recent[2].Score)
	}
	if dash.Recent[3].ShowScore || dash.Recent[3].Score != nil {
		t.Errorf("hidden row = show %v score %+v", dash.Recent[3].ShowScore, dash.Recent[3].Score)
	}
}

func TestBuildAttemptRowsVisibility(t *testing.T) {
	svc := readTestService()
	records := []attemptRecord{
		{ID: 1, Number: 1, CreatedAt: readTestAgo(3 * time.Hour), UpdatedAt: readTestAgo(2 * time.Hour)},
		readAttempt(2, 2, readNullTime(readTestAgo(time.Hour)), sql.NullFloat64{}),
		readAttempt(3, 3, readNullTime(readTestAgo(time.Hour)), readNullScore(0)),
		readAttempt(4, 4, readNullTime(readTestAgo(time.Hour)), readNullScore(1.5)),
		readAttempt(5, 5, readNullTime(readTestAgo(time.Hour)), readNullScore(0.5)),
	}

	shown := svc.buildAttemptRows(records, true)
	if len(shown) != 5 {
		t.Fatalf("shown len = %d", len(shown))
	}
	if shown[0].State != viewmodel.AttemptInProgress || shown[0].ResultHref != "/attempt/1" {
		t.Errorf("in-progress row = %v %q", shown[0].State, shown[0].ResultHref)
	}
	if !shown[1].Waiting || shown[1].Score != nil {
		t.Errorf("NULL submitted row = waiting %v score %+v", shown[1].Waiting, shown[1].Score)
	}
	requireScore(t, shown[2].Score, 0)
	if shown[3].Score != nil {
		t.Errorf("out-of-range row score = %+v, want nil", *shown[3].Score)
	}
	requireScore(t, shown[4].Score, 50)
	if shown[2].ResultHref != "/attempt/3/result" {
		t.Errorf("submitted result href = %q", shown[2].ResultHref)
	}

	hidden := svc.buildAttemptRows(records, false)
	for i, row := range hidden {
		if row.Score != nil {
			t.Errorf("hidden row %d score = %+v, want nil", i, *row.Score)
		}
	}
	if !hidden[1].Waiting {
		t.Error("hidden waiting row lost its waiting flag")
	}
}

func TestBuildStudentHistory(t *testing.T) {
	svc := readTestService()

	registrations := make([]registrationRow, 0, 13)
	quizzes := map[int64]quizRow{}
	for i := int64(1); i <= 12; i++ {
		registrations = append(registrations, readRegistration(i, i, readTestAgo(time.Duration(13-i)*time.Hour)))
		quizzes[i] = readQuiz(i, fmt.Sprintf("Kuis %02d", i), "closed", true)
	}
	registrations = append(registrations, readRegistration(13, 999, readTestNow))

	snap := studentSnapshot{
		registrations: registrations,
		quizzes:       quizzes,
		attempts: map[int64][]attemptRecord{
			5: {
				readAttempt(501, 1, readNullTime(readTestAgo(24*time.Hour)), readNullScore(0.8)),
				{ID: 502, Number: 2, CreatedAt: readTestAgo(30 * time.Hour), UpdatedAt: readTestAgo(30 * time.Hour)},
				readAttempt(504, 3, readNullTime(readTestAgo(23*time.Hour)), readNullScore(1.5)),
			},
		},
	}

	first := svc.buildStudentHistory(snap, "1")
	if first.Rows.Total != 12 || first.Rows.Page != 1 || len(first.Rows.Items) != 10 {
		t.Fatalf("page 1 = total %d page %d len %d", first.Rows.Total, first.Rows.Page, len(first.Rows.Items))
	}
	if first.Rows.Items[0].Quiz.ID != 12 || first.Rows.Items[9].Quiz.ID != 3 {
		t.Errorf("page 1 order = first %d last %d", first.Rows.Items[0].Quiz.ID, first.Rows.Items[9].Quiz.ID)
	}
	if first.Pagination.Total != 12 || first.Pagination.Pages != 2 || first.Pagination.Page != 1 {
		t.Errorf("pagination = %+v", first.Pagination)
	}

	row := first.Rows.Items[7] // quiz 5
	if row.Quiz.ID != 5 {
		t.Fatalf("row quiz = %d, want 5", row.Quiz.ID)
	}
	if row.State != viewmodel.ParticipationInProgress {
		t.Errorf("row state = %v, want in progress", row.State)
	}
	requireScore(t, row.BestScore, 80)
	if row.LastAttempt == nil {
		t.Error("row LastAttempt = nil")
	}
	if len(row.Attempts) != 3 {
		t.Fatalf("row attempts = %d, want 3", len(row.Attempts))
	}
	requireScore(t, row.Attempts[0].Score, 80)
	if row.Attempts[1].Score != nil {
		t.Errorf("open attempt score = %+v, want nil", *row.Attempts[1].Score)
	}
	if row.Attempts[2].Score != nil {
		t.Errorf("out-of-range attempt score = %+v, want nil", *row.Attempts[2].Score)
	}
	if row.DetailHref != "/student/history/5" {
		t.Errorf("DetailHref = %q", row.DetailHref)
	}

	for _, raw := range []string{"99", "-5", "abc", ""} {
		got := svc.buildStudentHistory(snap, raw)
		wantPage := 1
		if raw == "99" {
			wantPage = 2
		}
		if got.Rows.Page != wantPage {
			t.Errorf("page %q = %d, want %d", raw, got.Rows.Page, wantPage)
		}
		if raw == "99" && (len(got.Rows.Items) != 2 || got.Rows.Items[0].Quiz.ID != 2) {
			t.Errorf("page 99 items = %d first %d", len(got.Rows.Items), got.Rows.Items[0].Quiz.ID)
		}
	}
}

func TestBuildStudentHistoryEmpty(t *testing.T) {
	svc := readTestService()
	page := svc.buildStudentHistory(studentSnapshot{}, "")
	if page.Rows.Total != 0 || !page.Rows.Empty() {
		t.Fatalf("empty history = total %d items %d", page.Rows.Total, len(page.Rows.Items))
	}
	if page.Empty.Title != "Belum ada riwayat kuis." {
		t.Errorf("empty title = %q", page.Empty.Title)
	}
	if page.Empty.Body != "Kuis yang Anda ikuti akan tercatat di sini beserta setiap percobaannya." {
		t.Errorf("empty body = %q", page.Empty.Body)
	}
	if page.Empty.Icon != "history" || page.Empty.Action == nil {
		t.Fatalf("empty icon/action = %q/%v", page.Empty.Icon, page.Empty.Action)
	}
	if page.Empty.Action.Label != "Gabung kuis" || page.Empty.Action.Href != "/join" ||
		page.Empty.Action.Style != "outline" {
		t.Errorf("empty action = %+v", *page.Empty.Action)
	}
}

func TestBuildStudentHistoryDetail(t *testing.T) {
	svc := readTestService()
	actor := Actor{Kind: ActorStudent, StudentID: 5, GradeLevel: 11, MajorID: 2}

	base := readQuiz(42, "Kuis Detail", "open", true)
	base.TeacherName = "Bu Guru"
	base.AllowRetake = false
	base.MaxAttempts = 2

	submitted := []attemptRecord{
		readAttempt(701, 1, readNullTime(readTestAgo(time.Hour)), readNullScore(0.75)),
	}
	open := []attemptRecord{{
		ID: 702, Number: 1, CreatedAt: readTestAgo(time.Hour), UpdatedAt: readTestAgo(30 * time.Minute),
	}}

	closed := base
	closed.State = "closed"
	retake := base
	retake.AllowRetake = true
	quota := base
	quota.MaxAttempts = 1
	quota.AllowRetake = true
	privateOff := base
	privateOff.Visibility = "private"
	privateOff.TargetGrade = sql.NullInt64{Int64: 10, Valid: true}
	privateOff.TargetMajor = sql.NullInt64{Int64: 2, Valid: true}
	privateOff.TargetMajorName = "IPA"
	privateOn := privateOff
	privateOn.TargetGrade = sql.NullInt64{Int64: 11, Valid: true}
	invalidQuota := base
	invalidQuota.MaxAttempts = 0
	invalidQuota.AllowRetake = false

	cases := []struct {
		name    string
		quiz    quizRow
		records []attemptRecord
		actor   Actor
		want    bool
	}{
		{"open zero attempts offers the first attempt", base, nil, actor, true},
		{"open attempt blocks retry", base, open, actor, false},
		{"closed quiz blocks retry", closed, nil, actor, false},
		{"spent quota blocks retry", quota, submitted, actor, false},
		{"no-retake single attempt blocks retry", base, submitted, actor, false},
		{"allow-retake below quota permits retry", retake, submitted, actor, true},
		{"ineligible private quiz blocks retry", privateOff, nil, actor, false},
		{"eligible private quiz permits retry", privateOn, nil, actor, true},
		{"invalid quota fails closed at one", invalidQuota, nil, actor, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			detail := svc.buildStudentHistoryDetail(tc.actor, tc.quiz, tc.records)
			if detail.CanRetry != tc.want {
				t.Errorf("CanRetry = %v, want %v", detail.CanRetry, tc.want)
			}
			// The quiz page is 404 once the quiz closes (PAR-088), so the
			// detail page only offers it while the quiz is open.
			wantHref := ""
			if tc.quiz.State == "open" {
				wantHref = quizPath(uint32(tc.quiz.ID))
			}
			if detail.RetryHref != wantHref {
				t.Errorf("RetryHref = %q, want %q for state %q", detail.RetryHref, wantHref, tc.quiz.State)
			}
		})
	}

	detail := svc.buildStudentHistoryDetail(actor, base, nil)
	if detail.State != viewmodel.ParticipationRegistered {
		t.Errorf("state = %v, want registered", detail.State)
	}
	if detail.MaxAttempts != 1 {
		t.Errorf("MaxAttempts = %d, want normalized 1", detail.MaxAttempts)
	}
	if detail.RetryHref != "/quiz/42" || detail.BackHref != "/student/history" {
		t.Errorf("hrefs = %q / %q", detail.RetryHref, detail.BackHref)
	}
	if detail.Audience != "Semua kelas" {
		t.Errorf("Audience = %q", detail.Audience)
	}
	if detail.Quiz.Title != "Kuis Detail" || detail.TeacherName != "Bu Guru" {
		t.Errorf("identity = %+v / %q", detail.Quiz, detail.TeacherName)
	}

	shown := svc.buildStudentHistoryDetail(actor, base, submitted)
	if shown.State != viewmodel.ParticipationCompleted || len(shown.Attempts) != 1 {
		t.Errorf("completed detail = %v with %d attempts", shown.State, len(shown.Attempts))
	}
	requireScore(t, shown.Attempts[0].Score, 75)
	if shown.Attempts[0].MaxAttempts != 1 {
		t.Errorf("attempt row MaxAttempts = %d, want normalized 1", shown.Attempts[0].MaxAttempts)
	}

	hiddenQuiz := base
	hiddenQuiz.ShowScore = false
	hidden := svc.buildStudentHistoryDetail(actor, hiddenQuiz, submitted)
	if hidden.Attempts[0].Score != nil || hidden.ShowScore {
		t.Errorf("hidden detail score = %+v show = %v", hidden.Attempts[0].Score, hidden.ShowScore)
	}
}

// ---- Integration ----------------------------------------------------------

type readQuizSeed struct {
	owner       int64
	title       string
	visibility  string
	state       string
	code        any
	invite      string
	targetGrade any
	targetMajor any
	published   any
	closed      any
	allowRetake bool
	maxAttempts int
	showScore   bool
}

func seedReadAccount(t *testing.T, db *store.DB, email, name, role string) int64 {
	t.Helper()
	var approved any
	if role == "teacher" {
		approved = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	result, err := db.Exec(`INSERT INTO accounts
		(full_name, email, password_hash, role, status, approved_at)
		VALUES (?, ?, 'not-a-real-hash', ?, 'active', ?)`, name, email, role, approved)
	if err != nil {
		t.Fatalf("seed account %s: %v", email, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("seed account id: %v", err)
	}
	return id
}

func seedReadMajor(t *testing.T, db *store.DB, name, code string) int64 {
	t.Helper()
	result, err := db.Exec("INSERT INTO majors (name, code) VALUES (?, ?)", name, code)
	if err != nil {
		t.Fatalf("seed major %s: %v", name, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("seed major id: %v", err)
	}
	return id
}

func seedReadStudent(t *testing.T, db *store.DB, accountID, majorID int64, grade int) int64 {
	t.Helper()
	result, err := db.Exec("INSERT INTO students (account_id, grade_level, major_id) VALUES (?, ?, ?)",
		accountID, grade, majorID)
	if err != nil {
		t.Fatalf("seed student: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("seed student id: %v", err)
	}
	return id
}

func seedReadQuiz(t *testing.T, db *store.DB, in readQuizSeed) int64 {
	t.Helper()
	result, err := db.Exec(`INSERT INTO quizzes
		(owner_account_id, title, visibility, state, code, invite_token,
		 target_grade, target_major_id, allow_retake, max_attempts, show_score,
		 review_mode, published_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'questions_no_answers', ?, ?)`,
		in.owner, in.title, in.visibility, in.state, in.code, in.invite,
		in.targetGrade, in.targetMajor, in.allowRetake, in.maxAttempts, in.showScore,
		in.published, in.closed)
	if err != nil {
		t.Fatalf("seed quiz %s: %v", in.title, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("seed quiz id: %v", err)
	}
	return id
}

func seedReadQuestions(t *testing.T, db *store.DB, quizID int64, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if _, err := db.Exec(
			"INSERT INTO questions (quiz_id, position, type, prompt) VALUES (?, ?, 'essay', ?)",
			quizID, i, fmt.Sprintf("Soal nomor %d", i)); err != nil {
			t.Fatalf("seed question %d for quiz %d: %v", i, quizID, err)
		}
	}
}

func seedReadRegistration(t *testing.T, db *store.DB, quizID, studentID int64, created time.Time) int64 {
	t.Helper()
	result, err := db.Exec(
		"INSERT INTO registrations (quiz_id, student_id, created_at) VALUES (?, ?, ?)",
		quizID, studentID, created)
	if err != nil {
		t.Fatalf("seed registration quiz %d: %v", quizID, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("seed registration id: %v", err)
	}
	return id
}

func seedReadAttempt(t *testing.T, db *store.DB, regID int64, number int, created, updated time.Time, submitted any, score any) int64 {
	t.Helper()
	result, err := db.Exec(`INSERT INTO quiz_attempts
		(registration_id, attempt_no, submitted_at, final_score, shuffle_seed, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?)`, regID, number, submitted, score, created, updated)
	if err != nil {
		t.Fatalf("seed attempt %d for registration %d: %v", number, regID, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("seed attempt id: %v", err)
	}
	return id
}

func TestStudentReadSurfacesIntegration(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	ctx := context.Background()
	svc := readTestService()
	svc.db = db

	teacher := seedReadAccount(t, db, "guru@example.test", "Guru Pengampu", "teacher")
	major := seedReadMajor(t, db, "Ilmu Pengetahuan Alam", "IPA")
	studentAccount := seedReadAccount(t, db, "siswa@example.test", "Ayu Lestari", "student")
	student := seedReadStudent(t, db, studentAccount, major, 11)
	actor := ActorInput{AccountID: studentAccount, Role: viewmodel.RoleStudent, FullName: "Ayu Lestari"}

	publicOpen := func(title, code string, published time.Time, allowRetake bool, maxAttempts int, showScore bool) readQuizSeed {
		return readQuizSeed{
			owner: teacher, title: title, visibility: "public", state: "open",
			code: code, invite: "invite-" + code, published: published,
			allowRetake: allowRetake, maxAttempts: maxAttempts, showScore: showScore,
		}
	}

	q1 := seedReadQuiz(t, db, publicOpen("Kuis Satu", "111111", readTestAgo(10*24*time.Hour), true, 2, true))
	q2 := seedReadQuiz(t, db, readQuizSeed{
		owner: teacher, title: "Kuis Dua", visibility: "public", state: "closed",
		code: "222222", invite: "invite-222222", published: readTestAgo(20 * 24 * time.Hour),
		closed: readTestAgo(24 * time.Hour), allowRetake: false, maxAttempts: 1, showScore: true,
	})
	q3 := seedReadQuiz(t, db, publicOpen("Kuis Tiga", "333333", readTestAgo(24*time.Hour), false, 1, true))
	// q4 is open and eligible but has no live questions: PAR-134 hides it.
	seedReadQuiz(t, db, publicOpen("Kuis Empat", "444444", readTestAgo(2*24*time.Hour), false, 1, true))
	q5 := seedReadQuiz(t, db, readQuizSeed{
		owner: teacher, title: "Kuis Lima", visibility: "private", state: "open",
		invite: "invite-lima", targetGrade: 11, targetMajor: major,
		published: readTestAgo(3 * 24 * time.Hour), allowRetake: true, maxAttempts: 2, showScore: false,
	})
	seedReadQuiz(t, db, readQuizSeed{
		owner: teacher, title: "Kuis Enam", visibility: "private", state: "open",
		invite: "invite-enam", targetGrade: 10, targetMajor: major,
		published: readTestAgo(4 * 24 * time.Hour), allowRetake: true, maxAttempts: 2, showScore: true,
	})

	seedReadQuestions(t, db, q1, 2)
	seedReadQuestions(t, db, q2, 1)
	seedReadQuestions(t, db, q3, 1)
	seedReadQuestions(t, db, q5, 1)

	r1 := seedReadRegistration(t, db, q1, student, readTestAgo(5*time.Hour))
	r2 := seedReadRegistration(t, db, q2, student, readTestAgo(6*time.Hour))
	seedReadAttempt(t, db, r1, 1, readTestAgo(5*time.Hour), readTestAgo(5*time.Hour),
		readTestAgo(4*time.Hour), 0.5)
	seedReadAttempt(t, db, r1, 2, readTestAgo(2*time.Hour), readTestAgo(time.Hour), nil, nil)
	seedReadAttempt(t, db, r2, 1, readTestAgo(3*time.Hour), readTestAgo(2*time.Hour),
		readTestAgo(2*time.Hour), 0.75)

	dash, err := svc.StudentDashboard(ctx, StudentDashboardInput{Actor: actor})
	if err != nil {
		t.Fatalf("StudentDashboard: %v", err)
	}
	if dash.GreetingName != "Ayu" {
		t.Errorf("GreetingName = %q", dash.GreetingName)
	}
	if dash.Stats.Assigned != 5 || dash.Stats.Registered != 0 ||
		dash.Stats.InProgress != 1 || dash.Stats.Completed != 1 {
		t.Errorf("stats = %+v", dash.Stats)
	}
	requireScore(t, dash.Stats.BestScore, 75)
	if len(dash.Ready) != 0 {
		t.Errorf("Ready = %d, want 0", len(dash.Ready))
	}
	if len(dash.Live) != 1 || dash.Live[0].ID != q1 || dash.Live[0].QuestionCount != 2 {
		t.Errorf("Live = %+v", dash.Live)
	}
	if len(dash.Latest) != 2 || dash.Latest[0].ID != q3 || dash.Latest[1].ID != q5 {
		t.Errorf("Latest = %+v", dash.Latest)
	}
	if len(dash.Recent) != 2 || dash.Recent[0].Quiz.ID != q2 || dash.Recent[1].Quiz.ID != q1 {
		t.Errorf("Recent = %+v", dash.Recent)
	}
	requireScore(t, dash.Recent[0].Score, 75)
	requireScore(t, dash.Recent[1].Score, 50)

	history, err := svc.StudentHistory(ctx, StudentHistoryInput{Actor: actor, Page: "1"})
	if err != nil {
		t.Fatalf("StudentHistory: %v", err)
	}
	if history.Rows.Total != 2 || len(history.Rows.Items) != 2 {
		t.Fatalf("history = total %d items %d", history.Rows.Total, len(history.Rows.Items))
	}
	first, second := history.Rows.Items[0], history.Rows.Items[1]
	if first.Quiz.ID != q1 || second.Quiz.ID != q2 {
		t.Errorf("history order = %d %d", first.Quiz.ID, second.Quiz.ID)
	}
	if first.State != viewmodel.ParticipationInProgress || second.State != viewmodel.ParticipationCompleted {
		t.Errorf("history states = %v %v", first.State, second.State)
	}
	requireScore(t, first.BestScore, 50)
	requireScore(t, second.BestScore, 75)
	if first.LastAttempt == nil || len(first.Attempts) != 2 {
		t.Errorf("first row last = %v attempts = %d", first.LastAttempt, len(first.Attempts))
	}
	if history.Pagination.Total != 2 || history.Pagination.Page != 1 {
		t.Errorf("pagination = %+v", history.Pagination)
	}

	detail, err := svc.StudentHistoryDetail(ctx, StudentHistoryDetailInput{Actor: actor, QuizID: q1})
	if err != nil {
		t.Fatalf("StudentHistoryDetail: %v", err)
	}
	if detail.State != viewmodel.ParticipationInProgress || detail.CanRetry {
		t.Errorf("detail state = %v canRetry = %v", detail.State, detail.CanRetry)
	}
	if detail.MaxAttempts != 2 || detail.RetryHref != fmt.Sprintf("/quiz/%d", q1) ||
		detail.BackHref != "/student/history" {
		t.Errorf("detail = max %d retry %q back %q", detail.MaxAttempts, detail.RetryHref, detail.BackHref)
	}
	if len(detail.Attempts) != 2 {
		t.Fatalf("detail attempts = %d", len(detail.Attempts))
	}
	if detail.Attempts[0].Number != 1 || detail.Attempts[1].Number != 2 {
		t.Errorf("attempt order = %d %d", detail.Attempts[0].Number, detail.Attempts[1].Number)
	}
	requireScore(t, detail.Attempts[0].Score, 50)
	if detail.Attempts[0].MaxAttempts != 2 {
		t.Errorf("attempt row MaxAttempts = %d, want 2", detail.Attempts[0].MaxAttempts)
	}
	if detail.Attempts[1].Score != nil || detail.Attempts[1].State != viewmodel.AttemptInProgress {
		t.Errorf("open attempt = %+v", detail.Attempts[1])
	}

	closedDetail, err := svc.StudentHistoryDetail(ctx, StudentHistoryDetailInput{Actor: actor, QuizID: q2})
	if err != nil {
		t.Fatalf("StudentHistoryDetail closed: %v", err)
	}
	if closedDetail.CanRetry || closedDetail.State != viewmodel.ParticipationCompleted {
		t.Errorf("closed detail = canRetry %v state %v", closedDetail.CanRetry, closedDetail.State)
	}

	if _, err := svc.StudentHistoryDetail(ctx, StudentHistoryDetailInput{Actor: actor, QuizID: q3}); !apperr.IsNotFound(err) {
		t.Errorf("unregistered quiz error = %v, want 404", err)
	}
	if _, err := svc.StudentHistoryDetail(ctx, StudentHistoryDetailInput{Actor: actor, QuizID: 0}); !apperr.IsNotFound(err) {
		t.Errorf("zero quiz id error = %v, want 404", err)
	}
}
