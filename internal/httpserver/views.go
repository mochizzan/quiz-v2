package httpserver

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ViewLoader parses a template directory once at construction and renders
// pages by slash-relative path from that directory (e.g.
// "pages/teacher/dashboard.html", "hello.html"). It is the render path for
// every SSR page; Fiber's built-in Views renderer is intentionally not used.
//
// Template contracts (fixed by the frontend plan — every template obeys them):
//
//   - A page file body is exactly {{define "content"}} … {{end}} (plus,
//     rarely, extra defines such as a per-page "partials/title" override).
//   - A layout file is {{define "layouts/<name>.html"}} … {{template "content" .}} … {{end}}.
//   - A partial file is {{define "partials/<name>.html"}} … {{end}} and never
//     defines "content".
//
// Every file under dir/layouts/ and dir/partials/ is parsed into EVERY page
// set, named by its slash-relative path (layouts/<rel>, partials/<rel>), each
// parsed with an explicit name via t.New(name).Parse(...) so nested
// directories never collide on base filenames. Each page gets its OWN
// independent template set: partials and layouts first, the page file parsed
// LAST — so a page's own defines (e.g. its "partials/title" override) win in
// that page's set and can never leak into any other page.
//
// Layout selection is by the page's slash-relative path:
//
//	pages/auth/*       -> layouts/auth.html
//	pages/teacher/*    -> layouts/teacher.html
//	pages/student/*    -> layouts/student.html
//	pages/profile.html -> layouts/student.html
//	pages/attempt/*    -> layouts/base.html
//	pages/error/*      -> layouts/error.html
//	any other path     -> layouts/base.html when the page defines content
//
// A file WITHOUT a "content" define (e.g. the root-level hello.html) is a
// standalone full document and executes directly, without any layout. A page
// whose convention-mapped layout file is absent errors at Render time.
type ViewLoader struct {
	dir      string
	pages    map[string]renderable // slash-relative page path -> execution plan
	parseErr error                 // captured at construction so Render reports it, never panics
}

// renderable is one page's execution plan: the page's own template set plus
// the name of the template to execute inside it — the page's
// convention-mapped layout for a page that defines content, or the page
// itself (standalone full document) when it does not.
type renderable struct {
	set  *template.Template
	name string
}

// templateSource pairs a template's set name (its slash-relative path) with
// the file's text.
type templateSource struct {
	name string
	text string
}

// NewViewLoader walks dir and builds one independent template set per page.
// A missing directory or an unparsable template is not fatal: the error is
// kept and returned by Render, so construction always succeeds and never
// panics.
func NewViewLoader(dir string) *ViewLoader {
	v := &ViewLoader{dir: dir, pages: map[string]renderable{}}
	if err := v.build(); err != nil {
		v.parseErr = err
	}
	return v
}

// build walks dir, classifies every *.html file by its slash-relative path
// (layouts/ and partials/ are shared into every page set; everything else is
// a page), then constructs each page's set with its own file parsed last.
// The first parse error aborts construction and is surfaced by Render.
func (v *ViewLoader) build() error {
	var partials, layouts, pages []templateSource
	err := filepath.WalkDir(v.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".html") {
			return nil
		}
		rel, err := filepath.Rel(v.dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		text, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		src := templateSource{name: rel, text: string(text)}
		switch {
		case strings.HasPrefix(rel, "partials/"):
			partials = append(partials, src)
		case strings.HasPrefix(rel, "layouts/"):
			layouts = append(layouts, src)
		default:
			pages = append(pages, src)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("httpserver: walk templates in %s: %w", v.dir, err)
	}
	sortSources(partials)
	sortSources(layouts)
	sortSources(pages)

	// Shared files are parsed into every page set: partials first, then
	// layouts, each with an explicit template name (the slash-relative path).
	shared := make([]templateSource, 0, len(partials)+len(layouts))
	shared = append(shared, partials...)
	shared = append(shared, layouts...)

	for _, page := range pages {
		set := template.New(page.name)
		for _, src := range shared {
			if _, err := set.New(src.name).Parse(src.text); err != nil {
				return fmt.Errorf("httpserver: parse %s in page set %s: %w", src.name, page.name, err)
			}
		}
		// The page file is parsed LAST so its defines (e.g. a
		// "partials/title" override) replace earlier definitions in this
		// page's set only — sets are independent, so nothing leaks.
		if _, err := set.Parse(page.text); err != nil {
			return fmt.Errorf("httpserver: parse %s: %w", page.name, err)
		}
		// Only pages define "content"; a file without it is a standalone
		// full document and executes directly. A content page executes its
		// convention-mapped layout (ExecuteTemplate errors at Render time
		// when that layout file is absent).
		exec := page.name
		if set.Lookup("content") != nil {
			exec = layoutFor(page.name)
		}
		v.pages[page.name] = renderable{set: set, name: exec}
	}
	return nil
}

// layoutFor maps a page's slash-relative path to the layout template its
// "content" define renders through. Pages outside the path convention fall
// back to layouts/base.html; standalone files (no content define) never
// consult this table.
func layoutFor(page string) string {
	switch {
	case strings.HasPrefix(page, "pages/auth/"):
		return "layouts/auth.html"
	case strings.HasPrefix(page, "pages/teacher/"):
		return "layouts/teacher.html"
	case strings.HasPrefix(page, "pages/student/"):
		return "layouts/student.html"
	case page == "pages/profile.html":
		return "layouts/student.html"
	case strings.HasPrefix(page, "pages/attempt/"):
		return "layouts/base.html"
	case strings.HasPrefix(page, "pages/error/"):
		return "layouts/error.html"
	default:
		return "layouts/base.html"
	}
}

// sortSources orders template sources by slash-relative path so set
// construction (and therefore error reporting) is deterministic.
func sortSources(srcs []templateSource) {
	sort.Slice(srcs, func(i, j int) bool { return srcs[i].name < srcs[j].name })
}

// Render executes the page's template set into w: the convention-mapped
// layout for a page that defines content, or the page itself for a standalone
// file. name is the slash-relative path from the loader directory (e.g.
// "pages/teacher/dashboard.html", "hello.html"). It returns a non-nil error
// when the directory failed to parse (including a directory with no matching
// files), when name is unknown, or when the page's convention-mapped layout
// file is absent.
func (v *ViewLoader) Render(w io.Writer, name string, data any) error {
	if v.parseErr != nil {
		return v.parseErr
	}
	page, ok := v.pages[name]
	if !ok {
		return fmt.Errorf("httpserver: template %q not found in %s", name, v.dir)
	}
	return page.set.ExecuteTemplate(w, page.name, data)
}
