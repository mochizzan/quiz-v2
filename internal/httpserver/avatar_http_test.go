package httpserver

// avatar_http_test.go drives the profile-photo routes through the real Fiber
// pipeline with injected fakes: no database, no cgo codec, and no real session
// store. It covers the transport contract of design §11: CSRF ordering, the
// flash keys, removal, asset serving, and the request cap. The compression
// itself is tested in internal/avatar under cgo.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/authn"
	"quiz-v2/internal/avatar"
	"quiz-v2/internal/avatarid"
	"quiz-v2/internal/config"
	"quiz-v2/internal/identity"
	"quiz-v2/internal/viewmodel"
)

// avatarTestTimeout is generous enough for a 12 MB body on a Windows test
// runner; app.Test defaults to one second.
var avatarTestTimeout = fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true}

// avatarTestWebP is the bytes the fake codec writes. The asset route never
// sniffs a stored avatar, so any payload exercises the transport contract.
var avatarTestWebP = []byte("RIFF\x00\x00\x00\x00WEBPtest")

// avatarTestStudentToken and avatarTestTeacherToken are the fake session
// cookies the resolver understands.
const (
	avatarTestStudentToken    = "student-session-token"
	avatarTestTeacherToken    = "teacher-session-token"
	avatarTestMustChangeToken = "must-change-session-token"
)

// fakeSessionResolver is the pipeline's session seam without a database.
type fakeSessionResolver struct {
	sessions map[string]*authn.Session
	err      error
	slid     []int64
}

// Resolve returns the fixed session for a token, or nil for an unknown token.
func (f *fakeSessionResolver) Resolve(ctx context.Context, token string) (*authn.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sessions[token], nil
}

// Slide records the session id instead of writing an expiry.
func (f *fakeSessionResolver) Slide(ctx context.Context, id int64) error {
	f.slid = append(f.slid, id)
	return nil
}

// fakePhotoProcessor is the codec seam. It writes a fixed WebP instead of
// decoding an image, so the HTTP tests stay cgo-free.
type fakePhotoProcessor struct {
	acquireErr error
	processErr error
	output     []byte

	acquired  int
	released  int
	processed int
	lastSrc   string
	lastDst   string
}

// TryAcquire reserves a fake slot or returns the configured pool error.
func (p *fakePhotoProcessor) TryAcquire() (PhotoSlot, error) {
	if p.acquireErr != nil {
		return nil, p.acquireErr
	}
	p.acquired++
	return &fakePhotoSlot{p: p}, nil
}

// fakePhotoSlot is one fake conversion slot.
type fakePhotoSlot struct {
	p        *fakePhotoProcessor
	released bool
}

// Process writes the fixed payload or returns the configured conversion error.
func (s *fakePhotoSlot) Process(ctx context.Context, srcPath, dstPath string) error {
	s.p.processed++
	s.p.lastSrc = srcPath
	s.p.lastDst = dstPath
	if s.p.processErr != nil {
		return s.p.processErr
	}
	out := s.p.output
	if out == nil {
		out = avatarTestWebP
	}
	return os.WriteFile(dstPath, out, 0o640)
}

// Release returns the slot exactly once.
func (s *fakePhotoSlot) Release() {
	if s.released {
		return
	}
	s.released = true
	s.p.released++
}

// fakeAvatarStore mirrors identity.AvatarStore's rename-then-commit order
// without a database. Commit renames the produced file, so the handler's
// deferred cleanup cannot remove the committed bytes.
type fakeAvatarStore struct {
	dir string

	live  map[string]int64
	names map[int64]string

	commitErr error
	removeErr error
	hasErr    error

	commits  int
	removes  int
	lastName string
}

// newFakeAvatarStore returns an empty store rooted at dir.
func newFakeAvatarStore(dir string) *fakeAvatarStore {
	return &fakeAvatarStore{
		dir:   dir,
		live:  map[string]int64{},
		names: map[int64]string{},
	}
}

// Dir returns the avatar root.
func (s *fakeAvatarStore) Dir() string { return s.dir }

