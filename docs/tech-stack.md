# VibeGrid Tech Stack

## Stack Pick

| Layer | Choice | Rationale |
| --- | --- | --- |
| Frontend | Next.js App Router | Keeps the UI fast to iterate and easy to deploy. |
| Backend | Go stdlib HTTP service | Owns game rules, sessions, attempts, and API contracts without JS backend churn. |
| Frontend language | TypeScript | Keeps puzzle, attempt, and admin contracts explicit in the UI. |
| Styling | Tailwind CSS | Fast iteration with a small custom visual system. |
| Backend data path | Go structs now, Postgres next | In-memory store ships the first slice; `backend/db/schema.sql` defines the durable target. |
| Database | Postgres | Reliable relational fit for puzzles, groups, tiles, attempts, guesses, and stats. |
| Identity | Guest cookie sessions; admin login for operations | Best v1 UX; public accounts can be layered in later only if retention proves they matter. |
| Validation | Go request validation at API edge | The backend is the source of truth for legal guesses and attempt state. |
| Icons | Lucide React | Familiar controls without custom icon work. |
| Deployment | Vercel frontend + Fly/Render/Railway Go API | Keeps frontend and backend deployable as independent services. |

## Why This Stack

This is a small product with real state, not a complex platform. Next.js keeps the public game UI quick to iterate. Go owns the backend because attempts, idempotency, session cookies, publishing, archive, and stats should not depend on framework-specific frontend internals.

The key technical decision is server-side validation. The player UI receives tile text and ids, but not group membership. The Go API validates selected ids and only reveals a group after a correct guess.

## What Is Built vs Stubbed Today

- Attempts, guesses, idempotency, mistakes, completion, and failure state are
  durable in Postgres via a transaction-safe store (`PostgresAttemptStore`), with
  an in-memory store as the no-database fallback. Migrations live in
  `backend/db/migrations/` and apply on startup.
- Puzzle *content* is still static and served from the seed package. DB-backed
  puzzles and admin authoring are the next step (P1: Editor Desk); until then
  `attempts.puzzle_id` is a plain reference rather than a foreign key.
- The client keeps selected-tile UI state locally, but the server is the source
  of truth for attempt state.

## Testing Direction

- Unit test the Go puzzle engine first.
- Add repository tests after DB-backed attempts land.
- Add Playwright coverage for select, submit, lock group, fail, complete, and share flows.
