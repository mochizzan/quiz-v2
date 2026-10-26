package httpserver_test

// teacher_hierarchy_pages_test.go pins the teacher quiz drill-down family's
// design contract (DESIGN §7/§8): the quiz detail screen is the hub, and results
// and monitor are drill-downs that carry a back link plus at most one job
// control. Rendering the templates directly from ./templates — the same way
// monitor_live_test.go and profile_pages_test.go do — keeps the assertions on
// the rendered affordances, which have no transport surface of their own.

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"quiz-v2/internal/viewmodel"
)

// pageBody returns the rendered screen between the .page-stack opening element
// and the closing </main>: the page's own markup without the shared navbar,
// dialog and footer chrome, which also carries buttons and must not be counted
// as part of a screen's action hierarchy.
func pageBody(t *testing.T, html string) string {
	t.Helper()
	start := strings.Index(html, `<div class="page-stack"`)
	end := strings.Index(html, "</main>")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("page body markers not found (page-stack=%d main=%d)", start, end)
	}
	return html[start:end]
}

// between returns the markup from start up to the first end after it.
func between(t *testing.T, html, start, end string) string {
	t.Helper()
	i := strings.Index(html, start)
	if i < 0 {
		t.Fatalf("marker %q not found", start)
	}
	rest := html[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		t.Fatalf("closing marker %q not found after %q", end, start)
	}
	return rest[:j]
}

// TestQuizDetailIsTheOnlyHub pins §3.1: the status card holds the lifecycle
// forms only, the header primary appears once, and the duplicate
// results/monitor/grading/self links are gone.
func TestQuizDetailIsTheOnlyHub(t *testing.T) {
	addHref := "/teacher/quizzes/9/questions/new"
	emptyAction := viewmodel.Primary("Tambah soal", addHref)
	base := viewmodel.QuizDetail{
		Quiz:            viewmodel.QuizRef{ID: 9, Title: "Ulangan Bab 3"},
		State:           viewmodel.QuizDraft,
		Visibility:      viewmodel.VisibilityPublic,
		Audience:        "Semua siswa",
		Editable:        true,
		AddQuestionHref: addHref,
		ResultsHref:     "/teacher/quizzes/9/results",
		MonitorHref:     "/teacher/quizzes/9/monitor",
		Empty: viewmodel.EmptyState{
			Title:  "Belum ada soal",
			Body:   "Tambahkan soal pertama.",
			Icon:   "quiz",
			Action: &emptyAction,
		},
	}

	emptyHTML := pageBody(t, renderPage(t, "pages/teacher/quiz-detail.html",
		viewmodel.PageData{Base: viewmodel.Base{Title: "Detail kuis"}, Data: base}))

	// The deleted .lifecycle-links row and every destination it duplicated.
	for _, gone := range []string{
		"lifecycle-links",
		"Lihat hasil",
		"Monitor langsung",
		"Nilai esai",
		"Ubah data kuis",
	} {
		if strings.Contains(emptyHTML, gone) {
			t.Errorf("quiz-detail still renders %q", gone)
		}
	}
	// One action, one primary, in one region: the empty panel owns Tambah soal,
	// the header suppresses its copy, and the settings save is a secondary.
	if got := strings.Count(emptyHTML, "btn btn-solid theme-primary"); got != 1 {
		t.Errorf("empty draft primaries = %d, want 1 (the empty-state Tambah soal)", got)
	}
	head := between(t, emptyHTML, `<header class="page-head">`, `</header>`)
	if strings.Contains(head, "Tambah soal") {
		t.Error("header repeats Tambah soal while the empty panel owns it")
	}
	if !strings.Contains(emptyHTML, `class="btn btn-outline theme-primary w-100 mt-4"`) {
		t.Error("Simpan pengaturan must be a secondary button, not the screen primary")
	}

	// With rows in the list the header owns the primary again, still exactly one.
	base.Questions = []viewmodel.QuestionRow{{
		ID:           1,
		Position:     1,
		Type:         viewmodel.QuestionSingle,
		Body:         "Pilih satu",
		OptionCount:  2,
		CorrectLabel: "A",
		EditHref:     "/teacher/quizzes/9/questions/1",
		DeleteHref:   "/teacher/quizzes/9/questions/1/delete",
	}}
	fullHTML := pageBody(t, renderPage(t, "pages/teacher/quiz-detail.html",
		viewmodel.PageData{Base: viewmodel.Base{Title: "Detail kuis"}, Data: base}))

	head = between(t, fullHTML, `<header class="page-head">`, `</header>`)
	if !strings.Contains(head, `href="`+addHref+`"`) || !strings.Contains(head, "Tambah soal") {
		t.Error("header must carry Tambah soal once the question list has rows")
	}
	if got := strings.Count(fullHTML, "btn btn-solid theme-primary"); got != 1 {
		t.Errorf("draft-with-questions primaries = %d, want 1", got)
	}
}

