package httpserver_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quiz-v2/internal/httpserver"
)

func TestRenderMissingTemplateReturnsError(t *testing.T) {
	v := httpserver.NewViewLoader(t.TempDir()) // empty dir, no templates
	err := v.Render(io.Discard, "hello.html", nil)
	if err == nil {
		t.Fatal("expected error for missing template, got nil")
	}
}

// writeTemplateFixture writes one fixture file under root (a t.TempDir()),
// creating any parent directories.
func writeTemplateFixture(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPageRendersThroughLayoutAndPartials(t *testing.T) {
	dir := t.TempDir()
	writeTemplateFixture(t, dir, "layouts/base.html",
		`{{define "layouts/base.html"}}HEAD {{template "content" .}} FOOT{{end}}`)
	writeTemplateFixture(t, dir, "partials/p.html",
		`{{define "partials/p.html"}}P-TOKEN{{end}}`)
	writeTemplateFixture(t, dir, "pages/x/y.html",
		`{{define "content"}}BODY {{template "partials/p.html" .}}{{end}}`)

	v := httpserver.NewViewLoader(dir)
	var buf strings.Builder
	if err := v.Render(&buf, "pages/x/y.html", nil); err != nil {
		t.Fatalf("Render(pages/x/y.html): %v", err)
	}
	out := buf.String()
	prev := -1
	for _, tok := range []string{"HEAD", "BODY", "P-TOKEN", "FOOT"} {
		idx := strings.Index(out, tok)
		if idx < 0 {
			t.Fatalf("output %q missing token %q", out, tok)
		}
		if idx < prev {
			t.Fatalf("output %q has token %q out of order", out, tok)
		}
		prev = idx
	}
}

func TestStandaloneFileRendersDirectly(t *testing.T) {
	dir := t.TempDir()
	const solo = "<!doctype html>\n<html lang=\"id\">\n<head><title>Solo</title></head>\n<body>\n  <h1>Solo</h1>\n</body>\n</html>\n"
	writeTemplateFixture(t, dir, "solo.html", solo)

	v := httpserver.NewViewLoader(dir)
	var buf strings.Builder
	if err := v.Render(&buf, "solo.html", nil); err != nil {
		t.Fatalf("Render(solo.html): %v", err)
	}
	if buf.String() != solo {
		t.Errorf("got %q, want the standalone file rendered as-is", buf.String())
	}
}

func TestMissingLayoutReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeTemplateFixture(t, dir, "pages/auth/orphan.html", `{{define "content"}}BODY{{end}}`)
	// No layouts/ directory: the convention-mapped layouts/auth.html is absent.

	v := httpserver.NewViewLoader(dir)
	if err := v.Render(io.Discard, "pages/auth/orphan.html", nil); err == nil {
		t.Fatal("expected error for a page whose convention-mapped layout is absent, got nil")
	}
}

func TestPageCanOverrideTitleDefine(t *testing.T) {
	dir := t.TempDir()
	writeTemplateFixture(t, dir, "partials/t.html",
		`{{define "partials/title"}}Judul Default{{end}}`)
	writeTemplateFixture(t, dir, "layouts/base.html",
		`{{define "layouts/base.html"}}<title>{{template "partials/title" .}}</title>{{template "content" .}}{{end}}`)
	writeTemplateFixture(t, dir, "pages/a.html",
		`{{define "partials/title"}}Judul Halaman{{end}}{{define "content"}}BODY-A{{end}}`)
	writeTemplateFixture(t, dir, "pages/b.html",
		`{{define "content"}}BODY-B{{end}}`)

	v := httpserver.NewViewLoader(dir)

	var a strings.Builder
	if err := v.Render(&a, "pages/a.html", nil); err != nil {
		t.Fatalf("Render(pages/a.html): %v", err)
	}
	if !strings.Contains(a.String(), "<title>Judul Halaman</title>") {
		t.Errorf("page a: got %q, want its own title override", a.String())
	}

	var b strings.Builder
	if err := v.Render(&b, "pages/b.html", nil); err != nil {
		t.Fatalf("Render(pages/b.html): %v", err)
	}
	if !strings.Contains(b.String(), "<title>Judul Default</title>") {
		t.Errorf("page b: got %q, want the default partial title", b.String())
	}
	if strings.Contains(b.String(), "Judul Halaman") {
		t.Errorf("page a's title define leaked into page b: %q", b.String())
	}
}
