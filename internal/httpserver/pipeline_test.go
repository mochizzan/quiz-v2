package httpserver

// pipeline_test.go pins the one-shot flash transport (OPS-210) and the
// route-aware body caps and deadlines (AVT-020…AVT-026, OPS-218). The GET that
// consumes a flash must also delete the cookie it came in, with the same Path
// it was written with. The cap tests deliberately go through the real Fiber
// pipeline: a unit test of the reader alone would not prove which route gets
// which outcome.

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/authoring"
	"quiz-v2/internal/config"
)

func TestTakeFlashClearsWithMatchingPath(t *testing.T) {
	srv, err := newServer(
		config.Config{SessionSigningKey: strings.Repeat("k", 32)},
		nil,
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	token, err := srv.signer.Token("flash", []byte("graded"), time.Now().UTC())
	if err != nil {
		t.Fatalf("sign flash: %v", err)
	}

	app := fiber.New()
	app.Get("/next", func(c fiber.Ctx) error {
		flash := srv.takeFlash(c)
		if flash == nil {
			return c.SendString("none")
		}
		return c.SendString(flash.Title)
	})

	req := httptest.NewRequest(http.MethodGet, "/next", nil)
	req.AddCookie(&http.Cookie{Name: cookieFlash, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := string(body); got != "Nilai esai tersimpan." {
		t.Errorf("body = %q, want the flash title", got)
	}

	// A deletion without Path omits it from Set-Cookie, so the browser keeps
	// the original "/" cookie and replays the message on every GET.
	clear := strings.ToLower(resp.Header.Get("Set-Cookie"))
	if !strings.Contains(clear, strings.ToLower(cookieFlash)+"=") ||
		!strings.Contains(clear, "path=/") ||
		!strings.Contains(clear, "max-age=0") {
		t.Errorf("Set-Cookie = %q, want a Path=/ Max-Age=0 deletion", clear)
	}
}

// pipelineApp builds a minimal Fiber app around the real serveRoute. It is the
// same pipeline production uses; only the route table is smaller so a test can
// observe one boundary without a database or a real codec.
func pipelineApp(srv *server, routes ...PageRoute) *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit:                    maxUploadBody,
		StreamRequestBody:            true,
		DisablePreParseMultipartForm: true,
	})
	for _, route := range routes {
		r := route
		if r.Method == fiber.MethodPost {
			app.Post(r.Path, func(c fiber.Ctx) error { return srv.serveRoute(c, r) })
		}
	}
	return app
}

// registeredRoute returns the production route declaration for one method and
// path. The deadline test uses the real declaration so its 10-minute budget
// cannot drift from the route that ships.
func registeredRoute(t *testing.T, method, path string) PageRoute {
	t.Helper()
	for _, route := range RegisteredPages() {
		if route.Method == method && route.Path == path {
			return route
		}
	}
	t.Fatalf("route %s %s is not registered", method, path)
	return PageRoute{}
}

// pipelineRequest sends one request through the test app with the supplied
// cookies. It exists so every test in this file uses the same generous timeout:
// the 12 MB cases are slower than Fiber's one-second default.
func pipelineRequest(t *testing.T, app *fiber.App, req *http.Request, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	for _, cookie := range cookies {
		if cookie != nil {
			req.AddCookie(cookie)
		}
	}
	resp, err := app.Test(req, avatarTestTimeout)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	return resp
}