// The reorder footer is checked against a parsed DOM, not raw substrings: the
// hook must sit on the submit button itself, and a hidden descendant must not
// satisfy the button's hidden requirement.

func htmlAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasHTMLAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func hasHTMLClass(n *html.Node, class string) bool {
	for _, field := range strings.Fields(htmlAttr(n, "class")) {
		if field == class {
			return true
		}
	}
	return false
}

func findHTMLNode(root *html.Node, match func(*html.Node) bool) *html.Node {
	if root.Type == html.ElementNode && match(root) {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLNode(child, match); found != nil {
			return found
		}
	}
	return nil
}

func collectHTMLNodes(root *html.Node, match func(*html.Node) bool, out *[]*html.Node) {
	if root.Type == html.ElementNode && match(root) {
		*out = append(*out, root)
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		collectHTMLNodes(child, match, out)
	}
}

func htmlContains(root, target *html.Node) bool {
	if root == target {
		return true
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if htmlContains(child, target) {
			return true
		}
	}
	return false
}

func htmlNodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// TestQuizDetailSaveOrderVisibilityContract pins the reorder footer contract:
// the save-order control exists on an editable draft with questions, but the
// server renders it hidden so only JavaScript can reveal it after the order
// changes. Locked and empty pages do not render the control at all.
func TestQuizDetailSaveOrderVisibilityContract(t *testing.T) {
	questions := []viewmodel.QuestionRow{
		{
			ID:           1,
			Position:     1,
			Type:         viewmodel.QuestionSingle,
			Body:         "Pilih satu",
			OptionCount:  2,
			CorrectLabel: "A",
			EditHref:     "/teacher/quizzes/9/questions/1",
			DeleteHref:   "/teacher/quizzes/9/questions/1/delete",
		},
		{
			ID:           2,
			Position:     2,
			Type:         viewmodel.QuestionSingle,
			Body:         "Pilih dua",
			OptionCount:  2,
			CorrectLabel: "B",
			EditHref:     "/teacher/quizzes/9/questions/2",
			DeleteHref:   "/teacher/quizzes/9/questions/2/delete",
		},
	}
	base := viewmodel.QuizDetail{
		Quiz:       viewmodel.QuizRef{ID: 9, Title: "Ulangan Bab 3"},
		Visibility: viewmodel.VisibilityPublic,
		Audience:   "Semua siswa",
		Empty: viewmodel.EmptyState{
			Title: "Belum ada soal",
			Body:  "Tambahkan soal pertama.",
			Icon:  "quiz",
		},
	}

	cases := []struct {
		name      string
		state     viewmodel.QuizState
		editable  bool
		questions []viewmodel.QuestionRow
	}{
		{"editable draft with questions", viewmodel.QuizDraft, true, questions},
		{"open quiz with questions", viewmodel.QuizOpen, false, questions},
		{"closed quiz with questions", viewmodel.QuizClosed, false, questions},
		{"draft with no questions", viewmodel.QuizDraft, true, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := base
			page.State = tc.state
			page.Editable = tc.editable
			page.Questions = tc.questions
			rendered := pageBody(t, renderPage(t, "pages/teacher/quiz-detail.html",
				viewmodel.PageData{Base: viewmodel.Base{Title: "Detail kuis"}, Data: page}))
			doc, err := html.Parse(strings.NewReader(rendered))
			if err != nil {
				t.Fatalf("parse rendered quiz detail: %v", err)
			}

			var reorderForms []*html.Node
			collectHTMLNodes(doc, func(n *html.Node) bool {
				return n.Data == "form" && htmlAttr(n, "action") == "/teacher/quizzes/9/questions/reorder"
			}, &reorderForms)
			var hooks []*html.Node
			collectHTMLNodes(doc, func(n *html.Node) bool {
				return hasHTMLAttr(n, "data-save-order")
			}, &hooks)

			if !tc.editable || len(tc.questions) == 0 {
				if len(reorderForms) != 0 {
					t.Errorf("locked or empty page renders %d reorder forms, want 0", len(reorderForms))
				}
				if len(hooks) != 0 {
					t.Errorf("locked or empty page renders %d save-order hooks, want 0", len(hooks))
				}
				if save := findHTMLNode(doc, func(n *html.Node) bool {
					return n.Data == "button" && strings.Contains(htmlNodeText(n), "Simpan urutan")
				}); save != nil {
					t.Error("locked or empty page renders a Simpan urutan button")
				}
				return
			}

			if len(reorderForms) != 1 {
				t.Fatalf("reorder forms = %d, want 1", len(reorderForms))
			}
			form := reorderForms[0]
			if len(hooks) != 1 {
				t.Fatalf("save-order hooks on page = %d, want 1", len(hooks))
			}
			save := hooks[0]
			if save.Data != "button" {
				t.Fatalf("data-save-order sits on <%s>, want <button>", save.Data)
			}
			if !htmlContains(form, save) {
				t.Error("save-order button is outside the reorder form")
			}
			if !hasHTMLAttr(save, "hidden") {
				t.Error("save-order button lacks the server-rendered hidden attribute")
			}
			if got := htmlAttr(save, "type"); got != "submit" {
				t.Errorf("save-order button type = %q, want submit", got)
			}
			if !hasHTMLClass(save, "btn-outline") || hasHTMLClass(save, "btn-solid") {
				t.Errorf("save-order button must stay secondary, class = %q", htmlAttr(save, "class"))
			}
			if text := htmlNodeText(save); !strings.Contains(text, "Simpan urutan") {
				t.Errorf("save-order button label = %q, want Simpan urutan", text)
			}
			if save.Parent == nil || save.Parent.Data != "div" || !hasHTMLClass(save.Parent, "builder-foot") {
				t.Error("save-order button must sit in .builder-foot")
			}
			if icon := findHTMLNode(save, func(n *html.Node) bool {
				return n.Data == "use" && htmlAttr(n, "href") == "#i-check"
			}); icon == nil {
				t.Error("save-order button lost its check icon")
			}

			help := findHTMLNode(form, func(n *html.Node) bool {
				return n.Data == "p" && hasHTMLClass(n, "field-help")
			})
			if help == nil {
				t.Fatal("reorder footer help text is missing")
			}
			copy := htmlNodeText(help)
			for _, want := range []string{"muncul setelah urutan berubah", "memerlukan JavaScript"} {
				if !strings.Contains(copy, want) {
					t.Errorf("reorder footer copy is missing %q: %q", want, copy)
				}
			}

			orderInput := findHTMLNode(form, func(n *html.Node) bool {
				return n.Data == "input" && htmlAttr(n, "id") == "question-order"
			})
			if orderInput == nil || htmlAttr(orderInput, "name") != "order" {
				t.Error("reorder form is missing the #question-order hidden input")
			}

			settings := findHTMLNode(doc, func(n *html.Node) bool {
				return n.Data == "form" && hasHTMLClass(n, "settings-form")
			})
			if settings == nil {
				t.Fatal("settings form is missing")
			}
			if !hasHTMLAttr(settings, "data-dirty-form") {
				t.Error("settings form must opt into the dirty-form save flow")
			}
			settingsSave := findHTMLNode(settings, func(n *html.Node) bool {
				return n.Data == "button" && htmlAttr(n, "type") == "submit"
			})
			if settingsSave == nil {
				t.Fatal("settings save button is missing")
			}
			if hasHTMLAttr(settingsSave, "hidden") {
				t.Error("settings save button must stay visible")
			}
			if !hasHTMLAttr(settingsSave, "data-dirty-submit") {
				t.Error("settings save button must carry data-dirty-submit")
			}
			if hasHTMLAttr(settingsSave, "data-save-order") {
				t.Error("save-order hook leaked into the settings form")
			}
			if text := htmlNodeText(settingsSave); !strings.Contains(text, "Simpan pengaturan") {
				t.Errorf("settings save label = %q, want Simpan pengaturan", text)
			}

			retake := findHTMLNode(settings, func(n *html.Node) bool {
				return n.Data == "input" && htmlAttr(n, "name") == "allow_retake"
			})
			if retake == nil || !hasHTMLAttr(retake, "data-retake-toggle") {
				t.Error("allow_retake switch must carry data-retake-toggle")
			}
			attempts := findHTMLNode(settings, func(n *html.Node) bool {
				return n.Data == "input" && htmlAttr(n, "name") == "max_attempts"
			})
			if attempts == nil {
				t.Fatal("max_attempts field is missing")
			}
			row := attempts.Parent
			for row != nil && !(row.Data == "div" && hasHTMLClass(row, "field-row")) {
				row = row.Parent
			}
			if row == nil || !hasHTMLAttr(row, "data-retake-field") {
				t.Error("max_attempts field-row must carry data-retake-field")
			}

			status := findHTMLNode(doc, func(n *html.Node) bool {
				return n.Data == "section" && htmlAttr(n, "aria-labelledby") == "lifecycle-title"
			})
			if status == nil {
				t.Fatal("Status kuis card is missing")
			}
			var statusHooks []*html.Node
			collectHTMLNodes(status, func(n *html.Node) bool {
				return hasHTMLAttr(n, "data-save-order")
			}, &statusHooks)
			if len(statusHooks) != 0 {
				t.Error("save-order hook leaked into the Status kuis card")
			}
		})
	}
}

