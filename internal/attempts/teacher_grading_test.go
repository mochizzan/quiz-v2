package attempts

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/viewmodel"
)

// gradingPerson groups the ids of one participant's seeded attempt.
type gradingPerson struct {
	studentID int64
	regID     int64
	attemptID int64
	singleID  int64
	essay1ID  int64
	essay2ID  int64
}

// gradingWorld is the shared grading fixture: one owned quiz with an objective
// slot and two essays, a second owned quiz, a foreign quiz, an in-progress
// attempt, a graded essay, and several pending essays.
type gradingWorld struct {
	f *teacherFixture

	quiz    int64
	quiz2   int64
	foreign int64

	singleQ int64
	essay1Q int64
	essay2Q int64
	optA    int64
	optB    int64

	ayu   gradingPerson
	budi  gradingPerson
	citra gradingPerson
	dodi  gradingPerson
	eka   gradingPerson
	fajar gradingPerson

	ekaGradedAt time.Time
}

func newGradingWorld(t *testing.T) *gradingWorld {
	t.Helper()
	f := newTeacherFixture(t)
	w := &gradingWorld{f: f}

	w.quiz = f.newQuiz(f.teacherID, "Ujian Esai", "open", true, false, 1, "2026-10-20 09:00:00")
	w.quiz2 = f.newQuiz(f.teacherID, "Kuis Kedua", "open", true, false, 1, "2026-10-20 09:30:00")
	w.foreign = f.newQuiz(f.otherTeacherID, "Punyanya Lain", "open", true, false, 1, "2026-10-20 08:00:00")

	w.singleQ = f.question(w.quiz, 1, "single", "Pilih satu")
	w.essay1Q = f.question(w.quiz, 2, "essay", "Jelaskan A")
	w.essay2Q = f.question(w.quiz, 3, "essay", "Jelaskan B")
	w.optA = f.option(w.singleQ, 1, "A", true)
	w.optB = f.option(w.singleQ, 2, "B", false)
	q2Essay := f.question(w.quiz2, 1, "essay", "Jelaskan C")
	qForeignEssay := f.question(w.foreign, 1, "essay", "Rahasia")

	// Ayu: submitted, two pending essays, one objective answer already scored.
	ayuStudent := f.newStudent("Ayu Lestari", 10)
	w.ayu.studentID = ayuStudent
	w.ayu.regID = f.register(w.quiz, ayuStudent, "2026-10-20 10:00:00")
	w.ayu.attemptID = f.attempt(w.ayu.regID, 1, 21, "2026-10-23 10:00:00", nil,
		"2026-10-23 09:00:00", "2026-10-23 10:00:00")
	w.ayu.singleID = f.answer(w.ayu.attemptID, w.singleQ, nil, 1.0, nil, nil, "2026-10-23 10:00:00")
	f.selectOption(w.ayu.singleID, w.optA)
	w.ayu.essay1ID = f.answer(w.ayu.attemptID, w.essay1Q, "Jawaban Ayu 1", nil, nil, nil, nil)
	w.ayu.essay2ID = f.answer(w.ayu.attemptID, w.essay2Q, "Jawaban Ayu 2", nil, nil, nil, nil)

	// Budi: newer submission, one pending essay.
	budiStudent := f.newStudent("Budi Santoso", 11)
	w.budi.studentID = budiStudent
	w.budi.regID = f.register(w.quiz, budiStudent, "2026-10-20 11:00:00")
	w.budi.attemptID = f.attempt(w.budi.regID, 1, 22, "2026-10-23 11:00:00", nil,
		"2026-10-23 10:30:00", "2026-10-23 11:00:00")
	w.budi.essay1ID = f.answer(w.budi.attemptID, w.essay1Q, "Jawaban Budi", nil, nil, nil, nil)

	// Citra: second owned quiz, one pending essay.
	citraStudent := f.newStudent("Citra Dewi", 12)
	w.citra.studentID = citraStudent
	w.citra.regID = f.register(w.quiz2, citraStudent, "2026-10-20 12:00:00")
	w.citra.attemptID = f.attempt(w.citra.regID, 1, 23, "2026-10-23 09:00:00", nil,
		"2026-10-23 08:30:00", "2026-10-23 09:00:00")
	w.citra.essay1ID = f.answer(w.citra.attemptID, q2Essay, "Jawaban Citra", nil, nil, nil, nil)

	// Dodi: in progress, so his essay is not a gradable resource.
	dodiStudent := f.newStudent("Dodi Pratama", 10)
	w.dodi.studentID = dodiStudent
	w.dodi.regID = f.register(w.quiz, dodiStudent, "2026-10-20 13:00:00")
	w.dodi.attemptID = f.attempt(w.dodi.regID, 1, 24, nil, nil,
		"2026-10-23 12:00:00", "2026-10-23 12:00:00")
	w.dodi.essay1ID = f.answer(w.dodi.attemptID, w.essay1Q, "Draf Dodi", nil, nil, nil, nil)

	// Eka: already graded today by the acting teacher, with feedback.
	ekaStudent := f.newStudent("Eka Putra", 11)
	w.eka.studentID = ekaStudent
	w.eka.regID = f.register(w.quiz, ekaStudent, "2026-10-20 14:00:00")
	w.ekaGradedAt = time.Now().UTC()
	w.eka.attemptID = f.attempt(w.eka.regID, 1, 25, "2026-10-23 08:00:00", nil,
		"2026-10-23 07:30:00", "2026-10-23 08:00:00")
	w.eka.essay1ID = f.answer(w.eka.attemptID, w.essay1Q, "Jawaban Eka",
		0.8550, "Bagus", f.teacherID, w.ekaGradedAt)

	// Fajar: a pending essay in another teacher's quiz, never in this queue.
	fajarStudent := f.newStudent("Fajar Nugroho", 12)
	w.fajar.studentID = fajarStudent
	w.fajar.regID = f.register(w.foreign, fajarStudent, "2026-10-20 15:00:00")
	w.fajar.attemptID = f.attempt(w.fajar.regID, 1, 26, "2026-10-23 10:30:00", nil,
		"2026-10-23 10:00:00", "2026-10-23 10:30:00")
	w.fajar.essay1ID = f.answer(w.fajar.attemptID, qForeignEssay, "Jawaban Fajar", nil, nil, nil, nil)

	return w
}

// TestGradingQueue pins ATT-201…ATT-207: pending predicate, order, counts that
// ignore filters, filter combinations, empty states, and the graded detail
// remaining reachable.
func TestGradingQueue(t *testing.T) {
	w := newGradingWorld(t)
	f := w.f

	page, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("GradingQueue: %v", err)
	}
	queue := page.Queue
	if queue.Counts.Pending != 4 || queue.Counts.GradedToday != 1 {
		t.Fatalf("counts = %+v, want pending 4 and graded today 1", queue.Counts)
	}
	if queue.Rows.Total != 4 || len(queue.Rows.Items) != 4 || queue.Rows.PerPage != 20 {
		t.Fatalf("rows = total %d items %d perPage %d, want 4/4/20",
			queue.Rows.Total, len(queue.Rows.Items), queue.Rows.PerPage)
	}
	wantOrder := []struct {
		answerID int64
		name     string
	}{
		{w.budi.essay1ID, "Budi Santoso"},
		{w.ayu.essay1ID, "Ayu Lestari"},
		{w.ayu.essay2ID, "Ayu Lestari"},
		{w.citra.essay1ID, "Citra Dewi"},
	}
	for i, want := range wantOrder {
		row := queue.Rows.Items[i]
		if row.AnswerID != want.answerID || row.StudentName != want.name {
			t.Errorf("row %d = (%d, %q), want (%d, %q)",
				i, row.AnswerID, row.StudentName, want.answerID, want.name)
		}
		if row.Href != gradingAnswerPath(want.answerID) {
			t.Errorf("row %d href = %q", i, row.Href)
		}
		if row.WaitingLabel == "" || row.QuestionExcerpt == "" || row.ClassLabel == "" {
			t.Errorf("row %d has an empty label: %+v", i, row)
		}
	}
	if queue.Rows.Items[1].ClassLabel != "X RPL" || queue.Rows.Items[0].ClassLabel != "XI RPL" {
		t.Errorf("class labels = %q, %q", queue.Rows.Items[1].ClassLabel, queue.Rows.Items[0].ClassLabel)
	}
	if queue.Empty.Title != "" {
		t.Errorf("non-empty queue has empty copy: %+v", queue.Empty)
	}

	// Filters narrow rows but never the two header counts.
	byQuizPage, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{
		AccountID: f.teacherID, QuizRaw: idString(w.quiz2),
	})
	if err != nil {
		t.Fatalf("quiz filter: %v", err)
	}
	byQuiz := byQuizPage.Queue
	if byQuiz.Rows.Total != 1 || byQuiz.Counts.Pending != 4 || byQuiz.Counts.GradedToday != 1 {
		t.Errorf("quiz filter = total %d counts %+v", byQuiz.Rows.Total, byQuiz.Counts)
	}
	if byQuiz.Rows.Items[0].QuizTitle != "Kuis Kedua" {
		t.Errorf("quiz filter row = %+v", byQuiz.Rows.Items[0])
	}

	byClassPage, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{
		AccountID: f.teacherID, ClassRaw: "XI",
	})
	if err != nil {
		t.Fatalf("class filter: %v", err)
	}
	byClass := byClassPage.Queue
	if byClass.Rows.Total != 1 || byClass.Rows.Items[0].AnswerID != w.budi.essay1ID {
		t.Errorf("class XI rows = %+v, want only Budi", byClass.Rows.Items)
	}
	if byClass.Counts.Pending != 4 {
		t.Errorf("class filter changed Pending: %d", byClass.Counts.Pending)
	}

	byPromptPage, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{
		AccountID: f.teacherID, QueryRaw: "JELASKAN A",
	})
	if err != nil {
		t.Fatalf("prompt search: %v", err)
	}
	byPrompt := byPromptPage.Queue
	if byPrompt.Rows.Total != 2 || byPrompt.Rows.Items[0].AnswerID != w.budi.essay1ID ||
		byPrompt.Rows.Items[1].AnswerID != w.ayu.essay1ID {
		t.Errorf("prompt search = %+v, want both essay-1 answers", byPrompt.Rows.Items)
	}
	byTitlePage, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{
		AccountID: f.teacherID, QueryRaw: "kuis kedua",
	})
	if err != nil {
		t.Fatalf("title search: %v", err)
	}
	byTitle := byTitlePage.Queue
	if byTitle.Rows.Total != 1 || byTitle.Rows.Items[0].AnswerID != w.citra.essay1ID {
		t.Errorf("title search = %+v, want Citra", byTitle.Rows.Items)
	}
	byNamePage, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{
		AccountID: f.teacherID, QueryRaw: "BUDI",
	})
	if err != nil {
		t.Fatalf("name search: %v", err)
	}
	byName := byNamePage.Queue
	if byName.Rows.Total != 1 || byName.Rows.Items[0].AnswerID != w.budi.essay1ID {
		t.Errorf("name search = %+v, want Budi", byName.Rows.Items)
	}

	nonePage, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{
		AccountID: f.teacherID, QueryRaw: "tidak ada",
	})
	if err != nil {
		t.Fatalf("no match: %v", err)
	}
	none := nonePage.Queue
	if none.Rows.Total != 0 || none.Empty.Title != "Tidak ada esai yang cocok dengan filter." {
		t.Errorf("filtered empty = %d / %q", none.Rows.Total, none.Empty.Title)
	}
	if none.Empty.Action == nil || none.Empty.Action.Href != "/teacher/grading" {
		t.Errorf("filtered-empty action = %+v", none.Empty.Action)
	}

	// The queue search field is normalized, and invalid filters fall back.
	fallbackPage, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{
		AccountID: f.teacherID, QueryRaw: "  BUDI  ", ClassRaw: "XIII",
		QuizRaw: "not-an-id", PageRaw: "0",
	})
	if err != nil {
		t.Fatalf("fallback: %v", err)
	}
	fallback := fallbackPage.Queue
	if fallback.Query.Q != "budi" || fallback.Rows.Total != 1 || fallback.Rows.Page != 1 {
		t.Errorf("fallback = %+v page %d", fallback.Query, fallback.Rows.Page)
	}

	// A graded answer is not in the queue but is reachable by direct URL and
	// renders the stored score and feedback with the one-decimal label.
	detail, err := f.svc.GradingDetail(f.ctx, GradingDetailInput{
		GradingQueueInput: GradingQueueInput{AccountID: f.teacherID},
		AnswerID:          w.eka.essay1ID,
	})
	if err != nil {
		t.Fatalf("graded detail: %v", err)
	}
	sel := detail.Selected
	if sel == nil {
		t.Fatal("graded detail has no Selected")
	}
	if sel.Answer.State != viewmodel.EssayGraded || sel.Answer.Score == nil ||
		sel.Answer.Score.Int() != 86 || sel.Answer.Feedback != "Bagus" ||
		sel.Feedback != "Bagus" {
		t.Errorf("graded detail = %+v", sel.Answer)
	}
	if sel.Answer.WaitingLabel != "Dinilai 85,5%" {
		t.Errorf("graded WaitingLabel = %q, want Dinilai 85,5%%", sel.Answer.WaitingLabel)
	}
	if sel.Suggested != nil {
		t.Errorf("Suggested = %+v, want nil (ATT-213)", sel.Suggested)
	}
	if sel.NextHref != gradingAnswerPath(w.budi.essay1ID) {
		t.Errorf("graded NextHref = %q, want the first pending essay", sel.NextHref)
	}
}

