// Package httpserver builds the Fiber HTTP application: route registration,
// request handlers and server-side rendering through the view loader.
package httpserver

import (
	"bytes"
	"context"
	"sort"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"

	"quiz-v2/internal/config"
	"quiz-v2/internal/viewmodel"
)

// defaultTemplatesDir is the template directory relative to the process
// working directory (repo root under go test, WORKDIR /app in the container).
const defaultTemplatesDir = "./templates"

// healthzTimeout bounds the DB ping performed by GET /healthz so a hung
// database answers 503 instead of holding the request open.
const healthzTimeout = 2 * time.Second

// Pinger is the liveness dependency of GET /healthz.
type Pinger interface {
	Ping(context.Context) error
}

// New builds a Fiber app with every registered page route, GET /healthz and a
// final 404 catch-all. The app is returned unstarted so tests can drive it
// with app.Test.
func New(cfg config.Config) *fiber.App {
	return NewWithStore(cfg, pingerFunc(func(context.Context) error { return nil }))
}

// NewWithStore is New with GET /healthz wired to pinger.
func NewWithStore(cfg config.Config, pinger Pinger) *fiber.App {
	app := fiber.New()
	views := NewViewLoader(defaultTemplatesDir)

	// Serve the vendored carrier assets (Bootstrap 6, jQuery 4, Gotham
	// Rounded, app.js) with a one-day cache.
	app.Use("/static", static.New("./static", static.Config{MaxAge: 86400}))

	// Register static paths before parameterised ones and longer parameterised
	// paths before their prefixes. Fiber matches in registration order, so this
	// is what keeps /teacher/quizzes/new away from /teacher/quizzes/:id.
	routes := RegisteredPages()
	sort.SliceStable(routes, func(i, j int) bool { return routeLess(routes[i], routes[j]) })

	for _, route := range routes {
		r := route
		demo := cfg.Demo
		handler := func(c fiber.Ctx) error {
			return renderPage(c, views, r, newPageRequest(c, r.Path, demo))
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

	// 404 catch-all, registered LAST so it only sees requests no page claimed.
	app.Use(func(c fiber.Ctx) error {
		data := viewmodel.PageData{
			Base: viewmodel.Base{
				Title: "Halaman tidak ditemukan",
				Demo:  cfg.Demo,
				CSRF:  CSRFPlaceholder,
			},
			Data: viewmodel.ErrorPage{
				Code:     404,
				Title:    "Halaman tidak ditemukan",
				Message:  "Alamat yang Anda tuju tidak tersedia atau sudah dipindahkan.",
				HomeHref: "/login",
			},
		}
		var buf bytes.Buffer
		if err := views.Render(&buf, "pages/error/404.html", data); err == nil {
			return c.Status(fiber.StatusNotFound).Type("html").Send(buf.Bytes())
		}
		return c.Status(fiber.StatusNotFound).SendString("halaman tidak ditemukan")
	})

	return app
}

// pingerFunc adapts a func(context.Context) error to the Pinger interface.
type pingerFunc func(context.Context) error

// Ping calls f.
func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }
