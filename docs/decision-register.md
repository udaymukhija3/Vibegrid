# VibeGrid decision register

**Ratified:** 21 August 2026. A row changes only when product behavior, tests,
and this document change together.

## Locked product decisions

| Area | Decision | Reason |
| --- | --- | --- |
| Category | Delayed social authorship game | The authored card and known crew create the differentiated value; hidden grouping is legacy only. |
| Primary unit | One private crew of 3–8 known people | Meaning and reveal suspense depend on relationship context. System cap remains 20. |
| Daily input | One prompt plus exactly 12 unique fragments | Constrained enough to compare, broad enough for multiple readings, and distinct from a 4×4 solve grid. |
| Player output | Exactly 4 fragments plus a 1–40 rune title | Four creates a memorable card; the title makes the selection an authored interpretation. |
| Answer model | No answer key, category mapping, correctness, difficulty, timer, or mistake count | The crew evaluates resonance, not editor intent. |
| Phases | Day D make; D+1 blind judge; D+2 reveal | Delayed stages create return behavior and fit an async group-chat tempo. |
| Judge eligibility | Only members who made a card in that round | Voting is reciprocal participation, not spectator influence. |
| Ballot | One vote for another member’s card; no self-vote | Simple, legible, and resistant to self-promotion. |
| Reveal | Author names and final counts reveal to current crew members | The social recognition is the payoff; blind authorship is temporary. |
| Official result | At least 3 cards and 2 votes | Below that, reveal honestly as a quiet round and do not extend streak. |
| Ties | Preserve every top tie | No arbitrary tiebreaker or earliest-submit advantage. |
| Streak | Consecutive official crew results through the latest revealed date | Measures group recurrence, not one person opening the app. |
| Public experience | Complete local practice: make, house ballot, immediate reveal | Teaches the whole loop without fake people, signup, or durable public content. |
| Identity | HttpOnly guest browser session; per-crew display name | Lowest-friction durable membership without global profiles. |
| Invite security | Rotatable capability code separate from internal crew id | A leaked link can be revoked without destroying history. |
| Ownership | Creator owns; owner leave transfers to longest-standing member; last leave deletes | Keeps access control operable and defines crew deletion. |
| Privacy | Nonmembers get only crew identity plus today’s public prompt/count; stage content is member-only | Invite knowledge alone must not reveal private cards or roster. |
| Board lifecycle | First persisted board for a date is immutable | Retries, open clients, cards, and historical results must share one palette. |
| Editorial supply | Human-authored scheduled board wins; deterministic curated bank fills a missing date | No empty day, no automatic AI publishing, and history remains frozen. |
| Timezone | UTC globally | One unambiguous rollover for all crews; deployments and defaults match. |
| Realtime | Visible-tab polling every 30 seconds | Adequate for async play; streaming waits for evidence. |
| Search | Index only product-level public pages; never crew or compatibility links | Crews are private capability spaces and old `/p` grids are not the active product. |
| Visual direction | One Toy × Arcade × Sticker launch system | Big tactile hierarchy, dark social energy, and a recognizable 3×4 palette; no theme chooser. |
| Legacy | Old `/p` URLs and APIs remain compatibility-only | Preserve old shares without letting legacy mechanics govern the roadmap. |

## Locked engineering decisions

| Area | Decision | Proof obligation |
| --- | --- | --- |
| Mutation authority | Membership and invariants are checked in a Postgres transaction | Integration tests cover outsider, duplicate, replay, eligibility, self-vote, and one-ballot behavior. |
| Idempotency | Client card/vote ids are persisted; exact replay returns the original object; changed replay conflicts | Client retries reuse the same body and server tests assert no second row. |
| Disclosure | Go builds member-aware make/judge/result projections | Unit tests assert outsiders see no private data and judge cards omit authors. |
| Schema | Unique constraints mirror one-card, replay, and one-ballot invariants | Migration and real-Postgres test. |
| Historical identity | Card stores author display-name snapshot and opaque member id | Removing a membership cannot orphan the visible history. |
| Public fallback | Practice works without DB; crew writes return explicit 503 | No in-memory fake of a durable social system. |
| Admin board authoring | Future/today date, one prompt, 12 distinct fragments, strict insert | Frozen date conflicts; first stored board wins. |
| Runtime shape | Exported Next.js embedded into one Go binary with same-origin API | Docker/build/E2E smoke. |
| Operational truth | External provider state is manual until verified | Docs separate code evidence from backups, alerts, domains, and traffic claims. |

## Explicitly rejected ideas

