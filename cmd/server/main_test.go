package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureUploads covers the startup probe. A deployment whose upload volume
// is missing or read-only must stop at boot, not fail on the first real upload.
func TestEnsureUploads(t *testing.T) {
	t.Run("creates a missing directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "uploads")
		if err := ensureUploads(dir); err != nil {
			t.Fatalf("ensureUploads: %v", err)
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("uploads dir missing after ensure: %v", err)
		}
		// The probe must not leave a file behind.
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("probe left %d entries behind", len(entries))
		}
	})

	t.Run("rejects a path that cannot be a directory", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		if err := ensureUploads(blocker); err == nil {
			t.Fatal("ensureUploads accepted a regular file as the upload root")
		}
	})
}
