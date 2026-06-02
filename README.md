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
- Anonymous session cookie
- Server-owned in-memory attempts with idempotent client guess handling
- Postgres schema draft in `backend/db/schema.sql`
- Product decision register and engineering roadmap in `docs/`
