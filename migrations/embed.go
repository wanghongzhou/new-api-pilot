package migrations

import "embed"

// Files contains immutable production migrations bundled into the binary.
//
//go:embed *.sql
var Files embed.FS
