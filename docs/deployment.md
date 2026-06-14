# Deploying VibeGrid

This is the complete runbook to take VibeGrid from the repo to a live URL.

## What you are deploying

VibeGrid is **one Go binary** that serves both the web app and the API from a
single origin. The Docker build compiles the Next.js app to a static export,
embeds it into the Go binary via `go:embed` (along with the SQL migrations), and
produces a tiny distroless image that listens on `:8081` and runs as a non-root
user.

```
Browser ── HTTPS ──▶ container (Go binary: embedded Next export + /api/*) ──▶ Postgres
```

Because it is a normal long-running server process, it deploys to a **container
host**, not to a static/edge platform.

> ### Read this if you came from Cloudflare
> Cloudflare **Pages** (static) and **Workers** (V8/WASM isolates) cannot run an
> arbitrary Go binary, and this app is single-origin (the Go process serves the
> frontend itself — there is no separate static site to host). So a Cloudflare
> *compute* deployment of this architecture was never going to work; that is the
> most likely reason the old one died.
>
> You can still use Cloudflare, but only as **DNS + CDN/proxy in front of the
> container origin**. Two important caveats if you do:
> 1. Prefer **DNS-only (grey cloud)** to start. The app derives the client IP for
>    rate limiting from `Fly-Client-IP` / `X-Real-IP` (`clientIP` in
>    `backend/internal/vibegrid/rate_limits.go`). If you proxy through Cloudflare
>    (orange cloud), the real client IP arrives in `CF-Connecting-IP`, which the
>    app does **not** read yet — so per-IP limits would bucket all traffic under
>    Cloudflare's IPs. Either stay grey-cloud, or add `CF-Connecting-IP` to
>    `clientIP` before enabling the proxy.
> 2. The app already sets HSTS and forces HTTPS at the host; keep Cloudflare SSL
>    mode on **Full (strict)**, not Flexible, to avoid redirect loops.

The repo ships the production pieces: multi-stage `Dockerfile`, `fly.toml`,
`/healthz` + `/readyz` + `/metrics`, route-aware CSP, embedded migrations, and a
`vibegrid migrate` subcommand used as the release step.

---

## 0. Prerequisites

- A container host account. **Fly.io** is pre-configured here; Render or Railway
  work too (see "Other hosts" at the end).
- A **managed Postgres** provider: Neon, Supabase, Fly Postgres, or Railway.
- CLIs: `flyctl` (`brew install flyctl`), `psql` (to verify the DB), `openssl`
  (to generate secrets).
- The repo checked out, CI green on `main`.

---

## 1. Provision Postgres

1. Create a managed Postgres database.
2. Copy the **pooled** connection string if the provider offers one (Neon and
   Supabase do). Keep the direct/session string handy too — see the pooler note.
3. Note the connection cap. The app sets `SetMaxOpenConns(10)` **per machine**
   (`backend/internal/vibegrid/postgres_store.go`), so:
   `10 × (machine count) ≤ pooler/instance connection limit`.
4. Enable **daily backups and point-in-time recovery now**, before any real
   traffic. Record retention, RPO, and RTO.

> **Pooler gotcha (pgx + PgBouncer).** The app uses the `pgx` driver. If your
> connection string points at a **transaction-mode** pooler (Supabase's `6543`
> pgbouncer port, some Neon setups), prepared-statement caching can throw
> `prepared statement "..." already exists`. Fix by appending
> `?default_query_exec_mode=simple_protocol` to `DATABASE_URL`, or use the
> **session-mode / direct** connection string. This is a classic "it worked then
> randomly 500s" failure — set it correctly up front.

No manual schema step is needed: migrations run automatically as the release
command (step 3), and starter puzzles are seeded idempotently on every boot.

---

## 2. Generate secrets

```bash
# Admin browser login (pick a strong password) and the cookie signing secret:
VIBEGRID_ADMIN_PASSWORD="$(openssl rand -base64 24)"
VIBEGRID_ADMIN_SESSION_SECRET="$(openssl rand -hex 32)"
# Optional automation/API token (the web UI uses the password + cookie instead):
VIBEGRID_ADMIN_TOKEN="$(openssl rand -hex 32)"
echo "ADMIN_PASSWORD=$VIBEGRID_ADMIN_PASSWORD"   # save these in your password manager
echo "SESSION_SECRET=$VIBEGRID_ADMIN_SESSION_SECRET"
echo "ADMIN_TOKEN=$VIBEGRID_ADMIN_TOKEN"
```

