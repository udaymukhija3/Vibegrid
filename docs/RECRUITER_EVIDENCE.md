# VibeGrid Recruiter Evidence Map

**Updated:** August 1, 2026

**Read with:** [`production-readiness.md`](production-readiness.md)

This map shows where VibeGrid's strongest engineering claims are implemented,
how they are verified, what the deployment currently proves, and which remaining
gap prevents a stronger claim.

## Engineering thesis

VibeGrid demonstrates that a small consumer puzzle can require serious backend
correctness: server-authoritative guesses, retry-safe attempts, secure guest and
admin boundaries, durable content and moderation workflows, and a compact
single-binary deployment. Its strongest evidence is in game-state correctness,
admin session security, Postgres-backed workflows, and the Go/static-Next runtime.
Its weakest evidence is recovery, canonical daily persistence, release
governance, external observability, and full-browser production-path testing.

## Evidence matrix

| Capability | Why product needs it | Existing code evidence | Verification evidence | Deployment/ops evidence | Status | Next action |
| --- | --- | --- | --- | --- | --- | --- |
| Server-authoritative guessing | Prevent answer leakage and client cheating | `server.go`, attempt stores, response schemas | Go tests, security smoke, idempotent guess replay | Live API serves spoiler-safe contracts | **PROVEN** | Add real-browser recovery and load tests |
| Retry-safe attempt state | Networks retry and responses can be lost | Client guess IDs, transactional Postgres guess path | Unit/integration replay and race tests | Durable path exists on managed Postgres | **PROVEN** | Apply idempotency pattern to other mutations |
| Canonical daily lifecycle | Archive, streaks, links, stats, and numbering require one durable identity | `bank_source.go`, puzzle stores, stats | Deterministic-board tests only | Live archive omits fallback days | **MISSING** | Persist every published daily and constrain date/number |
| Durable Postgres model | Attempts, admin, moderation, and stats must survive restarts | Embedded migrations, stores, indexes, cleanup | CI runs real-Postgres tests; local tests may skip | Managed DB is connected | **CREDIBLE_BUT_THIN** | Add constraints, restore proof, and production-path E2E |
| Secure guest boundary | Public play must not require identity or expose answers | Guest cookie/session path, body caps, rate limits | Security contract and HTTP tests | Same-origin Go deployment reduces cookie/CORS complexity | **PROVEN** | Verify proxy-derived client identity and load behavior |
| Secure admin browser session | Moderation and publishing are privileged | Opaque hash-only sessions, CSRF, revocation, secure cookies | Auth/CSRF/security tests | Production secrets are provider-managed | **CREDIBLE_BUT_THIN** | Add named MFA identity before multiple operators |
| UGC moderation lifecycle | Anonymous creation requires review and takedown | Pending creation, approval, reports, appeals, audit log | Store/handler tests | Admin desk is deployed | **PARTIAL** | Make transitions atomic; add creator claim and bot protection |
| Cache correctness | Public reads need low latency without stale takedowns | Bounded cache, negative TTL, singleflight | Cache tests | In-process cache active | **PARTIAL** | Add generation-safe invalidation and deterministic race test |
| Abuse controls | Public writes and identifiers can be attacked | Body caps, validation, blocklist, DB/in-memory limits | Hardening/security tests | Distributed limiter available with Postgres | **PARTIAL** | Turnstile; remove DB writes from read hot path |
| Runtime bounds | Small service must avoid leaks and unbounded resource use | HTTP/DB timeouts, pool bounds, graceful shutdown, bounded cache | Go tests/race/vet | `/healthz`, `/readyz`, protected metrics | **CREDIBLE_BUT_THIN** | Load test; add pool/cache/limiter signals |
| Static single-binary deploy | Simple topology reduces operational surface | Dockerfile, embedded static export, Go entry point | Build and HTTP E2E smoke | Public Render service exists | **PROVEN** | Replace free/demo posture before production claims |
| Release safety | Production changes must be gated and reversible | CI and deploy workflow files | Main CI has passed | Direct Render auto-deploy remains enabled | **MISSING** | Protect main, require CI, deploy hook, rollback drill |
| Data recovery | User and moderation data must survive loss | Encrypted backup workflow and restore runbook scaffold | Workflow syntax/runs only | Inspected green run skipped backup | **MISSING** | Configure backup/PITR, heartbeat, restore drill |
| Observability | Operator must detect failure and latency | Structured logs, metrics, health/readiness, alert templates | Endpoint/security tests | No verified external alert routing | **PARTIAL** | External monitor, Sentry, backup heartbeat, tested notification |
| Frontend resilience | Players need usable loading/error/recovery behavior | Runtime Zod schemas, local attempt reconciliation, responsive UI | Small Vitest suite and HTTP smoke | Live desktop/mobile flow observed | **CREDIBLE_BUT_THIN** | Playwright, axe, mobile CTA improvement |
| Product analytics | Team must distinguish use from code completeness | Puzzle outcome stats | Stats tests | No external product funnel | **MISSING** | Privacy-reviewed event taxonomy and funnel dashboard |
| Editorial puzzle quality | Trust depends on one fair semantic solution | Curated group bank and preview UI | Structural validation only | Fallback guarantees availability, not fairness | **PARTIAL** | Persist candidate, manual board review, ambiguity checklist |

## Verification ladder

Run the following before merging significant launch work:

```bash
npm run typecheck
npm run lint
npm test
npm run test:security
go test ./backend/...
go vet ./backend/...
go test -race ./backend/...
npm run build
git diff --check
```

Also run with the required environment and infrastructure:

```bash
TEST_DATABASE_URL="postgres://..." go test -race ./backend/...
npm run test:e2e
npm audit --omit=dev --audit-level=high
govulncheck ./backend/...
```

The passing command is only as strong as the path it exercises. Record when a
database test skipped, a network audit could not run, or an E2E test used the
in-memory store.

## Deployment status

**Status: BLOCKED for broad production; READY_WITH_MANUAL_STEPS for a controlled beta.**

Exact blockers:

1. Backup and restore are not proven.
2. Fallback dailies are not canonical persisted puzzles.
3. Production deployment is not forced through protected CI.
4. Current dependency findings require upgrades.
5. External monitoring and alert delivery are not verified.

## Highest-value proof to add

1. Restore drill record with measured recovery time.
2. Real-Postgres test proving persisted daily → play → archive → streak.
3. Concurrent moderation/takedown test proving no stale cache visibility.
4. Duplicate/retry tests for create, report, and appeal.
5. Playwright flow for player, creator, and moderator journeys.
6. Production-shaped load test with DB pool, cache, limiter, and latency output.
7. Post-deploy smoke and rollback evidence tied to a release SHA.
