// Package httpserver builds the Fiber HTTP application: route registration,
// request handlers and server-side rendering through the view loader.
package httpserver

import (
	"bytes"
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"

	"quiz-v2/internal/config"
)

// defaultTemplatesDir is the template directory relative to the process
// working directory (repo root under go test, WORKDIR /app in the container).
const defaultTemplatesDir = "./templates"

// healthzTimeout bounds the DB ping performed by GET /healthz so a hung
// database answers 503 instead of holding the request open.
const healthzTimeout = 2 * time.Second

// Pinger is the liveness dependency of GET /healthz. *sql.DB satisfies it
// through database/sql, and main adapts the free function store.Ping to it;
// tests supply stubs.
type Pinger interface {
	Ping(context.Context) error
}

// New builds a Fiber app with the routes for this task registered:
// GET / renders templates/hello.html through the view loader (render error →
// 500) and GET /healthz returns plain "ok" while its pinger reports healthy.
// The app is returned unstarted so tests can drive it with app.Test; cfg is
// consumed by the process entrypoint that calls Listen.
// New is NewWithStore with a pinger that always returns nil, so a caller
// without a database keeps the plain 200 /healthz.
func New(cfg config.Config) *fiber.App {
	return NewWithStore(cfg, pingerFunc(func(context.Context) error { return nil }))
}

// NewWithStore is New with GET /healthz wired to pinger: each request pings
// with a 2s timeout and maps the outcome to 200 "ok" (ping succeeded) or 503
// "db unavailable" (ping failed or timed out). A database that drops after
// startup is therefore surfaced through the status code, never fatally.
func NewWithStore(cfg config.Config, pinger Pinger) *fiber.App {
	app := fiber.New()
	views := NewViewLoader(defaultTemplatesDir)

	// Serve the vendored carrier assets (Bootstrap 6, jQuery 4, Plus Jakarta
	// Sans, app.js) with a one-day cache. Fiber v3 has no app.Static: the
	// in-module static middleware intercepts only /static/* and calls
	// c.Next() for a missing file, so a 404 catch-all registered later still
	// sees unresolved /static paths.
	app.Use("/static", static.New("./static", static.Config{MaxAge: 86400}))

	app.Get("/", func(c fiber.Ctx) error {
		var buf bytes.Buffer
		if err := views.Render(&buf, "hello.html", nil); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("internal server error")
		}
		return c.Status(fiber.StatusOK).Type("html").Send(buf.Bytes())
	})

	app.Get("/healthz", func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), healthzTimeout)
		defer cancel()
		if err := pinger.Ping(ctx); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("db unavailable")
		}
		return c.Status(fiber.StatusOK).SendString("ok")
	})

	_ = cfg // Addr is used by the caller (main), not the app itself.

	return app
}

// pingerFunc adapts a func(context.Context) error to the Pinger interface.
type pingerFunc func(context.Context) error

// Ping calls f.
func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }
