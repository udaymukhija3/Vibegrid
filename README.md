<div align="center">
  <img src="public/vibegrid-mark.svg" alt="VibeGrid" width="72" height="72" />
  <h1>VibeGrid</h1>
  <p><strong>Group the words. Guess the vibe.</strong></p>
  <p>A daily semantic grouping puzzle: 16 tiles, 4 hidden vibe-based categories, 4 mistakes, and a spoiler-safe result to share.</p>
  <p>
    <a href="https://github.com/udaymukhija3/Vibegrid/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/udaymukhija3/Vibegrid/actions/workflows/ci.yml/badge.svg" /></a>
  </p>
  <p>
    <a href="#current-state">Current state</a> |
    <a href="#what-is-built">What is built</a> |
    <a href="#run-it-locally">Run locally</a> |
    <a href="#deployment-status">Deployment</a> |
    <a href="#for-portfolio-and-ai-agents">Portfolio notes</a>
  </p>
</div>

VibeGrid is a daily word-grouping game with real product plumbing behind it:
guest attempts, server-authoritative guessing, spoiler-safe sharing, community
puzzle links, admin publishing, moderation, analytics, and production-shaped
observability. It ships as one Go binary that serves an exported Next.js front
end and the API from the same origin.

This is not a static mockup. The core player loop, durable Postgres path, admin
desk, moderation queue, migrations, CI workflow, Docker image, Fly/Render config,
and monitoring templates are all present in the repo.

## Current State

- **Working local app:** `npm run dev` starts the Go API on
  `http://localhost:8081` and the Next.js frontend on `http://localhost:3000`.
- **No database required for a quick demo:** without `DATABASE_URL`, the backend
  uses in-memory attempts plus the same date-driven daily generator, so the game
  is playable immediately.
- **Guided public demo path:** `/demo` starts a fresh seeded room, and the room
  URL can be opened in a private window or second browser to show another guest
  attempt without sign-in or setup.
- **Postgres unlocks the full product:** durable attempts, stats, streaks,
  community puzzle creation, admin publishing, reports, appeals, moderation, and
  audit logs require `DATABASE_URL`.
- **Deployment scaffolding is ready:** the repo includes a multi-stage
  Node+Go `Dockerfile`, `fly.toml`, `render.yaml`, embedded SQL migrations,
  `/healthz`, `/readyz`, `/metrics`, structured logs, alert rules, and a starter
  Grafana dashboard.
- **Permanent public hosting is not recorded here yet:** treat this as a
  deploy-ready portfolio project until a real production URL, managed Postgres,
  backup/restore drill, and external monitoring are verified and documented.

## What Is Built

| Area | Current behavior |
| --- | --- |
| Player game | Daily 4x4 grid, Standard guided mode, Hard mode, one-away feedback, 4-mistake terminal failure, elapsed timer, and shareable spoiler-safe result grid. |
| Game rules | Go validates guesses server-side. The browser receives tile ids/text and vibe hints, but never receives tile-to-group answer mappings. |
| Guest persistence | Public play uses a guest session cookie. Attempts survive refreshes; with Postgres they are durable beyond process restarts. |
| Daily content | Explicitly scheduled editorial puzzles win for their publish date. Empty days are filled by a deterministic evergreen generator that composes a date-specific board from curated bank groups, so the daily keeps changing without a cron job or manual authoring every night. |
| Archive/share links | Published editorial puzzles appear in `/archive`; any playable puzzle can be opened at `/p/<id>`. |
| Community puzzles | `/create` lets users build a 4x4 puzzle from scratch or from starter packs and receive a shareable `/p/<id>` link. Requires Postgres. |
| Admin desk | `/admin` supports password-backed admin login, draft creation, publish-by-date, archive/reinstate, and per-puzzle analytics. Requires Postgres and admin env vars. |
| Moderation | Players can report puzzles without logging in; admins can review reports, archive/reinstate content, handle appeals, and inspect an audit log. Requires Postgres. |
| Analytics | Public completion stats are computed from attempts/guesses and shown only after the player finishes and enough players exist; admins also get wrong-guess heatmaps. |
| Operations | Health/readiness probes, Prometheus metrics, structured request logs, route-aware security headers, rate limits, body caps, Docker/Fly/Render config, and deploy smoke scripts are checked in. |