// Commit moves srcPath to its final name, replaces any old photo, and records
// the account's new name.
func (s *fakeAvatarStore) Commit(ctx context.Context, accountID int64, srcPath, newName string) error {
	s.commits++
	s.lastName = newName
	if s.commitErr != nil {
		return s.commitErr
	}
	dst := filepath.Join(s.dir, newName)
	if err := os.Rename(srcPath, dst); err != nil {
		return err
	}
	if old := s.names[accountID]; old != "" && old != newName {
		delete(s.live, old)
		_ = os.Remove(filepath.Join(s.dir, old))
	}
	s.names[accountID] = newName
	s.live[newName] = accountID
	return nil
}

// Remove clears the account's name and unlinks the file. Removing nothing is
// success, exactly like the production store.
func (s *fakeAvatarStore) Remove(ctx context.Context, accountID int64) error {
	s.removes++
	if s.removeErr != nil {
		return s.removeErr
	}
	if old := s.names[accountID]; old != "" {
		delete(s.names, accountID)
		delete(s.live, old)
		if err := os.Remove(filepath.Join(s.dir, old)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// HasLiveOwner reports whether a committed name still belongs to an account.
func (s *fakeAvatarStore) HasLiveOwner(ctx context.Context, name string) (bool, error) {
	if s.hasErr != nil {
		return false, s.hasErr
	}
	_, ok := s.live[name]
	return ok, nil
}

// avatarHTTPFixture is one server plus the fakes behind it.
type avatarHTTPFixture struct {
	t        *testing.T
	srv      *server
	app      *fiber.App
	store    *fakeAvatarStore
	photos   *fakePhotoProcessor
	sessions *fakeSessionResolver
	csrf     string
}

// newAvatarHTTPFixture builds the real route table over injected fakes.
func newAvatarHTTPFixture(t *testing.T) *avatarHTTPFixture {
	t.Helper()
	dir := t.TempDir()
	srv, err := newServer(
		config.Config{SessionSigningKey: strings.Repeat("k", 32), UploadsDir: dir},
		nil,
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	resolver := &fakeSessionResolver{sessions: map[string]*authn.Session{
		avatarTestStudentToken: {
			ID: 1, AccountID: 11, Role: viewmodel.RoleStudent,
			FullName: "Siti Aminah", Email: "siti@example.sch.id",
			GradeLevel: 11, MajorName: "Rekayasa Perangkat Lunak",
		},
		avatarTestTeacherToken: {
			ID: 2, AccountID: 22, Role: viewmodel.RoleTeacher,
			FullName: "Budi Santoso", Email: "guru@sekolah.sch.id",
		},
		avatarTestMustChangeToken: {
			ID: 3, AccountID: 33, Role: viewmodel.RoleStudent,
			FullName: "Ayu Lestari", Email: "ayu@example.sch.id",
			MustChangePassword: true, GradeLevel: 10,
		},
	}}
	srv.sessions = resolver
	srv.Photos = &fakePhotoProcessor{}
	srv.Avatars = newFakeAvatarStore(dir)

	csrf, err := srv.signer.CSRF()
	if err != nil {
		t.Fatalf("mint CSRF: %v", err)
	}

	return &avatarHTTPFixture{
		t:        t,
		srv:      srv,
		app:      newAvatarTestApp(srv),
		store:    srv.Avatars.(*fakeAvatarStore),
		photos:   srv.Photos.(*fakePhotoProcessor),
		sessions: resolver,
		csrf:     csrf,
	}
}

// newAvatarTestApp mounts the registered route table plus the two asset routes.
// The route order matches production so /profile/photo/remove wins over any
// parameterised prefix.
func newAvatarTestApp(srv *server) *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit:                    maxUploadBody,
		StreamRequestBody:            true,
		DisablePreParseMultipartForm: true,
	})
	routes := RegisteredPages()
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
	app.Get("/avatars/:name", srv.serveAvatar)
	return app
}

// photoPart is one multipart field: a filename makes it a file part.
type photoPart struct {
	name     string
	filename string
	content  []byte
}

// multipartBody builds a multipart body and its Content-Type.
func multipartBody(t *testing.T, parts ...photoPart) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, p := range parts {
		if p.filename == "" {
			if err := w.WriteField(p.name, string(p.content)); err != nil {
				t.Fatalf("write field %q: %v", p.name, err)
			}
			continue
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition",
			fmt.Sprintf(`form-data; name=%q; filename=%q`, p.name, p.filename))
		header.Set("Content-Type", "application/octet-stream")
		fw, err := w.CreatePart(header)
		if err != nil {
			t.Fatalf("create part %q: %v", p.name, err)
		}
		if _, err := fw.Write(p.content); err != nil {
			t.Fatalf("write part %q: %v", p.name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return buf.Bytes(), w.FormDataContentType()
}

// uploadParts returns a normal token-first upload.
func (f *avatarHTTPFixture) uploadParts(photo []byte) []photoPart {
	return []photoPart{
		{name: "csrf_token", content: []byte(f.csrf)},
		{name: "photo", filename: "photo.jpg", content: photo},
	}
}

// post sends one request with the session and CSRF cookies attached.
func (f *avatarHTTPFixture) post(target, contentType string, body []byte, sessionToken string) *http.Response {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set(fiber.HeaderContentType, contentType)
	}
	if sessionToken != "" {
		req.AddCookie(&http.Cookie{Name: cookieSession, Value: sessionToken})
	}
	req.AddCookie(&http.Cookie{Name: cookieCSRF, Value: f.csrf})
	resp, err := f.app.Test(req, avatarTestTimeout)
	if err != nil {
		f.t.Fatalf("POST %s: %v", target, err)
	}
	return resp
}

// upload posts a multipart body through the photo route.
func (f *avatarHTTPFixture) upload(sessionToken string, parts ...photoPart) *http.Response {
	f.t.Helper()
	body, contentType := multipartBody(f.t, parts...)
	return f.post("/profile/photo", contentType, body, sessionToken)
}

// flashKey parses the signed flash cookie into its key.
func (f *avatarHTTPFixture) flashKey(resp *http.Response) string {
	f.t.Helper()
	for _, cookie := range resp.Cookies() {
		if cookie.Name != cookieFlash || cookie.Value == "" {
			continue
		}
		payload, err := f.srv.signer.ParseToken("flash", cookie.Value, 60*time.Second, time.Now().UTC())
		if err != nil {
			f.t.Fatalf("parse flash: %v", err)
		}
		return string(payload)
	}
	return ""
}

// expectRedirect asserts a 303 to target with the given flash key.
func (f *avatarHTTPFixture) expectRedirect(resp *http.Response, target, flash string) {
	f.t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		f.t.Fatalf("status = %d, want 303 (body %q)", resp.StatusCode, body)
	}
	if got := resp.Header.Get(fiber.HeaderLocation); got != target {
		f.t.Errorf("Location = %q, want %q", got, target)
	}
	if got := f.flashKey(resp); got != flash {
		f.t.Errorf("flash = %q, want %q", got, flash)
	}
}

// expectStatus asserts a status and returns the body.
func expectStatus(t *testing.T, resp *http.Response, want int) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d (body %q)", resp.StatusCode, want, body)
	}
	return string(body)
}

