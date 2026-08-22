"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { ArrowRight, LockKeyhole, Sparkles, Users } from "lucide-react";
import { VibeCard } from "@/components/VibeCard";
import { VibeComposer } from "@/components/VibeComposer";
import { VibeHeader } from "@/components/VibeHeader";
import { fetchTodayVibeBoard } from "@/lib/api";
import { useResource } from "@/hooks/useResource";
import type { VibeBoard, VibeCard as VibeCardType } from "@/types/vibe";

type PracticeStep = "make" | "judge" | "result";

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

function VibeGridHome({ board }: { board: VibeBoard }) {
  return (
    <div className="vg-shell">
      <VibeHeader />

      <section className="grid gap-8 py-10 lg:grid-cols-[0.82fr_1.18fr] lg:items-center lg:py-16">
        <div>
          <p className="vg-meta text-lime">Daily social creativity · no right answer</p>
          <h1 className="mt-4 max-w-3xl text-5xl font-black leading-[0.92] tracking-[-0.065em] text-cream sm:text-7xl">
            Make the vibe.
            <br />
            Let the crew decide.
          </h1>
          <p className="mt-6 max-w-xl text-lg font-semibold leading-8 text-cream/[.68]">
            Everyone gets the same twelve fragments. Pick four, name what you made, then come back
            to judge yesterday&apos;s cards with the authors hidden.
          </p>
          <div className="mt-7 flex flex-wrap gap-3">
            <a href="#practice" className="vg-primary-button">
              Try the practice round
              <ArrowRight aria-hidden size={18} strokeWidth={3} />
            </a>
            <Link href="/crews" className="vg-secondary-button">
              <Users aria-hidden size={17} />
              Find your crew
            </Link>
          </div>
          <div className="mt-8 grid max-w-lg grid-cols-3 gap-2">
            <Proof label="Make" value="4 of 12" />
            <Proof label="Judge" value="Blind" />
            <Proof label="Return" value="Daily" />
          </div>
        </div>

        <HeroCard board={board} />
      </section>

      <section id="practice" className="scroll-mt-6 border-t-2 border-cream/[.14] py-10 lg:py-14">
        <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
          <div>
            <p className="vg-meta text-violet-light">Practice · immediate reveal</p>
            <h2 className="mt-2 text-3xl font-black text-cream sm:text-5xl">Build one before you invite anyone.</h2>
          </div>
          <p className="max-w-sm text-sm font-semibold leading-6 text-cream/[.58]">
            The real crew loop unfolds across days. This house round shows the whole rhythm now.
          </p>
        </div>
        <PracticeRound board={board} />
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

function HeroCard({ board }: { board: VibeBoard }) {
  return (
    <div className="vg-hero-card">
      <div className="flex items-center justify-between gap-4">
        <p className="vg-meta text-ink/[.55]">Board {String(board.boardNumber).padStart(3, "0")}</p>
        <span className="rounded-full bg-ink px-3 py-1 font-mono text-[0.68rem] font-bold uppercase tracking-[0.1em] text-lime">
          Today
        </span>
      </div>
      <h2 className="mt-4 text-3xl font-black leading-[1.02] sm:text-4xl">{board.prompt}</h2>
      <div className="mt-6 grid grid-cols-3 gap-2">
        {board.tiles.slice(0, 9).map((tile, index) => (
          <span key={tile.id} className={index === 4 ? "vg-hero-fragment vg-hero-fragment-accent" : "vg-hero-fragment"}>
            {tile.text}
          </span>
        ))}
      </div>
      <div className="mt-6 flex items-center justify-between gap-3 border-t-2 border-ink/[.15] pt-4">
        <p className="text-sm font-bold">Twelve fragments. One card that sounds like you.</p>
        <Sparkles aria-hidden className="text-violet" size={22} />
      </div>
    </div>
  );
}

function PracticeRound({ board }: { board: VibeBoard }) {
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
              .filter((tile): tile is VibeBoard["tiles"][number] => Boolean(tile));
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

  const revealed = ballot
    .map((card, index) => ({
      ...card,
      authorName: card.isYours ? "You" : `House card ${index + 1}`,
      votes: card.id === vote ? 3 : card.isYours ? 1 : 0,
      winner: card.id === vote
    }))
    .sort((left, right) => (right.votes ?? 0) - (left.votes ?? 0));

  return (
    <div className="vg-dark-panel">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="vg-meta text-lime">Result</p>
          <h3 className="mt-2 text-3xl font-black text-cream sm:text-4xl">That is the whole game.</h3>
          <p className="mt-3 max-w-2xl font-semibold leading-7 text-cream/[.62]">
            In a real crew, yesterday&apos;s cards appear here, everybody gets one non-self vote, and
            the revealed result becomes part of your crew&apos;s history.
          </p>
        </div>
        <Link href="/crews" className="vg-primary-button">
          Make a private crew
          <Users aria-hidden size={18} />
        </Link>
      </div>
      <div className="mt-6 grid gap-4 md:grid-cols-2">
        {revealed.map((card) => <VibeCard key={card.id} card={card} />)}
      </div>
    </div>
  );
}

function practiceHouseCards(board: VibeBoard): VibeCardType[] {
  const recipes = [
    { id: "house-one", title: "Still technically fine", indices: [0, 5, 8, 11] },
    { id: "house-two", title: "The soft launch", indices: [2, 4, 7, 10] },
    { id: "house-three", title: "Quietly unraveling", indices: [1, 3, 6, 9] }
  ];
  return recipes.map((recipe) => ({
    id: recipe.id,
    title: recipe.title,
    isYours: false,
    tiles: recipe.indices.map((index) => board.tiles[index])
  }));
}

function Proof({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-l-2 border-cream/20 pl-3">
      <p className="font-mono text-[0.65rem] font-bold uppercase tracking-[0.12em] text-cream/[.42]">{label}</p>
      <p className="mt-1 text-sm font-black text-cream">{value}</p>
    </div>
  );
}
