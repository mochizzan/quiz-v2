package main

import (
	"strings"
	"testing"

	"quiz-v2/internal/secure"
)

// TestBootstrapHash pins the seed contract: bootstrap-teacher is mandatory, so
// a deployment missing its handoff secret must fail before it opens a pool, and
// the plaintext path must produce a hash the login path can verify.
func TestBootstrapHash(t *testing.T) {
	t.Run("plaintext is hashed and verifies", func(t *testing.T) {
		hash, err := bootstrapHash("Guru1234", "")
		if err != nil {
			t.Fatalf("bootstrapHash: %v", err)
		}
		if !strings.HasPrefix(hash, "$2") || len(hash) != 60 {
			t.Fatalf("hash = %q, want a bcrypt hash", hash)
		}
		if !secure.VerifyPassword(hash, "Guru1234") {
			t.Error("the hash does not verify against its password")
		}
	})

	t.Run("pre-hashed is passed through", func(t *testing.T) {
		hash, err := secure.HashPassword("Guru1234")
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		got, err := bootstrapHash("", hash)
		if err != nil {
			t.Fatalf("bootstrapHash: %v", err)
		}
		if got != hash {
			t.Errorf("hash = %q, want the supplied %q", got, hash)
		}
	})

	t.Run("missing both is fatal", func(t *testing.T) {
		if _, err := bootstrapHash("", ""); err == nil {
			t.Fatal("bootstrapHash accepted a run with no password at all")
		}
	})

	t.Run("both is ambiguous and fatal", func(t *testing.T) {
		hash, err := secure.HashPassword("Guru1234")
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		if _, err := bootstrapHash("Guru1234", hash); err == nil {
			t.Fatal("bootstrapHash accepted both forms")
		}
	})

	t.Run("plaintext must satisfy the password policy", func(t *testing.T) {
		if _, err := bootstrapHash("short", ""); err == nil {
			t.Fatal("bootstrapHash accepted a password the login form would reject")
		}
	})
}
