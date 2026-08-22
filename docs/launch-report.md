# VibeGrid — Launch Report (historical; superseded)

**Date:** 2026-08-20
**Decision update:** 2026-08-21
**Status:** **OVERRULED as a product strategy. Retained only as a record of the
pre-pivot audit.**
**Scope:** content audit, design audit, new design direction, and go-to-market for a real product launch
**Framing:** this is a product you intend to ship, not a portfolio piece. Every recommendation below assumes an audience of players, not hiring managers.

---

## 0. Supersession notice — what changed and why

This report correctly diagnosed several execution problems in the old product:
the interface lacked a coherent visual voice, crews were buried, private links
needed stronger landing states, the daily content supply was too small, and the
deployment story still had manual gaps. It then made the wrong strategic move:
it treated those problems as obstacles to launching a better **Connections-style
grouping puzzle**.

That premise has been overruled.

Polishing the board, writing 120 more hidden-category puzzles, improving its share
grid, and placing multiplayer around the result would have made VibeGrid a more
credible competitor, but not a sufficiently original product. The central act
would still have been selecting four pre-authored items to recover a correct
answer under mistake and time pressure. “Connections, but situations and moods”
is positioning language, not durable product differentiation.

The 2026-08-21 decision replaces the old game with an asynchronous private-crew
authorship ritual. The authoritative specification is
[`docs/product-vision.md`](product-vision.md), and the irreversible decisions are
recorded in [`docs/decision-register.md`](decision-register.md). Where this report
conflicts with either document, those documents win.

### 0.1 The new vision

> **VibeGrid is a daily private-crew ritual where friends use the same fragments
> to make a tiny portrait of their day, then discover whose interpretation the
> crew felt most.**

The fragments are prompts for expression, not clues to a hidden taxonomy. There
is no canonical grouping and no correct solution. The creative and social output
of the crew is the content.

Every daily board contains one human-written prompt and exactly 12 fragments. A
round unfolds across three UTC dates:

1. **Make — day D.** Each crew member privately chooses four fragments and gives
   the combination a short title. There is no timer, difficulty, mistake counter,
   hint, or answer validation.
2. **Judge — day D+1.** Only people who made a card may vote. They see the eligible
   cards without authors and cast exactly one non-self vote. They cannot revise
   the ballot or vote twice.
3. **Reveal — day D+2.** Authors, vote totals, ties, and crew history become
   visible. Ties remain ties; the product does not manufacture a winner.

A result is official only when at least three cards and two ballots exist.
Undersized rounds end quietly and do not extend the crew streak. This makes the
streak evidence of a continuing group ritual, not individual compulsion.

The public homepage offers a complete local practice round so a visitor can
understand the emotional act without entering a crew. Durable play belongs in a
private crew reached through an invite capability link. Crew pages, claims,
legacy puzzle shares, demos, admin pages, and API routes are excluded from search.

### 0.2 Exactly what this report overruled

| Earlier recommendation or assumption | 2026-08-21 ruling | Reason |
|---|---|---|
| The product is a daily 4×4 hidden-group puzzle | **Removed from the primary product** | It left the core interaction recognisably derivative even with better writing. |
| “Connections, but for feelings” is the position | **Rejected** | It defines VibeGrid through another product and describes flavour, not a new behavior. |
| Build a runway of 90–120 category puzzles | **Cancelled** | The new editorial unit is a 12-fragment expressive prompt, with no answer key or fairness graph. |
| Difficulty modes, four mistakes, one-away feedback, elapsed time | **Removed** | They reward answer recovery and performance; the new product rewards authorship and recognition. |
| The solved result and emoji grid are the growth engine | **Replaced** | The primary retention engine is the make → blind judgment → named reveal cycle inside a real crew. |
| Crews are a secondary wedge placed after solo play | **Reversed** | The crew is the product. Solo practice explains it; it is not the durable loop. |
| Public puzzle archive should compound through SEO | **Rejected for private play** | Crew expression is intimate and capability-linked. Indexing those surfaces would violate the product's social contract. |
| Community submissions should feed the daily solution bank | **Deferred and re-scoped** | Any future community system must author prompts/fragments and must not reintroduce hidden answers. |
| Alternate-solution embeddings are the right AI investment | **Cancelled** | There is deliberately no canonical solution to police. |
| Per-puzzle cold landing should expose a friend's solved result | **Replaced** | Private crew state is disclosed only to members and only at the correct stage. Public acquisition uses a spoiler-free practice round. |

