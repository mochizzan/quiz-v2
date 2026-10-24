package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/authn"
	"quiz-v2/internal/config"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// Body caps are pinned by OPS-218: one mebibyte for a form post, four kibibytes
// for a beacon, ten mebibytes for a multipart upload. They are checked before
// parsing so an oversized body never reaches validation.
const (
	maxFormBody    = 1 << 20
	maxBeaconBody  = 4 << 10
	maxUploadBody  = 10 << 20
	beaconRouteKey = "POST /attempt/:id/beacon"
)

// server is the per-process HTTP dependency set. It is built once and shared by
// every route handler; no handler reaches a package-level global.
type server struct {
	cfg      config.Config
	db       *store.DB
	views    *ViewLoader
	signer   *secure.Signer
	sessions SessionResolver
	limiter  *secure.Limiter
	logger   *slog.Logger
	// stop is closed when the process begins a graceful shutdown. Every open
	// SSE connection selects on it so a stop does not wait for the next tick
	// (ATT-197). stopOnce makes the close idempotent across repeated Shutdown
	// calls.
	stop     chan struct{}
	stopOnce sync.Once
	// streams caps concurrent SSE connections per teacher+quiz (OPS-173,
	// OPS-231). It is process-local, like the limiter, and entries disappear
	// when their last connection ends.
	streams *streamSlots
	// enforceAuth is false only for the fixture preview build (APP_DEMO with no
	// database), where no session can exist.
	enforceAuth bool
}

// SessionResolver is the part of authn.Store the pipeline needs. It is an
// interface so an integration test can supply a fixed identity without a
// database; production always injects authn.Store.
type SessionResolver interface {
	Resolve(ctx context.Context, token string) (*authn.Session, error)
	Slide(ctx context.Context, id int64) error
}

// demoSigningKey is the fixed key APP_DEMO uses when none is configured. It is
// deliberately not random: a demo restart must not invalidate the forms a
// reviewer already has open. It is never used when APP_DEMO is off, because
// config.Validate refuses to start (IDN-050).
const demoSigningKey = "quiz-v2-demo-only-signing-key-never-production"

// newServer builds the dependency set. A missing or short signing key is an
// error, never a silent fallback (IDN-050); main turns that into a fatal exit.
func newServer(cfg config.Config, db *store.DB, logger *slog.Logger) (*server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	key := []byte(cfg.SessionSigningKey)
	if len(key) == 0 && cfg.Demo {
		key = []byte(demoSigningKey)
	}
	signer, err := secure.NewSigner(key)
	if err != nil {
		return nil, err
	}
	s := &server{
		cfg:         cfg,
		db:          db,
		views:       NewViewLoader(defaultTemplatesDir),
		signer:      signer,
		limiter:     secure.NewLimiter(nil),
		logger:      logger,
		stop:        make(chan struct{}),
		streams:     newStreamSlots(streamConnectionCap),
		enforceAuth: db != nil || !cfg.Demo,
	}
	if db != nil {
		s.sessions = authn.NewStore(db)
	}
	return s, nil
}

// streamStop returns the channel closed at graceful shutdown (ATT-197).
func (s *server) streamStop() <-chan struct{} { return s.stop }

// beginShutdown closes the stream stop channel exactly once. It is the
// OnPreShutdown hook so a graceful Shutdown drains SSE handlers instead of
// waiting for their next 2-second tick.
func (s *server) beginShutdown() {
	s.stopOnce.Do(func() { close(s.stop) })
}

// prepare runs the global pipeline (IDN-020) for one route and returns the
// request context. A false second result means the pipeline already wrote the
// response — a redirect, an error page, or a 404 — and the handler must not run.
//
// Order is load-bearing: body cap, session, CSRF, must-change guard, class.
// CSRF precedes the guard so a cross-site POST from a must-change session fails
// 403 instead of being redirected (IDN-071); the guard precedes the class so a
// must-change session never reaches another domain's handler.
func (s *server) prepare(c fiber.Ctx, route PageRoute) (*Ctx, bool) {
	x := &Ctx{
		DB:     s.db,
		Demo:   s.cfg.Demo,
		ReqID:  requestID(),
		Params: map[string]string{},
		Query:  url.Values{},
		Form:   url.Values{},
		server: s,
		c:      c,
	}

	// Path parameters are derived from the registered pattern, not the raw
	// path, so a builder never sees an unvalidated segment as a param name.
	for _, seg := range strings.Split(route.Path, "/") {
		if strings.HasPrefix(seg, ":") {
			name := strings.TrimPrefix(seg, ":")
			x.Params[name] = c.Params(name)
		}
	}
	c.Request().URI().QueryArgs().VisitAll(func(k, v []byte) {
		x.Query.Add(string(k), string(v))
	})
	if route.Method == fiber.MethodPost {
		parseForm(c, x.Form)
	}
	if ip := authn.ClientIP(c.RemoteAddr().String()); ip != nil {
		x.IP = ip.String()
	}

	if !s.checkBodyCap(c, x, route) {
		return nil, false
	}
	if !s.resolveSession(c, x) {
		return nil, false
	}
	if !s.checkCSRF(c, x, route) {
		return nil, false
	}
	if !s.checkMustChange(c, x, route) {
		return nil, false
	}
	x.Flash = s.takeFlash(c, x.Demo)
	if !s.checkClass(c, x, route) {
		return nil, false
	}
	return x, true
}

