// Package model holds one plain struct per table in the schema defined by
// docs/superpowers/specs/2026-10-23-schema-and-migrations-design.md.
//
// The structs are data carriers only: no methods, no struct tags, no
// validation, no queries. Field order follows the table definition with the
// audit kit (created_at, updated_at, deleted_at) last.
//
// Type mapping:
//
//	INT UNSIGNED         uint32
//	SMALLINT UNSIGNED    uint16
//	TINYINT UNSIGNED     uint8
//	TINYINT(1) UNSIGNED  bool
//	DATETIME             time.Time
//	VARCHAR/CHAR/ENUM    string
//	BINARY/VARBINARY     []byte
//	DECIMAL(5,4)         float64
//
// Nullable columns use a pointer, or a nil slice for binary columns. The one
// exception is deleted_at, which is NOT NULL and uses the epoch sentinel
// 1970-01-01 00:00:00 for live rows.
package model
