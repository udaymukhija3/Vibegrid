# VibeGrid Resume Points

## Link line

VibeGrid — semantic daily puzzle game | Go, Postgres, Next.js, TypeScript |
GitHub: https://github.com/udaymukhija3/Vibegrid | Demo: temporary public link
available while permanent Fly.io deployment is being wired

## Strong resume bullets

- Built VibeGrid, a daily semantic grouping puzzle with anonymous persistent
  attempts, spoiler-safe sharing, user-created puzzle links, admin publishing,
  moderation, and puzzle analytics.
- Reworked the app into a single Go web/API binary that serves an exported
  Next.js front end, validates guesses server-side, protects the answer key from
  the browser, and supports same-origin deployment.
- Implemented transaction-safe Postgres attempt storage with row locking and
  idempotent client guess IDs, preventing duplicate clicks or racing submits
  from corrupting mistakes, completion state, or stats.
- Added launch-grade user-generated-content controls: rate-limited puzzle
  creation, blocklisted terms, player reports, admin archive/reinstate actions,
  appeals, and moderation audit logging.
- Built operational scaffolding for a production demo, including embedded SQL
  migrations, `/healthz`, `/readyz`, `/metrics`, structured request logging,
  alert-rule templates, a Grafana starter dashboard, and Fly.io deployment
  configuration.
- Covered backend behavior with Go unit/integration tests, Postgres-backed race
  tests, frontend type/tests/build checks, and deploy smoke tests for play,
  archive, create/share, policy, OG image, and metrics routes.

## Shorter variants

- Built a Go/Postgres/Next.js daily puzzle app with server-authoritative game
  rules, persistent anonymous attempts, shareable UGC puzzles, admin publishing,
  moderation, and deploy-ready observability.
- Implemented idempotent, transaction-safe guess handling in Postgres so
  refreshes, double-clicks, and concurrent submissions preserve correct game
  state.
- Shipped production-facing backend features for a consumer game: migrations,
  health/readiness probes, metrics, rate limits, structured logs, CI, and
  smoke-test coverage.

## Honest caveats

- Permanent hosting is not live yet in this checkout; the current fastest demo
  path is a temporary public tunnel, with Fly.io + managed Postgres documented
  as the permanent path.
- The game is intentionally anonymous for v1, so one-attempt enforcement is
  cookie-bound rather than account-bound.