// assertNoTempFiles proves no dot-prefixed staging file survived.
func (f *avatarHTTPFixture) assertNoTempFiles() {
	f.t.Helper()
	entries, err := os.ReadDir(f.store.Dir())
	if err != nil {
		f.t.Fatalf("read avatar dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			f.t.Errorf("staging temp file %q survived", entry.Name())
		}
	}
}

// assertNoPhoto proves a failed upload committed nothing and left no staging
// file behind. A conversion failure may have called the codec, so processed is
// checked only where the test also asserts no byte was read before the token.
func (f *avatarHTTPFixture) assertNoPhoto() {
	f.t.Helper()
	if len(f.store.names) != 0 {
		f.t.Fatalf("store committed a photo: %+v", f.store.names)
	}
	f.assertNoTempFiles()
}

func TestAvatarUploadStudentAndTeacher(t *testing.T) {
	f := newAvatarHTTPFixture(t)

	t.Run("student", func(t *testing.T) {
		resp := f.upload(avatarTestStudentToken, f.uploadParts([]byte("jpeg bytes"))...)
		f.expectRedirect(resp, "/profile", flashPhotoSaved)

		name := f.store.names[11]
		if name == "" {
			t.Fatal("student photo was not committed")
		}
		if !avatarid.Valid(name) {
			t.Fatalf("committed name %q is outside the avatar grammar", name)
		}
		data, err := os.ReadFile(filepath.Join(f.store.Dir(), name))
		if err != nil {
			t.Fatalf("read committed photo: %v", err)
		}
		if !bytes.Equal(data, avatarTestWebP) {
			t.Errorf("committed bytes = %q, want the fake codec output", data)
		}
		if f.photos.acquired != 1 || f.photos.released != 1 {
			t.Errorf("slots acquired/released = %d/%d, want 1/1", f.photos.acquired, f.photos.released)
		}
		if f.photos.lastSrc == f.photos.lastDst {
			t.Errorf("codec source and destination are both %q", f.photos.lastSrc)
		}
		if _, err := os.Stat(f.photos.lastSrc); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("staged source %q survived", f.photos.lastSrc)
		}
		f.assertNoTempFiles()
	})

	t.Run("teacher", func(t *testing.T) {
		resp := f.upload(avatarTestTeacherToken, f.uploadParts([]byte("jpeg bytes"))...)
		f.expectRedirect(resp, "/teacher/profile", flashPhotoSaved)

		name := f.store.names[22]
		if name == "" {
			t.Fatal("teacher photo was not committed")
		}
		if _, err := os.Stat(filepath.Join(f.store.Dir(), name)); err != nil {
			t.Fatalf("teacher photo file: %v", err)
		}
		f.assertNoTempFiles()
	})
}

