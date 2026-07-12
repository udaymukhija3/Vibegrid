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

func TestProductionConfigHelpersRejectUnsafeValues(t *testing.T) {
	environment, err := runtimeEnvironment("")
	if err != nil || environment != "production" {
		t.Fatalf("unset environment must default to production, got %q, %v", environment, err)
	}
	// Absence degrades (with a runtime warning); only explicit bad values are
	// fatal. A fatal absence check bricked every production deploy once — the
	// platform env never received the new variable, so the binary always died
	// at boot while the old release kept serving.
	if value, err := validatedPublicBaseURL("", true); err != nil || value == "" {
		t.Fatalf("missing public base URL must fall back, not fail boot: %q, %v", value, err)
	}
	if _, err := validatedPublicBaseURL("http://vibegrid.example", true); err == nil {
		t.Fatal("production must reject a non-HTTPS public URL")
	}
	if _, err := validatedPublicBaseURL("https://vibegrid.example/not-an-origin", true); err == nil {
		t.Fatal("production must reject a public URL with a path")
	}
	if err := validateTrustedProxyCIDRs([]string{"not-a-cidr"}); err == nil {
		t.Fatal("invalid trusted proxy CIDR must fail startup")
	}
	if err := validateTrustedProxyCIDRs([]string{"10.0.0.0/8", "2001:db8::/32"}); err != nil {
		t.Fatalf("valid trusted proxy CIDRs rejected: %v", err)
	}
}
