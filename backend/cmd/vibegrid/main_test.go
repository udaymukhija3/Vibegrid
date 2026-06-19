package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestBuildDepsRequiresDatabaseWhenConfigured(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := buildDeps(context.Background(), logger, "", true)
	if err == nil {
		t.Fatal("expected missing database error")
	}
	if !strings.Contains(err.Error(), "VIBEGRID_REQUIRE_DATABASE=true") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildDepsKeepsNoDatabaseModeForLocalRuns(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	deps, err := buildDeps(context.Background(), logger, "", false)
	if err != nil {
		t.Fatalf("buildDeps: %v", err)
	}
	t.Cleanup(deps.close)

	if deps.attempts == nil {
		t.Fatal("expected in-memory attempt store")
	}
	if deps.puzzles == nil {
		t.Fatal("expected demo puzzle source")
	}
	if deps.ready != nil || deps.dbStats != nil || deps.puzzleCacheStats != nil {
		t.Fatal("no-database mode should not expose database collectors")
	}
}
