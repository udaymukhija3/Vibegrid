"use client";

import { useCallback, useMemo } from "react";
import Link from "next/link";
import { fetchPuzzleById } from "@/lib/api";
import { useResource } from "@/hooks/useResource";
import { VibeGridGame } from "@/components/VibeGridGame";

export function PlaySharedPuzzle({ puzzleId: explicitPuzzleId }: { puzzleId?: string }) {
  const puzzleId = useMemo(() => explicitPuzzleId ?? puzzleIdFromPath(), [explicitPuzzleId]);
  const loader = useCallback(() => fetchPuzzleById(puzzleId), [puzzleId]);
  const state = useResource(loader, "This shared grid is not available.");

  if (state.status === "ready") {
    return (
      <div>
        <aside className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-xl border-2 border-amber bg-amber/[.12] px-4 py-3 text-sm font-bold text-cream">
          <span>Legacy grid · this link is preserved, but hidden-category puzzles are no longer the main VibeGrid product.</span>
          <Link href="/" className="underline decoration-2 underline-offset-4 hover:text-lime">
            Try the current VibeGrid
          </Link>
        </aside>
        <VibeGridGame puzzle={state.data} />
      </div>
    );
  }

  return (
    <div className="mx-auto flex min-h-[calc(100vh-2.5rem)] max-w-3xl items-center justify-center">
      <div className="vg-panel w-full p-6 text-center">
        <h1 className="text-3xl font-extrabold">VibeGrid</h1>
        <p className="mt-3 font-medium text-neutral-600">
          {state.status === "loading" ? "Loading this grid." : state.message}
        </p>
        {state.status === "error" && (
          <>
            <Link
              href="/"
              className="vg-button-primary mt-4"
            >
              Try today&apos;s practice
            </Link>
            <p className="mx-auto mt-5 max-w-xl border-t border-neutral-200 pt-5 text-sm font-medium text-neutral-600">
              Legacy grid creators can request a review from the private claim link issued at submission.
            </p>
          </>
        )}
      </div>
    </div>
  );
}

function puzzleIdFromPath() {
  if (typeof window === "undefined") {
    return "";
  }

  const [, id = ""] = window.location.pathname.match(/^\/p\/([^/]+)/) ?? [];
  return decodeURIComponent(id);
}
