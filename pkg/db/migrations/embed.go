package migrations

import "embed"

// FS contains all *.up.sql migration files, embedded at compile time.
//
//go:embed *.up.sql
var FS embed.FS
