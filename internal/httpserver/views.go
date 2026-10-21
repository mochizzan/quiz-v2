package httpserver

import (
	"html/template"
	"io"
	"path/filepath"
)

// ViewLoader parses a directory of *.html templates once at construction and
// renders them by base filename (e.g. "hello.html"). It is the render path for
// every SSR page; Fiber's built-in Views renderer is intentionally not used.
type ViewLoader struct {
	dir      string
	tmpl     *template.Template
	parseErr error // captured at construction so Render reports it, never panics
}

// NewViewLoader parses every *.html file in dir. A missing, empty or
// unparsable directory is not fatal: the error is kept and returned by Render,
// so construction always succeeds.
func NewViewLoader(dir string) *ViewLoader {
	v := &ViewLoader{dir: dir}
	v.tmpl, v.parseErr = template.ParseGlob(filepath.Join(dir, "*.html"))
	return v
}

// Render executes the template whose base filename is name into w. It returns
// a non-nil error when the directory failed to parse (including a directory
// with no matching files) or when name is not a defined template.
func (v *ViewLoader) Render(w io.Writer, name string, data any) error {
	if v.parseErr != nil {
		return v.parseErr
	}
	return v.tmpl.ExecuteTemplate(w, name, data)
}
