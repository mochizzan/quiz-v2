package httpserver

// teacher_routes_test.go pins the transport glue of the teacher-attempts
// surface: domain-error mapping, the 422 draft overlay, and the SSE handshake
// headers.

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/attempts"
	"quiz-v2/internal/config"
	"quiz-v2/internal/viewmodel"
)

const teacherRouteTestKey = "0123456789abcdef0123456789abcdef"

func TestAttemptTeacherErrorMapping(t *testing.T) {
	if err := attemptTeacherError(nil); err != nil {
		t.Errorf("nil = %v, want nil", err)
	}

	var appErr *apperr.Error
	if err := attemptTeacherError(attempts.ErrNotFound); !errors.As(err, &appErr) ||
		appErr.Status() != http.StatusNotFound {
		t.Errorf("ErrNotFound mapped to %v, want a 404 app error", err)
	}

	raw := errors.New("driver exploded")
	appErr = nil
	if err := attemptTeacherError(raw); !errors.As(err, &appErr) ||
		appErr.Status() != http.StatusInternalServerError {
		t.Errorf("raw error mapped to %v, want a 500 app error", err)
	}

	// A typed error keeps its class and identity; it is never re-wrapped.
	typed := apperr.Field("score", attempts.FieldErrRange)
	if got := attemptTeacherError(typed); got != typed {
		t.Errorf("typed error = %v, want the same pointer", got)
	}
}

func TestGradingDraftOverlay(t *testing.T) {
	if got := gradingDraft(nil, attempts.GradeInput{}); got != nil {
		t.Errorf("nil detail = %+v, want nil", got)
	}

	ungraded := &viewmodel.GradingDetail{
		Answer: viewmodel.EssayAnswer{State: viewmodel.EssayUngraded},
	}
	got := gradingDraft(ungraded, attempts.GradeInput{
		ScoreRaw:    "85,5",
		FeedbackRaw: "  Catatan guru  ",
	})
	if got != ungraded {
		t.Fatal("draft replaced the detail pointer")
	}
	// The frozen template renders Score.Int, so the one-decimal submission is
	// echoed as its whole display percent (86), not 85.5.
	if got.Score == nil || got.Score.Int() != 86 {
		t.Errorf("draft score = %v, want 86", got.Score)
	}
	if got.Feedback != "  Catatan guru  " {
		t.Errorf("draft feedback = %q, want the verbatim value", got.Feedback)
	}

	// An invalid score clears the draft score but keeps the feedback.
	invalid := gradingDraft(ungraded, attempts.GradeInput{
		ScoreRaw:    "85,55",
		FeedbackRaw: "Catatan",
	})
	if invalid.Score != nil {
		t.Errorf("invalid draft score = %v, want nil", invalid.Score)
	}
	if invalid.Feedback != "Catatan" {
		t.Errorf("invalid draft feedback = %q", invalid.Feedback)
	}

	// A graded answer is read-only: the stored score and feedback win.
	stored := &viewmodel.Score{Percent: 75}
	graded := &viewmodel.GradingDetail{
		Answer:   viewmodel.EssayAnswer{State: viewmodel.EssayGraded},
		Score:    stored,
		Feedback: "Tersimpan",
	}
	got = gradingDraft(graded, attempts.GradeInput{
		ScoreRaw:    "100",
		FeedbackRaw: "Baru",
	})
	if got.Score != stored || got.Feedback != "Tersimpan" {
		t.Errorf("graded draft = %+v, want the stored score/feedback", got)
	}
}

// TestStreamSlotsCap pins OPS-173/OPS-231: at most ten live connections per
// teacher+quiz key, an independent bucket per quiz, and an idempotent release.
func TestStreamSlotsCap(t *testing.T) {
	slots := newStreamSlots(streamConnectionCap)
	leases := make([]*streamLease, 0, streamConnectionCap)
	for i := 0; i < streamConnectionCap; i++ {
		lease := slots.acquire("7:12")
		if lease == nil {
			t.Fatalf("acquire %d refused below the cap", i+1)
		}
		leases = append(leases, lease)
	}
	if lease := slots.acquire("7:12"); lease != nil {
		t.Fatal("acquire above the cap succeeded")
	}
	// A different quiz (or teacher) has its own bucket.
	other := slots.acquire("7:13")
	if other == nil {
		t.Fatal("a second quiz lost its own slots")
	}
	other.Release()

	leases[0].Release()
	leases[0].Release() // idempotent: the connection may release twice
	if lease := slots.acquire("7:12"); lease == nil {
		t.Fatal("released slot was not reusable")
	} else {
		lease.Release()
	}
	for _, lease := range leases[1:] {
		lease.Release()
	}
	// Every entry disappears at zero, so the registry stays bounded.
	if len(slots.active) != 0 {
		t.Errorf("active keys = %d, want 0", len(slots.active))
	}
}

func TestSetStreamHeaders(t *testing.T) {
	app := fiber.New()
	app.Get("/stream", func(c fiber.Ctx) error {
		setStreamHeaders(c)
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/stream", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	for header, want := range map[string]string{
		"Content-Type":      "text/event-stream; charset=utf-8",
		"Cache-Control":     "no-cache, no-store, no-transform",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	} {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("CORS header = %q, want none", got)
	}
}

func TestTeacherStreamAnonymousIsEmpty404(t *testing.T) {
	app := New(config.Config{Addr: ":8080", SessionSigningKey: teacherRouteTestKey})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/teacher/quizzes/123/stream", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("anonymous stream body = %q, want empty", body)
	}
}
