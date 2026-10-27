package httpserver

import (
	"path"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
)

// compressedExts are file formats that ship pre-compressed on disk or in the
// wire format itself (woff2 fonts, raster images, the XLSX zip): gzipping
// them again burns CPU and shrinks nothing.
var compressedExts = map[string]bool{
	".avif": true, ".gif": true, ".ico": true, ".jpeg": true,
	".jpg": true, ".png": true, ".webp": true, ".woff": true,
	".woff2": true, ".xlsx": true,
}

// skipCompression keeps the compressor away from paths where compression is
// wrong or pointless:
//   - GET …/stream — the SSE monitor stream (ATT-191): compression would
//     buffer live events; the route's own no-transform header exempts it too;
//   - POST …/beacon — the one CSRF-exempt endpoint (ATT-156) stays
//     byte-exact;
//   - /avatars and /uploads — ETag'd binary assets (AVT-062);
//   - /healthz — a two-byte probe gains nothing;
//   - any pre-compressed file format by extension, wherever it lives
//     (the export is /teacher/quizzes/:id/export.xlsx).
func skipCompression(c fiber.Ctx) bool {
	p := c.Path()
	if p == "/healthz" {
		return true
	}
	if c.Method() == fiber.MethodGet && strings.HasSuffix(p, "/stream") {
		return true
	}
	if c.Method() == fiber.MethodPost && strings.HasSuffix(p, "/beacon") {
		return true
	}
	if strings.HasPrefix(p, "/avatars/") || strings.HasPrefix(p, "/uploads/") {
		return true
	}
	return compressedExts[path.Ext(p)]
}

// newCompress returns the app's compression middleware. Text representations
// (HTML, CSS, JS, SVG) are compressed at best speed — measured −72% on
// /login and −82% on bootstrap.min.css — while everything above is passed
// through identity-encoded. Fiber's middleware adds Vary: Accept-Encoding on
// every path, skips HEAD, 204/206/304, Range requests, and responses that
// declare no-transform, and re-stamps the ETag when compression changes the
// representation.
func newCompress() fiber.Handler {
	return compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
		Next:  skipCompression,
	})
}