### 0.3 What survives from the report

The pivot is not a rejection of all the work below. These findings remain valid
and have been carried into the rebuilt product:

- **Human editorial voice matters.** Prompts and fragments stay curated; there is
  no auto-published generation path.
- **Crews are the defensible social foundation.** They have moved from a feature
  to the primary product boundary.
- **The Toy/Arcade/Sticker design exploration was directionally right.** The live
  system now fuses tactile cards, hard offset shadows, a deep-ink ground, cream
  surfaces, four signal colours, Bricolage Grotesque, and IBM Plex Mono.
- **Cold starts, domains, analytics, backups, and launch operations still matter.**
  They remain release work, but they cannot substitute for a differentiated core.
- **Claims must be evidenced.** Product, engineering, security, deployment, and
  manual-provider work are separated in the production and recruiter evidence
  documents.

### 0.4 Current implementation status

The superseding vision is not merely a proposal. It is represented in the current
codebase by the 12-fragment board model, immutable daily board publishing,
Postgres-backed cards and ballots, replay-safe mutations, stage-aware disclosure,
make/judge/reveal crew interfaces, the local practice round, the board-authoring
desk, updated indexing boundaries, and the fused visual system.

The old `/p/<id>` hidden-category engine is retained only as labelled, no-index
compatibility for existing links. It is not the VibeGrid product direction and
must not drive new roadmap, positioning, content, or design decisions.

---

## Historical report below

Everything below records the 2026-08-20 analysis in its original strategic
context. Recommendations are not current unless explicitly preserved in §0.3.

---

## 1. The short version

VibeGrid is further along than most launched daily puzzle games. The engine is
server-authoritative and transactional, the social layer (crews) is built and is
genuinely differentiated, and — the thing that actually matters — **the writing
is good.** "Productivity cosplay." "Sunday scaries." "Gate gremlin." That voice
is the product.

Three things stand between it and a launch, in order of how much they cost you:

| # | Blocker | Severity | Cost to fix |
|---|---|---|---|
| 1 | **12 puzzles in daily rotation** — a 12-day loop | Launch-blocking | 90–120 puzzles of runway, or a sustainable pipeline |
| 2 | **The UI reads as machine-generated** | Converts acquisition into bounce | ~1 week of focused work |
| 3 | **No cold-landing screen; `onrender.com` in every share** | Caps the growth loop at its narrowest point | ~1 day |

Item 3 is the cheapest and highest-leverage thing on this list. Do it first.

---

## 2. What exists today

Verified against the repo, not the docs.

**Working and live** at `vibegrid.onrender.com`:

- Daily 4×4 grid, three modes (`easy` / `standard` / `hard`), one-away feedback,
  4-mistake terminal failure, live elapsed timer
