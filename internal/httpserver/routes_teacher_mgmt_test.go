package httpserver_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// TestTeacherMgmtPages pins plan Task 12's four management screens: students
// (which owns the teacher-list loading/empty/error demos), anonymous
// participants, majors and teacher approval. Every table must sit inside a
// .table-responsive wrapper (spec §15.4).
func TestTeacherMgmtPages(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		title    string
		contains []string
		demos    []string
		absent   []string
	}{
		{
			name:     "students",
			path:     "/teacher/students",
			title:    "Kelola siswa — Twisty Quiz",
			contains: []string{`lang="id"`, "Kelola siswa", "Ayu Lestari", "Bayu Prakoso", "Cindy Halim", "Dimas Ardiansyah", "Eka Nuraini", "Tambah siswa", "btn-solid", `class="table-responsive"`},
			demos:    []string{"loading", "empty", "error"},
			absent:   []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:     "anonymous",
			path:     "/teacher/anonymous",
			title:    "Peserta anonim — Twisty Quiz",
			// Plan pin for this screen is "word badges only" — it carries no
			// primary action, so btn-solid is deliberately NOT asserted here.
			contains: []string{`lang="id"`, "Peserta anonim", "cookie peserta", "09.15 WIB", `class="table-responsive"`},
			absent:   []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:     "majors",
			path:     "/teacher/majors",
			title:    "Jurusan — Twisty Quiz",
			contains: []string{`lang="id"`, "Jurusan", "IPA", "IPS", "Bahasa", "Kejuruan", "Tambah jurusan", "btn-solid", `class="list-group`},
			absent:   []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:     "teachers",
			path:     "/teacher/teachers",
			title:    "Kelola guru — Twisty Quiz",
			contains: []string{`lang="id"`, "Kelola guru", "Menunggu persetujuan", "Guru aktif", "Ratna Wijaya", "Setujui", "Tolak", "btn-solid", `data-bs-toggle="dialog"`},
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
