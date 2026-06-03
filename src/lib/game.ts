export const MAX_MISTAKES = 4;

export function formatElapsedTime(startedAt: string, finishedAt = new Date().toISOString()) {
  const elapsedMs = Math.max(0, Date.parse(finishedAt) - Date.parse(startedAt));
  const totalSeconds = Math.floor(elapsedMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;

  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

export function formatSeconds(totalSeconds: number) {
  const safe = Math.max(0, Math.round(totalSeconds));
  const minutes = Math.floor(safe / 60);
  const seconds = safe % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

// Coloured squares for the spoiler-safe share grid, indexed by a group's
// colorIndex (0..3) to mirror the on-screen mint / yolk / tomato / plum palette.
export const SHARE_SQUARES = ["🟩", "🟨", "🟥", "🟪"];

// buildShareGrid turns the player's guess history into Wordle/Connections-style
// rows of coloured squares. Each guess is one row; each tile is coloured by the
// group it actually belonged to (known once the puzzle is over), so the grid
// shows the path to the solution without naming any group.
export function buildShareGrid(guessHistory: string[][], colorByTile: Record<string, number>): string[] {
  return guessHistory
    .filter((row) => row.length > 0)
    .map((row) => row.map((tileId) => SHARE_SQUARES[(colorByTile[tileId] ?? 0) % SHARE_SQUARES.length]).join(""));
}

export function buildShareText(input: {
  puzzleNumber: number;
  mistakes: number;
  mistakesAllowed: number;
  solvedCount: number;
  groupCount: number;
  startedAt: string;
  finishedAt?: string;
  failed: boolean;
  grid?: string[];
}) {
  const status = input.failed
    ? `${input.solvedCount}/${input.groupCount} vibes found`
    : `Solved in ${formatElapsedTime(input.startedAt, input.finishedAt)}`;

  const lines = [`VibeGrid #${input.puzzleNumber}`];
  if (input.grid && input.grid.length > 0) {
    lines.push(...input.grid);
  }
  lines.push(`${status} · ${input.mistakes}/${input.mistakesAllowed} mistakes`);
  lines.push("Group the words. Guess the vibe.");
  return lines.join("\n");
}
