package httpserver

// respond_test.go pins the one success transport exception: a quiet autosave
// answers 204 with no Location and no flash cookie, while every other success
// keeps the 303 + one-shot flash pair (OPS-201, ATT-125).

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"

	"github.com/gofiber/fiber/v3"
)

func TestRespondRedirectNoContentIs204(t *testing.T) {
	srv, err := newServer(
		config.Config{SessionSigningKey: strings.Repeat("k", 32)},
		nil,
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	app := fiber.New()
	app.Post("/autosave", func(c fiber.Ctx) error {
		// Path and Flash are noise on an autosave: the client stays put.
		return srv.respondRedirect(c, Redirect{NoContent: true, Path: "/ignored", Flash: "saved"})
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/autosave", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		t.Errorf("Location = %q, want empty", loc)
	}
	if cookie := resp.Header.Get("Set-Cookie"); strings.Contains(cookie, cookieFlash) {
		t.Errorf("Set-Cookie = %q, want no flash cookie", cookie)
	}
}

func TestRespondRedirectExplicitIs303WithFlash(t *testing.T) {
	srv, err := newServer(
		config.Config{SessionSigningKey: strings.Repeat("k", 32)},
		nil,
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	app := fiber.New()
	app.Post("/save", func(c fiber.Ctx) error {
		return srv.respondRedirect(c, Redirect{Path: "/attempt/7?q=1", Flash: "saved"})
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/save", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/attempt/7?q=1" {
		t.Errorf("Location = %q, want /attempt/7?q=1", loc)
	}
	if cookie := resp.Header.Get("Set-Cookie"); !strings.Contains(cookie, cookieFlash) {
		t.Errorf("Set-Cookie = %q, want the %s flash cookie", cookie, cookieFlash)
	}
}
