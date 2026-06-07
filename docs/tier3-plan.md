# Tier 3 implementation plan

Two work items, written to be picked up cold in a new chat. Both are grounded in
the current code (post-PR #1 `harden/tier1-integrity-abuse`). Verify with the
same gates used so far: `gofmt`, `go vet`, `go test -race ./...` against Postgres
16, and frontend `typecheck` / `lint` / `test` / `build`.

---

## Item A — Raster OG images + de-duplicate OG tags

### Why
Link previews are broken on every platform that matters. Today both OG images are
SVG, and SVG OG images are ignored by Twitter/X, Facebook, LinkedIn, iMessage,
Slack, WhatsApp, and Discord — they require PNG/JPEG/WebP, ideally with explicit
`og:image:width`/`height`. On top of that, OG tags are emitted twice (Next static
metadata **and** the Go injector), so served pages carry conflicting tags.

### Current state (read these first)
- `renderPuzzleOGImage(puzzle)` in `backend/internal/vibegrid/server.go` builds an
  **SVG string**; `handlePuzzleOGImage` serves it at `/api/og/puzzles/{id}.svg`
  with `Content-Type: image/svg+xml`.
- `withFrontendMetadata` + `frontendMetadataFor` + `injectFrontendMetadata` (same
  file) inject `og:*` / `twitter:*` into `<head>` for document routes, pointing
  `og:image` at the `.svg` URL (per-puzzle for `/p/{id}`, else `/vibegrid-mark.svg`).
- `src/app/layout.tsx` **also** emits `openGraph` + image via Next metadata
  (`images: ["/vibegrid-mark.svg"]`) → duplicate tags.
- `public/` contains only `vibegrid-mark.svg` (no raster default).
- Tests to update: `TestSharedPuzzleHTMLInjectsOGMetadata` and
  `TestPuzzleOGImageEndpoint` in `game_test.go` both assert `.svg`.

### Design decision: how to rasterize in Go
Recommended: **draw the card directly with `github.com/fogleman/gg`** and encode
PNG via stdlib `image/png`. Pure Go, no cgo, works in the distroless runtime.
- For fonts, use `golang.org/x/image/font/gofont/goregular` + `gobold` (font bytes
  compiled into the binary via `freetype/truetype`) — **no embedded asset files**,
  license-clean.
- Reuse the layout math already in `renderPuzzleOGImage` (1200×630, tile grid,
  `colors`, `truncateOGText`); just draw rects + text with gg instead of emitting
  SVG markup.

Rejected alternatives:
- `oksvg`/`rasterx` (rasterize the existing SVG): lightest, but its `<text>` and
  `<pattern>` support is weak — the puzzle labels would likely drop. Avoid.
- `resvg-go` (wazero/wasm): high fidelity, pure Go, but a heavy dependency for a
  card we fully control. Overkill.
- Headless Chromium: not available in distroless; no.

### Backend changes
1. Add deps: `github.com/fogleman/gg`, `golang.org/x/image` (gofont, freetype/truetype).
2. New `renderPuzzleOGPNG(puzzle Puzzle) ([]byte, error)` — gg port of the SVG
   layout returning PNG bytes. Keep tile truncation + color logic.
3. Route: serve `/api/og/puzzles/{id}.png` → `image/png`, keep the long
   `Cache-Control` already on the SVG handler. Decide whether to keep `.svg`
   (fine to keep for debugging, but it must NOT be the OG target). Strip the
   `.png`/`.svg` suffix in the handler like it does today.
4. Optional but recommended: in-process LRU of rendered PNG bytes keyed by puzzle
   id (content is immutable) — mirror `cachedPuzzleStore` / `cachedStatsStore`
   patterns so a viral puzzle isn't re-rasterized on every crawl. Invalidate is
   unnecessary (content never changes; only status, which doesn't affect the card).
5. Static default raster: add `public/og-default.png` (1200×630 brand card) and
   point non-puzzle routes' `og:image` at it instead of `vibegrid-mark.svg`.
   (Generate it once; can reuse `renderPuzzleOGPNG`-style drawing or a static asset.)

### OG-tag de-duplication
Make the **Go injector the single source of truth** for social tags on document
routes (it already produces correct per-puzzle tags):
1. In `src/app/layout.tsx`, remove the `openGraph` block (keep `title`,
   `description`, `metadataBase` for the browser tab / canonical only). This stops
   Next from emitting `og:*`.
2. In `frontendMetadataFor`, switch the dynamic image URL from `.svg` to `.png`.
3. In `injectFrontendMetadata`, add `og:image:width` (1200), `og:image:height`
   (630), keep `og:image:type` (now `image/png`), and add `og:image:secure_url`
   (absolute https — `requestBaseURL` already yields it). These dimensions
   materially improve card rendering.

### Tests
- `renderPuzzleOGPNG`: decode with `image/png`, assert bounds 1200×630, non-empty.
- Handler: `GET /api/og/puzzles/{id}.png` → 200, `Content-Type: image/png`, decodes.
- Injection: served `/p/{id}` HTML contains exactly **one** `og:image`, ending
  `.png`, with width/height, and **no** `image/svg+xml` og:image. Update
  `TestSharedPuzzleHTMLInjectsOGMetadata` and `TestPuzzleOGImageEndpoint`.
- Manual post-deploy: run the Twitter/X card validator + paste a `/p/{id}` link in
  iMessage/Slack to confirm a real preview renders.

### Risks / notes
- `gofont` covers Latin glyphs; non-Latin tile text renders as missing glyphs.
  Acceptable for v1 — note it, consider a CJK-capable font later if needed.
- PNG rasterization is more CPU than string-building; the per-id cache + edge
  cache keep origin load negligible.

---

## Item B — Explicit "no puzzle today" state

### Why
`handleTodayPuzzle` → `PostgresPuzzleStore.TodaysPuzzle` runs
`... where publish_date <= today order by publish_date desc limit 1`. On any day
with no puzzle for *today*, it silently serves the **previous** day's board — and
a returning player who already solved it sees a finished grid with no new game.
Seed content (`seed.go`) runs out within days, so this fires quickly in practice.

### Decision needed from product (ask before implementing)
1. **Cadence semantics** — pick one:
   - **(A) Strict daily (recommended for a "daily" brand):** "today" = puzzle with
     `publish_date == today`. If none, return an explicit empty state.
   - **(B) Rolling with banner:** keep serving the latest, but when
     `latest.publish_date < today` mark it stale (`stale: true`) and show a banner
     ("No new puzzle today — here's the latest").
   (A) is honest and pushes the real fix — an authoring/scheduling pipeline. (B) is
   gentler if cadence is intentionally sparse.
2. **Streak fairness** (matters for A, see below): should days with *no published
   puzzle* be skipped so they don't break streaks? Recommended **yes**.
3. **Fallback affordance:** when there's no puzzle today, offer "play the most
   recent," archive, and create — or just archive/create?

### Backend changes (for Option A)
- Add a store method for the strict case, e.g.
  `TodaysPuzzleStrict(ctx, today)` returning the puzzle where
  `publish_date == today::date` (or `ErrPuzzleNotFound`), plus reuse a
  `LatestPublished(ctx, today)` for the fallback link. Add to the `PuzzleSource`
  interface and implement in `PostgresPuzzleStore`, `StaticPuzzleSource`, and the
  `cachedPuzzleStore` delegate (and the test fake).
- Change `handleTodayPuzzle` to a **200 with a discriminated payload** rather than
  404-on-missing:
  - Found: `{ "available": true, "puzzle": <PublicPuzzle> }`.
  - Missing: `{ "available": false, "latest": { "id", "puzzleNumber",
    "publishDate" } | null }`.
  (200-with-`available:false` is cleaner for the client than overloading 404.)
- Keep `dailyCacheControl()` (it already caps `s-maxage` near rollover); the
  no-puzzle payload flips at rollover too, so the capped TTL is correct.

### Streak fairness (the subtle part)
`computeStreak` / `PostgresStatsStore.SessionStreak` count consecutive calendar
days the session completed an editorial daily. With strict-daily, a date that had
**no puzzle published** is an unplayable gap; today it would *break* a streak
unfairly (completed 06-04 and 06-06, nothing on 06-05 → gap).
Fix: compute streaks over the set of **published daily dates**, treating
no-puzzle days as non-breaking (skip, don't reset). This means `SessionStreak`
needs the published-date calendar (a `select distinct publish_date from puzzles
where origin='EDITORIAL' and status='PUBLISHED' and publish_date <= today`) joined
against the session's completions. Update `computeStreak` to walk the published
calendar rather than raw consecutive dates. Add unit tests for the gap case.

### Frontend changes
- `src/lib/api.ts` `fetchTodayPuzzle`: change the schema to the discriminated
  union (`available: true/false`). Return a typed result the caller can branch on.
- `src/components/VibeGridApp.tsx`: when `available === false`, render an explicit
  "No puzzle today" card (brand header + "Come back tomorrow", plus CTAs: Archive,
  Create, and — if `latest` present and Option allows — "Play the latest"). Today
  this component only has loading / ready / error states; add the empty state.
- Keep the existing error state for real failures (distinct from "no puzzle").

### Tests
- Backend: `TodaysPuzzleStrict` returns only an exact-date match; `handleTodayPuzzle`
  returns `available:false` (+ latest) when today is empty, `available:true` when
  present. Streak: a no-puzzle gap day does not break the streak.
- Frontend: `fetchTodayPuzzle` parses both payload shapes; `VibeGridApp` renders the
  empty state on `available:false` (component test or rely on typecheck + manual).

### Risks / notes
- This is the product risk made concrete — the real mitigation is an authoring
  cadence + draft queue + preview (out of scope here, but the empty state is what
  makes the gap visible enough to force that work).
- Coordinate the API contract change with the frontend in the same PR so `today`
  never returns an unhandled shape.

---

## Suggested sequencing
Do **Item A** first (smaller, isolated, high growth leverage, no product decision
needed). Do **Item B** after the cadence + streak-fairness decisions are made,
since the API contract and streak logic both depend on them.
