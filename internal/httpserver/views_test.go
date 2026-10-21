package httpserver_test

import (
	"io"
	"testing"

	"quiz-v2/internal/httpserver"
)

func TestRenderMissingTemplateReturnsError(t *testing.T) {
	v := httpserver.NewViewLoader(t.TempDir()) // empty dir, no templates
	err := v.Render(io.Discard, "hello.html", nil)
	if err == nil { t.Fatal("expected error for missing template, got nil") }
}
