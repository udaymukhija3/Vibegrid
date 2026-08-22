# VibeGrid resume and interview points

Use wording that proves what the repository contains without inventing users,
traffic, or production operations.

## Strong one-line description

Built an asynchronous private-crew game in Go, Postgres, and Next.js where
players author four-fragment “vibe cards,” judge them blind the next day, and
reveal authors and votes afterward.

## Resume bullets

- Reframed a derivative hidden-group puzzle into an original three-stage social
  authorship loop, then implemented the pivot across product rules, responsive
  UI, API contracts, persistence, editorial tooling, policies, sharing, tests,
  and deployment documentation.
- Designed transaction-authorized Go/Postgres card and ballot mutations with
  database uniqueness constraints, no-self-vote and makers-only eligibility,
  immutable dated boards, author snapshots, and exact replay semantics for lost
  responses and duplicate submits.
- Built member-aware make/judge/reveal projections that withhold crew content
  from outsiders, show only a player’s own card during make, remove author names
  from blind ballots, and preserve official-result thresholds and ties.
- Delivered a single-container architecture in which a Go binary embeds the
  exported Next.js frontend and SQL migrations, with health/readiness probes,
  protected bounded-label metrics, structured logs, rate limits, secure admin
  sessions, CI, Docker, and Fly/Render runbooks.
- Created an authenticated immutable board pipeline for one prompt and twelve
  fragments per UTC date, plus a deterministic curated fallback and editorial
  QA doctrine.

## Interview story

1. **Recognition:** the implementation was polished, but its primary action was
   NYT Connections with social comparison attached.
2. **Decision:** stop visual iteration and change who creates meaning—the player,
   not the editor.
3. **Mechanic:** twelve fragments → choose four → title → delayed blind vote →
   author reveal.
4. **Engineering consequence:** the system now had temporal phases, reciprocal
   eligibility, privacy staging, retry semantics, ties, quiet rounds, and
   immutable historical palettes.
5. **Scope discipline:** public feed, accounts, live rooms, AI generation, and
   native apps were rejected until real crews prove the loop.
6. **Honesty:** provider backup/restore, external alerts, accessibility E2E, and
   user retention remain evidence gaps.

## Safe claims

- “Implemented” or “built” for code and tests in this repo.
- “Designed for” replay safety, concurrency, privacy staging, and a
  single-container deploy.
- “Deployment scaffolding” for Fly/Render/Docker until a public SHA and provider
  evidence are recorded.
- “Asynchronous multiplayer” because multiple members author and vote in one
  durable crew state across days.

## Avoid

- “Production-scale,” “production launched,” or uptime/traffic numbers.
- “Realtime multiplayer.” The UI polls.
- “Anonymous” without qualification. Judge authorship is temporarily hidden;
  results reveal to crew members.
- “End-to-end tested” unless the real-Postgres multi-browser flow has run.
- “Backed up” until a provider restore drill is documented.
- “AI-powered.” It is intentionally human-authored.
