"use client";

import { useEffect, useState } from "react";
import { VibeGridGame } from "@/components/VibeGridGame";
import type { PublicPuzzle } from "@/types/puzzle";

type LoadState =
  | { status: "loading" }
  | { status: "ready"; puzzle: PublicPuzzle }
  | { status: "error"; message: string };

export function VibeGridApp() {
  const [state, setState] = useState<LoadState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;

    async function loadPuzzle() {
      try {
        const response = await fetch("/api/puzzles/today", {
          credentials: "include"
        });

        if (!response.ok) {
          throw new Error("Could not load today's grid.");
        }

        const puzzle = (await response.json()) as PublicPuzzle;
        if (!cancelled) {
          setState({ status: "ready", puzzle });
        }
      } catch {
        if (!cancelled) {
          setState({ status: "error", message: "Could not load today's grid." });
        }
      }
    }

    void loadPuzzle();

    return () => {
      cancelled = true;
    };
  }, []);

  if (state.status === "ready") {
    return <VibeGridGame puzzle={state.puzzle} />;
  }

  return (
    <div className="mx-auto flex min-h-[calc(100vh-2.5rem)] max-w-3xl items-center justify-center">
      <div className="w-full rounded border-2 border-ink bg-white p-5 text-center shadow-[0_6px_0_#171717]">
        <h1 className="text-3xl font-black">VibeGrid</h1>
        <p className="mt-3 font-semibold text-neutral-600">
          {state.status === "loading" ? "Loading today's grid." : state.message}
        </p>
      </div>
    </div>
  );
}