| Idea | Why rejected now |
| --- | --- |
| “Connections, but multiplayer” | The central action remains derivative and multiplayer is merely comparative. |
| Real-time co-op selection | Changes the product into scheduling and conflict management before the async loop is validated. |
| Public card feed | Removes relationship context, raises moderation cost, and rewards performance for strangers. |
| Self-votes or spectator votes | Break reciprocal participation and makes low-participation tallies noisy. |
| Forced single winner | Turns submission timing or arbitrary rules into a hidden tiebreak. |
| User-authored boards at launch | Multiplies safety and quality problems before the core editorial format is proven. |
| AI automatic board generation | Taste and safe ambiguity are the product; auto-publishing would hide the weakest link. |
| Multiple visual themes | Dilutes recognition and postpones choosing a product personality. |
| Accounts before evidence | Adds auth/recovery/privacy surface without improving the core loop for an existing crew. |

## Open decisions with evidence gates

| Decision | Evidence required | Default until then |
| --- | --- | --- |
| Notifications | Crews make but fail to return for judge/reveal | No push/email. Test an optional calendar or web-push reminder only after funnel instrumentation. |
| Phase duration | Multi-timezone crews miss 24-hour stages | Keep daily UTC stages; consider a 36-hour grace only with missed-stage data. |
| Crew-size guidance | Official-round rate is poor below a measured size | UI says invite at least three; system cap 20. |
| Result archive navigation | Crews complete enough rounds to need browsing | Show only latest reveal now; add private pagination when history exists. |
| Accounts/cross-device recovery | Browser loss or multi-device use is a measured retention problem | Guest session remains primary. |
| Realtime delivery | Poll delay appears in user complaints or interaction data | Keep 30-second visible polling. |
| Native app | Browser crews retain and notifications are the limiting factor | Responsive web only. |
| Fallback board repetition | A crew reaches day 13 on the curated bank instead of authored boards | Author real boards ahead of the rotation. See the gap recorded below; the bank is a safety net, not a cadence. |

## Known gaps recorded 22 August 2026

Verified by reading current code and running the local suites, not by reading
docs. Each row is a true statement about the working tree on this date.

| Gap | Evidence | Consequence | Standing decision |
| --- | --- | --- | --- |
| The curated fallback holds **12 boards** on a `days % 12` rotation, while board numbers increment independently (`vibe_boards.go:138-152`) | `index := days % len(vibeBoardTemplates)` with `number := 47 + days` | Day 13 serves board **#59 carrying #47's exact fragments**. A repeat that announces itself as new is worse than an obvious one: the fragments are the creative material, so a returning crew notices it is titling a palette it has already used. | Treat the bank strictly as an outage cushion. Phase 3 authoring must stay ahead of the rotation window. If the bank is ever the live source for 12 consecutive dates, that is an editorial incident, not a fallback working as designed. |
| The pivot is **uncommitted** | Last commit `fa321d1` (17 Aug); 57 modified and 17 untracked files | Every document here describes the working tree in the present tense. `main` and `vibegrid.onrender.com` still serve the retired hidden-category game. | No document may claim the new product is deployed until a SHA carrying it is live. Re-read this row before writing recruiter or launch copy. |
| `TestPostgresVibeRoundTransactions` **skips** without `DATABASE_URL` | `--- SKIP` on a default `go test ./internal/vibegrid/` | The transaction-internal membership, makers-only, no-self, and one-ballot checks are not exercised by an ordinary local or default CI run. The invariants are asserted by database constraints that a skipped test never reaches. | Production-readiness P0.1 already requires this run. Until it is recorded against a SHA, the transactional guarantees are claimed by construction, not by evidence. |
| `DemoWalkthrough.tsx:7` still mounts the retired `VibeGridGame` | `import { VibeGridGame } from "@/components/VibeGridGame"` | The compatibility argument covers `/p/<id>` links that already exist in the wild. A demo route is not an existing link; it is a live surface teaching the superseded mechanic to a first-time visitor. | Retire the demo route or rebuild it on the make/judge/reveal loop. Compatibility is for inbound history only, never for a surface we still advertise. |

## Change protocol

Any proposal that alters a locked row must include:

1. the user problem and evidence;
2. the revised privacy/reveal contract;
3. schema and migration implications;
4. server and runtime-schema changes;
5. unit, Postgres, smoke, and accessibility checks appropriate to the risk;
6. updated product vision, policy copy, recruiter evidence, and rollback plan.

Visual novelty or additional engineering complexity is not evidence.
