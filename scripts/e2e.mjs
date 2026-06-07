#!/usr/bin/env node

import { spawn } from "node:child_process";
import { mkdir, readdir, rm, cp } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { runSmoke } from "./smoke.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const frontendOut = path.join(root, "out");
const embeddedOut = path.join(root, "backend", "internal", "frontend", "out");
const goCache = path.join(root, ".cache", "go-build");
const e2eBinary = path.join(root, "dist", "vibegrid-e2e");

async function main() {
  await mkdir(goCache, { recursive: true });
  await mkdir(path.dirname(e2eBinary), { recursive: true });
  await run("npm", ["run", "build"]);
  await copyStaticExport();
  await run("go", ["build", "-o", e2eBinary, "./backend/cmd/vibegrid"], { GOCACHE: goCache });

  const port = Number(process.env.VIBEGRID_E2E_PORT ?? "18081");
  const addr = `127.0.0.1:${port}`;
  const baseUrl = `http://${addr}`;
  const server = spawn(e2eBinary, [], {
    cwd: root,
    env: {
      ...process.env,
      GOCACHE: goCache,
      VIBEGRID_ADDR: addr,
      VIBEGRID_TIMEZONE: "UTC",
      VIBEGRID_BLOCKED_TERMS: "blocked-smoke-term"
    },
    stdio: ["ignore", "pipe", "pipe"]
  });

  const logs = [];
  server.stdout.on("data", (chunk) => rememberLog(logs, chunk));
  server.stderr.on("data", (chunk) => rememberLog(logs, chunk));

  try {
    await waitForReady(baseUrl, server, logs);
    await runSmoke({
      baseUrl,
      mutate: true,
      log: (line) => console.log(`e2e ${line}`)
    });
    console.log(`e2e passed at ${baseUrl}`);
  } finally {
    await stop(server);
  }
}

async function copyStaticExport() {
  await mkdir(embeddedOut, { recursive: true });
  for (const entry of await readdir(embeddedOut)) {
    if (entry !== ".keep") {
      await rm(path.join(embeddedOut, entry), { recursive: true, force: true });
    }
  }
  for (const entry of await readdir(frontendOut)) {
    await cp(path.join(frontendOut, entry), path.join(embeddedOut, entry), { recursive: true });
  }
}

function run(command, args, env = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: root,
      env: { ...process.env, ...env },
      stdio: "inherit"
    });
    child.on("error", reject);
    child.on("exit", (code, signal) => {
      if (code === 0) {
        resolve();
      } else {
        reject(new Error(`${command} ${args.join(" ")} exited with ${signal ?? code}`));
      }
    });
  });
}

async function waitForReady(baseUrl, child, logs) {
  const deadline = Date.now() + 45_000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`server exited before ready:\n${logs.join("")}`);
    }
    try {
      const response = await fetch(`${baseUrl}/readyz`);
      if (response.ok) {
        return;
      }
    } catch {
      // Server is still starting.
    }
    await delay(500);
  }
  throw new Error(`server did not become ready:\n${logs.join("")}`);
}

async function stop(child) {
  if (child.exitCode !== null) {
    return;
  }

  child.kill("SIGTERM");
  const exited = new Promise((resolve) => child.once("exit", resolve));
  const timeout = delay(5_000).then(() => {
    if (child.exitCode === null) {
      child.kill("SIGKILL");
    }
  });
  await Promise.race([exited, timeout]);
}

function rememberLog(logs, chunk) {
  logs.push(chunk.toString());
  while (logs.length > 40) {
    logs.shift();
  }
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack : error);
  process.exit(1);
});
