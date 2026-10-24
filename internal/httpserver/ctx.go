package httpserver

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/authn"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// Cookie names are exported so tests and other packages can set and clear the
// same cookies the pipeline issues. They are fixed by IDN-067.
const (
	// CookieSession carries the raw session token.
	CookieSession = cookieSession
	// CookieCSRF carries the signed double-submit token.
	CookieCSRF = cookieCSRF
)

// RouteClass returns the declared authorization class for a method+path pair.
// It is exported for the conformance suite, which asserts the matrix matches
// the pinned surface (IDN-015).
func RouteClass(method, path string) (AuthClass, bool) { return classFor(method, path) }

// Cookie names are fixed by IDN-067. They never carry user-controlled text.
const (
	cookieSession = "quiz_session"
	cookieCSRF    = "quiz_csrf"
	cookieFlash   = "quiz_flash"
	cookiePending = "quiz_pending"
)

// Redirect is what a POST handler returns on success: a relative target and an
// optional closed flash key. The HTTP layer turns it into a 303 and sets the
// one-shot flash cookie (OPS-201, OPS-210).
type Redirect struct {
	Path  string
	Flash string
}

// Ctx is the request-scoped value a handler receives. It carries the resolved
// identity, the database, the validated CSRF token, and the parsed inputs, but
// never the Fiber context, so a handler can be tested without an HTTP server.
//
// A handler does not write a status code or a header. It returns data or an
// *apperr.Error and the HTTP layer decides.
type Ctx struct {
	DB      *store.DB
	User    *viewmodel.CurrentUser
	Session *authn.Session
	CSRF    string
	IP      string
	ReqID   string
	Params  map[string]string
	Query   url.Values
	Form    url.Values
	Flash   *viewmodel.Flash

	server *server
	c      fiber.Ctx
}

// Param returns a path parameter or "".
func (x *Ctx) Param(name string) string { return x.Params[name] }

// IntParam parses a path parameter, returning 0 when absent or invalid.
func (x *Ctx) IntParam(name string) int64 {
	n, _ := strconv.ParseInt(x.Params[name], 10, 64)
	return n
}

// QueryValue returns one query value, with invalid input falling back to "".
// Query values never 422 (R-21).
func (x *Ctx) QueryValue(name string) string { return x.Query.Get(name) }

// Value returns one submitted form value, trimmed of leading/trailing space.
func (x *Ctx) Value(name string) string { return strings.TrimSpace(x.Form.Get(name)) }

// RawValue returns a submitted form value exactly as sent, for passwords.
func (x *Ctx) RawValue(name string) string { return x.Form.Get(name) }

// Has reports whether the form submitted a non-empty value for name.
func (x *Ctx) Has(name string) bool { return strings.TrimSpace(x.Form.Get(name)) != "" }

// Checked reports whether a checkbox was submitted as "on"/"1"/"true".
func (x *Ctx) Checked(name string) bool {
	switch strings.ToLower(x.Form.Get(name)) {
	case "on", "1", "true", "yes":
		return true
	}
	return false
}

// IsAuthenticated reports whether a live session was resolved.
func (x *Ctx) IsAuthenticated() bool { return x.Session != nil }

// IsTeacher reports an active teacher session.
func (x *Ctx) IsTeacher() bool { return x.Session != nil && x.Session.IsTeacher() }

// IsStudent reports an active student session.
func (x *Ctx) IsStudent() bool { return x.Session != nil && x.Session.IsStudent() }

// Redirect builds the success result for a POST handler.
func (x *Ctx) Redirect(path, flash string) Redirect {
	return Redirect{Path: path, Flash: flash}
}

// Page assembles the chrome for a production handler. It fills exactly the
// shared fields the layout reads; a handler supplies only the title, the active
// nav key and the screen model.
func (x *Ctx) Page(title, nav string, data any) viewmodel.PageData {
	return viewmodel.PageData{
		Base: viewmodel.Base{
			Title:  title,
			Active: nav,
			User:   x.User,
			Flash:  x.Flash,
			CSRF:   x.CSRF,
		},
		Data: data,
	}
}

// Signer exposes the application signer so a domain can mint its own
// purpose-bound tokens (pending teacher, join ticket, reset reveal) without
// duplicating the key or the format.
func (x *Ctx) Signer() *secure.Signer { return x.server.signer }

// Limiter exposes the process-local fixed-window limiter (OPS-231). Every
// domain keys it by purpose plus identity and never shares a bucket between
// purposes.
func (x *Ctx) Limiter() *secure.Limiter { return x.server.limiter }

// UploadsDir returns the configured upload root.
func (x *Ctx) UploadsDir() string { return x.server.cfg.UploadsDir }

// SeeOther makes a handler redirect instead of rendering. A GET uses it for the
// role router and for idempotent PRG; a POST should return Redirect instead.
func (x *Ctx) SeeOther(path, flash string) error {
	return &RedirectError{To: Redirect{Path: path, Flash: flash}}
}

// Now returns the current time. Handlers use the database clock for writes;
// this is only for token windows, which are app-clock by design.
func (x *Ctx) Now() time.Time { return time.Now().UTC() }

// SetSessionCookie writes the session cookie with the pinned attributes
// (IDN-067): HttpOnly, SameSite=Lax, Path=/, Secure from configuration, and no
// Max-Age, so it dies with the browser session.
func (x *Ctx) SetSessionCookie(token string) {
	x.setCookie(cookieSession, token, 0, "/")
}

// ClearSessionCookie removes the session cookie with the same attributes used
// to set it. A mismatched clear leaves the old cookie alive (IDN-069).
func (x *Ctx) ClearSessionCookie() { x.setCookie(cookieSession, "", -1, "/") }

// RotateCSRF mints a fresh CSRF token, stores it in the cookie, and returns it
// for the response body or a caller that needs to re-render a form.
func (x *Ctx) RotateCSRF() (string, error) {
	token, err := x.server.signer.CSRF()
	if err != nil {
		return "", err
	}
	x.CSRF = token
	x.setCookie(cookieCSRF, token, 0, "/")
	return token, nil
}

// SetCookie writes an application cookie with the standard attributes. path
// narrows the scope for the two cookies that need it.
func (x *Ctx) SetCookie(name, value string, maxAge int, path string) {
	x.setCookie(name, value, maxAge, path)
}

// ClearCookie expires a cookie at the same path it was set.
func (x *Ctx) ClearCookie(name, path string) { x.setCookie(name, "", -1, path) }

// setCookie is the single place cookie attributes are decided.
func (x *Ctx) setCookie(name, value string, maxAge int, path string) {
	if path == "" {
		path = "/"
	}
	x.c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		Secure:   x.server.cfg.CookieSecure,
		HTTPOnly: true,
		SameSite: "Lax",
	})
}

// Cookie returns the raw request cookie value, or "" when the request did not
// carry it. Domain adapters use it for cookies the session pipeline does not
// own (the participant identity cookie, PAR-005).
func (x *Ctx) Cookie(name string) string {
	if x.c == nil {
		return ""
	}
	return x.c.Cookies(name)
}

// Context returns the request context a domain command runs under.
func (x *Ctx) Context() context.Context {
	if x.c == nil {
		return context.Background()
	}
	return x.c.Context()
}
