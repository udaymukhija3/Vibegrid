# Observability Runbook

VibeGrid exposes three operational endpoints:

- `/healthz` - process liveness.
- `/readyz` - readiness; checks Postgres when `DATABASE_URL` is set.
- `/metrics` - bearer-protected Prometheus text metrics for request count,
  status, and latency, plus connection-pool, puzzle-cache, and notification-outbox series (see below).

## Public Uptime Checks

Create external checks from Better Stack, UptimeRobot, Pingdom, Grafana Cloud, or
the hosting provider:

- `GET https://<domain>/healthz` every 60 seconds, alert after 2 failures.
- `GET https://<domain>/readyz` every 60 seconds, alert after 2 failures.
- `GET https://<domain>/` every 5 minutes, alert on non-2xx or timeout.
- `GET https://<domain>/api/puzzles/today` every 5 minutes, alert on non-2xx.

Use `/readyz` for deploy routing and database incidents. Use `/healthz` for
process-level restarts.

## Metrics

Import the files under `monitoring/` into the chosen metrics stack:

- `monitoring/prometheus.yml` - scrape config for `/metrics`.
- `monitoring/alert-rules.yml` - launch alerts for scrape failure, 5xx rate, p95
  latency, and no observed traffic.
- `monitoring/grafana-dashboard.json` - dashboard starter for traffic, errors,
  and latency by route.

Replace `vibegrid.example.com` with the real domain before importing. Mount a
root-readable file at `/etc/prometheus/secrets/vibegrid_metrics_token` containing
the app's `VIBEGRID_METRICS_TOKEN` (with no surrounding quotes). The checked-in
Prometheus example sends it as a Bearer token; a request without it receives
`401`.

### Exposed series

HTTP (always):

- `vibegrid_http_requests_total{method,route,status}`
- `vibegrid_http_request_duration_seconds{...}` (histogram)
- `vibegrid_http_metric_series_dropped_total` (counter; alert if it rises)

Route labels are a fixed, bounded set. Unknown API routes collapse to `/api/*`
and the process keeps at most 256 HTTP metric series, so attacker-controlled
paths cannot grow metrics memory without bound.

Connection pool (when `DATABASE_URL` is set) — watch the wait series for pool
saturation under load:

- `vibegrid_db_open_connections`, `vibegrid_db_in_use_connections`,
  `vibegrid_db_idle_connections` (gauges)
- `vibegrid_db_wait_count_total`, `vibegrid_db_wait_seconds_total` (counters)

Puzzle content cache (when `DATABASE_URL` is set) — hit rate validates the
per-guess read-path optimization:

- `vibegrid_puzzle_cache_hits_total`, `vibegrid_puzzle_cache_misses_total`,
  `vibegrid_puzzle_cache_evictions_total` (counters)
- `vibegrid_puzzle_cache_entries` (gauge)

Transactional notification outbox (when `DATABASE_URL` is set):

- `vibegrid_notification_outbox_pending`, `vibegrid_notification_outbox_retrying`
- `vibegrid_notification_outbox_dead`
- `vibegrid_notification_outbox_oldest_pending_seconds`

Alert when the oldest pending event exceeds the operator response target and
immediately when a dead event appears. A missing webhook does not roll back a
product mutation; the event remains durable until delivery is configured.

## Logs

The Go server writes structured `slog` request logs with method, path, status,
duration, client IP, and user agent. Configure the host's log drain to ship
stdout/stderr to a durable log store such as Axiom, Datadog, Grafana Loki,
Honeycomb, Logtail, or the platform log service.

Minimum saved fields:

- timestamp
- message
- method
- path
- status
- duration_ms
- client_ip
- user_agent

Create saved searches for:

- `status >= 500`
- `path = /api/community/puzzles and status >= 400`
- `path starts with /api/admin and status in 401,403,500`
- `message contains panic or error`

## Error Tracking

For launch, backend errors are visible through 5xx metrics and structured logs,
and frontend crashes render through the app error boundary. Add Sentry or an
equivalent tracker when credentials are available:

- Frontend: capture `src/app/error.tsx` exceptions.
- Backend: capture panic recovery and unexpected 5xx paths.
- Alerts: page on new high-volume errors and regressions after deploy.

## Backups

Backups are owned by the managed Postgres provider, not the app container.
Before public launch:

- Enable daily backups and point-in-time recovery.
- Record backup retention, RPO, and RTO in the provider dashboard.
- Run one restore drill into a fresh database before announcing the link widely.
- Confirm the restored database can pass `vibegrid migrate` and `/readyz`.
