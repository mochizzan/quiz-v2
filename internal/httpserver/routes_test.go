package httpserver

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
)

// TestRootRedirectsToLogin pins the home route: GET / answers 303 See Other
// pointing at /login (the login page itself lands in Task 7).
func TestRootRedirectsToLogin(t *testing.T) {
	app := New(config.Config{Addr: ":8080"})
	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 303 {
		t.Fatalf("status = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

// TestRegisteredRouteRendersTemplate pins registerPage + the registry loop in
// NewWithStore: a route registered at init/test time is served by the app with
// its template through the view loader.
func TestRegisteredRouteRendersTemplate(t *testing.T) {
	registerPage(PageRoute{Method: "GET", Path: "/__test", Template: "hello.html", Nav: "x"})

	app := New(config.Config{Addr: ":8080"})
	resp, err := app.Test(httptest.NewRequest("GET", "/__test", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Halo, Dunia!") {
		t.Errorf("body must contain the hello copy, got %q", string(body))
	}
}

// TestUnknownPathReturns404 pins the catch-all: an unregistered path answers
// 404. The body is deliberately NOT asserted — Task 6 replaces the plain-text
// fallback with the styled pages/error/404.html.
func TestUnknownPathReturns404(t *testing.T) {
	app := New(config.Config{Addr: ":8080"})
	resp, err := app.Test(httptest.NewRequest("GET", "/definitely-missing", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestRegisteredPagesIsCopy pins the defensive copy: mutating the returned
// slice must not corrupt the package registry seen by later calls.
func TestRegisteredPagesIsCopy(t *testing.T) {
	first := RegisteredPages()
	if len(first) == 0 {
		t.Fatal("RegisteredPages() is empty; routes_home.go must register at least /hello")
	}
	original := first[0]
	originalLen := len(first)

	first[0] = PageRoute{Method: "POST", Path: "/__mutated", Template: "nope.html"}

	second := RegisteredPages()
	if len(second) != originalLen {
		t.Fatalf("len = %d after mutating copy, want %d", len(second), originalLen)
	}
	if second[0] != original {
		t.Errorf("registry mutated through the copy: got %+v, want %+v", second[0], original)
	}
}
