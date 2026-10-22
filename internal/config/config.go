package config

import (
	"os"
	"strings"
)

// Config holds runtime configuration for the application.
type Config struct {
	Addr              string // APP_ADDR, default ":8080"
	DBDSN             string // DB_DSN, default "quiz:quiz@tcp(db:3306)/quiz?parseTime=true"
	CookieSecure      bool   // COOKIE_SECURE, default false (spec §12)
	SessionSigningKey string // SESSION_SIGNING_KEY, default "" (empty until auth task)
	Demo              bool   // APP_DEMO=1 enables ?state= previews and the demo strip
}

// Load reads configuration from environment variables, falling back to
// defaults when a variable is unset or empty.
func Load() Config {
	return Config{
		Addr:              envOr("APP_ADDR", ":8080"),
		DBDSN:             envOr("DB_DSN", "quiz:quiz@tcp(db:3306)/quiz?parseTime=true"),
		CookieSecure:      strings.EqualFold(os.Getenv("COOKIE_SECURE"), "true"),
		SessionSigningKey: os.Getenv("SESSION_SIGNING_KEY"),
		Demo:              strings.EqualFold(os.Getenv("APP_DEMO"), "1") || strings.EqualFold(os.Getenv("APP_DEMO"), "true"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
