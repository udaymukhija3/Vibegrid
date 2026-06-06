# VibeGrid — Production Readiness & Product Review

Status as of June 5, 2026: feature-complete locally (play / create+share / admin
author / stats / moderation), single-container deploy scaffolding present, CI
configured, **not deployed to a permanent host yet**.
This document tracks what is already launch-ready and what still needs attention
after the first GitHub push.

---

## Part 1 — Path to production grade

Ordered by priority. P0 = blocks a credible public launch; P1 = production
hardening; P2 = operational/legal; P3 = scale (only if traffic warrants).

### P0 — Source control & deploy (do first)

- [ ] Push to GitHub; protect `main`; require the CI workflow to pass before merge.
- [ ] Confirm CI is green in the cloud (it runs Go race tests against a Postgres
      service + frontend lint/typecheck/test/build).
- [x] **Containerize the app** — Node+Go multi-stage Dockerfile builds the Next
      static export and embeds it in a non-root distroless Go runtime image.
- [ ] **Deploy single web/API container** to Fly.io or Render.
- [ ] **Managed Postgres** (Neon or Supabase). Set `DATABASE_URL` as a platform
      secret. Mind connection limits: `SetMaxOpenConns(10)` × instances must stay
      under the DB/pooler cap — use the provider's connection pooler.
- [x] **Migrations on deploy, not on every boot.** Fly runs `vibegrid migrate`
      as a release command; app startup connects and seeds without migrating.
- [ ] Production env: `VIBEGRID_SECURE_COOKIES=true`, strong
      `VIBEGRID_ADMIN_PASSWORD` + `VIBEGRID_ADMIN_SESSION_SECRET`, optional
      rotated `VIBEGRID_ADMIN_TOKEN` for automation, fixed `VIBEGRID_TIMEZONE`
      (see "daily rollover" below), and `VIBEGRID_ALLOWED_ORIGINS` only for
      non-same-origin clients.
- [ ] **Verify the cookie path in production.** The Go binary serves web and API
      same-origin; confirm `vibegrid_session` persists after a guess + refresh.
- [ ] Custom domain + HTTPS (automatic on Fly/most platforms).
- [x] Readiness vs liveness: `/healthz` exists for liveness and `/readyz` pings
      the DB when Postgres is configured.

### P1 — Security

- [x] **Security headers**: HSTS / `nosniff` / referrer / frame headers and
      route-aware CSP are set.
- [x] **Distributed rate limiting beyond create.** `POST /api/community/puzzles`
      and `POST /api/guesses` use a Postgres-backed fixed-window limiter when
      `DATABASE_URL` is configured, with in-memory fallback for local runs.
- [x] **Input length caps.** Body size and per-field lengths are capped for
      category names, explanations, and tiles.
- [x] **UGC moderation.** Community creation is unauthenticated free text, but
      the launch path now includes blocklisted terms, DB-backed reports, reason
      codes, an admin moderation queue, archive/reinstate actions, appeals, and
      an audit log.
- [ ] **Dependency scanning**: `govulncheck` for Go and `npm audit` /
      Dependabot/Renovate in CI.
- [x] **Admin auth threat model.** The web UI uses a password-backed signed
      HttpOnly session cookie. A static bearer token remains as an automation
      fallback, and moderation actions are audit logged.
- [ ] Confirm parameterized queries everywhere (they are) and document it.

### P1 — Reliability & data

- [ ] **Backups / PITR** on managed Postgres; the runbook is documented, but the
      provider setting must be enabled in the production account.
- [x] **Observability**: request-logging middleware (method, path, status,
      latency, client IP, user agent), `/metrics`, alert-rule templates, and a
      starter Grafana dashboard are checked in. External log drain/error
      tracking still needs provider credentials.
- [x] **Alerting templates** on scrape failure, 5xx spikes, slow requests, and
      no traffic are checked in under `monitoring/`.
- [ ] Graceful shutdown (already implemented) — verify it cooperates with the
      platform's rolling-deploy drain.

### P1 — Testing & QA

- [x] **End-to-end smoke tests** for production routes and create/share are in
      `scripts/e2e.mjs` and `scripts/smoke.mjs`. Playwright browser coverage is
      still a useful next layer.
- [ ] Component tests (React Testing Library) for the game board and the draft form.
- [ ] Add `govulncheck` + `npm audit` + Playwright to CI.
- [ ] A quick load test (k6/vegeta) on `POST /api/guesses` to confirm the
      transactional path holds under real concurrency, not just the race test.
- [ ] Accessibility audit (axe/Lighthouse); the UI is decent but formalize it.

### P2 — Frontend production concerns

- [x] **SEO & link previews.** `/p/[id]` uses Go fallback metadata injection and
      a dynamic puzzle OG image. Add `sitemap.xml` and `robots.txt` before launch.
- [ ] Product analytics (Plausible/PostHog) to measure the funnel (distinct from
      the in-app puzzle stats).
