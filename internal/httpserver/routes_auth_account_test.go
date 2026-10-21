package httpserver_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// TestAuthAccountPages pins plan Task 8's three account screens: each renders
// 200 through the auth layout with its own title, its pinned copy, at least
// one v6-only class, and none of the v5/CDN spellings the prototypes carry.
func TestAuthAccountPages(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		title    string
		contains []string
		absent   []string
	}{
		{
			name:     "register",
			path:     "/register",
			title:    "Daftar akun — Twisty Quiz",
			contains: []string{`lang="id"`, "Daftar akun", "btn-solid", "theme-primary", "Akun guru perlu persetujuan admin sekolah.", "Sudah punya akun? Masuk", `aria-label="Tampilkan kata sandi"`},
			absent:   []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:     "pending",
			path:     "/auth/pending",
			title:    "Menunggu persetujuan — Twisty Quiz",
			contains: []string{`lang="id"`, "Menunggu persetujuan admin", "btn-solid", "SMA Nusantara", "#i-clock"},
			absent:   []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:     "change-password",
			path:     "/change-password",
			title:    "Ubah kata sandi — Twisty Quiz",
			contains: []string{`lang="id"`, "Ubah kata sandi", "Simpan kata sandi", "btn-solid", "Minimal 8 karakter", "Kata sandi berhasil diubah."},
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
			for _, bad := range tc.absent {
				if strings.Contains(html, bad) {
					t.Errorf("page must NOT contain %q (v5/CDN leak)", bad)
				}
			}
		})
	}
}
