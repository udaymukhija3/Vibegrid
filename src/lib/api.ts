import { z } from "zod";
import { ApiError, apiFetch, idempotencyHeaders } from "@/lib/http";
import type { DraftPuzzleInput, EasyHintResponse, PublicPuzzle, PuzzleTemplate, VibeHint } from "@/types/puzzle";

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
// guided Easy/Medium play. Order is colour-stable; the UI reveals one at a time.
export async function fetchVibes(id: string): Promise<VibeHint[]> {
  const payload = z
    .object({ vibes: z.array(vibeHintSchema) })
    .parse(await getJSON(`/api/puzzles/${encodeURIComponent(id)}/vibes`));
  return payload.vibes;
}

const easyHintSchema = z.object({
  name: z.string(),
  colorIndex: z.number(),
  text: z.string()
});

const easyHintResponseSchema = z.object({
  available: z.boolean(),
  guessCount: z.number(),
  requiredGuessCount: z.number(),
  hint: easyHintSchema.optional()
}) satisfies z.ZodType<EasyHintResponse>;

export async function fetchEasyHint(id: string): Promise<EasyHintResponse> {
  return easyHintResponseSchema.parse(await getJSON(`/api/puzzles/${encodeURIComponent(id)}/easy-hint`));
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

const publicConfigSchema = z.object({ turnstileSiteKey: z.string() });

export async function fetchPublicConfig(): Promise<{ turnstileSiteKey: string }> {
  return publicConfigSchema.parse(await getJSON("/api/public-config"));
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
  puzzleNumber: z.number(),
  status: z.literal("PENDING"),
  claimSecret: z.string(),
  claimPath: z.string(),
  playPath: z.string()
});

export type CreatedPuzzle = z.infer<typeof createdPuzzleSchema>;

const errorBodySchema = z.object({ error: z.string() });

// createCommunityPuzzle posts a user-authored puzzle and surfaces the server's
// validation message (e.g. duplicate tiles) so the create page can show why.
export async function createCommunityPuzzle(
  input: DraftPuzzleInput,
  turnstileToken: string
): Promise<CreatedPuzzle> {
  const response = await apiFetch("/api/community/puzzles", {
    method: "POST",
    headers: idempotencyHeaders({
      "Content-Type": "application/json",
      "X-VibeGrid-Turnstile": turnstileToken
    }),
    body: JSON.stringify(input)
  });

  const payload: unknown = await response.json().catch(() => null);

  if (!response.ok) {
    const parsed = errorBodySchema.safeParse(payload);
    throw new ApiError(parsed.success ? parsed.data.error : `Request failed (${response.status})`, response.status);
  }

  const created = createdPuzzleSchema.parse(payload);
  return created;
}

const creatorPuzzleStatusSchema = z.object({
  id: z.string(),
  puzzleNumber: z.number(),
  status: z.enum(["DRAFT", "PENDING", "PUBLISHED", "ARCHIVED"]),
  updatedAt: z.string(),
  withdrawn: z.boolean(),
  canWithdraw: z.boolean(),
  canAppeal: z.boolean(),
  playPath: z.string().optional()
});

export type CreatorPuzzleStatus = z.infer<typeof creatorPuzzleStatusSchema>;

export async function fetchCreatorPuzzleStatus(id: string, claimSecret: string): Promise<CreatorPuzzleStatus> {
  const response = await apiFetch(`/api/community/puzzles/${encodeURIComponent(id)}/claim`, {
    headers: { "X-VibeGrid-Creator-Claim": claimSecret }
  });
  const payload: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    const parsed = errorBodySchema.safeParse(payload);
    throw new ApiError(parsed.success ? parsed.data.error : `Request failed (${response.status})`, response.status);
  }
  return creatorPuzzleStatusSchema.parse(payload);
}

export async function withdrawCreatorPuzzle(id: string, claimSecret: string): Promise<CreatorPuzzleStatus> {
  const response = await apiFetch(`/api/community/puzzles/${encodeURIComponent(id)}/withdraw`, {
    method: "POST",
    headers: idempotencyHeaders({ "X-VibeGrid-Creator-Claim": claimSecret })
  });
  const payload: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    const parsed = errorBodySchema.safeParse(payload);
    throw new ApiError(parsed.success ? parsed.data.error : `Request failed (${response.status})`, response.status);
  }
  return creatorPuzzleStatusSchema.parse(payload);
}

const crewSchema = z.object({
  // The invite code is the crew's only public handle. The internal id never
  // leaves the server, so a rotated code cannot be mapped back to the crew.
  inviteCode: z.string(),
  name: z.string(),
  joinPath: z.string(),
  isOwner: z.boolean()
});

export type Crew = z.infer<typeof crewSchema>;

