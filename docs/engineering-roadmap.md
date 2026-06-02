# VibeGrid Engineering Roadmap

## P0: Playable Public MVP

- App shell with daily puzzle route.
- Public puzzle payload that hides solution metadata.
- Go guess submission API.
- Exact group matching and duplicate-tile validation.
- Anonymous session cookie.
- Attempt persistence in Postgres.
- Idempotent guess handling through `clientGuessId`.
- Completed and failed attempt states.
- Share text generation.
- Seed script and migration workflow.

## P1: Editor Desk

- Admin authentication.
- Puzzle draft form.
- Four-group editor with tile uniqueness validation.
- Preview mode using the same player UI.
- Publish scheduler with one puzzle per date.
- Draft, published, archived states.
- Puzzle QA checklist before publishing.

## P2: Retention And Stats

- Streak calculation.
- Archive route with previous puzzles.
- Completion stats: solve rate, median mistakes, median time.
- Spoiler-safe result history.
- Device/session migration path if accounts are introduced.

## P3: Quality And Scale

- Playwright coverage for core flows.
- Rate limiting on guess submission.
- Admin audit log.
- Observability for failed submissions and duplicate guesses.
- AI-assisted draft generation behind admin-only review.
- CDN/static caching for public puzzle payloads.

## Engineering Risks

- Client answer leakage if solution metadata is sent to the browser.
- Race conditions on rapid duplicate submissions.
- Streak ambiguity across timezones.
- Puzzle quality becoming dependent on tools instead of editorial review.
- Admin publishing conflicts if one date can receive multiple published puzzles.
