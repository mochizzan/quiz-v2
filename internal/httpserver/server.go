// Package httpserver builds the Fiber HTTP application: route registration,
// request handlers and server-side rendering through the view loader.
package httpserver

import (
	"bytes"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/config"
)

// defaultTemplatesDir is the template directory relative to the process
// working directory (repo root under go test, WORKDIR /app in the container).
const defaultTemplatesDir = "./templates"

// New builds a Fiber app with the routes for this task registered:
// GET / renders templates/hello.html through the view loader (render error →
// 500) and GET /healthz returns plain "ok" (Task 4 makes it DB-aware).
// The app is returned unstarted so tests can drive it with app.Test; cfg is
// consumed by the process entrypoint that calls Listen.
func New(cfg config.Config) *fiber.App {
	app := fiber.New()
	views := NewViewLoader(defaultTemplatesDir)

	app.Get("/", func(c fiber.Ctx) error {
		var buf bytes.Buffer
		if err := views.Render(&buf, "hello.html", nil); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("internal server error")
		}
		return c.Status(fiber.StatusOK).Type("html").Send(buf.Bytes())
	})

	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusOK).SendString("ok")
	})

	_ = cfg // Addr is used by the caller (Task 6), not the app itself.

	return app
}