// TestGradingDetail pins ATT-211…ATT-214: the selected essay, its context
// rows, the next-pending link, and the 404 surface for objective/in-progress/
// foreign/missing answers.
func TestGradingDetail(t *testing.T) {
	w := newGradingWorld(t)
	f := w.f

	page, err := f.svc.GradingDetail(f.ctx, GradingDetailInput{
		GradingQueueInput: GradingQueueInput{AccountID: f.teacherID},
		AnswerID:          w.ayu.essay1ID,
	})
	if err != nil {
		t.Fatalf("GradingDetail: %v", err)
	}
	sel := page.Selected
	if sel == nil {
		t.Fatal("Selected is nil")
	}
	if sel.Answer.ID != w.ayu.essay1ID || sel.Answer.QuestionID != w.essay1Q ||
		sel.Answer.StudentName != "Ayu Lestari" || sel.Answer.ClassLabel != "X RPL" ||
		sel.Answer.QuizTitle != "Ujian Esai" || sel.Answer.QuestionBody != "Jelaskan A" ||
		sel.Answer.Text != "Jawaban Ayu 1" || sel.Answer.QuestionImage != nil {
		t.Errorf("Answer = %+v", sel.Answer)
	}
	if sel.Answer.State != viewmodel.EssayUngraded || sel.Answer.Score != nil || sel.Score != nil ||
		sel.Answer.Feedback != "" || sel.Feedback != "" {
		t.Errorf("ungraded detail carries a score/feedback: %+v", sel)
	}
	if sel.Attempt.ID != w.ayu.attemptID || sel.Attempt.QuizID != w.quiz ||
		sel.Attempt.QuizTitle != "Ujian Esai" || sel.Attempt.StudentName != "Ayu Lestari" ||
		sel.Attempt.AttemptNo != 1 || sel.Attempt.MaxAttempts != 1 {
		t.Errorf("Attempt = %+v", sel.Attempt)
	}
	if sel.Attempt.StartedAt.IsZero() || sel.Answer.SubmittedAt.IsZero() ||
		!sel.Answer.SubmittedAt.Time().Equal(time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("detail times = started %v submitted %v", sel.Attempt.StartedAt, sel.Answer.SubmittedAt)
	}
	if sel.Action != gradingAnswerPath(w.ayu.essay1ID) || sel.BackHref != "/teacher/grading" {
		t.Errorf("detail links = action %q back %q", sel.Action, sel.BackHref)
	}
	if sel.NextHref != gradingAnswerPath(w.ayu.essay2ID) {
		t.Errorf("NextHref = %q, want the next pending answer", sel.NextHref)
	}

	// Context is every other slot in derived order, never the current one.
	if len(sel.Context) != 2 {
		t.Fatalf("context rows = %d, want 2", len(sel.Context))
	}
	if got := sel.Context[0]; got.Position != 1 || got.Type != viewmodel.QuestionSingle ||
		got.Body != "Pilih satu" || got.StudentAnswer != "A" || got.Key != "A" ||
		got.Score == nil || got.Score.Int() != 100 || got.Verdict != "Benar" {
		t.Errorf("context[0] = %+v", got)
	}
	if got := sel.Context[1]; got.Position != 3 || got.Type != viewmodel.QuestionEssay ||
		got.Body != "Jelaskan B" || got.StudentAnswer != "Jawaban Ayu 2" ||
		got.Key != "—" || got.Score != nil || got.Verdict != "Menunggu nilai" {
		t.Errorf("context[1] = %+v", got)
	}

	// The next-pending link wraps to the head of the queue.
	last, err := f.svc.GradingDetail(f.ctx, GradingDetailInput{
		GradingQueueInput: GradingQueueInput{AccountID: f.teacherID},
		AnswerID:          w.citra.essay1ID,
	})
	if err != nil {
		t.Fatalf("last detail: %v", err)
	}
	if last.Selected.NextHref != gradingAnswerPath(w.budi.essay1ID) {
		t.Errorf("wrap NextHref = %q, want Budi", last.Selected.NextHref)
	}

	for _, tc := range []struct {
		name     string
		answerID int64
	}{
		{"objective", w.ayu.singleID},
		{"in progress", w.dodi.essay1ID},
		{"foreign quiz", w.fajar.essay1ID},
		{"missing", 999999},
	} {
		_, err := f.svc.GradingDetail(f.ctx, GradingDetailInput{
			GradingQueueInput: GradingQueueInput{AccountID: f.teacherID},
			AnswerID:          tc.answerID,
		})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s detail error = %v, want ErrNotFound", tc.name, err)
		}
	}
}

