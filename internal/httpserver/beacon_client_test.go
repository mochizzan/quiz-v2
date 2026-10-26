package httpserver_test

// beacon_client_test.go pins the client half of ATT-156..ATT-160: both attempt
// surfaces expose the focus endpoint and load the page-scoped client that
// posts to it, and that client reacts only to the events that mean the quiz
// surface left the screen. There is no browser harness, so the wiring is
// checked structurally — the same approach monitor_live_test.go uses for the
// SSE client. The server side of the same route has its own tests in
// attempts/beacon_test.go and beacon_http_test.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quiz-v2/internal/attempts"
	"quiz-v2/internal/viewmodel"
)

const (
	beaconClientPath = "static/js/beacon.js"
	beaconScriptTag  = `<script type="module" src="/static/js/beacon.js"></script>`
	beaconAttemptID  = int64(12)
	beaconHref       = "/attempt/12/beacon"
)

// checkBeaconWiring asserts one attempt surface carries the endpoint hook and
// loads the client after app.js (the shared modules always come first).
func checkBeaconWiring(t *testing.T, name, html string) {
	t.Helper()
	hook := `data-beacon-href="` + beaconHref + `"`
	if !strings.Contains(html, hook) {
		t.Errorf("%s missing the focus beacon hook %s", name, hook)
	}
	appIdx := strings.Index(html, `<script type="module" src="/static/js/app.js"></script>`)
	beaconIdx := strings.Index(html, beaconScriptTag)
	if beaconIdx < 0 {
		t.Errorf("%s must load %s", name, beaconScriptTag)
		return
	}
	if appIdx < 0 || beaconIdx < appIdx {
		t.Errorf("%s must load beacon.js after app.js (app=%d beacon=%d)", name, appIdx, beaconIdx)
	}
}

// TestAttemptPagesExposeTheFocusBeacon renders the question and review screens
// and pins the two halves of the wiring: the endpoint the client posts to, and
// the page-scoped module that posts it.
func TestAttemptPagesExposeTheFocusBeacon(t *testing.T) {
	question := attempts.QuestionPage{
		AttemptQuestion: viewmodel.AttemptQuestion{
			Header: viewmodel.AttemptHeader{
				ID:          beaconAttemptID,
				QuizID:      3,
				QuizTitle:   "Kuis Fokus",
				StudentName: "Ayu Lestari",
				ClassLabel:  "XII-1",
				AttemptNo:   1,
				MaxAttempts: 2,
				Avatar:      viewmodel.Avatar{Initials: "AL"},
			},
			Progress:   viewmodel.AttemptProgress{Answered: 1, Total: 2},
			Position:   1,
			Total:      2,
			Type:       viewmodel.QuestionSingle,
			Body:       "Soal pertama?",
			QuizHref:   "/quiz/abc",
			IsLast:     false,
			BeaconHref: beaconHref,
		},
		QuestionID: 7,
		AnswerHref: "/attempt/12/answer",
	}
	qHTML := renderPage(t, "pages/attempt/question.html", viewmodel.PageData{
		Base: viewmodel.Base{Title: "Kuis Fokus", CSRF: "csrf-token"},
		Data: question,
	})
	checkBeaconWiring(t, "question.html", qHTML)

	review := viewmodel.AttemptReview{
		Header:     question.Header,
		Progress:   viewmodel.AttemptProgress{Answered: 1, Total: 2},
		SubmitHref: "/attempt/12/submit",
		BackHref:   "/attempt/12",
		BeaconHref: beaconHref,
	}
	rHTML := renderPage(t, "pages/attempt/review.html", viewmodel.PageData{
		Base: viewmodel.Base{Title: "Kuis Fokus", CSRF: "csrf-token"},
		Data: review,
	})
	checkBeaconWiring(t, "review.html", rHTML)
}

// TestBeaconClientIsPageScoped: foot.html is shared by every page, so the
// beacon client must not be a global script — it stays inert on the dashboards
// and the teacher screens, exactly like monitor.js.
func TestBeaconClientIsPageScoped(t *testing.T) {
	foot := readPartial(t, "foot")
	if strings.Contains(foot, beaconClientPath) {
		t.Errorf("foot.html must not load %s; only the attempt pages do", beaconClientPath)
	}
}

// TestBeaconClientEmitsFocusSignals pins the client's contract structurally:
// it reads its endpoint from the page, sends every kind as a plain
// navigator.sendBeacon body (the route accepts text/plain or JSON only), and
// listens to the three signals that mean the surface left the screen — plus
// the guards that stop one switch from counting twice and in-page navigation
// from counting at all.
func TestBeaconClientEmitsFocusSignals(t *testing.T) {
	path := filepath.FromSlash(beaconClientPath)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	src := stripJSComments(string(b))

	for _, want := range []string{
		`document.querySelector("[data-beacon-href]")`,
		`navigator.sendBeacon(`,
		`"visibilitychange"`,
		`window.addEventListener("blur"`,
		`"pagehide"`,
		`document.hidden`,
		`"hidden"`,
		`"blur"`,
		`event.submitter`,
		`"autosave"`,
		`closest("a[href]")`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("beacon.js missing %q", want)
		}
	}

	// The client owns one job and one transport: no HTML parsing (the payload
	// and the page are server-rendered) and no second network API beside
	// sendBeacon.
	for _, banned := range []string{
		"innerHTML", "insertAdjacentHTML", "document.write", "outerHTML",
		"fetch(", "XMLHttpRequest", "EventSource", "WebSocket",
	} {
		if strings.Contains(src, banned) {
			t.Errorf("beacon.js must not use %q", banned)
		}
	}
}
