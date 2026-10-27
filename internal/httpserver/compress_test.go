package httpserver_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// gzipGet sends GET url with Accept-Encoding: gzip through the Fiber test
// client. All compression tests share it so the negotiation header is stated
// exactly once.
func gzipGet(t *testing.T, app *fiber.App, url string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// gunzip decodes a gzip response body.
func gunzip(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer zr.Close()
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	return body
}

// TestCompressLoginHTMLWithGzip: a client that offers gzip receives the login
// page as gzip, the decompressed bytes are the real page, and the response
// tells shared caches the body varies on Accept-Encoding.
func TestCompressLoginHTMLWithGzip(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})

	resp := gzipGet(t, app, "/login")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ce := resp.Header.Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", ce)
	}
	if vary := resp.Header.Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
		t.Errorf("Vary = %q, want it to contain Accept-Encoding", vary)
	}
	body := gunzip(t, resp)
	if !strings.Contains(string(body), "Masuk ke akun sekolah") {
		t.Error("decompressed body is not the login page")
	}
}

// TestCompressStaticTextWithGzip: vendored CSS and JS travel compressed while
// the one-day Cache-Control convention is untouched; a client without
// Accept-Encoding still receives identity bytes.
func TestCompressStaticTextWithGzip(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})
	const css = "/static/vendor/bootstrap/bootstrap.min.css"

	resp := gzipGet(t, app, css)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ce := resp.Header.Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("css Content-Encoding = %q, want gzip", ce)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "max-age=86400") {
		t.Errorf("css Cache-Control = %q, want it to contain max-age=86400", cc)
	}
	if vary := resp.Header.Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
		t.Errorf("css Vary = %q, want it to contain Accept-Encoding", vary)
	}

	// The compressed body must round-trip to the exact vendored bytes.
	plain, err := app.Test(httptest.NewRequest(http.MethodGet, css, nil))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(plain.Body)
	if got := gunzip(t, resp); !bytes.Equal(got, raw) {
		t.Errorf("gunzipped css = %d bytes, identity response = %d bytes, want equal", len(got), len(raw))
	}

	js := gzipGet(t, app, "/static/vendor/jquery/jquery-4.0.0.min.js")
	if ce := js.Header.Get("Content-Encoding"); ce != "gzip" {
		t.Errorf("js Content-Encoding = %q, want gzip", ce)
	}

	if ce := plain.Header.Get("Content-Encoding"); ce != "" {
		t.Errorf("identity request Content-Encoding = %q, want empty", ce)
	}
}

// TestCompressSkipsWoff2: Gotham Rounded faces are already compressed on
// disk; gzipping them again would only burn CPU for no smaller transfer, so
// the response must stay identity-encoded.
func TestCompressSkipsWoff2(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})

	resp := gzipGet(t, app, "/static/fonts/gotham-rounded/gotham-rounded-book.woff2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ce := resp.Header.Get("Content-Encoding"); ce != "" {
		t.Fatalf("woff2 Content-Encoding = %q, want empty", ce)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) < 4 || string(body[:4]) != "wOF2" {
		t.Error("woff2 body must survive uncompressed")
	}
}

// TestCompressSkipsHealthz: the probe answer is two bytes; compressing it
// would grow the body, so /healthz stays outside the compressor.
func TestCompressSkipsHealthz(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})

	resp := gzipGet(t, app, "/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ce := resp.Header.Get("Content-Encoding"); ce != "" {
		t.Fatalf("healthz Content-Encoding = %q, want empty", ce)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("healthz body = %q, want ok", body)
	}
}
