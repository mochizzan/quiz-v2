package httpserver

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/viewmodel"
)

// PageRoute describes one server-rendered route.
//
// Two handler styles exist while the backend is being wired:
//
//   - Load/Act are the production handlers. They receive a *Ctx (identity,
//     database, validated CSRF, parsed inputs), return typed data or an
//     *apperr.Error, and never touch Fiber or a status code.
//   - Build/Action are the fixture handlers the UI build uses. They stay for
//     APP_DEMO previews and are replaced route by route as each domain lands.
//
// Handler is the escape hatch for non-page endpoints (SSE stream, beacon).
type PageRoute struct {
	Method   string
	Path     string
	Template string
	Nav      string

	// Auth is informational: the authoritative matrix is routeClasses, keyed
	// by method+path. It exists so a route file states its own class next to
	// the route it protects.
	Auth AuthClass
	// CSRFExempt is reserved for POST /attempt/:id/beacon; validation rejects
	// any other route that sets it.
	CSRFExempt bool

	Load func(*Ctx) (viewmodel.PageData, error)
	Act  func(*Ctx) (Redirect, error)

	// ActForm is Act with the ability to hand back a rendered form when the
	// command fails validation. A POST that rejects input must answer 422 with
	// the user's values and field errors (plan §7), and Act alone cannot carry
	// data through its error. A route uses either Act or ActForm, never both;
	// ActForm wins if a route sets it.
	ActForm func(*Ctx) (viewmodel.PageData, Redirect, error)

	Build    func(PageRequest) viewmodel.PageData
	Action   func(PageRequest) string
	Redirect string
	// Handler is the raw Fiber escape hatch (the SSE stream).
	Handler func(fiber.Ctx) error
	// HandlerCtx is the escape hatch for non-page endpoints that need the
	// resolved request context: database, session, identity cookie, limiter,
	// and path params. The beacon uses it. It wins over Handler when both are
	// set.
	HandlerCtx func(*Ctx) error
}

