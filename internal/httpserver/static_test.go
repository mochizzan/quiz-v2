package httpserver_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// TestStaticBootstrapCSSServed: the vendored Bootstrap 6 stylesheet is served
// from /static with the go-web-ssr caching convention (MaxAge 86400).
func TestStaticBootstrapCSSServed(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080"})
	resp, err := app.Test(httptest.NewRequest("GET", "/static/bootstrap/bootstrap.min.css", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Errorf("Content-Type = %q, want it to contain text/css", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "max-age=86400") {
		t.Errorf("Cache-Control = %q, want it to contain max-age=86400", cc)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), ".btn-solid") {
		t.Error("bootstrap.min.css body must contain the v6-only class .btn-solid")
	}
}

// TestStaticJQueryServed: the vendored jQuery 4 build is served from /static.
func TestStaticJQueryServed(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080"})
	resp, err := app.Test(httptest.NewRequest("GET", "/static/jquery/jquery-4.0.0.min.js", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "jQuery") {
		t.Error("jquery body must contain jQuery")
	}
}

// TestStaticFontServed: the vendored Plus Jakarta Sans variable woff2 is
// served from /static (the file's binary signature is checked at vendoring,
// not here — see plan Task 1 step 2).
func TestStaticFontServed(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080"})
	resp, err := app.Test(httptest.NewRequest("GET", "/static/fonts/plus-jakarta-sans/PlusJakartaSans-latin.woff2", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
