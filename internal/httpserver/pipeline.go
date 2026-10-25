package httpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/authn"
	"quiz-v2/internal/authoring"
	"quiz-v2/internal/config"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// Body caps are pinned by OPS-218: one mebibyte for a form post, four kibibytes
// for a beacon, sixteen mebibytes for a multipart upload. They are checked
// before parsing so an oversized body never reaches validation. The check is
// the declared Content-Length, not a counted body: the server runs with
// StreamRequestBody, so an oversized body is rejected without being buffered
// (AVT-020).
//
// maxAvatarRequest is the photo route's own cap in bytes: 200 MB (AVT-019).
// It is not part of the ordinary upload cap because a photo outlives every
// other body in the app by two orders of magnitude and is bounded by its own
// streaming, counted, 1-byte-probe check (AVT-041).
const (
	maxFormBody      = 1 << 20
	maxBeaconBody    = 4 << 10
	maxUploadBody    = 16 << 20
	maxAvatarRequest = 200_000_000
	beaconRouteKey   = "POST /attempt/:id/beacon"
	avatarRouteKey   = "POST /profile/photo"
	avatarPartsCap   = 16
	csrfTokenCap     = 4 << 10
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

	// Photos is the codec seam for a profile photo. It is nil in a build that
	// wires no processor; the upload handler answers 500 rather than guessing
	// (AVT-055).
	Photos PhotoProcessor
	// Avatars is the on-disk store the upload commits into and the serve route
	// reads from. It is nil in a build that wires no store.
	Avatars AvatarStore
}

// SessionResolver is the part of authn.Store the pipeline needs. It is an
// interface so an integration test can supply a fixed identity without a
// database; production always injects authn.Store.
type SessionResolver interface {
	Resolve(ctx context.Context, token string) (*authn.Session, error)
	Slide(ctx context.Context, id int64) error
}

