"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Play } from "lucide-react";
import { toast } from "sonner";
import { ARCHIVE_PAGE_SIZE, fetchPublishedPuzzles } from "@/lib/api";
import { formatDifficulty } from "@/lib/displayLabels";
import type { PublicPuzzle } from "@/types/puzzle";

export function ArchiveList() {
  const [puzzles, setPuzzles] = useState<PublicPuzzle[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");
  const [message, setMessage] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [reachedEnd, setReachedEnd] = useState(false);

  // loadPage fetches the page starting at offset and appends any new puzzles.
  // A page shorter than ARCHIVE_PAGE_SIZE means we have reached the end.
  const loadPage = useCallback(async (offset: number) => {
    const page = await fetchPublishedPuzzles(ARCHIVE_PAGE_SIZE, offset);
    setPuzzles((current) => {
      const seen = new Set(current.map((puzzle) => puzzle.id));
      const merged = [...current];
      for (const puzzle of page) {
        if (!seen.has(puzzle.id)) {
          merged.push(puzzle);
        }
      }
      return merged;
    });
    if (page.length < ARCHIVE_PAGE_SIZE) {
      setReachedEnd(true);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await loadPage(0);
        if (!cancelled) {
          setStatus("ready");
        }
      } catch (error) {
        if (!cancelled) {
          setMessage(error instanceof Error ? error.message : "Could not load archive.");
          setStatus("error");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [loadPage]);

  async function loadMore() {
    if (loadingMore || reachedEnd) {
      return;
    }
    setLoadingMore(true);
    try {
      await loadPage(puzzles.length);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not load more.");
    } finally {
      setLoadingMore(false);
    }
  }

  if (status === "loading") {
    return <p className="mt-6 font-black text-neutral-600">Loading archive.</p>;
  }

  if (status === "error") {
    return <p className="mt-6 font-black text-tomato">{message}</p>;
  }

  if (puzzles.length === 0) {
    return <p className="mt-6 font-black text-neutral-600">No puzzles in the archive yet.</p>;
  }

  return (
    <>
      <section className="mt-6 grid gap-3">
        {puzzles.map((puzzle) => (
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

      {!reachedEnd && (
        <button
          type="button"
          onClick={loadMore}
          disabled={loadingMore}
          className="mt-4 inline-flex h-11 w-full items-center justify-center rounded border-2 border-ink bg-white px-4 font-black shadow-[0_4px_0_#171717] disabled:opacity-50"
        >
          {loadingMore ? "Loading" : "Load more"}
        </button>
      )}
    </>
  );
}
