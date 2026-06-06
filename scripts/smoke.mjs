#!/usr/bin/env node

import { randomUUID } from "node:crypto";
import { pathToFileURL } from "node:url";

class CookieJar {
  #cookies = new Map();

  store(response) {
    const raw = response.headers.get("set-cookie");
    if (!raw) {
      return;
    }
    const first = raw.split(";")[0];
    const separator = first.indexOf("=");
    if (separator > 0) {
      this.#cookies.set(first.slice(0, separator), first.slice(separator + 1));
    }
  }

  header() {
    return Array.from(this.#cookies, ([name, value]) => `${name}=${value}`).join("; ");
  }

  has(name) {
    return this.#cookies.has(name);
  }
}

export async function runSmoke({
  baseUrl = process.env.VIBEGRID_BASE_URL ?? "http://127.0.0.1:3000",
  mutate = false,
  createCommunity = false,
  log = console.log
} = {}) {
  const base = normalizeBaseURL(baseUrl);
  const jar = new CookieJar();

  async function request(path, init = {}) {
    const headers = new Headers(init.headers ?? {});
    const cookie = jar.header();
    if (cookie) {
      headers.set("Cookie", cookie);
    }

    const response = await fetch(new URL(path, base), {
      ...init,
      headers
    });
    jar.store(response);
    return response;
  }

  async function expectJSON(path, status = 200, init = {}) {
    const response = await request(path, init);
    assert(response.status === status, `${path} returned ${response.status}, expected ${status}`);
    const payload = await response.json();
    return { response, payload };
  }

  async function expectText(path, status = 200, init = {}) {
    const response = await request(path, init);
    assert(response.status === status, `${path} returned ${response.status}, expected ${status}`);
    return { response, text: await response.text() };
  }

  const health = await expectJSON("/healthz");
  assert(health.payload.ok === true, "/healthz did not return ok=true");
  log("ok healthz");

  const ready = await expectJSON("/readyz");
  assert(ready.payload.ready === true, "/readyz did not return ready=true");
  log("ok readyz");

  const root = await expectText("/");
  assert(isHTML(root.response), "/ did not return HTML");
  log("ok frontend shell");

  const today = await expectJSON("/api/puzzles/today");
  const puzzle = today.payload;
  assert(typeof puzzle.id === "string" && puzzle.id.length > 0, "today puzzle has no id");
  assert(Number.isInteger(puzzle.puzzleNumber), "today puzzle has no puzzle number");
  assert(Array.isArray(puzzle.tiles) && puzzle.tiles.length === 16, "today puzzle does not expose 16 tiles");
  log(`ok today puzzle #${puzzle.puzzleNumber}`);

  const attempt = await expectJSON(`/api/attempts/${encodeURIComponent(puzzle.id)}`);
  assert(attempt.payload.puzzleId === puzzle.id, "attempt payload references the wrong puzzle");
  assert(jar.has("vibegrid_session"), "attempt did not set a vibegrid_session cookie");
  log("ok attempt session");

  const archive = await expectJSON("/api/puzzles");
  assert(Array.isArray(archive.payload), "archive did not return an array");
  log(`ok archive (${archive.payload.length} puzzle${archive.payload.length === 1 ? "" : "s"})`);

  const direct = await expectJSON(`/api/puzzles/${encodeURIComponent(puzzle.id)}`);
  assert(direct.payload.id === puzzle.id, "direct puzzle lookup returned a different puzzle");
  log("ok direct puzzle api");

  const og = await expectText(`/api/og/puzzles/${encodeURIComponent(puzzle.id)}.svg`);
  assert(og.response.headers.get("content-type")?.includes("image/svg+xml"), "OG image was not SVG");
  assert(og.text.includes("<svg"), "OG image body was not SVG");
  log("ok og image");

  const shared = await expectText(`/p/${encodeURIComponent(puzzle.id)}`);
  assert(isHTML(shared.response), "shared puzzle route did not return HTML");
  log("ok shared puzzle page");

  for (const route of ["/policy", "/terms", "/privacy"]) {
    const policy = await expectText(route);
    assert(isHTML(policy.response), `${route} did not return HTML`);
  }
  log("ok policy pages");

  if (mutate) {
    const tileIds = puzzle.tiles.slice(0, 4).map((tile) => tile.id);
    const guess = await expectJSON("/api/guesses", 200, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        puzzleId: puzzle.id,
        selectedTileIds: tileIds,
        clientGuessId: `smoke-${randomUUID()}`
      })
    });
    assert(guess.payload.ok === true, "guess response was not ok");
    assert(guess.payload.attempt?.puzzleId === puzzle.id, "guess did not update the expected attempt");
    log("ok guess write path");
  }

  if (createCommunity) {
    const created = await expectJSON("/api/community/puzzles", 201, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(sampleCommunityPuzzle())
    });
    assert(created.payload.ok === true && created.payload.id, "community create did not return a puzzle id");
    await expectJSON(`/api/puzzles/${encodeURIComponent(created.payload.id)}`);
    await expectText(`/p/${encodeURIComponent(created.payload.id)}`);
    log(`ok community create #${created.payload.puzzleNumber}`);

    const report = await expectJSON("/api/reports", 201, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        puzzleId: created.payload.id,
        reason: "SPAM",
        details: "Smoke test report for the moderation queue.",
        contact: ""
      })
    });
    assert(report.payload.ok === true && report.payload.id, "report did not return a moderation id");
    log("ok moderation report write");
  }

  const metrics = await expectText("/metrics");
  assert(metrics.text.includes("vibegrid_up 1"), "metrics did not expose vibegrid_up");
  assert(metrics.text.includes("vibegrid_http_requests_total"), "metrics did not expose request counters");
  log("ok metrics");

  return { baseUrl: base.toString(), puzzleId: puzzle.id, puzzleNumber: puzzle.puzzleNumber };
}

