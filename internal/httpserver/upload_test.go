package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/authoring"
	"quiz-v2/internal/config"
)

// uploadName is a well-formed generated name for the fixture file.
func uploadName() string { return strings.Repeat("a", 32) + ".png" }

// uploadFixture writes one file into a temp root and returns the server plus
// the record the domain lookup would have produced (AUT-054).
func uploadFixture(t *testing.T, data []byte) (*server, *authoring.ImageRecord) {
	t.Helper()
	dir := t.TempDir()
	name := uploadName()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	sha := sha256.Sum256(data)
	return &server{
			cfg:    config.Config{UploadsDir: dir},
			logger: slog.New(slog.DiscardHandler),
		}, &authoring.ImageRecord{
			Name:   name,
			MIME:   "image/png",
			Bytes:  int64(len(data)),
			SHA256: sha[:],
		}
}

// uploadApp mounts readUpload behind the same route shape as serveUpload so
// the header contract can be tested without a database.
func uploadApp(srv *server, record *authoring.ImageRecord) *fiber.App {
	app := fiber.New()
	app.Get("/uploads/:name", func(c fiber.Ctx) error {
		body, status := srv.readUpload(c, record, c.Params("name"))
		if status != 0 {
			return c.SendStatus(status)
		}
		return c.Send(body)
	})
	return app
}

func TestReadUploadHeadersAndValidators(t *testing.T) {
	data := []byte("not-a-real-png-but-the-route-never-sniffs")
	srv, record := uploadFixture(t, data)
	app := uploadApp(srv, record)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/uploads/"+record.Name, nil))
	if err != nil {
		t.Fatalf("GET upload: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get(fiber.HeaderContentType); got != "image/png" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := resp.Header.Get(fiber.HeaderContentLength); got != strconv.Itoa(len(data)) {
		t.Fatalf("Content-Length = %q", got)
	}
	wantETag := `"` + hex.EncodeToString(record.SHA256) + `"`
	if got := resp.Header.Get(fiber.HeaderETag); got != wantETag {
		t.Fatalf("ETag = %q, want %q", got, wantETag)
	}
	if got := resp.Header.Get(fiber.HeaderCacheControl); got != uploadCacheControl {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := resp.Header.Get(fiber.HeaderXContentTypeOptions); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := resp.Header.Get(fiber.HeaderContentDisposition); got != "inline" {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := resp.Header.Get(fiber.HeaderLastModified); got == "" {
		t.Fatalf("Last-Modified is missing")
	}

	// The ETag is honoured as a weak comparison, as a list, and as `*`.
	for _, header := range []string{wantETag, "W/" + wantETag, `"other", ` + wantETag, "*"} {
		req := httptest.NewRequest(http.MethodGet, "/uploads/"+record.Name, nil)
		req.Header.Set(fiber.HeaderIfNoneMatch, header)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("conditional GET %q: %v", header, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotModified {
			t.Fatalf("If-None-Match %q status = %d, want 304", header, resp.StatusCode)
		}
		if got := resp.Header.Get(fiber.HeaderETag); got != wantETag {
			t.Fatalf("304 ETag = %q", got)
		}
		if got := resp.Header.Get(fiber.HeaderCacheControl); got != uploadCacheControl {
			t.Fatalf("304 Cache-Control = %q", got)
		}
		if got := resp.Header.Get(fiber.HeaderContentLength); got != "" {
			t.Fatalf("304 Content-Length = %q, want empty", got)
		}
	}

	// A non-matching validator is a full response.
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+record.Name, nil)
	req.Header.Set(fiber.HeaderIfNoneMatch, `"stale"`)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("stale GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stale If-None-Match status = %d, want 200", resp.StatusCode)
	}
}

func TestReadUploadRejectsSizeDriftAndNonRegular(t *testing.T) {
	data := []byte("0123456789")
	srv, record := uploadFixture(t, data)
	app := uploadApp(srv, record)

	// Stored bytes differ from the file: the tombstone is a 404, never a body
	// that disagrees with the row (AUT-054).
	record.Bytes++
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/uploads/"+record.Name, nil))
	if err != nil {
		t.Fatalf("GET drifted: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("size drift status = %d, want 404", resp.StatusCode)
	}
	record.Bytes--

	// A directory that happens to carry the name is not a regular file.
	if err := os.Remove(filepath.Join(srv.cfg.UploadsDir, record.Name)); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(srv.cfg.UploadsDir, record.Name), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/uploads/"+record.Name, nil))
	if err != nil {
		t.Fatalf("GET directory: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("directory status = %d, want 404", resp.StatusCode)
	}
}

func TestServeUploadNameGrammar(t *testing.T) {
	// A malformed name is 404 before any pool or filesystem access. The nil
	// pool also pins the orphan case: no live row can exist, so a valid
	// name is 404 as well, never a 500 page (AUT-054).
	srv := &server{logger: slog.New(slog.DiscardHandler)}
	app := fiber.New()
	app.Get("/uploads/:name", srv.serveUpload)

	names := []string{
		"x.png",
		strings.Repeat("A", 32) + ".png",
		strings.Repeat("a", 32) + ".jpeg",
		strings.Repeat("a", 32) + ".gif",
		strings.Repeat("a", 31) + ".png",
	}
	for _, name := range names {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/uploads/"+name, nil))
		if err != nil {
			t.Fatalf("GET %q: %v", name, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%q status = %d, want 404", name, resp.StatusCode)
		}
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/uploads/"+uploadName(), nil))
	if err != nil {
		t.Fatalf("GET valid: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("orphan upload status = %d, want 404", resp.StatusCode)
	}
}