func TestAvatarUploadCSRFOrderingAndSessionBinding(t *testing.T) {
	f := newAvatarHTTPFixture(t)

	cases := []struct {
		name  string
		parts []photoPart
		csrf  string
	}{
		{
			name:  "missing token",
			parts: []photoPart{{name: "photo", filename: "photo.jpg", content: []byte("jpeg bytes")}},
		},
		{
			name: "token after the file",
			parts: []photoPart{
				{name: "photo", filename: "photo.jpg", content: []byte("jpeg bytes")},
				{name: "csrf_token", content: []byte(f.csrf)},
			},
		},
		{
			name: "token from another session",
			parts: []photoPart{
				{name: "csrf_token", content: []byte("someone-elses-token")},
				{name: "photo", filename: "photo.jpg", content: []byte("jpeg bytes")},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := f.upload(avatarTestStudentToken, tc.parts...)
			expectStatus(t, resp, http.StatusForbidden)
			f.assertNoPhoto()
			if f.store.commits != 0 {
				t.Errorf("store commits = %d, want 0", f.store.commits)
			}
			if f.photos.acquired != 0 || f.photos.processed != 0 {
				t.Errorf("codec touched the body before CSRF: acquired=%d processed=%d",
					f.photos.acquired, f.photos.processed)
			}
		})
	}

	t.Run("no CSRF cookie", func(t *testing.T) {
		body, contentType := multipartBody(t, f.uploadParts([]byte("jpeg bytes"))...)
		req := httptest.NewRequest(http.MethodPost, "/profile/photo", bytes.NewReader(body))
		req.Header.Set(fiber.HeaderContentType, contentType)
		req.AddCookie(&http.Cookie{Name: cookieSession, Value: avatarTestStudentToken})
		resp, err := f.app.Test(req, avatarTestTimeout)
		if err != nil {
			t.Fatalf("POST photo: %v", err)
		}
		expectStatus(t, resp, http.StatusForbidden)
		f.assertNoPhoto()
	})

	t.Run("token part over the field cap", func(t *testing.T) {
		resp := f.upload(avatarTestStudentToken,
			photoPart{name: "csrf_token", content: bytes.Repeat([]byte("x"), csrfTokenCap+1)},
			photoPart{name: "photo", filename: "photo.jpg", content: []byte("jpeg bytes")},
		)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		f.assertNoPhoto()
	})
}

func TestAvatarUploadMustChange(t *testing.T) {
	f := newAvatarHTTPFixture(t)

	t.Run("valid token", func(t *testing.T) {
		resp := f.upload(avatarTestMustChangeToken, f.uploadParts([]byte("jpeg bytes"))...)
		f.expectRedirect(resp, "/change-password", identity.FlashMustChange)
		f.assertNoPhoto()
	})

	t.Run("bad token", func(t *testing.T) {
		resp := f.upload(avatarTestMustChangeToken,
			photoPart{name: "csrf_token", content: []byte("wrong")},
			photoPart{name: "photo", filename: "photo.jpg", content: []byte("jpeg bytes")},
		)
		expectStatus(t, resp, http.StatusForbidden)
		f.assertNoPhoto()
	})
}

