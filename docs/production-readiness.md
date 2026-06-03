# VibeGrid — Production Readiness & Product Review

Status at time of writing: feature-complete locally (play / create+share / admin
author / stats), ~4,300 LOC, CI configured, **not deployed and not on GitHub**.
This document is the plan to take it to production grade, plus a critical review
of the business logic and feature set. It is a checklist to execute in a fresh
session — nothing here is implemented yet.

---

## Part 1 — Path to production grade

Ordered by priority. P0 = blocks a credible public launch; P1 = production
hardening; P2 = operational/legal; P3 = scale (only if traffic warrants).

### P0 — Source control & deploy (do first)

- [ ] Push to GitHub; protect `main`; require the CI workflow to pass before merge.
- [ ] Confirm CI is green in the cloud (it runs Go race tests against a Postgres
      service + frontend lint/typecheck/test/build).
- [ ] **Containerize the Go API** — multi-stage Dockerfile, `scratch`/`distroless`
      final image, non-root user, `CGO_ENABLED=0` static binary.
- [ ] **Deploy API** to Fly.io or Render (long-lived service); **deploy web** to
      Vercel. Wire the Next rewrite `GO_BACKEND_URL` to the deployed API URL.
- [ ] **Managed Postgres** (Neon or Supabase). Set `DATABASE_URL` as a platform
      secret. Mind connection limits: `SetMaxOpenConns(10)` × instances must stay
      under the DB/pooler cap — use the provider's connection pooler.
- [ ] **Migrations on deploy, not on every boot.** Today migrations run in
      `OpenDB` on startup. With >1 instance, concurrent `goose.Up` can race — move
      it to a one-off release/deploy step (or add an advisory lock) and have the
      app fail fast if the schema version is behind.
- [ ] Production env: `VIBEGRID_SECURE_COOKIES=true`, `VIBEGRID_ALLOWED_ORIGINS`
      = the real domain, a strong rotated `VIBEGRID_ADMIN_TOKEN`, fixed
      `VIBEGRID_TIMEZONE` (see "daily rollover" below).
- [ ] **Verify the cookie path through the proxy.** The Go API issues the
      `vibegrid_session` cookie; it reaches the browser via the Next `/api/*`
      rewrite. Confirm `Set-Cookie` propagates through the rewrite and the cookie
      is first-party on the production domain (test login/attempt persistence on
      the deployed site, not just locally). This is the most likely deploy gotcha.
- [ ] Custom domain + HTTPS (automatic on Vercel/Fly).
- [ ] Readiness vs liveness: `/healthz` exists (liveness); add a readiness check
      that pings the DB so deploys don't route traffic before the DB is reachable.

### P1 — Security

- [ ] **Security headers**: CSP, HSTS, `X-Content-Type-Options: nosniff`,
      `Referrer-Policy`, `frame-ancestors`. None are set today (add via Next
      `headers()` and/or Go middleware).
- [ ] **Rate limiting beyond create.** Only `POST /api/community/puzzles` is
      limited, and the limiter is in-memory per-instance (resets on deploy, not
      shared). Add limits to `POST /api/guesses`, and move the limiter to Redis
      for multi-instance correctness.
- [ ] **Input length caps.** Body size is capped, but tile text / category names
      have no max length — a community puzzle can carry very long strings (within
      64 KiB) that break the UI. Add per-field length limits in `Validate()`.
- [ ] **UGC moderation.** Community creation is unauthenticated free text →
      profanity/abuse/illegal content. Unlisted links limit spread but not
      creation. Add at minimum a profanity filter on submit and a report/takedown
      path. (See also Part 2.)
- [ ] **Dependency scanning**: `govulncheck` for Go and `npm audit` /
      Dependabot/Renovate in CI.
- [ ] **Admin auth threat model.** Single static bearer token is acceptable for a
      solo admin, but: never log it, rotate via secret, and add an admin **audit
      log** (who published what, when) for accountability.
- [ ] Confirm parameterized queries everywhere (they are) and document it.

### P1 — Reliability & data

- [ ] **Backups / PITR** on managed Postgres; document RPO/RTO.
- [ ] **Observability**: request-logging middleware (method, path, status,
      latency, request id); ship `slog` output to a log store; add error tracking
      (Sentry, frontend + backend); basic metrics (request rate / latency / 5xx)
      and uptime monitoring on `/healthz`.
- [ ] **Alerting** on 5xx spikes, DB connection failures, and deploy failures.
- [ ] Graceful shutdown (already implemented) — verify it cooperates with the
      platform's rolling-deploy drain.

### P1 — Testing & QA

- [ ] **End-to-end tests (Playwright)** for the core flows: play→solve, play→fail,
      create→share→play, admin draft→publish. This is the biggest test gap; the
      front end currently has only 2 trivial unit tests.
- [ ] Component tests (React Testing Library) for the game board and the draft form.
- [ ] Add `govulncheck` + `npm audit` + Playwright to CI.
- [ ] A quick load test (k6/vegeta) on `POST /api/guesses` to confirm the
      transactional path holds under real concurrency, not just the race test.
- [ ] Accessibility audit (axe/Lighthouse); the UI is decent but formalize it.

### P2 — Frontend production concerns

- [ ] **SEO & link previews.** Home and `/p/[id]` are client-rendered (fetch on
      mount) → no SSR content and a generic OG preview for shared links. Add
      proper per-route metadata, a real OG image (not the SVG mark), dynamic OG
      for shared puzzles, `sitemap.xml`, and `robots.txt`. Shared links are the
      growth surface — their preview matters.
- [ ] Product analytics (Plausible/PostHog) to measure the funnel (distinct from
      the in-app puzzle stats).
- [ ] Error boundary + nicer loading skeletons; Lighthouse/perf budget pass.

### P2 — Product, legal, ops

- [ ] **Privacy policy + ToS.** You set cookies and store attempts + IP (for rate
      limiting) → disclosure required; consider GDPR/cookie consent for EU users.
- [ ] Data retention policy for anonymous attempts.
- [ ] UGC content policy + report/takedown mechanism (ties to moderation above).
- [ ] Full favicon/app-icon set + social images (basic favicon done).
- [ ] Cost model at expected scale (Postgres + Fly + Vercel).

### P3 — Scale (only if traffic warrants)

- [ ] **Puzzle read path is the known scaling cliff.** Every public request loads
      *all* puzzles (3 queries assembling groups+tiles) via `PuzzleSource.Puzzles`.
      Fine for a small archive; add a per-id query + a short TTL cache (or CDN edge
      cache on the public payload) before the archive grows large.
- [ ] CDN/edge caching for `GET /api/puzzles/today` and `/api/puzzles`.
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
     cannot edit or remove their puzzle; it is permanent. Decide whether a
     delete/report path is needed (recommended).
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
