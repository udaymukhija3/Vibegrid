import { describe, expect, it } from "vitest";
import { practiceBallot } from "./practiceBallot";
import type { VibeCard } from "@/types/vibe";

function permutations<T>(values: T[]): T[][] {
  return values.length === 0 ? [[]] : values.flatMap((value, index) =>
    permutations(values.filter((_, other) => other !== index)).map((rest) => [value, ...rest])
  );
}

const houses: VibeCard[] = [1, 2, 3].map((number) => ({
  id: `house-${number}`, title: `House ${number}`, isYours: false, tiles: []
}));
const card: VibeCard = {
  id: "practice-you", title: "Calendar cosplay", isYours: true,
  tiles: ["a", "b", "c", "d"].map((id) => ({ id, text: id }))
};

describe("practice ballot", () => {
  it("gives all 24 click orders of the same card identical votes and winners", () => {
    const tally = (tiles: VibeCard["tiles"]) => {
      const yours = { ...card, tiles };
      const result = practiceBallot([...houses, yours], yours, "house-1");
      return { tied: result.tied, youWon: result.youWon,
        cards: result.cards.map(({ id, votes, winner }) => ({ id, votes, winner })) };
    };
    for (const tiles of permutations(card.tiles)) {
      expect(tally(tiles)).toEqual(tally(card.tiles));
    }
  });

  it("casts four non-self ballots and crowns only a sole plurality", () => {
    const outcomes = new Set<string>();
    for (let index = 0; index < 100; index += 1) {
      const yours = { ...card, title: `Interpretation ${index}` };
      const result = practiceBallot([...houses, yours], yours, "house-1");
      expect(result.cards.reduce((sum, entry) => sum + entry.votes, 0)).toBe(4);
      expect(result.cards.every((entry) => entry.votes <= 3)).toBe(true);
      const leaders = result.cards.filter((entry) => entry.votes === result.cards[0].votes);
      expect(result.tied).toBe(leaders.length > 1);
      expect(result.cards.filter((entry) => entry.winner)).toHaveLength(result.tied ? 0 : 1);
      outcomes.add(result.youWon ? "win" : result.tied ? "tie" : "loss");
    }
    expect([...outcomes].sort()).toEqual(["loss", "tie", "win"]);
  });
});
