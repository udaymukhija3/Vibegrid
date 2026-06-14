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

## Item B — Daily supply without daily manual authoring

### Why
The daily game should not require an operator to write and publish a puzzle every
night. Exact-date editorial puzzles should still win when they exist, but an
empty date must not replay yesterday or go dark.

### Shipped contract
- `handleTodayPuzzle` computes "today" from the configured server timezone.
- `PostgresPuzzleStore.TodaysPuzzle` can still return the latest published
  editorial puzzle through today, but `bankPuzzleSource` only accepts that result
  when its `publish_date` is exactly today.
- If no exact-date editorial puzzle exists, `bankPuzzleSource` composes a
  deterministic `vibegrid-YYYY-MM-DD` board from the curated bank group pool.
- Generated daily answers stay server-side and use the normal attempt/guess path.
- The generated daily can be resolved later by id, so share links and result
  URLs keep working.

### Tests
- Same date produces the same generated daily.
- A one-year window of generated dates has no repeated group set.
- Every generated board has four groups, four tiles per group, stable color
  indexes, and no duplicate tile text.
- A scheduled exact-date puzzle overrides the generator.
- `/api/puzzles/today` does not 404 when only the bank source is available.

### Remaining work
- Add admin visibility for the source of today's puzzle (editorial vs generated)
  and the size/health of the bank group pool.
- Add draft preview and a scheduling calendar so generated dailies are a safety
  net, not a substitute for editorial quality.
- If the product later wants deliberate off-days, put a strict empty-state
  contract behind a feature flag and update streak semantics at the same time.

---

## Suggested sequencing
Do **Item A** first (smaller, isolated, high growth leverage, no product decision
needed). Do **Item B** after the cadence + streak-fairness decisions are made,
since the API contract and streak logic both depend on them.
