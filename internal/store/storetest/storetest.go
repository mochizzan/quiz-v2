// Package storetest provides the integration-test harness for packages that
// need a real MariaDB. It is imported only from _test.go files.
//
// An integration test is skipped, not failed, when TEST_DB_DSN is unset. That
// keeps `go test ./...` green on a machine with no database while still running
// the full suite in CI and locally with:
//
//	TEST_DB_DSN='quiz:quiz@tcp(127.0.0.1:3306)/quiz?parseTime=true&loc=UTC' go test ./...
package storetest

import (
	"context"
	"os"
	"testing"
	"time"

	"quiz-v2/internal/migrate"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/testlock"
)

// tables is every domain table, ordered so a delete never violates a foreign
// key: children before parents. schema_migrations is deliberately excluded.
var tables = []string{
	"attempt_answer_selections",
	"focus_loss_events",
	"attempt_answers",
	"quiz_attempts",
	"registrations",
	"anonymous_participants",
	"answer_options",
	"questions",
	"quizzes",
	"password_reset_tokens",
	"sessions",
	"students",
	"majors",
	"accounts",
}

// DB opens the test database and skips the test when TEST_DB_DSN is unset. The
// pool is closed automatically at the end of the test.
func DB(t *testing.T) *store.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("integration test: set TEST_DB_DSN to run")
	}
	// One schema is shared by every database-using test binary, so hold the
	// cross-process lock while this process is testing.
	lockCtx, lockCancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer lockCancel()
	if err := testlock.Acquire(lockCtx); err != nil {
		t.Fatalf("storetest: %v", err)
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("storetest: open: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := store.Ping(ctx, db); err != nil {
		t.Fatalf("storetest: ping %s: %v", dsn, err)
	}
	// Integration tests assume the schema exists. A fresh test database does
	// not, so bring it current first: the runner is idempotent and takes the
	// advisory lock, so this is cheap and safe to repeat.
	schemaCtx, schemaCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer schemaCancel()
	if _, err := migrate.Apply(schemaCtx, db); err != nil {
		t.Fatalf("storetest: apply migrations: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Reset deletes every domain row in foreign-key-safe order. Each test that
// writes starts with Reset so the suite is order-independent.
func Reset(t *testing.T, db *store.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, table := range tables {
		if _, err := db.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			t.Fatalf("storetest: reset %s: %v", table, err)
		}
	}
}