## Architecture

```text
Browser -> Go binary (embedded Next.js static export + /api/*) -> Postgres
```

In development, Next.js rewrites `/api/*` to the Go backend for fast UI
iteration. In production, the Go binary serves the static frontend and API from
one origin, which keeps cookies, CORS, and deployment simpler.

Important implementation points:

- `backend/internal/vibegrid` owns game rules, sessions, attempts, puzzle stores,
  admin routes, moderation, stats, metrics, and SEO helpers.
- `backend/db/migrations` contains embedded SQL migrations. `vibegrid migrate`
  is the release-time migration command.
- `src/app`, `src/components`, `src/lib`, and `src/types` contain the Next.js
  frontend, API client, runtime response validation, game UI, admin desk, create
  flow, archive, policy, privacy, and terms pages.
- `backend/internal/frontend` embeds the static Next.js export into the Go
  binary for the single-container deploy path.
- `scripts/dev.mjs`, `scripts/e2e.mjs`, and `scripts/smoke.mjs` are the local
  development and smoke-test entry points.

## Run It Locally

Install dependencies and start both servers:

```bash
npm install
npm run dev
```

Open `http://localhost:3000`.

Useful commands:

```bash
npm run dev:backend   # Go API only on :8081
npm run dev:web       # Next.js only on :3000
npm run migrate:backend
npm run test          # Vitest frontend tests
npm run test:backend  # Go backend tests
npm run typecheck
npm run build
```

### Run with Postgres

Use Postgres for the durable path:

```bash
createdb vibegrid
DATABASE_URL="postgres://USER@localhost:5432/vibegrid?sslmode=disable" npm run migrate:backend
DATABASE_URL="postgres://USER@localhost:5432/vibegrid?sslmode=disable" npm run dev:backend
```

Then run `npm run dev:web` in another terminal and open
`http://localhost:3000`.

Integration tests use a real Postgres database when `TEST_DATABASE_URL` is set:

```bash
createdb vibegrid_test
TEST_DATABASE_URL="postgres://USER@localhost:5432/vibegrid_test?sslmode=disable" go test -race ./backend/...
```

See [.env.example](.env.example) for environment variables such as
`DATABASE_URL`, `VIBEGRID_ADMIN_PASSWORD`,
`VIBEGRID_ADMIN_SESSION_SECRET`, `VIBEGRID_ADMIN_TOKEN`,
`VIBEGRID_ALLOWED_ORIGINS`, `VIBEGRID_SECURE_COOKIES`,
`VIBEGRID_TIMEZONE`, and `VIBEGRID_MIGRATE_ON_BOOT`.

## Routes To Try

- `/` - today's puzzle.
- `/demo` - starts a guided seeded demo room.
- `/demo/<room>` - plays that seeded room; open the same link in a private
  window or second browser to simulate another guest.
- `/archive` - previous editorial daily puzzles.
- `/create` - public puzzle builder; returns a shareable `/p/<id>` link.
- `/p/<id>` - play a puzzle by link.
- `/admin` - Editor Desk for drafts, publishing, archive/reinstate, analytics,
  reports, appeals, and moderation audit logs.
- `/policy`, `/terms`, `/privacy` - community rules and launch policy copy.
- `/healthz`, `/readyz`, `/metrics`, `/robots.txt`, `/sitemap.xml` - operational
  and SEO endpoints served by the Go binary.

## Deployment Status

The intended production shape is a single web/API container plus managed
Postgres. The checked-in deploy paths are:

- **Fly.io:** `fly.toml` uses the Dockerfile and runs `vibegrid migrate` as the
  release command before traffic is served.
- **Render + Neon:** `render.yaml` supports a free-tier portfolio deployment.
  Because Render free has no release hook, it uses `VIBEGRID_MIGRATE_ON_BOOT=true`
  for a single-instance boot migration.
- **Any container host:** build the Dockerfile, set the env vars from
  `.env.example`, attach Postgres, run migrations once per release, and route
  health checks to `/readyz`.

For a temporary public demo of the no-database game loop, run the app and tunnel
the Go server:

```bash
npm install
npm run dev
cloudflared tunnel --url http://localhost:8081
```

