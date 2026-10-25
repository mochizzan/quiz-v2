package httpserver

import (
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/authoring"
)

// upload.go serves the question images the authoring domain stores on disk.
// GET /uploads/:name is a read-only operational route: it is not a page, it has
// no template, and it is registered with /healthz rather than through
// registerPage, so the 74-screen pin is unaffected (contract §6).
//
// The name is treated as hostile. The closed grammar of AUT-015 is checked
// before anything else, then the name must resolve to a live question of a live
// quiz, then the file must be a regular file of exactly the stored size. The
// file is never listed, transformed, ranged, or served from a name that no live
// row references (AUT-054, AUT-INV-14).

// uploadCacheControl is the one cache policy the route may advertise: shared
// caches never store a capability image, and the browser revalidates every time
// because the file can disappear when the question or quiz is deleted.
const uploadCacheControl = "private, no-cache, must-revalidate"

// serveUpload answers GET /uploads/:name.
func (s *server) serveUpload(c fiber.Ctx) error {
	name := c.Params("name")

	// Grammar first: no database or filesystem call for a malformed name.
	// Uppercase hex, `.jpeg`, extra dots, separators and traversal are 404.
	if !authoring.ValidImageName(name) {
		return c.SendStatus(fiber.StatusNotFound)
	}

	// A server built without a pool has no live row to resolve. That is the
	// orphan case, not an error page (AUT-054).
	if s.db == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}

	svc := authoring.New(s.db, s.cfg.UploadsDir)
	record, err := svc.LookupImage(c.Context(), name)
	if err != nil {
		if apperr.IsNotFound(err) {
			return c.SendStatus(fiber.StatusNotFound)
		}
		appErr := apperr.From(err)
		s.logger.Error("upload lookup failed", "rule", appErr.Rule(), "name", name, "error", appErr)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	data, status := s.readUpload(c, record, name)
	if status != 0 {
		return c.SendStatus(status)
	}
	return c.Send(data)
}

// readUpload validates the stored file and writes the 200/304 headers. It
// returns a non-zero status when the caller must answer without a body.
func (s *server) readUpload(c fiber.Ctx, record *authoring.ImageRecord, name string) ([]byte, int) {
	// The grammar guarantees a single path segment, so Join cannot escape the
	// upload root. Lstat refuses a symlink: only a regular file is served.
	full := filepath.Join(s.cfg.UploadsDir, name)
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() || info.Size() != record.Bytes {
		s.logUploadMissing(record)
		return nil, fiber.StatusNotFound
	}

	// Read before any header is written, so a read failure is a clean 500
	// rather than a truncated 200 (AUT-015).
	data, err := os.ReadFile(full)
	if err != nil {
		s.logger.Error("upload read failed", "rule", "AUT-015", "name", name,
			"question_id", record.QuestionID, "error", err)
		return nil, fiber.StatusInternalServerError
	}
	// A file swapped between Lstat and ReadFile is a size drift: 404, never a
	// body that disagrees with image_bytes (AUT-054).
	if int64(len(data)) != record.Bytes {
		s.logUploadMissing(record)
		return nil, fiber.StatusNotFound
	}

	etag := `"` + hex.EncodeToString(record.SHA256) + `"`
	c.Set(fiber.HeaderETag, etag)
	c.Set(fiber.HeaderLastModified, info.ModTime().UTC().Format(http.TimeFormat))
	c.Set(fiber.HeaderCacheControl, uploadCacheControl)
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	c.Set(fiber.HeaderContentDisposition, "inline")

	if etagMatches(c.Get(fiber.HeaderIfNoneMatch), etag) {
		return nil, fiber.StatusNotModified
	}

	c.Set(fiber.HeaderContentType, record.MIME)
	c.Set(fiber.HeaderContentLength, strconv.Itoa(len(data)))
	return data, 0
}

// logUploadMissing records the tombstone the spec requires (OPS-061). It logs
// the name and question id only, never image bytes.
func (s *server) logUploadMissing(record *authoring.ImageRecord) {
	s.logger.Warn("upload_missing", "rule", "OPS-061", "name", record.Name,
		"question_id", record.QuestionID)
}

// etagMatches implements the If-None-Match comparison of RFC 9110 §13.1.2:
// a list of entity tags, an optional weakness prefix, and the `*` wildcard.
// Weak comparison is the correct one for If-None-Match, so `W/"x"` matches.
func etagMatches(header, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}
