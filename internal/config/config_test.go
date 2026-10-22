package config_test

import (
	"testing"

	"quiz-v2/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("COOKIE_SECURE", "")
	t.Setenv("APP_ADDR", "")
	t.Setenv("DB_DSN", "")
	t.Setenv("APP_DEMO", "")
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
	if cfg.Demo {
		t.Error("Demo must default to false; production must not reveal previews")
	}
}

func TestLoadOverridesFromEnv(t *testing.T) {
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("SESSION_SIGNING_KEY", "test-key")
	t.Setenv("APP_DEMO", "1")
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
	if !cfg.Demo {
		t.Error("Demo should be true when APP_DEMO=1")
	}
}

func TestLoadDemoAcceptsTrue(t *testing.T) {
	t.Setenv("APP_DEMO", "true")
	if !config.Load().Demo {
		t.Error("Demo should be true when APP_DEMO=true")
	}
}
