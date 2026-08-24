"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { ArrowRight, Infinity as InfinityIcon, LockKeyhole, RefreshCw, Users } from "lucide-react";
import { VibeCard } from "@/components/VibeCard";
import { VibeComposer } from "@/components/VibeComposer";
import { VibeHeader } from "@/components/VibeHeader";
import { VibeIntroDialog } from "@/components/VibeIntroDialog";
import { fetchTodayVibeBoard, fetchUnlimitedVibeBoard } from "@/lib/api";
import { useResource } from "@/hooks/useResource";
import type { Tile } from "@/types/puzzle";
import type { VibeCard as VibeCardType, VibePracticeBoard } from "@/types/vibe";

type PracticeStep = "make" | "judge" | "result";
type PracticeMode = "daily" | "unlimited";

export function VibeGridApp() {
  const boardState = useResource(fetchTodayVibeBoard, "Could not load today's fragments.");

  if (boardState.status !== "ready") {
    return (
      <div className="vg-shell">
        <VibeHeader />
        <section className="vg-dark-panel mt-8 text-center">
          <p className="vg-meta">VibeGrid</p>
          <h1 className="mt-3 text-3xl font-black text-cream">
            {boardState.status === "loading" ? "Setting out the fragments…" : boardState.message}
          </h1>
        </section>
      </div>
    );
  }

  return <VibeGridHome board={boardState.data} />;
}

function VibeGridHome({ board }: { board: VibePracticeBoard }) {
  const practiceRef = useRef<HTMLElement | null>(null);
  const [activeBoard, setActiveBoard] = useState(board);
  const [mode, setMode] = useState<PracticeMode>("daily");
  const [unlimitedSequence, setUnlimitedSequence] = useState(0);
  const [dealing, setDealing] = useState(false);
  const [dealError, setDealError] = useState("");
  const startPractice = useCallback(() => {
    practiceRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    practiceRef.current?.querySelector<HTMLButtonElement>(".vg-fragment")?.focus({ preventScroll: true });
  }, []);

  const showDaily = useCallback(() => {
    setActiveBoard(board);
    setMode("daily");
    setDealError("");
  }, [board]);

  const dealUnlimited = useCallback(async (sequence: number) => {
    if (dealing) {
      return;
    }
    setDealing(true);
    setDealError("");
    try {
      const next = await fetchUnlimitedVibeBoard(sequence);
      setActiveBoard(next);
      setUnlimitedSequence(sequence);
      setMode("unlimited");
      window.requestAnimationFrame(() => {
        practiceRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
      });
    } catch (error) {
      setDealError(error instanceof Error ? error.message : "Could not deal another board.");
    } finally {
      setDealing(false);
    }
  }, [dealing]);

  return (
    <div className="vg-shell">
      <VibeHeader actions={<VibeIntroDialog onStart={startPractice} />} />

      <section ref={practiceRef} id="practice" className="scroll-mt-5 py-7 sm:py-9">
        <div className="mb-5 flex flex-wrap items-end justify-between gap-4 sm:mb-6">
          <div>
            <p className="vg-meta text-lime">
              {mode === "daily"
                ? `Practice · board ${String(activeBoard.boardNumber).padStart(3, "0")} · immediate reveal`
                : `Unlimited · deal ${String(unlimitedSequence + 1).padStart(3, "0")} · immediate reveal`}
            </p>
            <h1 className="mt-2 text-3xl font-black tracking-[-0.04em] text-cream sm:text-5xl">
              {mode === "daily" ? "Make today's card." : "Keep making."}
            </h1>
            <p className="mt-2 max-w-xl text-sm font-semibold leading-6 text-cream/[.58] sm:text-base">
              {mode === "daily"
                ? "Pick four fragments and name what you made. This practice round reveals immediately; a real crew judges tomorrow."
                : "No timer, lives, score, or stopping point. Finish the loop and another curated deal is ready."}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Practice mode">
            <button
              type="button"
              aria-pressed={mode === "daily"}
              onClick={showDaily}
              className={mode === "daily" ? "vg-primary-button" : "vg-secondary-button"}
            >
              Today
            </button>
            <button
              type="button"
              aria-pressed={mode === "unlimited"}
              disabled={dealing}
              onClick={() => void dealUnlimited(mode === "unlimited" ? unlimitedSequence : 0)}
              className={mode === "unlimited" ? "vg-primary-button" : "vg-secondary-button"}
            >
              <InfinityIcon aria-hidden size={18} />
              {dealing ? "Dealing…" : "Unlimited"}
            </button>
            <Link href="/crews" className="vg-secondary-button">
              <Users aria-hidden size={17} />
              Find your crew
            </Link>
          </div>
        </div>
        {dealError && <p role="alert" className="mb-4 font-bold text-coral">{dealError}</p>}
        <PracticeRound
          key={activeBoard.id}
          board={activeBoard}
          mode={mode}
          dealing={dealing}
          onNext={() => void dealUnlimited(mode === "unlimited" ? unlimitedSequence + 1 : 0)}
        />
      </section>

      <footer className="flex flex-wrap items-center justify-between gap-4 border-t-2 border-cream/[.14] py-8 font-mono text-xs uppercase tracking-[0.1em] text-cream/[.45]">
        <span>VibeGrid · board {String(board.boardNumber).padStart(3, "0")}</span>
        <nav aria-label="Product policies" className="flex flex-wrap gap-4">
          <Link href="/policy" className="hover:text-lime">Crew rules</Link>
          <Link href="/privacy" className="hover:text-lime">Privacy</Link>
          <Link href="/terms" className="hover:text-lime">Terms</Link>
        </nav>
      </footer>
    </div>
  );
}