// PageRequest is the fixture-era request: path params, the query map, submitted
// form values, and whether APP_DEMO is on. It is retained for Build/Action so
// the existing demo pages keep working; new handlers take *Ctx.
type PageRequest struct {
	Params map[string]string
	Query  map[string]string
	Form   url.Values
	Demo   bool

	csrf  string
	user  *viewmodel.CurrentUser
	flash *viewmodel.Flash
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

// CSRFPlaceholder is the token a page renders when it is built outside the HTTP
// pipeline (unit tests, template rendering). A real request always carries a
// signed token from Ctx.CSRF.
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
	if user == nil {
		user = r.user
	}
	flash := r.flash
	if flash == nil {
		flash = viewmodel.FlashFor(r.FlashKey())
	}
	csrf := r.csrf
	if csrf == "" {
		csrf = CSRFPlaceholder
	}
	return viewmodel.PageData{
		Base: viewmodel.Base{
			Title:  title,
			Active: nav,
			User:   user,
			Flash:  flash,
			CSRF:   csrf,
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

// commandDeadline is OPS-100's request budget for a mutating command. A
// command that exceeds it fails as a 500; a GET is exempt because the SSE
// route is deliberately long-lived.
const commandDeadline = 10 * time.Second

// serveRoute is the single entry point for every registered route. The pipeline
// runs first; then the production handler if one exists, then the fixture
// handler, then the legacy redirect table.
func (s *server) serveRoute(c fiber.Ctx, route PageRoute) error {
	if route.Method == fiber.MethodPost {
		ctx, cancel := context.WithTimeout(c.Context(), commandDeadline)
		defer cancel()
		c.SetContext(ctx)
	}
	x, ok := s.prepare(c, route)
	if !ok {
		return nil
	}
	if route.HandlerCtx != nil {
		return route.HandlerCtx(x)
	}
	if route.Handler != nil {
		return route.Handler(c)
	}

	// Demo mode serves the fixture world; production prefers the real handler.
	// The two never mix, so a fixture can never answer a production request and
	// a real handler can never be skipped in favour of demo data.
	if x.Demo {
		return s.serveFixture(c, x, route)
	}

	switch {
	case route.Method == fiber.MethodGet && route.Load != nil:
		data, err := route.Load(x)
		return s.finish(x, route, data, err)
	case route.Method == fiber.MethodPost && route.ActForm != nil:
		data, r, err := route.ActForm(x)
		if err != nil {
			return s.finish(x, route, data, err)
		}
		return s.redirectFlash(c, r.Path, r.Flash)
	case route.Method == fiber.MethodPost && route.Act != nil:
		r, err := route.Act(x)
		if err != nil {
			return s.finish(x, route, viewmodel.PageData{}, err)
		}
		return s.redirectFlash(c, r.Path, r.Flash)
	default:
		return s.serveFixture(c, x, route)
	}
}

// serveFixture is the fixture-era path: static redirect, POST action, or a
// Build function rendering demo data.
func (s *server) serveFixture(c fiber.Ctx, x *Ctx, route PageRoute) error {
	switch {
	case route.Redirect != "":
		return s.redirect(c, route.Redirect)
	case route.Method == fiber.MethodPost || route.Action != nil:
		target := ""
		if route.Action != nil {
			target = route.Action(x.legacyRequest())
		}
		if target == "" {
			return s.respondError(c, apperr.NotFoundf())
		}
		return s.redirect(c, target)
	case route.Build != nil:
		return s.finish(x, route, route.Build(x.legacyRequest()), nil)
	default:
		return s.respondError(c, apperr.NotFoundf())
	}
}

// legacyRequest adapts *Ctx to the fixture-era PageRequest so Build/Action keep
// receiving the same shape they always did, now with the real identity and
// CSRF token attached.
func (x *Ctx) legacyRequest() PageRequest {
	r := PageRequest{
		Params: x.Params,
		Query:  map[string]string{},
		Form:   x.Form,
		Demo:   x.Demo,
		csrf:   x.CSRF,
		user:   x.User,
		flash:  x.Flash,
	}
	for k, vs := range x.Query {
		if len(vs) > 0 {
			r.Query[k] = vs[0]
		}
	}
	return r
}

// RedirectError is returned by a handler that must answer 303 instead of
// rendering: the role router (IDN-026) and an idempotent GET. It is a transport
// signal, not a business failure, so it is never logged as an error.
type RedirectError struct {
	To Redirect
}

// Error implements error.
func (e *RedirectError) Error() string { return "redirect to " + e.To.Path }

// finish renders a handler's result: its data at 200, or the mapped error page.
func (s *server) finish(x *Ctx, route PageRoute, data viewmodel.PageData, err error) error {
	if err != nil {
		// A GET that must redirect (role router, idempotent PRG) says so with
		// RedirectError rather than rendering a page.
		var redirect *RedirectError
		if errors.As(err, &redirect) {
			return s.redirectFlash(x.c, redirect.To.Path, redirect.To.Flash)
		}
		// A 422 re-renders the submitting form when the handler supplied one;
		// otherwise the generic error page carries the message.
		//
		// From normalises the error first: a handler that returns an untyped
		// error (an integrity witness, a driver failure) must become a logged
		// 500, not a 404. Passing the raw errors.As target to respondError
		// would leave it nil and render "not found" for a server fault.
		appErr := apperr.From(err)
		if appErr.Kind() == apperr.Internal && appErr.Rule() == "" {
			appErr = appErr.WithRule("OPS-215")
		}
		if appErr.Kind() == apperr.Validation && data.Data != nil {
			data.Base.Errors = viewmodel.FieldErrors(appErr.Fields())
			if data.Base.CSRF == "" {
				data.Base.CSRF = x.CSRF
			}
			if data.Base.User == nil {
				data.Base.User = x.User
			}
			data.Base.Demo = x.Demo
			return s.render(x.c, route.Template, data, appErr.Status())
		}
		return s.respondError(x.c, appErr)
	}
	if data.Base.CSRF == "" {
		data.Base.CSRF = x.CSRF
	}
	if data.Base.User == nil {
		data.Base.User = x.User
	}
	if data.Base.Flash == nil {
		data.Base.Flash = x.Flash
	}
	data.Base.Demo = x.Demo
	return s.render(x.c, route.Template, data, fiber.StatusOK)
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

// renderBuffer keeps the render path allocation-light and, more importantly,
// guarantees a template failure produces no partial body.
var renderBuffer bytes.Buffer