- [ ] Error boundary + nicer loading skeletons; Lighthouse/perf budget pass.

### P2 — Product, legal, ops

- [x] **Privacy policy + ToS.** Launch copy lives at `/privacy` and `/terms`.
- [ ] Data retention policy for anonymous attempts.
- [x] UGC content policy + report/takedown mechanism (ties to moderation above)
      lives at `/policy` and in the admin moderation queue.
- [ ] Full favicon/app-icon set (basic favicon + dynamic social image done).
- [ ] Cost model at expected scale (Postgres + Fly + Vercel).

### P3 — Scale (only if traffic warrants)

- [x] **Puzzle read path targeted.** Public today/archive/by-id paths use
      targeted Postgres queries with supporting indexes instead of loading the
      full archive.
- [x] CDN/edge-ready cache headers for `GET /api/puzzles/today` and
      `/api/puzzles`, capped near daily rollover.
- [ ] Redis for shared rate limiting and any caching across instances.
- [ ] Read replica for stats queries if they get heavy.

---

## Part 2 — Business logic & feature review (sharpening pass)

Decisions and gaps to resolve before/with launch. None are bugs in the strict
sense; they are under-specified behaviors and product risks.

### Decisions to make

1. **Daily rollover & timezone.** "Today" = latest published puzzle with
   `publish_date <= today` in a server timezone (default Asia/Kolkata). Pick one
   global launch timezone (UTC is the safe default) and document when "today"
   flips for users elsewhere. **Define the no-puzzle-today fallback:** today the
   logic silently serves the most recent published puzzle (i.e., replays
   yesterday's) if none is published for the date — decide if that is desired or
   if there should be an explicit "no puzzle today" state.

2. **Content sustainability — the #1 product risk for a daily game.** The daily
   model depends on a human authoring a good puzzle every day. There is no
   scheduling calendar, no draft queue depth, and **no preview** (admins publish
   blind — there is no "play this draft as a player" before publishing). Plan a
   publishing cadence, a calendar/queue view, and a preview mode. The deferred
   AI-assisted-draft (human-reviewed) idea is the relevant mitigation.

3. **Identity model.** "One attempt per puzzle" and the 4-mistake cap are
   **cookie-bound, not identity-bound** — clearing cookies / incognito gives a
   fresh attempt. Acceptable for a casual daily puzzle (Wordle/Connections are
   similar), but it means stats are gameable and real **streaks/cross-device
   continuity are impossible without accounts.** Decide: stay anonymous (fine for
   v1) or add optional accounts later for integrity + retention.

4. **Puzzle fairness check.** Structural validation exists (4×4, unique tiles)
   but there is **no alternate-solution / ambiguity check** — a puzzle can
   accidentally contain a tile that fits two groups, which feels unfair and
   erodes trust fast. The new wrong-guess heatmap gives post-hoc signal; consider
   a pre-publish check (manual red-herring list first; embeddings-assisted later —
   this is the sanctioned "ML story", not the cut difficulty bandit).

5. **Community/UGC model.** Confirm the intended shape:
   - Link-only (no public gallery) — currently true, and the right default given
     moderation cost. Keep it.
   - **No ownership/edit/delete** — unauthenticated creation means a creator
     cannot edit their puzzle. Removal happens through report/admin archive, and
     reinstatement happens through appeal/admin review.
   - **Shared puzzle numbering** — community and editorial share one
     `puzzle_number` sequence, so "VibeGrid #5" might be a community puzzle.
     Decide whether community puzzles need separate numbering/labels in share text.

6. **Stats credibility threshold.** "How others did" shows from the first player;
   with N=1–2 the percentages are noise. Add a minimum-N gate (e.g., hide until
   ~20 players) before showing percentages. Also note stats are gameable via
   cookie reset — fine as flavor, not as anything load-bearing.

7. **Difficulty is editorial guesswork.** `EASY/MEDIUM/HARD` is a static admin
   label, never validated against outcomes. The stats layer now makes
   **calibration possible** (predicted vs actual difficulty from solve rate /
   mistakes / time) — a natural next analytics feature.

### High-leverage feature opportunities

- **Emoji/color-block share** (Wordle/Connections style). Text share works, but
  the spoiler-safe colored-grid is *the* viral mechanic and is cheap — `colorIndex`
  already exists per group. Highest growth-per-effort item.
- **Streaks** (needs the identity decision above to be meaningful).
- **Draft preview mode** for admins (quality + the publishing-blind gap).
- **Difficulty calibration** view (builds on the stats layer).
- **Profanity filter + report** for UGC (also a launch safety requirement).

### Explicit non-goals (still correct)

Real-time multiplayer, the contextual-bandit adaptive difficulty (cut: needs
traffic it will not have and breaks the shared-daily ritual), payments/
monetization, and native mobile. The async "play with friends" need is already
met by community puzzles + share text.
