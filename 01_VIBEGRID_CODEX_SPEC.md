# VibeGrid — Codex-Ready Product & Build Spec

## One-line pitch

**VibeGrid is a daily puzzle where players group 16 tiles into 4 hidden vibe-based categories.**

Tagline:

> Group the words. Guess the vibe. Try not to overthink it.

## Product category

Daily puzzle / semantic grouping game / internet toy.

## Core user promise

A 2–5 minute daily puzzle that feels smart, funny, cultural, and shareable.

## Why this is worth building

VibeGrid is the fastest product in the shortlist to ship as a polished public app. It has:
- simple UI
- clear rules
- game state
- session handling
- streaks
- shareable results
- puzzle authoring/admin tooling
- concurrency-safe attempt handling
- optional AI-assisted puzzle generation later

It is not technically huge, but it is product-shaped.

## MVP rules

- One daily puzzle.
- Puzzle has 16 tiles.
- Hidden solution has 4 groups of 4.
- User selects exactly 4 tiles and submits.
- If correct, group locks and reveals group name.
- If incorrect, mistake count increments.
- User gets 4 mistakes.
- Puzzle is completed when all groups are solved.
- Result screen shows mistakes, time, solved groups, and share text.

## Example puzzle

Tiles:

```text
espresso, linen, Vespa, balcony
Slack, deck, panic, 9:59
rain, jazz, window, lamp
oats, whey, hoodie, deadlift
```

Groups:

```text
Italian summer: espresso, linen, Vespa, balcony
Corporate hostage situation: Slack, deck, panic, 9:59
Noir evening: rain, jazz, window, lamp
Gym bro morning: oats, whey, hoodie, deadlift
```

## Product personality

VibeGrid should not sound generic.

Category examples:
- Bangalore founder starter pack
- Sad European film
- Corporate séance
- Soft launch of a personality
- Gym bro breakfast
- Things said before a bad decision
- Airport personality collapse
- Group chat currently on fire
- Fake wellness era
- Premium mediocrity
- Crisis disguised as brunch
- Men explaining coffee

## User stories

### Anonymous player
- As a user, I can open today’s puzzle without signing up.
- As a user, I can select and deselect tiles.
- As a user, I can submit a group of 4.
- As a user, I get immediate feedback.
- As a user, I can share my result.
- As a user, my attempt persists if I refresh.

### Returning player
- As a user, I can see my streak.
- As a user, I can see previous puzzles.
- As a user, I can compare my solve time/mistakes to global stats.

### Admin/editor
- As an admin, I can create draft puzzles.
- As an admin, I can define 4 groups of 4 tiles.
- As an admin, I can preview a puzzle.
- As an admin, I can publish a puzzle for a date.

## MVP feature scope

### Must-have
- Daily puzzle route
- 16-tile grid
- Select exactly 4
- Guess validation
- Mistake counter
- Locked solved groups
- Completed/failed state
- Shareable result text
- Anonymous session cookie
- Persisted attempt
- Seeded puzzles

### Should-have
- Admin puzzle creation
- Basic stats
- Streak
- Archive page

### Later
- User accounts
- Friend leaderboard
- Global leaderboard
- Hints
- AI puzzle assistant
- Image/audio puzzle variants
- Multiplayer race mode

## Non-goals for v1

Do not build:
- Multiplayer
- AI generation
- Native mobile
- Complex login
- Payment/subscription
- Social feed
- Comments

## Suggested stack

- Next.js App Router
- TypeScript
- Tailwind
- Postgres
- Prisma
- Auth optional; anonymous sessions first
- Vercel deployment
- Neon/Supabase Postgres

## Data model

### Prisma sketch

```prisma
model Puzzle {
  id           String        @id @default(cuid())
  puzzleNumber Int          @unique
  publishDate DateTime      @unique
  status       PuzzleStatus @default(DRAFT)
  difficulty   Difficulty   @default(MEDIUM)
  createdAt    DateTime     @default(now())
  updatedAt    DateTime     @updatedAt

  groups       PuzzleGroup[]
  tiles        PuzzleTile[]
  attempts     Attempt[]
}

model PuzzleGroup {
  id          String   @id @default(cuid())
  puzzleId    String
  name        String
  explanation String
  colorIndex  Int

  puzzle      Puzzle   @relation(fields: [puzzleId], references: [id])
  tiles       PuzzleTile[]
}

model PuzzleTile {
  id        String @id @default(cuid())
  puzzleId  String
  groupId   String
  text      String

  puzzle    Puzzle @relation(fields: [puzzleId], references: [id])
  group     PuzzleGroup @relation(fields: [groupId], references: [id])
}

model Attempt {
  id          String    @id @default(cuid())
  puzzleId    String
  userId      String?
  sessionId   String?
  mistakes    Int       @default(0)
  completed   Boolean   @default(false)
  failed      Boolean   @default(false)
  startedAt   DateTime  @default(now())
  completedAt DateTime?

  puzzle      Puzzle    @relation(fields: [puzzleId], references: [id])
  guesses     AttemptGuess[]

  @@unique([puzzleId, userId])
  @@unique([puzzleId, sessionId])
}

model AttemptGuess {
  id             String   @id @default(cuid())
  attemptId      String
  clientGuessId  String
  selectedTileIds String[]
  isCorrect      Boolean
  matchedGroupId String?
  createdAt      DateTime @default(now())

  attempt         Attempt @relation(fields: [attemptId], references: [id])

  @@unique([attemptId, clientGuessId])
}

enum PuzzleStatus {
  DRAFT
  PUBLISHED
  ARCHIVED
}

enum Difficulty {
  EASY
  MEDIUM
  HARD
}
```

