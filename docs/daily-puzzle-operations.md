# Daily board operations

The filename is retained for old links. The active object is a **daily board**,
not a hidden-answer puzzle.

## Board contract

- one UTC publish date;
- one prompt, 1–140 runes;
- exactly twelve distinct fragments, each 1–28 runes;
- no groups, intended quartet, difficulty, answer, timer, or mistake budget;
- immutable after the first row for that date is stored.

## Normal workflow

1. Sign in at `/admin`.
2. Choose today or a future UTC date.
3. Write the prompt and twelve fragments.
4. Complete the editorial QA in `product-vision.md`.
5. Freeze the board. A duplicate date returns conflict by design.
6. Verify `GET /api/vibes/today` when that UTC date becomes active.

The board room lists the latest 90 stored boards. A missing scheduled date is
filled by the deterministic curated bank in `vibe_boards.go`; the first request
persists that snapshot when Postgres is available.

## Before freezing

- Make six meaningfully different titled cards.
- Check that no obvious intended quartet dominates.
- Check duplicate/case-folded fragment text.
- Read prompt and fragments aloud.
- Check the smallest mobile width.
- Review private-person, harassment, protected-class, sexual, and IP risk.
- Confirm the date in UTC, especially near local midnight.

## Mistake or unsafe board

### Before it has been persisted

Edit the draft in your working notes and freeze the corrected version.

### After the date is frozen but before anyone plays

The application has no edit endpoint. That is intentional. If an exceptional
operator correction is required, use an audited migration or carefully reviewed
SQL only after confirming there are zero submissions for the board. Record the
change and rerun smoke. Do not add a casual “edit” button.

### After any card exists

Do not mutate the board. A card’s selected ids and every member’s interpretation
refer to that exact palette. If content is unsafe, take the service/crew path out
of circulation while deciding a forward fix; never silently rewrite history.

## Rollover checks

At UTC midnight:

- `/api/vibes/today` returns the new id/date/number/prompt and twelve fragments;
- a crew daily response moves prior today → judge and prior judge → result;
- an already-open composer is not forcibly swapped mid-action; refresh/fetch
  reconciles it;
- stored prior boards remain byte-for-byte stable;
- cache headers do not serve yesterday past the bounded rollover window.

## Editorial fallback incident

If an unexpected curated fallback ships:

1. verify the active UTC date and configured timezone;
2. query `vibe_daily_boards` for that date;
3. determine whether the editor attempted a duplicate/frozen date;
4. do not overwrite if any `vibe_submissions` rows exist;
5. record the miss and improve the publishing checklist.

Fallback availability is a reliability feature. It is not a substitute for a
human editorial queue.
