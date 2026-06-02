"use client";

import { useEffect, useState } from "react";
import type { PublicPuzzle } from "@/types/puzzle";

type ArchiveState =
  | { status: "loading" }
  | { status: "ready"; puzzles: PublicPuzzle[] }
  | { status: "error"; message: string };

export function ArchiveList() {
  const [state, setState] = useState<ArchiveState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;

    async function loadArchive() {
      try {
        const response = await fetch("/api/puzzles", {
          credentials: "include"
        });

        if (!response.ok) {
          throw new Error("Could not load archive.");
        }

        const puzzles = (await response.json()) as PublicPuzzle[];
        if (!cancelled) {
          setState({ status: "ready", puzzles });
        }
      } catch {
        if (!cancelled) {
          setState({ status: "error", message: "Could not load archive." });
        }
      }
    }

    void loadArchive();

    return () => {
      cancelled = true;
    };
  }, []);

  if (state.status === "loading") {
    return <p className="mt-6 font-black text-neutral-600">Loading archive.</p>;
  }

  if (state.status === "error") {
    return <p className="mt-6 font-black text-tomato">{state.message}</p>;
  }

  return (
    <section className="mt-6 grid gap-3">
      {state.puzzles.map((puzzle) => (
        <article
          key={puzzle.id}
          className="grid grid-cols-[1fr_auto] items-center gap-4 rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]"
        >
          <div>
            <p className="text-lg font-black">VibeGrid #{puzzle.puzzleNumber}</p>
            <p className="text-sm text-neutral-600">{puzzle.publishDate}</p>
          </div>
          <span className="rounded bg-yolk px-3 py-1 text-xs font-black uppercase tracking-[0.12em]">
            {puzzle.difficulty}
          </span>
        </article>
      ))}
    </section>
  );
}

