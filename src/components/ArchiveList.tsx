"use client";

import { fetchPublishedPuzzles } from "@/lib/api";
import { useResource } from "@/hooks/useResource";

export function ArchiveList() {
  const state = useResource(fetchPublishedPuzzles, "Could not load archive.");

  if (state.status === "loading") {
    return <p className="mt-6 font-black text-neutral-600">Loading archive.</p>;
  }

  if (state.status === "error") {
    return <p className="mt-6 font-black text-tomato">{state.message}</p>;
  }

  return (
    <section className="mt-6 grid gap-3">
      {state.data.map((puzzle) => (
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
