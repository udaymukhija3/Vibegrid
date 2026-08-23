# Deploying VibeGrid

VibeGrid deploys as one long-running Go binary serving the embedded static
Next.js export and same-origin `/api/*` routes, backed by managed Postgres.

```text
browser ──HTTPS──▶ Go container (embedded web + API + migrations) ──▶ Postgres
```

It is not a static Pages/Workers application. Cloudflare can proxy/DNS in front
of the container, but its static/edge runtimes do not replace the Go process.

## Readiness boundary

The checked-in Docker/Fly/Render configuration is deployment scaffolding. Do
not claim a production launch until the final domain, provider backups/PITR,
restore drill, external checks, alert delivery, and deployed SHA are verified.

## 1. Provision Postgres

Use a managed provider such as Neon, Supabase, Fly Postgres, or Railway.

1. Create a database and note its connection cap.
2. Prefer a provider-supported pooled URL if compatible. The app caps open
   connections per instance; total instance pools must stay below the provider
   limit.
3. For transaction-mode PgBouncer, add the driver’s simple-protocol option if
   the provider requires it; otherwise use a session/direct connection.
4. Enable backups and PITR before inviting a crew.
5. Run a scratch restore and record actual RPO/RTO. The runbook is
   `docs/runbooks/database-restore.md`.

## 2. Generate secrets

```bash
VIBEGRID_ADMIN_PASSWORD="$(openssl rand -base64 24)"
VIBEGRID_ADMIN_SESSION_SECRET="$(openssl rand -hex 32)"
VIBEGRID_ADMIN_TOKEN="$(openssl rand -hex 32)"
VIBEGRID_METRICS_TOKEN="$(openssl rand -hex 32)"
```

Store them in the provider/password manager, never the repository. Browser
admin login needs both password and session secret. The token is optional for
automation; metrics token is required in production.

## 3. Required production environment

| Variable | Requirement |
| --- | --- |
| `DATABASE_URL` | Managed Postgres URL. |
| `VIBEGRID_REQUIRE_DATABASE` | `true`; crew service must not silently degrade. |
| `VIBEGRID_SECURE_COOKIES` | `true` behind HTTPS. |
| `VIBEGRID_ADMIN_PASSWORD` | Strong secret for the single-operator board room. |
| `VIBEGRID_ADMIN_SESSION_SECRET` | Independent random secret. |
| `VIBEGRID_METRICS_TOKEN` | Bearer secret protecting `/metrics`. |
| `VIBEGRID_PUBLIC_BASE_URL` | Exact final HTTPS origin, no path/query. |
| `VIBEGRID_TIMEZONE` | `UTC`; this is the ratified global phase rollover. |
| `VIBEGRID_TRUSTED_PROXY_CIDRS` | Only documented platform proxy source ranges, or empty. Never `0.0.0.0/0`/`::/0`. |

Optional:

- `VIBEGRID_ADMIN_TOKEN` for non-browser administration.
- `VIBEGRID_BLOCKED_TERMS` for prompt/fragment/title safety screening.
- `VIBEGRID_ALLOWED_ORIGINS` only for an intentional non-same-origin client.
- `VIBEGRID_OPERATOR_WEBHOOK_URL` and bot-verification keys only if those legacy
  operational/community paths are still in use.
- `VIBEGRID_MIGRATE_ON_BOOT=true` only on a single-instance host without a
  release hook. Prefer a release migration.

`NEXT_PUBLIC_APP_URL` affects build-time frontend metadata. The Go server’s
canonical, robots, sitemap, and injected metadata use
`VIBEGRID_PUBLIC_BASE_URL`, which is the runtime authority.

## 4. Migrations

Run once per release before new instances serve traffic:

```bash
DATABASE_URL="postgres://..." npm run migrate:backend
```

Fly uses the release command in `fly.toml`. Render free can use
`VIBEGRID_MIGRATE_ON_BOOT=true` only while exactly one instance boots. Migration
18 adds immutable daily boards, submissions, and votes; it is additive and
leaves legacy tables for old `/p` links.

## 5. Build and run the production artifact

```bash
docker build -t vibegrid:local .
docker run --rm -p 8081:8081 \
  -e DATABASE_URL="postgres://..." \
  -e VIBEGRID_REQUIRE_DATABASE=true \
  -e VIBEGRID_SECURE_COOKIES=true \
  -e VIBEGRID_PUBLIC_BASE_URL="https://vibegrid.example.com" \
  -e VIBEGRID_TIMEZONE=UTC \
  -e VIBEGRID_ADMIN_PASSWORD="$VIBEGRID_ADMIN_PASSWORD" \
  -e VIBEGRID_ADMIN_SESSION_SECRET="$VIBEGRID_ADMIN_SESSION_SECRET" \
  -e VIBEGRID_METRICS_TOKEN="$VIBEGRID_METRICS_TOKEN" \
  vibegrid:local
```

