import { z } from "zod";
import type { PublicPuzzle } from "@/types/puzzle";

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
