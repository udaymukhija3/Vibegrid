import type { VibeCard } from "@/types/vibe";

// fnv1a gives the practice round a stable seed. The same card always draws the
// same reaction, so the result cannot be rerolled by replaying the round — and
// a different card genuinely draws a different one.
function fnv1a(value: string): number {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash;
}

// practiceBallot plays out the round the way a four-person crew would: every
// house card casts one vote, for someone other than itself, and so do you.
//
// It replaces a reveal that awarded three votes to whatever the player picked
// and exactly one to the player, every time — so the only round a newcomer ever
// saw was one they lost to a bot. Here the house reads your card and sometimes
// backs it, so practice can be won, tied, or lost, which is the actual range of
// the game it is selling.
export function practiceBallot(ballot: VibeCard[], yourCard: VibeCard, yourVote: string) {
  // The fragments are a set. Another click order cannot reroll the house votes.
  const seed = fnv1a(`${yourCard.title}|${yourCard.tiles.map((tile) => tile.id).sort().join(",")}`);
  const votes = new Map<string, number>(ballot.map((card) => [card.id, 0]));
  const award = (id: string) => votes.set(id, (votes.get(id) ?? 0) + 1);

  ballot.forEach((voter, index) => {
    if (voter.isYours) {
      return;
    }
    const choices = ballot.filter((card) => card.id !== voter.id);
    award(choices[(seed >>> (index * 5)) % choices.length].id);
  });
  award(yourVote);

  const top = Math.max(...votes.values());
  const cardsAtTop = [...votes.values()].filter((count) => count === top).length;
  const cards = ballot
    .map((card) => ({
      ...card,
      authorName: card.isYours ? "You" : "House card",
      votes: votes.get(card.id) ?? 0,
      winner: cardsAtTop === 1 && (votes.get(card.id) ?? 0) === top
    }))
    .sort((left, right) => right.votes - left.votes || left.title.localeCompare(right.title));

  return { cards, tied: cardsAtTop > 1, youWon: cards.some((card) => card.isYours && card.winner) };
}
