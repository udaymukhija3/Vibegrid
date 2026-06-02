# VibeGrid Decision Register

## Ratified For Scaffold

| Area | Decision | Why |
| --- | --- | --- |
| Product shape | Daily puzzle, one grid per date | Keeps the ritual simple and shareable. |
| Identity | Anonymous sessions first | Removes signup friction and still supports persistence. |
| Core rules | 16 tiles, 4 groups, 4 mistakes | Familiar enough to learn instantly. |
| Validation | Go server-side guess validation | Prevents client source from becoming the answer key. |
| Persistence | Postgres attempts (transaction-safe), in-memory fallback | Durable, idempotent, concurrency-safe attempt state; in-memory store keeps tests and no-DB runs fast. |
| Stack | Go API, Next.js, TypeScript, Tailwind, Postgres | Keeps backend rules independent and frontend iteration fast. |
| Launch timezone | Asia/Kolkata in the scaffold | Matches the current workspace context; should be revisited before public launch. |

## Product Decisions Waiting To Be Made

| Decision | Why It Matters | Suggested Default |
| --- | --- | --- |
| Launch timezone | Determines when the daily grid rolls over and how sharing feels across geographies. | Pick one global timezone for v1; local-time puzzles can wait. |
| Editorial boundaries | Vibe names are the product's personality and risk surface. | Write a short style guide with allowed humor, banned targets, and regional reference rules. |
| Difficulty ladder | Players need fair puzzles, not just funny categories. | Use three bands: easy semantic sets, medium cultural associations, hard misdirection. |
| Share format | Drives virality and must avoid spoilers. | Text-only first; add colored blocks only when categories have stable color semantics. |
| Archive access | Changes retention and streak pressure. | Show previous puzzles, but keep streak tied only to current-day play. |
| Failure UX | Determines whether players can learn after losing. | Reveal all groups after four mistakes; mark result as failed. |
| Streak rules | Edge cases become support issues quickly. | Streak increments on completed current-day puzzle only. |
| Global stats | Stats can motivate or shame depending on presentation. | Show median mistakes and solve rate after completion, not before. |
| Admin workflow | Puzzle quality depends on review, preview, and publishing safety. | Draft -> preview -> publish, with one puzzle per date. |
| Puzzle QA process | Bad puzzles break trust faster than bugs. | Require a human test solve before publishing. |
| Auth path | Accounts unlock sync and leaderboards but add friction. | Delay accounts until anonymous retention is proven. |
| Moderation posture | Later AI/admin content needs guardrails. | Keep all published puzzles human-reviewed. |
| Brand/legal | Cultural references and trademarks may appear in tiles. | Allow common references, avoid using brands as insults or endorsements. |
| Monetization | Can distort the toy if introduced too early. | No monetization in v1. |
