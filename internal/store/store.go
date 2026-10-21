// Package store provides thin wrappers around database/sql for the MySQL
// backend: opening a pool from a DSN and liveness checks used by /healthz.
package store

import (
	"context"
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
)

// Open opens a database/sql pool for the given MySQL DSN. The driver parses
// the DSN eagerly, so a malformed DSN returns an error here; an unreachable
// server does not (the caller decides whether to ping and how fatal failure
// is — main treats it as a warning, /healthz reports 503).
func Open(dsn string) (*sql.DB, error) {
	return sql.Open("mysql", dsn)
}

// Ping reports whether the database answers within the caller's context
// deadline. It exists as a free function so callers (and tests) can supply
// stubs to httpserver.NewWithStore.
func Ping(ctx context.Context, db *sql.DB) error {
	return db.PingContext(ctx)
}
