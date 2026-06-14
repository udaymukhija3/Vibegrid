import { z } from "zod";
import { ApiError, apiFetch } from "@/lib/http";
import type { DraftPuzzleInput, PublicPuzzle, PuzzleTemplate, VibeHint } from "@/types/puzzle";

// Runtime schemas for the public API surface. Validating responses at the
// boundary means a contract drift between the Go backend and the UI fails loudly
// here instead of as a confusing render-time crash deeper in the tree.
const tileSchema = z.object({
  id: z.string(),
  text: z.string()
});

const publicPuzzleSchema = z.object({
  id: z.string(),
  puzzleNumber: z.number(),
  publishDate: z.string().optional(),
  difficulty: z.enum(["EASY", "MEDIUM", "HARD"]),
  tiles: z.array(tileSchema),
  groupCount: z.number(),
  mistakesAllowed: z.number()
}) satisfies z.ZodType<PublicPuzzle>;

async function getJSON(url: string): Promise<unknown> {
  const response = await apiFetch(url, { credentials: "include" });
  if (!response.ok) {
    throw new ApiError(`Request to ${url} failed with ${response.status}`, response.status);
  }
  return response.json();
}

export async function fetchTodayPuzzle(): Promise<PublicPuzzle> {
  return publicPuzzleSchema.parse(await getJSON("/api/puzzles/today"));
}

const sessionStatusSchema = z.object({
  mode: z.literal("guest"),
  guest: z.object({
    active: z.boolean(),
    label: z.string(),
    cookieName: z.string(),
    maxAgeDays: z.number()
  }),
  admin: z.object({
    authenticated: z.boolean(),
    cookieName: z.string(),
    expiresAt: z.string().optional()
  })
});

export type SessionStatus = z.infer<typeof sessionStatusSchema>;

export async function fetchSessionStatus(): Promise<SessionStatus> {
  return sessionStatusSchema.parse(await getJSON("/api/session"));
}

// The archive grows by one puzzle a day, so it is paginated. Callers page with
// offset = number already loaded and stop when a short page (< ARCHIVE_PAGE_SIZE)
// comes back.
export const ARCHIVE_PAGE_SIZE = 30;

export async function fetchPublishedPuzzles(
  limit: number = ARCHIVE_PAGE_SIZE,
  offset = 0
): Promise<PublicPuzzle[]> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  return z.array(publicPuzzleSchema).parse(await getJSON(`/api/puzzles?${params.toString()}`));
}

export async function fetchPuzzleById(id: string): Promise<PublicPuzzle> {
  return publicPuzzleSchema.parse(await getJSON(`/api/puzzles/${encodeURIComponent(id)}`));
}

const vibeHintSchema = z.object({
  name: z.string(),
  colorIndex: z.number()
});

// fetchVibes returns the puzzle's group names + colours (no tile mapping) for
// guided Standard mode. Order is colour-stable; the UI reveals one at a time.
export async function fetchVibes(id: string): Promise<VibeHint[]> {
  const payload = z
    .object({ vibes: z.array(vibeHintSchema) })
    .parse(await getJSON(`/api/puzzles/${encodeURIComponent(id)}/vibes`));
  return payload.vibes;
}

const puzzleTemplateSchema = z.object({
  id: z.string(),
  title: z.string(),
  difficulty: z.enum(["EASY", "MEDIUM", "HARD"]),
  groups: z.array(
    z.object({
      name: z.string(),
      explanation: z.string(),
      tiles: z.array(z.string())
    })
  )
});

// fetchPuzzleTemplates returns the curated starter packs for the create page —
// either played as-is or loaded into the builder to tweak.
export async function fetchPuzzleTemplates(): Promise<PuzzleTemplate[]> {
  const payload = z
    .object({ templates: z.array(puzzleTemplateSchema) })
    .parse(await getJSON("/api/puzzle-templates"));
  return payload.templates;
}

const puzzleStatsSchema = z.object({
  players: z.number(),
  solveRate: z.number(),
  failRate: z.number(),
  medianMistakes: z.number(),
  medianSolveSeconds: z.number().optional()
});

export type PuzzleStats = z.infer<typeof puzzleStatsSchema>;

export async function fetchPuzzleStats(id: string): Promise<PuzzleStats> {
  return puzzleStatsSchema.parse(await getJSON(`/api/puzzles/${encodeURIComponent(id)}/stats`));
}

const streakSchema = z.object({
  currentStreak: z.number(),
  longestStreak: z.number(),
  totalCompleted: z.number()
});

export type StreakSummary = z.infer<typeof streakSchema>;

export async function fetchStreak(): Promise<StreakSummary> {
  return streakSchema.parse(await getJSON("/api/streak"));
}

const createdPuzzleSchema = z.object({
  ok: z.literal(true),
  id: z.string(),
  puzzleNumber: z.number()
});

const errorBodySchema = z.object({ error: z.string() });

// createCommunityPuzzle posts a user-authored puzzle and surfaces the server's
// validation message (e.g. duplicate tiles) so the create page can show why.
export async function createCommunityPuzzle(
  input: DraftPuzzleInput
): Promise<{ id: string; puzzleNumber: number }> {
  const response = await apiFetch("/api/community/puzzles", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input)
  });

  const payload: unknown = await response.json().catch(() => null);

  if (!response.ok) {
    const parsed = errorBodySchema.safeParse(payload);
    throw new ApiError(parsed.success ? parsed.data.error : `Request failed (${response.status})`, response.status);
  }

  const created = createdPuzzleSchema.parse(payload);
  return { id: created.id, puzzleNumber: created.puzzleNumber };
}

const createdModerationSchema = z.object({
  ok: z.literal(true),
  id: z.string()
});

async function postPublicMutation(url: string, input: unknown): Promise<{ id: string }> {
  const response = await apiFetch(url, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input)
  });
  const payload: unknown = await response.json().catch(() => null);

  if (!response.ok) {
    const parsed = errorBodySchema.safeParse(payload);
    throw new ApiError(parsed.success ? parsed.data.error : `Request failed (${response.status})`, response.status);
  }

  const created = createdModerationSchema.parse(payload);
  return { id: created.id };
}

export async function reportPuzzle(input: {
  puzzleId: string;
  reason: string;
  details: string;
  contact: string;
}): Promise<{ id: string }> {
  return postPublicMutation("/api/reports", input);
}

export async function appealPuzzle(input: {
  puzzleId: string;
  contact: string;
  message: string;
}): Promise<{ id: string }> {
  return postPublicMutation("/api/appeals", input);
}
