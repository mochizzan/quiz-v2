package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// TestCompressSkipsContractEndpoints: the exclusion paths stay
// identity-encoded even when the client offers gzip and the handler answers
// with compressible bytes, while a non-excluded path on the same app is
// compressed (positive control). The stream handler here deliberately omits
// the real route's no-transform header so the path rule is isolated from it.
func TestCompressSkipsContractEndpoints(t *testing.T) {
	app := fiber.New()
	app.Use(newCompress())
	app.Post("/attempt/:id/beacon", func(c fiber.Ctx) error {
		return c.SendString(strings.Repeat("accepted ", 200))
	})
	app.Get("/teacher/quizzes/:id/stream", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream; charset=utf-8")
		return c.SendString(strings.Repeat("data: ping\n\n", 100))
	})
	app.Get("/teacher/quizzes/:id/export.xlsx", func(c fiber.Ctx) error {
		return c.SendString(strings.Repeat("PK\x03\x04", 100))
	})
	app.Get("/avatars/:name", func(c fiber.Ctx) error {
		return c.SendString(strings.Repeat("RIFF####WEBPVP8 ", 100))
	})
	app.Get("/uploads/:name", func(c fiber.Ctx) error {
		return c.SendString(strings.Repeat("\x89PNG\r\n\x1a\n", 100))
	})
	app.Get("/open", func(c fiber.Ctx) error {
		return c.SendString(strings.Repeat("compress me ", 200))
	})

	skips := []struct{ name, method, url string }{
		{"beacon", http.MethodPost, "/attempt/7/beacon"},
		{"stream", http.MethodGet, "/teacher/quizzes/7/stream"},
		{"export xlsx", http.MethodGet, "/teacher/quizzes/7/export.xlsx"},
		{"avatar", http.MethodGet, "/avatars/x.webp"},
		{"upload", http.MethodGet, "/uploads/x.png"},
	}
	for _, tc := range skips {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.url, nil)
			req.Header.Set("Accept-Encoding", "gzip")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if ce := resp.Header.Get("Content-Encoding"); ce != "" {
				t.Fatalf("%s Content-Encoding = %q, want empty", tc.name, ce)
			}
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/open", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if ce := resp.Header.Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("positive control Content-Encoding = %q, want gzip", ce)
	}
}
