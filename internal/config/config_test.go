package config_test

import (
	"errors"
	"strings"
	"testing"

	"quiz-v2/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("COOKIE_SECURE", "")
	t.Setenv("APP_ADDR", "")
	t.Setenv("DB_DSN", "")
	t.Setenv("UPLOADS_DIR", "")
	cfg := config.Load()
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if cfg.CookieSecure != false {
		t.Error("CookieSecure must default to false (spec §12)")
	}
	if cfg.DBDSN == "" {
		t.Error("DBDSN must fall back to its default")
	}
	if cfg.UploadsDir != "./uploads" {
		t.Errorf("UploadsDir = %q, want ./uploads", cfg.UploadsDir)
	}
}

func TestLoadOverridesFromEnv(t *testing.T) {
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("SESSION_SIGNING_KEY", "test-key")
	t.Setenv("UPLOADS_DIR", "/srv/uploads")
	cfg := config.Load()
	if cfg.Addr != ":9090" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":9090")
	}
	if cfg.CookieSecure != true {
		t.Error("CookieSecure should be true when COOKIE_SECURE=true")
	}
	if cfg.SessionSigningKey != "test-key" {
		t.Errorf("SessionSigningKey = %q", cfg.SessionSigningKey)
	}
	if cfg.UploadsDir != "/srv/uploads" {
		t.Errorf("UploadsDir = %q, want /srv/uploads", cfg.UploadsDir)
	}
}

// TestValidateRejectsUnusableKeys pins IDN-050 and OPS-232: a process that
// cannot sign a session or CSRF token must not start, and the published
// development default is a known value, not a secret.
func TestValidateRejectsUnusableKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want error
	}{
		{name: "empty", key: "", want: config.ErrSigningKey},
		{name: "too short", key: "test-key", want: config.ErrSigningKey},
		{
			name: "published development default",
			key:  "dev-only-signing-key-change-me-32b",
			want: config.ErrDevSigningKey,
		},
		{name: "32 random bytes", key: strings.Repeat("k", 32), want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := config.Validate(config.Config{SessionSigningKey: tc.key})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Validate(%q) = %v, want %v", tc.key, err, tc.want)
			}
		})
	}
}
