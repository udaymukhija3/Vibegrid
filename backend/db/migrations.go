// Package db holds the SQL migrations for VibeGrid's durable state, embedded so
// the binary can migrate its own database on startup without shipping loose
// .sql files alongside it.
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
