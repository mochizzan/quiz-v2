package httpserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/attempts"
	"quiz-v2/internal/export"
	"quiz-v2/internal/viewmodel"
)

// This file is the "teacher-dashboard" route group, owned by the attempts and
// grading domain: the teacher overview, the per-quiz results table, the live
// monitor and its SSE endpoint. Authoring owns the quiz list and builder
// (routes_teacher_core.go).

// streamRouteKey is the SSE route's matrix key. It is special-cased by the
// pipeline because an EventSource endpoint has no HTML error surface.
const streamRouteKey = "GET /teacher/quizzes/:id/stream"

// teacherActor returns the signed-in account id. The route class has already
// admitted an active teacher; a nil session is a programming fault answered as
// the same 404 the class guard produces.
func teacherActor(x *Ctx) (int64, error) {
	if x.Session == nil {
		return 0, apperr.NotFoundf()
	}
	return x.Session.AccountID, nil
}

// attemptTeacherError maps the attempts domain's read failures onto the error
// taxonomy: a missing/foreign resource is 404, a typed application error keeps
// its class, and an untyped failure (an integrity witness or a driver error) is
// a 500. Every teacher read uses this instead of passing a raw error through,
// because finish only recognises *apperr.Error.
func attemptTeacherError(err error) error {
	if err == nil {
		return nil
	}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	if errors.Is(err, attempts.ErrNotFound) {
		return apperr.NotFoundf()
	}
	return apperr.Internalf(err)
}

// setStreamHeaders writes ATT-192's handshake headers. No CORS header is set,
// so a cross-origin EventSource cannot read the stream.
func setStreamHeaders(c fiber.Ctx) {
	c.Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Set("Cache-Control", "no-cache, no-store, no-transform")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")
}

// streamConnectionCap is OPS-173/OPS-231's per teacher+quiz subscriber ceiling.
// Excess connections are refused with 429 before any SSE header is written.
const streamConnectionCap = 10

// streamSlots counts live SSE connections per teacher+quiz key. It is a
// process-local, mutex-protected map; an entry exists only while at least one
// connection is open, so the table cannot grow without live subscribers.
type streamSlots struct {
	mu     sync.Mutex
	limit  int
	active map[string]int
}

func newStreamSlots(limit int) *streamSlots {
	return &streamSlots{limit: limit, active: map[string]int{}}
}

// acquire reserves one slot or reports that the ceiling is reached.
func (s *streamSlots) acquire(key string) *streamLease {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[key] >= s.limit {
		return nil
	}
	s.active[key]++
	return &streamLease{slots: s, key: key}
}

func (s *streamSlots) release(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := s.active[key]; n <= 1 {
		delete(s.active, key)
		return
	}
	s.active[key] = s.active[key] - 1
}

// streamLease is one reserved connection. Release is idempotent because the
// stream callback and the SendStreamWriter error path can both reach it.
type streamLease struct {
	slots *streamSlots
	key   string
	once  sync.Once
}

func (l *streamLease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() { l.slots.release(l.key) })
}

// teacherStream serves GET /teacher/quizzes/:id/stream (ATT-191…ATT-198).
//
// The route runs as HandlerCtx because the response is not a page: the pipeline
// has already resolved the session and CSRF token, and this handler does the
// one ownership decision before any SSE header is written. An EventSource
// cannot follow a redirect usefully, so every rejection is an empty body.
func teacherStream(x *Ctx) error {
	c := x.c

	// ATT-191 authorization: not signed in, non-teacher, foreign, or unknown is
	// an empty 404. An owner with a forced password change is an empty 403; the
	// global must-change guard lets this route through for exactly this check.
	if x.Session == nil || !x.Session.IsTeacher() {
		c.Status(fiber.StatusNotFound)
		return nil
	}
	input := attempts.StreamInput{QuizID: x.IntParam("id"), AccountID: x.Session.AccountID}
	svc := attemptService(x)
	if _, err := svc.StreamStart(x.Context(), input); err != nil {
		var appErr *apperr.Error
		switch {
		case errors.As(err, &appErr):
			// A DB or invariant failure is a normal 500 before headers (OPS-213).
			return x.server.respondError(c, appErr)
		case errors.Is(err, attempts.ErrNotFound):
			c.Status(fiber.StatusNotFound)
			return nil
		default:
			return x.server.respondError(c, apperr.From(err))
		}
	}
	if x.Session.MustChangePassword {
		c.Status(fiber.StatusForbidden)
		return nil
	}

	// OPS-173: the subscriber cap is decided before the stream starts, so the
	// rejection is a normal status with an empty body (OPS-205).
	key := fmt.Sprintf("%d:%d", input.AccountID, input.QuizID)
	lease := x.server.streams.acquire(key)
	if lease == nil {
		c.Set("Retry-After", "1")
		c.Status(fiber.StatusTooManyRequests)
		return nil
	}

	setStreamHeaders(c)
	stop := x.server.streamStop()
	// The callback runs after the handler returns, so it captures the service,
	// the immutable input, and the shutdown channel. x itself is not safe there.
	err := c.SendStreamWriter(func(w *bufio.Writer) {
		defer lease.Release()
		emit := func(name string, data []byte) error {
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data); err != nil {
				return err
			}
			return w.Flush()
		}
		// A client disconnect surfaces as a write error inside Stream and ends
		// the loop; the request context is already gone by then, so the tick
		// transactions run under a background context and stop on the channel.
		_ = svc.Stream(context.Background(), input, stop, emit)
	})
	if err != nil {
		lease.Release()
	}
	return err
}

