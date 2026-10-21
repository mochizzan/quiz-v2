package httpserver_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// TestTeacherGradingPages pins plan Task 11's two screens: the essay grading
// queue (which owns the full grading-family state demos) and the attempt
// recap, each with pinned copy and none of the v5/CDN spellings.
func TestTeacherGradingPages(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		title    string
		contains []string
		demos    []string
		absent   []string
	}{
		{
			name:     "grading",
			path:     "/teacher/grading",
			title:    "Penilaian esai — Twisty Quiz",
			contains: []string{`lang="id"`, "Penilaian esai", "Ayu Lestari", "Menunggu penilaian", "Simpan penilaian", "btn-solid", "theme-warning"},
			demos:    []string{"saving", "error", "empty", "loading", "locked", "saved"},
			absent:   []string{"btn-primary", "d-lg-block", "form-select", "https://"},
		},
		{
			name:     "recap",
			path:     "/teacher/recap",
			title:    "Rekap percobaan — Twisty Quiz",
			contains: []string{`lang="id"`, "Rekap percobaan", "Ayu Lestari", "09.15 WIB", "Fokus hilang", "Kembali ke penilaian", "btn-solid", `font-variant-numeric: tabular-nums`},
			demos:    []string{"error", "locked"},
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
