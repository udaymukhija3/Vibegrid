import { z } from "zod";
import { ApiError, apiFetch } from "@/lib/http";
import type {
  AdminPuzzle,
  DraftPuzzleInput,
  ModerationAction,
  ModerationAppeal,
  ModerationReport
} from "@/types/puzzle";

const adminPuzzleSchema = z.object({
  id: z.string(),
  puzzleNumber: z.number(),
  publishDate: z.string(),
  status: z.enum(["DRAFT", "PUBLISHED", "ARCHIVED"]),
  difficulty: z.enum(["EASY", "MEDIUM", "HARD"]),
  origin: z.enum(["EDITORIAL", "COMMUNITY"]),
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

async function adminFetch(path: string, init?: RequestInit): Promise<unknown> {
  const response = await apiFetch(path, {
    ...init,
    credentials: "include",
    headers: {
      ...(init?.headers ?? {}),
      "Content-Type": "application/json"
    }
  });

  const payload: unknown = await response.json().catch(() => null);

  if (!response.ok) {
    const parsed = errorBodySchema.safeParse(payload);
    throw new ApiError(parsed.success ? parsed.data.error : `Request failed (${response.status})`, response.status);
  }

  return payload;
}

export async function loginAdmin(password: string): Promise<void> {
  await adminFetch("/api/admin/session", {
    method: "POST",
    body: JSON.stringify({ password })
  });
}

export async function logoutAdmin(): Promise<void> {
  await adminFetch("/api/admin/session", { method: "DELETE" });
}

export async function checkAdminSession(): Promise<boolean> {
  const response = await apiFetch("/api/admin/session", { credentials: "include" });
  return response.ok;
}

export async function fetchAdminPuzzles(): Promise<AdminPuzzle[]> {
  return z.array(adminPuzzleSchema).parse(await adminFetch("/api/admin/puzzles"));
}

export async function createDraftPuzzle(input: DraftPuzzleInput): Promise<AdminPuzzle> {
  return adminPuzzleSchema.parse(
    await adminFetch("/api/admin/puzzles", {
      method: "POST",
      body: JSON.stringify(input)
    })
  );
}

export async function publishPuzzle(puzzleId: string, publishDate: string): Promise<void> {
  await adminFetch(`/api/admin/puzzles/${puzzleId}/publish`, {
    method: "POST",
    body: JSON.stringify({ publishDate })
  });
}

export async function archivePuzzle(puzzleId: string): Promise<void> {
  await adminFetch(`/api/admin/puzzles/${puzzleId}/archive`, {
    method: "POST"
  });
}

export async function reinstatePuzzle(puzzleId: string): Promise<void> {
  await adminFetch(`/api/admin/puzzles/${puzzleId}/reinstate`, {
    method: "POST"
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

export async function fetchAnalytics(puzzleId: string): Promise<PuzzleAnalytics> {
  return analyticsSchema.parse(await adminFetch(`/api/admin/puzzles/${puzzleId}/analytics`));
}

const moderationStatusSchema = z.enum(["OPEN", "ACTIONED", "DISMISSED", "RESOLVED"]);
const moderationReportSchema = z.object({
  id: z.string(),
  puzzleId: z.string(),
  puzzleNumber: z.number(),
  puzzleStatus: z.enum(["DRAFT", "PUBLISHED", "ARCHIVED"]),
  puzzleOrigin: z.enum(["EDITORIAL", "COMMUNITY"]),
  reason: z.string(),
  details: z.string(),
  contact: z.string(),
  status: moderationStatusSchema,
  createdAt: z.string(),
  resolvedAt: z.string().optional(),
  resolutionNote: z.string()
}) satisfies z.ZodType<ModerationReport>;

const moderationAppealSchema = z.object({
  id: z.string(),
  puzzleId: z.string(),
  puzzleNumber: z.number(),
  puzzleStatus: z.enum(["DRAFT", "PUBLISHED", "ARCHIVED"]),
  puzzleOrigin: z.enum(["EDITORIAL", "COMMUNITY"]),
  contact: z.string(),
  message: z.string(),
  status: moderationStatusSchema,
  createdAt: z.string(),
  resolvedAt: z.string().optional(),
  resolutionNote: z.string()
}) satisfies z.ZodType<ModerationAppeal>;

const moderationActionSchema = z.object({
  id: z.string(),
  reportId: z.string().optional(),
  appealId: z.string().optional(),
  puzzleId: z.string().optional(),
  puzzleNumber: z.number().optional(),
  actor: z.string(),
  action: z.string(),
  reason: z.string(),
  note: z.string(),
  createdAt: z.string()
}) satisfies z.ZodType<ModerationAction>;

export async function fetchReports(): Promise<ModerationReport[]> {
  const payload = z.object({ reports: z.array(moderationReportSchema) }).parse(await adminFetch("/api/admin/moderation/reports"));
  return payload.reports;
}

export async function resolveReport(id: string, action: "ARCHIVE" | "DISMISS", note: string): Promise<void> {
  await adminFetch(`/api/admin/moderation/reports/${id}/resolve`, {
    method: "POST",
    body: JSON.stringify({ action, note })
  });
}

export async function fetchAppeals(): Promise<ModerationAppeal[]> {
  const payload = z.object({ appeals: z.array(moderationAppealSchema) }).parse(await adminFetch("/api/admin/moderation/appeals"));
  return payload.appeals;
}

export async function resolveAppeal(id: string, action: "REINSTATE" | "CLOSE", note: string): Promise<void> {
  await adminFetch(`/api/admin/moderation/appeals/${id}/resolve`, {
    method: "POST",
    body: JSON.stringify({ action, note })
  });
}

export async function fetchAuditLog(): Promise<ModerationAction[]> {
  const payload = z.object({ actions: z.array(moderationActionSchema) }).parse(await adminFetch("/api/admin/moderation/audit"));
  return payload.actions;
}
