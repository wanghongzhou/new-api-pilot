package migrations

import "embed"

// Files contains the authoritative schema migrations bundled into the binary.
//
//go:embed *.sql
var Files embed.FS
