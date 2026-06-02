# VibeGrid

VibeGrid is a daily semantic grouping puzzle: 16 tiles, 4 hidden vibe-based categories, 4 mistakes, and a spoiler-safe result to share.

## Product Vision

The launch version should feel like a sharp internet toy rather than a generic word game. It is quick, cultural, replay-light, and built around one daily ritual.

## Tech Stack

- Go backend
- Next.js App Router
- TypeScript
- Tailwind CSS
- Postgres
- Anonymous sessions first

## Getting Started

```bash
npm install
npm run dev
```

`npm run dev` starts the Go API on `http://localhost:8081` and the Next frontend on the first available Next port, usually `http://localhost:3000`.

Useful commands:

```bash
npm run dev:backend
npm run dev:web
npm run test:backend
npm run test
npm run typecheck
npm run build
```

## Current Shape

- Playable seeded daily puzzle
- Go server-side guess validation
- Anonymous session cookie (HttpOnly, `Secure` in production)
- Durable, transaction-safe attempts in Postgres: each guess is recorded inside a
  transaction that locks the attempt row, so retries and concurrent submissions
  are idempotent and cannot corrupt mistake counts or completion state
- In-memory store fallback when `DATABASE_URL` is unset (tests, quick local runs)
- SQL migrations in `backend/db/migrations/`, applied automatically on startup
- Product decision register and engineering roadmap in `docs/`

## Database

The API runs without a database (in-memory store) when `DATABASE_URL` is unset.
For the durable path, point `DATABASE_URL` at Postgres and the backend migrates
itself on boot:

```bash
createdb vibegrid
DATABASE_URL="postgres://USER@localhost:5432/vibegrid?sslmode=disable" npm run dev:backend
```

Integration tests run against a real Postgres when `TEST_DATABASE_URL` is set,
and are skipped otherwise:

```bash
createdb vibegrid_test
TEST_DATABASE_URL="postgres://USER@localhost:5432/vibegrid_test?sslmode=disable" npm run test:backend
```
