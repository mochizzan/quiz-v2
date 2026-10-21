package httpserver_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// TestStudentAreaPages pins plan Task 13's three screens: the student
// dashboard (which owns the student-family loading/empty/error demos), the
// quiz history table and the profile card.
func TestStudentAreaPages(t *testing.T) {
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
			path:     "/student",
			title:    "Dasbor siswa — Twisty Quiz",
			contains: []string{`lang="id"`, "Halo, Ayu", "XII IPA 2", "Kuis Pengetahuan Umum — Sesi 1", "Terbuka", "Mulai kuis", "Fisika Kinematika — Kuis 3", "Besok, 09.00 WIB", "Terjadwal", "btn-solid", "aria-current=\"page\""},
			demos:    []string{"loading", "empty", "error"},
			absent:   []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:     "history",
			path:     "/student/history",
			title:    "Riwayat kuis — Twisty Quiz",
			contains: []string{`lang="id"`, "Riwayat kuis", "20 Okt 2026", "Selesai", `class="table-responsive"`, "btn-solid", `font-variant-numeric: tabular-nums`},
			absent:   []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:  "profile",
			path:  "/profile",
			title: "Profil — Twisty Quiz",
			// Plan pins both actions as .btn-outline (Ubah kata sandi, Keluar)
			// and an edit form with disabled fields — no primary action, so
			// btn-solid is deliberately NOT asserted here.
			contains: []string{`lang="id"`, "Ayu Lestari", "Ubah data lewat admin sekolah.", "Ubah kata sandi", "Keluar", "#i-signout", "aria-current=\"page\""},
			absent:   []string{"btn-primary", "btn-solid", "d-lg-block", "https://"},
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
