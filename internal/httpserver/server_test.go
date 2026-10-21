package httpserver_test

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// TestMain runs tests from the repository root so that relative paths such as
// ./templates resolve to the real templates directory rather than the package
// directory (go test sets the working directory to the package dir).
func TestMain(m *testing.M) {
	orig, err := os.Getwd() // package dir: <repo>/internal/httpserver
	if err != nil {
		panic(err)
	}
	root := filepath.Join(orig, "..", "..")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		panic("httpserver tests: go.mod not found at repo root " + root + ": " + err.Error())
	}
	if err := os.Chdir(root); err != nil {
		panic(err)
	}
	// os.Exit skips deferred calls, so restore explicitly after the run.
	code := m.Run()
	if err := os.Chdir(orig); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestHelloRoute(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080"})
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `lang="id"`) {
		t.Error("page must declare lang=id (spec §15.10)")
	}
	if !strings.Contains(string(body), "Halo, Dunia!") {
		t.Error("page must render the hello copy")
	}
}

func TestHealthzRoute(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080"})
	req := httptest.NewRequest("GET", "/healthz", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

type deadPinger struct{}

func (deadPinger) Ping(context.Context) error { return errors.New("db down") }

func TestHealthzReturns503WhenDBDown(t *testing.T) {
	app := httpserver.NewWithStore(config.Config{Addr: ":8080"}, deadPinger{})
	resp, err := app.Test(httptest.NewRequest("GET", "/healthz", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 503 {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}
