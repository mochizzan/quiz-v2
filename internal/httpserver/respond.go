package httpserver

import (
	"bytes"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/viewmodel"
)

// htmlHeaders are set on every HTML response (OPS-218). They never carry a
// token, account id, or email.
func (s *server) htmlHeaders(c fiber.Ctx) {
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	c.Set("X-Frame-Options", "DENY")
	referrer := "same-origin"
	// The two token-bearing pages must not leak the token onward (IDN-068).
	switch c.Path() {
	case "/auth/pending", "/reset-password":
		referrer = "no-referrer"
	}
	c.Set("Referrer-Policy", referrer)
	// Authenticated and error HTML is never cacheable: back-button after logout
	// must not reveal a signed-in page.
	c.Set(fiber.HeaderCacheControl, "no-store")
}

// render executes one page template and writes it with the given status. A
// template failure becomes the 500 page exactly once; if that also fails, a
// plain body is the last resort so a render error never becomes an empty 200.
func (s *server) render(c fiber.Ctx, template string, data viewmodel.PageData, status int) error {
	s.htmlHeaders(c)
	var buf bytes.Buffer
	if err := s.views.Render(&buf, template, data); err != nil {
		s.logger.Error("render failed", "rule", "OPS-206", "template", template,
			"status", status, "error", err)
		if template == "pages/error/500.html" {
			return c.Status(fiber.StatusInternalServerError).
				Type("text/plain; charset=utf-8").
				SendString(apperr.MsgServerError)
		}
		return s.render(c, "pages/error/500.html", s.errorPage(c, apperr.Internalf(err)), fiber.StatusInternalServerError)
	}
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.Status(status).Send(buf.Bytes())
}

// respondError turns a classified failure into its one page and status. Every
// non-2xx outcome in the system goes through here, so the status table and the
// copy table cannot drift apart (OPS-200…OPS-206).
func (s *server) respondError(c fiber.Ctx, err *apperr.Error) error {
	if err == nil {
		err = apperr.NotFoundf()
	}
	if err.Kind() == apperr.RateLimited {
		c.Set("Retry-After", itoa(err.RetryAfter()))
	}
	if err.Kind() == apperr.Internal {
		s.logger.Error("internal error",
			"rule", err.Rule(),
			"status", err.Status(),
			"path", c.Path(),
			"method", c.Method(),
			"error", err)
	}
	template := errorTemplate(err.Kind())
	return s.render(c, template, s.errorPage(c, err), err.Status())
}

// errorTemplate maps a failure class to its page. 422 is a fallback only: a
// handler that returns a validation error with data re-renders its own form.
func errorTemplate(k apperr.Kind) string {
	switch k {
	case apperr.NotFound:
		return "pages/error/404.html"
	case apperr.Forbidden:
		return "pages/error/403.html"
	case apperr.Validation:
		return "pages/error/422.html"
	case apperr.RateLimited:
		return "pages/error/429.html"
	default:
		return "pages/error/500.html"
	}
}

// errorPage builds the template data for one failure. The body copy is the
// error's own message, which is what keeps the preview route and the real
// failure identical (OPS-217).
func (s *server) errorPage(c fiber.Ctx, err *apperr.Error) viewmodel.PageData {
	code := err.Status()
	title := "Terjadi kesalahan"
	switch err.Kind() {
	case apperr.NotFound:
		title = "Halaman tidak ditemukan"
	case apperr.Forbidden:
		title = "Akses ditolak"
	case apperr.Validation:
		title = "Isian tidak valid"
	case apperr.RateLimited:
		title = "Terlalu banyak percobaan"
	}
	return viewmodel.PageData{
		Base: viewmodel.Base{
			Title: title,
			CSRF:  c.Cookies(cookieCSRF),
		},
		Data: viewmodel.ErrorPage{
			Code:     code,
			Title:    title,
			Message:  err.Message(),
			HomeHref: "/login",
		},
	}
}

// redirect answers 303 with a relative Location. It is the only redirect status
// the application emits (IDN-014, OPS-201).
func (s *server) redirect(c fiber.Ctx, target string) error {
	return c.Redirect().Status(fiber.StatusSeeOther).To(target)
}

// redirectFlash stores the one-shot flash and then redirects.
func (s *server) redirectFlash(c fiber.Ctx, target, flash string) error {
	s.setFlashCookie(c, flash)
	return s.redirect(c, target)
}

// itoa is strconv.Itoa without the import in every caller.
func itoa(n int) string {
	if n <= 0 {
		return "1"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
