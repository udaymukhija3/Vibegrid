import { describe, expect, it } from "vitest";
import { buildShareGrid, buildShareText, formatElapsedTime } from "@/lib/game";

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

  it("builds a coloured share grid from guess history", () => {
    const colorByTile = { a: 0, b: 1, c: 2, d: 3 };
    // One mixed (wrong) guess, then one clean (correct) guess.
    const grid = buildShareGrid(
      [
        ["a", "b", "c", "d"],
        ["a", "a", "a", "a"]
      ],
      colorByTile
    );
    expect(grid).toEqual(["🟩🟨🟥🟪", "🟩🟩🟩🟩"]);
  });

  it("embeds the grid in the share text", () => {
    const text = buildShareText({
      puzzleNumber: 7,
      mistakes: 1,
      mistakesAllowed: 4,
      solvedCount: 4,
      groupCount: 4,
      startedAt: "2026-06-02T10:00:00.000Z",
      finishedAt: "2026-06-02T10:01:00.000Z",
      failed: false,
      grid: ["🟩🟩🟩🟩"],
      shareUrl: "https://vibegrid.app/p/example"
    });
    expect(text).toContain("🟩🟩🟩🟩");
    expect(text).toContain("VibeGrid #7");
    expect(text).toContain("https://vibegrid.app/p/example");
  });
});
