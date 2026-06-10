"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Copy, ExternalLink, Pencil, Play } from "lucide-react";
import { toast } from "sonner";
import { createCommunityPuzzle, fetchPuzzleTemplates } from "@/lib/api";
import { PuzzleDraftForm } from "@/components/PuzzleDraftForm";
import type { DraftPuzzleInput, PuzzleTemplate } from "@/types/puzzle";

type Share = { id: string; url: string; number: number };

const difficultyStyles: Record<string, string> = {
  EASY: "bg-mint",
  MEDIUM: "bg-yolk",
  HARD: "bg-tomato"
};

function toDraft(template: PuzzleTemplate): DraftPuzzleInput {
  return {
    difficulty: template.difficulty,
    groups: template.groups.map((group) => ({
      name: group.name,
      explanation: group.explanation,
      tiles: [...group.tiles]
    }))
  };
}

export function CreatePuzzle() {
  const [share, setShare] = useState<Share | null>(null);
  const [copied, setCopied] = useState(false);
  const [templates, setTemplates] = useState<PuzzleTemplate[] | null>(null);
  const [initialDraft, setInitialDraft] = useState<DraftPuzzleInput | undefined>(undefined);
  const [loadedTitle, setLoadedTitle] = useState<string | null>(null);
  const [formKey, setFormKey] = useState(0);
  const [playingId, setPlayingId] = useState<string | null>(null);
  const formRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetchPuzzleTemplates()
      .then((loaded) => {
        if (!cancelled) {
          setTemplates(loaded);
        }
      })
      .catch(() => {
        // The picker is a convenience; if it fails to load, the blank builder still works.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function publishDraft(input: DraftPuzzleInput) {
    const created = await createCommunityPuzzle(input);
    setShare({
      id: created.id,
      url: `${window.location.origin}/p/${created.id}`,
      number: created.puzzleNumber
    });
    setCopied(false);
  }

  async function copyLink() {
    if (!share) {
      return;
    }
    try {
      await navigator.clipboard.writeText(share.url);
      setCopied(true);
      toast.success("Copied link.");
    } catch {
      toast.error("Could not copy link.");
    }
  }

  async function playTemplate(template: PuzzleTemplate) {
    if (playingId) {
      return;
    }
    setPlayingId(template.id);
    try {
      await publishDraft(toDraft(template));
      toast.success("Your grid is live.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not create that grid.");
    } finally {
      setPlayingId(null);
    }
  }

  function applyTemplate(template: PuzzleTemplate) {
    setInitialDraft(toDraft(template));
    setLoadedTitle(template.title);
    setFormKey((key) => key + 1);
    toast.success(`Loaded "${template.title}" — tweak and publish.`);
    requestAnimationFrame(() => formRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }));
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

      {templates && templates.length > 0 && (
        <section className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
          <h2 className="text-lg font-black">Start from a pack</h2>
          <p className="mt-1 text-sm font-semibold text-neutral-600">
            No blank-page panic. Play one as-is, or load it below to make it yours.
          </p>
          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            {templates.map((template) => (
              <div key={template.id} className="flex flex-col justify-between rounded border-2 border-ink p-3">
                <div className="flex items-start justify-between gap-2">
                  <h3 className="text-base font-black leading-tight">{template.title}</h3>
                  <span
                    className={`shrink-0 rounded border-2 border-ink px-2 py-0.5 text-[0.65rem] font-black ${
                      difficultyStyles[template.difficulty] ?? "bg-white"
                    }`}
                  >
                    {template.difficulty}
                  </span>
                </div>
                <p className="mt-1 text-xs font-semibold text-neutral-500">
                  {template.groups.length} vibes · {template.groups.length * 4} tiles
                </p>
                <div className="mt-3 grid grid-cols-2 gap-2">
                  <button
                    type="button"
                    disabled={playingId !== null}
                    onClick={() => void playTemplate(template)}
                    className="inline-flex h-9 items-center justify-center gap-1.5 rounded border-2 border-ink bg-mint px-2 text-sm font-black shadow-[0_3px_0_#171717] disabled:opacity-50"
                  >
                    <Play aria-hidden size={15} />
                    {playingId === template.id ? "Creating…" : "Play this"}
                  </button>
                  <button
                    type="button"
                    onClick={() => applyTemplate(template)}
                    className="inline-flex h-9 items-center justify-center gap-1.5 rounded border-2 border-ink bg-white px-2 text-sm font-black shadow-[0_3px_0_#171717]"
                  >
                    <Pencil aria-hidden size={15} />
                    Use as template
                  </button>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      <section ref={formRef} className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
        {loadedTitle && (
          <p className="mb-3 rounded border-2 border-ink bg-yolk px-3 py-2 text-xs font-black">
            Prefilled from “{loadedTitle}”. Edit anything, then publish.
          </p>
        )}
        <PuzzleDraftForm
          key={formKey}
          initialDraft={initialDraft}
          submitLabel="Create & get link"
          onSubmit={async (input) => {
            await publishDraft(input);
            toast.success("Your grid is live.");
          }}
        />
        <p className="mt-4 text-xs font-semibold leading-5 text-neutral-500">
          By creating a grid, you agree to the{" "}
          <Link href="/policy" className="font-black text-ink underline decoration-2 underline-offset-4">
            community rules
          </Link>
          ,{" "}
          <Link href="/terms" className="font-black text-ink underline decoration-2 underline-offset-4">
            terms
          </Link>
          , and{" "}
          <Link href="/privacy" className="font-black text-ink underline decoration-2 underline-offset-4">
            privacy notice
          </Link>
          .
        </p>
      </section>
    </div>
  );
}