Share `/demo` for a fresh guided walkthrough. If you want to show a second
viewer, copy the generated `/demo/<room>` URL and open it in a private window or
another browser; it uses the same seeded room with a separate guest attempt.

Then smoke-test the temporary URL:

```bash
npm run smoke:deploy -- --base-url https://<temporary-demo-url>
```

Use [docs/deployment.md](docs/deployment.md) for the full production runbook.
Before describing it as production live, verify the real host/domain, secure
guest cookie persistence, managed Postgres backups/PITR, one restore drill,
external uptime checks, and log/metrics retention.

## Verification

The local and CI verification ladder is:

```bash
npm run test
npm run test:backend
npm run typecheck
npm run build
```

GitHub Actions is configured to run:

- Go formatting, `go vet`, and `go test -race ./...` against a Postgres service.
- `npm ci`, lint, typecheck, Vitest, and the static Next.js build.

The deploy smoke script checks the runtime routes that matter for a public demo:
play, archive, create/share where supported, policy pages, health/readiness,
metrics, robots/sitemap, and OG metadata.

## Known Gaps

These are the main things not to overstate:

- No permanent production URL is documented in the repo yet.
- Public player accounts, OAuth, leaderboards, cross-device identity, and account
  recovery are intentionally not implemented for v1.
- Real-time multiplayer, live rooms, matchmaking, presence, and chat are out of
  scope; the multiplayer loop is async sharing and community puzzle links.
- AI-assisted puzzle generation is not part of the shipped app. Any future AI
  work should be admin-reviewed draft assistance, not automatic publishing.
- Shared puzzle metadata exists, but the current OG image endpoint is SVG. PNG
  social cards for more reliable unfurls are a launch polish item.
- External production ops still need provider setup: managed Postgres backups,
  restore drill, dependency scanning, log drain, uptime monitor, and real alert
  routing.

## For Portfolio And AI Agents

If you are using this README to update a portfolio website, use the framing below.
It is intentionally specific and avoids claiming a public production launch.

**Short portfolio title:** VibeGrid - daily semantic grouping puzzle.

**One-sentence summary:** Built a Go/Postgres/Next.js daily puzzle app with
server-authoritative game rules, guest attempt persistence, shareable community
puzzles, admin publishing, moderation, analytics, CI, Docker deployment
scaffolding, and Prometheus-style observability.

**Good tags:** Go, Postgres, Next.js, TypeScript, Tailwind CSS, Docker, CI,
observability, product engineering, moderation tooling.

**Strong proof points to mention:**

- Server-side validation prevents the browser from receiving the answer key.
- Postgres attempt storage uses transactional/idempotent guess handling so
  refreshes, retries, and double-clicks do not corrupt game state.
- The project includes a full product surface, not only gameplay: create/share,
  admin publishing, moderation, reports/appeals, analytics, policies, and ops
  endpoints.
- The deploy path is single-container and same-origin: the Go binary serves the
  exported Next.js frontend and API.
- The repo contains production-oriented scaffolding: migrations, health/readiness
  probes, metrics, structured logs, security headers, rate limits, CI, Docker,
  Fly/Render config, and smoke tests.

**Do not claim unless a later commit proves it:**

- A permanent public production URL is live.
- Real users or production traffic exist.
- Public accounts, OAuth, native mobile, live multiplayer, or leaderboards exist.
- AI is generating or publishing puzzles.
- Backups, restore drills, external monitoring, or alert routing are already
  configured in a provider account.

For resume wording, see [docs/resume-points.md](docs/resume-points.md). For the
remaining launch plan, see [docs/launch-sprint-plan.md](docs/launch-sprint-plan.md)
and [docs/production-readiness.md](docs/production-readiness.md).

## Project Docs

- [Deployment runbook](docs/deployment.md)
- [Production readiness review](docs/production-readiness.md)
- [Launch sprint plan](docs/launch-sprint-plan.md)
- [Observability runbook](docs/observability.md)
- [Product vision](docs/product-vision.md)
- [Decision register](docs/decision-register.md)
- [Tech stack notes](docs/tech-stack.md)
- [Resume points](docs/resume-points.md)

## License

[MIT](LICENSE)
