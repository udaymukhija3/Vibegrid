"use client";

import { VibeGridGame } from "@/components/VibeGridGame";
import { fetchTodayPuzzle } from "@/lib/api";
import { useResource } from "@/hooks/useResource";

export function VibeGridApp() {
  const state = useResource(fetchTodayPuzzle, "Could not load today's grid.");

  if (state.status === "ready") {
    return <VibeGridGame puzzle={state.data} />;
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