// newServer builds the dependency set. A missing or short signing key is an
// error, never a silent fallback (IDN-050); main turns that into a fatal exit.
func newServer(cfg config.Config, db *store.DB, logger *slog.Logger) (*server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	signer, err := secure.NewSigner([]byte(cfg.SessionSigningKey))
	if err != nil {
		return nil, err
	}
	s := &server{
		cfg:     cfg,
		db:      db,
		views:   NewViewLoader(defaultTemplatesDir),
		signer:  signer,
		limiter: secure.NewLimiter(nil),
		logger:  logger,
		stop:    make(chan struct{}),
		streams: newStreamSlots(streamConnectionCap),
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
// Order is load-bearing: body cap, form parse, session, CSRF, must-change guard,
// class. CSRF precedes the guard so a cross-site POST from a must-change session
// fails 403 instead of being redirected (IDN-071); the guard precedes the class so
// a must-change session never reaches another domain's handler.
//
// A Stream route (AVT-024) stops early: the pipeline checks the declared cap and
// resolves the session, then hands the untouched body to the handler, which owns
// the form parse, the CSRF check from the first part, and a post-token must-change
// re-check. prepare must not consume a body the codec is about to read.
func (s *server) prepare(c fiber.Ctx, route PageRoute) (*Ctx, bool) {
	x := &Ctx{
		DB:     s.db,
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
	if ip := authn.ClientIP(c.RemoteAddr().String()); ip != nil {
		x.IP = ip.String()
	}

	// A streamed upload keeps one connection open for the length of the codec
	// run. Closing it afterwards (AVT-044) stops a keep-alive connection whose
	// request body was deliberately left unread from stranding the next request
	// on that connection.
	if streamRoutes[routeKey(route.Method, route.Path)] {
		c.Response().Header.SetConnectionClose()
	}

	if !s.checkBodyCap(c, x, route) {
		return nil, false
	}
	// A Stream route's form belongs to its handler: reading it here would
	// consume the single part the codec must stream.
	if route.Method == fiber.MethodPost && !route.Stream {
		switch parseForm(c, x.Form) {
		case formOK:
		case formTooLarge:
			s.respondError(c, apperr.Field("_form", msgBodyTooLarge).WithRule("AUT-034"))
			return nil, false
		default:
			s.respondError(c, apperr.Field("_form", authoring.MsgFormInvalid))
			return nil, false
		}
	}
	if !s.resolveSession(c, x) {
		return nil, false
	}
	// CSRF and the must-change guard are skipped only for a Stream route, which
	// is always a POST: its token lives in the body the handler must not let the
	// pipeline consume (AVT-024, IDN-020a). A GET Stream route cannot exist —
	// streamRoutes holds POST /profile/photo only, and validation rejects any
	// other Stream flag — so this branch can never swallow a token issuance.
	if !route.Stream {
		if !s.checkCSRF(c, x, route) {
			return nil, false
		}
		if !s.checkMustChange(c, x, route) {
			return nil, false
		}
	}
	x.Flash = s.takeFlash(c)
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
	// AVT-080: the chrome's badge is mapped once per request from the session's
	// stored name. AvatarURL validates the name before composing the URL, so a
	// bad database value renders initials instead of a broken link.
	if x.User != nil {
		x.User.AvatarURL = viewmodel.AvatarURL(sess.AvatarName)
	}
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
		return s.verifyFormCSRF(c, x, x.Form.Get("csrf_token"))
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

// verifyFormCSRF compares the session's CSRF cookie with a token taken from a
// request body. The comparison is shared by checkCSRF and the streamed photo
// upload, which reads its token itself from the first multipart part (AVT-030):
// both must fail the same way, reissuing the cookie so a reload works and
// answering 403 without touching the rest of the body.
func (s *server) verifyFormCSRF(c fiber.Ctx, x *Ctx, token string) bool {
	cookie := c.Cookies(cookieCSRF)
	if !s.signer.MatchCSRF(cookie, token) {
		// Reissue: the error page's "reload" advice must actually work.
		if fresh, err := s.signer.CSRF(); err == nil {
			x.setCookie(cookieCSRF, fresh, 0, "/")
		}
		s.respondError(c, apperr.Forbiddenf("").WithRule("INV-28"))
		return false
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
func (s *server) checkClass(c fiber.Ctx, x *Ctx, route PageRoute) bool {
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

// msgBodyTooLarge is AUT-034's transport-overflow copy for the authoring
// multipart POSTs. It is deliberately distinct from the generic form-invalid
// copy: an oversized *body* is a transport failure the user cannot fix by
// editing a field, and AUT-034 pins both the string and the 422 status.
const msgBodyTooLarge = "Badan permintaan terlalu besar."

// checkBodyCap rejects an oversized or unframed request body before anything
// parses it (OPS-218, AVT-020). It reads only the declared Content-Length, never
// the body: with StreamRequestBody on, c.Body() would drain a 200 MB upload into
// memory just to measure it.
//
// The route families differ, and the difference is pinned by the design's cap
// table (AVT-022):
//
//	POST /profile/photo  200,000,000  no length → 413, chunked → 413, over → 413
//	authoring multipart   16 MiB       no length → empty form, chunked → 422 AUT-034, over → 422 AUT-034
//	ordinary form POST     1 MiB       no length → empty form, chunked → 422 generic, over → 422 generic
//	beacon POST            4 KiB       chunked/over → empty 400
//
// Only the avatar row uses the 413 page: this change adds one failure outcome
// rather than reclassifying every form.
func (s *server) checkBodyCap(c fiber.Ctx, x *Ctx, route PageRoute) bool {
	if route.Method != fiber.MethodPost {
		return true
	}
	key := routeKey(route.Method, route.Path)
	declared := c.Request().Header.ContentLength()

	if key == avatarRouteKey {
		// AVT-020/AVT-022. A declared length of 0 with no stream is the
		// request carrying neither Content-Length nor Transfer-Encoding: the
		// form requires a body, so it is the same 413 as any other missing
		// framing. An explicit Content-Length: 0 reaches the handler with a
		// non-nil empty stream and takes the photo-empty flash instead; the
		// distinction is the stream, not the parsed integer, because fasthttp
		// normalises the length-less case to a declared 0 (AVT-022).
		if declared < 0 || declared > maxAvatarRequest ||
			(declared == 0 && c.Request().BodyStream() == nil) {
			// Connection: close was already set by prepare for this route; the
			// client may still be sending and the server must not drain the
			// rest to reuse the socket.
			s.respondError(c, apperr.TooLargef().WithRule("AVT-020"))
			return false
		}
		return true
	}

	limit := maxFormBody
	multipart := strings.HasPrefix(string(c.Request().Header.ContentType()), "multipart/")
	switch {
	case key == beaconRouteKey:
		limit = maxBeaconBody
	case multipart:
		limit = maxUploadBody
	}

	// declared == -1 is chunked: there is no size to check against, so the body
	// is rejected before parsing rather than read to find out (AVT-022). A
	// length-less POST normalises to 0 and falls through as an empty form; only
	// the avatar route treats that as an error.
	if declared == -1 || declared > limit {
		if key == beaconRouteKey {
			c.Status(fiber.StatusBadRequest)
			return false
		}
		if multipart {
			s.respondError(c, apperr.Field("_form", msgBodyTooLarge).WithRule("AUT-034"))
			return false
		}
		s.respondError(c, apperr.Field("_form", authoring.MsgFormInvalid).WithRule("OPS-218"))
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

// takeFlash reads and clears the one-shot flash. Only the signed cookie can
// carry it: a query key would let a link mint a message (OPS-210, IDN-028).
// The cookie is Path=/, so the deletion must repeat that path (OPS-210).
func (s *server) takeFlash(c fiber.Ctx) *viewmodel.Flash {
	raw := c.Cookies(cookieFlash)
	if raw == "" {
		return nil
	}
	// The deletion must carry the same Path as setFlashCookie: ClearCookie
	// sends no Path, so a browser would treat it as /teacher/... and keep the
	// original "/" cookie, replaying the flash on every GET until it expires.
	c.Cookie(&fiber.Cookie{
		Name:     cookieFlash,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		Secure:   s.cfg.CookieSecure,
		HTTPOnly: true,
		SameSite: "Lax",
	})
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

// formStatus reports how a form body ended up unusable. The two failure kinds
// are separate user-visible outcomes: an over-cap body is a transport failure
// (AUT-034) and a body that parses badly inside its cap is a form failure.
type formStatus int

const (
	formOK formStatus = iota
	// formTooLarge: the bounded reader reached its cap+1 probe byte.
	formTooLarge
	// formInvalid: the body was inside its cap but did not parse.
	formInvalid
)

// parseForm copies submitted fields into a neutral map. It never reports a body
// larger than its cap: the caller answers the right 422 for the failure kind.
//
// The urlencoded path deliberately stops using PostArgs(): PostArgs calls
// bodyBytes(), which reads a streamed body to EOF, so a lying Content-Length
// would be measured after the fact. Instead the body is copied through the
// bounded reader (AVT-022, OPS-218) and parsed with url.ParseQuery. Multipart
// goes through Fiber's form parser, which is bounded by BodyLimit — exactly the
// authoring cap — and reports a body that crosses it. Any other content type
// (the JSON beacon) carries no form and is left unread for its own bounded
// handler.
func parseForm(c fiber.Ctx, dst url.Values) formStatus {
	ct := string(c.Request().Header.ContentType())
	switch {
	case strings.HasPrefix(ct, "multipart/"):
		form, err := c.MultipartForm()
		if err != nil || form == nil {
			// After checkBodyCap a body inside the cap cannot be over the
			// parser's limit, so any failure here is transport garbage: a
			// missing boundary or a truncated body, which keeps the generic
			// form-invalid page (AVT-031, AVT-093).
			return formInvalid
		}
		for k, vs := range form.Value {
			for _, v := range vs {
				dst.Add(k, v)
			}
		}
		return formOK
	case ct == "application/x-www-form-urlencoded":
		body, err := readBoundedBody(c, maxFormBody+1)
		if err != nil {
			if errors.Is(err, errBodyRead) {
				return formTooLarge
			}
			return formInvalid
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return formInvalid
		}
		for k, vs := range values {
			for _, v := range vs {
				dst.Add(k, v)
			}
		}
		return formOK
	}
	return formOK
}

// errBodyRead is the bounded reader's over-cap sentinel: the body reached the
// probe byte, so it is at least as large as the cap it was measured against.
var errBodyRead = errors.New("httpserver: request body exceeds its cap")

// readBoundedBody copies at most limit bytes of the request body. A body that
// fills the whole limit is over cap and returns errBodyRead, so a caller passes
// its cap+1 and learns "too large" one byte before the cap is reached.
//
// It reads the stream when the request is streamed and the already-buffered
// copy otherwise; in the streamed case nothing can bypass the cap, because a
// lying Content-Length only truncates the stream earlier (AVT-022).
func readBoundedBody(c fiber.Ctx, limit int) ([]byte, error) {
	stream := c.Request().BodyStream()
	if stream == nil {
		// No stream: fasthttp buffered the body within BodyLimit, or there is
		// no body at all. The copy is bounded before it starts.
		body := c.Request().Body()
		if len(body) >= limit {
			return nil, errBodyRead
		}
		return body, nil
	}
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(stream, int64(limit)))
	if err != nil {
		return nil, err
	}
	if n >= int64(limit) {
		return nil, errBodyRead
	}
	return buf.Bytes(), nil
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
