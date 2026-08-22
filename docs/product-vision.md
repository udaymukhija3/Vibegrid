# VibeGrid product vision

**Ratified:** 21 August 2026

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

Every date has one immutable board:

- one evocative prompt;
- twelve short, human-written fragments;
- no categories, group ids, difficulty, timer, mistakes, or answer key.

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
| Day D+2 · Reveal | See authors, votes, winner or tie | Nothing in that result | Recognition and conversation |

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

An official result needs at least three cards and two ballots. With fewer,
VibeGrid still reveals the cards and the actual votes but labels the round quiet
and does not extend the crew streak.

This rule prevents a one-person or two-person room from manufacturing a
“winner,” avoids fake activity, and makes the streak a measure of recurring
group participation rather than one dedicated user’s habit.

Ties are preserved. A social game should not invent a secondary ranking rule
just to force a crown.

## The public practice round

The homepage must teach the full promise without requiring friends or a signup:

1. make a card from today’s real board;
2. judge it against three clearly labeled house cards with authors hidden;
3. see an immediate practice result;
4. create a crew if the loop felt good.

House cards are tutorial fixtures, never presented as real people or activity.
Public practice state is local and disposable. This avoids a hollow landing
page while keeping the private crew as the only durable social product.

## Editorial doctrine

A good board is not twelve synonyms and not three disguised categories. It is a
palette with useful tension.

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

### Editorial QA before freezing

1. Make six titled cards from the palette without reusing the same quartet.
2. Ask whether two different titles can plausibly use at least one shared tile.
3. Remove any four that read like an intended hidden category.
4. Read every title/fragment combination for harassment and private-person risk.
5. Preview the 320px board and the social card.
6. Freeze the date. Never edit a live board; replace a future date before it is
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
- A 3×4 fragment palette—not a 4×4 solved-color grid—is the core silhouette.
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
- Database uniqueness constraints mirror one-card and one-ballot rules.
- Client-generated replay ids survive timeouts; changed replay input is rejected.
- Dated board rows are immutable snapshots. Editorial fallback code may evolve
  without rewriting history.
- Display names are copied into card history; removing a membership cannot make
  old results unintelligible.
- Crew links never enter sitemap, public feeds, or analytics labels.
- No-database mode serves practice and explicitly refuses fake durable crews.
- New product claims need a test, a runtime check, or a documented manual gap.

## Success criteria

Do not optimize for raw page views first. The product is working when small
groups complete recurring social loops.

Initial product measures, once privacy-reviewed analytics exists:

- practice completion → crew creation;
- crew creation → third member joined within 72 hours;
- eligible member make rate per board;
- maker → next-day judge return rate;
- official round rate;
- crews with a second official round within seven days;
- invite rotation/removal frequency as a safety signal;
- quiet-round rate by crew size.

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