// TestResultsCarriesScopedActions pins §3.2 as amended 2026-10-25: the header
// carries two secondary actions, the grading queue and the XLSX export, both
// scoped to this quiz. A zero-attempt empty state has no action because the back
// link is already that destination.
func TestResultsCarriesScopedActions(t *testing.T) {
	page := viewmodel.QuizResults{
		Quiz:        viewmodel.QuizRef{ID: 9, Title: "Ulangan Bab 3"},
		BackHref:    "/teacher/quizzes/9",
		GradingHref: "/teacher/grading?quiz=9",
		ExportHref:  "/teacher/quizzes/9/export.xlsx",
		ReviewMode:  viewmodel.ReviewQuestionsNoVerdict,
		Rows:        viewmodel.NewList([]viewmodel.ResultsRow{}, 1, 10),
		Empty: viewmodel.EmptyState{
			Title: "Belum ada percobaan.",
			Body:  "Percobaan siswa akan muncul di sini setelah mereka mengirim jawaban.",
			Icon:  "quiz",
		},
	}
	html := pageBody(t, renderPage(t, "pages/teacher/results.html",
		viewmodel.PageData{Base: viewmodel.Base{Title: "Hasil kuis"}, Data: page}))

	checkBackAffordance(t, html, "/teacher/quizzes/9", "Kembali ke detail kuis")

	head := between(t, html, `<header class="page-head">`, `</header>`)
	if !strings.Contains(head, `href="/teacher/grading?quiz=9"`) {
		t.Error("the grading action must be scoped to this quiz")
	}
	if !strings.Contains(head, "Nilai esai kuis ini") {
		t.Error("the scoped grading action must name its scope")
	}
	if !strings.Contains(head, `href="/teacher/quizzes/9/export.xlsx"`) {
		t.Error("the export action must be scoped to this quiz")
	}
	if !strings.Contains(head, "Unduh XLSX") {
		t.Error("the export action must be labelled Unduh XLSX")
	}
	if got := strings.Count(head, "btn "); got != 2 {
		t.Errorf("header actions = %d, want exactly two secondary actions", got)
	}
	if strings.Contains(head, "btn-primary") {
		t.Error("the results header must carry no primary action")
	}
	if strings.Contains(html, `href="/teacher/grading"`) {
		t.Error("results still links to the whole-school grading queue")
	}
	if empty := between(t, html, `<div class="empty-panel">`, `</div>`); strings.Contains(empty, "<a ") {
		t.Errorf("zero-attempt empty state repeats the back link as an action: %q", empty)
	}
}

