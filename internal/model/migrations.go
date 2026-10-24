package model

import "time"

// SchemaMigration is one row of schema_migrations (§6.15). CreatedAt is the
// applied timestamp; the name is the migration filename without ".up.sql".
type SchemaMigration struct {
	ID        uint32
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt time.Time
}
