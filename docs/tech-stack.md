# VibeGrid technical stack

The stack follows the product’s risk: small responsive interaction in the
browser, social and privacy rules in one server, and durable relational
constraints in Postgres.

## Frontend

- Next.js 15 App Router, exported as static files.
- React 19 and TypeScript with strict type checking.
- Zod runtime parsing for every active public/crew/admin response.
- Tailwind CSS plus a small semantic component layer in `globals.css`.
- Bricolage Grotesque and IBM Plex Mono vendored locally; no build-time font
  network dependency.
- Lucide icons and Sonner feedback.
- Vitest for pure client/storage/network behavior.

The public practice loop is client-local. Durable crew state is always loaded
from the same-origin Go API with credentials included.

## Backend

- Go standard `net/http` mux and middleware.
- `database/sql` with the Postgres driver and explicit timeouts/pool bounds.
- Embedded goose-style SQL migrations.
- Server-side stage projection and authorization.
- Transactional card/vote writes with database uniqueness constraints.
- Opaque cookie sessions, admin CSRF/revocation, rate limits, request/body caps,
  structured `slog`, Prometheus-text metrics, health/readiness, and graceful
  shutdown.

## Persistence model

Active product tables:

- `vibe_daily_boards`: immutable dated prompt and 12-tile JSON snapshot.
- `vibe_submissions`: crew/board/member, author-name snapshot, title, four tile
  ids, client replay id.
- `vibe_votes`: crew/board/voter/target, client replay id.
- `crews` and `crew_members`: rotatable capability invite plus session-scoped
  membership and owner lifecycle.

Legacy puzzle, attempt, moderation, and community tables remain for old `/p`
links. They are not the active product model.

## Runtime

```text
Next static export ─┐
SQL migrations ─────┼─ go:embed → one Go binary → Postgres
Go API/server ──────┘
```

The production image is multi-stage and distroless/non-root. Same-origin serving
keeps guest/admin cookie and CORS behavior simple.

## Why not a larger stack

- No WebSocket/SSE until an async polling delay is measured as a problem.
- No Redis while Postgres and bounded in-process caches meet the topology.
- No message broker for the core loop; phase changes derive from date.
- No auth provider until cross-device recovery is a proven need.
- No AI service because editorial taste and safe ambiguity are not an inference
  throughput problem.

Each deferred dependency would add runtime, security, cost, and portfolio claims
that the current product does not yet earn.
