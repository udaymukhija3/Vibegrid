"use client";

import { fetchPublishedPuzzles } from "@/lib/api";
import { useResource } from "@/hooks/useResource";

export function AdminPuzzlePipeline() {
  const state = useResource(fetchPublishedPuzzles, "Could not load puzzle pipeline.");

  if (state.status === "loading") {
    return <p className="py-4 font-black text-neutral-600">Loading pipeline.</p>;
  }

  if (state.status === "error") {
    return <p className="py-4 font-black text-tomato">{state.message}</p>;
  }

  return (
    <div className="divide-y divide-neutral-200">
      {state.data.map((puzzle) => (
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