## API contract

```http
GET /api/puzzles/today
```

Returns today’s puzzle with shuffled tiles but not group IDs.

```json
{
  "puzzleId": "pzl_123",
  "puzzleNumber": 12,
  "publishDate": "2026-06-02",
  "tiles": [
    { "id": "tile_1", "text": "espresso" }
  ],
  "attempt": {
    "id": "att_123",
    "mistakes": 0,
    "completed": false,
    "solvedGroups": []
  }
}
```

```http
POST /api/attempts
```

Creates or returns existing attempt for puzzle/session.

```json
{
  "puzzleId": "pzl_123"
}
```

```http
POST /api/attempts/:attemptId/guesses
```

Submits a guess.

```json
{
  "clientGuessId": "uuid-from-client",
  "selectedTileIds": ["tile_1", "tile_2", "tile_3", "tile_4"]
}
```

Response:

```json
{
  "isCorrect": true,
  "matchedGroup": {
    "id": "grp_1",
    "name": "Italian summer",
    "explanation": "These all evoke an Italian summer scene."
  },
  "mistakes": 1,
  "completed": false,
  "failed": false
}
```

## Concurrency and locking

Guess submission must be transaction-safe.

Pseudo-flow:

```text
BEGIN
  SELECT attempt FOR UPDATE
  if attempt.completed or attempt.failed: return current state
  if clientGuessId already exists: return previous result
  validate selectedTileIds
  determine if all selected tiles share same group
  insert AttemptGuess
  if incorrect: increment mistakes
  if mistakes >= 4: mark failed
  if all 4 groups solved: mark completed
COMMIT
```

Why this matters:
- Double clicks should not count as two mistakes.
- Retried HTTP requests should be idempotent.
- Refreshes should not create duplicate attempts.
- Concurrent guesses should not corrupt state.

## File scaffold

```text
vibegrid/
  app/
    page.tsx
    archive/page.tsx
    admin/puzzles/page.tsx
    api/
      puzzles/today/route.ts
      attempts/route.ts
      attempts/[attemptId]/guesses/route.ts
      admin/puzzles/route.ts
  components/
    PuzzleGrid.tsx
    PuzzleTile.tsx
    SolvedGroup.tsx
    MistakeCounter.tsx
    ShareResult.tsx
    AdminPuzzleForm.tsx
  lib/
    db.ts
    session.ts
    puzzleValidation.ts
    shareText.ts
    attemptService.ts
  prisma/
    schema.prisma
    seed.ts
  tests/
    puzzleValidation.test.ts
    attemptService.test.ts
```

## Acceptance tests

### Puzzle validation
- Selecting fewer than 4 tiles returns error.
- Selecting more than 4 tiles returns error.
- Selecting 4 from the same group is correct.
- Selecting mixed groups is incorrect.

### Attempt safety
- User/session can only create one attempt per puzzle.
- Reusing the same clientGuessId returns the original result.
- Double submission does not double-count mistakes.
- Completed attempts reject further guesses.
- Failed attempts reject further guesses.

### UI
- Solved groups lock and leave grid.
- Mistake count updates.
- Result screen appears after completion.
- Share text is copyable.

## First Codex prompt

```text
Build the VibeGrid MVP from this spec using Next.js, TypeScript, Tailwind, Prisma, and Postgres.

Implement:
1. Daily puzzle page.
2. Anonymous session cookie.
3. Seeded sample puzzles.
4. Attempt creation.
5. Transaction-safe guess submission with idempotency.
6. Puzzle grid UI.
7. Mistake counter.
8. Solved groups.
9. Share result text.

Do not implement auth, AI puzzle generation, payments, or multiplayer yet.

After implementation, provide:
- setup instructions
- .env.example
- database migration steps
- seed command
- manual test cases
```