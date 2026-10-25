package httpserver_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// TestErrorPagesRender pins the styled error pages: the 404 catch-all
// (server.go renders pages/error/404.html for any unclaimed path) must answer
// 404 with the styled page, and the preview routes registered by
// routes_error.go must answer 200 with their own copy. TestMain chdirs to the
// repository root, so ./templates resolves under app.Test.
func TestErrorPagesRender(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})

	t.Run("404 catch-all", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest("GET", "/definitely-missing", nil))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 404 {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		html := string(body)
		if !strings.Contains(html, `lang="id"`) {
			t.Errorf("404 page must declare lang=id (spec Â§15.10), got %q", html)
		}
		if !strings.Contains(html, "Halaman tidak ditemukan") {
			t.Errorf("404 page must contain its heading copy, got %q", html)
		}
	})

	t.Run("GET /error/500", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest("GET", "/error/500", nil))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d, want 200 (route registered by routes_error.go)", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if html := string(body); !strings.Contains(html, "Terjadi kesalahan") {
			t.Errorf("500 page must contain its heading copy, got %q", html)
		}
	})

	// Every error screen is also a preview route, so its template is exercised
	// without provoking the real condition.
	for _, tc := range []struct{ path, heading string }{
		{"/error/403", "Akses ditolak"},
		{"/error/422", "Permintaan tidak valid"},
		{"/error/429", "Terlalu banyak permintaan"},
	} {
		t.Run("GET "+tc.path, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("GET", tc.path, nil))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d, want 200 (route registered by routes_error.go)", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			if html := string(body); !strings.Contains(html, tc.heading) {
				t.Errorf("%s page must contain its heading copy, got %q", tc.path, html)
			}
		})
	}
}

// TestLayoutWrappersArePresent pins the layout chrome every page renders
// through (Task 6 layouts + Task 5 head/foot partials), driven through the
// /error/500 page: carrier stylesheets with tokens.css AFTER bootstrap.min.css
// (spec Â§15.1), the Bootstrap ESM bundle script, the forced-light html root,
// and exactly ONE <style> element (the @font-face block from head.html).
func TestLayoutWrappersArePresent(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})
	resp, err := app.Test(httptest.NewRequest("GET", "/error/500", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	bootstrapIdx := strings.Index(html, `href="/static/vendor/bootstrap/bootstrap.min.css"`)
	tokensIdx := strings.Index(html, `href="/static/css/tokens.css"`)
	if bootstrapIdx < 0 || tokensIdx < 0 {
		t.Fatalf("page must link both stylesheets (bootstrap=%d tokens=%d)", bootstrapIdx, tokensIdx)
	}
	if bootstrapIdx > tokensIdx {
		t.Error("tokens.css link must appear AFTER bootstrap.min.css (spec Â§15.1)")
	}
	for _, want := range []string{
		`type="module" src="/static/vendor/bootstrap/bootstrap.bundle.min.js"`,
		`<html lang="id"`,
		`data-bs-theme="light"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
	if n := strings.Count(html, "<style"); n != 1 {
		t.Errorf("rendered page must carry exactly ONE <style> element (the @font-face block), found %d", n)
	}
}
