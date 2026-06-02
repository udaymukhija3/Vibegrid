export const MAX_MISTAKES = 4;

export function formatElapsedTime(startedAt: string, finishedAt = new Date().toISOString()) {
  const elapsedMs = Math.max(0, Date.parse(finishedAt) - Date.parse(startedAt));
  const totalSeconds = Math.floor(elapsedMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;

  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
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
}) {
  const status = input.failed
    ? `${input.solvedCount}/${input.groupCount} vibes found`
    : `solved in ${formatElapsedTime(input.startedAt, input.finishedAt)}`;

  return [
    `VibeGrid #${input.puzzleNumber}`,
    status,
    `mistakes ${input.mistakes}/${input.mistakesAllowed}`,
    "Group the words. Guess the vibe."
  ].join("\n");
}
