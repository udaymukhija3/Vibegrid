"use client";

import { useCallback } from "react";
import Link from "next/link";
import { fetchPuzzleById } from "@/lib/api";
import { useResource } from "@/hooks/useResource";
import { VibeGridGame } from "@/components/VibeGridGame";

export function PlaySharedPuzzle({ puzzleId }: { puzzleId: string }) {
  const loader = useCallback(() => fetchPuzzleById(puzzleId), [puzzleId]);
  const state = useResource(loader, "This puzzle could not be found.");

  if (state.status === "ready") {
    return <VibeGridGame puzzle={state.data} />;
  }

  return (
    <div className="mx-auto flex min-h-[calc(100vh-2.5rem)] max-w-3xl items-center justify-center">
      <div className="w-full rounded border-2 border-ink bg-white p-5 text-center shadow-[0_6px_0_#171717]">
        <h1 className="text-3xl font-black">VibeGrid</h1>
        <p className="mt-3 font-semibold text-neutral-600">
          {state.status === "loading" ? "Loading this grid." : state.message}
        </p>
        {state.status === "error" && (
          <Link
            href="/"
            className="mt-4 inline-flex h-11 items-center justify-center rounded border-2 border-ink bg-mint px-4 font-black shadow-[0_4px_0_#171717]"
          >
            Play today&apos;s puzzle
          </Link>
        )}
      </div>
    </div>
  );
}
