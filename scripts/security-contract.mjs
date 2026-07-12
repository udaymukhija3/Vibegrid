#!/usr/bin/env node

import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const goRoots = [path.join(root, "backend", "internal", "vibegrid"), path.join(root, "backend", "cmd", "vibegrid")];

const sqlCallPattern = /\b(?:QueryContext|QueryRowContext|ExecContext|PrepareContext|Prepare)\s*\(/;
const riskyBuilders = [
  { pattern: /fmt\.Sprintf\s*\(/, label: "fmt.Sprintf" },
  { pattern: /strings\.Builder\b/, label: "strings.Builder" },
  { pattern: /bytes\.Buffer\b/, label: "bytes.Buffer" },
  { pattern: /\btemplate\.(?:Must|New)\b/, label: "template builder" }
];

const allowedQueryAppend = 'query += " for update"';
const failures = [];
let sqlFiles = 0;

for (const dir of goRoots) {
  for await (const file of walk(dir)) {
    if (!file.endsWith(".go") || file.endsWith("_test.go")) {
      continue;
    }

    const source = await readFile(file, "utf8");
    if (!sqlCallPattern.test(source)) {
      continue;
    }
    sqlFiles++;

    const lines = source.split("\n");
    for (const [index, line] of lines.entries()) {
      for (const { pattern, label } of riskyBuilders) {
        if (pattern.test(line)) {
          failures.push(`${relative(file)}:${index + 1} uses ${label} in a SQL-bearing file`);
        }
      }

      if (/query\s*\+=/.test(line) && !line.includes(allowedQueryAppend)) {
        failures.push(`${relative(file)}:${index + 1} appends to a SQL query outside the allowed row-lock clause`);
      }
    }
  }
}

await assertNoBroadTrustedProxyDefaults();

if (failures.length > 0) {
  console.error("security contract failed:");
  for (const failure of failures) {
    console.error(`- ${failure}`);
  }
  process.exit(1);
}

console.log(`security contract passed (${sqlFiles} SQL-bearing Go files inspected)`);

async function* walk(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      yield* walk(fullPath);
    } else {
      yield fullPath;
    }
  }
}

async function assertNoBroadTrustedProxyDefaults() {
  const configFiles = ["fly.toml", "render.yaml", ".env.example"];
  for (const file of configFiles) {
    const source = await readFile(path.join(root, file), "utf8");
    for (const [index, line] of source.split("\n").entries()) {
      if (!line.includes("VIBEGRID_TRUSTED_PROXY_CIDRS")) {
        continue;
      }
      if (line.includes("0.0.0.0/0") || line.includes("::/0")) {
        failures.push(`${file}:${index + 1} configures a broad trusted proxy CIDR`);
      }
    }
  }
}

function relative(file) {
  return path.relative(root, file);
}
