package vibegrid

import (
	"context"
	"os"
	"testing"
)

// TestMigrateOnBootFromScratch runs the full embedded migration chain through the
// production code path (OpenDB -> runMigrations -> goose.Up) against a schema
// reset to its pre-migration state. It guards the 00010 regression where a
// dollar-quoted DO block, unfenced by goose StatementBegin/StatementEnd markers,
// was split on its inner semicolon and crashed migrate-on-boot with "unterminated
// dollar-quoted string" (SQLSTATE 42601).
func TestMigrateOnBootFromScratch(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration tests")
	}
	ctx := context.Background()

	// Reset to a pristine, unmigrated schema so goose applies every migration
	// (including 00010) from scratch on this run rather than skipping ones a
	// previous test already recorded in goose_db_version.
	bare, err := openDB(ctx, databaseURL, false)
	if err != nil {
		t.Fatalf("open without migrations: %v", err)
	}
	if _, err := bare.ExecContext(ctx, "drop schema public cascade; create schema public"); err != nil {
		_ = bare.Close()
		t.Fatalf("reset schema: %v", err)
	}
	_ = bare.Close()

	// The production boot path: OpenDB applies migrations via goose. Before the
	// fix this returned the SQLSTATE 42601 parse error.
	database, err := OpenDB(ctx, databaseURL)
	if err != nil {
		t.Fatalf("migrate on boot: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var n int
	if err := database.QueryRowContext(ctx,
		`select count(*) from pg_constraint
		 where conname = 'attempt_guesses_attempt_id_client_guess_id_key'
		   and conrelid = 'attempt_guesses'::regclass`,
	).Scan(&n); err != nil {
		t.Fatalf("check constraint: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected the attempt_guesses idempotency constraint to exist, found %d", n)
	}
}
