package httpserver_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// TestTeacherCorePages pins plan Task 9's three teacher screens: dashboard,
// quiz list and live monitor, each on the teacher layout with its nav key,
// pinned copy, required ARIA and the state demos this family owns.
func TestTeacherCorePages(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		title    string
		contains []string
		demos    []string
		absent   []string
	}{
		{
			name:     "dashboard",
			path:     "/teacher",
			title:    "Dasbor guru — Twisty Quiz",
			contains: []string{`lang="id"`, "Selamat datang kembali, Bu Ratna", "xl:row-cols-4", `role="progressbar"`, `aria-live="polite"`, "btn-solid", "Biologi Sel — Kuis 4", "1.480", "aria-current=\"page\""},
			absent:   []string{"btn-primary", "col-xl-7", "d-lg-block", "https://"},
		},
		{
			name:     "quizzes",
			path:     "/teacher/quizzes",
			title:    "Daftar kuis — Twisty Quiz",
			contains: []string{`lang="id"`, "Daftar kuis", "Biologi Sel — Kuis 4", "Kimia Organik — Kuis 2", "Fisika Kinematika — Kuis 3", "Sejarah Indonesia — Kuis 1", "btn-solid"},
			demos:    []string{"empty"},
			absent:   []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:     "monitor",
			path:     "/teacher/monitor",
			title:    "Monitor kuis — Twisty Quiz",
			contains: []string{`lang="id"`, "Monitor langsung", "Ayu Lestari", "Bayu Prakoso", "Cindy Halim", "Dimas Ardiansyah", "Eka Nuraini", "Fokus hilang", `aria-busy`},
			demos:    []string{"loading", "stale", "locked"},
			absent:   []string{"btn-primary", "d-lg-block", "https://"},
		},
	}

	app := httpserver.New(config.Config{Addr: ":8080"})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("GET", tc.path, nil))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			html := string(body)

			if !strings.Contains(html, tc.title) {
				t.Errorf("page must carry title %q", tc.title)
			}
			for _, want := range tc.contains {
				if !strings.Contains(html, want) {
					t.Errorf("page missing %q", want)
				}
			}
			for _, demo := range tc.demos {
				if !strings.Contains(html, `data-state-demo="`+demo+`"`) {
					t.Errorf("page must ship its %q state demo (spec §15.8)", demo)
				}
			}
			for _, bad := range tc.absent {
				if strings.Contains(html, bad) {
					t.Errorf("page must NOT contain %q (v5/CDN leak)", bad)
				}
			}
		})
	}
}
