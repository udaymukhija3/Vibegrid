# VibeGrid agent handoff

Start here when picking up this repo in a new Codex/chat session.

## Current state

VibeGrid is a public portfolio web app: a daily semantic grouping puzzle with a
Go API/binary, exported Next.js frontend, Postgres-backed durable mode, admin
desk, community submissions, moderation, stats, and deployment scaffolding.

The app is intended to be recruiter-visible. Prioritize issues a recruiter,
senior engineer, or basic security reviewer could spot quickly from code or by
poking the deployed app. Keep claims honest: if a provider setting, public URL,
backup, or production secret is not verified in code/docs, treat it as manual
work rather than done.

## First commands

```bash
git status --short
npm run typecheck
npm run lint
npm test
go test ./backend/...
```

For larger backend/security changes also run:

```bash
go vet ./backend/...
go test -race ./backend/...
npm run build
git diff --check
```

`npm run test:e2e` binds a local port and may fail inside restricted sandboxes.
`npm audit --omit=dev --audit-level=high` needs registry network access. Run
those locally or in CI if the sandbox blocks bind/network access.

## Architecture map

- `backend/cmd/vibegrid` loads config, validates production guardrails, wires
  stores, runs migrations, starts cleanup loops, and serves the Go binary.
- `backend/internal/vibegrid` owns game rules, sessions, attempts, admin auth,
  puzzle stores, community creation, moderation, stats, metrics, SEO, and HTTP
  middleware.
- `backend/db/migrations` contains embedded SQL migrations used by
  `vibegrid migrate`.
- `src/app`, `src/components`, `src/lib`, and `src/types` own the Next.js UI,
  API clients, runtime response schemas, game board, admin desk, create flow,
  policy/privacy/terms pages, and tests.
- `scripts/smoke.mjs` and `scripts/e2e.mjs` are runtime verification entry
  points.
- `docs/deployment.md`, `docs/observability.md`, and
  `docs/production-readiness.md` are the operational handoff.

## Security/product decisions already implemented

- Production startup fails fast without the required database, secure cookies,
  metrics token, exact HTTPS public base URL, and safe CORS/proxy settings.
- Public canonical/robots/sitemap/social metadata use `VIBEGRID_PUBLIC_BASE_URL`,
  not request or forwarded host headers.
- Client IP headers are trusted only from configured proxy CIDRs.
- Admin browser login uses opaque, revocable, HttpOnly sessions; only token
  hashes are stored. Cookie-authenticated admin mutations require CSRF.
- `/metrics` is disabled unless `VIBEGRID_METRICS_TOKEN` is configured, and then
  requires a bearer token.
- Public API metrics labels are bounded; unknown API paths collapse to `/api/*`.
- Public puzzle reads and anonymous writes are rate-limited. Shared Postgres
  rate-limit failures fail closed on write paths.
- Guest attempt data expires after up to 30 days and is pruned.
- Community puzzles are created as `PENDING`; only admin approval makes them
  playable by direct `/p/<id>` link.
- The admin desk has queue-health coverage, exact board preview, publish,
  approval, archive/reinstate, analytics, reports, appeals, and audit logs.
- JSON mutation endpoints require `Content-Type: application/json`; request
  bodies and public identifiers are size/shape capped before storage work.

## Manual work that code cannot finish

- Push/stage/commit/PR if the worktree is still local and unstaged.
- Provision managed Postgres, enable backups/PITR, and perform one restore drill.
- Set production secrets: `DATABASE_URL`, `VIBEGRID_REQUIRE_DATABASE=true`,
  `VIBEGRID_SECURE_COOKIES=true`, `VIBEGRID_ADMIN_PASSWORD`,
  `VIBEGRID_ADMIN_SESSION_SECRET`, `VIBEGRID_METRICS_TOKEN`,
  `VIBEGRID_PUBLIC_BASE_URL`, timezone, and optional `VIBEGRID_ADMIN_TOKEN`.
- Configure `VIBEGRID_TRUSTED_PROXY_CIDRS` only from verified platform proxy
  source ranges. Never use `0.0.0.0/0` or `::/0`.
- Mount the Prometheus bearer token file expected by `monitoring/prometheus.yml`
  or adapt the scrape config in the target metrics platform.
- Configure uptime checks, log drain/error tracking, alert routing, branch
  protections, and Dependabot/Renovate in GitHub/provider UIs.
- Run `npm run test:e2e` and `npm audit --omit=dev --audit-level=high` in an
  environment with local-port binding and network access.

## How to continue safely

- Use `rg`/`rg --files` first for search.
- Preserve unrelated dirty-tree changes.
- Use `apply_patch` for file edits.
- After changing backend routes, update route metrics labels and tests.
- After changing public/admin API responses, update TypeScript Zod schemas.
- After changing launch/product state, update README and `docs/production-readiness.md`.
- Keep public-facing copy human and specific; avoid debug/API leakage in UI text.
