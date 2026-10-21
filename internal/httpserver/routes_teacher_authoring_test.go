package httpserver_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// TestTeacherAuthoringPages pins plan Task 10's four authoring screens: each
// carries breadcrumb + stepper chrome where pinned, its own state demos, and
// none of the v5/CDN spellings.
func TestTeacherAuthoringPages(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		title    string
		contains []string
		demos    []string
		absent   []string
	}{
		{
			name:     "quiz-edit",
			path:     "/teacher/quizzes/edit",
			title:    "Edit kuis — Twisty Quiz",
			contains: []string{`lang="id"`, `class="breadcrumb"`, "Biologi Sel — Kuis 4", "Tambah soal", "Terbitkan kuis", "btn-solid", `data-bs-toggle="dialog"`},
			demos:    []string{"loading", "empty", "error", "saving", "saved", "locked"},
			absent:   []string{"btn-primary", "d-lg-block", "form-select", "https://"},
		},
		{
			name:     "question-editor",
			path:     "/teacher/questions",
			title:    "Editor soal — Twisty Quiz",
			contains: []string{`lang="id"`, "Pilihan Ganda", "Jadikan kunci", "Simpan soal", "Kembali ke kuis", "btn-solid"},
			absent:   []string{"btn-primary", "form-check-input", "d-lg-block", "https://"},
		},
		{
			name:     "image-upload",
			path:     "/teacher/questions/image",
			title:    "Unggah gambar — Twisty Quiz",
			contains: []string{`lang="id"`, "Seret gambar ke sini atau pilih berkas", "Maksimal 10 MB", "btn-solid", "#i-upload"},
			demos:    []string{"saving"},
			absent:   []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:     "quiz-settings",
			path:     "/teacher/quizzes/settings",
			title:    "Pengaturan kuis — Twisty Quiz",
			contains: []string{`lang="id"`, "Acak soal", "Simpan pengaturan", "Tidak tersedia untuk v1", "btn-solid"},
			demos:    []string{"saved", "locked"},
			absent:   []string{"btn-primary", "form-check-input", "d-lg-block", "https://"},
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
