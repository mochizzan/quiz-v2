package httpserver

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/viewmodel"
)

// PageRoute describes one server-rendered route. GET routes carry a Build
// function that returns the page model; POST routes carry an Action that
// validates nothing server-side (UI-only build) and returns the PRG redirect
// target. Redirect is the fixed-target shortcut for static redirects such as
// GET / → /login.
type PageRoute struct {
	Method   string
	Path     string
	Template string
	Nav      string
	Build    func(PageRequest) viewmodel.PageData
	Action   func(PageRequest) string
	Redirect string
	// Handler is the escape hatch for non-page endpoints (SSE stream, 204
	// beacon). When set it runs instead of the template/redirect path.
	Handler func(fiber.Ctx) error
}

// PageRequest is the request-scoped data a builder may read: path params, the
// query map, submitted form values, and whether APP_DEMO is on. Builders never
// touch Fiber directly, which keeps page fixtures testable and templates pure.
type PageRequest struct {
	Params map[string]string
	Query  map[string]string
	Form   url.Values
	Demo   bool
}

// Param returns a path parameter or "".
func (r PageRequest) Param(name string) string { return r.Params[name] }

// IntParam parses a path parameter, returning 0 when it is absent or invalid.
func (r PageRequest) IntParam(name string) int64 {
	n, _ := strconv.ParseInt(r.Params[name], 10, 64)
	return n
}

// State returns the demo-only ?state= preview key. In production the key is
// ignored entirely so a hand-edited URL cannot change the rendered data.
func (r PageRequest) State() string {
	if !r.Demo {
		return ""
	}
	return r.Query["state"]
}

// FlashKey returns the closed ?flash= key, if any.
func (r PageRequest) FlashKey() string { return r.Query["flash"] }

// PageData is the template data every page receives.
type PageData = viewmodel.PageData

// CSRFPlaceholder is the token the UI-only forms carry until the backend
// issues real per-session tokens.
const CSRFPlaceholder = "demo-csrf-token"

// pageRoutes is the package-level route registry. Each route group registers
// itself from init() in its own routes_<group>.go file; nobody edits server.go
// to add a page.
var pageRoutes []PageRoute

// registerPage adds a route to the registry.
func registerPage(r PageRoute) {
	pageRoutes = append(pageRoutes, r)
}

// RegisteredPages returns a defensive copy of the registry.
func RegisteredPages() []PageRoute {
	pages := make([]PageRoute, len(pageRoutes))
	copy(pages, pageRoutes)
	return pages
}

// page builds the common chrome for a fixture-backed page.
func page(r PageRequest, title, nav string, user *viewmodel.CurrentUser, data any) viewmodel.PageData {
	return viewmodel.PageData{
		Base: viewmodel.Base{
			Title:  title,
			Active: nav,
			User:   user,
			Flash:  viewmodel.FlashFor(r.FlashKey()),
			CSRF:   CSRFPlaceholder,
			Demo:   r.Demo,
		},
		Data: data,
	}
}

// action is the one-line PRG helper: redirect to path with a flash key.
func action(path, flash string) func(PageRequest) string {
	return func(PageRequest) string {
		if flash == "" {
			return path
		}
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		return path + sep + "flash=" + flash
	}
}

// pageNumber reads ?page= and clamps bad or missing input to the first page.
func pageNumber(r PageRequest) int {
	n, err := strconv.Atoi(r.Query["page"])
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// withParam substitutes :name tokens in a redirect target from the request.
func withParam(path string, r PageRequest) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = r.Params[strings.TrimPrefix(p, ":")]
		}
	}
	return strings.Join(parts, "/")
}

// renderPage is the single page handler. Static redirects win; POST actions
// redirect 303; GET builders render through the view loader. A render error
// maps to a plain-text 500, never a stack trace.
func renderPage(c fiber.Ctx, views *ViewLoader, route PageRoute, req PageRequest) error {
	if route.Handler != nil {
		return route.Handler(c)
	}
	if route.Redirect != "" {
		return c.Redirect().Status(fiber.StatusSeeOther).To(route.Redirect)
	}
	if route.Method == fiber.MethodPost || route.Action != nil {
		target := ""
		if route.Action != nil {
			target = route.Action(req)
		}
		if target == "" {
			return c.Status(fiber.StatusNotFound).SendString("not found")
		}
		return c.Redirect().Status(fiber.StatusSeeOther).To(target)
	}
	data := viewmodel.PageData{}
	if route.Build != nil {
		data = route.Build(req)
	}
	var buf bytes.Buffer
	if err := views.Render(&buf, route.Template, data); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("internal server error")
	}
	return c.Status(fiber.StatusOK).Type("html").Send(buf.Bytes())
}

// newPageRequest copies Fiber's request into the neutral PageRequest shape.
// The route path is parsed for :params so builders never need Fiber's Ctx.
func newPageRequest(c fiber.Ctx, routePath string, demo bool) PageRequest {
	req := PageRequest{
		Params: map[string]string{},
		Query:  c.Queries(),
		Form:   url.Values{},
		Demo:   demo,
	}
	for _, seg := range strings.Split(routePath, "/") {
		if strings.HasPrefix(seg, ":") {
			name := strings.TrimPrefix(seg, ":")
			req.Params[name] = c.Params(name)
		}
	}
	if c.Method() == fiber.MethodPost {
		c.Request().PostArgs().VisitAll(func(k, v []byte) {
			req.Form.Add(string(k), string(v))
		})
	}
	return req
}

// routeLess orders registration so static paths win over parameterised ones:
// /teacher/quizzes/new must register before /teacher/quizzes/:id, and longer
// parameterised paths before their shorter prefixes. Fiber matches in
// registration order, so this ordering IS the router's priority.
func routeLess(a, b PageRoute) bool {
	ap, bp := strings.Count(a.Path, ":"), strings.Count(b.Path, ":")
	if ap != bp {
		return ap < bp
	}
	as, bs := strings.Count(a.Path, "/"), strings.Count(b.Path, "/")
	if as != bs {
		return as > bs
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return a.Method < b.Method
}
