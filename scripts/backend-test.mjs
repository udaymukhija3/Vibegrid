#!/usr/bin/env node

import { spawn } from "node:child_process";
import { mkdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const requireDatabase = process.argv.includes("--require-database");
const databaseURL = process.env.TEST_DATABASE_URL?.trim();

if (!databaseURL) {
  const message = [
    "TEST_DATABASE_URL is not set.",
    "Postgres transaction, concurrency, authorization, and migration tests will be skipped."
  ].join(" ");
  if (requireDatabase) {
    console.error(`${message}\nSet TEST_DATABASE_URL to run the required durable backend suite.`);
    process.exit(1);
  }
  console.warn(`WARNING: ${message}`);
}

// The schema upgrade/rollback tests roll migrations backwards, so they need a
// database that is not shared with TEST_DATABASE_URL. Without it they skip, and
// a silently skipped rollback test is how a broken Down path reaches a bad night
// undetected.
const migrationDatabaseURL = process.env.TEST_MIGRATION_DATABASE_URL?.trim();
if (!migrationDatabaseURL) {
  const message = [
    "TEST_MIGRATION_DATABASE_URL is not set.",
    "Schema upgrade-path and rollback tests will be skipped."
  ].join(" ");
  if (requireDatabase) {
    console.error(`${message}\nPoint it at a scratch database (it is torn down and rebuilt).`);
    process.exit(1);
  }
  console.warn(`WARNING: ${message}`);
}

const goCache = path.join(root, ".cache", "go-build");
await mkdir(goCache, { recursive: true });

const child = spawn("go", ["test", "./backend/..."], {
  cwd: root,
  env: { ...process.env, GOCACHE: goCache },
  stdio: "inherit"
});

child.on("error", (error) => {
  console.error(error);
  process.exit(1);
});
child.on("exit", (code, signal) => {
  if (code === 0) process.exit(0);
  console.error(`go test exited with ${signal ?? code}`);
  process.exit(code ?? 1);
});
