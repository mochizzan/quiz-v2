// Package httpserver builds the Fiber HTTP application: route registration,
// the request pipeline (session, CSRF, authorization class), server-side
// rendering, and the one error taxonomy every handler speaks.
package httpserver

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/config"
	"quiz-v2/internal/store"
)

// defaultTemplatesDir is the template directory relative to the process
// working directory (repo root under go test, WORKDIR /app in the container).
const defaultTemplatesDir = "./templates"

// healthzTimeout bounds the DB ping performed by GET /healthz so a hung
// database answers 503 instead of holding the request open.
const healthzTimeout = 2 * time.Second

// Pinger is the liveness dependency of GET /healthz.
type Pinger interface {
	Ping(context.Context) error
}

// Deps is everything the app needs from the process. A nil DB is allowed: the
// template-rendering tests build the app without one, and any route that needs
// data answers 500 rather than panicking.
type Deps struct {
	DB     *store.DB
	Pinger Pinger
	Logger *slog.Logger
	// Photos is the profile-photo codec. It is nil in a build that wires no
	// processor (and in every test: the codecs are cgo), and the upload route
	// answers 500 rather than guessing at a conversion (AVT-055).
	Photos PhotoProcessor
	// Avatars overrides the on-disk photo store. nil means "build identity's
	// store over the configured upload root for each request", which is what
	// production does.
	Avatars AvatarStore
}

// New builds the app with no database and a healthy stub pinger. It is the
// convenience constructor for tests; main uses NewWithDeps.
func New(cfg config.Config) *fiber.App {
	app, err := NewWithDeps(cfg, Deps{})
	if err != nil {
		return brokenApp(err)
	}
	return app
}

// NewWithStore is New with GET /healthz wired to pinger.
func NewWithStore(cfg config.Config, pinger Pinger) *fiber.App {
	app, err := NewWithDeps(cfg, Deps{Pinger: pinger})
	if err != nil {
		return brokenApp(err)
	}
	return app
}

// NewWithDeps builds the real application. A configuration error (a missing or
// short signing key) is returned, never downgraded to a warning: a process that
// cannot sign CSRF tokens must not accept a form (IDN-050).
func NewWithDeps(cfg config.Config, deps Deps) (*fiber.App, error) {
	srv, err := newServer(cfg, deps.DB, deps.Logger)
	if err != nil {
		return nil, err
	}
	srv.Photos = deps.Photos
	srv.Avatars = deps.Avatars
	pinger := deps.Pinger
	if pinger == nil {
		pinger = pingerFunc(func(context.Context) error { return nil })
	}

	app := fiber.New(fiber.Config{
		// BodyLimit is the authoring multipart parser's cap (16 MiB), not the
		// avatar cap: raising it to 200 MB would raise every route's parser
		// limit without enforcing the photo limit. The photo route checks its
		// own declared cap and bounds its own stream (AVT-023, AUT-034).
		BodyLimit: maxUploadBody,
		// StreamRequestBody makes every body a stream: fasthttp pre-reads at
		// most 8 KiB and the rest arrives on demand, so a 200 MB upload is
		// never buffered into memory just to be measured (AVT-020).
		StreamRequestBody: true,
		// DisablePreParseMultipartForm is mandatory: without it fasthttp
		// writes multipart temp files during its own pre-parse, before the
		// photo handler can require the CSRF token as the first part
		// (AVT-030).
		DisablePreParseMultipartForm: true,
		// ReadTimeout is the transfer budget for a 200 MB upload on a slow
		// link; IdleTimeout keeps idle keep-alives bounded. The conversion
		// budget is the route's own Deadline, not this (AVT-025, AVT-038).
		ReadTimeout: 10 * time.Minute,
		IdleTimeout: 60 * time.Second,
	})

	// A graceful stop must end open SSE responses: they select on the server's
	// stop channel, which this pre-shutdown hook closes (ATT-197). Registering
	// it before the routes keeps the lifespan owned by the constructor.
	app.Hooks().OnPreShutdown(func() error {
		srv.beginShutdown()
		return nil
	})

	// Compress text responses at best speed. Registered before /static so the
	// vendored CSS and JS are compressed too; the exclusion rules live in
	// newCompress (compress.go).
	app.Use(newCompress())

	// Serve the vendored carrier assets (Bootstrap 6, jQuery 4, Gotham
	// Rounded, app.js) with a one-day cache. Registered first so a static
	// request never pays for session or CSRF work (IDN-020 step 1).
	app.Use("/static", static.New("./static", static.Config{MaxAge: 86400}))

	// Register static paths before parameterised ones and longer parameterised
	// paths before their prefixes. Fiber matches in registration order, so this
	// is what keeps /teacher/quizzes/new away from /teacher/quizzes/:id.
	routes := RegisteredPages()
	if err := validateRouteClasses(routes); err != nil {
		return nil, err
	}
	sort.SliceStable(routes, func(i, j int) bool { return routeLess(routes[i], routes[j]) })

	for _, route := range routes {
		r := route
		handler := func(c fiber.Ctx) error { return srv.serveRoute(c, r) }
		if r.Method == fiber.MethodPost {
			app.Post(r.Path, handler)
			continue
		}
		app.Get(r.Path, handler)
	}

	app.Get("/healthz", func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), healthzTimeout)
		defer cancel()
		c.Set(fiber.HeaderCacheControl, "no-store")
		if err := pinger.Ping(ctx); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("db unavailable")
		}
		return c.Status(fiber.StatusOK).SendString("ok")
	})

	// GET /uploads/:name serves authoring's question images from disk. It is
	// registered here rather than through registerPage because it is an asset
	// route, not a screen, so it does not affect the 74-screen pin (contract §6).
	app.Get("/uploads/:name", srv.serveUpload)

	// GET /avatars/:name serves profile photos the same way: an asset route
	// registered directly, outside the contract-route pin while still carrying
	// an authorization class (AVT-062, OPS-220).
	app.Get("/avatars/:name", srv.serveAvatar)

	// Catch-all, registered LAST so it only sees requests no route claimed.
	// There is no 405 page: a known path with the wrong method lands here
	// (OPS-214).
	app.Use(func(c fiber.Ctx) error {
		return srv.respondError(c, apperr.NotFoundf().WithRule("OPS-214"))
	})

	return app, nil
}

// brokenApp answers every request with the 500 page and logs the startup fault.
// It exists only so New's signature can stay non-erroring for callers that have
// no way to handle one; main never uses it.
func brokenApp(err error) *fiber.App {
	slog.Error("httpserver: configuration invalid", "error", err)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		return c.Status(fiber.StatusInternalServerError).
			Type("text/plain; charset=utf-8").
			SendString(apperr.MsgServerError)
	})
	return app
}

// pingerFunc adapts a func(context.Context) error to the Pinger interface.
type pingerFunc func(context.Context) error

// Ping calls f.
func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }
