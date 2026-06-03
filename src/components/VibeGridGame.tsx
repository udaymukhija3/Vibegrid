"use client";

import { useEffect, useMemo, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import clsx from "clsx";
import { Archive, RotateCcw, Send, Share2, Shuffle, Sparkles } from "lucide-react";
import { buildShareText, formatElapsedTime, formatSeconds } from "@/lib/game";
import { fetchPuzzleStats, type PuzzleStats } from "@/lib/api";
import { HowToPlay } from "@/components/HowToPlay";
import type { AttemptSnapshot, GuessResponse, PublicPuzzle, SolvedGroup, Tile } from "@/types/puzzle";

type StoredAttempt = {
  puzzleId: string;
  selectedTileIds: string[];
  solvedGroups: SolvedGroup[];
  revealedGroups: SolvedGroup[];
  mistakes: number;
  guessCount: number;
  startedAt: string;
  completedAt?: string;
  failed: boolean;
  completed: boolean;
};

const emptyAttempt = (puzzleId: string): StoredAttempt => ({
  puzzleId,
  selectedTileIds: [],
  solvedGroups: [],
  revealedGroups: [],
  mistakes: 0,
  guessCount: 0,
  startedAt: new Date().toISOString(),
  failed: false,
  completed: false
});

const groupColors = [
  "bg-mint text-ink",
  "bg-yolk text-ink",
  "bg-tomato text-ink",
  "bg-plum text-white"
];

export function VibeGridGame({ puzzle }: { puzzle: PublicPuzzle }) {
  const storageKey = `vibegrid:attempt:${puzzle.id}`;
  const [attempt, setAttempt] = useState<StoredAttempt>(() => emptyAttempt(puzzle.id));
  const [message, setMessage] = useState("Fresh grid. Dangerous confidence optional.");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [copied, setCopied] = useState(false);
  const [hasLoadedAttempt, setHasLoadedAttempt] = useState(false);
  const [tileOrder, setTileOrder] = useState(() => puzzle.tiles.map((tile) => tile.id));
  const [stats, setStats] = useState<PuzzleStats | null>(null);

  useEffect(() => {
    const storedAttempt = window.localStorage.getItem(storageKey);

    if (!storedAttempt) {
      setHasLoadedAttempt(true);
      return;
    }

    try {
      const parsed = JSON.parse(storedAttempt) as StoredAttempt;

      if (parsed.puzzleId === puzzle.id) {
        setAttempt({
          ...emptyAttempt(puzzle.id),
          ...parsed,
          revealedGroups: parsed.revealedGroups ?? []
        });
      }
    } catch {
      window.localStorage.removeItem(storageKey);
    }

    setHasLoadedAttempt(true);
  }, [puzzle.id, storageKey]);

  useEffect(() => {
    if (!hasLoadedAttempt) {
      return;
    }

    let cancelled = false;

    async function loadAttempt() {
      try {
        const response = await fetch(`/api/attempts/${puzzle.id}`, {
          credentials: "include"
        });

        if (!response.ok) {
          return;
        }

        const serverAttempt = (await response.json()) as AttemptSnapshot;
        if (!cancelled) {
          setAttempt((current) => mergeServerAttempt(current, serverAttempt));
        }
      } catch {
        setMessage("Could not sync attempt. Local board is still here.");
      }
    }

    void loadAttempt();

    return () => {
      cancelled = true;
    };
  }, [hasLoadedAttempt, puzzle.id]);

  useEffect(() => {
    if (!hasLoadedAttempt) {
      return;
    }

    window.localStorage.setItem(storageKey, JSON.stringify(attempt));
  }, [attempt, hasLoadedAttempt, storageKey]);

  const displayedGroups = useMemo(() => {
    const solvedGroupIds = new Set(attempt.solvedGroups.map((group) => group.id));
    const revealedOnly = attempt.revealedGroups.filter((group) => !solvedGroupIds.has(group.id));

    return [...attempt.solvedGroups, ...revealedOnly].sort(
      (left, right) => left.colorIndex - right.colorIndex
    );
  }, [attempt.revealedGroups, attempt.solvedGroups]);

  const displayedTileIds = useMemo(
    () => new Set(displayedGroups.flatMap((group) => group.tileIds)),
    [displayedGroups]
  );

  const selectedTileIds = new Set(attempt.selectedTileIds);
  const isComplete = attempt.completed || attempt.solvedGroups.length === puzzle.groupCount;
  const isOver = isComplete || attempt.failed;

  useEffect(() => {
    if (!isOver) {
      return;
    }

    let cancelled = false;
    fetchPuzzleStats(puzzle.id)
      .then((loaded) => {
        if (!cancelled) {
          setStats(loaded);
        }
      })
      .catch(() => {
        // Stats are a nice-to-have; a failure should never disrupt the result screen.
      });

    return () => {
      cancelled = true;
    };
  }, [isOver, puzzle.id]);

  const tilesById = useMemo(() => new Map(puzzle.tiles.map((tile) => [tile.id, tile])), [puzzle.tiles]);
  const remainingTiles = tileOrder
    .map((tileId) => tilesById.get(tileId))
    .filter((tile): tile is Tile => tile !== undefined)
    .filter((tile) => !displayedTileIds.has(tile.id));

  function toggleTile(tileId: string) {
    if (isOver || displayedTileIds.has(tileId)) {
      return;
    }

    setCopied(false);
    setAttempt((current) => {
      const isSelected = current.selectedTileIds.includes(tileId);

      if (isSelected) {
        return {
          ...current,
          selectedTileIds: current.selectedTileIds.filter((selectedTileId) => selectedTileId !== tileId)
        };
      }

      if (current.selectedTileIds.length === 4) {
        return current;
      }

      return {
        ...current,
        selectedTileIds: [...current.selectedTileIds, tileId]
      };
    });
  }

  function shuffleRemaining() {
    setTileOrder((currentOrder) => {
      const unsolved = currentOrder.filter((tileId) => !displayedTileIds.has(tileId));
      const solved = currentOrder.filter((tileId) => displayedTileIds.has(tileId));
      const rotated = unsolved.length > 1 ? [...unsolved.slice(1), unsolved[0]] : unsolved;

      return [...solved, ...rotated];
    });
  }

  async function submitGuess() {
    if (attempt.selectedTileIds.length !== 4 || isSubmitting || isOver) {
      return;
    }

    setIsSubmitting(true);
    setCopied(false);

    try {
      const response = await fetch("/api/guesses", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify({
          puzzleId: puzzle.id,
          selectedTileIds: attempt.selectedTileIds,
          clientGuessId: crypto.randomUUID()
        })
      });

      const result = (await response.json()) as GuessResponse;

      setAttempt((current) => {
        if (!result.ok) {
          setMessage(result.error);
          return current;
        }

        const nextAttempt = mergeServerAttempt(
          {
            ...current,
            selectedTileIds: []
          },
          result.attempt
        );

        if (result.isCorrect) {
          setMessage(nextAttempt.completed ? "All vibes found. Suspiciously competent." : result.group.name);
          return nextAttempt;
        }

        setMessage(nextAttempt.failed ? "Four misses. The grid wins today." : "Not that vibe.");
        return nextAttempt;
      });
    } catch {
      setMessage("Could not submit. The grid is being dramatic.");
    } finally {
      setIsSubmitting(false);
    }
  }

  function resetAttempt() {
    const nextAttempt = emptyAttempt(puzzle.id);
    setAttempt(nextAttempt);
    setCopied(false);
    setMessage("Fresh grid. Dangerous confidence optional.");
    window.localStorage.setItem(storageKey, JSON.stringify(nextAttempt));
  }

  async function shareResult() {
    const shareText = buildShareText({
      puzzleNumber: puzzle.puzzleNumber,
      mistakes: attempt.mistakes,
      mistakesAllowed: puzzle.mistakesAllowed,
      solvedCount: attempt.solvedGroups.length,
      groupCount: puzzle.groupCount,
      startedAt: attempt.startedAt,
      finishedAt: attempt.completedAt,
      failed: attempt.failed
    });

    await navigator.clipboard.writeText(shareText);
    setCopied(true);
  }

  return (
    <div className="mx-auto grid min-h-[calc(100vh-2.5rem)] max-w-6xl grid-rows-[auto_1fr] gap-5">
      <header className="flex flex-wrap items-center justify-between gap-4 border-b-4 border-ink pb-4">
        <div className="flex items-center gap-3">
          <Image src="/vibegrid-mark.svg" width={48} height={48} alt="" className="rounded" priority />
          <div>
            <h1 className="text-3xl font-black leading-none sm:text-4xl">VibeGrid</h1>
            <p className="mt-1 text-sm font-semibold text-neutral-600">
              #{puzzle.puzzleNumber}
              {puzzle.publishDate ? ` / ${puzzle.publishDate}` : ""}
            </p>
          </div>
        </div>

        <nav className="flex items-center gap-2">
          <HowToPlay />
          <Link
            href="/create"
            aria-label="Make your own"
            title="Make your own"
            className="inline-flex h-11 w-11 items-center justify-center rounded border-2 border-ink bg-white shadow-[0_4px_0_#171717]"
          >
            <Sparkles aria-hidden size={18} />
          </Link>
          <Link
            href="/archive"
            aria-label="Archive"
            title="Archive"
            className="inline-flex h-11 w-11 items-center justify-center rounded border-2 border-ink bg-white shadow-[0_4px_0_#171717]"
          >
            <Archive aria-hidden size={18} />
          </Link>
          <button
            aria-label="Shuffle tiles"
            title="Shuffle tiles"
            className="inline-flex h-11 w-11 items-center justify-center rounded border-2 border-ink bg-white shadow-[0_4px_0_#171717] disabled:opacity-40"
            disabled={isOver || remainingTiles.length < 2}
            type="button"
            onClick={shuffleRemaining}
          >
            <Shuffle aria-hidden size={18} />
          </button>
          <button
            aria-label="Reset attempt"
            title="Reset attempt"
            className="inline-flex h-11 w-11 items-center justify-center rounded border-2 border-ink bg-white shadow-[0_4px_0_#171717]"
            type="button"
            onClick={resetAttempt}
          >
            <RotateCcw aria-hidden size={18} />
          </button>
        </nav>
      </header>

      <section className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="min-w-0">
          <div className="grid gap-3">
            {displayedGroups.map((group) => {
              const isSolved = attempt.solvedGroups.some((solvedGroup) => solvedGroup.id === group.id);

              return (
                <section
                  key={group.id}
                  className={clsx(
                    "rounded border-2 border-ink p-4 shadow-[0_5px_0_#171717]",
                    groupColors[group.colorIndex % groupColors.length]
                  )}
                >
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <h2 className="text-xl font-black">{group.name}</h2>
                      <p className="mt-1 text-sm font-semibold opacity-80">{group.explanation}</p>
                    </div>
                    <p className="text-sm font-black uppercase tracking-[0.14em]">
                      {isSolved ? "Locked" : "Revealed"}
                    </p>
                  </div>
                  <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
                    {group.tiles.map((tile) => (
                      <div
                        key={tile.id}
                        className="flex min-h-14 items-center justify-center rounded border border-ink bg-white/80 px-2 text-center text-sm font-black"
                      >
                        {tile.text}
                      </div>
                    ))}
                  </div>
                </section>
              );
            })}
          </div>

          <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4 sm:gap-3">
            {remainingTiles.map((tile) => {
              const isSelected = selectedTileIds.has(tile.id);

              return (
                <button
                  key={tile.id}
                  className={clsx(
                    "flex aspect-[1.45] min-h-20 items-center justify-center rounded border-2 border-ink px-2 text-center text-base font-black shadow-tile transition sm:text-lg",
                    isSelected
                      ? "translate-y-1 bg-ink text-white shadow-[0_4px_0_rgba(23,23,23,0.18)]"
                      : "bg-white hover:-translate-y-0.5 hover:bg-yolk"
                  )}
                  type="button"
                  aria-pressed={isSelected}
                  onClick={() => toggleTile(tile.id)}
                >
                  <span className="break-words leading-tight">{tile.text}</span>
                </button>
              );
            })}
          </div>
        </div>

        <aside className="flex flex-col justify-between gap-4 rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
          <div>
            <div className="grid grid-cols-2 gap-3">
              <div className="rounded border border-neutral-200 p-3">
                <p className="text-xs font-black uppercase tracking-[0.14em] text-neutral-500">Selected</p>
                <p className="mt-1 text-2xl font-black">{attempt.selectedTileIds.length}/4</p>
              </div>
              <div className="rounded border border-neutral-200 p-3">
                <p className="text-xs font-black uppercase tracking-[0.14em] text-neutral-500">Mistakes</p>
                <p className="mt-1 text-2xl font-black">
                  {attempt.mistakes}/{puzzle.mistakesAllowed}
                </p>
              </div>
            </div>

            <div className="mt-4 grid grid-cols-4 gap-2" aria-label="Mistake counter">
              {Array.from({ length: puzzle.mistakesAllowed }).map((_, index) => (
                <div
                  key={index}
                  className={clsx(
                    "h-3 rounded border border-ink",
                    index < attempt.mistakes ? "bg-tomato" : "bg-neutral-100"
                  )}
                />
              ))}
            </div>

            <p className="mt-5 min-h-12 text-lg font-black leading-snug">{message}</p>

            <div className="mt-4 border-t border-neutral-200 pt-4 text-sm font-semibold text-neutral-600">
              <p>Elapsed {formatElapsedTime(attempt.startedAt, attempt.completedAt)}</p>
              <p className="mt-1">Guesses {attempt.guessCount}</p>
            </div>

            {isOver && stats && stats.players > 0 && (
              <div className="mt-4 rounded border-2 border-ink bg-plum/10 p-3">
                <p className="text-xs font-black uppercase tracking-[0.14em] text-plum">How others did</p>
                <div className="mt-2 grid grid-cols-2 gap-2 text-sm font-semibold">
                  <p>{Math.round(stats.solveRate * 100)}% solved</p>
                  <p>{stats.players} {stats.players === 1 ? "player" : "players"}</p>
                  <p>~{stats.medianMistakes.toFixed(0)} mistakes</p>
                  {stats.medianSolveSeconds !== undefined && (
                    <p>~{formatSeconds(stats.medianSolveSeconds)} median</p>
                  )}
                </div>
              </div>
            )}
          </div>

          <div className="grid gap-2">
            {!isOver ? (
              <button
                className="inline-flex h-12 items-center justify-center gap-2 rounded border-2 border-ink bg-mint px-4 font-black shadow-[0_5px_0_#171717] disabled:translate-y-1 disabled:bg-neutral-200 disabled:text-neutral-500 disabled:shadow-[0_2px_0_#171717]"
                type="button"
                disabled={attempt.selectedTileIds.length !== 4 || isSubmitting}
                onClick={submitGuess}
              >
                <Send aria-hidden size={18} />
                Submit
              </button>
            ) : (
              <button
                className="inline-flex h-12 items-center justify-center gap-2 rounded border-2 border-ink bg-yolk px-4 font-black shadow-[0_5px_0_#171717]"
                type="button"
                onClick={shareResult}
              >
                <Share2 aria-hidden size={18} />
                {copied ? "Copied" : "Share"}
              </button>
            )}

            <Link
              href="/create"
              className="inline-flex h-11 items-center justify-center gap-2 rounded border border-neutral-300 bg-white px-4 text-sm font-black text-neutral-700"
            >
              <Sparkles aria-hidden size={16} />
              Make your own
            </Link>
          </div>
        </aside>
      </section>
    </div>
  );
}

function mergeServerAttempt(current: StoredAttempt, serverAttempt: AttemptSnapshot): StoredAttempt {
  const displayedTileIds = new Set(
    [...serverAttempt.solvedGroups, ...serverAttempt.revealedGroups].flatMap((group) => group.tileIds)
  );

  return {
    ...current,
    puzzleId: serverAttempt.puzzleId,
    selectedTileIds:
      serverAttempt.completed || serverAttempt.failed
        ? []
        : current.selectedTileIds.filter((tileId) => !displayedTileIds.has(tileId)),
    solvedGroups: serverAttempt.solvedGroups,
    revealedGroups: serverAttempt.revealedGroups,
    mistakes: serverAttempt.mistakes,
    guessCount: serverAttempt.guessCount,
    startedAt: serverAttempt.startedAt,
    completedAt: serverAttempt.completedAt,
    failed: serverAttempt.failed,
    completed: serverAttempt.completed
  };
}