The final image is distroless and non-root. The binary listens on `:8081` by
default and serves both app and API.

## 6. Fly

`fly.toml` builds the Dockerfile, runs `/vibegrid migrate` as a release command,
forces HTTPS, sets UTC, and checks `/readyz`.

```bash
fly launch --copy-config --no-deploy
fly secrets set \
  DATABASE_URL="postgres://..." \
  VIBEGRID_REQUIRE_DATABASE="true" \
  VIBEGRID_ADMIN_PASSWORD="$VIBEGRID_ADMIN_PASSWORD" \
  VIBEGRID_ADMIN_SESSION_SECRET="$VIBEGRID_ADMIN_SESSION_SECRET" \
  VIBEGRID_ADMIN_TOKEN="$VIBEGRID_ADMIN_TOKEN" \
  VIBEGRID_METRICS_TOKEN="$VIBEGRID_METRICS_TOKEN" \
  VIBEGRID_PUBLIC_BASE_URL="https://vibegrid.example.com"
fly deploy
fly status
fly logs
```

## 7. Render

`render.yaml` describes the web service and expected environment. On plans with
a release hook, use it for migrations. On a single free instance without one,
boot migration is the pragmatic fallback. Do not scale multiple boot-migrating
instances concurrently.

## 8. DNS, TLS, and proxies

- Point the final domain at the container host and require HTTPS.
- Set `VIBEGRID_PUBLIC_BASE_URL` to that exact origin before launch.
- With Cloudflare, use Full (strict), not Flexible.
- Trust forwarding headers only from verified source CIDRs. If stable ranges are
  unavailable, leave the allowlist empty and enforce client limits at the edge.
- Verify rate-limit identity in logs with two actual client networks.

## 9. Production verification

Automated:

```bash
npm run smoke:deploy -- --base-url https://<domain>
npm run smoke:deploy -- --base-url https://<domain> --mutate \
  --metrics-token "$VIBEGRID_METRICS_TOKEN"
```

The mutating smoke creates a temporary crew, submits a card twice with the same
client replay id, and proves only one card exists. It skips durable mutation
only when the environment explicitly has no database; production must not skip.

Manual checklist:

- `/` completes make → house judge → practice reveal on 320px and desktop.
- `/api/vibes/today` returns one prompt, 16 unique `{id,text}` fragments in a
  4×4 practice projection, UTC
  date, and no groups/answers/difficulty/mistake fields.
- `/api/vibes/practice/0` and `/api/vibes/practice/12` return distinct,
  deterministic 4×4 Unlimited deals with immutable public cache headers.
- Create a crew, join from two isolated browser profiles, and submit three cards.
- With 4 members verify 3×4; join a fifth before a new dated board opens and
  verify 4×4; join again after open and verify the frozen count does not change.
- Use backdated fixtures or a test clock to verify blind authors on judge and
  named authors/ties/quiet labels on reveal.
- Rotate invite: old link fails for a newcomer; existing member access remains.
- `/admin` authenticates over HTTPS, creates a future board, and rejects a
  duplicate frozen date.
- `/robots.txt` disallows admin; `/sitemap.xml` includes `/crews` but no `/crew/`
  or `/p/` links.
- `/metrics` rejects a wrong token and works with the configured token.
- Secure cookies persist after refresh on the final same-origin domain.

## 10. Monitoring and alerts

External checks:

- `/healthz` every minute for process liveness.
- `/readyz` every minute for database reachability.
- `/` and `/api/vibes/today` every five minutes for product availability.

Scrape `/metrics` with the bearer token and alert on 5xx, latency, DB pool wait,
and readiness. Existing cache/outbox metrics still cover compatibility systems;
add product-funnel metrics only after a privacy review. Ship structured logs to
a durable destination and test one routed alert.

## 11. Rollback

1. Stop promotion if the release migration fails; old instances should continue.
2. Re-promote the prior immutable image tag/SHA.
3. Migration 18 is additive, so the prior binary can ignore its tables.
4. Do not run a destructive down migration against crew data during incident
   rollback.
5. Rerun health/readiness and non-mutating smoke, then inspect logs/metrics.

Record the rollback rehearsal before broad launch.

## 12. Local no-database demonstration

```bash
npm install
npm run dev
```

The public practice round works. Crew APIs return an explicit `503`; this mode
does not prove durable multiplayer. Use Postgres for portfolio or beta demos of
the actual product.
