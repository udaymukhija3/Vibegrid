import { describe, expect, it } from "vitest";
import { buildShareText, formatElapsedTime } from "@/lib/game";

describe("game UI helpers", () => {
  it("formats elapsed time", () => {
    expect(
      formatElapsedTime("2026-06-02T10:00:00.000Z", "2026-06-02T10:02:09.000Z")
    ).toBe("2:09");
  });

  it("builds spoiler-safe share text", () => {
    expect(
      buildShareText({
        puzzleNumber: 1,
        mistakes: 1,
        mistakesAllowed: 4,
        solvedCount: 4,
        groupCount: 4,
        startedAt: "2026-06-02T10:00:00.000Z",
        finishedAt: "2026-06-02T10:02:09.000Z",
        failed: false
      })
    ).toContain("VibeGrid #1");
  });
});
