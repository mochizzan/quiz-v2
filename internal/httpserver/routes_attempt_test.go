package httpserver_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// TestAttemptFlowPages pins plan Task 14's three attempt screens. Attempt
// surfaces deliberately carry NO loading state (spec §15.8), so the test
// asserts its absence on the question page alongside the error/locked demos.
func TestAttemptFlowPages(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		title    string
		contains []string
		demos    []string
		absent   []string
	}{
		{
			name:     "question",
			path:     "/attempt",
			title:    "Kuis Pengetahuan Umum — Twisty Quiz",
			contains: []string{`lang="id"`, "Soal 1 dari 5", `role="progressbar"`, `aria-valuenow="20"`, "Pilihan Ganda", "Terjawab: 0/5", "Mars", "Lihat semua", "#nav-drawer", "btn-solid", `aria-live="polite"`},
			demos:    []string{"error", "locked"},
			absent:   []string{"btn-primary", "data-state-demo=\"loading\"", "d-lg-block", "https://"},
		},
		{
			name:     "review",
			path:     "/attempt/review",
			title:    "Tinjau jawaban — Twisty Quiz",
			contains: []string{`lang="id"`, "Tinjau jawaban Anda", "belum dijawab", "Kumpulkan jawaban", "Kembali ke soal", `data-bs-toggle="dialog"`, "btn-solid"},
			absent:   []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:     "result",
			path:     "/attempt/result",
			title:    "Hasil kuis — Twisty Quiz",
			contains: []string{`lang="id"`, "Jawaban berhasil dikumpulkan", "09.42 WIB", "Terima kasih, Ayu", "Kembali ke dasbor", "#i-check", "btn-solid"},
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
					t.Errorf("page must NOT contain %q", bad)
				}
			}
		})
	}
}
