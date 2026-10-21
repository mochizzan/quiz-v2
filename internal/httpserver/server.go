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

// New builds a Fiber app with the routes for this task registered: every
// route in the package registry (routes.go, one routes_<group>.go file per
// page group) plus GET /healthz returning plain "ok" while its pinger reports
// healthy, and a final 404 catch-all. The app is returned unstarted so tests
// can drive it with app.Test; cfg is consumed by the process entrypoint that
// calls Listen.
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

	// The page registry (routes.go) is turned into Fiber routes: one handler
	// per registered PageRoute, closed over its own route value. Pages live in
	// one routes_<group>.go file each and self-register from init(), so
	// parallel page tasks never contend on this file.
	for _, route := range RegisteredPages() {
		r := route
		handler := func(c fiber.Ctx) error {
			return renderPage(c, views, r)
		}
		if r.Method == fiber.MethodPost {
			app.Post(r.Path, handler)
			continue
		}
		app.Get(r.Path, handler)
	}

	app.Get("/healthz", func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), healthzTimeout)
		defer cancel()
		if err := pinger.Ping(ctx); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("db unavailable")
		}
		return c.Status(fiber.StatusOK).SendString("ok")
	})

	// 404 catch-all. Registered LAST (after /healthz and every page) so it
	// only sees requests no route claimed — a Use middleware registered
	// earlier would shadow them. It renders the styled error page when that
	// template exists; while Task 6 has not landed pages/error/404.html yet,
	// the render fails and the plain-text fallback keeps the app functional.
	app.Use(func(c fiber.Ctx) error {
		var buf bytes.Buffer
		if err := views.Render(&buf, "pages/error/404.html", PageData{}); err == nil {
			return c.Status(fiber.StatusNotFound).Type("html").Send(buf.Bytes())
		}
		return c.Status(fiber.StatusNotFound).SendString("halaman tidak ditemukan")
	})

	_ = cfg // Addr is used by the caller (main), not the app itself.

	return app
}

// pingerFunc adapts a func(context.Context) error to the Pinger interface.
type pingerFunc func(context.Context) error

// Ping calls f.
func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }
