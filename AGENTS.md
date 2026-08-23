# VibeGrid agent handoff

Start here in a new coding session.

## Product source of truth

VibeGrid is an asynchronous private-crew authorship game, not a semantic
grouping puzzle. A dated board contains one prompt and a 28-fragment master
palette with no answer key. Each crew receives a frozen four-column projection:
1–4 members get 3 rows, 5–8 get 4, 9–12 get 5, 13–16 get 6, and 17–20 get 7.
On day D a member chooses four fragments and titles the
combination; on D+1 eligible makers cast one blind non-self vote; on D+2 authors
and votes reveal. An official result needs at least three cards and two votes.

The public homepage has today's local practice plus an explicitly disposable
Unlimited mode whose deterministic curated deals can continue without durable
state. The durable social product requires Postgres and lives in crews. Read `docs/product-vision.md` and
`docs/decision-register.md` before changing the loop.

The old hidden-group engine remains behind legacy `/p/<id>` links and APIs only
so old shares keep resolving. Do not restore its mechanics, language, archive,
builder, score, timer, difficulty, mistakes, or result grid to the primary
product or sitemap.

## First commands

```bash
git status --short
npm run typecheck
npm run lint
npm test
npm run test:security
npm run test:backend
```

For backend, schema, security, or release work also run:

```bash
go vet ./backend/...
go test -race ./backend/...
npm run build
git diff --check
```

`TEST_DATABASE_URL` enables real Postgres integration tests. `npm run test:e2e`
binds a local port and may need an unrestricted environment. Registry audits
need network access.

## Architecture map

- `backend/internal/vibegrid/vibe_boards.go`: fallback editorial palettes and
  board validation.
- `backend/internal/vibegrid/vibe_rounds.go`: card/vote transactions, replay
  semantics, snapshots, and streak queries.
- `backend/internal/vibegrid/vibe_round_handlers.go`: make/judge/reveal response
  projection and staged disclosure.
- `backend/internal/vibegrid/vibe_board_admin.go`: immutable dated authoring.
- `backend/db/migrations/00018_vibe_rounds.sql`: base product schema.
- `backend/db/migrations/00019_variable_vibe_boards.sql`: additive master-palette
  expansion and frozen per-crew sizing.
- `src/components/VibeGridApp.tsx`: public practice.
- `src/components/CrewRoom.tsx`: durable crew experience.
- `src/components/VibeComposer.tsx`, `VibeCard.tsx`: interaction primitives.
- `src/components/VibeBoardDesk.tsx`: editor board room.
- `src/lib/api.ts`, `src/types/vibe.ts`: runtime-validated client contracts.
- `scripts/smoke.mjs`, `scripts/e2e.mjs`: runtime verification.

The Go binary still embeds the exported Next.js frontend and legacy migrations,
stores, moderation, and route compatibility. Removal is a separate migration
project, not drive-by cleanup.

## Product invariants

- A new editorial master has 28 unique fragment ids/texts and no grouping
  metadata; legacy 12-fragment boards remain immutable.
- The rendered board always has four columns. Public practice is 4x4. A crew's
  3–7 row size is frozen against membership on first member open and cannot be
  resized by later joins, leaves, removals, retries, or concurrent opens.
- Unlimited mode deals another local 4x4 board on demand. It has no correctness,
  timer, lives, streak, persistence, invented people, or effect on crew phases.
- A card has exactly 4 distinct fragments from that board and a 1–40 rune title.
- One member gets one card per crew/board and one vote per crew/board.
- Only a member who submitted can vote; the target must be another card in the
  same crew and board.
- Membership authorization for mutations stays inside the database transaction.
- Client replay ids are stable across network retries. Reusing one with changed
  input is a conflict.
- Nonmembers never receive member lists, cards, ballots, votes, or results.
- During make, members receive only their own card. During judge, author names
  are absent. During result, authors reveal and ties remain ties.
- Board content is immutable after the first row for a publish date is stored.
- Crew links are capability secrets: never put them in sitemap or public feeds.

## Security and operations already implemented

- Production guardrails for database, secure cookies, HTTPS public base URL,
  metrics token, CORS, and trusted proxy CIDRs.
- Opaque revocable admin cookies, hash-only storage, and CSRF for cookie writes.
- Bounded API metric labels, request bodies, identifiers, rate limits, DB
  timeouts, pool limits, health/readiness, structured logs, and graceful stop.
- Single-container Go + embedded static Next export, additive embedded
  migrations, Fly/Render/Docker scaffolding, and CI/security contracts.

Do not claim provider backups, a restore drill, permanent production uptime,
external alert delivery, traffic, or user retention unless the repo contains
current evidence.

## Change discipline

- Use `rg`/`rg --files` first and preserve unrelated dirty work.
- Use `apply_patch` for source edits.
- After route changes, update metric labels, runtime schemas, smoke tests, and
  route docs.
- After schema changes, add constraints plus a Postgres integration test.
- After product-contract changes, update README, product vision, decision
  register, privacy/terms/policy, recruiter evidence, and production readiness.
- Keep board prompts human, specific, socially generative, and safe. All 28
  fragments need multiple plausible combinations; they must not secretly form
  three or four intended categories.
- Prefer honest quiet states over invented activity or fake social proof.