- Server-side guess validation — the browser never receives tile→group mappings
- Guest play with no signup; attempts durable for 30 days via Postgres
- Emoji share grid (`buildShareGrid`, `src/lib/game.ts`)
- Streaks, archive, per-puzzle stats gated behind `MIN_STATS_PLAYERS=20`
- Community puzzle creation → review queue → unlisted-by-link play
- Crews: invite-link groups, rotatable invite codes, owner controls, spoiler-safe
  boards (other members' grids are not loaded until your own attempt is finished)
- Admin desk, moderation queue, appeals, audit log
- Ops: `/healthz`, `/readyz`, bearer-gated `/metrics`, structured logs, rate limits,
  transactional notification outbox, nightly encrypted backups, CI-gated deploys,
  Dependabot, four runbooks

**Deliberately absent:**

- **Product analytics.** `docs/production-readiness.md:1018` lists PostHog/Plausible
  as a P1 item; nothing is installed. There is Prometheus for ops, but no retention
  cohorts, no funnel, no share→visit→play conversion. **You cannot run a launch blind.**
- Accounts (anonymous cookie identity only — so no cross-device sync)
- Any fairness or solution-uniqueness guarantee; validation is structural only

---

## 3. Content audit

### 3.1 How much there really is

| Set | Puzzles | Where it's used | Status |
|---|---|---|---|
| `PuzzleBank()` | **12** | The live daily, rotated by date | **Active — this is what players get** |
| `SeedPuzzles()` | **9** | Dated 2026-06-02 → 06-10 | **Expired.** Reachable only via `/archive` |
| `PuzzleTemplates()` | **6** | `/create` starter packs | Walled off from the daily so answers can't spoil it |

**27 puzzles total = 108 groups = 432 written tile phrases.**

But the number that governs the product is **12**. `bankPuzzleSource` rotates the
bank by day whenever nothing is explicitly scheduled, so:

- A daily player sees their first repeat on **day 13**
- They have seen every puzzle in the product within **two weeks**
- `/archive` lists only the 9 expired June seeds — none of the puzzles they've
  actually been playing

A daily-ritual game's entire promise is *there is a tomorrow*. Marketing into a
12-day loop converts acquisition directly into churn, and you only get to launch
on Hacker News once.

**Target before any launch push: 90–120 puzzles** (~4 months of runway) plus a
cadence that keeps ahead of players.

### 3.2 Is any of it AI-generated? No.

Checked properly rather than trusting the docs:

- `backend/go.mod` has four direct dependencies — `pgx`, `lib/pq`, `goose`,
  `x/sync`. No AI SDK, no HTTP client for one.
- Repo-wide grep for `openai|anthropic|claude|gpt-|llm|gemini|huggingface` returns
  no real hits (only `.claude` in `.dockerignore` and the substring "allM" inside
  `vi.restoreAllMocks()`).
- Every puzzle is a literal Go struct in `puzzle_bank.go`, `seed.go`, and
  `puzzle_templates.go`. There is no generation path and no admin LLM assist.

**100% hand-written.** Worth naming the irony: the content is entirely human and
it's the best thing here; the UI is the generated part. That's backwards from
where the value is.

### 3.3 A content rule you need before you scale

Measured across the 12 bank puzzles (185 tile phrases parsed):

```
mean 11.6 chars   median 12   p90 15   max 18
24 tiles are 15+ characters
```

Longest offenders: `between-set selfie` (18), `where's the locker` (18),
`abandoned by noon` (17), `noise-cancelling` (16), `three paragraphs` (16).

At 375px phone width, a 4-column grid with 14px page padding and 6px gaps leaves
roughly **74px of text width per tile.** "abandoned by noon" needs three lines at
11.5px. NYT Connections gets away with a 4×4 because it uses mostly single words;
your phrases are 2–4 words — and that is precisely what makes them good.

**Recommendation:** enforce a ceiling (~16 characters) in
`AdminPuzzleInput.Validate`, so quality doesn't quietly drift as you scale from
12 puzzles to 120. This is a content-design constraint, not a nitpick: without it,
the phrases that make VibeGrid distinctive become the phrases that break its layout.

### 3.4 Should you use AI to generate puzzles?

**No — not for the daily.** The voice is the moat and volume won't help you if the
writing regresses to the mean. Two better uses:

1. **LLM-assisted drafting with you as editor**, through the existing `/admin` desk.
   Never auto-publish. Treat it as a first-draft generator you reject most of.
2. **The embeddings-based alternate-solution checker** already in your plan. This
   is the right AI story: offline, data-light, and it fixes your real known gap
   (no fairness/uniqueness guarantee) instead of papering over a non-problem.

The cheapest long-term answer to content is **the community queue you already
built.** Promoting reviewed submissions into the daily rotation is 80% shipped.

---

## 4. Design audit — why it reads as machine-generated

Your instinct was right. Here is the evidence, counted from source:

| Tell | Evidence | Why it reads that way |
|---|---|---|
| Everything shouts | `font-extrabold` **42×** | No hierarchy. When all text is max weight, nothing is emphasised. |
| **Inter never loads** | zero `next/font` imports; `globals.css:38` names Inter | You have been looking at system-ui (SF/Segoe) the whole time. The design never saw its own typeface. |
| One radius everywhere | `rounded-lg` **55×**, vs 1 `md`, 1 `sm` | Same corner on a 44px icon button and a full-page panel. |
| Everything floats | 22 soft shadows + `hover:-translate-y-0.5` | Uniform elevation = no depth hierarchy. |
| Figma-default palette | mint / tomato / yolk / plum / pool | Five saturated pastels on off-white. |
| Generative background | radial blobs + 52px grid overlay (`globals.css:20-27`) | The canonical AI-UI background. Says nothing about the product. |

**The structural problem is bigger than any of these.** `.vg-desk` is a
three-column dashboard — dark spine rail, board, control rail — wrapped around a
4×4 grid. Wordle and Connections are a single centred column with almost nothing
around them, because the game is a 30-second moment of focus. VibeGrid
dashboard-ifies a ritual, and the rail spends its space narrating what the board
already shows (a stat cell reading "Selected 2/4" beside four visibly selected
tiles).

**The chrome competes with the game.** Fixing the palette alone would produce a
prettier dashboard.

---

## 5. The new design

**Canvas:** https://claude.ai/code/artifact/419fd9fb-7ad4-4a1f-b8b8-6102405ed591
(opens on the launch set; earlier A/B/C explorations are on the second page — you
can export any artboard as PNG)

### 5.1 The direction

A fusion of the two you responded to: **tactile chunk + mono system.**

- **Hard offset shadows, never blurs.** This is the single change that most
  separates it from the current look.
- **IBM Plex Mono** carries every number, label, timer and counter.
  **Bricolage Grotesque** carries the words.
- **Deep ink ground** (`oklch(0.19 0.028 265)`) so tiles and the share grid pop in
  a screenshot — which is how most people will first see this.
- **Four fixed vibe colours** — lime, amber, coral, violet — identical across the
  board, the result grid, the crew rows, the chat message and the link preview.
  The goal is that the grid is recognisable in a feed *before* the wordmark is.

### 5.2 The five screens, and why each exists

**1 · Board** — deliberately the least important screen in the set. Mobile-first:
submit sits in the thumb zone and stays dead until four tiles are picked. Solved
groups collapse to a one-line bar instead of the current tall banner, buying back
a full tile row on a phone. The stat rail is gone.

**2 · Result — the growth engine.** One dominant action instead of five competing
buttons. **The crew board moves here**, to the moment of completion. Crews are your
only real edge over NYT Connections and they currently sit behind a spine-rail icon
where nobody finds them. Showing "Priya beat you by 34 seconds, Dev is still
playing" the instant someone finishes is what makes them return tomorrow *and*
invite the two who haven't played. "Next grid 7h 46m" is the retention hook.

**3 · Cold landing — the screen you don't have.** Every shared link today drops a
stranger on the homepage. This drops them on their friend's result: social proof,
a curiosity gap, one CTA, and "no signup" said out loud. This is the exact point
where share-rate becomes play-rate, and right now it's a dead end.

**4 · Share text** — a teardown of what `buildShareText` (`src/lib/game.ts:92`)
actually emits today versus what it could. Three findings:

- **`vibegrid.onrender.com` is in every shared message.** The most-read copy in
  your entire growth loop says "hobby project." A domain is the cheapest launch
  fix you have.
- **Nothing in it makes a non-player curious.** Scores mean something to people who
  already play; to everyone else it's noise. Naming one solved vibe — "Sunday
  scaries" — is a hook a stranger recognises, and it gives away nothing about the
  other three.
- **"Medium" leaks a setting** the reader doesn't care about.

**5 · Link preview (1200×630)** — what renders in WhatsApp, iMessage and Slack
before anyone clicks. You ship one static `og.png`; generating it per puzzle is one
handler and makes every shared link look current. Colours only, no words — nothing
spoils.

### 5.3 Implementation notes

Most of the visual change lives in the token layer — `tailwind.config.ts` and the
`@layer components` block in `globals.css`. Palette, type scale, radius ramp and
shadow removal are largely one file. The component churn is mechanical (stripping
`font-extrabold`, collapsing `.vg-desk` to a centred column). The cold-landing
screen is genuinely new work: a route plus a share-result read path.

Rough estimate: **1 week** for the full set, of which the token layer is a day.

---

## 6. Go-to-market

### 6.1 Positioning

The category is crowded and NYT Connections is the gorilla. Your differentiation
is real but it is **not** technical:

> Connections groups by **taxonomy** ("types of knots"). VibeGrid groups by
> **situation and mood.** You don't recall a fact — you recognise a feeling.

That is funnier, more relatable, and far more shareable. Lead every surface with
the puzzle content itself. Never lead with "a daily semantic grouping puzzle" —
that's engineer-speak for something that should sound like a joke you're in on.

**Positioning line to test:** *"Connections, but for feelings you didn't know were
universal."*

Secondary wedge: **crews.** "Play the daily with your group chat" is a natively
viral pitch in a way a solo puzzle is not, and Connections has no answer to it.

### 6.2 The growth model

Daily puzzle games have one dominant channel and it isn't paid. Wordle went ~90 →
300k players in three months on **one mechanic**: the spoiler-free share grid. You
have it. GTM is therefore mostly about sharpening that loop and then pointing
known-good distribution at it.

**Phase 0 — Instrument (2 weeks). Blocking.**
Install PostHog or Plausible. Minimum viable set:
- D1 / D7 / D30 retention by acquisition cohort
- Share-copy → inbound visit → first-play conversion
- Funnel drop-off: landing → mode select → first guess → completion

Watch the **mode-select step** specifically. It was added as a mobile first-run
gate, and any interstitial before the board is a prime drop-off point.

**Phase 1 — Earn the right to acquire (4–8 weeks).**
Spend nothing and post nowhere until **D7 ≥ ~20% and D30 ≥ ~10%**. Below that you
have a leaky bucket and acquisition just fills it faster. This phase is content
runway plus whatever the funnel data exposes.

**Phase 2 — Distribution, cheapest-first.**

- **SEO — the sleeper channel and highest ROI.** "connections alternative", "games
  like connections", "daily word game" are high-intent and recurring. `/archive`
  is a natural indexable corpus: one page per puzzle, each with its own long tail.
  This compounds while you sleep.
- **Aggregators and directories.** Underrated. "Games like Wordle" listicles and
  daily-puzzle hub sites rank well and send steady recurring traffic. Getting
  listed is outreach, not spend.
- **Reddit** — r/WordGames, r/dailygames, r/puzzles, r/webgames. Participate for
  weeks before posting; these communities punish drive-by launches.
- **Show HN / Product Hunt** — real spikes, but one-shot, and they mostly convert
  engineers rather than daily players. Fire only once retention is proven.
- **Short-form video** — solve-reaction content over the funnier puzzles. Highest
  ceiling, highest effort.

**Phase 3 — Lean on crews.** Make the crew invite a primary CTA rather than a nav
icon (screen 2 above already does this). Unlike the share grid, a crew creates an
*obligation* to come back tomorrow.

### 6.3 Be explicit about what winning means

Daily word games monetise badly below enormous scale. NYT bought Wordle for a low
seven figures as a subscription funnel, not for ad revenue. Realistic outcomes:

- **(a)** A small beloved thing with a few thousand daily players
- **(b)** An acquisition target for a media company wanting a puzzle vertical
- **(c)** A revenue business — requires scale this category rarely reaches solo

**(b)** requires **(a)**, and **(a)** requires the content runway. Decide which one
you're aiming at before you spend money.

---

## 7. Sequenced plan

| Order | Work | Why here | Size |
|---|---|---|---|
| 1 | **Buy a domain**, retire `onrender.com` from share text | Highest leverage per hour of any item | hours |
| 2 | **Cold-landing screen** (`/p/<id>` → friend's result + CTA) | The loop's narrowest point | ~1 day |
| 3 | **Rewrite share text** — drop mode, add a vibe-name hook, per-puzzle link | Same loop, same day's work | hours |
| 4 | **Install product analytics** | Everything after this is blind without it | ~2 days |
| 5 | **Design overhaul** — token layer, then components | Determines whether acquired traffic stays | ~1 week |
| 6 | **Per-puzzle OG image** handler | Makes every shared link look alive | ~1 day |
| 7 | **Tile length cap** in `AdminPuzzleInput.Validate` | Must land *before* bulk authoring | hours |
| 8 | **Content runway to 90–120 puzzles** | The actual launch gate | ongoing, months |
| 9 | **Promote community submissions into the daily** | Makes content sustainable | ~3 days |
| 10 | Wait for D7 ≥ 20%, then Phase 2 distribution | Don't fill a leaky bucket | — |

Items 1–3 are a single day's work and should happen this week regardless of
everything else.

---

## 8. Open decisions

These gate specific work and are yours to call:

1. **Which outcome are you aiming at** — (a), (b) or (c) from §6.3? Changes how
   much to invest in content pipeline vs. polish.
2. **Timezone / day rollover.** Still unresolved from `production-readiness.md`.
   Matters more at launch than it does now — a tab open across midnight, and
   players in IST vs US.
3. **Accounts.** Anonymous cookie identity means no cross-device sync, and Safari
   ITP caps cookie lifetime. At some retention level this becomes the ceiling.
4. **Hosting region.** You are in India; Render free is Oregon with a ~15-min
   spin-down and a 24s cold start against an 8s client timeout
   (`API_TIMEOUT_MS`, `src/lib/http.ts:1`). The keep-warm Action patches it; it is
   not a launch-grade answer. Singapore + Neon `ap-southeast-1` is the cheap move.
5. **Does the daily stay free forever?** Affects whether accounts are worth building.

---

## 9. Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Launch on a 12-puzzle loop | High if rushed | Fatal to retention | Gate launch on 90+ banked |
| Free-tier cold start on a traffic spike | Medium | Bounce at the worst moment | Paid tier before any launch post |
| Content quality regresses while scaling | High | Kills the only moat | Length cap + you stay editor-in-chief |
| Fairness gap — a tile plausibly fits two groups | Medium | Player anger, bad reviews | The embeddings checker |
| Crews stay undiscovered | High as built today | Loses the differentiator | Screen 2 above |
| Launching before analytics | — | Wasted spend, no learning | Phase 0 is blocking |

---

## 10. Bottom line

The engineering is not the problem and hasn't been for a while. What stands
between VibeGrid and a real launch is **twelve puzzles, a generated-looking skin,
and a growth loop that leaks at its narrowest point** — and the third of those is
a day's work.

Fix the loop this week. Redesign next. Then write puzzles for four months while
the SEO compounds, and don't post anywhere until D7 tells you people are coming
back.