function PracticeRound({
  board,
  mode,
  dealing,
  onNext
}: {
  board: VibePracticeBoard;
  mode: PracticeMode;
  dealing: boolean;
  onNext: () => void;
}) {
  const [step, setStep] = useState<PracticeStep>("make");
  const [yourCard, setYourCard] = useState<VibeCardType | null>(null);
  const [vote, setVote] = useState<string>("");
  const houseCards = useMemo(() => practiceHouseCards(board), [board]);
  const ballot = yourCard ? [...houseCards, yourCard] : houseCards;

  if (step === "make") {
    return (
      <div className="vg-dark-panel">
        <VibeComposer
          board={board}
          submitLabel="Show me the ballot"
          onSubmit={({ title, selectedTileIds }) => {
            const selected = selectedTileIds
              .map((id) => board.tiles.find((tile) => tile.id === id))
              .filter((tile): tile is Tile => Boolean(tile));
            setYourCard({ id: "practice-you", title, tiles: selected, isYours: true });
            setStep("judge");
          }}
        />
      </div>
    );
  }

  if (step === "judge") {
    return (
      <div className="vg-dark-panel">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <p className="vg-meta text-violet-light">Judge the cards</p>
            <h3 className="mt-2 text-3xl font-black text-cream">Which one lands?</h3>
          </div>
          <p className="flex items-center gap-2 text-sm font-semibold text-cream/[.58]">
            <LockKeyhole aria-hidden size={16} /> Authors stay hidden until the result.
          </p>
        </div>
        <div className="mt-6 grid gap-4 md:grid-cols-2">
          {ballot.map((card) => (
            <VibeCard
              key={card.id}
              card={card}
              selectable
              disabled={card.isYours}
              selected={vote === card.id}
              onSelect={() => setVote(card.id)}
            />
          ))}
        </div>
        <div className="mt-5 flex justify-end">
          <button type="button" disabled={!vote} onClick={() => setStep("result")} className="vg-primary-button">
            Lock my vote
            <ArrowRight aria-hidden size={18} strokeWidth={3} />
          </button>
        </div>
      </div>
    );
  }

  const { cards: revealed, tied, youWon } = practiceBallot(ballot, yourCard!, vote);

  return (
    <div className="vg-dark-panel">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="vg-meta text-lime">Result</p>
          <h3 className="mt-2 text-3xl font-black text-cream sm:text-4xl">
            {youWon ? "Your card took it." : tied ? "The room split the vote." : "The room went the other way."}
          </h3>
          <p className="mt-3 max-w-2xl font-semibold leading-7 text-cream/[.62]">
            In a real crew, yesterday&apos;s cards appear here, everybody gets one non-self vote, and
            the revealed result becomes part of your crew&apos;s history.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={onNext} disabled={dealing} className="vg-secondary-button">
            <RefreshCw aria-hidden size={18} />
            {dealing ? "Dealing…" : mode === "unlimited" ? "Deal another" : "Keep going"}
          </button>
          <Link href="/crews" className="vg-primary-button">
            Make a private crew
            <Users aria-hidden size={18} />
          </Link>
        </div>
      </div>
      <div className="mt-6 grid gap-4 md:grid-cols-2">
        {revealed.map((card) => <VibeCard key={card.id} card={card} />)}
      </div>
    </div>
  );
}

function practiceHouseCards(board: VibePracticeBoard): VibeCardType[] {
  return board.houseCards.map((houseCard, index) => ({
    id: `house-${index + 1}`,
    title: houseCard.title,
    isYours: false,
    tiles: houseCard.tileIndices
      .map((tileIndex) => board.tiles[tileIndex])
      .filter((tile): tile is Tile => Boolean(tile))
  }));
}

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
function practiceBallot(ballot: VibeCardType[], yourCard: VibeCardType, yourVote: string) {
  const seed = fnv1a(`${yourCard.title}|${yourCard.tiles.map((tile) => tile.id).join(",")}`);
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
