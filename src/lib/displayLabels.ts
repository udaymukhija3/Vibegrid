import type { Difficulty, PuzzleOrigin, PuzzleStatus } from "@/types/puzzle";

export function formatDifficulty(difficulty: Difficulty) {
  switch (difficulty) {
    case "EASY":
      return "Easy";
    case "HARD":
      return "Hard";
    case "MEDIUM":
    default:
      return "Medium";
  }
}

export function formatStatus(status: PuzzleStatus) {
  switch (status) {
    case "PUBLISHED":
      return "Published";
    case "ARCHIVED":
      return "Archived";
    case "DRAFT":
    default:
      return "Draft";
  }
}

export function formatOrigin(origin: PuzzleOrigin) {
  switch (origin) {
    case "COMMUNITY":
      return "Community";
    case "EDITORIAL":
    default:
      return "Editorial";
  }
}
