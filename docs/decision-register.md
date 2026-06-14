# VibeGrid Decision Register

## Ratified For Scaffold

| Area | Decision | Why |
| --- | --- | --- |
| Product shape | Daily puzzle, one grid per date | Keeps the ritual simple and shareable. |
| Identity | Guest play for public players; admin login only for the Editor Desk | Removes signup friction, keeps the daily playable on first load, and still supports operator access. |
| Core rules | 16 tiles, 4 groups, 4 mistakes | Familiar enough to learn instantly. |
| Validation | Go server-side guess validation | Prevents client source from becoming the answer key. |
| Persistence | Postgres attempts (transaction-safe), in-memory fallback | Durable, idempotent, concurrency-safe attempt state; in-memory store keeps tests and no-DB runs fast. |
| Stack | Go API, Next.js, TypeScript, Tailwind, Postgres | Keeps backend rules independent and frontend iteration fast. |
| Launch timezone | Asia/Kolkata in the scaffold | Matches the current workspace context; should be revisited before public launch. |
| Multiplayer | Async sharing and community puzzle links only for v1 | Preserves the shared-daily ritual without real-time rooms, presence, matchmaking, or chat moderation. |

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
| Moderation posture | Later AI/admin content needs guardrails. | Keep all published puzzles human-reviewed. |
| Brand/legal | Cultural references and trademarks may appear in tiles. | Allow common references, avoid using brands as insults or endorsements. |
| Monetization | Can distort the toy if introduced too early. | No monetization in v1. |
| Puzzle immutability after publish | Editing a published puzzle can break in-flight attempts — stored solved-group tile ids stop matching the new tiles. | Treat published content as immutable; an edit becomes a new puzzle/date. (Rendering already degrades safely on a mismatch; this is a publish policy, not a bug.) |
| Open-tab daily rollover | A tab left open past the daily flip still holds yesterday's grid; the client only refetches the current puzzle on load. | Leave the open tab on its puzzle and reconcile on next load — don't yank a grid out from under a guess. (Sub-case of *Launch timezone*.) |

> The last two were surfaced by the cross-tab state-reconciliation work (branch
> `cross-tab-state-sync`). That branch already makes the client↔server merge
> forward-only (a solved board is never wiped by a cleared/expired cookie), but
> these two are product/policy calls it deliberately did **not** decide. See
> [`launch-sprint-plan.md`](launch-sprint-plan.md) for how decisions D1–D7 map to
> specific sprints.

## Deferred Bets

| Area | Revisit When | Default Until Then |
| --- | --- | --- |
| Player login | Anonymous retention, streak usage, or cross-device support becomes a real product problem. | Keep public play as guest-first; do not block launch on accounts, OAuth, leaderboards, or account recovery. |
| Real-time multiplayer | There is evidence that live rooms are worth the infra, moderation, and scheduling cost. | Treat share text and community puzzle links as the multiplayer loop. |
