// Package db holds the SQL migrations for VibeGrid's durable state, embedded so
// the release-time migrate command can run without shipping loose .sql files.
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
