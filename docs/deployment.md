# Deploying VibeGrid

Topology: **Vercel** (Next.js web) + **Fly.io** (Go API container) + **Neon**
(Postgres). The web app proxies `/api/*` to the API via a Next rewrite, so the
browser only ever talks same-origin.

```
Browser ──▶ Vercel (Next + /api/* rewrite) ──▶ Fly (Go API) ──▶ Neon (Postgres)
```

Everything below is repo-ready: the Dockerfile, `fly.toml` (migrations as a
release command, `/readyz` health check), security/cache headers, and a `migrate`
subcommand all exist. You supply accounts and run the commands.

## 1. Database (Neon)

1. Create a Neon project; copy the **pooled** connection string.
2. Keep it for the API secret below. Migrations apply automatically on first
   deploy (the Fly release command runs `vibegrid migrate`).

## 2. API (Fly.io)

```bash
# from the repo root (Dockerfile + fly.toml live here)
fly launch --copy-config --no-deploy      # pick a unique app name + region
fly secrets set \
  DATABASE_URL="postgres://USER:PASS@HOST/db?sslmode=require" \
  VIBEGRID_ADMIN_TOKEN="$(openssl rand -hex 32)" \
  VIBEGRID_ALLOWED_ORIGINS="https://YOUR-VERCEL-DOMAIN"
fly deploy
fly status          # confirm the machine is healthy (/readyz passing)
```

Non-secret config (`VIBEGRID_ADDR`, `VIBEGRID_TIMEZONE`,
`VIBEGRID_SECURE_COOKIES`) is in `fly.toml`. Note the API URL Fly prints
(e.g. `https://vibegrid-api.fly.dev`) for the next step.

## 3. Web (Vercel)

1. Import the GitHub repo into Vercel (framework auto-detected as Next.js).
2. Set environment variables:
   - `GO_BACKEND_URL` = the Fly API URL (e.g. `https://vibegrid-api.fly.dev`)
   - `NEXT_PUBLIC_APP_URL` = your Vercel/custom domain
3. Deploy. Add a custom domain if desired (Vercel handles HTTPS).
4. Set `VIBEGRID_ALLOWED_ORIGINS` on Fly to the **final** web domain and redeploy
   the API.

## 4. Verify in production (do not skip)

- `https://<web>/` loads today's puzzle.
- **Session cookie through the proxy:** play a guess, refresh — the attempt
  persists. The cookie is issued by the API and must survive the Vercel→Fly
  rewrite as a first-party cookie on the web domain. This is the most likely
  cross-service gotcha; test it explicitly.
- `https://<web>/admin` — paste `VIBEGRID_ADMIN_TOKEN`, create + publish a puzzle.
- `https://<web>/create` — build a community puzzle, open the `/p/<id>` link.
- `curl -i https://<api>/readyz` returns 200 (DB reachable).

## 5. CI/CD

CI (`.github/workflows/ci.yml`) already gates tests on every push/PR. To deploy
on merge to `main`:

- **Web:** connect the repo in Vercel — it auto-deploys `main` and gives preview
  deploys per PR.
- **API:** add a workflow step that runs `flyctl deploy --remote-only` with a
  `FLY_API_TOKEN` repo secret (`fly tokens create deploy`).

## Notes & alternatives

- **Migrations** run via the Fly `release_command`; instances also no-op migrate
  on boot (idempotent), so first deploy is safe.
- **Connection limits:** the API pool caps at 10 connections per instance — keep
  `min_machines_running` low or use Neon's pooler (the pooled string) so you stay
  under Postgres limits.
- **Caching:** `/api/puzzles/today` and `/api/puzzles` send `Cache-Control` for
  CDN/edge caching.
- **Simpler one-platform option:** Render or Railway can host the API, the web
  app, and Postgres together — one bill, no cross-origin/proxy subtlety. Fewer
  moving parts for a demo; the Vercel+Fly+Neon split shows more range.
- **Rollback:** `fly releases` / `fly deploy --image <previous>`; Vercel keeps
  every deployment for instant rollback.
