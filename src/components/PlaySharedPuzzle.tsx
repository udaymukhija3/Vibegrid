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
    return <VibeGridGame puzzle={state.data} />;
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
              Play today&apos;s puzzle
            </Link>
            <p className="mx-auto mt-5 max-w-xl border-t border-neutral-200 pt-5 text-sm font-medium text-neutral-600">
              Grid creators can request a review from the private claim link issued at submission.
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
