# VibeGrid production readiness

**Review date:** 23 August 2026
**Decision:** recruiter demonstration ready; controlled beta requires the P0
evidence below; broad launch is blocked.

## Executive assessment

The reimagined product is coherent in code: public practice, durable private
crews, immutable boards, one-card/one-ballot transactions, staged privacy,
blind judging, reveal thresholds, replay safety, editor authoring, and a unified
visual system. The repository remains stronger operationally than most small
game prototypes.

The largest risk is now product evidence, not missing feature breadth. No
repository change can prove that groups will invite a third person, return to
judge, or enjoy the reveal. The correct next launch is controlled and measured.

| Posture | Status | Conditions |
| --- | --- | --- |
| Local public practice | **READY** | Direct composer, first-visit dialog, and deterministic Unlimited deals work without database. |
| Local durable crew demo | **READY WITH DB** | Migrations applied; at least three browser sessions. |
| Recruiter demo | **READY WITH DISCLOSED GAPS** | Run verification; never claim traffic/backups/alerts. |
| Controlled beta | **BLOCKED ON P0** | Provider durability, E2E, accessibility pass, privacy-reviewed funnel, seeded crews. |
| Broad consumer launch | **BLOCKED** | Controlled cohort retention plus completed ops evidence and safety path. |

## P0 exit criteria for a controlled beta

### 1. Production-shaped data path

- Provision managed Postgres.
- Apply migrations `00018_vibe_rounds.sql` and
  `00019_variable_vibe_boards.sql` through the release workflow.
- Run the full Go race suite with `TEST_DATABASE_URL` and retain the CI link.
- Run `npm run test:e2e` against the embedded single-binary build.
- Run `npm run smoke:deploy -- --mutate` against the deployed URL; verify crew
  creation, card submission, exact replay, and reloaded daily state.

### 2. Recovery truth

- Enable provider backups/PITR and record retention.
- Restore into a scratch database.
- Run migrations, query boards/cards/votes, and start `/readyz` against it.
- Record RPO, measured RTO, date, operator, source backup, and cleanup.

The checked-in workflow and runbook are scaffolding, not proof that the provider
is configured.

### 3. Security and privacy check

- Confirm secure session/admin cookies over the final HTTPS domain.
- Configure only verified provider proxy CIDRs; never broad trust ranges.
- Verify crew routes do not appear in sitemap, logs do not record invite query
  values, and nonmember daily responses contain no roster/cards/results.
- Perform a hostile HTTP pass: oversize body, invalid ids, outsider submit/vote,
  self-vote, duplicate vote, changed replay, rotated invite, removed member.
- Decide and implement a contact/report path before inviting people outside a
  directly supported cohort. The current owner controls are not a platform-wide
  reporting system.

### 4. Browser and accessibility proof

- Add a multi-context Playwright flow over real Postgres for create → three
  joins → make → judge fixture → reveal fixture.
- Add automated accessibility checks for homepage introduction, join, composer,
  ballot, result, owner controls, and admin authoring.
- Manually verify 320px layout, 200% zoom, VoiceOver or NVDA labels, and reduced
  motion. Keyboard selection/focus restoration and an iPhone 17 Pro simulator
  touch pass through onboarding, practice ballot/reveal, and durable crew join
  were completed on 23 August 2026; the simulator pass also caught and closed a
  Mobile Safari bottom-toolbar obstruction on final controls.
- Test Safari/iOS, Chrome/Android, and one desktop Chromium/Firefox path.
- Exercise 3×4, 4×4, and 7×4 palettes at phone width; confirm every surface
  remains four columns and final controls clear the browser toolbar.

### 5. Product evidence with privacy discipline

- Ratify a minimal event taxonomy without card titles, fragment text, display
  names, crew invite codes, or stable cross-crew identity.
- Measure: practice complete, crew created, third member joined, card submitted,
  eligible judge returned, ballot cast, official reveal, second official reveal.
- Recruit 5–10 known crews rather than opening a public feed.
- Review qualitative safety after the first reveals.

## Product correctness review

