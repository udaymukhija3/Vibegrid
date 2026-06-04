"use client";

import Link from "next/link";
import { Play } from "lucide-react";
import { fetchPublishedPuzzles } from "@/lib/api";
import { formatDifficulty } from "@/lib/displayLabels";
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
        <Link
          key={puzzle.id}
          href={`/p/${puzzle.id}`}
          className="grid grid-cols-[1fr_auto] items-center gap-4 rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717] transition hover:-translate-y-0.5 hover:bg-yolk/20"
        >
          <div>
            <p className="text-lg font-black">VibeGrid #{puzzle.puzzleNumber}</p>
            <p className="text-sm text-neutral-600">{puzzle.publishDate}</p>
          </div>
          <div className="flex items-center gap-2">
            <span className="rounded bg-yolk px-3 py-1 text-xs font-black">
              {formatDifficulty(puzzle.difficulty)}
            </span>
            <span className="inline-flex h-9 w-9 items-center justify-center rounded border-2 border-ink bg-white">
              <Play aria-hidden size={16} />
            </span>
          </div>
        </Link>
      ))}
    </section>
  );
}