function normalizeBaseURL(value) {
  const url = new URL(value);
  if (!url.pathname.endsWith("/")) {
    url.pathname += "/";
  }
  return url;
}

function isHTML(response) {
  return response.headers.get("content-type")?.includes("text/html");
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function sampleCommunityPuzzle() {
  return {
    difficulty: "MEDIUM",
    groups: [
      {
        name: "Launch snacks",
        explanation: "Things eaten while watching deploy logs.",
        tiles: ["chips", "tea", "banana", "trail mix"]
      },
      {
        name: "Desk weather",
        explanation: "Tiny forecasts from a working setup.",
        tiles: ["warm laptop", "cold coffee", "window glare", "fan hum"]
      },
      {
        name: "Commit feelings",
        explanation: "Moods that arrive right before pushing.",
        tiles: ["relief", "doubt", "focus", "nerve"]
      },
      {
        name: "Browser rituals",
        explanation: "Things checked before sharing a link.",
        tiles: ["reload", "copy URL", "open tab", "check mobile"]
      }
    ]
  };
}

function cliOptions(argv) {
  const args = [...argv];
  const options = {
    baseUrl: process.env.VIBEGRID_BASE_URL ?? "http://127.0.0.1:3000",
    mutate: process.env.VIBEGRID_SMOKE_MUTATE === "true",
    createCommunity: process.env.VIBEGRID_SMOKE_CREATE_COMMUNITY === "true"
  };

  for (let index = 0; index < args.length; index++) {
    const arg = args[index];
    if (arg === "--mutate") {
      options.mutate = true;
    } else if (arg === "--create-community") {
      options.createCommunity = true;
    } else if (arg === "--base-url") {
      options.baseUrl = args[++index];
    } else if (!arg.startsWith("--")) {
      options.baseUrl = arg;
    }
  }
  return options;
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  runSmoke(cliOptions(process.argv.slice(2)))
    .then(({ baseUrl, puzzleNumber }) => {
      console.log(`smoke passed for ${baseUrl} (VibeGrid #${puzzleNumber})`);
    })
    .catch((error) => {
      console.error(error instanceof Error ? error.message : error);
      process.exit(1);
    });
}