func TestAvatarUploadFlashMappings(t *testing.T) {
	f := newAvatarHTTPFixture(t)

	// An empty multipart body has no parts at all: the handler must treat it
	// as a user miss (photo-empty), never as an attempted CSRF (403).
	emptyBody, emptyType := multipartBody(t)
	resp := f.post("/profile/photo", emptyType, emptyBody, avatarTestStudentToken)
	f.expectRedirect(resp, "/profile", flashPhotoEmpty)
	f.assertNoPhoto()

	cases := []struct {
		name       string
		photo      []byte
		processErr error
		acquireErr error
		want       string
	}{
		{"empty photo part", nil, nil, nil, flashPhotoEmpty},
		{"text bytes", []byte("not an image"), avatar.ErrType, nil, flashPhotoType},
		{"oversized pixels", []byte("header"), avatar.ErrBig, nil, flashPhotoBig},
		{"undecodable heic", []byte("heic"), avatar.ErrUnreadable, nil, flashPhotoUnreadable},
		{"generic conversion failure", []byte("jpeg"), avatar.ErrFailed, nil, flashPhotoFailed},
		{"full pool", []byte("jpeg"), nil, avatar.ErrBusy, flashPhotoBusy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f.photos.processErr = tc.processErr
			f.photos.acquireErr = tc.acquireErr
			f.photos.processed = 0
			f.photos.acquired = 0
			f.photos.released = 0

			resp := f.upload(avatarTestStudentToken, f.uploadParts(tc.photo)...)
			f.expectRedirect(resp, "/profile", tc.want)
			f.assertNoPhoto()
		})
	}

	t.Run("commit failure", func(t *testing.T) {
		f.photos.processErr = nil
		f.photos.acquireErr = nil
		f.photos.processed = 0
		f.store.commitErr = errors.New("database down")
		resp := f.upload(avatarTestStudentToken, f.uploadParts([]byte("jpeg"))...)
		f.expectRedirect(resp, "/profile", flashPhotoFailed)
		f.store.commitErr = nil
		f.assertNoTempFiles()
		if len(f.store.names) != 0 {
			t.Fatalf("store kept a name after a failed commit: %+v", f.store.names)
		}
	})

	t.Run("session vanished at commit", func(t *testing.T) {
		f.store.commitErr = identity.ErrSessionGone
		resp := f.upload(avatarTestStudentToken, f.uploadParts([]byte("jpeg"))...)
		f.expectRedirect(resp, "/login", identity.FlashSessionExpired)
		f.store.commitErr = nil
		f.assertNoTempFiles()
	})

	t.Run("second photo part", func(t *testing.T) {
		resp := f.upload(avatarTestStudentToken,
			photoPart{name: "csrf_token", content: []byte(f.csrf)},
			photoPart{name: "photo", filename: "one.jpg", content: []byte("jpeg")},
			photoPart{name: "photo", filename: "two.jpg", content: []byte("jpeg")},
		)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		f.assertNoTempFiles()
	})
}

func TestAvatarUploadPartCap(t *testing.T) {
	f := newAvatarHTTPFixture(t)

	// The token is part one; fourteen extras plus the photo are the remaining
	// fifteen, so the body is exactly at the cap.
	parts := []photoPart{{name: "csrf_token", content: []byte(f.csrf)}}
	for i := 0; i < avatarPartsCap-2; i++ {
		parts = append(parts, photoPart{name: fmt.Sprintf("extra-%d", i), content: []byte("x")})
	}
	parts = append(parts, photoPart{name: "photo", filename: "photo.jpg", content: []byte("jpeg")})
	resp := f.upload(avatarTestStudentToken, parts...)
	f.expectRedirect(resp, "/profile", flashPhotoSaved)

	// One more extra crosses the cap: the request is transport garbage, so
	// the generic 422 applies and nothing is committed.
	f.store.names = map[int64]string{}
	f.store.live = map[string]int64{}
	parts = []photoPart{{name: "csrf_token", content: []byte(f.csrf)}}
	for i := 0; i < avatarPartsCap-1; i++ {
		parts = append(parts, photoPart{name: fmt.Sprintf("extra-%d", i), content: []byte("x")})
	}
	parts = append(parts, photoPart{name: "photo", filename: "photo.jpg", content: []byte("jpeg")})
	resp = f.upload(avatarTestStudentToken, parts...)
	expectStatus(t, resp, http.StatusUnprocessableEntity)
	f.assertNoTempFiles()
	if len(f.store.names) != 0 {
		t.Fatalf("store committed after the part cap: %+v", f.store.names)
	}
}

