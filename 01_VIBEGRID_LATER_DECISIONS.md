# VibeGrid — Later Product Decisions

## Product status

VibeGrid remains the **fastest shippable consumer product** in the shortlist.

It is a game, so it can and should be gamified. But the gamification should be native to the puzzle loop, not bolted-on nonsense.

## Final product positioning

**VibeGrid is a daily semantic puzzle where players group 16 tiles into 4 hidden vibe-based categories.**

Tagline options:
- Group the words. Guess the vibe.
- Four groups. Sixteen clues. One very specific kind of overthinking.
- Find the hidden vibe.

## Surface simplicity

The user sees:
- 16 tiles
- select 4
- submit
- solve all groups
- share result

That is it.

## Hidden engineering depth

VibeGrid should be treated as a real game engine, not just frontend state.

### Must-have serious systems

1. Server-authoritative validation
2. Transaction-safe guess submission
3. Idempotent guesses using `clientGuessId`
4. Anonymous session persistence
5. One attempt per puzzle per user/session
6. Admin puzzle builder
7. Puzzle publishing calendar
8. Share result generation
9. Event telemetry
10. Puzzle quality dashboard

## Important later decision

The serious version of VibeGrid is not “AI-generated puzzles.”

The serious version is:

> a daily puzzle engine with fairness, quality control, telemetry, and adaptive difficulty.

AI can assist admin creation later, but should never auto-publish puzzles.

## Deep system features to add after MVP

### 1. Alternate-solution checker

Problem:
A puzzle may accidentally contain another valid group.

Example:
```text
espresso, latte, matcha, oat milk
```

Maybe the intended category is “café drinks,” but those words may also be part of another vibe.

Build an admin tool that flags:
- suspicious semantic clusters
- likely accidental categories
- tiles that appear in multiple plausible groups
- weak group explanations

V1 can be manual:
- admin enters known red herrings

V2 can be assisted:
- embeddings/LLM propose possible alternate groups
- admin approves/rejects warnings

### 2. Wrong-guess heatmap

For each puzzle, show:
- most common wrong groupings
- average mistake count
- where users abandon
- which tile combinations confuse users

This makes puzzle design data-driven.

### 3. Difficulty calibration

For each puzzle:

```text
predicted difficulty: medium
actual difficulty: hard
reason: high failure rate, high solve time, common wrong grouping
```

### 4. Puzzle quality score

Internal score:
- clarity
- fairness
- red-herring strength
- humor
- cultural specificity
- difficulty
- shareability

This is admin-facing.

### 5. Adaptive difficulty later

Only after enough data.

Start rule-based:
```text
if user failed last two hard puzzles:
  serve medium archive puzzle
elif user solves too easily:
  serve harder puzzle
```

Later contextual bandit:
- context: skill profile, recent mistakes, solve time
- action: easy/medium/hard/theme
- reward: completion + share + return - frustration

## What not to do

Do not:
- add power-ups
- add coins
- add avatars
- add cartoon mascots
- add fake rewards
- make it a children’s puzzle app
- let AI auto-publish unreviewed puzzles

## Game design decisions

### Correct puzzle feel

The reveal should make users say:

> “Ah, fair. Annoying, but fair.”

Not:
> “That was arbitrary.”

### Difficulty source

Good difficulty comes from:
- overlapping associations
- red herrings
- abstract but fair categories
- culturally recognizable references

Bad difficulty comes from:
- obscure trivia
- arbitrary vibes
- weak explanations
- private jokes only you understand

## Engineering talking points

Portfolio bullet:

> Built a daily semantic puzzle game with server-authoritative validation, transaction-safe attempts, idempotent guess submission, admin puzzle publishing, puzzle telemetry, and difficulty calibration tools.

## Go backend recommendation

Core modules:

```text
internal/puzzles
internal/attempts
internal/sessions
internal/validation
internal/admin
internal/events
internal/metrics
internal/difficulty
```

Transaction for guess submission:

```text
BEGIN
  SELECT attempt FOR UPDATE
  check completion/failure
  check idempotency key
  validate selected tiles
  insert guess
  update solved groups/mistakes/completion
COMMIT
```

## Product maturity path

### V1
Playable daily puzzle.

### V2
Admin puzzle builder + telemetry.

### V3
Quality dashboard + wrong-guess heatmap.

### V4
Difficulty calibration.

### V5
AI-assisted puzzle draft/evaluation.

## Final recommendation

Build VibeGrid first if the immediate goal is shipping.

It is the cleanest “simple surface, deep system” product in the shortlist.