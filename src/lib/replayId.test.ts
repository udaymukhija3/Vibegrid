import { describe, expect, it } from "vitest";
import { replayIdFor } from "@/lib/api";

// The server accepts a replay key of [A-Za-z0-9_-]{1,96} (validateVibeClientID).
const CLIENT_ID_PATTERN = /^[A-Za-z0-9_-]{1,96}$/;

describe("replayIdFor", () => {
  it("produces a key the server will accept", () => {
    const id = replayIdFor("card", "vibe-2026-08-21", "Under control", "t1", "t2", "t3", "t4");
    expect(id).toMatch(CLIENT_ID_PATTERN);
  });

  it("is stable across repeated presses of the same intent", () => {
    const args = ["vibe-2026-08-21", "Under control", "t1", "t2", "t3", "t4"] as const;
    const first = replayIdFor("card", ...args);
    const second = replayIdFor("card", ...args);
    expect(second).toBe(first);
  });

  it("changes when the title changes", () => {
    const a = replayIdFor("card", "vibe-2026-08-21", "Under control", "t1", "t2", "t3", "t4");
    const b = replayIdFor("card", "vibe-2026-08-21", "Out of control", "t1", "t2", "t3", "t4");
    expect(b).not.toBe(a);
  });

  it("changes when the selected fragments change", () => {
    const a = replayIdFor("card", "vibe-2026-08-21", "Same", "t1", "t2", "t3", "t4");
    const b = replayIdFor("card", "vibe-2026-08-21", "Same", "t1", "t2", "t3", "t5");
    expect(b).not.toBe(a);
  });

  it("changes when the fragment ORDER changes", () => {
    // The server compares selectedTileIds with slices.Equal, so a reordered
    // selection is a different request and must not collide with the first.
    const a = replayIdFor("card", "vibe-2026-08-21", "Same", "t1", "t2", "t3", "t4");
    const b = replayIdFor("card", "vibe-2026-08-21", "Same", "t4", "t3", "t2", "t1");
    expect(b).not.toBe(a);
  });

  it("changes when the board changes", () => {
    const a = replayIdFor("card", "vibe-2026-08-21", "Same", "t1", "t2", "t3", "t4");
    const b = replayIdFor("card", "vibe-2026-08-22", "Same", "t1", "t2", "t3", "t4");
    expect(b).not.toBe(a);
  });

  it("separates ballots from cards by prefix", () => {
    expect(replayIdFor("vote", "board", "card-1")).not.toBe(replayIdFor("card", "board", "card-1"));
  });

  it("distinguishes ballot targets", () => {
    const a = replayIdFor("vote", "vibe-2026-08-20", "card_aaa");
    const b = replayIdFor("vote", "vibe-2026-08-20", "card_bbb");
    expect(a).not.toBe(b);
    expect(a).toMatch(CLIENT_ID_PATTERN);
  });

  it("does not collide across a realistic draft space", () => {
    const seen = new Set<string>();
    for (let index = 0; index < 5000; index += 1) {
      seen.add(replayIdFor("card", "vibe-2026-08-21", `Draft ${index}`, "t1", "t2", "t3", "t4"));
    }
    expect(seen.size).toBe(5000);
  });
});
