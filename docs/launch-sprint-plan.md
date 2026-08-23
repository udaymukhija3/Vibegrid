# VibeGrid launch and proof plan

The build phase has changed. The active product is already implemented; the
remaining work should reduce uncertainty, not expand scope.

## Phase 0 — Product reset

**Outcome:** complete in code.

- Replaced hidden-group solving with crew-sized four-column card authorship.
- Defined make D / judge D+1 / reveal D+2.
- Locked reciprocal makers-only ballot, no self-votes, ties, and official-round
  threshold.
- Demoted old `/p` loop to compatibility and removed it from the sitemap.
- Replaced public, crew, editor, policy, brand mark, OG, and documentation
  surfaces with one product story.

Exit proof: repository search and smoke contract show the root route/API no
longer exposes groups, answers, difficulty, mistakes, or a 4×4 board.

## Phase 1 — Correctness closure

**Outcome:** implemented; production-shaped evidence still required.

- Immutable board/card/vote migration plus additive master expansions and
  frozen crew/date row counts.
- Transaction-internal membership checks.
- One-card, replay, makers-only, no-self, one-ballot invariants.
- Member-aware stage projections.
- Tie and quiet-round semantics.
- Board authoring API and editor desk.

Exit proof:

```bash
TEST_DATABASE_URL="postgres://..." go test -race ./backend/...
npm run test:security
```

Retain the CI result. Add an explicit concurrent two-submit/two-vote race case if
the database CI does not already exercise constraint races.

## Phase 2 — Browser proof

**Goal:** prove the actual social journey, not only handlers.

1. Add test-only clock/fixture support that cannot enable in production.
2. Launch three isolated browser contexts against one real-Postgres server.
3. Create crew, join twice, make three cards.
4. Advance fixture date; assert each eligible member sees cards but no authors.
5. Vote, attempt self-vote/second vote, refresh after an intercepted response.
6. Advance again; assert authors, tally, tie behavior, official label, streak.
7. Rotate invite; assert old link fails and existing members retain access.
8. Remove one member; assert their access ends and old result name stays.

Add axe checks and keyboard-only interactions to the same critical states.

Exit proof: video or CI trace plus Playwright report for Chromium and WebKit.

## Phase 3 — Editorial rehearsal

**Goal:** prove one week of boards is actually generative.

- Author and freeze seven future boards in `/admin`.
- For each board, make at least six titled cards in an internal sheet.
- Reject a palette if one quartet dominates or fragments secretly form groups.
- Preview 320px mobile at 3/4/7 rows, desktop, and 1200×630 social card.
- Run a UTC rollover with open clients and confirm the prior board stays
  historically stable.
- Record who owns daily board QA and what happens if tomorrow is not authored.

Exit proof: seven-board editorial QA record plus rollover smoke. The curated
fallback bank remains the safety net, not the desired editorial cadence.

## Phase 4 — Operations rehearsal

**Goal:** make durability and detection real.

- Deploy a SHA through the guarded workflow.
- Enable managed backups/PITR; restore into scratch; record RPO/RTO.
- Configure external `/healthz`, `/readyz`, `/`, and `/api/vibes/today` checks.
- Route at least one deliberate test alert to a human.
- Scrape metrics and chart request failure/latency plus DB pool signals.
- Run `smoke:deploy -- --mutate`, dependency audits, and a small polling/load
  profile.
- Roll back to the previous image; confirm migration compatibility.

Exit proof: deployment, restore, alert, smoke, and rollback records tied to SHAs.

## Phase 5 — Controlled product validation

**Goal:** determine whether the loop deserves more investment.

- Recruit 5–10 existing friend groups with at least three willing people.
- Instrument only anonymous, content-free stage events.
- Observe the make → next-day judge → reveal → second-round funnel for two weeks.
- Interview crews after a reveal, not immediately after onboarding.
- Record quiet-round rate, third-member time, judge return, official reveals,
  second official reveal, and safety discomfort.

Decision gate:

- If crews do not invite a third person, fix crew proposition/onboarding.
- If they make but do not judge, test reminders or phase duration.
- If they judge but reveals feel flat, improve editorial palettes and result
  conversation prompts.
- If reveals create discomfort, narrow content and strengthen reporting before
  growth.
- If crews reach a second official reveal, invest in private history and the
  smallest proven notification channel.

Do not respond to a weak funnel with accounts, realtime, AI, a public feed, or a
native app.

## Deferred roadmap

Only pull an item when its evidence gate in the decision register fires:

- optional web-push/calendar reminders;
- private result history pagination;
- cross-device account recovery;
- SSE/WebSocket delivery;
- native mobile shell;
- creator tools for crew-specific prompts.

The product’s next milestone is not feature completeness. It is two official
reveals in one week from a crew that was not the builder.