// resolveSession reads the session cookie, resolves it to a live account, and
// slides the expiry. A cookie that no longer resolves is cleared and the request
// continues as a visitor; a broken student profile is a 500, never a silent
// zero grade (IDN-043, IDN-047).
func (s *server) resolveSession(c fiber.Ctx, x *Ctx) bool {
	if s.sessions == nil {
		return true
	}
	token := c.Cookies(cookieSession)
	if token == "" {
		return true
	}
	sess, err := s.sessions.Resolve(c.Context(), token)
	if err != nil {
		s.respondError(c, apperr.Internalf(err).WithRule("IDN-047"))
		return false
	}
	if sess == nil {
		x.ClearSessionCookie()
		return true
	}
	x.Session = sess
	x.User = sess.CurrentUser()
	// A failed slide must not fail the request: the session is still valid for
	// the remainder of its window and the next request retries (IDN-042).
	if err := s.sessions.Slide(c.Context(), sess.ID); err != nil {
		s.logger.Warn("session slide failed", "rule", "IDN-042", "request_id", x.ReqID, "error", err)
	}
	return true
}

// checkCSRF issues a token on every GET and verifies it on every POST except
// the beacon. A failure answers 403 and reissues the cookie so a reload works
// (IDN-049, OPS-203).
func (s *server) checkCSRF(c fiber.Ctx, x *Ctx, route PageRoute) bool {
	key := routeKey(route.Method, route.Path)
	if route.Method == fiber.MethodPost && csrfExempt[key] {
		if !s.sameOrigin(c) {
			// The beacon is a script endpoint, not a page: its 403 is empty.
			if key == beaconRouteKey {
				c.Status(fiber.StatusForbidden)
				return false
			}
			s.respondError(c, apperr.Forbiddenf("").WithRule("OPS-203"))
			return false
		}
		return true
	}
	if route.Method == fiber.MethodPost {
		cookie := c.Cookies(cookieCSRF)
		if !s.signer.MatchCSRF(cookie, x.Form.Get("csrf_token")) {
			// Reissue: the error page's "reload" advice must actually work.
			if token, err := s.signer.CSRF(); err == nil {
				x.setCookie(cookieCSRF, token, 0, "/")
			}
			s.respondError(c, apperr.Forbiddenf("").WithRule("INV-28"))
			return false
		}
		x.CSRF = cookie
		return true
	}
	cookie := c.Cookies(cookieCSRF)
	if !s.signer.VerifyCSRF(cookie) {
		token, err := s.signer.CSRF()
		if err != nil {
			s.respondError(c, apperr.Internalf(err).WithRule("IDN-049"))
			return false
		}
		cookie = token
		x.setCookie(cookieCSRF, cookie, 0, "/")
	}
	x.CSRF = cookie
	return true
}

// checkMustChange blocks every path but the two that can clear the flag
// (IDN-070). It runs after CSRF and before the route class so the redirect can
// never be used to skip an authorization decision.
func (s *server) checkMustChange(c fiber.Ctx, x *Ctx, route PageRoute) bool {
	if x.Session == nil || !x.Session.MustChangePassword {
		return true
	}
	switch routeKey(route.Method, route.Path) {
	case "GET /change-password", "POST /change-password", "POST /logout":
		return true
	case streamRouteKey:
		// The stream cannot follow a redirect usefully. It passes the guard and
		// re-checks the flag itself, after ownership, so a foreign quiz stays
		// 404 and only the owner sees the 403 (ATT-023, ATT-191).
		return true
	}
	s.redirectFlash(c, "/change-password", "must_change")
	return false
}

// checkClass applies the authorization matrix. A signed-out user is redirected
// to login with the expired-session flash; a wrong role gets 404 so the route's
// existence does not leak (IDN-012, IDN-016).
//
// Enforcement is skipped only for the fixture preview build: APP_DEMO with no
// database has no accounts at all, so every route would redirect to a login
// nobody can complete. The demo holds no data and is never a production mode
// (config.Validate requires a real signing key as soon as APP_DEMO is off).
func (s *server) checkClass(c fiber.Ctx, x *Ctx, route PageRoute) bool {
	if !s.enforceAuth {
		return true
	}
	if routeKey(route.Method, route.Path) == streamRouteKey {
		// An EventSource endpoint has no HTML error surface: the handler answers
		// an unauthorised or foreign request with an empty 404 itself (ATT-191).
		return true
	}
	class, ok := classFor(route.Method, route.Path)
	if !ok {
		s.respondError(c, apperr.Internalf(nil).WithRule("IDN-015"))
		return false
	}
	authed := x.Session != nil
	switch class {
	case ClassPublic, ClassAnonymousOrPublic:
		return true
	case ClassGuestOnly:
		if authed {
			s.redirect(c, homeFor(x.Session))
			return false
		}
		return true
	case ClassAnyAuth:
		if !authed {
			s.redirectFlash(c, "/login", "session_expired")
			return false
		}
		return true
	case ClassStudent:
		if !authed {
			s.redirectFlash(c, "/login", "session_expired")
			return false
		}
		if !x.Session.IsStudent() {
			s.respondError(c, apperr.NotFoundf().WithRule("IDN-012"))
			return false
		}
		return true
	case ClassTeacher, ClassOwnerTeacher:
		if !authed {
			s.redirectFlash(c, "/login", "session_expired")
			return false
		}
		if !x.Session.IsTeacher() {
			s.respondError(c, apperr.NotFoundf().WithRule("IDN-012"))
			return false
		}
		return true
	default:
		s.respondError(c, apperr.Internalf(nil).WithRule("IDN-015"))
		return false
	}
}