// TestMonitorDrillDownHasNoHeaderActions pins §3.3: the back link is the whole
// navigation, and the empty branch adds no second route to the quiz.
func TestMonitorDrillDownHasNoHeaderActions(t *testing.T) {
	page := viewmodel.MonitorPage{
		Quiz:        viewmodel.QuizRef{ID: 9, Title: "Ulangan Bab 3"},
		QuizHref:    "/teacher/quizzes/9",
		GeneratedAt: viewmodel.NewLocalTime(time.Date(2026, 10, 25, 2, 15, 0, 0, time.UTC)),
		Empty: viewmodel.EmptyState{
			Title: "Kuis tidak sedang berlangsung",
			Body:  "Monitor hanya tersedia selama kuis dibuka.",
			Icon:  "clock",
		},
	}
	html := pageBody(t, renderPage(t, "pages/teacher/monitor.html",
		viewmodel.PageData{Base: viewmodel.Base{Title: "Monitor kuis"}, Data: page}))

	stack := between(t, html, `<div class="page-stack"`, `<header class="page-head">`)
	if !strings.Contains(stack, `href="/teacher/quizzes/9"`) ||
		!strings.Contains(stack, "Kembali ke detail kuis") ||
		!strings.Contains(stack, "#i-arrow-left") ||
		!strings.Contains(stack, "link-quiet") {
		t.Errorf("monitor back affordance is not the quiet labelled link: %q", stack)
	}

	head := between(t, html, `<header class="page-head">`, `</header>`)
	if strings.Contains(head, "<a ") || strings.Contains(head, "btn ") {
		t.Errorf("monitor header must carry no actions: %q", head)
	}
	for _, gone := range []string{"Hasil", "Penilaian"} {
		if strings.Contains(head, gone) {
			t.Errorf("monitor header still repeats %q", gone)
		}
	}

	if empty := between(t, html, `<div class="empty-panel">`, `</div>`); strings.Contains(empty, "<a ") {
		t.Errorf("monitor empty state repeats the back link as an action: %q", empty)
	}
}

