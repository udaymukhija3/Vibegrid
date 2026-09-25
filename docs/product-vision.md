# VibeGrid product vision

**Ratified:** 21 August 2026; crew-sized board contract ratified 23 August 2026;
adversarial identity/session amendment ratified 27 August 2026; first-card
size lock, two-card official floor, sole-plurality crown, and honest practice
ballot amended 24 August 2026 (PR #39) and recorded here 25 September 2026

**Status:** source of truth for product, design, content, and engineering
**Supersedes:** the hidden-category daily puzzle described by the launch report

## The hard truth

The first VibeGrid was a polished NYT Connections clone. Its words were more
internet-native and it had crews, moderation, a Go backend, and unusually
serious operational scaffolding, but the user’s central action was unchanged:
recover four groups chosen by an editor. Multiplayer compared solitary results
after the fact. The visual redesigns could make that loop stylish; none could
make it original.

That work was not wasted. It produced a trustworthy runtime, private group
membership, transactional persistence, admin security, deployment scaffolding,
and a sharp visual vocabulary. But the mechanism had to be replaced before more
design or engineering polish could be justified.

## The thesis

> VibeGrid is a delayed social authorship game. A shared daily constraint gives
> people just enough material to make a revealing little interpretation; blind
> judging turns those interpretations into a recurring conversation about the
> people in the room.

Short version: **Make the vibe. Let the crew decide.**

This is not a game about knowing the answer. It is a game about making something
that feels inevitable once someone names it—and wondering which friend saw it
that way.

## The primitive

Every date has one immutable editorial master:

- one evocative prompt;
- twenty-eight short, human-written fragments;
- no categories, group ids, difficulty, timer, mistakes, or answer key.

Every crew sees a nested, four-column projection of that master. The number of
rows follows current membership until the crew's first card on that dated board
is submitted, and is frozen at that moment:

| Members at first card | Rows | Fragments |
| --- | --- | --- |
| 1–4 | 3 | 12 |
| 5–8 | 4 | 16 |
| 9–12 | 5 | 20 |
| 13–16 | 6 | 24 |
| 17–20 | 7 | 28 |

Opening the room is a read and never decides the size. The natural order is to
make a crew, glance at today's board, and then send the invite; freezing on
first open would lock a crew that fills to twelve before anyone plays onto the
three-row board sized for its founder. Once the first card exists the choice is
frozen for the crew and date: a join, leave, removal, refresh, retry, or second
server cannot resize the palette under existing cards. Because every size is a
prefix of the same master, a selection drafted before a join stays valid. Different crews may therefore receive different amounts
of material on the same date; every member inside one crew receives exactly the
same historical projection. The first 12, 16, 20, 24, and 28 fragments must each
stand as an intentionally edited palette—not padding exposed only at scale.

Every member may make one card:

- choose exactly four fragments;
- arrange no secret order; the selected set is the material;
- write a title of at most 40 characters;
- lock it for that crew and board.

The title is essential. Four fragments alone are a selection; the title is the
interpretive act. “meal prep + monday dread + five tabs + 11pm panic” becomes
“Calendar cosplay.” That authorship is the smallest unit worth sharing and
judging.

## The temporal loop

VibeGrid deliberately spans three days:

| Stage | What a member can do | What stays hidden | Product feeling |
| --- | --- | --- | --- |
| Day D · Make | Choose four and title one card | Everyone else’s cards | Private authorship |
| Day D+1 · Judge | Eligible makers cast one vote for another card | Authors and live tally | Social inference |
| Day D+2 · Reveal | See authors, votes, and a sole winner or a split | Nothing in that result | Recognition and conversation |

The daily crew page stacks these stages: latest reveal first, yesterday’s
ballot second, today’s make third. That creates a reason to return that is not
“consume another puzzle.” Each visit closes one social loop and opens another.

## Why delayed, not live

Live party play would optimize for scheduling, presence, synchronized state,
and loud personalities. VibeGrid is meant to fit the tempo of an existing group
chat. Delayed phases allow different time zones, work schedules, and response
styles while preserving anticipation.

Polling is sufficient for v1. Realtime transport is not a product feature; it
becomes justified only if measured use shows that a 30-second visible-tab
refresh damages the experience.

## Why private crews are the product

The same card has more meaning when made by one of six known people than by one
of six thousand strangers. Private context creates the suspense—“this is either
Priya or Dev”—and makes the reveal conversational.

Consequences:

- crew rooms are capability links, not indexable public content;
- there is no public feed, global follower graph, or engagement leaderboard;
- author names reveal only to current crew members;
- owners can rotate leaked invites and remove members;
- inside jokes are allowed, but targeting or humiliating a member is not;
- the product must feel worthwhile with 3–8 people before it pursues reach.

## Thresholds and quiet rounds

An official result needs at least two cards and two ballots from distinct,
stable crew identities. Two is the honest floor: below it there is nothing to
choose between, and a two-person crew that both makes and judges has run the
ritual exactly as designed. An earlier floor of three cards left that most
common private group permanently unofficial with a streak pinned at zero. With fewer, VibeGrid still reveals the cards and the
actual votes but labels the round quiet and ends the current consecutive crew
streak; a quiet day does not pause a streak.

The floor is held in one pair of constants that the result view and the streak
query both read, so the two cannot drift apart.

This rule prevents one active membership from manufacturing a “winner,” avoids
fake activity, and makes the streak a measure of recurring group participation
rather than one dedicated browser’s habit. It is not proof of unique humans:
VibeGrid has no accounts, so an invited attacker controlling several fresh
browsers can create several anonymous identities. Private-crew trust and owner
controls remain part of the v1 boundary; the UI and documentation must never
describe the threshold as identity verification.

A crown requires a sole plurality. When two or more cards share the top, the
reveal reports a split, sets `tied`, and crowns nobody; a social game should not
invent a secondary ranking rule just to force a crown, and crowning every tied
card would hand the whole crew a crown for the most natural voting pattern
(everyone backing someone else once). At two people the vote is forced and
always splits, so those crews get a round that counts and an honest “nobody took
it outright.”

## The public practice round

The homepage must teach the full promise without requiring friends or a signup:

1. make a card from today’s real board;
2. judge it against three clearly labeled house cards with authors hidden;
3. see an immediate practice result;
4. create a crew if the loop felt good.

House cards are tutorial fixtures, never presented as real people or activity.
Their titles belong to the editorial template alongside the prompt and
fragments. The practice ballot plays out like a four-person crew: every card
casts one non-self vote, the house reading the player's card through a stable
hash of its title and fragments. Practice can therefore be won, tied, or lost,
and replaying an identical card always gives the identical result, so the
outcome cannot be rerolled. It must never be rigged toward the player's pick or
against the player.
Public practice state is local and disposable. This avoids a hollow landing
page while keeping the private crew as the only durable social product.

Practice deliberately uses four rows (16 fragments). It makes the familiar
four-column interaction immediately legible without restoring hidden groups:
there is still no partition, correctness, mistake budget, or result grid. The
player authors one titled card and judges interpretations.

The playable composer is the homepage—not a destination below a marketing hero
or an interstitial explanation screen. A concise first-visit dialog explains
make → judge → reveal over the real board, can be dismissed with one action, is
remembered by that browser, and remains available from “How it works.” Closing
it leaves focus at the first fragment. Returning visitors land directly on the
composer. Product explanation must never become a second front door before the
first meaningful action.

### Unlimited practice

Some people will understand the mechanic only after making several cards, and
some will simply want to keep playing. The public surface therefore has an
explicit **Unlimited** mode. Completing make → blind house ballot → reveal can
deal another 4×4 palette immediately.

Unlimited is a sandbox, not a second social economy:

- card titles, selections, and votes remain disposable browser state;
- it has no timer, lives, correctness, score, leaderboard, streak, account, or
  fabricated public participation;
- it never advances, backfills, or otherwise changes a crew's dated stages;
- each deal stays inside one coherent 28-fragment editorial master; deterministic
  rotation and coprime stepping vary the visible 16 without mixing prompts;
- the activity can continue indefinitely, but the curated source material is
  finite and will eventually cycle. The product must never call a reshuffle a
  newly human-authored board.

This is the right kind of abundance: unlimited opportunities to interpret,
while the daily crew reveal remains scarce, relational, and worth returning for.
If Unlimited begins to reduce crew creation or judge return, it is subordinate
and may be moved later in the funnel.

## Editorial doctrine

A good board is not twenty-eight synonyms, seven disguised categories, or twelve
good fragments followed by filler. It is a layered palette with useful tension.

### A good prompt

- asks the player to build, cast, diagnose, or name a recognizable situation;
- is specific enough to provoke taste but broad enough for competing readings;
- concerns behavior, atmosphere, or self-recognition more often than trivia;
- can produce affectionate, cynical, chaotic, and sincere cards;
- reads cleanly aloud and in a shared preview.

Examples:

- Build a first impression that will not survive the evening.
- Build the person every group chat eventually creates.
- Build an airport personality you would deny having.
- Build a wellness era with a short expiry date.

### A good fragment set

- contains concrete actions, objects, phrases, and micro-signals;
- varies emotional valence and specificity;
- gives several fragments more than one plausible role;
- supports at least six compelling cards in an internal editorial pass;
- contains no duplicate text, concealed demographic target, answer-key coding,
  or single obviously dominant quartet;
- stays short enough to read on a phone tile.
- remains generative at every nested breakpoint: first 12, 16, 20, 24, and all
  28 fragments;
- uses the later rows to widen tone and possibility, not introduce an intended
  category or a more obviously correct quartet.

### Editorial QA before freezing

1. Make six titled cards at 12 fragments, then repeat at 16, 20, 24, and 28.
2. Ask whether two different titles can plausibly use at least one shared tile.
3. Remove any four that read like an intended hidden category.
4. Check that each added row expands choices rather than merely diluting them.
5. Read every title/fragment combination for harassment and private-person risk.
6. Preview 320px at 3, 4, and 7 rows plus desktop and the social card.
7. Freeze the date. Never edit a live board; replace a future date before it is
   first persisted or choose another date.

## Design doctrine

The launch set had one valuable instinct: a recognizable feed silhouette before
the wordmark. The new system keeps that rigor but assigns it to the new mechanic.

- Deep ink is the continuous ground.
- Cream cards are physical authored objects.
- Lime marks action and readiness; amber is anticipation; coral is a boundary;
  violet marks subjective selection and judgment.
- Bricolage Grotesque carries voice; IBM Plex Mono carries dates, counts, and
  system truth.
- Hard offset shadows make cards feel handled, not like generic SaaS panels.
- A four-column stack of tactile fragments is the core silhouette. Its height
  changes with the room; solved colour bands never appear.
- Motion is small, responsive, and reduced-motion safe. No confetti is required
  to manufacture significance.

Toy supplies the big hierarchy, Arcade supplies the dark social energy, Sticker
supplies tactility. Main and Terminal are references for restraint and density,
not alternate skins. The product has one visual direction.

## Engineering doctrine

Seriousness means the social rules remain true under retries, refreshes,
concurrency, removed members, leaked links, and partial participation.

- The server, not the client, owns phase projection and disclosure.
- Membership is checked inside each mutation transaction.
- A durable `(crew, anonymous identity) → member id` mapping survives leave and
  removal. Rejoining restores the same round identity, card, and ballot limits;
  deleting the crew removes the mapping.
- Database constraints mirror one-card and one-ballot rules and make cross-round
  vote targets and self-votes impossible even outside the application path.
- Client-generated replay ids survive timeouts; changed replay input is rejected.
  Every crew mutation also honors `Idempotency-Key`, so a network retry replays
  one stored response instead of treating the header as decoration.
- Blind ballot order is stable but derived from cryptographically random card
  ids, never submission time or live roster activity.
- The browser cookie is a revocable bearer, not the durable identity itself.
  Postgres stores only its SHA-256 hash; a member can replace a suspected leaked
  browser key without losing crew history.
- A public document view may mint the browser cookie but does not create a
  database row. First API bootstrap maps new bearers to a separate one-way-
  derived identity; only cookies proven to own pre-migration rows retain their
  legacy value. Active session rows refresh at most hourly, so a 30-second crew
  poll is a read rather than 2,880 writes per idle tab per day. Unknown-session
  creation also has a source-network ceiling before it can write.
- Every cookie-authenticated crew mutation requires an explicit same-origin CSRF
  token in addition to JSON/CORS framing.
- Last-member leave is an acknowledged destructive operation because it deletes
  the crew, cards, ballots, results, invite, and durable member mappings.
- Untrusted display names and titles reject control, bidi, and invisible Unicode
  format characters that could disguise owner controls or blind cards.
- In-process public-read rate-limit state is bounded with eviction; exhausting
  its key budget cannot turn the key ceiling into a global first-visitor outage.
- Dated board rows are immutable snapshots. Editorial fallback code may evolve
  without rewriting history.
- Per-crew row count is inserted once, when a member first submits a card for
  that board, under the same crew lock used by joins and leaves. The freeze runs
  before title and selection validation, so a rejected first attempt still fixes
  the size. Opening the room only reads the lock. The database uniqueness key is
  the final arbiter for concurrent submits.
- The original 12-fragment row and its 16-fragment expansion are stored
  separately so the schema migration is additive and the prior binary can still
  boot during rollback. A roll-forward is required after cards use expansion ids.
- Display names are copied into card history; removing a membership cannot make
  old results unintelligible.
- Crew links never enter sitemap, public feeds, or analytics labels.
- No-database mode serves practice and explicitly refuses fake durable crews.
- Unlimited deals are deterministic public reads; selections and tutorial votes
  never enter Postgres or operational logs as authored content.
- New product claims need a test, a runtime check, or a documented manual gap.

Reminders follow the same identity rule. A push subscription is keyed by the
stable anonymous identity, never the browser bearer, so replacing a leaked key
keeps reminders attached and the table holds no plaintext credential. The
reminder is one opt-in digest per crew per day, silent when nothing is waiting,
and ships off until VAPID keys are configured.

The guest-session recovery boundary is intentionally narrow. Replacing a live
browser key revokes a copied cookie while preserving identity; clearing all
browser data or losing the only device still loses the anonymous account. That
is an honest v1 limitation, not cross-device recovery.

## Success criteria

Do not optimize for raw page views first. The product is working when small
groups complete recurring social loops.

Initial product measures, once privacy-reviewed analytics exists:

- practice completion → crew creation;
- daily-practice versus Unlimited completion → crew creation, so the sandbox is
  tested for teaching value rather than assumed to help;
- crew creation → third member joined within 72 hours;
- eligible member make rate per board;
- maker → next-day judge return rate;
- official round rate;
- crews with a second official round within seven days;
- invite rotation/removal frequency as a safety signal;
- quiet-round rate by crew size.
- card overlap, completion time, and abandonment by frozen row count.

The north-star candidate is **weekly crews with two or more official reveals**.
It measures authored participation, judgment, and return behavior without
rewarding public virality or one-user streak farming.

## Non-goals for v1

- Rebuilding hidden categories under new art.
- Public accounts, profiles, following, DMs, or a global social graph.
- Live rooms, presence, chat, matchmaking, or synchronized countdowns.
- A public card feed or popularity leaderboard.
- AI-generated automatic publishing.
- Monetization, subscriptions, ads, or collectible economies.
- Infinite boards, user-authored fragment sets, or competitive ranked modes.
- Native apps before the browser loop shows retention.

## Kill tests

Pause or change the product if evidence shows any of the following after a
meaningful controlled beta:

- people enjoy practice but do not invite a third member;
- crews make once but do not return to judge;
- most cards converge on one obvious quartet, meaning editorial constraints are
  functioning like hidden answers;
- reveal creates discomfort more often than conversation;
- small crews cannot reach official thresholds and do not grow;
- the delayed loop is consistently forgotten even after notification work is
  considered.

More engineering will not repair a failed social loop. Validate the loop before
accounts, realtime, mobile apps, or scale work.

## The one-sentence test for every feature

> Does this help a small existing group make more revealing interpretations,
> judge them more fairly, or return for the reveal?

If not, it is probably not VibeGrid.
