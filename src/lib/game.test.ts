import { describe, expect, it } from "vitest";
import {
  ATTEMPT_STORAGE_PREFIX,
  cleanupStoredAttempts,
  buildShareGrid,
  buildShareText,
  formatElapsedTime
} from "@/lib/game";

class FakeStorage implements Pick<Storage, "length" | "key" | "getItem" | "removeItem"> {
  private values = new Map<string, string>();

  get length() {
    return this.values.size;
  }

  key(index: number) {
    return Array.from(this.values.keys())[index] ?? null;
  }

  getItem(key: string) {
    return this.values.get(key) ?? null;
  }

  removeItem(key: string) {
    this.values.delete(key);
  }

  setItem(key: string, value: string) {
    this.values.set(key, value);
  }
}

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

  it("cleans up stale and corrupt stored attempts", () => {
    const storage = new FakeStorage();
    const now = Date.parse("2026-06-04T12:00:00.000Z");
    const freshKey = `${ATTEMPT_STORAGE_PREFIX}fresh`;
    const staleKey = `${ATTEMPT_STORAGE_PREFIX}stale`;
    const corruptKey = `${ATTEMPT_STORAGE_PREFIX}corrupt`;
    const unrelatedKey = "vibegrid:adminToken";

    storage.setItem(freshKey, JSON.stringify({ startedAt: "2026-05-20T12:00:00.000Z" }));
    storage.setItem(staleKey, JSON.stringify({ startedAt: "2026-04-01T12:00:00.000Z" }));
    storage.setItem(corruptKey, "{");
    storage.setItem(unrelatedKey, "keep-me");

    cleanupStoredAttempts(storage, now);

    expect(storage.getItem(freshKey)).not.toBeNull();
    expect(storage.getItem(staleKey)).toBeNull();
    expect(storage.getItem(corruptKey)).toBeNull();
    expect(storage.getItem(unrelatedKey)).toBe("keep-me");
  });
});