const crewBoardEntrySchema = z.object({
  // Present only in the owner's view — it is the handle for removing someone.
  memberId: z.string().optional(),
  displayName: z.string(),
  isYou: z.boolean(),
  playing: z.boolean(),
  solved: z.boolean(),
  failed: z.boolean(),
  solvedCount: z.number(),
  mistakes: z.number(),
  elapsedSeconds: z.number().optional(),
  // Absent until the viewer has finished today's grid: the server withholds
  // every other player's grid rather than trusting the client to hide it.
  grid: z.array(z.string()).optional()
});

export type CrewBoardEntry = z.infer<typeof crewBoardEntrySchema>;

const crewBoardSchema = z.object({
  crew: crewSchema,
  puzzleId: z.string(),
  puzzleNumber: z.number(),
  groupCount: z.number(),
  isMember: z.boolean(),
  spoilersUnlocked: z.boolean(),
  members: z.array(crewBoardEntrySchema)
});

export type CrewBoard = z.infer<typeof crewBoardSchema>;

// CrewsUnavailableError marks the no-database deployment mode, where crews are
// switched off entirely. Callers hide the feature instead of showing an error.
export class CrewsUnavailableError extends Error {}

async function crewMutation(url: string, body: unknown): Promise<Crew> {
  const response = await apiFetch(url, {
    method: "POST",
    credentials: "include",
    headers: idempotencyHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify(body)
  });
  const payload: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    const parsed = errorBodySchema.safeParse(payload);
    const message = parsed.success ? parsed.data.error : `Request failed (${response.status})`;
    throw response.status === 503 ? new CrewsUnavailableError(message) : new ApiError(message, response.status);
  }
  return crewSchema.parse(payload);
}

export async function createCrew(name: string, displayName: string): Promise<Crew> {
  return crewMutation("/api/crews", { name, displayName });
}

export async function joinCrew(inviteCode: string, displayName: string): Promise<Crew> {
  return crewMutation(`/api/crews/${encodeURIComponent(inviteCode)}/join`, { displayName });
}

// Issues a new invite code, killing every link already shared for this crew.
export async function rotateCrewInvite(inviteCode: string): Promise<Crew> {
  return crewMutation(`/api/crews/${encodeURIComponent(inviteCode)}/rotate`, {});
}

async function crewAction(url: string): Promise<void> {
  const response = await apiFetch(url, {
    method: "POST",
    credentials: "include",
    headers: idempotencyHeaders({ "Content-Type": "application/json" }),
    body: "{}"
  });
  if (!response.ok) {
    const payload: unknown = await response.json().catch(() => null);
    const parsed = errorBodySchema.safeParse(payload);
    throw new ApiError(
      parsed.success ? parsed.data.error : `Request failed (${response.status})`,
      response.status
    );
  }
}

export async function removeCrewMember(inviteCode: string, memberId: string): Promise<void> {
  return crewAction(
    `/api/crews/${encodeURIComponent(inviteCode)}/members/${encodeURIComponent(memberId)}/remove`
  );
}

export async function leaveCrew(inviteCode: string): Promise<void> {
  return crewAction(`/api/crews/${encodeURIComponent(inviteCode)}/leave`);
}

export async function fetchCrewBoard(crewId: string): Promise<CrewBoard> {
  const response = await apiFetch(`/api/crews/${encodeURIComponent(crewId)}`, { credentials: "include" });
  if (!response.ok) {
    const message = `Request failed (${response.status})`;
    throw response.status === 503 ? new CrewsUnavailableError(message) : new ApiError(message, response.status);
  }
  return crewBoardSchema.parse(await response.json());
}

export async function fetchMyCrews(): Promise<Crew[]> {
  const response = await apiFetch("/api/crews", { credentials: "include" });
  if (response.status === 503) {
    // No-database mode: report "no crews" rather than an error the UI must explain.
    return [];
  }
  if (!response.ok) {
    throw new ApiError(`Request failed (${response.status})`, response.status);
  }
  return z.array(crewSchema).parse(await response.json());
}

const createdModerationSchema = z.object({
  ok: z.literal(true),
  id: z.string()
});

async function postPublicMutation(url: string, input: unknown, turnstileToken: string): Promise<{ id: string }> {
  const response = await apiFetch(url, {
    method: "POST",
    credentials: "include",
    headers: idempotencyHeaders({
      "Content-Type": "application/json",
      "X-VibeGrid-Turnstile": turnstileToken
    }),
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
}, turnstileToken: string): Promise<{ id: string }> {
  return postPublicMutation("/api/reports", input, turnstileToken);
}

export async function appealPuzzle(input: {
  puzzleId: string;
  contact: string;
  message: string;
}, claimSecret: string, turnstileToken: string): Promise<{ id: string }> {
  const response = await apiFetch("/api/appeals", {
    method: "POST",
    credentials: "include",
    headers: idempotencyHeaders({
      "Content-Type": "application/json",
      "X-VibeGrid-Creator-Claim": claimSecret,
      "X-VibeGrid-Turnstile": turnstileToken
    }),
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
