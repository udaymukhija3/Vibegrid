<div align="center">
  <img src="public/vibegrid-mark.svg" alt="VibeGrid" width="72" height="72" />
  <h1>VibeGrid</h1>
  <p><strong>Group the words. Guess the vibe.</strong></p>
  <p>A daily semantic grouping puzzle: 16 tiles, 4 hidden vibe-based categories, 4 mistakes, and a spoiler-safe result to share.</p>
  <p>
    <a href="https://github.com/udaymukhija3/Vibegrid/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/udaymukhija3/Vibegrid/actions/workflows/ci.yml/badge.svg" /></a>
  </p>
  <p>
    <a href="#demo-status">Demo status</a> ·
    <a href="#what-to-try">What to try</a> ·
    <a href="#architecture">Architecture</a> ·
    <a href="docs/resume-points.md">Resume bullets</a>
  </p>
</div>

<!-- Add a screenshot or GIF here once deployed, e.g. docs/demo.png -->

VibeGrid is a daily word-grouping game with real product plumbing: anonymous
persistent attempts, spoiler-safe sharing, user-created puzzle links, admin
publishing, moderation, analytics, and deploy-ready observability. It ships as
one Go binary that serves an exported Next.js front end plus the API.

## Demo status

This checkout is demo-ready locally, but it is not on a permanent public host as
of June 5, 2026. The intended production path is a single Fly.io web/API
container plus managed Postgres; see [docs/deployment.md](docs/deployment.md).

For a same-day public demo of the core game, run the app and share a temporary
tunnel to the Go server:

```bash
npm install
npm run dev
cloudflared tunnel --url http://localhost:8081
```

Then smoke-test the link:

```bash
npm run smoke:deploy -- --base-url https://<temporary-demo-url>
```

The temporary no-database demo supports play, refresh persistence, archive,
shared puzzle pages, OG images, health/readiness, and metrics. The create,
admin, moderation, and durable analytics flows require `DATABASE_URL` and are
covered by the Postgres path documented below.

## What to try

- Play the daily puzzle at `/`.
- Refresh after a guess; the anonymous attempt should persist.
- Open `/archive` and play a previous daily grid.
- On a database-backed run, create a puzzle at `/create`, then share the
  generated `/p/<id>` link.
- On a database-backed run, report a community puzzle, then review
  moderation/admin behavior from `/admin`.

## Highlights

- **Server-authoritative gameplay.** The browser receives tile text and ids but
  never group membership; the Go API validates every guess and only reveals a
  group after a correct submission. The answer key never reaches the client.
- **Transaction-safe, idempotent guesses.** Each guess runs inside a Postgres
  transaction that `SELECT … FOR UPDATE`-locks the attempt row. A unique
  `(attempt_id, client_guess_id)` constraint makes retries and double-clicks
  idempotent, so concurrent submissions can't corrupt mistake counts or
  completion state. (Proven by concurrency tests run under the race detector.)
- **Pluggable storage.** A `Store`/`PuzzleSource` interface backs both a Postgres
  implementation and an in-memory one, so the app runs (and tests) with or
  without a database.
- **Release-time migrations.** Embedded SQL migrations (goose) run through the
  `vibegrid migrate` subcommand before serving traffic.
- **User-generated content.** A public, rate-limited create flow lets anyone
  author a puzzle and share a play-by-link; community puzzles stay out of the
  daily rotation by design.
- **Moderation workflow.** Players can report community puzzles in-app; admins
  review reports, archive or reinstate puzzles, handle appeals, and keep an
  audit log in Postgres.
- **Analytics from first principles.** Completion stats (solve rate, median
  mistakes/time) and an admin wrong-guess heatmap are computed straight from the
  attempt and guess tables with SQL aggregates — no separate analytics pipeline.
- **Production observability.** `/healthz`, `/readyz`, `/metrics`, structured
  request logs, alert rules, and a starter Grafana dashboard are checked in for
  launch operations.
- **Tested and CI'd.** Go unit + integration tests (including a Postgres service
  container in GitHub Actions) and a typed, lint-clean front end.

## Tech stack

| Layer | Choice |
| --- | --- |
| API | Go (stdlib `net/http`, `log/slog`) |
| Database | Postgres (`pgx`, goose migrations) |
| Web | Next.js App Router, React, TypeScript |
| Styling | Tailwind CSS |
| Identity | Anonymous session cookies |

## Architecture

```
Browser ──▶ Go binary (embedded static Next export + /api/*) ──▶ Postgres
```

The Go service owns static file serving, game rules, sessions, attempts,
idempotency, puzzle authoring, and dynamic OG images. In development, Next still
uses a local `/api/*` rewrite to the Go API for fast UI iteration.

## Getting started

```bash
npm install
npm run dev
```

`npm run dev` starts the Go API on `http://localhost:8081` and the Next front end
on `http://localhost:3000`. Without a database it uses an in-memory store and a
seeded puzzle, so it runs out of the box.

Useful commands:

```bash
npm run dev:backend   # Go API only
npm run dev:web       # Next.js only
npm run migrate:backend
npm run test          # front-end tests
npm run test:backend  # Go tests
npm run typecheck
npm run build
```

## Database

Set `DATABASE_URL` to use the durable, transaction-safe Postgres path; run
migrations before starting the backend.

```bash
createdb vibegrid
DATABASE_URL="postgres://USER@localhost:5432/vibegrid?sslmode=disable" npm run migrate:backend
DATABASE_URL="postgres://USER@localhost:5432/vibegrid?sslmode=disable" npm run dev:backend
```

Integration tests run against a real Postgres when `TEST_DATABASE_URL` is set,
and are skipped otherwise:

```bash
createdb vibegrid_test
TEST_DATABASE_URL="postgres://USER@localhost:5432/vibegrid_test?sslmode=disable" go test -race ./backend/...
```

See [.env.example](.env.example) for all configuration
(`VIBEGRID_ADMIN_PASSWORD`, `VIBEGRID_ADMIN_SESSION_SECRET`,
`VIBEGRID_ADMIN_TOKEN`, `VIBEGRID_ALLOWED_ORIGINS`,
`VIBEGRID_SECURE_COOKIES`, …).

## Routes

- `/` — today's puzzle. `/archive` — past daily puzzles.
- `/create` — public puzzle builder; returns a shareable `/p/<id>` link.
- `/p/<id>` — play any puzzle by link.
- `/admin` — Editor Desk (author drafts, publish one puzzle per date, review
  reports and appeals). Requires a database and either
  `VIBEGRID_ADMIN_PASSWORD` + `VIBEGRID_ADMIN_SESSION_SECRET` for the web UI or
  `VIBEGRID_ADMIN_TOKEN` for legacy automation.
- `/policy`, `/terms`, `/privacy` — community rules and launch policy copy.

## Deployment

The repo is deploy-ready: a multi-stage Node+Go `Dockerfile`, `fly.toml` with
migrations as a release command and a `/readyz` health check, a `vibegrid
migrate` subcommand, embedded static frontend serving, route-aware CSP, and
security/cache headers. Target topology is Fly.io (single web/API container) +
managed Postgres. Step-by-step instructions are in
[docs/deployment.md](docs/deployment.md).

## Project docs

Product vision, daily puzzle operations, the decision register, the engineering
roadmap, and the tech stack rationale live in [`docs/`](docs/).

## License

[MIT](LICENSE)