// TestGradingBackLinkOnlyWhenScoped pins §3.4: the queue is a top-level
// destination, so only a quiz-scoped queue gets a back link, and it sits first
// in reading order above the heading.
func TestGradingBackLinkOnlyWhenScoped(t *testing.T) {
	scoped := viewmodel.GradingPage{BackHref: "/teacher/quizzes/9/results"}
	html := pageBody(t, renderPage(t, "pages/teacher/grading.html",
		viewmodel.PageData{Base: viewmodel.Base{Title: "Penilaian esai"}, Data: scoped}))

	stack := between(t, html, `<div class="page-stack">`, `<div class="page-head">`)
	if !strings.Contains(stack, `href="/teacher/quizzes/9/results"`) {
		t.Errorf("scoped grading back link missing: %q", stack)
	}
	if !strings.Contains(stack, "Kembali ke hasil kuis") || !strings.Contains(stack, "#i-arrow-left") ||
		!strings.Contains(stack, "link-quiet") {
		t.Errorf("scoped grading back affordance must be the quiet labelled link: %q", stack)
	}
	if strings.Contains(stack, "<button") {
		t.Error("the back affordance must be a link, never a button")
	}

	unscoped := viewmodel.GradingPage{}
	html = pageBody(t, renderPage(t, "pages/teacher/grading.html",
		viewmodel.PageData{Base: viewmodel.Base{Title: "Penilaian esai"}, Data: unscoped}))
	stack = between(t, html, `<div class="page-stack">`, `<div class="page-head">`)
	if strings.Contains(stack, "link-quiet") || strings.Contains(stack, "Kembali") {
		t.Errorf("unscoped grading queue must have no back link: %q", stack)
	}
}

