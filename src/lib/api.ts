import { z } from "zod";
import type { DraftPuzzleInput, PublicPuzzle } from "@/types/puzzle";

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
  publishDate: z.string(),
  difficulty: z.enum(["EASY", "MEDIUM", "HARD"]),
  tiles: z.array(tileSchema),
  groupCount: z.number(),
  mistakesAllowed: z.number()
}) satisfies z.ZodType<PublicPuzzle>;

async function getJSON(url: string): Promise<unknown> {
  const response = await fetch(url, { credentials: "include" });
  if (!response.ok) {
    throw new Error(`Request to ${url} failed with ${response.status}`);
  }
  return response.json();
}

export async function fetchTodayPuzzle(): Promise<PublicPuzzle> {
  return publicPuzzleSchema.parse(await getJSON("/api/puzzles/today"));
}

export async function fetchPublishedPuzzles(): Promise<PublicPuzzle[]> {
  return z.array(publicPuzzleSchema).parse(await getJSON("/api/puzzles"));
}

export async function fetchPuzzleById(id: string): Promise<PublicPuzzle> {
  return publicPuzzleSchema.parse(await getJSON(`/api/puzzles/${encodeURIComponent(id)}`));
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
  const response = await fetch("/api/community/puzzles", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input)
  });

  const payload: unknown = await response.json().catch(() => null);

  if (!response.ok) {
    const parsed = errorBodySchema.safeParse(payload);
    throw new Error(parsed.success ? parsed.data.error : `Request failed (${response.status})`);
  }

  const created = createdPuzzleSchema.parse(payload);
  return { id: created.id, puzzleNumber: created.puzzleNumber };
}