// TestPipelineAvatarBodyFraming pins AVT-022's four avatar rows: a bodyless
// request is 413, an explicit zero-length multipart body is the empty-upload
// flash, a chunked body is 413, and a 12 MB body is accepted. It also pins the
// ordinary-route contrast: chunked and 12 MB urlencoded forms stay 422.
func TestPipelineAvatarBodyFraming(t *testing.T) {
	t.Run("bodyless avatar is 413", func(t *testing.T) {
		f := newAvatarHTTPFixture(t)
		req := httptest.NewRequest(http.MethodPost, "/profile/photo", nil)
		resp := pipelineRequest(t, f.app, req)
		body := expectStatus(t, resp, http.StatusRequestEntityTooLarge)
		if !strings.Contains(body, apperr.MsgTooLarge) {
			t.Errorf("413 body does not carry %q", apperr.MsgTooLarge)
		}
		if !resp.Close {
			t.Error("bodyless 413 must close the connection")
		}
		f.assertNoPhoto()
	})

	t.Run("explicit zero-length avatar is empty upload", func(t *testing.T) {
		f := newAvatarHTTPFixture(t)
		req := httptest.NewRequest(http.MethodPost, "/profile/photo", bytes.NewReader(nil))
		req.Header.Set(fiber.HeaderContentType, "multipart/form-data; boundary=xyz")
		resp := pipelineRequest(t, f.app, req,
			&http.Cookie{Name: cookieSession, Value: avatarTestStudentToken},
			&http.Cookie{Name: cookieCSRF, Value: f.csrf})
		f.expectRedirect(resp, "/profile", flashPhotoEmpty)
		f.assertNoPhoto()
	})

	t.Run("chunked avatar is 413", func(t *testing.T) {
		f := newAvatarHTTPFixture(t)
		// app.Test dumps the request with httputil.DumpRequest. A nil body is
		// http.NoBody, so it writes a valid empty chunked body and no
		// Content-Length; setting ContentLength=-1 would dump the literal
		// "Content-Length: -1" and fail header parsing.
		req := httptest.NewRequest(http.MethodPost, "/profile/photo", nil)
		req.TransferEncoding = []string{"chunked"}
		req.Header.Set(fiber.HeaderContentType, "multipart/form-data; boundary=xyz")
		resp := pipelineRequest(t, f.app, req,
			&http.Cookie{Name: cookieSession, Value: avatarTestStudentToken},
			&http.Cookie{Name: cookieCSRF, Value: f.csrf})
		body := expectStatus(t, resp, http.StatusRequestEntityTooLarge)
		if !strings.Contains(body, apperr.MsgTooLarge) {
			t.Errorf("chunked 413 body does not carry %q", apperr.MsgTooLarge)
		}
		f.assertNoPhoto()
	})

	t.Run("chunked ordinary form is 422", func(t *testing.T) {
		f := newAvatarHTTPFixture(t)
		req := httptest.NewRequest(http.MethodPost, "/profile/photo/remove", nil)
		req.TransferEncoding = []string{"chunked"}
		req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")
		resp := pipelineRequest(t, f.app, req)
		body := expectStatus(t, resp, http.StatusUnprocessableEntity)
		if !strings.Contains(body, apperr.MsgValidation) {
			t.Errorf("chunked form body does not carry %q", apperr.MsgValidation)
		}
		if f.store.removes != 0 {
			t.Errorf("chunked form reached the remove handler")
		}
	})

	t.Run("12 MB urlencoded form is 422", func(t *testing.T) {
		f := newAvatarHTTPFixture(t)
		req := httptest.NewRequest(http.MethodPost, "/profile/photo/remove",
			bytes.NewReader(bytes.Repeat([]byte("a"), 12<<20)))
		req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")
		resp := pipelineRequest(t, f.app, req)
		body := expectStatus(t, resp, http.StatusUnprocessableEntity)
		if !strings.Contains(body, apperr.MsgValidation) {
			t.Errorf("12 MB form body does not carry %q", apperr.MsgValidation)
		}
		if f.store.removes != 0 {
			t.Errorf("12 MB form reached the remove handler")
		}
	})

	t.Run("12 MB avatar is accepted", func(t *testing.T) {
		f := newAvatarHTTPFixture(t)
		resp := f.upload(avatarTestStudentToken,
			f.uploadParts(bytes.Repeat([]byte("j"), 12<<20))...)
		f.expectRedirect(resp, "/profile", flashPhotoSaved)
		if f.store.names[11] == "" {
			t.Fatal("12 MB upload did not commit")
		}
		if f.photos.processed != 1 {
			t.Errorf("codec calls = %d, want 1", f.photos.processed)
		}
		f.assertNoTempFiles()
	})
}