func TestStagePhotoPartCapUnit(t *testing.T) {
	run := func(parts int) (int64, error) {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		for i := 0; i < parts; i++ {
			if i == parts-1 {
				fw, err := w.CreateFormFile("photo", "photo.jpg")
				if err != nil {
					t.Fatalf("create photo: %v", err)
				}
				_, _ = fw.Write([]byte("jpeg"))
				continue
			}
			if err := w.WriteField(fmt.Sprintf("extra-%d", i), "x"); err != nil {
				t.Fatalf("write field: %v", err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		dst, err := os.CreateTemp(t.TempDir(), "stage-*")
		if err != nil {
			t.Fatalf("temp: %v", err)
		}
		defer dst.Close()
		return stagePhoto(multipart.NewReader(&body, w.Boundary()), dst)
	}

	// The caller already consumed the token, so fifteen remaining parts are a
	// sixteen-part body in total and stay legal.
	if n, err := run(avatarPartsCap - 1); err != nil {
		t.Fatalf("%d remaining parts: %v", avatarPartsCap-1, err)
	} else if n != int64(len("jpeg")) {
		t.Fatalf("%d remaining parts staged %d bytes, want %d", avatarPartsCap-1, n, len("jpeg"))
	}

	// Sixteen remaining parts make seventeen in total: the cap rejects it
	// before the extra field's body is read.
	if _, err := run(avatarPartsCap); !errors.Is(err, errFieldTooLong) {
		t.Fatalf("%d remaining parts: got %v, want errFieldTooLong", avatarPartsCap, err)
	}
}

func TestAvatarUploadDeclaredOverCap(t *testing.T) {
	f := newAvatarHTTPFixture(t)

	req := httptest.NewRequest(http.MethodPost, "/profile/photo", bytes.NewReader(make([]byte, 8<<10)))
	req.ContentLength = maxAvatarRequest + 1
	req.Header.Set(fiber.HeaderContentType, "multipart/form-data; boundary=xyz")
	req.AddCookie(&http.Cookie{Name: cookieSession, Value: avatarTestStudentToken})
	req.AddCookie(&http.Cookie{Name: cookieCSRF, Value: f.csrf})
	resp, err := f.app.Test(req, avatarTestTimeout)
	if err != nil {
		t.Fatalf("POST over-cap photo: %v", err)
	}
	body := expectStatus(t, resp, http.StatusRequestEntityTooLarge)
	if !strings.Contains(body, apperr.MsgTooLarge) {
		t.Errorf("413 body does not carry %q", apperr.MsgTooLarge)
	}
	if !strings.Contains(body, `href="/"`) {
		t.Errorf("413 page must link HomeHref \"/\"")
	}
	// fasthttp surfaces SetConnectionClose as Response.Close rather than a
	// literal Connection header in the client response.
	if !resp.Close {
		t.Errorf("response does not close the connection")
	}
	f.assertNoPhoto()

	// The preview route renders the same template and copy without a 200 MB
	// request, so the real page cannot drift from the audited preview. It is a
	// normal 200 page whose content is the 413 screen.
	preview, err := f.app.Test(httptest.NewRequest(http.MethodGet, "/error/413", nil), avatarTestTimeout)
	if err != nil {
		t.Fatalf("GET /error/413: %v", err)
	}
	previewBody := expectStatus(t, preview, http.StatusOK)
	for _, want := range []string{apperr.MsgTooLarge, `href="/"`, "Error 413"} {
		if !strings.Contains(previewBody, want) {
			t.Errorf("preview 413 missing %q", want)
		}
	}
	if !strings.Contains(body, "Error 413") {
		t.Errorf("real 413 missing the error code")
	}
}

func TestAvatarRemove(t *testing.T) {
	f := newAvatarHTTPFixture(t)
	seed := func(accountID int64, name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(f.store.Dir(), name), avatarTestWebP, 0o640); err != nil {
			t.Fatalf("seed avatar: %v", err)
		}
		f.store.names[accountID] = name
		f.store.live[name] = accountID
	}

	t.Run("student", func(t *testing.T) {
		name := strings.Repeat("a", 32) + ".webp"
		seed(11, name)
		body := "csrf_token=" + f.csrf
		resp := f.post("/profile/photo/remove", "application/x-www-form-urlencoded", []byte(body), avatarTestStudentToken)
		f.expectRedirect(resp, "/profile", flashPhotoRemoved)
		if got := f.store.names[11]; got != "" {
			t.Errorf("student name = %q, want cleared", got)
		}
		if _, err := os.Stat(filepath.Join(f.store.Dir(), name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("avatar file %q survived removal", name)
		}
	})

	t.Run("teacher", func(t *testing.T) {
		name := strings.Repeat("b", 32) + ".webp"
		seed(22, name)
		body := "csrf_token=" + f.csrf
		resp := f.post("/profile/photo/remove", "application/x-www-form-urlencoded", []byte(body), avatarTestTeacherToken)
		f.expectRedirect(resp, "/teacher/profile", flashPhotoRemoved)
		if got := f.store.names[22]; got != "" {
			t.Errorf("teacher name = %q, want cleared", got)
		}
	})

	t.Run("no photo is idempotent", func(t *testing.T) {
		body := "csrf_token=" + f.csrf
		resp := f.post("/profile/photo/remove", "application/x-www-form-urlencoded", []byte(body), avatarTestStudentToken)
		f.expectRedirect(resp, "/profile", flashPhotoRemoved)
	})

	t.Run("store failure", func(t *testing.T) {
		f.store.removeErr = errors.New("database down")
		body := "csrf_token=" + f.csrf
		resp := f.post("/profile/photo/remove", "application/x-www-form-urlencoded", []byte(body), avatarTestStudentToken)
		f.expectRedirect(resp, "/profile", flashPhotoRemoveFailed)
		f.store.removeErr = nil
	})

	t.Run("session vanished", func(t *testing.T) {
		f.store.removeErr = identity.ErrSessionGone
		body := "csrf_token=" + f.csrf
		resp := f.post("/profile/photo/remove", "application/x-www-form-urlencoded", []byte(body), avatarTestStudentToken)
		f.expectRedirect(resp, "/login", identity.FlashSessionExpired)
		f.store.removeErr = nil
	})

	t.Run("missing CSRF", func(t *testing.T) {
		resp := f.post("/profile/photo/remove", "application/x-www-form-urlencoded", []byte("csrf_token=wrong"), avatarTestStudentToken)
		expectStatus(t, resp, http.StatusForbidden)
	})
}

func TestServeAvatar(t *testing.T) {
	f := newAvatarHTTPFixture(t)
	name := strings.Repeat("c", 32) + ".webp"
	path := filepath.Join(f.store.Dir(), name)
	if err := os.WriteFile(path, avatarTestWebP, 0o640); err != nil {
		t.Fatalf("write avatar: %v", err)
	}
	f.store.live[name] = 11

	t.Run("200 headers", func(t *testing.T) {
		resp, err := f.app.Test(httptest.NewRequest(http.MethodGet, "/avatars/"+name, nil), avatarTestTimeout)
		if err != nil {
			t.Fatalf("GET avatar: %v", err)
		}
		body := expectStatus(t, resp, http.StatusOK)
		if !bytes.Equal([]byte(body), avatarTestWebP) {
			t.Errorf("body = %q, want the stored bytes", body)
		}
		wantETag := `"` + name + `"`
		for header, want := range map[string]string{
			fiber.HeaderContentType:         "image/webp",
			fiber.HeaderCacheControl:        avatarCacheControl,
			fiber.HeaderETag:                wantETag,
			fiber.HeaderContentDisposition:  "inline",
			fiber.HeaderXContentTypeOptions: "nosniff",
			fiber.HeaderContentLength:       fmt.Sprint(len(avatarTestWebP)),
		} {
			if got := resp.Header.Get(header); got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		}
		if resp.Header.Get(fiber.HeaderLastModified) == "" {
			t.Error("Last-Modified is missing")
		}
	})

	t.Run("304 on If-None-Match", func(t *testing.T) {
		wantETag := `"` + name + `"`
		for _, header := range []string{wantETag, "W/" + wantETag, `"other", ` + wantETag, "*"} {
			req := httptest.NewRequest(http.MethodGet, "/avatars/"+name, nil)
			req.Header.Set(fiber.HeaderIfNoneMatch, header)
			resp, err := f.app.Test(req, avatarTestTimeout)
			if err != nil {
				t.Fatalf("conditional GET %q: %v", header, err)
			}
			_ = expectStatus(t, resp, http.StatusNotModified)
			if got := resp.Header.Get(fiber.HeaderETag); got != wantETag {
				t.Errorf("304 ETag = %q, want %q", got, wantETag)
			}
			if got := resp.Header.Get(fiber.HeaderContentLength); got != "" {
				t.Errorf("304 Content-Length = %q, want empty", got)
			}
		}
	})

	t.Run("If-Modified-Since alone is ignored", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/avatars/"+name, nil)
		req.Header.Set(fiber.HeaderIfModifiedSince, time.Now().UTC().Add(time.Hour).Format(http.TimeFormat))
		resp, err := f.app.Test(req, avatarTestTimeout)
		if err != nil {
			t.Fatalf("GET avatar: %v", err)
		}
		_ = expectStatus(t, resp, http.StatusOK)
	})

	t.Run("404 misses", func(t *testing.T) {
		dirName := strings.Repeat("d", 32) + ".webp"
		if err := os.Mkdir(filepath.Join(f.store.Dir(), dirName), 0o755); err != nil {
			t.Fatalf("mkdir avatar: %v", err)
		}
		emptyName := strings.Repeat("e", 32) + ".webp"
		if err := os.WriteFile(filepath.Join(f.store.Dir(), emptyName), nil, 0o640); err != nil {
			t.Fatalf("write empty avatar: %v", err)
		}
		bigName := strings.Repeat("f", 32) + ".webp"
		if err := os.WriteFile(filepath.Join(f.store.Dir(), bigName), make([]byte, avatar.MaxAvatarBytes+1), 0o640); err != nil {
			t.Fatalf("write oversize avatar: %v", err)
		}
		f.store.live[emptyName] = 11
		f.store.live[bigName] = 11
		f.store.live[dirName] = 11

		names := []string{
			"x.webp",                                  // bad grammar
			strings.Repeat("A", 32) + ".webp",         // uppercase alias
			strings.Repeat("a", 31) + ".webp",         // short
			strings.Repeat("a", 32) + ".png",          // wrong extension
			"../" + strings.Repeat("a", 32) + ".webp", // traversal
			strings.Repeat("9", 32) + ".webp",         // orphan
			dirName, emptyName, bigName,
		}
		for _, miss := range names {
			resp, err := f.app.Test(httptest.NewRequest(http.MethodGet, "/avatars/"+miss, nil), avatarTestTimeout)
			if err != nil {
				t.Fatalf("GET %q: %v", miss, err)
			}
			_ = expectStatus(t, resp, http.StatusNotFound)
		}
	})

	t.Run("symlink is not served", func(t *testing.T) {
		linkName := strings.Repeat("1", 32) + ".webp"
		target := filepath.Join(t.TempDir(), "target.webp")
		if err := os.WriteFile(target, avatarTestWebP, 0o640); err != nil {
			t.Fatalf("write target: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(f.store.Dir(), linkName)); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		f.store.live[linkName] = 11
		resp, err := f.app.Test(httptest.NewRequest(http.MethodGet, "/avatars/"+linkName, nil), avatarTestTimeout)
		if err != nil {
			t.Fatalf("GET symlink: %v", err)
		}
		_ = expectStatus(t, resp, http.StatusNotFound)
	})

	t.Run("owner lookup failure is 500", func(t *testing.T) {
		f.store.hasErr = errors.New("database down")
		resp, err := f.app.Test(httptest.NewRequest(http.MethodGet, "/avatars/"+name, nil), avatarTestTimeout)
		if err != nil {
			t.Fatalf("GET avatar: %v", err)
		}
		_ = expectStatus(t, resp, http.StatusInternalServerError)
		f.store.hasErr = nil
	})
}
