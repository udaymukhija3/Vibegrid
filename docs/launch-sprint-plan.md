# VibeGrid — Launch & Production Sprint Plan

Status anchor: the app is **feature-complete locally** (play / create+share / admin
author / stats / moderation) and **pushed to GitHub** (PRs #1, #2 merged, CI wired),
but **not yet deployed to a permanent host**. This document turns the remaining
work in [`production-readiness.md`](production-readiness.md) and
[`tier3-plan.md`](tier3-plan.md) into **vertical, shippable slices**.

> Companion docs: [`production-readiness.md`](production-readiness.md) (the P0–P3
> backlog + business-logic review), [`tier3-plan.md`](tier3-plan.md) (the two
> code-ready items), [`deployment.md`](deployment.md) (the runbook),
> [`observability.md`](observability.md).

---

## How to read this plan

- **Slice, not layer.** Each sprint is a *thin vertical cut* that ends in
  something a user (or operator) can observe end-to-end — not "do all the
  backend, then all the frontend." Every sprint ships.
- **Ordered by risk × value.** Earlier sprints remove launch-blocking risk;
  later ones add growth and durability. Stop wherever the portfolio/product goal
  is met — each sprint leaves the app in a better, releasable state.
- **One PR-able unit per slice where possible.** Big API-contract changes
  (e.g. the `today` payload) move frontend + backend in the *same* PR so a route
  never returns an unhandled shape.
- **Sizing** is rough (S ≈ ½ day, M ≈ 1–2 days, L ≈ 3–5 days) for a solo dev.

### Cross-cutting engineering standards (apply to every sprint)

These are the "best practices" guardrails; they are not re-listed per sprint.

1. **Trunk-based + protected `main`.** Branch per slice, PR, CI green before
   merge, squash. No direct pushes to `main`.
2. **CI is the gate.** Every PR must pass: `gofmt`, `go vet`,
   `go test -race ./...` (Postgres service), `npm run lint`, `npm run typecheck`,
   `npm test`, `npm run build`. New behavior ships with new tests.
3. **Testing pyramid.** Unit/table tests for logic, integration tests against
   real Postgres for stores/handlers, a thin e2e/smoke layer for routes. Don't
   push coverage up the pyramid (no logic-in-e2e).
4. **Feature-flag risky slices.** Anything that changes the live daily contract
   (no-puzzle-today, accounts) goes behind an env flag so it can be dark-launched
   and rolled back without a redeploy.
5. **Observability per slice.** A slice isn't done until its failure mode is
   visible — a metric, a log line, or an alert rule under [`monitoring/`](../monitoring).
6. **Reversibility.** Each slice has a rollback note (flag off, revert migration,
   re-promote previous deploy). Migrations are additive and backward-compatible
   within a sprint.
7. **Decision gates.** Where a slice needs a product call, it's listed under
   **Decisions to lock** below and blocks only that slice.

### Decisions to lock (owner: product = you)

These gate specific sprints; resolve before that sprint starts. (Several from
[`production-readiness.md`](production-readiness.md) Part 2 are already settled —
listed for completeness.)

| # | Decision | Gates | Recommended default |
|---|----------|-------|---------------------|
| D1 | **Launch timezone** for "today" rollover | Sprint 0, 2 | `UTC` (document the flip time) |
| D2 | **No-puzzle-today**: strict-daily empty state vs. rolling "stale" banner | Sprint 2 | Strict-daily (A) — honest for a "daily" brand |
| D3 | **Streak fairness**: skip no-puzzle days so they don't break streaks | Sprint 2 | Yes (skip gaps) |
| ✓ | **Public login model** | — | Guest play for v1; admin login only |
| D4 | **Optional accounts trigger**: when to add cross-device identity | Sprint 8 | Only after retention or cross-device streak demand is proven |
| D5 | **Community numbering**: shared `puzzle_number` vs. separate label | Sprint 2 (cheap) | Separate "Community" label in share text |
| D6 | Fairness check approach: manual red-herring list vs. embeddings-assisted | Sprint 7 | Manual first, embeddings later (sanctioned ML story) |
| D7 | Anonymous-attempt **data-retention** window | Sprint 3 | 13 months, then aggregate-and-purge |
| ✓ | Stats min-N gate (=20) | — | Done (`MIN_STATS_PLAYERS`) |
| ✓ | Real-time multiplayer / adaptive-difficulty bandit | — | Out of scope; async links cover v1 |

Locked for launch: public players are guests, not account holders. "Login" means
the existing admin sign-in for the Editor Desk. Multiplayer means share text and
community puzzle links, not live rooms.

---

## Sprint map (at a glance)

| Sprint | Slice (user-facing outcome) | Size | Blocks on |
|--------|-----------------------------|------|-----------|
| **0** | The app is live on a public HTTPS URL and a stranger can play today's puzzle | M | D1 |
| **1** | Shared links render real preview cards everywhere (iMessage/Slack/X) | M | — |
| **2** | The daily never silently replays yesterday; streaks survive gap days | L | D1, D2, D3, D5 |
| **3** | Trust & durability: backups, dep-scanning, retention, abuse confirmed | M | D7 |
| **4** | Quality gates: browser e2e + component + load + a11y in CI | L | — |
| **5** | We can see the funnel and whether difficulty labels are honest | M | — |
| **6** | Authors can preview a draft and schedule a queue (content sustainability) | L | — |
| **7** | Puzzles are checked for ambiguity before publish (fairness) | L | D6 |
| **8** | (Optional) Accounts for cross-device streaks; scale-outs | XL | D4 |

---

## Sprint 0 — Go live (deploy the app that already exists)

**Slice goal:** a person who is not you can open a public HTTPS URL, play today's
puzzle, make a guess, refresh, and still have their attempt — with the operator
able to see health and metrics.

**Why first:** everything else is iteration on a deployed product. Nothing here
is new feature code; it's wiring the existing container + DB to a host. This is
the single biggest piece of "waiting to be shipped."

**Scope**
- Provision **Neon Postgres** (free tier) and **Render Web Service** from
  [`render.yaml`](../render.yaml) (or Fly via [`fly.toml`](../fly.toml)).
- Set production secrets: `DATABASE_URL` (pooled), `VIBEGRID_SECURE_COOKIES=true`,
  strong `VIBEGRID_ADMIN_PASSWORD` + `VIBEGRID_ADMIN_SESSION_SECRET`, fixed
  `VIBEGRID_TIMEZONE` (**D1**), `VIBEGRID_ALLOWED_ORIGINS` if any non-same-origin
  client. On Render free: `VIBEGRID_MIGRATE_ON_BOOT=true`; on Fly: keep the
  `vibegrid migrate` release command (do **not** set boot-migrate).
- Confirm `SetMaxOpenConns(10) × instances` stays under Neon's pooler cap.
- **Verify the cookie path in prod** — the top day-one risk: make a guess, refresh,
  confirm `vibegrid_session` persists same-origin behind the host's proxy.
- Custom domain + automatic HTTPS; confirm `/healthz` (liveness) and `/readyz`
  (DB ping) are wired to the platform's health checks.
- Confirm **CI is green in the cloud** and **protect `main`** (require CI).
- Keep Render warm against the ~15-min idle spin-down with a free uptime ping to
  `/healthz` (per the runbook).

**Out of scope:** any new feature; OG raster (Sprint 1); no-puzzle state (Sprint 2).

**Acceptance criteria**
- `https://<domain>/` serves the daily; a guess + refresh preserves the attempt.
- `scripts/smoke.mjs` passes against the production URL (`npm run smoke:deploy`).
- `/metrics` scrapes; `/readyz` returns 200 with DB attached.
- Admin can log in at `/admin` over HTTPS with the production password.

**Testing / verification:** run the smoke script against prod; manually drive one
full play + one create-and-share; check `/metrics` and logs.

**Rollback:** re-promote the previous deploy (none yet — so: keep the container
image tag; Render/Fly one-click rollback once a second deploy exists).

**Definition of Done:** public URL works for a stranger on mobile + desktop;
operator dashboards populate; `main` protected; runbook updated with the real
host + domain.

---

## Sprint 1 — Link previews that actually render (Tier-3 Item A)

**Slice goal:** pasting a `/p/{id}` link into iMessage, Slack, WhatsApp, X,
Discord, or LinkedIn shows a real branded puzzle card.

**Why now:** the share loop is the cheapest growth mechanic and it's currently
**broken** — both OG images are SVG, which every major unfurler ignores, and OG
tags are emitted twice (Next metadata *and* the Go injector) so served pages
carry conflicting tags. Small, isolated, no product decision. (Full design in
[`tier3-plan.md`](tier3-plan.md) Item A.)

**Scope**
- Add `renderPuzzleOGPNG(puzzle) ([]byte, error)` — port the existing
  `renderPuzzleOGImage` layout (1200×630) to `github.com/fogleman/gg` + stdlib
  `image/png`; fonts via `golang.org/x/image/font/gofont` (compiled in, license-clean,
  no cgo → works in distroless).
- Serve `/api/og/puzzles/{id}.png` (`image/png`, keep the long `Cache-Control`).
  Add a small in-process LRU of rendered PNG bytes keyed by id (mirror
  `cachedPuzzleStore`) so a viral puzzle isn't re-rasterized per crawl.
- **De-duplicate OG tags:** make the Go injector the single source of truth —
  remove the `openGraph` block from [`src/app/layout.tsx`](../src/app/layout.tsx);
  point `frontendMetadataFor` at `.png`; add `og:image:width`/`height`/`secure_url`.
- Add a static `public/og-default.png` for non-puzzle routes.

**Out of scope:** CJK/non-Latin glyph fonts (note it; `gofont` is Latin-only — fine for v1).

**Acceptance criteria**
- `GET /api/og/puzzles/{id}.png` → 200, `image/png`, decodes to 1200×630.
- A served `/p/{id}` page has **exactly one** `og:image`, ending `.png`, with
  width/height, and **no** `image/svg+xml` og:image.

**Testing:** unit (decode PNG, assert bounds); handler test (content-type +
decode); injection test updated (`TestSharedPuzzleHTMLInjectsOGMetadata`,
`TestPuzzleOGImageEndpoint`); **post-deploy**: X card validator + a real iMessage/Slack paste.

**Rollback:** revert PR; the `.svg` endpoint can stay as a debug fallback.

**DoD:** a real link unfurls with a correct card on at least X, Slack, iMessage.

---

## Sprint 2 — The daily holds up day-to-day (Tier-3 Item B)

**Slice goal:** on a day with no published puzzle, players see an explicit "no
puzzle today" state (not a silently replayed yesterday), and a missing day does
not unfairly break a streak.

**Why now:** `TodaysPuzzle` runs `... where publish_date <= today ... limit 1`, so
an empty day silently serves the **previous** puzzle — and seed content runs out
within days, so this fires fast in production. This is the product risk made
concrete. (Full design in [`tier3-plan.md`](tier3-plan.md) Item B.)

**Decisions:** D2 (strict empty-state vs. rolling banner), D3 (skip gap days for
streaks), D1 (timezone), D5 (community numbering — cheap to fold in here).

**Scope (Option A / strict-daily, recommended)**
- Add `TodaysPuzzleStrict(ctx, today)` (exact `publish_date == today`) +
  `LatestPublished(ctx, today)` for the fallback link to the `PuzzleSource`
  interface; implement in `PostgresPuzzleStore`, `StaticPuzzleSource`,
  `cachedPuzzleStore`, and the test fake.
- Change `handleTodayPuzzle` to a **200 with a discriminated payload**:
  `{available:true, puzzle}` or `{available:false, latest:{...}|null}` (cleaner
  than overloading 404). Keep `dailyCacheControl()` (caps `s-maxage` near rollover).
- **Streak fairness (D3):** compute streaks over the set of *published daily
  dates* so no-puzzle days are non-breaking; update `computeStreak` +
  `SessionStreak` and add a gap-day unit test.
- **Frontend (same PR):** `fetchTodayPuzzle` returns the discriminated union;
  `VibeGridApp` renders a "No puzzle today" card (Come back tomorrow + CTAs:
  Archive, Create, and — if allowed — Play the latest). Keep the error state
  distinct from the empty state.
- **D5:** if community puzzles should read differently, add a "Community" label to
  share text / OG card without changing the shared `puzzle_number` sequence.

**Acceptance criteria**
- Empty today → `available:false` (+ `latest`); populated today → `available:true`.
- Player on an empty day sees the empty card, never a finished replayed grid.
- Completed 06-04 and 06-06 with nothing published 06-05 → streak intact.

**Testing:** backend store + handler tests for both shapes; streak gap-day test;
frontend parses both payloads + renders the empty state.

**Rollback:** flag `VIBEGRID_STRICT_DAILY`; off = current behavior.

**DoD:** verified in-browser for both an empty and a populated day; streak math
covered by tests; API contract change shipped frontend+backend together.

---

## Sprint 3 — Trust & durability (security + data the user never sees but depends on)

**Slice goal:** the data survives a bad day and the supply chain is watched —
backups are real, dependencies are scanned, retention is defined, abuse limits hold.

**Why now:** once real users (and UGC) exist, a lost DB or a known CVE is an
existential, not cosmetic, problem. Most of the hard surface (rate limiting,
moderation, parameterized queries, security headers, signed admin session) is
already built — this sprint *confirms and operationalizes* it.

**Decisions:** D7 (retention window).

**Scope**
- **Backups / PITR** enabled on Neon (provider toggle); document restore steps in
  the runbook and do one **test restore**.
- **Dependency scanning in CI:** `govulncheck ./...` for Go; `npm audit`
  (+ Dependabot or Renovate) for the frontend. Fail CI on high severity.
- **Data-retention policy (D7):** scheduled job (or documented manual step) to
  aggregate-then-purge anonymous attempts older than the chosen window; publish
  the policy text alongside `/privacy`.
- **Confirm & document** parameterized queries everywhere (they are) and the
  admin-auth threat model; add a load/abuse note for the Postgres rate limiter.
- Wire one **external error/log drain** (provider creds) so 5xx alerts in
  [`monitoring/alert-rules.yml`](../monitoring/alert-rules.yml) reach you.

**Acceptance criteria**
- A test restore brings back a known row.
- CI fails on a seeded high-severity vuln; passes clean otherwise.
- Attempts past the retention window are purged/aggregated by an automated step.

**Testing:** CI job demonstration; restore drill documented with timing.

**Rollback:** n/a (operational); retention job is dry-run-first.

**DoD:** backups proven by restore; CI scans live; retention automated; alerts
deliver to a real destination.

---

## Sprint 4 — Quality gates (lock in correctness so future slices move fast)

**Slice goal:** a regression in the play flow, the board, or the create form is
caught by CI before merge, and the transactional guess path is proven under real
concurrency.

**Why now:** the engine is already race-tested, but the *user-facing* flows lean
on manual checks. Hardening the test pyramid now pays back every later sprint.

**Scope**
- **Playwright** browser e2e for the three flows (play→win/lose, create→share,
  admin login→publish); run headless in CI. (`scripts/e2e.mjs`/`smoke.mjs` stay as
  the fast route-level layer.)
- **Component tests** (React Testing Library) for `VibeGridGame` (selection,
  4-mistake cap, one-away message, share-grid build) and `PuzzleDraftForm`
  (validation). Include a **responsive regression test** asserting the board is
  4 columns at mobile widths (guards the fix just shipped).
- **Load test** (k6 or vegeta) on `POST /api/guesses` to confirm the
  `SELECT … FOR UPDATE` + idempotency path holds under concurrent real traffic,
  not just the `-race` test.
- **Accessibility audit** (axe + Lighthouse) on the board, result, create, and
  admin; fix keyboard/focus/contrast issues; add an axe check to Playwright.

**Acceptance criteria**
- CI runs Playwright + component + axe and is green; a deliberately broken
  selection rule turns CI red.
- Load test report shows no lost/duplicated guesses and acceptable p95 latency.
- Lighthouse a11y ≥ 95 on the main flows.

**Testing:** the sprint *is* tests; verify each gate fails on an injected defect.

**DoD:** all new gates in CI; load + a11y reports checked into `docs/`.

---

## Sprint 5 — See the funnel & calibrate difficulty

**Slice goal:** you can see how many people land → start → finish → share, and
whether `EASY/MEDIUM/HARD` labels match real outcomes.

**Why now:** post-launch you need to *measure* to prioritize. The in-app puzzle
stats already exist; this adds product analytics (distinct from them) and turns
the existing stats into a calibration view.

**Scope**
- **Product analytics** (Plausible or PostHog — privacy-friendly, cookieless
  preferred): instrument page view, puzzle start, completion (win/lose), share
  click, create-submit. Keep it consistent with the privacy policy + retention.
- **Difficulty calibration view** in the Editor Desk: predicted (label) vs. actual
  (solve rate / median mistakes / median time) per puzzle, built on the existing
  `StatsStore` aggregates and `/api/admin/puzzles/{id}/analytics`.
- Add an **error boundary polish + loading skeletons** pass and a Lighthouse perf
  budget check (folds the remaining P2 frontend item in here).

**Acceptance criteria**
- The funnel is visible in the analytics dashboard for a real session.
- The admin calibration view flags a puzzle whose label disagrees with outcomes.

**Testing:** analytics events asserted in e2e (fire on the right actions); the
calibration query covered by a stats test.

**DoD:** funnel measurable; calibration visible; perf budget enforced in CI.

---

## Sprint 6 — Content sustainability (the #1 long-term product risk)

**Slice goal:** authoring a good puzzle every day is sustainable — you can preview
a draft as a player and schedule a queue, so the daily never goes dark by accident.

**Why now:** a daily game lives or dies on cadence. Today admins **publish blind**
(no preview) and there's **no queue/calendar** — Sprint 2's empty state makes a
gap *visible*, this sprint makes it *avoidable*.

**Scope**
- **Draft preview mode:** "play this draft as a player" before publishing
  (admin-only route rendering `VibeGridGame` against an unpublished draft;
  excluded from public today/archive — reuse the `origin`/`status` gating).
- **Scheduling / queue calendar** in the Editor Desk: see which dates have a
  published puzzle, draft depth, and gaps ahead; schedule a draft to a future
  `publish_date`. Pairs directly with Sprint 2's strict-daily contract.
- (Stretch, optional) **AI-assisted draft** — generate candidate groups/tiles for
  *human review* (never auto-publish). The sanctioned, product-safe use of AI here;
  not the cut difficulty bandit.

**Acceptance criteria**
- An admin can open a draft, play it fully, then publish — without it ever being
  publicly reachable pre-publish.
- The calendar shows gaps for the next N days and lets you schedule into them.

**Testing:** handler tests that drafts are preview-reachable for admins and 404 for
the public; scheduling sets `publish_date` and shows in the calendar.

**DoD:** no more blind publishes; a visible queue with gap warnings.

---

## Sprint 7 — Puzzle fairness (the sanctioned ML story)

**Slice goal:** before a puzzle goes live, it's checked for the unfair case where a
tile plausibly fits two groups — protecting the trust the daily ritual depends on.

**Why now:** structural validation (4×4, unique tiles) exists, but there's **no
alternate-solution / ambiguity check**. One unfair puzzle erodes trust fast. The
wrong-guess heatmap gives only *post-hoc* signal.

**Decisions:** D6 (manual red-herring list first vs. embeddings-assisted).

**Scope**
- **Phase 1 (manual):** let authors mark intended red-herrings and require an
  explanation per group; a pre-publish lint that flags obvious cross-group overlaps.
- **Phase 2 (embeddings-assisted, offline, data-light):** for each tile, compute
  similarity to each group's centroid; flag tiles whose top-2 group affinities are
  close (ambiguous) at publish time. Offline + cached; doesn't touch the live play
  path and doesn't break the shared-daily ritual — this is the documented,
  sanctioned ML use (distinct from the cut adaptive-difficulty bandit).

**Acceptance criteria**
- A deliberately ambiguous test puzzle is flagged pre-publish; a clean one passes.
- The check runs at authoring/publish time only, never on the player request path.

**Testing:** unit tests over crafted ambiguous vs. clean puzzles; verify no added
latency to `GET /api/puzzles/today`.

**DoD:** authors get an ambiguity warning before publishing; documented as the ML
narrative for the portfolio.

---

## Sprint 8 — (Optional) accounts & scale

Only if retention data (Sprint 5), cross-device streak demand, or traffic
justifies it. None of this blocks launch; each item is independently shippable.

- **Optional accounts (D4):** opt-in auth for cross-device streaks + integrity
  (cookie-bound attempts are gameable via incognito today). Keep guest play as
  the default path; accounts are additive, behind a flag, and never required to
  play the daily. Decide the provider and migration of existing cookie-bound
  streaks only when this moves from bet to build.
- **Scale-outs (only if traffic warrants):** Redis for shared rate limiting +
  cross-instance caching (replaces the Postgres fixed-window limiter at scale);
  read replica for heavy stats queries; CJK-capable OG font if non-Latin tiles
  appear; full favicon/app-icon set; cost model at expected scale.

**DoD per item:** anonymous play unaffected; new path behind a flag; load-tested.

---

## What this plan deliberately leaves out

Real-time multiplayer, the contextual-bandit adaptive-difficulty engine
(cut: needs traffic it won't have and breaks the shared-daily ritual),
payments/monetization, and native mobile. The async "play with friends" need is
already met by community puzzles + spoiler-safe share text, so live rooms,
presence, matchmaking, and chat stay out of v1.
