import { z } from "zod";
import type { AdminPuzzle, DraftPuzzleInput } from "@/types/puzzle";

const adminPuzzleSchema = z.object({
  id: z.string(),
  puzzleNumber: z.number(),
  publishDate: z.string(),
  status: z.enum(["DRAFT", "PUBLISHED", "ARCHIVED"]),
  difficulty: z.enum(["EASY", "MEDIUM", "HARD"]),
  groups: z.array(
    z.object({
      id: z.string(),
      name: z.string(),
      explanation: z.string(),
      colorIndex: z.number(),
      tiles: z.array(z.object({ id: z.string(), text: z.string() }))
    })
  )
}) satisfies z.ZodType<AdminPuzzle>;

const errorBodySchema = z.object({ error: z.string() });

// adminFetch attaches the bearer token and surfaces the backend's error message
// (e.g. "tile X appears more than once") so the editor can show why a save was
// rejected instead of a generic failure.
async function adminFetch(token: string, path: string, init?: RequestInit): Promise<unknown> {
  const response = await fetch(path, {
    ...init,
    headers: {
      ...(init?.headers ?? {}),
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`
    }
  });

  const payload: unknown = await response.json().catch(() => null);

  if (!response.ok) {
    const parsed = errorBodySchema.safeParse(payload);
    throw new Error(parsed.success ? parsed.data.error : `Request failed (${response.status})`);
  }

  return payload;
}

export async function fetchAdminPuzzles(token: string): Promise<AdminPuzzle[]> {
  return z.array(adminPuzzleSchema).parse(await adminFetch(token, "/api/admin/puzzles"));
}

export async function createDraftPuzzle(token: string, input: DraftPuzzleInput): Promise<AdminPuzzle> {
  return adminPuzzleSchema.parse(
    await adminFetch(token, "/api/admin/puzzles", {
      method: "POST",
      body: JSON.stringify(input)
    })
  );
}

export async function publishPuzzle(token: string, puzzleId: string, publishDate: string): Promise<void> {
  await adminFetch(token, `/api/admin/puzzles/${puzzleId}/publish`, {
    method: "POST",
    body: JSON.stringify({ publishDate })
  });
}

const analyticsSchema = z.object({
  stats: z.object({
    players: z.number(),
    solveRate: z.number(),
    failRate: z.number(),
    medianMistakes: z.number(),
    medianSolveSeconds: z.number().optional()
  }),
  wrongGuesses: z.array(z.object({ tiles: z.array(z.string()), count: z.number() }))
});

export type PuzzleAnalytics = z.infer<typeof analyticsSchema>;

export async function fetchAnalytics(token: string, puzzleId: string): Promise<PuzzleAnalytics> {
  return analyticsSchema.parse(await adminFetch(token, `/api/admin/puzzles/${puzzleId}/analytics`));
}
