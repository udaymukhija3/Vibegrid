#!/usr/bin/env node

import { randomUUID } from "node:crypto";
import { pathToFileURL } from "node:url";

class CookieJar {
  #cookies = new Map();

  store(response) {
    const raw = response.headers.get("set-cookie");
    if (!raw) return;
    const first = raw.split(";")[0];
    const separator = first.indexOf("=");
    if (separator > 0) this.#cookies.set(first.slice(0, separator), first.slice(separator + 1));
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
  metricsToken = process.env.VIBEGRID_METRICS_TOKEN ?? "",
  log = console.log
} = {}) {
  const base = normalizeBaseURL(baseUrl);
  const jar = new CookieJar();

  async function requestWithJar(activeJar, path, init = {}) {
    const headers = new Headers(init.headers ?? {});
    const cookie = activeJar.header();
    if (cookie) headers.set("Cookie", cookie);
    const response = await fetch(new URL(path, base), { ...init, headers });
    activeJar.store(response);
    return response;
  }

  async function request(path, init = {}) {
    return requestWithJar(jar, path, init);
  }

  async function expectJSON(path, status = 200, init = {}) {
    const response = await request(path, init);
    assert(response.status === status, `${path} returned ${response.status}, expected ${status}`);
    return { response, payload: await response.json() };
  }

  async function expectText(path, status = 200, init = {}) {
    const response = await request(path, init);
    assert(response.status === status, `${path} returned ${response.status}, expected ${status}`);
    return { response, text: await response.text() };
  }

  const health = await expectJSON("/healthz");
  assert(health.payload.ok === true, "/healthz did not return ok=true");
  const ready = await expectJSON("/readyz");
  assert(ready.payload.ready === true, "/readyz did not return ready=true");
  log("ok health/readiness");

  const root = await expectText("/");
  assert(isHTML(root.response), "/ did not return HTML");
  for (const route of ["/crews", "/archive", "/create", "/demo", "/policy", "/terms", "/privacy"]) {
    const page = await expectText(route);
    assert(isHTML(page.response), `${route} did not return HTML`);
  }
  log("ok public product pages");

  const today = await expectJSON("/api/vibes/today");
  const board = today.payload;
  assertVibeBoard(board, "today board");
  assert(board.tiles.length === 16, "public practice was not a 4x4 board");
  assert(today.response.headers.get("cache-control"), "today board had no cache policy");
  log(`ok board ${String(board.boardNumber).padStart(3, "0")} (${board.publishDate})`);

  const unlimited = await expectJSON("/api/vibes/practice/0");
  const unlimitedVariation = await expectJSON("/api/vibes/practice/12");
  assertVibeBoard(unlimited.payload, "unlimited board");
  assertVibeBoard(unlimitedVariation.payload, "unlimited board variation");
  assert(unlimited.payload.tiles.length === 16, "unlimited practice was not 4x4");
  assert(unlimited.payload.id !== unlimitedVariation.payload.id, "unlimited sequence did not advance");
  assert(
    unlimited.response.headers.get("cache-control")?.includes("immutable"),
    "unlimited board was not immutably cacheable"
  );
  const invalidUnlimited = await request("/api/vibes/practice/not-a-number");
  assert(invalidUnlimited.status === 404, `invalid unlimited sequence returned ${invalidUnlimited.status}`);
  log("ok unlimited practice sequence");

  const session = await expectJSON("/api/session");
  assert(session.payload.mode === "guest", "session did not report guest mode");
  assert(session.payload.guest?.active === true, "guest session was not active");
  assert(jar.has("vibegrid_session"), "guest session cookie was not set");
  log("ok guest browser identity");

  const robots = await expectText("/robots.txt");
  assert(robots.text.includes("Disallow: /admin"), "robots.txt did not protect admin");
  const sitemap = await expectText("/sitemap.xml");
  assert(sitemap.text.includes("/crews"), "sitemap did not include the crew entry point");
  assert(!sitemap.text.includes("/p/"), "sitemap advertised compatibility puzzle links");
  assert(!sitemap.text.includes("/crew/"), "sitemap advertised private crew rooms");
  log("ok search boundaries");

  if (mutate) {
    const create = await request("/api/crews", {
      method: "POST",
      headers: { "Content-Type": "application/json", "Idempotency-Key": randomUUID() },
      body: JSON.stringify({ name: `Smoke ${Date.now()}`, displayName: "Smoke" })
    });
    if (create.status === 503) {
      log("skip durable crew mutation (database unavailable)");
    } else {
      assert(create.status === 201, `/api/crews returned ${create.status}, expected 201`);
      const crew = await create.json();
      assert(typeof crew.inviteCode === "string" && crew.inviteCode.length > 8, "created crew had no invite code");

      // Join to the five-member breakpoint before anyone opens the board. Each
      // browser has its own capability session, matching real concurrent users.
      for (let index = 1; index < 5; index++) {
        const memberJar = new CookieJar();
        const joined = await requestWithJar(
          memberJar,
          `/api/crews/${encodeURIComponent(crew.inviteCode)}/join`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json", "Idempotency-Key": randomUUID() },
            body: JSON.stringify({ displayName: `Smoke ${index}` })
          }
        );
        assert(joined.status === 200, `smoke member ${index} could not join (${joined.status})`);
      }

      const daily = await expectJSON(`/api/crews/${encodeURIComponent(crew.inviteCode)}/daily`);
      assert(daily.payload.isMember === true, "crew creator was not a member");
      assertVibeBoard(daily.payload.today?.board, "crew today board");
      assert(daily.payload.today.board.tiles.length === 16, "five-member crew did not freeze at 4x4");

      const lateJar = new CookieJar();
      const lateJoin = await requestWithJar(lateJar, `/api/crews/${encodeURIComponent(crew.inviteCode)}/join`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": randomUUID() },
        body: JSON.stringify({ displayName: "Smoke late" })
      });
      assert(lateJoin.status === 200, `post-freeze member could not join (${lateJoin.status})`);
      const stillFrozen = await expectJSON(`/api/crews/${encodeURIComponent(crew.inviteCode)}/daily`);
      assert(stillFrozen.payload.today.board.tiles.length === 16, "post-freeze join resized the active board");

      const clientSubmissionId = `smoke_${randomUUID().replaceAll("-", "_")}`;
      const body = {
        boardId: board.id,
        title: "Smoke signal",
        selectedTileIds: board.tiles.slice(0, 4).map((tile) => tile.id),
        clientSubmissionId
      };
      const first = await expectJSON(`/api/crews/${encodeURIComponent(crew.inviteCode)}/submissions`, 201, {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": randomUUID() },
        body: JSON.stringify(body)
      });
      const replay = await expectJSON(`/api/crews/${encodeURIComponent(crew.inviteCode)}/submissions`, 201, {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": randomUUID() },
        body: JSON.stringify(body)
      });
      assert(first.payload.id === replay.payload.id, "submission replay created a second card");

      const reloaded = await expectJSON(`/api/crews/${encodeURIComponent(crew.inviteCode)}/daily`);
      assert(reloaded.payload.today?.submission?.id === first.payload.id, "crew daily did not preserve the submitted card");
      log("ok durable crew create/submit/replay");
    }
  }

  if (metricsToken) {
    const denied = await request("/metrics", { headers: { Authorization: "Bearer wrong-token" } });
    assert(denied.status === 401, `/metrics accepted a wrong token (${denied.status})`);
    const metrics = await expectText("/metrics", 200, { headers: { Authorization: `Bearer ${metricsToken}` } });
    assert(metrics.text.includes("vibegrid_up 1"), "metrics body missed vibegrid_up");
    assert(metrics.text.includes('route="/api/vibes/today"'), "metrics did not label the new board route");
    log("ok protected metrics");
  }

  return { baseUrl: base.toString().replace(/\/$/, ""), boardNumber: board.boardNumber };
}