// TestPipelineAuthoringMultipartKeepsImageCap proves the 16 MiB transport guard
// did not replace the 10 MiB business rule. A 12 MB multipart body is under the
// guard, so the pipeline parses it; the authoring validator still rejects the
// image. The route handler is a test seam because the real one needs a DB, but
// it calls the production questionUploads and ValidateQuestion functions.
func TestPipelineAuthoringMultipartKeepsImageCap(t *testing.T) {
	f := newAvatarHTTPFixture(t)
	route := registeredRoute(t, http.MethodPost, "/teacher/quizzes/:id/questions")
	route.HandlerCtx = func(x *Ctx) error {
		if got := x.Form.Get("type"); got != "single" {
			return x.c.Status(http.StatusInternalServerError).SendString("type not parsed: " + got)
		}
		if got := x.Form.Get("body"); got != "Soal yang valid" {
			return x.c.Status(http.StatusInternalServerError).SendString("body not parsed: " + got)
		}
		uploads, err := questionUploads(x)
		if err != nil {
			return x.c.Status(http.StatusInternalServerError).SendString("uploads: " + err.Error())
		}
		if len(uploads) != 1 {
			return x.c.Status(http.StatusInternalServerError).SendString("image part missing")
		}
		_, aerr := authoring.ValidateQuestion(questionInput(x, uploads))
		if aerr == nil {
			return x.c.SendString("accepted")
		}
		if got := aerr.Fields()["image"]; got != authoring.MsgImageTooLarge {
			return x.c.Status(http.StatusInternalServerError).SendString("image field: " + got)
		}
		return x.c.Status(aerr.Status()).SendString(authoring.MsgImageTooLarge)
	}

	app := pipelineApp(f.srv, route)
	body, contentType := multipartBody(t,
		photoPart{name: "csrf_token", content: []byte(f.csrf)},
		photoPart{name: "type", content: []byte("single")},
		photoPart{name: "body", content: []byte("Soal yang valid")},
		photoPart{name: "image", filename: "photo.png", content: bytes.Repeat([]byte("x"), 12<<20)},
	)
	req := httptest.NewRequest(http.MethodPost, "/teacher/quizzes/1/questions", bytes.NewReader(body))
	req.Header.Set(fiber.HeaderContentType, contentType)
	resp := pipelineRequest(t, app, req,
		&http.Cookie{Name: cookieSession, Value: avatarTestTeacherToken},
		&http.Cookie{Name: cookieCSRF, Value: f.csrf})
	got := expectStatus(t, resp, http.StatusUnprocessableEntity)
	if !strings.Contains(got, authoring.MsgImageTooLarge) {
		t.Errorf("422 body does not carry %q", authoring.MsgImageTooLarge)
	}
}

