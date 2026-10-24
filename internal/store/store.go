// Package store provides the database seam for the whole application: one
// pool with the pinned settings, the soft-delete sentinel, a transaction
// helper with the retry policy, and a classification of MySQL errors so
// handlers never parse driver text.
//
// Everything here is fixed by business-logic spec OPS-215 (error mapping),
// OPS-233 (DSN and pool), and schema spec §5.3 (the sentinel). No handler
// opens a pool and no handler inspects a driver error code directly.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

// LiveAt is the deleted_at sentinel every read filters on. A row is live when
// deleted_at equals this value and deleted when it holds a real timestamp.
// It is the epoch, not NULL, so that the unique keys that include deleted_at
// actually collide for live rows (schema spec §5.3).
const LiveAt = "1970-01-01 00:00:00"

// LiveAtTime is LiveAt as a time.Time for comparisons in Go.
var LiveAtTime = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

// Pool limits are pinned by OPS-233. They are not tuned per deployment.
const (
	maxOpenConns    = 25
	maxIdleConns    = 25
	connMaxLifetime = 30 * time.Minute
	connMaxIdleTime = 5 * time.Minute
)

// DB is the application pool. It embeds *sql.DB so callers that only need the
// standard surface (health checks, tests) use it directly.
type DB struct {
	*sql.DB
}

// Querier is the read/write surface shared by the pool and a transaction, so a
// repository function can be called from either context unchanged.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Querier returns the pool as a Querier.
func (db *DB) Querier() Querier { return db.DB }

// Open opens a pool for the given DSN and applies the pinned pool settings.
// The driver parses the DSN eagerly, so a malformed DSN fails here; an
// unreachable server does not (main treats that as a warning and /healthz
// reports 503 until it returns).
func Open(dsn string) (*DB, error) {
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(maxOpenConns)
	pool.SetMaxIdleConns(maxIdleConns)
	pool.SetConnMaxLifetime(connMaxLifetime)
	pool.SetConnMaxIdleTime(connMaxIdleTime)
	return &DB{DB: pool}, nil
}

// CheckDSN verifies the DSN carries the settings the business rules depend on.
// It is called at startup, not by Open, so tests can exercise Open with a
// minimal DSN. Requires parseTime and loc=UTC, and rejects multiStatements:
// without parseTime every DATETIME scans as []byte; without loc=UTC every
// parsed time is offset from UTC; multiStatements makes one injection a whole
// script (OPS-232, OPS-233).
func CheckDSN(dsn string) error {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("store: parse DSN: %w", err)
	}
	if cfg.ParseTime != true {
		return errors.New("store: DSN must set parseTime=true")
	}
	if cfg.Loc != time.UTC {
		return fmt.Errorf("store: DSN must set loc=UTC (got %q)", cfg.Loc.String())
	}
	if cfg.MultiStatements {
		return errors.New("store: DSN must not enable multiStatements")
	}
	if cfg.DBName == "" {
		return errors.New("store: DSN must name a database")
	}
	return nil
}

// Ping reports whether the database answers within the caller's context
// deadline. It is a free function so callers (and tests) can supply stubs to
// httpserver.NewWithStore.
func Ping(ctx context.Context, db *DB) error {
	return db.PingContext(ctx)
}

// Tx runs fn inside one transaction. It commits when fn returns nil and rolls
// back on any error. The rollback error is discarded: the business failure is
// what the caller needs to see, and a failed rollback means the connection is
// no longer usable anyway.
func (db *DB) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return nil
}

// TxRetry is Tx with the one permitted retry: a deadlock (1213) or lock wait
// timeout (1205) is retried exactly once, and only because the caller declared
// the unit idempotent by choosing this function. A second failure is returned
// to the caller, which maps it to 500 (OPS-215).
func (db *DB) TxRetry(ctx context.Context, fn func(*sql.Tx) error) error {
	err := db.Tx(ctx, fn)
	if err == nil {
		return nil
	}
	if c := Classify(err); c == Deadlock || c == LockWait {
		return db.Tx(ctx, fn)
	}
	return err
}

// Class is the MySQL failure family a caller may branch on.
type Class uint8

const (
	// ClassOther is anything the taxonomy does not name.
	ClassOther Class = iota
	// Duplicate is 1062: a unique key fired (OPS-141 maps this per domain).
	Duplicate
	// Deadlock is 1213.
	Deadlock
	// LockWait is 1205.
	LockWait
	// FKMissing is 1452: an insert referenced a row that does not exist.
	FKMissing
	// TooLong is 1406: a value exceeded its column.
	TooLong
	// CheckViolation is 3819: a CHECK constraint fired.
	CheckViolation
)

// Classify maps an error to its family without parsing driver text.
func Classify(err error) Class {
	var me *mysql.MySQLError
	if !errors.As(err, &me) {
		return ClassOther
	}
	switch me.Number {
	case 1062:
		return Duplicate
	case 1213:
		return Deadlock
	case 1205:
		return LockWait
	case 1452:
		return FKMissing
	case 1406:
		return TooLong
	case 3819:
		return CheckViolation
	default:
		return ClassOther
	}
}

// IsDuplicate reports the 1062 case, the one callers branch on most often.
func IsDuplicate(err error) bool { return Classify(err) == Duplicate }

// Code returns the MySQL error number, or 0 for a non-driver error. It exists
// for log lines; business code branches on Classify.
func Code(err error) uint16 {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number
	}
	return 0
}