// TestGradingQueuePagination pins the 20-row page and the clamp (ATT-204).
func TestGradingQueuePagination(t *testing.T) {
	f := newTeacherFixture(t)
	quiz := f.newQuiz(f.teacherID, "Antrean Panjang", "open", true, false, 1, "2026-10-20 09:00:00")
	essay := f.question(quiz, 1, "essay", "Jelaskan")
	for i := 1; i <= 21; i++ {
		student := f.newStudent("Siswa "+twoDigits(i), 10)
		reg := f.register(quiz, student, "2026-10-20 10:00:00")
		submitted := "2026-10-23 10:00:00"
		attempt := f.attempt(reg, 1, uint32(30+i), submitted, nil, submitted, submitted)
		f.answer(attempt, essay, "Jawaban", nil, nil, nil, nil)
	}

	firstPage, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	first := firstPage.Queue
	if first.Rows.Total != 21 || len(first.Rows.Items) != 20 ||
		first.Rows.Page != 1 || first.Pagination.Pages != 2 {
		t.Fatalf("page 1 = total %d items %d page %d pages %d",
			first.Rows.Total, len(first.Rows.Items), first.Rows.Page, first.Pagination.Pages)
	}
	if !strings.Contains(first.Pagination.NextHref, "page=2") {
		t.Errorf("page 1 next = %q, want page=2", first.Pagination.NextHref)
	}
	secondPage, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{
		AccountID: f.teacherID, PageRaw: "99",
	})
	if err != nil {
		t.Fatalf("page 99: %v", err)
	}
	second := secondPage.Queue
	if second.Rows.Page != 2 || len(second.Rows.Items) != 1 {
		t.Errorf("page 99 = page %d items %d, want page 2 with 1", second.Rows.Page, len(second.Rows.Items))
	}
}

