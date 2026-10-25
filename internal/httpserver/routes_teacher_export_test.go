package httpserver

// routes_teacher_export_test.go pins the transport glue of the quiz XLSX
// export: the route registration and class, the download filename helper, and
// the defensive nil-session branch. The workbook itself is covered by the
// attempts and export package tests.

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/config"
)

func TestExportSlug(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  string
	}{
		{"plain", "Ulangan Harian", "ulangan-harian"},
		{"mixed case", "UTS Ganjil 2026", "uts-ganjil-2026"},
		{"punctuation run", "  Kuis — “PAS” / 2026!! ", "kuis-pas-2026"},
		{"underscore and dots", "Bab_3.Final", "bab-3-final"},
		{"digits only", "12345", "12345"},
		{"empty fallback", "", "kuis"},
		{"symbols fallback", "— … !!!", "kuis"},
		{"non-ascii fallback", "日本語", "kuis"},
		{"ascii and non-ascii", "Kuis 日本 2", "kuis-2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := exportSlug(tc.title)
			if got != tc.want {
				t.Fatalf("exportSlug(%q) = %q, want %q", tc.title, got, tc.want)
			}
			if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
				t.Errorf("exportSlug(%q) = %q, want no leading or trailing hyphen", tc.title, got)
			}
			if strings.Contains(got, "--") {
				t.Errorf("exportSlug(%q) = %q, want runs collapsed to one hyphen", tc.title, got)
			}
			for i := 0; i < len(got); i++ {
				c := got[i]
				if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
					t.Errorf("exportSlug(%q) = %q contains non-ASCII byte %q", tc.title, got, c)
					break
				}
			}
		})
	}
}

func TestExportFilename(t *testing.T) {
	cases := []struct {
		name  string
		id    int64
		title string
		want  string
	}{
		{"title slug", 12, "Ulangan Harian Bab 3", "kuis-12-ulangan-harian-bab-3.xlsx"},
		{"empty title fallback", 7, "", "kuis-7-kuis.xlsx"},
		{"header-unsafe title", 5, "Halo\";\r\nHapus", "kuis-5-halo-hapus.xlsx"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := exportFilename(tc.id, tc.title)
			if got != tc.want {
				t.Fatalf("exportFilename(%d, %q) = %q, want %q", tc.id, tc.title, got, tc.want)
			}
			if !strings.HasPrefix(got, "kuis-"+strconv.FormatInt(tc.id, 10)+"-") {
				t.Errorf("filename %q does not include the quiz id", got)
			}
			if strings.ContainsAny(got, "\"\r\n") {
				t.Errorf("filename %q carries a quote or newline", got)
			}
			for i := 0; i < len(got); i++ {
				if got[i] > 0x7f {
					t.Errorf("filename %q is not ASCII-only", got)
					break
				}
			}
		})
	}
}

func TestTeacherExportRoute(t *testing.T) {
	const path = "/teacher/quizzes/:id/export.xlsx"

	var found PageRoute
	ok := false
	for _, p := range RegisteredPages() {
		if p.Method == "GET" && p.Path == path {
			found, ok = p, true
		}
	}
	if !ok {
		t.Fatalf("route GET %s is not registered", path)
	}
	if found.Auth != ClassOwnerTeacher {
		t.Errorf("route Auth = %q, want %q", found.Auth, ClassOwnerTeacher)
	}
	if found.Template != "" {
		t.Errorf("route Template = %q, want none (the response is a file)", found.Template)
	}
	if found.HandlerCtx == nil {
		t.Error("route HandlerCtx is nil, want teacherExport")
	}
	if found.Load != nil || found.Act != nil || found.ActForm != nil || found.Handler != nil {
		t.Error("route must run only through HandlerCtx")
	}
	if class, ok := RouteClass("GET", path); !ok || class != ClassOwnerTeacher {
		t.Errorf("RouteClass = %q, %v; want %q, true", class, ok, ClassOwnerTeacher)
	}
}

// TestTeacherExportMissingSessionIs404 covers the handler's defensive branch:
// the route class admits only a teacher, but a nil session is still answered as
// the same 404 and never reaches the read model.
func TestTeacherExportMissingSessionIs404(t *testing.T) {
	srv, err := newServer(config.Config{SessionSigningKey: teacherRouteTestKey}, nil, nil)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	app := fiber.New()
	app.Get("/export", func(c fiber.Ctx) error {
		return teacherExport(&Ctx{server: srv, c: c})
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/export", nil))
	if err != nil {
		t.Fatalf("GET /export: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Disposition"); got != "" {
		t.Errorf("Content-Disposition = %q, want none on a 404", got)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q, want the HTML error page", got)
	}
}
