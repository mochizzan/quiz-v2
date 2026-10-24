// Package testlock serialises the integration-test binaries that share one
// MariaDB schema.
//
// `go test ./...` runs each package's test binary in parallel. Every package
// that needs a database talks to the same TEST_DB_DSN schema, so without a
// cross-process lock two packages can migrate, seed, and reset the same tables
// at the same time (duplicate ledger rows, half-deleted fixtures, a seed that
// vanishes mid-test). The lock is taken once per process and held until the
// process exits; a test helper may call Acquire from every test safely.
package testlock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// lockName is the MariaDB advisory lock every database-using test binary takes.
// It is distinct from quiz.migrations, which serialises migration runners in
// production.
const lockName = "quiz.tests"

// waitSeconds bounds how long one test binary waits for another. A deadlock in
// a sibling package should fail the run with a clear message instead of hanging
// forever.
const waitSeconds = 300

// pollSeconds is the server-side wait of one GET_LOCK attempt. The test DSN
// carries a short readTimeout (10s), so asking MariaDB to block for the whole
// five-minute budget would time the driver's read out and poison the
// connection. Short polls keep each round trip inside that timeout and still
// cover the full budget.
const pollSeconds = 5

var (
	once sync.Once
	err  error
	// conn and pool are intentionally retained for the life of the process:
	// MariaDB releases GET_LOCK when the owning connection closes.
	conn *sql.Conn
	pool *sql.DB
)

// Acquire blocks until this process owns the cross-process test lock. When
// TEST_DB_DSN is unset there is no database to protect and Acquire is a no-op,
// so a helper can call it before deciding to skip.
func Acquire(ctx context.Context) error {
	once.Do(func() {
		dsn := os.Getenv("TEST_DB_DSN")
		if dsn == "" {
			return
		}
		db, oerr := sql.Open("mysql", dsn)
		if oerr != nil {
			err = fmt.Errorf("testlock: open: %w", oerr)
			return
		}
		c, cerr := db.Conn(ctx)
		if cerr != nil {
			_ = db.Close()
			err = fmt.Errorf("testlock: conn: %w", cerr)
			return
		}
		var got sql.NullInt64
		deadline := time.Now().Add(waitSeconds * time.Second)
		for {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				break
			}
			wait := pollSeconds
			if remaining < pollSeconds*time.Second {
				wait = int(remaining / time.Second)
				if wait < 1 {
					wait = 1
				}
			}
			if qerr := c.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, wait).Scan(&got); qerr != nil {
				_ = c.Close()
				_ = db.Close()
				err = fmt.Errorf("testlock: GET_LOCK: %w", qerr)
				return
			}
			if got.Valid && got.Int64 == 1 {
				conn, pool = c, db
				return
			}
		}
		_ = c.Close()
		_ = db.Close()
		err = errors.New("testlock: could not acquire quiz.tests within the wait window")
	})
	return err
}

// Hold is a convenience for tests: it acquires the lock under a bounded context
// and stops the test with a clear failure when the lock cannot be taken.
func Hold(ctx context.Context) error {
	lockCtx, cancel := context.WithTimeout(ctx, (waitSeconds+30)*time.Second)
	defer cancel()
	return Acquire(lockCtx)
}
