# Deploying VibeGrid

Topology: **Fly.io single container** + **managed Postgres**. The Docker build
exports the Next.js app, copies it into the Go module, and compiles one binary
that serves both the static frontend and `/api/*`.

```
Browser -> Fly.io (Go binary + embedded Next export) -> Postgres
```

The repo contains the production pieces: Node+Go multi-stage `Dockerfile`,
`fly.toml`, `/readyz`, route-aware CSP, embedded migrations, and a `vibegrid
migrate` subcommand used as the Fly release command.

## 1. Database

1. Create a managed Postgres database (Neon, Supabase, Fly Postgres, Render, or
   Railway are all fine for v1).
2. Copy the pooled connection string when your provider offers one.
3. Keep PITR/backups enabled before a public launch.

## 2. Fly.io App

```bash
fly launch --copy-config --no-deploy
fly secrets set \
  DATABASE_URL="postgres://USER:PASS@HOST/db?sslmode=require" \
  VIBEGRID_ADMIN_PASSWORD="<long-password>" \
  VIBEGRID_ADMIN_SESSION_SECRET="$(openssl rand -hex 32)" \
  VIBEGRID_ADMIN_TOKEN="$(openssl rand -hex 32)" \
  VIBEGRID_BLOCKED_TERMS="slur-one,slur-two"
fly deploy
fly status
```

`fly.toml` runs `/vibegrid migrate` once per release before the new machine
serves traffic. Runtime config in `fly.toml` sets `VIBEGRID_SECURE_COOKIES=true`
and `VIBEGRID_TIMEZONE=UTC`.

Set these only when needed:

- `VIBEGRID_ALLOWED_ORIGINS` if another origin must call the API directly. The
  normal single-container deployment is same-origin and does not need CORS.
- `VIBEGRID_ADMIN_TOKEN` is a legacy automation/API fallback. The `/admin` UI
  uses `VIBEGRID_ADMIN_PASSWORD` plus a signed HttpOnly session cookie.

## 3. Domain, Backups, and Monitoring

1. Add the production domain in the hosting provider and point DNS at the app.
   For Fly, add the cert, then create the DNS records it prints:

   ```bash
   fly certs add vibegrid.example.com
   fly certs show vibegrid.example.com
   ```

2. Enable managed Postgres daily backups and PITR. Record retention, RPO, RTO,
   and run one restore drill before broad sharing.
3. Configure uptime checks:
   - `GET https://<domain>/healthz`
   - `GET https://<domain>/readyz`
   - `GET https://<domain>/`
   - `GET https://<domain>/api/puzzles/today`
4. Import `monitoring/prometheus.yml`, `monitoring/alert-rules.yml`, and
   `monitoring/grafana-dashboard.json` into the chosen metrics stack after
   replacing the placeholder domain.
5. Add a platform log drain for stdout/stderr. The server logs request method,
   path, status, latency, client IP, and user agent as structured `slog` fields.
6. Add error tracking when credentials exist. Until then, alert from `/metrics`
   and logs on 5xx spikes and panic/error messages.

## 4. Verify Production

- `https://<app>/` loads today's puzzle.
- Play a guess, refresh, and confirm the attempt persists via the
  `vibegrid_session` cookie.
- `https://<app>/create` creates a community puzzle and `/p/<id>` opens through
  the Go static fallback.
- Report that community puzzle from the puzzle sidebar and confirm the report
  appears in `/admin`.
- Archive the report from `/admin`, open `/p/<id>`, submit an appeal, and confirm
  the appeal can reinstate the grid.
- `curl -i https://<app>/api/og/puzzles/<id>.svg` returns an SVG OG image.
- `curl -i https://<app>/readyz` returns 200 when Postgres is reachable.
- `curl -i https://<app>/metrics` exposes `vibegrid_up`,
  `vibegrid_http_requests_total`, and
  `vibegrid_http_request_duration_seconds`.
- `https://<app>/admin` accepts the admin password, creates a draft, publishes
  one puzzle for a date, and shows the moderation queue.

## 5. Local Production Smoke

```bash
npm run build
mkdir -p backend/internal/frontend/out
cp -R out/. backend/internal/frontend/out
GOCACHE="$PWD/.gocache" go build -o dist/vibegrid ./backend/cmd/vibegrid
VIBEGRID_ADDR=:8081 ./dist/vibegrid
```

Then open `http://localhost:8081`, `http://localhost:8081/p/vibegrid-2026-06-02`,
and `http://localhost:8081/api/og/puzzles/vibegrid-2026-06-02.svg`.

## 6. Rollback

Use `fly releases` to find the previous release and `fly deploy --image
<previous-image>` to roll back. Migrations are forward-only operationally, so
review every migration before deploy and prefer additive schema changes.