// TestGradeEssayWrite pins the one teacher write (ATT-221…ATT-228): stored
// units, provenance, feedback NULL, first-write-wins, final recompute, and the
// attempt's updated_at preservation.
func TestGradeEssayWrite(t *testing.T) {
	w := newGradingWorld(t)
	f := w.f
	oldUpdated := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	mustExec(t, f.db, `UPDATE quiz_attempts SET updated_at = ? WHERE id = ?`,
		oldUpdated, w.ayu.attemptID)

	if err := f.svc.GradeEssay(f.ctx, GradeInput{
		AccountID:   f.teacherID,
		AnswerID:    w.ayu.essay1ID,
		ScoreRaw:    "85,5",
		FeedbackRaw: "  Bagus sekali  ",
	}); err != nil {
		t.Fatalf("GradeEssay: %v", err)
	}

	var (
		score    float64
		feedback *string
		gradedBy int64
		gradedAt time.Time
	)
	if err := f.db.QueryRowContext(f.ctx, `
		SELECT score, feedback, graded_by_account_id, graded_at
		FROM attempt_answers WHERE id = ?`, w.ayu.essay1ID).
		Scan(&score, &feedback, &gradedBy, &gradedAt); err != nil {
		t.Fatalf("read graded answer: %v", err)
	}
	if !almostEqual(score, 0.8550) {
		t.Errorf("stored score = %.4f, want 0.8550", score)
	}
	if feedback == nil || *feedback != "  Bagus sekali  " {
		t.Errorf("stored feedback = %v, want the verbatim value", feedback)
	}
	if gradedBy != f.teacherID || gradedAt.IsZero() {
		t.Errorf("provenance = (%d, %v)", gradedBy, gradedAt)
	}

	var final *float64
	var updated time.Time
	if err := f.db.QueryRowContext(f.ctx, `
		SELECT final_score, updated_at FROM quiz_attempts WHERE id = ?`, w.ayu.attemptID).
		Scan(&final, &updated); err != nil {
		t.Fatalf("read attempt: %v", err)
	}
	if final != nil {
		t.Errorf("final_score = %v after one of two essays, want NULL", *final)
	}
	if !updated.Equal(oldUpdated) {
		t.Errorf("grading touched updated_at: %v, want %v", updated, oldUpdated)
	}

	// The queue lost the graded row and gained one graded-today count.
	afterPage, err := f.svc.GradingQueue(f.ctx, GradingQueueInput{AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("queue after grade: %v", err)
	}
	queue := afterPage.Queue
	if queue.Counts.Pending != 3 || queue.Counts.GradedToday != 2 {
		t.Errorf("queue after grade = %+v, want pending 3 and graded today 2", queue.Counts)
	}

	// A retry is a conflict, not a second write.
	err = f.svc.GradeEssay(f.ctx, GradeInput{
		AccountID: f.teacherID, AnswerID: w.ayu.essay1ID, ScoreRaw: "100",
	})
	if !errors.Is(err, ErrAlreadyGraded) {
		t.Fatalf("retry error = %v, want ErrAlreadyGraded", err)
	}
	if flash, ok := ConflictKey(err); !ok || flash != FlashAlreadyGraded {
		t.Errorf("retry conflict = (%q, %v), want %q", flash, ok, FlashAlreadyGraded)
	}
	var still float64
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT score FROM attempt_answers WHERE id = ?`, w.ayu.essay1ID).Scan(&still); err != nil {
		t.Fatalf("reread score: %v", err)
	}
	if !almostEqual(still, 0.8550) {
		t.Errorf("retry overwrote the score: %.4f", still)
	}

	// Grading the last essay completes the final score from all three slots:
	// (2*(10000+8550+0) + 3) / (2*3) = 6183 units = 0.6183.
	if err := f.svc.GradeEssay(f.ctx, GradeInput{
		AccountID: f.teacherID, AnswerID: w.ayu.essay2ID, ScoreRaw: "0", FeedbackRaw: "   ",
	}); err != nil {
		t.Fatalf("grade last essay: %v", err)
	}
	if err := f.db.QueryRowContext(f.ctx, `
		SELECT final_score, updated_at FROM quiz_attempts WHERE id = ?`, w.ayu.attemptID).
		Scan(&final, &updated); err != nil {
		t.Fatalf("read final: %v", err)
	}
	if final == nil || !almostEqual(*final, 0.6183) {
		t.Errorf("final_score = %v, want 0.6183", final)
	}
	if !updated.Equal(oldUpdated) {
		t.Errorf("last grade touched updated_at: %v", updated)
	}
	var (
		zeroScore float64
		emptyFB   *string
	)
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT score, feedback FROM attempt_answers WHERE id = ?`, w.ayu.essay2ID).
		Scan(&zeroScore, &emptyFB); err != nil {
		t.Fatalf("read zero essay: %v", err)
	}
	if zeroScore != 0 || emptyFB != nil {
		t.Errorf("zero/empty grade = (%v, %v), want 0 and NULL", zeroScore, emptyFB)
	}

	// Non-gradable resources are 404 before any write.
	for _, tc := range []struct {
		name     string
		answerID int64
	}{
		{"objective", w.ayu.singleID},
		{"in progress", w.dodi.essay1ID},
		{"foreign quiz", w.fajar.essay1ID},
		{"missing", 999999},
	} {
		err := f.svc.GradeEssay(f.ctx, GradeInput{
			AccountID: f.teacherID, AnswerID: tc.answerID, ScoreRaw: "80",
		})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s grade error = %v, want ErrNotFound", tc.name, err)
		}
	}

	// A graded answer from a colleague is still a conflict for this teacher.
	if err := f.svc.GradeEssay(f.ctx, GradeInput{
		AccountID: f.otherTeacherID, AnswerID: w.eka.essay1ID, ScoreRaw: "100",
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign graded answer = %v, want ErrNotFound", err)
	}
}

