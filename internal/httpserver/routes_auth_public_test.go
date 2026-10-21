package httpserver_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// authPublicCase pins one auth-public page of plan Task 7: the route answers
// 200 on the auth layout, carries the shared chrome, its own title, at least
// one v6-only class, and its required data-state-demo value — and never the
// v5 spellings the prototypes would leak.
type authPublicCase struct {
	name      string
	path      string
	title     string
	contains  []string
	stateDemo string
	absent    []string
}

func TestAuthPublicPages(t *testing.T) {
	cases := []authPublicCase{
		{
			name:      "login",
			path:      "/login",
			title:     "Masuk — Twisty Quiz",
			contains:  []string{`lang="id"`, "Selamat datang kembali", "btn-solid", "theme-primary", `role="group"`, "Ingat saya", "Lupa kata sandi?", "/join", "/register"},
			stateDemo: "error",
			absent:    []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:      "join",
			path:      "/join",
			title:     "Gabung dengan kode kuis — Twisty Quiz",
			contains:  []string{`lang="id"`, "Gabung dengan kode kuis", "btn-solid", "theme-primary", "QUIZ-7F2K", "Minta kode kepada guru Anda."},
			stateDemo: "saving",
			absent:    []string{"btn-primary", "d-lg-block", "https://"},
		},
		{
			name:      "identify",
			path:      "/identify",
			title:     "Identitas peserta — Twisty Quiz",
			contains:  []string{`lang="id"`, "Mulai kuis", "btn-solid", "theme-primary"},
			stateDemo: "locked",
			absent:    []string{"btn-primary", "d-lg-block", "https://"},
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
				t.Errorf("page must carry title %q (per-page title override)", tc.title)
			}
			for _, want := range tc.contains {
				if !strings.Contains(html, want) {
					t.Errorf("page missing %q", want)
				}
			}
			if tc.stateDemo != "" {
				if !strings.Contains(html, `data-state-demo="`+tc.stateDemo+`"`) {
					t.Errorf("page must ship its %q state demo (spec §15.8)", tc.stateDemo)
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
