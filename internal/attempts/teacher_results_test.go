package attempts

import (
	"errors"
	"strings"
	"testing"
)

// TestResultsMetricsFiltersAndPagination pins ATT-171…ATT-178: quiz-wide
// header metrics and average, tab counts that ignore only state, literal
// search, class/state filters, row order, page size, and the two empty states.
func TestResultsMetricsFiltersAndPagination(t *testing.T) {
	f := newTeacherFixture(t)

	// The owner's view ignores show_score (ATT-174, ATT-177).
	quiz := f.newQuiz(f.teacherID, "Ulangan Bab 3", "open", false, false, 1, "2026-09-20 09:00:00")

	type participant struct {
		name      string
		grade     int
		final     any
		submitted string
	}
	people := []participant{
		{"Ayu Lestari", 10, 0.7250, "2026-10-01 08:00:00"},
		{"Budi Santoso", 11, 0.0000, "2026-10-02 08:00:00"},
		{"Citra Dewi", 12, nil, "2026-10-03 08:00:00"},
		{"Dewi Anggraini", 10, nil, "2026-10-04 08:00:00"},
		{"Eka Putra", 11, nil, "2026-10-05 08:00:00"},
		{"Fajar Nugroho", 12, nil, "2026-10-06 08:00:00"},
		{"Gita Permata", 10, nil, "2026-10-07 08:00:00"},
		{"Hadi Wijaya", 11, nil, "2026-10-08 08:00:00"},
		{"Indra Saputra", 12, nil, "2026-10-09 08:00:00"},
		{"Joko Susilo", 10, nil, "2026-10-10 08:00:00"},
		{"Kiki Amelia", 11, nil, "2026-10-11 08:00:00"},
		{"Li%na Sari", 12, nil, "2026-10-12 08:00:00"},
	}
	for _, p := range people {
		student := f.newStudent(p.name, p.grade)
		reg := f.register(quiz, student, "2026-09-21 10:00:00")
		f.attempt(reg, 1, 11, p.submitted, p.final,
			"2026-09-21 11:00:00", p.submitted)
	}

	// Two in-progress attempts are counted in the header but never rows.
	for _, p := range []struct {
		name  string
		grade int
	}{
		{"Mita Sari", 10},
		{"Nanda Pratama", 11},
	} {
		student := f.newStudent(p.name, p.grade)
		reg := f.register(quiz, student, "2026-10-13 10:00:00")
		f.attempt(reg, 1, 12, nil, nil, "2026-10-13 10:00:00", "2026-10-13 10:05:00")
	}

	got, err := f.svc.Results(f.ctx, ResultsInput{QuizID: quiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("Results: %v", err)
	}
	if got.Counts.Submitted != 12 || got.Counts.InProgress != 2 {
		t.Fatalf("counts = (%d, %d), want (12, 2)", got.Counts.Submitted, got.Counts.InProgress)
	}
	if got.Counts.Average == nil || got.Counts.Average.String() != "36,3%" {
		t.Fatalf("average = %v, want 36,3%% (ATT-174)", got.Counts.Average)
	}
	if got.Rows.Total != 12 || got.Rows.PerPage != 10 || len(got.Rows.Items) != 10 {
		t.Fatalf("page 1 = total %d, perPage %d, items %d; want 12/10/10",
			got.Rows.Total, got.Rows.PerPage, len(got.Rows.Items))
	}
	if got.Rows.Items[0].StudentName != "Li%na Sari" {
		t.Errorf("first row = %q, want the newest submission", got.Rows.Items[0].StudentName)
	}
	// Tab counts apply class/search but not state: all=12, waiting=10, graded=2.
	if len(got.Tabs) != 3 || got.Tabs[0].Count != 12 || got.Tabs[1].Count != 10 || got.Tabs[2].Count != 2 {
		t.Fatalf("tabs = %+v, want counts 12/10/2", got.Tabs)
	}
	if !got.Tabs[0].Active {
		t.Error("default tab is not active")
	}
	// The header's grading action is this quiz's slice of the essay queue, never
	// the whole-school /teacher/grading the nav points at.
	if want := "/teacher/grading?quiz=" + idString(quiz); got.GradingHref != want {
		t.Errorf("GradingHref = %q, want %q", got.GradingHref, want)
	}
	if got.BackHref != quizPath(quiz) {
		t.Errorf("BackHref = %q, want the quiz detail", got.BackHref)
	}

	// Page 2 holds Budi (0%) then Ayu (73%); 0 is a score, never nil.
	page2, err := f.svc.Results(f.ctx, ResultsInput{QuizID: quiz, AccountID: f.teacherID, PageRaw: "2"})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2.Rows.Items) != 2 {
		t.Fatalf("page 2 items = %d, want 2", len(page2.Rows.Items))
	}
	if page2.Rows.Items[0].StudentName != "Budi Santoso" || page2.Rows.Items[1].StudentName != "Ayu Lestari" {
		t.Errorf("page 2 order = %q, %q; want Budi then Ayu",
			page2.Rows.Items[0].StudentName, page2.Rows.Items[1].StudentName)
	}
	if budi := page2.Rows.Items[0]; budi.Waiting || budi.Score == nil || budi.Score.Int() != 0 {
		t.Errorf("Budi row = %+v, want a graded 0%% score", budi)
	}
	if ayu := page2.Rows.Items[1]; ayu.Waiting || ayu.Score == nil || ayu.Score.Int() != 73 {
		t.Errorf("Ayu row = %+v, want a graded 73%% score", ayu)
	}
	if page2.Pagination.Page != 2 || page2.Pagination.Pages != 2 || !page2.Pagination.HasPrev() {
		t.Errorf("page 2 pagination = %+v", page2.Pagination)
	}

	// state=waiting filters rows but not tab counts.
	waiting, err := f.svc.Results(f.ctx, ResultsInput{
		QuizID: quiz, AccountID: f.teacherID, StateRaw: "bogus",
	})
	if err != nil {
		t.Fatalf("invalid state: %v", err)
	}
	if waiting.Query.Tab != "all" || len(waiting.Rows.Items) != 10 {
		t.Errorf("invalid state fallback = %q with %d rows, want all/10",
			waiting.Query.Tab, len(waiting.Rows.Items))
	}
	graded, err := f.svc.Results(f.ctx, ResultsInput{
		QuizID: quiz, AccountID: f.teacherID, StateRaw: "graded",
	})
	if err != nil {
		t.Fatalf("graded: %v", err)
	}
	if len(graded.Rows.Items) != 2 || graded.Rows.Items[0].StudentName != "Budi Santoso" {
		t.Errorf("graded rows = %+v, want Budi then Ayu", graded.Rows.Items)
	}
	if graded.Tabs[0].Count != 12 || graded.Tabs[1].Count != 10 || graded.Tabs[2].Count != 2 {
		t.Errorf("graded tab counts = %+v, want the unfiltered-by-state counts", graded.Tabs)
	}

	// class=XI: four submitted rows, header still 12/2.
	classXI, err := f.svc.Results(f.ctx, ResultsInput{
		QuizID: quiz, AccountID: f.teacherID, ClassRaw: "XI",
	})
	if err != nil {
		t.Fatalf("class XI: %v", err)
	}
	if len(classXI.Rows.Items) != 4 {
		t.Fatalf("class XI rows = %d, want 4", len(classXI.Rows.Items))
	}
	if classXI.Counts.Submitted != 12 || classXI.Counts.InProgress != 2 {
		t.Errorf("class filter changed quiz-wide counts: %+v", classXI.Counts)
	}
	if classXI.Tabs[0].Count != 4 || classXI.Tabs[1].Count != 3 || classXI.Tabs[2].Count != 1 {
		t.Errorf("class XI tabs = %+v, want 4/3/1", classXI.Tabs)
	}
	if !strings.Contains(classXI.Pagination.NextHref+classXI.Tabs[0].Href, "class=XI") {
		t.Errorf("class filter missing from generated links: %+v", classXI.Pagination)
	}

	// Search is a literal substring: "%" must match the one name containing
	// it, not every row.
	percent, err := f.svc.Results(f.ctx, ResultsInput{
		QuizID: quiz, AccountID: f.teacherID, QueryRaw: "  LI%NA  ",
	})
	if err != nil {
		t.Fatalf("percent search: %v", err)
	}
	if len(percent.Rows.Items) != 1 || percent.Rows.Items[0].StudentName != "Li%na Sari" {
		t.Fatalf("literal %% search rows = %+v, want only Li%%na", percent.Rows.Items)
	}
	if percent.Query.Q != "li%na" {
		t.Errorf("normalized query = %q, want li%%na", percent.Query.Q)
	}
	if percent.Tabs[0].Count != 1 || percent.Tabs[1].Count != 1 || percent.Tabs[2].Count != 0 {
		t.Errorf("search tab counts = %+v, want 1/1/0", percent.Tabs)
	}
	noMatch, err := f.svc.Results(f.ctx, ResultsInput{
		QuizID: quiz, AccountID: f.teacherID, QueryRaw: "zzz",
	})
	if err != nil {
		t.Fatalf("no match: %v", err)
	}
	if len(noMatch.Rows.Items) != 0 || noMatch.Empty.Title != "Tidak ada percobaan yang cocok." {
		t.Errorf("filtered empty = %+v / %q", noMatch.Rows, noMatch.Empty.Title)
	}
	if noMatch.Empty.Action == nil || noMatch.Empty.Action.Href != resultsPath(quiz) {
		t.Errorf("filtered-empty action = %+v, want the unfiltered results link", noMatch.Empty.Action)
	}

	// Invalid page and class fall back; the state filter still applies.
	fallback, err := f.svc.Results(f.ctx, ResultsInput{
		QuizID: quiz, AccountID: f.teacherID, PageRaw: "99", ClassRaw: "XIII", StateRaw: "waiting",
	})
	if err != nil {
		t.Fatalf("fallback: %v", err)
	}
	if fallback.Rows.Page != 1 || len(fallback.Rows.Items) != 10 {
		t.Errorf("waiting page 99 = page %d with %d rows, want page 1 with 10",
			fallback.Rows.Page, len(fallback.Rows.Items))
	}
	if strings.Contains(fallback.Tabs[0].Href, "class=") || strings.Contains(fallback.Tabs[0].Href, "state=") {
		t.Errorf("invalid filters were echoed into links: %q", fallback.Tabs[0].Href)
	}

	// Quiz-wide search + class + state: metrics stay quiz-wide, counts narrow.
	narrow, err := f.svc.Results(f.ctx, ResultsInput{
		QuizID: quiz, AccountID: f.teacherID, ClassRaw: "XI", QueryRaw: "kiki", StateRaw: "waiting",
	})
	if err != nil {
		t.Fatalf("narrow: %v", err)
	}
	if narrow.Counts.Submitted != 12 || narrow.Counts.InProgress != 2 ||
		narrow.Counts.Average == nil || narrow.Counts.Average.String() != "36,3%" {
		t.Errorf("narrow filters changed the header/average: %+v", narrow.Counts)
	}
	if narrow.Tabs[0].Count != 1 || narrow.Tabs[1].Count != 1 || narrow.Tabs[2].Count != 0 {
		t.Errorf("narrow tabs = %+v, want 1/1/0", narrow.Tabs)
	}

	// An open quiz with no submitted attempt has the "no attempts" empty state.
	emptyQuiz := f.newQuiz(f.teacherID, "Kosong", "open", true, false, 1, "2026-09-22 09:00:00")
	emptyStudent := f.newStudent("Peserta Kosong", 10)
	emptyReg := f.register(emptyQuiz, emptyStudent, "2026-09-22 10:00:00")
	f.attempt(emptyReg, 1, 13, nil, nil, "2026-09-22 10:00:00", "2026-09-22 10:01:00")
	empty, err := f.svc.Results(f.ctx, ResultsInput{QuizID: emptyQuiz, AccountID: f.teacherID})
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if empty.Counts.Submitted != 0 || empty.Counts.InProgress != 1 || empty.Counts.Average != nil {
		t.Errorf("empty counts = %+v, want submitted 0, in progress 1, no average", empty.Counts)
	}
	if empty.Empty.Title != "Belum ada percobaan." || empty.Rows.Page != 1 {
		t.Errorf("empty state = %q page %d, want the no-attempt copy on page 1",
			empty.Empty.Title, empty.Rows.Page)
	}
	// Zero attempts is not a filtered-empty list: the only destination is the
	// back link above the heading, so the empty state carries no action.
	if empty.Empty.Action != nil {
		t.Errorf("zero-attempt empty action = %+v, want none (the back link owns it)", empty.Empty.Action)
	}

	draft := f.newQuiz(f.teacherID, "Draf", "draft", true, false, 1, "2026-09-23 09:00:00")
	for _, tc := range []struct {
		name string
		in   ResultsInput
	}{
		{"draft", ResultsInput{QuizID: draft, AccountID: f.teacherID}},
		{"foreign", ResultsInput{QuizID: quiz, AccountID: f.otherTeacherID}},
		{"missing", ResultsInput{QuizID: 999999, AccountID: f.teacherID}},
	} {
		if _, err := f.svc.Results(f.ctx, tc.in); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s Results error = %v, want ErrNotFound", tc.name, err)
		}
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
