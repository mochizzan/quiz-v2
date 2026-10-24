package config

import (
	"errors"
	"os"
	"strings"

	"quiz-v2/internal/secure"
)

// Config holds runtime configuration for the application.
//
// Field names and defaults are pinned by business-logic spec OPS-232. A value
// that is required in production and absent there is a startup failure, not a
// fallback (see Validate).
type Config struct {
	Addr              string // APP_ADDR, default ":8080"
	DBDSN             string // DB_DSN, default points at the compose service
	CookieSecure      bool   // COOKIE_SECURE, default false (school LAN)
	SessionSigningKey string // SESSION_SIGNING_KEY, required
	UploadsDir        string // UPLOADS_DIR, default "./uploads"
}

// DefaultDSN carries the settings the rules depend on (OPS-233): parseTime so
// DATETIME scans into time.Time, loc=UTC so the driver does not shift it, and
// no multiStatements so one injection cannot become a script.
const DefaultDSN = "quiz:quiz@tcp(db:3306)/quiz?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&loc=UTC&timeout=5s&readTimeout=10s&writeTimeout=10s"

// Load reads configuration from environment variables, falling back to
// defaults when a variable is unset or empty.
func Load() Config {
	return Config{
		Addr:              envOr("APP_ADDR", ":8080"),
		DBDSN:             envOr("DB_DSN", DefaultDSN),
		CookieSecure:      strings.EqualFold(os.Getenv("COOKIE_SECURE"), "true"),
		SessionSigningKey: os.Getenv("SESSION_SIGNING_KEY"),
		UploadsDir:        envOr("UPLOADS_DIR", "./uploads"),
	}
}

// ErrSigningKey is returned when the process would run without a usable key.
var ErrSigningKey = errors.New("config: SESSION_SIGNING_KEY must be at least 32 bytes")

// devSigningKey is the value a compose file once shipped as a default. It is a
// valid length and therefore passes the minimum, but it is public knowledge, so
// accepting it would let anyone forge a session or CSRF token. Compare against
// it by value, not by length.
const devSigningKey = "dev-only-signing-key-change-me-32b"

// ErrDevSigningKey is returned when the well-known development key is supplied.
var ErrDevSigningKey = errors.New("config: SESSION_SIGNING_KEY is the published development default, generate a real secret")

// Validate rejects a configuration that would start a process unable to honour
// its security rules (IDN-050, OPS-232).
func Validate(cfg Config) error {
	if len(cfg.SessionSigningKey) < secure.MinKeyBytes {
		return ErrSigningKey
	}
	if cfg.SessionSigningKey == devSigningKey {
		return ErrDevSigningKey
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