// TestGradingHistoryFormIsEditable pins the history tab's template contract
// (grading-history spec §5): the graded detail is a prefilled, version-guarded
// edit form, while the pending detail keeps the first-grade labels.
func TestGradingHistoryFormIsEditable(t *testing.T) {
	gradedAt := viewmodel.NewLocalTime(time.Date(2026, 10, 24, 1, 0, 0, 0, time.UTC))
	updatedAt := viewmodel.NewLocalTime(time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC))
	submittedAt := viewmodel.NewLocalTime(time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC))
	stored := &viewmodel.Score{Percent: 86}
	tabs := []viewmodel.FilterTab{
		{Label: "Menunggu", Href: "/teacher/grading", Count: 2},
		{Label: "Sudah dinilai", Href: "/teacher/grading?state=graded", Active: true, Count: 3},
	}
	gradedPage := viewmodel.GradingPage{
		Queue: viewmodel.GradingQueue{
			Counts: viewmodel.GradingCounts{Pending: 2, GradedToday: 1, Graded: 3},
			Rows: viewmodel.NewList([]viewmodel.GradingRow{{
				AnswerID:    7,
				StudentName: "Eka Putra",
				ClassLabel:  "XI RPL",
				QuizTitle:   "Ujian Esai",
				Score:       stored,
				ScoreLabel:  "85,5",
				GradedLabel: "Dinilai 24 Okt 2026",
				EditedLabel: "Diubah 25 Okt 2026",
				Href:        "/teacher/grading/7?state=graded",
			}}, 1, 20),
			Tabs:  tabs,
			Query: viewmodel.QueryState{Tab: "graded"},
		},
		Selected: &viewmodel.GradingDetail{
			Answer: viewmodel.EssayAnswer{
				ID:           7,
				StudentName:  "Eka Putra",
				ClassLabel:   "XI RPL",
				QuizTitle:    "Ujian Esai",
				QuestionBody: "Jelaskan A",
				Text:         "Jawaban Eka",
				SubmittedAt:  submittedAt,
				State:        viewmodel.EssayGraded,
				Score:        stored,
				Feedback:     "Bagus",
				GradedAt:     &gradedAt,
				UpdatedAt:    &updatedAt,
				Edited:       true,
				Version:      3,
			},
			Attempt:    viewmodel.AttemptHeader{AttemptNo: 1, MaxAttempts: 1},
			Score:      stored,
			ScoreInput: "85,5",
			Feedback:   "Bagus",
			Action:     "/teacher/grading/7?state=graded",
			BackHref:   "/teacher/grading?state=graded",
			NextHref:   "/teacher/grading/8?state=graded",
		},
	}
	html := pageBody(t, renderPage(t, "pages/teacher/grading.html",
		viewmodel.PageData{Base: viewmodel.Base{Title: "Penilaian esai"}, Data: gradedPage}))

	// The history tab is active and its rows show score, graded, and edited.
	if !strings.Contains(html, `href="/teacher/grading?state=graded"`) ||
		!strings.Contains(html, `aria-current="page"`) {
		t.Error("the graded tab link is missing or not active")
	}
	if !strings.Contains(html, "85,5") || !strings.Contains(html, "Dinilai ") ||
		!strings.Contains(html, "Diubah ") {
		t.Error("history row is missing its score/graded/edited labels")
	}
	if !strings.Contains(html, ">85,5%<") {
		t.Error("graded detail header must show the one-decimal score")
	}

	// The form is an edit: value round-trips, the version rides along, and the
	// submit/skip words name the edit job.
	form := between(t, html, `<form class="grading-form"`, `</form>`)
	if !strings.Contains(form, `value="85,5"`) {
		t.Errorf("score input does not round-trip 85,5: %q", form)
	}
	if !strings.Contains(form, `type="text"`) || strings.Contains(form, `type="number"`) {
		t.Error("score input must be text: a number input silently drops the comma decimal")
	}
	if !strings.Contains(form, `name="version" value="3"`) {
		t.Errorf("version hidden input missing: %q", form)
	}
	if !strings.Contains(form, "Simpan perubahan") || strings.Contains(form, "Simpan nilai") {
		t.Errorf("graded submit label is wrong: %q", form)
	}
	if !strings.Contains(form, "Berikutnya") || strings.Contains(form, ">Lewati<") {
		t.Errorf("graded skip label is wrong: %q", form)
	}
	if !strings.Contains(form, "Perubahan menggantikan nilai sebelumnya") {
		t.Error("graded form is missing its overwrite note")
	}
	if strings.Contains(html, "Pilih esai untuk dinilai") {
		t.Error("graded tab rendered the pending placeholder")
	}

	// The same template keeps the first-grade form for the pending tab.
	pendingPage := gradedPage
	pendingPage.Queue.Query.Tab = "pending"
	pendingPage.Selected = &viewmodel.GradingDetail{
		Answer: viewmodel.EssayAnswer{
			ID:           9,
			StudentName:  "Ayu Lestari",
			QuizTitle:    "Ujian Esai",
			QuestionBody: "Jelaskan B",
			SubmittedAt:  submittedAt,
			State:        viewmodel.EssayUngraded,
		},
		Attempt:  viewmodel.AttemptHeader{AttemptNo: 1, MaxAttempts: 1},
		Action:   "/teacher/grading/9",
		BackHref: "/teacher/grading",
		NextHref: "/teacher/grading/10",
	}
	html = pageBody(t, renderPage(t, "pages/teacher/grading.html",
		viewmodel.PageData{Base: viewmodel.Base{Title: "Penilaian esai"}, Data: pendingPage}))
	form = between(t, html, `<form class="grading-form"`, `</form>`)
	if strings.Contains(form, `name="version"`) {
		t.Errorf("ungraded form carries a version token: %q", form)
	}
	if !strings.Contains(form, "Simpan nilai") || strings.Contains(form, "Simpan perubahan") {
		t.Errorf("pending submit label is wrong: %q", form)
	}
	if !strings.Contains(form, ">Lewati<") || strings.Contains(form, ">Berikutnya<") {
		t.Errorf("pending skip label is wrong: %q", form)
	}
	if strings.Contains(form, "Perubahan menggantikan nilai sebelumnya") {
		t.Error("pending form carries the graded overwrite note")
	}

	// Field errors are inline and wired to their controls.
	pendingPage.Selected.ScoreInput = "101"
	html = pageBody(t, renderPage(t, "pages/teacher/grading.html", viewmodel.PageData{
		Base: viewmodel.Base{
			Title: "Penilaian esai",
			Errors: viewmodel.FieldErrors{
				"score":    "Nilai harus antara 0 dan 100.",
				"feedback": "Umpan balik maksimal 1000 karakter.",
			},
		},
		Data: pendingPage,
	}))
	if !strings.Contains(html, `id="grading-score-error"`) ||
		!strings.Contains(html, `id="grading-feedback-error"`) {
		t.Error("inline field errors are missing")
	}
	if !strings.Contains(html, `aria-invalid="true"`) ||
		!strings.Contains(html, "grading-score-help grading-score-error") ||
		!strings.Contains(html, "grading-feedback-help grading-feedback-error") {
		t.Error("field errors are not wired through aria attributes")
	}
	if !strings.Contains(html, `value="101"`) {
		t.Error("a 422 re-render must echo the submitted score")
	}

	// The graded tab's empty right pane names the history job.
	emptyHistory := viewmodel.GradingPage{
		Queue: viewmodel.GradingQueue{
			Tabs:  tabs,
			Query: viewmodel.QueryState{Tab: "graded"},
		},
	}
	html = pageBody(t, renderPage(t, "pages/teacher/grading.html",
		viewmodel.PageData{Base: viewmodel.Base{Title: "Penilaian esai"}, Data: emptyHistory}))
	if !strings.Contains(html, "Pilih esai yang sudah dinilai") ||
		!strings.Contains(html, "memperbarui nilainya") {
		t.Error("the graded empty pane does not name the history job")
	}
}
