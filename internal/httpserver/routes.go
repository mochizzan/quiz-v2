package httpserver

import (
	"bytes"

	"github.com/gofiber/fiber/v3"
)

// PageRoute describes one server-rendered page: the HTTP method and path it is
// served on, the template (slash-relative to ./templates) that renders it, an
// optional redirect target, and the active-nav key exposed to templates as
// PageData.Active. Exactly one of Template / Redirect is used: when Redirect
// is non-empty the route answers 303 See Other and Template must be "".
type PageRoute struct {
	Method   string // "GET" or "POST"
	Path     string // e.g. "/login"
	Template string // e.g. "pages/auth/login.html"; must be "" when Redirect != ""
	Redirect string // e.g. "/login"; when set, the handler responds 303 See Other
	Nav      string // active-nav key exposed to templates as PageData.Active
}

// PageData is the template data every page receives; nav partials highlight
// the current section via {{if eq .Active "<key>"}}.
type PageData struct {
	Active string
}

// pageRoutes is the package-level route registry. Pages self-register through
// registerPage from an init() in their own routes_<group>.go file (one file
// per route group), which is the convention that makes parallel page tasks
// conflict-free: no two tasks ever edit the same Go file, because nobody edits
// server.go to add a page.
var pageRoutes []PageRoute

// registerPage adds a route to the registry. It is called from init() in the
// routes_<group>.go files (one file per route group, e.g. routes_home.go,
// routes_auth_public.go); server.go's NewWithStore turns the registry into
// Fiber routes for every app instance, so registering does not depend on
// which constructor built the app.
func registerPage(r PageRoute) {
	pageRoutes = append(pageRoutes, r)
}

// RegisteredPages returns a defensive copy of the registry for the
// conformance suite (Task 15): a caller may sort or mutate the result without
// corrupting the package registry.
func RegisteredPages() []PageRoute {
	pages := make([]PageRoute, len(pageRoutes))
	copy(pages, pageRoutes)
	return pages
}

// renderPage is the single page handler: a Redirect route answers 303 See
// Other (Fiber v3 builder API — c.Redirect(path, code) does not exist), and a
// Template route executes the template through the view loader with
// PageData{Active: r.Nav}. A render error (missing/unparsable template) maps
// to a plain-text 500, never a stack trace.
func renderPage(c fiber.Ctx, views *ViewLoader, r PageRoute) error {
	if r.Redirect != "" {
		return c.Redirect().Status(fiber.StatusSeeOther).To(r.Redirect)
	}
	var buf bytes.Buffer
	if err := views.Render(&buf, r.Template, PageData{Active: r.Nav}); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("internal server error")
	}
	return c.Status(fiber.StatusOK).Type("html").Send(buf.Bytes())
}
