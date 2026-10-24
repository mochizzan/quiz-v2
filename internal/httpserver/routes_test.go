package httpserver

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/viewmodel"

	"github.com/gofiber/fiber/v3"
)

// TestFinishMapsUntypedErrorTo500 pins OPS-215 at the transport seam. A handler
// that returns a bare error (an integrity witness, a driver failure) must
// render a logged 500. Before apperr.From, finish passed a nil errors.As target
// to respondError, which invents a 404 for a server fault.
func TestFinishMapsUntypedErrorTo500(t *testing.T) {
	srv, err := newServer(
		config.Config{SessionSigningKey: strings.Repeat("k", 32)},
		nil,
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	app := fiber.New()
	app.Get("/boom", func(c fiber.Ctx) error {
		return srv.finish(&Ctx{c: c}, PageRoute{}, viewmodel.PageData{}, errors.New("integrity witness"))
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/boom", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: an unclassified error is a server fault, not a missing row",
			resp.StatusCode)
	}
}

// TestRootRedirectsToLogin pins IDN-C01: the bare host is always a 303, and the
// signed-out target is the single sign-in screen. The redirect is a handler
// because a must-change session also needs a one-shot flash.
func TestRootRedirectsToLogin(t *testing.T) {
	app := New(config.Config{Addr: ":8080", SessionSigningKey: strings.Repeat("k", 32)})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/login" {
		t.Errorf("Location = %q, want /login", got)
	}
}

// TestRegisteredPagesIsCopy pins the defensive copy: mutating the returned
// slice must not corrupt the package registry seen by later calls.
func TestRegisteredPagesIsCopy(t *testing.T) {
	first := RegisteredPages()
	if len(first) == 0 {
		t.Fatal("RegisteredPages() is empty")
	}
	original := first[0]
	originalLen := len(first)
	first[0] = PageRoute{Method: "POST", Path: "/__mutated"}
	second := RegisteredPages()
	if len(second) != originalLen {
		t.Fatalf("len = %d after mutating copy, want %d", len(second), originalLen)
	}
	if second[0].Method != original.Method || second[0].Path != original.Path || second[0].Template != original.Template {
		t.Errorf("registry mutated through the copy: got %s %s (%s), want %s %s (%s)",
			second[0].Method, second[0].Path, second[0].Template,
			original.Method, original.Path, original.Template)
	}
}

// TestRouteLessOrdersStaticBeforeParams guards the Fiber registration order:
// static paths must beat parameterised paths, and deeper parameterised paths
// beat their prefixes.
func TestRouteLessOrdersStaticBeforeParams(t *testing.T) {
	cases := []struct {
		name string
		a, b PageRoute
	}{
		{
			name: "static new before id",
			a:    PageRoute{Method: "GET", Path: "/teacher/quizzes/new"},
			b:    PageRoute{Method: "GET", Path: "/teacher/quizzes/:id"},
		},
		{
			name: "results before id",
			a:    PageRoute{Method: "GET", Path: "/teacher/quizzes/:id/results"},
			b:    PageRoute{Method: "GET", Path: "/teacher/quizzes/:id"},
		},
		{
			name: "questions qid before questions new? static wins",
			a:    PageRoute{Method: "GET", Path: "/teacher/quizzes/:id/questions/new"},
			b:    PageRoute{Method: "GET", Path: "/teacher/quizzes/:id/questions/:qid"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !routeLess(tc.a, tc.b) {
				t.Errorf("routeLess(%q, %q) = false, want true", tc.a.Path, tc.b.Path)
			}
			if routeLess(tc.b, tc.a) {
				t.Errorf("routeLess(%q, %q) = true, want false", tc.b.Path, tc.a.Path)
			}
		})
	}
}

// TestRouteLessOrdersStaticBeforeParameterised is covered above.
