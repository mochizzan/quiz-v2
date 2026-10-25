package httpserver

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/viewmodel"
)

// PageRoute describes one server-rendered route.
//
// Load/Act are the production handlers. They receive a *Ctx (identity,
// database, validated CSRF, parsed inputs), return typed data or an
// *apperr.Error, and never touch Fiber or a status code. ActForm is Act with
// the ability to hand back a rendered form when the command fails validation.
//
// Handler/HandlerCtx are the escape hatches for non-page endpoints (the SSE
// stream and the beacon).
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

	// Handler is the raw Fiber escape hatch (the SSE stream).
	Handler func(fiber.Ctx) error
	// HandlerCtx is the escape hatch for non-page endpoints that need the
	// resolved request context: database, session, identity cookie, limiter,
	// and path params. The beacon and the photo upload use it. It wins over
	// Handler when both are set.
	HandlerCtx func(*Ctx) error

	// Stream marks a route whose request body is long-lived: one multi-hundred-
	// megabyte multipart part that must stay open while the codec runs (AVT-023).
	// prepare() skips the ordinary form parse and the CSRF cookie dance for a
	// Stream route and lets its handler own the body; the handler still enforces
	// CSRF itself from the first body part. AVT-016.
	Stream bool
	// Deadline overrides the POST command deadline for a route whose handler
	// legitimately outlives it. A slow single-part upload is not an overrun
	// (AVT-038), so the photo route declares ten minutes here; every other POST
	// keeps OPS-100's commandDeadline.
	Deadline time.Duration
}

// PageData is the template data every page receives.
type PageData = viewmodel.PageData

// CSRFPlaceholder is the token a page renders when it is built outside the HTTP
// pipeline (unit tests, template rendering). A real request always carries a
// signed token from Ctx.CSRF.
const CSRFPlaceholder = "csrf-placeholder"

// pageRoutes is the package-level route registry. Each route group registers
// itself from init() in its own routes_<group>.go file; nobody edits server.go
// to add a page.
var pageRoutes []PageRoute

// registerPage adds a route to the registry.
func registerPage(r PageRoute) {
	pageRoutes = append(pageRoutes, r)
}

// RegisteredPages returns a defensive copy of the registry, with each route's
// render template resolved. Copying keeps the package-global registry
// immutable while the resolved view is what the server and tests consume.
func RegisteredPages() []PageRoute {
	pages := make([]PageRoute, len(pageRoutes))
	copy(pages, pageRoutes)
	resolveTemplates(pages)
	return pages
}

// resolveTemplates fills in Template for an ActForm POST that did not declare
// one. A GET page names its template once; the POST that re-renders that same
// page on a 422 (finish → s.render(route.Template, …)) reuses it, so the path
// string cannot drift between the two. A POST whose form lives on a different
// path than any GET (POST /attempt/:id/answer renders GET /attempt/:id) keeps
// declaring its own Template explicitly.
func resolveTemplates(routes []PageRoute) {
	getTemplate := make(map[string]string, len(routes))
	for _, route := range routes {
		if route.Method == fiber.MethodGet && route.Template != "" {
			getTemplate[route.Path] = route.Template
		}
	}
	for i := range routes {
		if routes[i].Template == "" && routes[i].ActForm != nil {
			routes[i].Template = getTemplate[routes[i].Path]
		}
	}
}

// commandDeadline is OPS-100's request budget for a mutating command. A
// command that exceeds it fails as a 500; a GET is exempt because the SSE
// route is deliberately long-lived.
const commandDeadline = 10 * time.Second

// serveRoute is the single entry point for every registered route. The pipeline
// runs first, then the route's own handler.
func (s *server) serveRoute(c fiber.Ctx, route PageRoute) error {
	if route.Method == fiber.MethodPost {
		deadline := commandDeadline
		if route.Deadline > 0 {
			deadline = route.Deadline
		}
		ctx, cancel := context.WithTimeout(c.Context(), deadline)
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

	switch {
	case route.Method == fiber.MethodGet && route.Load != nil:
		data, err := route.Load(x)
		return s.finish(x, route, data, err)
	case route.Method == fiber.MethodPost && route.ActForm != nil:
		data, r, err := route.ActForm(x)
		if err != nil {
			return s.finish(x, route, data, err)
		}
		return s.respondRedirect(c, r)
	case route.Method == fiber.MethodPost && route.Act != nil:
		r, err := route.Act(x)
		if err != nil {
			return s.finish(x, route, viewmodel.PageData{}, err)
		}
		return s.respondRedirect(c, r)
	default:
		return s.respondError(c, apperr.NotFoundf())
	}
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