// TestPipelineDeadlines pins AVT-025: the photo route's conversion budget is
// ten minutes, while the ordinary removal POST keeps OPS-100's ten seconds. The
// handlers only report which side of one minute their context deadline fell on,
// so the assertion is about the route's deadline, not the handler's speed.
func TestPipelineDeadlines(t *testing.T) {
	f := newAvatarHTTPFixture(t)
	classify := func(x *Ctx) error {
		deadline, ok := x.Context().Deadline()
		if !ok {
			return x.c.SendString("none")
		}
		if time.Until(deadline) >= time.Minute {
			return x.c.SendString("long")
		}
		return x.c.SendString("short")
	}

	upload := registeredRoute(t, http.MethodPost, "/profile/photo")
	upload.HandlerCtx = classify
	remove := registeredRoute(t, http.MethodPost, "/profile/photo/remove")
	remove.HandlerCtx = classify
	if upload.Deadline != 10*time.Minute {
		t.Errorf("upload deadline = %v, want 10m", upload.Deadline)
	}
	if remove.Deadline != 0 {
		t.Errorf("remove deadline = %v, want the default command budget", remove.Deadline)
	}

	app := pipelineApp(f.srv, upload, remove)

	req := httptest.NewRequest(http.MethodPost, "/profile/photo", strings.NewReader("x"))
	req.Header.Set(fiber.HeaderContentType, "multipart/form-data; boundary=xyz")
	resp := pipelineRequest(t, app, req,
		&http.Cookie{Name: cookieSession, Value: avatarTestStudentToken})
	if got := expectStatus(t, resp, http.StatusOK); got != "long" {
		t.Errorf("upload deadline class = %q, want long", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/profile/photo/remove",
		strings.NewReader("csrf_token="+f.csrf))
	req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")
	resp = pipelineRequest(t, app, req,
		&http.Cookie{Name: cookieSession, Value: avatarTestStudentToken},
		&http.Cookie{Name: cookieCSRF, Value: f.csrf})
	if got := expectStatus(t, resp, http.StatusOK); got != "short" {
		t.Errorf("remove deadline class = %q, want short", got)
	}
}

// TestCapReader pins the one-byte probe: a body that ends exactly at the cap is
// legal, while the next byte trips errAvatarTooLarge. This is the reader that
// stops a lying Content-Length on the avatar route.
func TestCapReader(t *testing.T) {
	cases := []struct {
		name    string
		size    int
		limit   int
		wantErr error
	}{
		{"under cap", 7, 8, nil},
		{"exactly at cap", 8, 8, nil},
		{"one over cap", 9, 8, errAvatarTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := bytes.Repeat([]byte("a"), tc.size)
			got, err := io.ReadAll(newCapReader(bytes.NewReader(data), int64(tc.limit)))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			wantLen := tc.size
			if tc.wantErr != nil {
				wantLen = tc.limit
			}
			if len(got) != wantLen {
				t.Errorf("read %d bytes, want %d", len(got), wantLen)
			}
		})
	}
}

// TestReadBoundedBody pins the cap+1 reader used by the ordinary form and the
// beacon. A body exactly at the supplied limit is over cap; one byte less is
// accepted. Both the streamed and buffered paths must agree.
func TestReadBoundedBody(t *testing.T) {
	cases := []struct {
		name    string
		size    int
		limit   int
		wantErr bool
	}{
		{"empty", 0, 4, false},
		{"under cap", 3, 4, false},
		{"exactly at cap", 4, 4, true},
		{"over cap", 5, 4, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := bytes.Repeat([]byte("b"), tc.size)
			for _, streamed := range []bool{true, false} {
				name := "buffered"
				if streamed {
					name = "streamed"
				}
				t.Run(name, func(t *testing.T) {
					app := fiber.New()
					app.Post("/bounded", func(c fiber.Ctx) error {
						if streamed {
							c.Request().SetBodyStream(bytes.NewReader(data), -1)
						}
						got, err := readBoundedBody(c, tc.limit)
						if tc.wantErr {
							if !errors.Is(err, errBodyRead) {
								return c.Status(http.StatusInternalServerError).
									SendString("error = " + errString(err))
							}
							return c.SendString("over")
						}
						if err != nil {
							return c.Status(http.StatusInternalServerError).
								SendString("error = " + errString(err))
						}
						if !bytes.Equal(got, data) {
							return c.Status(http.StatusInternalServerError).
								SendString("body mismatch")
						}
						return c.SendString("ok")
					})
					req := httptest.NewRequest(http.MethodPost, "/bounded", bytes.NewReader(data))
					resp, err := app.Test(req)
					if err != nil {
						t.Fatalf("app.Test: %v", err)
					}
					want := "ok"
					if tc.wantErr {
						want = "over"
					}
					if got := expectStatus(t, resp, http.StatusOK); got != want {
						t.Errorf("body = %q, want %q", got, want)
					}
				})
			}
		})
	}
}

// errString avoids importing fmt just for a test diagnostic.
func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