If `VIBEGRID_ADMIN_PASSWORD` is set but `VIBEGRID_ADMIN_SESSION_SECRET` is not,
browser admin login is disabled (the app logs a warning). Set both.

---

## 3. Deploy to Fly.io

`fly.toml` is already configured: it builds the `Dockerfile`, runs
`/vibegrid migrate` as the release command (migrations land once, before any new
machine serves traffic), checks `/readyz`, forces HTTPS, sets
`VIBEGRID_SECURE_COOKIES=true` and `VIBEGRID_TIMEZONE=UTC`, and keeps one machine
warm.

```bash
# 1. Rename the app (edit `app = "..."` in fly.toml, or):
fly launch --copy-config --no-deploy        # creates the app, keeps fly.toml

# 2. Set secrets (never commit these):
fly secrets set \
  DATABASE_URL="postgres://USER:PASS@HOST:PORT/db?sslmode=require" \
  VIBEGRID_ADMIN_PASSWORD="$VIBEGRID_ADMIN_PASSWORD" \
  VIBEGRID_ADMIN_SESSION_SECRET="$VIBEGRID_ADMIN_SESSION_SECRET" \
  VIBEGRID_ADMIN_TOKEN="$VIBEGRID_ADMIN_TOKEN" \
  VIBEGRID_BLOCKED_TERMS="slur-one,slur-two"

# 3. Deploy:
fly deploy

# 4. Confirm:
fly status
fly logs
```

`fly deploy` runs the release command first; if `vibegrid migrate` fails (e.g.
bad `DATABASE_URL`), the release aborts and old machines keep serving. Watch
`fly logs` for `migrations applied` then `vibegrid listening`.

Set these only when needed:
- `VIBEGRID_ALLOWED_ORIGINS` — only if another origin must call the API directly.
  The normal single-container deploy is same-origin and needs no CORS.
