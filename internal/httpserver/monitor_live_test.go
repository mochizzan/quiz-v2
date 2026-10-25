package httpserver_test

// monitor_live_test.go pins the live monitor surface: the page carries the
// stream hooks, the metric/row cells the client patches, and the two page slots
// (noscript refresh + module script) that only an open quiz emits. The client
// itself is checked structurally because there is no browser harness; the same
// approach conformance_test.go uses for app.js.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/viewmodel"
)

func TestMonitorPageRendersLiveMonitorWiring(t *testing.T) {
	attemptID := int64(99)
	started := viewmodel.NewLocalTime(time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC))  // 07.00 WIB
	updated := viewmodel.NewLocalTime(time.Date(2026, 10, 25, 2, 30, 0, 0, time.UTC)) // 09.30 WIB
	generated := viewmodel.NewLocalTime(time.Date(2026, 10, 25, 2, 15, 0, 0, time.UTC))
	score := viewmodel.NewScore(80)

	page := viewmodel.MonitorPage{
		Quiz:        viewmodel.QuizRef{ID: 7, Title: "Kuis Langsung"},
		Progress:    viewmodel.MonitorProgress{Registered: 1, InProgress: 1, Answered: 2, Total: 4},
		StreamHref:  "/teacher/quizzes/7/stream",
		QuizHref:    "/teacher/quizzes/7",
		GeneratedAt: generated,
		Rows: []viewmodel.ParticipantRow{{
			RegistrationID: 42,
			AttemptID:      &attemptID,
			StudentName:    "Ayu Lestari",
			Initials:       "AL",
			ClassLabel:     "X RPL",
			State:          viewmodel.ParticipationInProgress,
			Answered:       2,
			Total:          4,
			FocusLoss:      1,
			Score:          &score,
			StartedAt:      &started,
			UpdatedAt:      &updated,
		}},
	}
	html := renderPage(t, "pages/teacher/monitor.html", viewmodel.PageData{Base: viewmodel.Base{Title: "Monitor kuis"}, Data: page})

	for _, want := range []string{
		`<div class="page-stack" data-monitor data-monitor-stream="/teacher/quizzes/7/stream">`,
		`data-monitor-stale`,
		`data-monitor-reload`,
		`data-monitor-metric="registered"`,
		`data-monitor-metric="in-progress"`,
		`data-monitor-metric="completed"`,
		`data-monitor-metric="answered">2 / 4</span>`,
		`data-monitor-metric="percent">50% dari seluruh jawaban</span>`,
		`data-monitor-progress`,
		`data-monitor-rows`,
		`data-registration-id="42" data-state="in_progress" data-attempt-id="99" data-answered="2" data-total="4"`,
		`data-cell="state"`,
		`data-cell="progress"`,
		`data-cell="score"`,
		`data-cell="focus"`,
		`data-cell="started"`,
		`data-cell="updated"`,
		`data-cell="action"`,
		`<td class="num" data-cell="started">07.00</td>`,
		`<td class="num" data-cell="updated">09.30</td>`,
		`class="score-pill theme-success">80%`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("monitor.html missing %q", want)
		}
	}

	// The live dot and the stream only belong to an open quiz, and the two page
	// slots render where the layout can execute them.
	if !strings.Contains(html, `Langsung`) {
		t.Error("open monitor must announce the live stream")
	}
	noscript := `<noscript><meta http-equiv="refresh" content="15"></noscript>`
	noscriptIdx := strings.Index(html, noscript)
	headEnd := strings.Index(html, "</head>")
	if noscriptIdx < 0 || headEnd < 0 || noscriptIdx > headEnd {
		t.Errorf("15s no-JS refresh must be in <head> (noscript=%d headEnd=%d)", noscriptIdx, headEnd)
	}
	appIdx := strings.Index(html, `<script type="module" src="/static/js/app.js"></script>`)
	monitorIdx := strings.Index(html, `<script type="module" src="/static/js/monitor.js"></script>`)
	if appIdx < 0 || monitorIdx < 0 || monitorIdx < appIdx {
		t.Errorf("monitor.js must load after app.js (app=%d monitor=%d)", appIdx, monitorIdx)
	}

	// The old manual-refresh copy is gone: the screen updates itself.
	for _, gone := range []string{"perbarui halaman", "muat ulang untuk memperbarui data"} {
		if strings.Contains(html, gone) {
			t.Errorf("stale manual-refresh copy %q still renders", gone)
		}
	}
}

func TestClosedMonitorDoesNotOpenAStream(t *testing.T) {
	page := viewmodel.MonitorPage{
		Quiz: viewmodel.QuizRef{ID: 8, Title: "Kuis Selesai"},
		Empty: viewmodel.EmptyState{
			Title: "Kuis tidak sedang berlangsung",
			Body:  "Monitor hanya tersedia selama kuis dibuka.",
		},
	}
	html := renderPage(t, "pages/teacher/monitor.html", viewmodel.PageData{Base: viewmodel.Base{Title: "Monitor kuis"}, Data: page})

	if !strings.Contains(html, `data-monitor-stream=""`) {
		t.Error("closed monitor must render an empty stream hook")
	}
	if strings.Contains(html, "/static/js/monitor.js") {
		t.Error("closed monitor must not load the live client")
	}
	if strings.Contains(html, `http-equiv="refresh"`) {
		t.Error("closed monitor must not schedule the no-JS refresh")
	}
	if strings.Contains(html, "data-monitor-stale") {
		t.Error("closed monitor must not render the disconnect banner")
	}
}

func TestMonitorScriptConsumesTheLiveStream(t *testing.T) {
	path := filepath.FromSlash("static/js/monitor.js")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read monitor.js: %v", err)
	}
	src := string(b)

	for _, want := range []string{
		`document.querySelector("[data-monitor]")`,
		`if (root && streamHref) run(root, streamHref);`,
		`new EventSource(streamHref)`,
		`"snapshot"`,
		`"participant.added"`,
		`"participant.updated"`,
		`"heartbeat"`,
		`"quiz.closed"`,
		`source.close()`,
		`window.location.reload()`,
		`[data-monitor-stale]`,
		`[data-monitor-metric=`,
		`adjustAnswered`,
		`if (!fromSnapshot) adjustAnswered`,
		`row.dataset.answered`,
		`row.dataset.attemptId`,
		`state === "completed"`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("monitor.js missing %q", want)
		}
	}

	// Stream data is untrusted text: the client must build nodes, never parse
	// payload strings as HTML. The SSE payload has no HTML by contract.
	for _, banned := range []string{"innerHTML", "insertAdjacentHTML", "document.write", "outerHTML"} {
		if strings.Contains(src, banned) {
			t.Errorf("monitor.js must not use %q", banned)
		}
	}
}
