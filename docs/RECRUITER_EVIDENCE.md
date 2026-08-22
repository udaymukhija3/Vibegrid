# VibeGrid recruiter evidence map

**Updated:** 21 August 2026
**Read with:** [`product-vision.md`](product-vision.md) and
[`production-readiness.md`](production-readiness.md)

## Engineering thesis

VibeGrid demonstrates serious product engineering through a small social game
whose rules cross time, people, privacy stages, and unreliable networks. The
strongest claim is not “I built a word puzzle.” It is:

> I identified that a polished product was mechanically derivative, replaced
> its core loop with an original asynchronous make–judge–reveal system, and
> carried that product contract through transactional storage, member-aware API
> projection, resilient clients, editorial operations, security, tests,
> observability, and a deployable single-binary runtime.

## Evidence matrix

Status meanings:

- **PROVEN** — direct code plus automated verification.
- **CREDIBLE** — implemented with meaningful tests, but production-shaped or
  external proof is still missing.
- **PARTIAL** — useful implementation exists but a material path is absent.
- **MANUAL** — provider or operational state cannot be completed in this repo.

| Capability | Product reason | Code evidence | Verification | Status | Honest next proof |
| --- | --- | --- | --- | --- | --- |
| Differentiated product contract | Avoid shipping a clone with cosmetic novelty | `docs/product-vision.md`, public/crew UI, legacy boundary | Active routes no longer expose solve mechanics; smoke checks new board shape | **PROVEN** | Controlled-beta behavior, not more features |
| Immutable daily constraint | Every crew must interpret the same historical palette | `vibe_boards.go`, `vibe_rounds.go`, `vibe_daily_boards` | Deterministic/validation tests; strict dated insert | **PROVEN** | UTC rollover soak on deployed DB |
| Transactional authorship | One member cannot race two cards into a round | `SubmitVibe`, unique `(crew,board,member)` constraint | Real-Postgres integration when `TEST_DATABASE_URL` exists | **CREDIBLE** | Record CI run with Postgres service |
| Replay-safe mutations | A timeout retry must not duplicate a card or ballot | client ids in API/store; frontend idempotency headers and cold-start replay | Go replay/conflict tests; smoke repeats a submission | **PROVEN** | Browser test that aborts the first response |
| Fair blind ballot | Makers vote once, not for themselves, without author leakage | `CastVibeVote`, `buildJudgeView` | Eligibility/self-vote/one-vote integration and disclosure unit tests | **PROVEN** | Multi-browser E2E over real DB |
| Staged privacy | Outsiders and premature phases must not receive crew content | `buildVibeCrewDaily`, capability crew routes | Unit test asserts outsider, make, blind judge, reveal views | **PROVEN** | Security review with hostile HTTP cases |
| Honest result semantics | Quiet rounds and ties must not invent significance | `buildResultView`, `CrewStreak` | Unit tie test; Postgres official-streak test | **PROVEN** | Product validation of thresholds |
| Crew access control | Leaked invites and departed members are real lifecycle cases | crew store/handlers: rotate, remove, leave, ownership transfer | Existing Go crew authorization tests | **PROVEN** | Browser owner-control E2E |
| Editorial operations | Daily quality cannot depend on editing code | `vibe_board_admin.go`, `VibeBoardDesk.tsx` | Strict input unit tests, auth/CSRF foundation, immutable conflict | **CREDIBLE** | Admin HTTP integration and visual preview test |
| Runtime contract validation | Backend drift should fail visibly in the client | Zod schemas in `api.ts`/`adminApi.ts` | Typecheck and frontend unit suite | **CREDIBLE** | Component tests for all phase states |
| No-DB honesty | A social persistence feature must not pretend to work in memory | public fallback plus explicit crew `503` | E2E smoke covers practice and skip reason | **PROVEN** | None; preserve boundary |
| Admin security | Board publishing is privileged | hash-only opaque sessions, CSRF, revocation, rate-limited login | Go auth/hardening tests and security contract | **PROVEN** | Named MFA identity before multiple operators |
| Abuse boundaries | Public capability links and text writes are attack surfaces | body/id caps, validation, blocklist, rate limits, safe proxy identity | hardening/security tests | **CREDIBLE** | Load/abuse test and crew-card report flow |
| Observability | A daily phased product needs detectable failures | route metrics, operation metrics, logs, health/readiness, pool/cache/outbox gauges | endpoint tests, bounded-label security test | **CREDIBLE** | External scrape, dashboard, and routed alert |
| Single-binary deployment | Same-origin cookies and a small ops surface | Dockerfile, embedded static Next export/migrations, Go server | build, backend tests, local E2E smoke | **PROVEN** | Public SHA-linked deploy smoke |
| Recovery | Crew history is durable only if data can be restored | backup workflow/runbook scaffolding | no completed provider restore artifact | **MANUAL** | Managed PITR plus timed restore drill |
| Product validation | Originality is not the same as demand | metrics are defined in product vision | no privacy-reviewed event store or user cohort | **PARTIAL** | 5–10 controlled crews; make→judge→reveal funnel |
| Accessibility | Selection, ballot, and focus must work beyond pointer use | semantic buttons, `aria-pressed`, labels, focus/reduced motion | lint/type checks; limited automated coverage | **PARTIAL** | Playwright + axe + keyboard/screen-reader pass |

## Verification ladder

```bash
npm run typecheck
npm run lint
npm test
npm run test:security
npm run test:backend
go vet ./backend/...
go test -race ./backend/...
npm run build
git diff --check
```

Production-shaped additions:

```bash
TEST_DATABASE_URL="postgres://..." go test -race ./backend/...
npm run test:e2e
npm audit --omit=dev --audit-level=high
govulncheck ./backend/...
```

A green test that skipped Postgres is not evidence of transactional behavior.
An E2E run in no-database mode proves the practice/runtime path, not crews.

## Demo path for an interview

1. Open `/` and make one practice card. Explain that there is no correct set.
2. Show the blind house ballot and reveal.
3. Open a real three-member crew across browser profiles if a test DB is
   available: make on day D fixtures, judge D-1, reveal D-2.
4. Show `vibe_rounds.go` and migration constraints, then the disclosure test.
5. Show `/admin`: one prompt, 12 fragments, immutable date.
6. Run the smoke script and explain which paths require Postgres.
7. State the manual gaps before the interviewer has to ask.

## Claims that are safe now

- Reimagined and implemented an asynchronous private-crew game around authored
  four-fragment cards, blind voting, delayed reveal, ties, and participation
  thresholds.
- Designed replay-safe, transaction-authorized Go/Postgres mutations with
  database constraints mirroring product rules.
- Built stage-specific privacy projections so outsiders, makers, judges, and
  result viewers receive different server payloads.
- Shipped an immutable daily content pipeline and authenticated editor surface.
- Preserved legacy shared links while removing them from the active product and
  search surface.
- Built a single-container Go + exported Next.js runtime with migrations,
  health/readiness, protected metrics, rate limits, CI, and deployment runbooks.

## Claims that are not safe yet

- Production launch, real traffic, user retention, or product-market fit.
- Verified backups, PITR, restore time, external uptime, or alert delivery.
- Full accessibility compliance or comprehensive browser/device coverage.
- Live/realtime multiplayer, accounts, cross-device recovery, notifications, or
  native mobile apps.
- AI generation or recommendation.

The recruiter-ready standard used here changed the work in one important way:
every attractive claim is paired with code evidence, verification evidence, and
the next missing proof. Architecture breadth alone is not treated as readiness.
