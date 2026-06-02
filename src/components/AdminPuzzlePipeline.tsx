"use client";

import { useEffect, useState } from "react";
import type { PublicPuzzle } from "@/types/puzzle";

type PipelineState =
  | { status: "loading" }
  | { status: "ready"; puzzles: PublicPuzzle[] }
  | { status: "error"; message: string };

export function AdminPuzzlePipeline() {
  const [state, setState] = useState<PipelineState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;

    async function loadPuzzles() {
      try {
        const response = await fetch("/api/puzzles", {
          credentials: "include"
        });

        if (!response.ok) {
          throw new Error("Could not load puzzle pipeline.");
        }

        const puzzles = (await response.json()) as PublicPuzzle[];
        if (!cancelled) {
          setState({ status: "ready", puzzles });
        }
      } catch {
        if (!cancelled) {
          setState({ status: "error", message: "Could not load puzzle pipeline." });
        }
      }
    }

    void loadPuzzles();

    return () => {
      cancelled = true;
    };
  }, []);

  if (state.status === "loading") {
    return <p className="py-4 font-black text-neutral-600">Loading pipeline.</p>;
  }

  if (state.status === "error") {
    return <p className="py-4 font-black text-tomato">{state.message}</p>;
  }

  return (
    <div className="divide-y divide-neutral-200">
      {state.puzzles.map((puzzle) => (
        <div key={puzzle.id} className="grid grid-cols-[auto_1fr_auto] items-center gap-3 py-4">
          <span className="font-black">{puzzle.puzzleNumber}</span>
          <div>
            <p className="font-black">{puzzle.publishDate}</p>
            <p className="text-sm text-neutral-600">{puzzle.tiles.length} tiles ready for review</p>
          </div>
          <span className="rounded bg-mint px-3 py-1 text-xs font-black uppercase tracking-[0.12em]">
            Published
          </span>
        </div>
      ))}
    </div>
  );
}

