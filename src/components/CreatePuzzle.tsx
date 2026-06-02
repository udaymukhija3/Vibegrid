"use client";

import { useState } from "react";
import Link from "next/link";
import { Copy, ExternalLink } from "lucide-react";
import { createCommunityPuzzle } from "@/lib/api";
import { PuzzleDraftForm } from "@/components/PuzzleDraftForm";

type Share = { id: string; url: string; number: number };

export function CreatePuzzle() {
  const [share, setShare] = useState<Share | null>(null);
  const [copied, setCopied] = useState(false);

  async function copyLink() {
    if (!share) {
      return;
    }
    await navigator.clipboard.writeText(share.url);
    setCopied(true);
  }

  return (
    <div className="mt-6 grid gap-6">
      {share && (
        <section className="rounded border-2 border-ink bg-mint p-4 shadow-[0_6px_0_#171717]">
          <h2 className="text-lg font-black">Your grid is live</h2>
          <p className="mt-1 text-sm font-semibold">
            VibeGrid #{share.number} — share this link and see who finds the vibe.
          </p>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <code className="max-w-full overflow-x-auto rounded border-2 border-ink bg-white px-3 py-2 text-sm font-bold">
              {share.url}
            </code>
            <button
              type="button"
              onClick={copyLink}
              className="inline-flex h-10 items-center gap-2 rounded border-2 border-ink bg-white px-3 text-sm font-black shadow-[0_3px_0_#171717]"
            >
              <Copy aria-hidden size={16} />
              {copied ? "Copied" : "Copy link"}
            </button>
            <Link
              href={`/p/${share.id}`}
              className="inline-flex h-10 items-center gap-2 rounded border-2 border-ink bg-yolk px-3 text-sm font-black shadow-[0_3px_0_#171717]"
            >
              <ExternalLink aria-hidden size={16} />
              Play it
            </Link>
          </div>
        </section>
      )}

      <section className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
        <PuzzleDraftForm
          submitLabel="Create & get link"
          onSubmit={async (input) => {
            const created = await createCommunityPuzzle(input);
            setShare({
              id: created.id,
              url: `${window.location.origin}/p/${created.id}`,
              number: created.puzzleNumber
            });
            setCopied(false);
          }}
        />
      </section>
    </div>
  );
}
