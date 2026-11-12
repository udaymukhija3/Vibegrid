import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import { chromium, expect } from "@playwright/test";

const [baseURL, clockToken] = process.argv.slice(2);
assert(baseURL && clockToken, "Run this through npm run test:e2e (requires its test-only clock).");
const artifactDir = "dist/browser-e2e";
await mkdir(artifactDir, { recursive: true });
const browser = await chromium.launch();
const contexts = [];
const pages = [];
const errors = [];

async function player(name, mobile = false) {
  const context = await browser.newContext({
    baseURL, viewport: mobile ? { width: 390, height: 844 } : { width: 1280, height: 900 },
    reducedMotion: "reduce"
  });
  contexts.push(context);
  await context.tracing.start({ screenshots: true, snapshots: true, sources: true });
  const page = await context.newPage();
  page.on("pageerror", (error) => errors.push(`${name}: ${error.message}`));
  pages.push({ name, page });
  return page;
}

async function daily(page, crewPath) {
  const response = await page.request.get(`/api/crews/${crewPath.split("/").at(-1)}/daily`);
  assert.equal(response.status(), 200);
  return response.json();
}

async function join(page, crewPath, name) {
  await page.goto(crewPath);
  await page.getByLabel("Your name in this crew").fill(name);
  await page.getByRole("button", { name: "Join and make today's vibe" }).click();
  await expect(page.getByRole("heading", { name: "Make today's vibe card" })).toBeAttached();
}

async function make(page, title) {
  const section = page.locator('section[aria-labelledby="make-heading"]');
  for (let index = 0; index < 4; index += 1) {
    await section.locator(".vg-fragment").nth(index).click();
  }
  await section.getByLabel("Name the vibe").fill(title);
  await section.getByRole("button", { name: "Lock it in" }).click();
  await expect(page.getByRole("heading", { name: "Your card is waiting for tomorrow." })).toBeVisible();
}

async function nextDay() {
  const response = await fetch(`${baseURL}/__test/next-day`, {
    method: "POST", headers: { "X-Test-Clock": clockToken }
  });
  assert.equal(response.status, 204);
}

try {
  const ari = await player("Ari");
  const bea = await player("Bea", true);
  const cam = await player("Cam");
  const spectator = await player("Dee");
  const outsider = await player("Outsider");
  await ari.goto("/crews");
  await ari.getByLabel("Crew name", { exact: true }).fill("Browser Night Shift");
  await ari.getByLabel("Your name in this crew").fill("Ari");
  await ari.getByRole("button", { name: "Make crew and get link" }).click();
  await expect(ari).toHaveURL(/\/crew\/[A-Za-z0-9_-]+$/);
  const crewPath = new URL(ari.url()).pathname;
  await expect(ari.locator(".vg-fragment")).toHaveCount(12);
  await join(bea, crewPath, "Bea");
  await join(cam, crewPath, "Cam");
  await join(spectator, crewPath, "Dee");
  await outsider.goto(crewPath);

  const makers = [ari, bea, cam];
  const names = ["Ari", "Bea", "Cam"];
  const titles = ["Calendar cosplay", "Inbox holiday", "Deadline weather"];
  await Promise.all(makers.map((page, index) => make(page, titles[index])));
  for (let index = 0; index < makers.length; index += 1) {
    await makers[index].reload();
    const state = await daily(makers[index], crewPath);
    assert.equal(state.today.submittedCount, 3);
    assert.equal(state.today.submission.title, titles[index]);
    for (const title of titles.filter((_, other) => index !== other)) {
      await expect(makers[index].getByRole("heading", { name: title, exact: true })).toHaveCount(0);
      assert(!JSON.stringify(state).includes(title), "make response disclosed another card");
    }
  }
  const preview = await daily(outsider, crewPath);
  assert.equal(preview.isMember, false);
  assert.equal(preview.members, undefined);
  assert.equal(preview.today.submission, undefined);
  assert.equal(preview.judge, undefined);
  assert.equal(preview.result, undefined);
  console.log("browser ok isolated identities, concurrent make, reload persistence, outsider privacy");

  await nextDay();
  await Promise.all([...makers, spectator, outsider].map((page) => page.reload()));
  const states = await Promise.all(makers.map((page) => daily(page, crewPath)));
  const ids = states.map((state) => state.judge.cards.find((card) => card.isYours).id);
  for (let index = 0; index < makers.length; index += 1) {
    const section = makers[index].locator('section[aria-labelledby="judge-heading"]');
    await expect(section.locator(".vg-vibe-card")).toHaveCount(3);
    await expect(section.getByRole("button").filter({ hasText: titles[index] })).toBeDisabled();
    assert(states[index].judge.cards.every((card) => card.authorName === undefined && card.votes === undefined));
  }
  await expect(spectator.getByRole("heading", { name: "You sat this one out." })).toBeVisible();
  assert.equal((await daily(spectator, crewPath)).judge.cards, undefined);

  // Ari backs Bea; Bea and Cam back Ari: a sole plurality, not a forced tie.
  const targets = [1, 0, 0];
  for (let index = 0; index < makers.length; index += 1) {
    const section = makers[index].locator('section[aria-labelledby="judge-heading"]');
    await section.getByRole("button").filter({ hasText: titles[targets[index]] }).click();
    await section.getByRole("button", { name: "Lock my vote" }).click();
    await expect(section.getByText("Your vote is locked. The tally stays hidden until reveal.")).toBeVisible();
    await makers[index].reload();
    const state = await daily(makers[index], crewPath);
    assert.equal(state.judge.hasVoted, true);
    assert.equal(state.judge.yourVoteId, ids[targets[index]]);
    assert(state.judge.cards.every((card) => card.authorName === undefined && card.votes === undefined));
  }
  console.log("browser ok next-day blind judging, no self-vote, makers-only eligibility, ballot persistence");

  await nextDay();
  await Promise.all([...makers, spectator, outsider].map((page) => page.reload()));
  for (const page of [...makers, spectator]) {
    const state = await daily(page, crewPath);
    assert.equal(state.result.official, true);
    assert.equal(state.result.tied, false);
    assert.equal(state.result.voteCount, 3);
    assert.equal(state.crewStreak, 1);
    assert.deepEqual(state.result.cards.map((card) => card.authorName).sort(), [...names].sort());
    assert.deepEqual(state.result.cards.filter((card) => card.winner).map((card) => card.id), [ids[0]]);
    const result = page.locator('section[aria-labelledby="latest-result"]');
    for (const title of titles) {
      await expect(result.getByRole("heading", { name: title, exact: true })).toBeVisible();
    }
    await expect(result.getByText("Ari", { exact: true })).toBeVisible();
    await expect(result.getByText("2 votes", { exact: true })).toBeVisible();
  }
  assert.equal((await daily(outsider, crewPath)).result, undefined);
  assert.deepEqual(errors, [], "browser exceptions");
  console.log("browser ok day-three reveal, authors, sole winner, exact counts, streak, continued outsider privacy");
} catch (error) {
  await Promise.allSettled(pages.map(({ name, page }) => page.screenshot({ path: `${artifactDir}/${name}.png`, fullPage: true })));
  throw error;
} finally {
  await Promise.allSettled(contexts.map((context, index) => context.tracing.stop({ path: `${artifactDir}/context-${index}.zip` })));
  await browser.close();
}
