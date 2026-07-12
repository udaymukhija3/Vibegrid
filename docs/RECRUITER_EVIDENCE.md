# VibeGrid Recruiter Evidence

## Engineering thesis

VibeGrid is strongest as a compact production web-app case: a daily semantic
puzzle that keeps the browser untrusted, persists anonymous play safely when
Postgres is enabled, accepts community content only through review, and deploys
as one same-origin Go service with measurable health, readiness, and runtime
signals. The project should prove product engineering depth through the smallest
set of real constraints that matter for this app: game authority, durable
attempt correctness, moderated publishing, deployability, and operations.

## Primary capabilities

| Capability | Why product needs it | Existing code evidence | Verification evidence | Deployment/ops evidence | Status | Next action |
|---|---|---|---|---|---|---|
| Server-authoritative puzzle play without answer-key leakage | A player can inspect network payloads; the app is only credible if the browser cannot derive tile-to-group answers before guessing. | `backend/internal/vibegrid/puzzles.go` exposes `PublicPuzzle` with tile ids/text only; `backend/internal/vibegrid/server.go` routes guesses through `EvaluateGuess`; `src/lib/api.ts` validates the public contract with Zod. | `backend/internal/vibegrid/hardening_test.go` checks public tile order is a stable permutation, not group-blocked; `scripts/smoke.mjs` now asserts public puzzle payloads expose no groups/answers/extra tile keys. | Same-origin Go binary serves static UI and `/api/*`, so the server remains the authority in the deployed topology. | PROVEN | Keep the smoke assertion in every deploy verification run. |
| Durable, idempotent anonymous attempts | Refreshes, retries, double-clicks, and two tabs must not corrupt guesses, mistake counts, or the spoiler-safe share grid. | `backend/internal/vibegrid/postgres_store.go` validates before allocation, locks attempt rows in a transaction, and uses `(attempt_id, client_guess_id)` uniqueness for replay safety. | `backend/internal/vibegrid/postgres_store_test.go` covers duplicate replay, concurrent duplicate races, distinct concurrent guesses, terminal failure, and fresh guess-history hydration; `scripts/smoke.mjs` now replays the same deployed guess and verifies counts/history do not advance. | `backend/db/migrations/00001_attempts.sql` defines attempt/guess uniqueness; `/metrics` exposes store operation latency/counters. | PROVEN | Run `TEST_DATABASE_URL=... go test -race ./backend/...` before claiming durable Postgres proof in a new environment. |
| Reviewed community creation and admin publishing/moderation | Public UGC must not become instantly playable or visible without review; admins need a real workflow to approve, archive, reinstate, and audit. | `backend/internal/vibegrid/community.go`, `admin.go`, `moderation_handlers.go`, and `postgres_puzzles.go` persist submissions as `PENDING`, separate public play from preview, and record moderation actions. | `backend/internal/vibegrid/community_test.go` proves pending submissions are hidden until approval; `backend/internal/vibegrid/admin_test.go` covers create/publish/archive and report/appeal/reinstate flow. | `README.md`, `/policy`, `/terms`, `/privacy`, and the deploy smoke `--create-community` path describe and exercise the review contract. | PROVEN | Production verification still needs one real create -> approve -> report -> reinstate pass against the live database. |
| Single-container deploy with production guardrails | The portfolio app needs a credible public URL without split-origin cookie/CORS complexity. | `Dockerfile`, `backend/internal/frontend`, and `backend/cmd/vibegrid/main.go` build one Go binary with embedded Next export and enforce production env requirements. | `scripts/e2e.mjs` builds the Next export, embeds it, builds the Go binary, starts it, and runs smoke against the real serving path. | `fly.toml`, `render.yaml`, `docs/deployment.md`, `/healthz`, `/readyz`, and `vibegrid migrate` define the deploy topology and migration ordering. | CREDIBLE_BUT_THIN | Permanent host, managed Postgres, backups/PITR, restore drill, and external uptime/logging are still manual. |
| Concrete security boundaries for a small public app | Anonymous writes, admin cookies, metrics, SQL access, and proxy headers are the highest-risk surfaces in this product. | `backend/internal/vibegrid/security.go`, `admin_auth.go`, `admin_sessions.go`, `rate_limits.go`, and `server.go` implement body caps, JSON content-type checks, trusted proxy CIDRs, CSRF for cookie admin mutations, opaque revocable admin sessions, and bearer-protected metrics. | `backend/internal/vibegrid/hardening_test.go`, `community_test.go`, `runtime_metrics_test.go`, and `scripts/security-contract.mjs` cover CSRF, logout revocation, public write limiter fail-closed behavior, proxy-header trust, metrics auth, oversized/non-JSON bodies, dynamic SQL builder drift, and broad proxy defaults. | Production startup fails without secure cookies, metrics token, exact HTTPS public URL, database requirement, and safe CORS/proxy settings; CI runs `npm run test:security`. | PROVEN | Keep dependency scanning and branch protections enabled in GitHub/provider settings. |
| Operational visibility without observability theater | A reviewer/operator should be able to answer whether the app is alive, ready, slow, failing, or saturating a bounded resource. | `backend/internal/vibegrid/metrics.go`, `observed_stores.go`, request logging middleware, DB pool stats, puzzle cache stats, and bounded route labels. | `backend/internal/vibegrid/runtime_metrics_test.go` verifies runtime gauges and route-label cardinality; `scripts/smoke.mjs` verifies protected metric families when a token is supplied. | `monitoring/prometheus.yml`, `monitoring/alert-rules.yml`, `monitoring/grafana-dashboard.json`, and `docs/observability.md` define the first operational view. | CREDIBLE_BUT_THIN | Wire a real scrape target, log drain, and alert route after the permanent host exists. |

## Why this is more than a CRUD tutorial

The central state transition is not a generic create/update/delete form: a guess
must be validated against hidden server-only puzzle structure, serialized with
other guesses for the same anonymous attempt, deduplicated by client guess id,
and reflected back to any browser tab through a durable ordered guess history.
The repo also covers a full content lifecycle around that loop: community draft
creation, admin review, public play gates, reports, appeals, audit logs,
release-time migrations, runtime probes, protected metrics, and deploy smoke
that exercises the same binary shape intended for production.

## Evidence gaps not worth solving with extra architecture

- Do not add Kubernetes, Redis, queues, or microservices for appearance. The
  current single-container shape is the right scale for the product.
- Do not add accounts/OAuth before there is a real product need for cross-device
  identity; guest sessions match the v1 daily puzzle contract.
- Do not claim production traffic, uptime, backups, or alerting until a real
  host and provider settings are verified.
