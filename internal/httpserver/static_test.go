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
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})
	resp, err := app.Test(httptest.NewRequest("GET", "/static/vendor/bootstrap/bootstrap.min.css", nil))
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
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})
	resp, err := app.Test(httptest.NewRequest("GET", "/static/vendor/jquery/jquery-4.0.0.min.js", nil))
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

// TestStaticFontFacesServed: every Gotham Rounded face declared in
// head.html's @font-face block is vendored and served from /static. Checking
// the wOF2 magic catches a truncating or non-woff2 conversion.
func TestStaticFontFacesServed(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})
	faces := []string{
		"gotham-rounded-light", "gotham-rounded-light-italic",
		"gotham-rounded-book", "gotham-rounded-book-italic",
		"gotham-rounded-medium", "gotham-rounded-medium-italic",
		"gotham-rounded-bold", "gotham-rounded-bold-italic",
	}
	for _, face := range faces {
		t.Run(face, func(t *testing.T) {
			url := "/static/fonts/gotham-rounded/" + face + ".woff2"
			resp, err := app.Test(httptest.NewRequest("GET", url, nil))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			if len(body) < 4 || string(body[:4]) != "wOF2" {
				sig := ""
				if len(body) >= 4 {
					sig = string(body[:4])
				}
				t.Errorf("%s is not a woff2 file (signature %q)", url, sig)
			}
		})
	}
}