// exportContentType is the only media type the export route emits.
const exportContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// teacherExport serves GET /teacher/quizzes/:id/export.xlsx (spec §9). The
// route runs as HandlerCtx because the response is a file, not a page: the
// pipeline has already resolved the teacher and the route class, and the
// handler makes the one ownership decision through the export read model.
//
// The workbook is built completely before any header or byte is written, so a
// failed read or build is the normal error page and never a partial download.
func teacherExport(x *Ctx) error {
	accountID, err := teacherActor(x)
	if err != nil {
		return x.server.respondError(x.c, apperr.From(attemptTeacherError(err)))
	}
	quizID := x.IntParam("id")
	data, err := attemptService(x).Export(x.Context(), attempts.ExportInput{
		QuizID:    quizID,
		AccountID: accountID,
	})
	if err != nil {
		return x.server.respondError(x.c, apperr.From(attemptTeacherError(err)))
	}
	workbook, err := export.Build(data, x.UploadsDir())
	if err != nil {
		return x.server.respondError(x.c, apperr.From(attemptTeacherError(err)))
	}

	x.c.Set("Content-Type", exportContentType)
	x.c.Set("Content-Disposition", `attachment; filename="`+exportFilename(quizID, data.Quiz.Title)+`"`)
	x.c.Set("Cache-Control", "no-store")
	return x.c.Send(workbook)
}

// exportFilename is the download name: kuis-<id>-<slug>.xlsx. Both parts are
// generated here, so the header can never carry a quote, newline, or
// non-ASCII byte from the quiz title.
func exportFilename(quizID int64, title string) string {
	return "kuis-" + strconv.FormatInt(quizID, 10) + "-" + exportSlug(title) + ".xlsx"
}

// exportSlug lowercases the ASCII letters and digits of a title and collapses
// every run of other bytes into one hyphen. Leading and trailing runs are
// dropped; a title with no ASCII alphanumeric falls back to "kuis".
func exportSlug(title string) string {
	var b strings.Builder
	b.Grow(len(title))
	gap := false
	for i := 0; i < len(title); i++ {
		c := title[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			if gap && b.Len() > 0 {
				b.WriteByte('-')
			}
			gap = false
			b.WriteByte(c)
		case c >= 'A' && c <= 'Z':
			if gap && b.Len() > 0 {
				b.WriteByte('-')
			}
			gap = false
			b.WriteByte(c + ('a' - 'A'))
		default:
			gap = b.Len() > 0
		}
	}
	if b.Len() == 0 {
		return "kuis"
	}
	return b.String()
}

// init registers the teacher overview, results, monitor and stream routes.
func init() {
	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher",
		Template: "pages/teacher/dashboard.html",
		Nav:      "dasbor",
		Auth:     ClassTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			accountID, err := teacherActor(x)
			if err != nil {
				return viewmodel.PageData{}, err
			}
			board, err := attemptService(x).TeacherDashboard(x.Context(), accountID, x.Session.FullName)
			if err != nil {
				return viewmodel.PageData{}, attemptTeacherError(err)
			}
			return x.Page("Dasbor guru", "dasbor", board), nil
		},
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/:id/results",
		Template: "pages/teacher/results.html",
		Nav:      "kuis",
		Auth:     ClassOwnerTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			accountID, err := teacherActor(x)
			if err != nil {
				return viewmodel.PageData{}, err
			}
			results, err := attemptService(x).Results(x.Context(), attempts.ResultsInput{
				QuizID:    x.IntParam("id"),
				AccountID: accountID,
				StateRaw:  x.QueryValue("state"),
				ClassRaw:  x.QueryValue("class"),
				QueryRaw:  x.QueryValue("q"),
				PageRaw:   x.QueryValue("page"),
			})
			if err != nil {
				return viewmodel.PageData{}, attemptTeacherError(err)
			}
			title := "Hasil kuis"
			if results.Quiz.Title != "" {
				title = "Hasil · " + results.Quiz.Title
			}
			return x.Page(title, "kuis", results), nil
		},
	})

	registerPage(PageRoute{
		Method:     "GET",
		Path:       "/teacher/quizzes/:id/export.xlsx",
		Auth:       ClassOwnerTeacher,
		HandlerCtx: teacherExport,
	})

	registerPage(PageRoute{
		Method:   "GET",
		Path:     "/teacher/quizzes/:id/monitor",
		Template: "pages/teacher/monitor.html",
		Nav:      "kuis",
		Auth:     ClassOwnerTeacher,
		Load: func(x *Ctx) (viewmodel.PageData, error) {
			accountID, err := teacherActor(x)
			if err != nil {
				return viewmodel.PageData{}, err
			}
			monitor, err := attemptService(x).MonitorPage(x.Context(), attempts.MonitorInput{
				QuizID:    x.IntParam("id"),
				AccountID: accountID,
			})
			if err != nil {
				return viewmodel.PageData{}, attemptTeacherError(err)
			}
			title := "Monitor kuis"
			if monitor.Quiz.Title != "" {
				title = "Monitor · " + monitor.Quiz.Title
			}
			return x.Page(title, "kuis", monitor), nil
		},
	})

	registerPage(PageRoute{
		Method:     "GET",
		Path:       "/teacher/quizzes/:id/stream",
		Auth:       ClassOwnerTeacher,
		HandlerCtx: teacherStream,
	})
}
