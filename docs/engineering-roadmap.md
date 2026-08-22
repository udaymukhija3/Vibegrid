# VibeGrid engineering roadmap

The active roadmap is evidence-driven. The former hidden-puzzle roadmap was
retired on 21 August 2026.

## Now: close proof gaps

1. Real-Postgres concurrent submission/vote race tests in CI.
2. Multi-browser Playwright make → judge → reveal flow with a test-only clock.
3. Component/accessibility tests for composer, blind ballot, results, join, and
   owner controls.
4. Admin board HTTP integration, mobile preview, and duplicate-date test.
5. Production-shaped load profile for crew polling and UTC rollover.
6. External deployment, restore, alert, smoke, and rollback evidence.
7. Privacy-reviewed funnel events and a small controlled cohort.

## Next only if cohort evidence supports it

- private result history;
- smallest effective judge/reveal reminder;
- browser-session recovery or optional account;
- realtime delivery if polling is observed as harmful;
- native shell if web retention exists and notification access is limiting.

## Explicitly not roadmap items

- rebuilding category puzzles;
- public card feed or global leaderboard;
- live rooms, matchmaking, or chat;
- AI automatic publishing;
- theme selector;
- speculative microservices, broker, or Redis layer.

See `launch-sprint-plan.md` for ordered acceptance criteria and
`decision-register.md` for the evidence gate on every deferred bet.
