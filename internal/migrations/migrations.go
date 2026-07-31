package migrations

import "embed"

// FS holds all SQL migration files embedded at compile time.
// This file lives at internal/migrations/ which can reach
// the migrations/ directory one level up via the module root.
//
//go:embed sql/*.sql
var FS embed.FS