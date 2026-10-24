// Package migrations embeds the schema files so a compiled binary carries the
// exact SQL it was built with. The seven .up.sql files are frozen (contract
// R-01); this bridge only makes them readable to the runner without a working
// directory or a copy step in the image.
package migrations

import "embed"

// SQL holds every *.up.sql in this directory, keyed by filename.
//
//go:embed *.up.sql
var SQL embed.FS