- `VIBEGRID_TIMEZONE` — `fly.toml` sets `UTC`. This defines when "today" rolls
  over. **Pick one canonical zone and keep it stable** (changing it later shifts
  every daily puzzle's boundary). Note: the code default if unset is
  `Asia/Kolkata`, so always set it explicitly in production (`fly.toml` does).
  No cron is required for daily rollover: `/api/puzzles/today` computes the
  current date on request. If no editorial puzzle is published exactly for that
  date, the evergreen generator composes a deterministic date-specific board from
  the curated bank.

---

## 4. Domain + TLS

On Fly:

```bash
fly certs add vibegrid.example.com
fly certs show vibegrid.example.com     # prints the A/AAAA/CNAME records to create
```

Create those DNS records at your DNS provider (Cloudflare is fine **as DNS**).
If you use Cloudflare, re-read the grey-cloud / `CF-Connecting-IP` /
SSL-Full-strict caveats in the box at the top.

Then set the public URL for correct absolute links in the statically-exported
metadata (optional — the Go server already injects correct per-request OG tags,
but this keeps Next's metadata consistent). It is a **build-time** value, so it
must be baked during the image build, e.g. add to the Dockerfile web stage:

```dockerfile
ARG NEXT_PUBLIC_APP_URL
ENV NEXT_PUBLIC_APP_URL=$NEXT_PUBLIC_APP_URL
```

and `fly deploy --build-arg NEXT_PUBLIC_APP_URL=https://vibegrid.example.com`.

---

## 5. Backups, monitoring, alerting

1. **Backups/PITR** — enable on the managed Postgres (step 1). Run **one restore
   drill** into a scratch DB before sharing the link widely, and confirm the
   restore passes `vibegrid migrate` and `/readyz`.
2. **Uptime checks** (Better Stack / UptimeRobot / Pingdom / provider):
   - `GET /healthz` every 60s, alert after 2 failures (process liveness)
   - `GET /readyz` every 60s, alert after 2 failures (DB reachable)
   - `GET /` and `GET /api/puzzles/today` every 5m, alert on non-2xx
3. **Metrics** — scrape `GET /metrics` (Prometheus text). Import the templates in
   `monitoring/` (`prometheus.yml`, `alert-rules.yml`, `grafana-dashboard.json`)
   after replacing the placeholder domain. Beyond HTTP request/latency series,
   `/metrics` also exposes (see `docs/observability.md`):
   - connection pool: `vibegrid_db_open_connections`, `..._in_use_connections`,
     `..._idle_connections`, `..._wait_count_total`, `..._wait_seconds_total`
     (watch the wait series for pool saturation)
   - puzzle cache: `vibegrid_puzzle_cache_hits_total` / `..._misses_total` /
     `..._evictions_total` / `..._entries` (hit rate of the per-request cache)
4. **Log drain** — ship stdout/stderr to a durable store (Axiom, Datadog, Loki,
   Logtail). The server emits structured `slog` JSON with method, path, status,
   `duration_ms`, `client_ip`, `user_agent`. On Fly: `fly logs` for ad hoc, or a
   log-shipper for retention.
5. **Error tracking** — add Sentry (or similar) when you have credentials. Until
   then, alert from `/metrics` on 5xx rate and from logs on `panic`/`error`.

---

## 6. Verify production

Run through this after the first deploy:

- `https://<domain>/` loads today's puzzle.
- Play a guess, refresh, confirm the attempt persists (the `vibegrid_session`
  cookie should be present and `Secure`).
- `https://<domain>/create` creates a community puzzle; `/p/<id>` opens it.
- Report that puzzle from the sidebar, then in `/admin` (log in with the admin
  password) confirm it appears in the moderation queue; archive it, reopen
  `/p/<id>`, submit an appeal, and reinstate it from the queue.
- `curl -sI https://<domain>/readyz` → 200 (DB reachable).
- `curl -s https://<domain>/metrics | grep vibegrid_` shows the HTTP, pool, and
  cache series.
- `curl -s https://<domain>/robots.txt` advertises the sitemap; `/sitemap.xml`
  lists `/` and the live puzzle `/p/<id>` URLs (and **not** future-dated ones).
- `curl -sI https://<domain>/api/og/puzzles/<id>.svg` returns an image.
- Publish one editorial puzzle for today from `/admin` and confirm `/` serves it.

---

## 7. Rollback

```bash
fly releases                      # find the previous good release/image
fly deploy --image <previous-image>
```

Migrations are forward-only operationally — review every migration before deploy
and prefer additive, backward-compatible schema changes so a rollback of the
binary stays compatible with the migrated schema.

---

## 8. Local production smoke (optional, before deploying)

Reproduce the container build path locally:

```bash
npm ci && npm run build
mkdir -p backend/internal/frontend/out && cp -R out/. backend/internal/frontend/out
( cd backend && CGO_ENABLED=0 go build -o ../dist/vibegrid ./cmd/vibegrid )

# With a local Postgres (durable path):
DATABASE_URL="postgres://localhost/vibegrid?sslmode=disable" \
VIBEGRID_ADMIN_PASSWORD=dev VIBEGRID_ADMIN_SESSION_SECRET=devsecret \
VIBEGRID_ADDR=:8081 ./dist/vibegrid
# migrations: ./dist/vibegrid migrate  (run once; or rely on it via DATABASE_URL)

# Without a database (in-memory, non-durable, seed puzzles only):
VIBEGRID_ADDR=:8081 ./dist/vibegrid
```

Then open `http://localhost:8081/`, `/p/vibegrid-2026-06-02`, `/metrics`,
`/robots.txt`, `/sitemap.xml`.

Or just build the image: `docker build -t vibegrid . && docker run -p 8081:8081 vibegrid`.

---

## Environment variable reference

| Variable | Required | Where | Purpose |
|---|---|---|---|
| `DATABASE_URL` | Yes (prod) | secret | Postgres DSN. Unset ⇒ in-memory, non-durable. Mind the pooler gotcha (step 1). |
| `VIBEGRID_ADMIN_PASSWORD` | Yes (prod) | secret | Admin browser login. |
| `VIBEGRID_ADMIN_SESSION_SECRET` | Yes (prod) | secret | HMAC key for the admin session cookie. Required alongside the password. |
| `VIBEGRID_ADMIN_TOKEN` | Optional | secret | Legacy bearer token for automation/API. |
| `VIBEGRID_SECURE_COOKIES` | Yes (prod) | `fly.toml` | `true` ⇒ `Secure` cookies. Requires HTTPS. |
| `VIBEGRID_TIMEZONE` | Yes (prod) | `fly.toml` | Defines daily rollover. Set explicitly (code default is `Asia/Kolkata`). |
| `VIBEGRID_ADDR` | No | `fly.toml`/image | Listen address. Unset ⇒ binds `:$PORT` if the platform injects `PORT`, else `:8081`. |
| `VIBEGRID_MIGRATE_ON_BOOT` | No | env | `true` ⇒ apply migrations on startup (single-instance hosts with no release hook, e.g. Render free). Don't use multi-instance. |
| `VIBEGRID_ALLOWED_ORIGINS` | Only cross-origin | secret/env | Comma-separated browser origins for CORS. Not needed same-origin. |
| `VIBEGRID_BLOCKED_TERMS` | Optional | secret/env | Comma-separated blocked terms for community puzzles. |
| `NEXT_PUBLIC_APP_URL` | Recommended | **build arg** | Public URL baked into the frontend export. Build-time only. |

---

## Free tier (Render + Neon, $0)

For a portfolio link that should stay up without a bill, deploy the same
container on **Render's free Web Service** and point it at a **free Neon
Postgres**. The repo ships a [`render.yaml`](../render.yaml) blueprint for this.

Two things differ from the Fly path and are already handled:
- **No release hook on free.** The free plan can't run `/vibegrid migrate` as a
  pre-deploy step, so the blueprint sets `VIBEGRID_MIGRATE_ON_BOOT=true` — the
  server applies migrations on startup. Safe here because the free plan runs a
  single instance (nothing races). Do **not** use this on multi-instance hosts.
- **Port.** The binary binds `$PORT` when set (Render injects it), so
  `VIBEGRID_ADDR` is left unset in the blueprint.

Steps:
1. **Neon** — create a free project; copy the connection string. Use the
   **direct** (non-pooled) string to dodge the PgBouncer prepared-statement
   gotcha in step 1 above; at one instance × `SetMaxOpenConns(10)` you are well
   under the free connection cap. Ensure it ends with `?sslmode=require`.
2. **Render** — New ➜ **Blueprint**, point it at this repo. Render reads
   `render.yaml` and creates the service. In the dashboard set the three
   `sync: false` secrets: `DATABASE_URL` (from Neon),
   `VIBEGRID_ADMIN_PASSWORD`, `VIBEGRID_ADMIN_SESSION_SECRET` (generate per
   step 2 above). Deploy.
3. **Keep it warm (optional but recommended).** Render free **spins down after
   ~15 min idle** (first hit then wakes in ~30–60 s). A free uptime monitor
   (UptimeRobot / cron-job.org) hitting `GET /healthz` every ~10 min keeps it
   awake and doubles as your liveness alert — within the free monthly hours.
4. Verify with the **section 6** checklist against your `…onrender.com` URL.

> **Truly always-on for $0 (no cold start)?** Only by running your own
> always-free VM (e.g. Oracle Cloud Always Free) with Docker + this image — more
> setup, no spin-down. Or pay ~$7/mo for a Render paid instance, or ~$2–5/mo on
> Fly/Railway, to drop the cold start entirely.

---

## Other hosts (non-Fly)

Any host that runs a container works. You need to replicate three things from
`fly.toml`:
1. Apply migrations before serving — either run `/vibegrid migrate` **once per
   release** (release/pre-deploy hook), or set `VIBEGRID_MIGRATE_ON_BOOT=true`
   on single-instance deploys (see the free-tier section). Either way
   `DATABASE_URL` must be set.
2. Set the env/secrets from the table above (`VIBEGRID_SECURE_COOKIES=true`, a
   fixed `VIBEGRID_TIMEZONE`).
3. Point the platform health check at **`/readyz`** and route HTTPS to the app's
   port — it listens on `$PORT` if the platform injects one, else `8081`.

Render: use the [`render.yaml`](../render.yaml) blueprint (free), or a Web
Service from the `Dockerfile` + a pre-deploy `/vibegrid migrate` on paid.
Railway: deploy the `Dockerfile`, attach Postgres, and either add a release
command or set `VIBEGRID_MIGRATE_ON_BOOT=true`.