| Invariant | Current implementation | Evidence | Gap |
| --- | --- | --- | --- |
| 28 unique master fragments, no answer mapping | strict master validation and additive expansion storage | unit + smoke | nested editorial ambiguity checklist is manual |
| Unlimited is isolated from durable play | public deterministic 4×4 deal endpoint; cards/ballots remain component state | determinism/max-sequence and in-process HTTP/cache tests; E2E assertion checked in | updated embedded E2E rerun and product cannibalization evidence |
| Crew-sized rows stay stable | crew-lock first-open snapshot + `(crew,board)` key; selection revalidated in transaction | row-band unit matrix; real-Postgres 32-open convergence, max-crew writes, post-freeze join, and join/freeze race | multi-browser join/open trace |
| One card per member/round | transaction + unique constraint | real-Postgres distinct-client contention under the Go race detector | retained release CI artifact needed |
| Exact retry is safe | persisted client id; changed input conflicts; loser reloads winner | sequential replay, 16-way identical card/vote races, durable E2E smoke | browser timeout E2E |
| Makers-only blind vote | transaction checks own submission and target | integration tests | multi-browser E2E |
| No self-vote / one ballot | transaction + unique constraint | integration plus distinct/identical concurrent race tests | sustained load profile |
| Outsider privacy | member-aware response projection | unit test | hostile HTTP matrix |
| Author hidden then revealed | judge/result views | unit test | screen-reader/browser QA |
| Official threshold and ties | result builder/streak query | unit + integration | validate with real crews |
| Frozen dated board | insert-once store + strict admin create | unit/store code | admin HTTP integration |
| UTC stage rollover | code/deploy/defaults set UTC | deterministic clock tests | deployed midnight soak |

## Security posture

### Credible in code

- Opaque guest capability session and per-crew membership.
- Opaque revocable admin session; only token hashes stored; CSRF on cookie
  mutations; throttled login.
- Size/shape validation, blocklist, bounded identifiers, request/DB timeouts,
  rate limits, bounded metric labels, secure headers, exact public base URL, and
  trusted-proxy allowlist.
- Private crew routes excluded from sitemap; nonmember stage projection.
- Capability-bearing request paths are normalized before logging and all crew
  API responses are centrally marked `private, no-store`.
- Admin board rows immutable after creation.

### Not complete

- No public crew-card reporting/appeal workflow.
- No named operator identity or MFA; acceptable only for one operator.
- No external penetration test or sustained load test. The 23 August local npm
  audit and Go vulnerability scan found no known dependency vulnerabilities;
  this is a dated local result, not continuous monitoring.
- Guest browser loss means membership loss; there is no recovery.
- Capability invites are secrets but still transferable by design.

## Reliability and observability

Implemented:

- process liveness, database readiness, graceful shutdown, DB pool bounds;
- structured request logs and request ids;
- protected Prometheus text metrics with bounded route labels;
- store-operation latency/count metrics plus DB/cache/outbox gauges;
- additive embedded migrations and release/boot migration options;
- mutation replay ids and database uniqueness constraints;
- local/runtime smoke checks for the new board contract.

Manual or missing:

- external uptime check and routed alert;
- durable log/error retention;
- real dashboard for make/judge/reveal failure rates;
- backup freshness heartbeat and restore artifact;
- load profile for simultaneous UTC rollover and crew polling;
- documented rollback rehearsal after migration 18.

## Rollout plan

1. **Internal dogfood:** three browser profiles, several backdated fixture
   rounds, inspect disclosure and copy.
2. **Five known crews:** direct support, no public acquisition, daily review of
   quiet rounds and discomfort reports.
3. **Decision gate after two weeks:** proceed only if crews reach the third
   member and return to judge/reveal; otherwise change prompt/phase/threshold
   before adding features.
4. **Controlled beta:** expand invites, add the proven reminder channel if
   return—not making—is the bottleneck.
5. **Broad launch:** only after recovery, external observability, safety intake,
   accessibility, and cohort retention are all evidenced.

## Manual launch checklist

- [ ] Final HTTPS domain and `VIBEGRID_PUBLIC_BASE_URL` verified.
- [ ] Managed Postgres attached; `VIBEGRID_REQUIRE_DATABASE=true`.
- [ ] Secure cookies, strong admin password/session secret, metrics token.
- [ ] UTC timezone fixed and midnight rollover observed.
- [ ] Trusted proxy CIDRs verified from provider documentation.
- [ ] Backups/PITR configured and restore drill recorded.
- [ ] External health/readiness/frontend/API checks route alerts to a human.
- [ ] Log/error retention and a deploy rollback path verified.
- [ ] Branch protection requires CI; deployed SHA recorded.
- [ ] Real-Postgres race suite, browser E2E, accessibility, and dependency audits
  retained as artifacts.
- [ ] Privacy-reviewed event taxonomy and controlled-cohort consent.
- [ ] Crew-card support/report contact available.

## Do not overstate

Do not call the product “production launched” because the code can boot in
production mode. Do not call a backup workflow a backup. Do not call polling
realtime. Do not call a local practice ballot multiplayer. Do not claim product
validation from the quality of the reimplementation.

The product and engineering foundations are now credible. The next proof must
come from real crews and real operations.