// homeFor is the role router used by guest-only redirects and `GET /`
// (IDN-024, IDN-026).
func homeFor(sess *authn.Session) string {
	if sess == nil {
		return "/login"
	}
	if sess.MustChangePassword {
		return "/change-password"
	}
	if sess.IsTeacher() {
		return "/teacher"
	}
	return "/student"
}

// checkBodyCap rejects an oversized body before parsing (OPS-218). Multipart
// uploads get the ten-mebibyte cap; everything else gets one mebibyte and the
// beacon four kibibytes.
func (s *server) checkBodyCap(c fiber.Ctx, x *Ctx, route PageRoute) bool {
	if route.Method != fiber.MethodPost {
		return true
	}
	limit := maxFormBody
	key := routeKey(route.Method, route.Path)
	switch {
	case key == beaconRouteKey:
		limit = maxBeaconBody
	case strings.HasPrefix(string(c.Request().Header.ContentType()), "multipart/"):
		limit = maxUploadBody
	}
	if len(c.Body()) > limit {
		if key == beaconRouteKey {
			c.Status(fiber.StatusBadRequest)
			return false
		}
		s.respondError(c, apperr.Validationf(map[string]string{
			"form": "Isian terlalu besar.",
		}).WithRule("OPS-218"))
		return false
	}
	return true
}

// sameOrigin requires a matching Origin for the CSRF-exempt beacon. ATT-159
// deliberately does not accept a Referer fallback: a beacon without Origin is
// not a browser same-origin send. The comparison includes the scheme, host,
// and port, so an https page cannot post to an http origin (or vice versa).
func (s *server) sameOrigin(c fiber.Ctx) bool {
	origin := string(c.Request().Header.Peek("Origin"))
	if origin == "" {
		return false
	}
	return sameHost(origin, c)
}

// sameHost compares an Origin URL's scheme and authority with the request's
// effective scheme and Host header. An Origin never carries a path, query, or
// fragment; any that does is not a browser Origin and is rejected.
func sameHost(raw string, c fiber.Ctx) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return strings.EqualFold(u.Scheme, c.Scheme()) &&
		strings.EqualFold(u.Host, c.Host())
}

// takeFlash reads and clears the one-shot flash. Production reads only the
// signed cookie; the query key is a demo-only preview (OPS-210, IDN-028).
func (s *server) takeFlash(c fiber.Ctx, demo bool) *viewmodel.Flash {
	if demo {
		if key := c.Query("flash"); key != "" {
			return viewmodel.FlashFor(key)
		}
	}
	raw := c.Cookies(cookieFlash)
	if raw == "" {
		return nil
	}
	c.ClearCookie(cookieFlash)
	payload, err := s.signer.ParseToken("flash", raw, 60*time.Second, time.Now().UTC())
	if err != nil {
		return nil
	}
	return viewmodel.FlashFor(string(payload))
}

// setFlashCookie signs and stores a flash key for the next GET.
func (s *server) setFlashCookie(c fiber.Ctx, key string) {
	if key == "" {
		return
	}
	token, err := s.signer.Token("flash", []byte(key), time.Now().UTC())
	if err != nil {
		return
	}
	c.Cookie(&fiber.Cookie{
		Name:     cookieFlash,
		Value:    token,
		Path:     "/",
		MaxAge:   60,
		Secure:   s.cfg.CookieSecure,
		HTTPOnly: true,
		SameSite: "Lax",
	})
}

// parseForm copies submitted fields into a neutral map. Multipart is parsed
// through Fiber's form parser so the upload routes read the same shape.
func parseForm(c fiber.Ctx, dst url.Values) {
	ct := string(c.Request().Header.ContentType())
	if strings.HasPrefix(ct, "multipart/") {
		if form, err := c.MultipartForm(); err == nil && form != nil {
			for k, vs := range form.Value {
				for _, v := range vs {
					dst.Add(k, v)
				}
			}
		}
		return
	}
	c.Request().PostArgs().VisitAll(func(k, v []byte) {
		dst.Add(string(k), string(v))
	})
}

// requestID returns a short random correlation id for logging. It is never
// exposed to a client.
func requestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "-"
	}
	return hex.EncodeToString(b)
}
