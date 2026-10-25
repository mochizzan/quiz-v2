package httpserver

// pipeline_test.go pins the one-shot flash transport (OPS-210): the GET that
// consumes a flash must also delete the cookie it came in, with the same Path
// it was written with.

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/config"

	"github.com/gofiber/fiber/v3"
)

func TestTakeFlashClearsWithMatchingPath(t *testing.T) {
	srv, err := newServer(
		config.Config{SessionSigningKey: strings.Repeat("k", 32)},
		nil,
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	token, err := srv.signer.Token("flash", []byte("graded"), time.Now().UTC())
	if err != nil {
		t.Fatalf("sign flash: %v", err)
	}

	app := fiber.New()
	app.Get("/next", func(c fiber.Ctx) error {
		flash := srv.takeFlash(c)
		if flash == nil {
			return c.SendString("none")
		}
		return c.SendString(flash.Title)
	})

	req := httptest.NewRequest(http.MethodGet, "/next", nil)
	req.AddCookie(&http.Cookie{Name: cookieFlash, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := string(body); got != "Nilai esai tersimpan." {
		t.Errorf("body = %q, want the flash title", got)
	}

	// A deletion without Path omits it from Set-Cookie, so the browser keeps
	// the original "/" cookie and replays the message on every GET.
	clear := strings.ToLower(resp.Header.Get("Set-Cookie"))
	if !strings.Contains(clear, strings.ToLower(cookieFlash)+"=") ||
		!strings.Contains(clear, "path=/") ||
		!strings.Contains(clear, "max-age=0") {
		t.Errorf("Set-Cookie = %q, want a Path=/ Max-Age=0 deletion", clear)
	}
}
