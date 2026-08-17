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
| Multiplayer | Crews: invite-link groups playing the same daily, with a shared board (2026-08-17) | Reverses the earlier async-only call. Keeps the shared-daily ritual — everyone still plays the same grid alone — while making "how did you do?" native instead of a copy-paste into a group chat. No shared board mutation, so no conflict rules, presence, or matchmaking. |
| Crew identity | Per-crew display name on top of the anonymous session cookie | A leaderboard needs a name; accounts are still not worth the friction. The name is scoped to one crew, so there is no global namespace to moderate. |
| Crew invites | Rotatable `invite_code`, separate from the crew's internal id (2026-08-17) | The id used to be the invite secret, which made a leaked link permanent and unrevocable. Rotating kills every shared link; members are unaffected because membership is by session and they re-enter from My crews. |
| Crew ownership | Founder owns it; leaving transfers to the longest-standing member, last one out deletes the crew | An ownerless crew could never be rotated or moderated again. Owner-only writes are enforced inside the mutating statement, so there is no check-then-act gap. |
| How-to-play | Auto-opens until the session has finished a grid, not once per browser (2026-08-17) | The old localStorage flag was wiped by incognito, cache clears, and Safari's cap on script-writable storage, so it re-prompted regulars while never returning for a first-timer who dismissed it. Completion is durable and cross-device. |
| Crew spoilers | Result grids are withheld server-side until the viewer finishes today's grid | A crew board that shows a friend's grid early hands out the answers. The grids are not loaded at all until unlocked, so there is nothing to read out of the payload. |
| Community sharing | Created puzzles are playable by link immediately; review gates public listing only (2026-08-17) | "Make a grid for your friends" cannot have a manual approval step in the middle. Unlisted grids never enter the daily, archive, or sitemap, and a takedown or withdrawal still kills the link. |

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
| Live crew race (SSE) | Crews are used enough that a 15s board refresh feels slow. | Crew boards poll every 15s while the tab is visible. Streaming needs the shared `http.TimeoutHandler` (which buffers responses) and the server's 15s `WriteTimeout` exempted for the stream path. |
| Shared-board co-op | There is evidence people want to solve one grid together, not race separate ones. | Crews race the same daily independently — no shared mistake budget, no conflict rules, no presence. |
