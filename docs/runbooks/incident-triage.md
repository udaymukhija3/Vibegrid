# Runbook: incident triage

Work top-down; each step tells you where the fault is.

## 1. Is the process up?

```sh
curl -s https://vibegrid.onrender.com/healthz   # liveness: process is running
curl -s https://vibegrid.onrender.com/readyz    # readiness: process can reach Postgres
```

- Both fail / time out → Render problem: dashboard → service → Events + Logs. Free tier cold start takes ~25s after idle — wait one minute and retry before declaring an incident.
- `healthz` OK, `readyz` 503 → database problem: Neon console (endpoint state, connection limits) and [Neon status page](https://neonstatus.com). The app degrades deliberately here: public puzzle reads stay up on the in-memory rate limiter; guesses/creates return 503 until the DB is back.

## 2. Did we just deploy?

Render dashboard → Deploys. If the incident started at a deploy boundary, roll back first, diagnose second (see deploy-and-rollback runbook).

## 3. What do the logs say?

Render dashboard → Logs. Every request is a JSON line with `status`, `duration_ms`, and `request_id`; every 500 logs its error with the same `request_id`. Filter for `"status":5` to find server errors, then search the `request_id` for the paired error line.

## 4. Known failure signatures

| Symptom | Likely cause | Action |
|---|---|---|
| "Loading today's grid…" for ~10s, then loads | Cold start; client retried with extended budget (`src/lib/http.ts`) | None; consider paid instance if frequent |
| keep-warm workflow failing | Service down or Render outage | Treat as step 1 |
| 503 on guesses, reads fine | Postgres down/unreachable (fail-closed writes) | Neon console; wait or restore |
| 429s reported by users | Rate limits (per-IP) — shared NAT or abuse | Check logs for the IP; raise limits in `server.go` constants only with evidence |
| Daily puzzle wrong/stale after midnight | CDN/browser cache within rollover window, or `VIBEGRID_TIMEZONE` misconfig | Verify `/api/puzzles/today` directly; check env |

## 5. Rollback vs fix-forward

Rollback when the incident is deploy-correlated or user-facing and the fix isn't obvious within ~15 minutes. Fix-forward only for one-line fixes that CI can validate. Production being down is never improved by debugging under pressure.