// TestGradeEssayValidation pins ATT-222/ATT-228: the score grammar and the
// field messages, and that a failed validation writes nothing.
func TestGradeEssayValidation(t *testing.T) {
	w := newGradingWorld(t)
	f := w.f

	valid := []struct {
		raw  string
		unit int
	}{
		{"100", 10000},
		{"0", 0},
		{"85,5", 8550},
		{"85.5", 8550},
		{" 85 ", 8500},
		{"00085", 8500},
		{"85,0", 8500},
	}
	for _, tc := range valid {
		units, err := GradeScore(tc.raw)
		if err != nil || units != tc.unit {
			t.Errorf("GradeScore(%q) = (%d, %v), want (%d, nil)", tc.raw, units, err, tc.unit)
		}
	}
	invalid := []string{"", "85,55", "-1", "1e2", "101", "85,", ",5", "abc", " "}
	for _, raw := range invalid {
		if _, err := GradeScore(raw); err == nil {
			t.Errorf("GradeScore(%q) accepted, want a validation error", raw)
		}
	}

	// Empty and malformed score messages.
	if _, err := GradeScore(""); err == nil || err.Fields()["score"] != FieldErrRequired {
		t.Errorf("empty score error = %v, want %q", err, FieldErrRequired)
	}
	if _, err := GradeScore("85,55"); err == nil || err.Fields()["score"] != FieldErrRange {
		t.Errorf("two-decimal score error = %v, want %q", err, FieldErrRange)
	}
	// Feedback is bounded by runes and trimmed only for the emptiness check.
	if fb, err := GradeFeedback("  hello  "); err != nil || fb == nil || *fb != "  hello  " {
		t.Errorf("feedback = (%v, %v), want the verbatim value", fb, err)
	}
	if fb, err := GradeFeedback("   "); err != nil || fb != nil {
		t.Errorf("blank feedback = (%v, %v), want nil/nil", fb, err)
	}
	if _, err := GradeFeedback(strings.Repeat("é", 1001)); err == nil ||
		err.Fields()["feedback"] != FieldErrFeedback {
		t.Errorf("1001-rune feedback error = %v", err)
	}

	// Both fields fail together, and nothing is written.
	err := f.svc.GradeEssay(f.ctx, GradeInput{
		AccountID: f.teacherID, AnswerID: w.ayu.essay1ID,
		ScoreRaw: "", FeedbackRaw: strings.Repeat("x", 1001),
	})
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind() != apperr.Validation {
		t.Fatalf("GradeEssay error = %v, want apperr.Validation", err)
	}
	fields := appErr.Fields()
	if fields["score"] != FieldErrRequired || fields["feedback"] != FieldErrFeedback {
		t.Errorf("fields = %v", fields)
	}
	var (
		score    *float64
		feedback *string
		final    *float64
	)
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT score, feedback FROM attempt_answers WHERE id = ?`, w.ayu.essay1ID).
		Scan(&score, &feedback); err != nil {
		t.Fatalf("read after invalid: %v", err)
	}
	if score != nil || feedback != nil {
		t.Errorf("invalid grade wrote (%v, %v)", score, feedback)
	}
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT final_score FROM quiz_attempts WHERE id = ?`, w.ayu.attemptID).Scan(&final); err != nil {
		t.Fatalf("read final after invalid: %v", err)
	}
	if final != nil {
		t.Errorf("invalid grade set final_score = %v", *final)
	}
}

func almostEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.00005
}

func idString(id int64) string {
	return strconv.FormatInt(id, 10)
}

func twoDigits(n int) string {
	return fmt.Sprintf("%02d", n)
}
