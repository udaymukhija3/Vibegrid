# VibeGrid decision register

**Ratified:** 21 August 2026; variable-row amendment 23 August 2026;
adversarial identity/session amendment 27 August 2026; round-counting amendment
24 August 2026 (PR #39, recorded here 25 September 2026). A row
changes only when product behavior, tests, and this document change together.

## Locked product decisions

| Area | Decision | Reason |
| --- | --- | --- |
| Category | Delayed social authorship game | The authored card and known crew create the differentiated value; hidden grouping is legacy only. |
| Primary unit | One private crew of 3–8 known people | Meaning and reveal suspense depend on relationship context. System cap remains 20. |
| Daily input | One prompt plus an immutable 28-fragment editorial master; no grouping metadata | Supports every crew-size projection without generating content at request time. |
| Crew board size | Four columns; 1–4 members = 3 rows, 5–8 = 4, 9–12 = 5, 13–16 = 6, 17–20 = 7 | More participants receive a wider creative palette while the core act remains choosing four. |
| Size snapshot | Opening the room is a read that projects an unstarted board from current membership; a member's first card submission for a crew/date freezes the band, and later joins, leaves, and removals do not resize it | Cards, retries, and open clients must refer to one stable palette once play starts. Freezing on first open let whoever looked first (usually the founder, before sending the invite) size the round for a crew of one. Crew locks linearize membership changes against the first submission. |
| Player output | Exactly 4 fragments plus a 1–40 rune title | Four creates a memorable card; the title makes the selection an authored interpretation. |
| Answer model | No answer key, category mapping, correctness, difficulty, timer, or mistake count | The crew evaluates resonance, not editor intent. |
| Phases | Day D make; D+1 blind judge; D+2 reveal | Delayed stages create return behavior and fit an async group-chat tempo. |
| Judge eligibility | Only members who made a card in that round | Voting is reciprocal participation, not spectator influence. |
| Ballot | One vote for another member’s card; no self-vote | Simple, legible, and resistant to self-promotion. |
| Reveal | Author names and final counts reveal to current crew members | The social recognition is the payoff; blind authorship is temporary. |
| Official result | At least 2 cards and 2 votes (`minOfficialVibeCards`, `minOfficialVibeBallots`, read by both the result view and the streak query) | Two people who both make and judge have run the ritual as designed; a three-card floor pinned the most common private group at a zero streak. Below the floor, reveal honestly as a quiet round and do not extend streak. |
| Crown and ties | A crown requires a sole plurality; a level top is reported as a split (`tied`) and nobody is crowned | No arbitrary tiebreaker or earliest-submit advantage, and no crowning the whole crew when everyone backs someone else once. Two-person rounds always split and say so. |
| Streak | Consecutive official crew results through the latest revealed date; a quiet revealed date resets rather than pauses it | Measures uninterrupted group recurrence, not one person opening the app. This hard-reset behavior was explicitly retained after adversarial review. |
| Public experience | The homepage opens directly on complete local practice: make, house ballot, immediate reveal | The playable board is the front door; a dismissible, remembered first-visit dialog teaches the loop without becoming a landing-page gate. No fake people, signup, or durable public content. |
| Practice ballot | House titles come from the editorial template; every card casts one non-self vote, the house reading the player's card through a stable hash | The previous reveal gave three votes to the player's pick and one to the player every time, so a newcomer could only lose to a bot. Practice can now be won, tied, or lost, and an identical card always replays to the identical result. |
| Unlimited practice | A clearly labeled local 4×4 sandbox can deal another deterministic palette after every reveal | It satisfies repeat play without adding correctness, scores, persistence, or a second fake community. The finite curated masters may cycle; Unlimited never changes crew stages or streaks. |
| Identity | A revocable HttpOnly browser bearer maps server-side to a stable anonymous identity; per-crew display name | A leaked cookie can be replaced without rewriting membership/history, while the product keeps no-account onboarding. Token hashes—not bearer values—are stored. Browser-data loss still has no recovery. |
| Invite security | Rotatable capability code separate from internal crew id | A leaked link can be revoked without destroying history. |
| Ownership | Creator owns; owner leave transfers to longest-standing member; last leave deletes only after explicit destructive confirmation | Keeps access control operable while making the cascade over cards, ballots, results, invite, and identity mappings visible before it runs. |
| Privacy | Nonmembers get crew identity plus the current editorial prompt/palette and aggregate counts; roster, cards, ballots, votes, and results are member-only | An invite can explain the room before joining without disclosing anything authored by its people. |
| Board lifecycle | First persisted master for a date and first crew-size snapshot for a crew/date are immutable | Retries, open clients, cards, and historical results must share one palette. |
| Editorial supply | Human-authored scheduled board wins; deterministic curated bank fills a missing date | No empty day, no automatic AI publishing, and history remains frozen. |
| Timezone | UTC globally | One unambiguous rollover for all crews; deployments and defaults match. |
| Realtime | Visible-tab polling every 30 seconds | Adequate for async play; streaming waits for evidence. |
| Search | Index only product-level public pages; never crew or compatibility links | Crews are private capability spaces and old `/p` grids are not the active product. |
| Visual direction | One Toy × Arcade × Sticker launch system | Big tactile hierarchy, dark social energy, and a recognizable four-column stack; no theme chooser. |
| Legacy | Old `/p` URLs and APIs remain compatibility-only | Preserve old shares without letting legacy mechanics govern the roadmap. |

## Locked engineering decisions

| Area | Decision | Proof obligation |
| --- | --- | --- |
| Mutation authority | Membership and invariants are checked in a Postgres transaction | Integration tests cover outsider, duplicate, replay, eligibility, self-vote, and one-ballot behavior. |
| Idempotency | Client card/vote ids are content-stable; every crew mutation honors `Idempotency-Key`; exact replay returns the original response and changed input conflicts | Client and server retries converge under lost responses, double presses, cold starts, and concurrent identical requests. Release tests assert one domain row. |
| Disclosure | Go builds member-aware make/judge/result projections | Unit tests assert outsiders see no private data and judge cards omit authors. |
| Schema | Stable crew-member mappings survive leave/removal; unique and foreign-key/check constraints mirror one-card, replay, one-ballot, same-round target, and no-self-vote invariants | Migrations and real-Postgres adversarial tests, including direct impossible-row inserts. |
| Variable palette storage | Keep the legacy 12 fragments in `vibe_daily_boards`; store 16 immutable expansion fragments additively; freeze `tile_count` in `vibe_crew_boards` | Previous binary can boot after migration; current code validates expansion selections inside the card transaction. |
| Historical identity | Card stores the author display-name snapshot; a durable crew identity mapping restores the same opaque member id on rejoin | Removing a membership cannot orphan visible history or mint a clean card/ballot slate. The mapping dies only with the crew. |
| Guest request security | Hash-only revocable bearer records plus explicit CSRF on every cookie-authenticated crew write | Fresh-session, copied-cookie revocation, exact rotation replay, missing/wrong CSRF, and hash-storage integration tests. |
| Guest-session scale | Public HTML mints no durable row; new bearer and identity are distinct; active polling refreshes storage at most hourly; unknown-session bootstrap is network-limited | Prevents public traffic and idle 30-second crew polling from becoming unbounded Postgres writes while retaining legacy browser access. |
| Blind ordering | Judge cards are ordered by random immutable card id, not creation time | Adversarial test proves ballot position does not reproduce observed submission order. |
| Bounded read limiter | In-memory key ceiling evicts least-recently-used entries instead of rejecting every unseen identity | Capacity regression plus shared-network first-visitor tests. |
| Public fallback | Practice works without DB; crew writes return explicit 503 | No in-memory fake of a durable social system. |
| Admin board authoring | Future/today date, one prompt, 28 distinct fragments, strict insert | Frozen date conflicts; first stored master and expansion win together. |
| Runtime shape | Exported Next.js embedded into one Go binary with same-origin API | Docker/build/E2E smoke. |
| Operational truth | External provider state is manual until verified | Docs separate code evidence from backups, alerts, domains, and traffic claims. |

## Explicitly rejected ideas

| Idea | Why rejected now |
| --- | --- |
| “Connections, but multiplayer” | The central action remains derivative and multiplayer is merely comparative. |
| Real-time co-op selection | Changes the product into scheduling and conflict management before the async loop is validated. |
| Endless durable crew rounds | Removes the delayed reveal that gives known people a reason to return and creates a much larger concurrent-state product. Unlimited remains disposable practice. |
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
| Crew-size formula | Controlled crews show scan cost or card quality is materially worse in a membership band; measure overlap, time-to-submit, completion, and abandonment by frozen row count | Keep the ratified 3–7 row bands. Change formula only with a migration and historical compatibility plan. |
| Notifications | Crews make but fail to return for judge/reveal | No push/email. Test an optional calendar or web-push reminder only after funnel instrumentation. |
| Phase duration | Multi-timezone crews miss 24-hour stages | Keep daily UTC stages; consider a 36-hour grace only with missed-stage data. |
| Crew-size guidance | Official-round rate is poor below a measured size | UI says two people are enough for a round that counts; system cap 20. |
| Result archive navigation | Crews complete enough rounds to need browsing | Show only latest reveal now; add private pagination when history exists. |
| Accounts/cross-device recovery | Browser loss or multi-device use is a measured retention problem | Guest session remains primary. |
| Realtime delivery | Poll delay appears in user complaints or interaction data | Keep 30-second visible polling. |
| Native app | Browser crews retain and notifications are the limiting factor | Responsive web only. |
| Fallback board repetition | A crew reaches day 13 on the curated bank instead of authored boards | Author real boards ahead of the rotation. See the gap recorded below; the bank is a safety net, not a cadence. |

## Known gaps verified 23 August 2026

Verified by reading current code and running the local suites, not by reading
docs. Each row is a true statement about the working tree on this date.

| Gap | Evidence | Consequence | Standing decision |
| --- | --- | --- | --- |
| The 16 new expansion fragments per fallback board have code validation but not a retained editorial playtest artifact at every 12/16/20/24/28 breakpoint | The bank contains 28 unique bounded fragments per board and unit validation passes; no human card-generation sheet is checked in | Technical capacity is proven; editorial quality at seven rows is not | Treat the expanded bank as beta material. Complete the nested editorial QA before a broad launch and replace weak future palettes through `/admin`, never by mutating a frozen date. |
| The curated fallback holds **12 boards** on a `days % 12` rotation, while board numbers increment independently (`VibeBoardForDate`) | `index := days % len(vibeBoardTemplates)` with `number := 47 + days` | Day 13 serves board **#59 carrying #47's exact fragments**. A repeat that announces itself as new is worse than an obvious one: the fragments are the creative material, so a returning crew notices it is titling a palette it has already used. | Treat the bank strictly as an outage cushion. Phase 3 authoring must stay ahead of the rotation window. If the bank is ever the live source for 12 consecutive dates, that is an editorial incident, not a fallback working as designed. |
| The public origin serves the pivot, but release provenance is not exposed | On 23 Aug 2026 the repository smoke passed against `https://vibegrid.onrender.com`: health/readiness, product pages, board 051 from `/api/vibes/today` with 12 fragments, guest session, and search boundaries all passed. The running service does not expose a deploy SHA. | The product is demonstrably live, but an operator cannot prove which commit is running or select a rollback solely from runtime evidence. The deployed mutating crew path was not exercised during this read-only smoke. | Record the deployed SHA in release metadata or a protected diagnostic, retain the CI/deploy link, then run the mutating smoke deliberately against a disposable production crew. Do not infer backups, alerts, or rollback readiness from a healthy origin. |

Closed findings are removed from this table rather than left to misdescribe the
repository. The former demo routes now mount the active practice loop, their
dead hidden-group components are deleted, and the Postgres suite covers both
distinct-client contention and 16 simultaneous identical card/vote replays.

## Adversarial audit disposition — 27 August 2026

The 26 August hostile audit was treated as executable evidence, not as a product
specification. Its one open P1 and every concrete suspected implementation gap
are now closed in code and regression tests:

| Finding | Disposition |
| --- | --- |
| Leave/rejoin mints a clean member id and permits extra cards, ballots, self-votes, and a manufactured official result | **Closed.** Durable crew identity mappings preserve the member id across leave, removal, and rejoin; the previously red exploit tests now pass. |
| Copied guest cookie cannot be revoked | **Closed.** Hash-only bearer records and replay-safe rotation invalidate the old token without moving the stable identity. |
| In-memory read limiter fails closed after 10,000 keys | **Closed.** Bounded LRU eviction admits new identities without unbounded memory. |
| Database admits cross-round vote targets and self-votes | **Closed.** Composite target foreign key plus no-self-vote check; direct SQL regression included. |
| Crew CSRF depends only on JSON/CORS framing | **Closed.** Explicit token required on all cookie-authenticated crew mutations. |
| Last-member leave silently destroys history | **Closed.** API and UI require explicit deletion confirmation under the crew lock. |
| Titles accept invisible/bidirectional format characters | **Closed.** Format characters are rejected in card titles, crew names, and display names. |
| `Idempotency-Key` is inert on crew mutations | **Closed.** All crew writes are wrapped; exact, changed-input, and concurrent-create behavior is tested. |
| Quiet round resets rather than pauses streak | **Retained product rule.** “Consecutive official results” means a quiet revealed date ends the streak. |
| Displayed board number saturates after roughly year 2318 | **Accepted cosmetic limit.** Date-derived board ids remain distinct; no reachable data collision exists. |
| Postgres tests can disappear from a green local run | **Closed as test truth.** The default command warns loudly without `TEST_DATABASE_URL`; `test:backend:postgres`, `test:all`, and CI require the durable path. E2E refuses to pass without `DATABASE_URL`. |

## Change protocol

Any proposal that alters a locked row must include:

1. the user problem and evidence;
2. the revised privacy/reveal contract;
3. schema and migration implications;
4. server and runtime-schema changes;
5. unit, Postgres, smoke, and accessibility checks appropriate to the risk;
6. updated product vision, policy copy, release evidence, and rollback plan.

Visual novelty or additional engineering complexity is not evidence.