function assertVibeBoard(board, label) {
  assert(board && typeof board === "object", `${label} was missing`);
  assert(typeof board.id === "string" && board.id.length > 0, `${label} had no id`);
  assert(Number.isInteger(board.boardNumber), `${label} had no board number`);
  assert(/^\d{4}-\d{2}-\d{2}$/.test(board.publishDate), `${label} had an invalid date`);
  assert(typeof board.prompt === "string" && board.prompt.length > 5, `${label} had no prompt`);
  assert(
    Array.isArray(board.tiles) && board.tiles.length >= 12 && board.tiles.length <= 28 && board.tiles.length % 4 === 0,
    `${label} did not expose complete four-column rows`
  );
  for (const forbiddenKey of ["groups", "answers", "answerKey", "difficulty", "mistakesAllowed"]) {
    assert(!(forbiddenKey in board), `${label} leaked obsolete field ${forbiddenKey}`);
  }
  const ids = new Set();
  const texts = new Set();
  for (const [index, tile] of board.tiles.entries()) {
    assert(Object.keys(tile).sort().join(",") === "id,text", `${label} fragment ${index} exposed unexpected fields`);
    ids.add(tile.id);
    texts.add(tile.text.toLocaleLowerCase());
  }
  assert(ids.size === board.tiles.length && texts.size === board.tiles.length, `${label} contained duplicate fragments`);
}

function normalizeBaseURL(value) {
  const url = new URL(value);
  if (url.protocol !== "http:" && url.protocol !== "https:") throw new Error("base URL must use http or https");
  url.pathname = url.pathname.replace(/\/$/, "") + "/";
  return url;
}

function isHTML(response) {
  return response.headers.get("content-type")?.includes("text/html");
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function cliOptions(argv) {
  const args = [...argv];
  const options = {
    baseUrl: process.env.VIBEGRID_BASE_URL ?? "http://127.0.0.1:3000",
    mutate: process.env.VIBEGRID_SMOKE_MUTATE === "true",
    metricsToken: process.env.VIBEGRID_METRICS_TOKEN ?? ""
  };
  for (let index = 0; index < args.length; index++) {
    const arg = args[index];
    if (arg === "--mutate") options.mutate = true;
    else if (arg === "--base-url") options.baseUrl = args[++index];
    else if (arg === "--metrics-token") options.metricsToken = args[++index] ?? "";
    else if (!arg.startsWith("--")) options.baseUrl = arg;
  }
  return options;
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  runSmoke(cliOptions(process.argv.slice(2)))
    .then(({ baseUrl, boardNumber }) => console.log(`smoke passed for ${baseUrl} (board ${boardNumber})`))
    .catch((error) => {
      console.error(error instanceof Error ? error.message : error);
      process.exit(1);
    });
}
