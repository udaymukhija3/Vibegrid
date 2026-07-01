"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Pencil, Play } from "lucide-react";
import { toast } from "sonner";
import { createCommunityPuzzle, fetchPuzzleTemplates } from "@/lib/api";
import { PuzzleDraftForm } from "@/components/PuzzleDraftForm";
import type { DraftPuzzleInput, PuzzleTemplate } from "@/types/puzzle";

type Submission = { number: number };

const difficultyStyles: Record<string, string> = {
  EASY: "bg-mint/45",
  MEDIUM: "bg-yolk/50",
  HARD: "bg-tomato/35"
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
  const [submission, setSubmission] = useState<Submission | null>(null);
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
    setSubmission({ number: created.puzzleNumber });
  }

  async function playTemplate(template: PuzzleTemplate) {
    if (playingId) {
      return;
    }
    setPlayingId(template.id);
    try {
      await publishDraft(toDraft(template));
      toast.success("Your grid was sent for review.");
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
    toast.success(`Loaded "${template.title}" — tweak and submit.`);
    requestAnimationFrame(() => formRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }));
  }

  return (
    <div className="mt-6 grid gap-6">
      {submission && (
        <section className="rounded-lg border border-mint/70 bg-mint/25 p-4 shadow-soft">
          <h2 className="text-lg font-extrabold">Your grid is in review</h2>
          <p className="mt-1 text-sm font-medium">
            VibeGrid #{submission.number} is waiting for an editor check. It will become shareable only after approval.
          </p>
        </section>
      )}

      {templates && templates.length > 0 && (
        <section className="vg-panel p-4">
          <h2 className="text-lg font-extrabold">Start from a pack</h2>
          <p className="mt-1 text-sm font-medium text-neutral-600">
            No blank-page panic. Submit one as-is for review, or load it below to make it yours.
          </p>
          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            {templates.map((template) => (
              <div key={template.id} className="flex flex-col justify-between rounded-lg border border-line bg-white/60 p-3">
                <div className="flex items-start justify-between gap-2">
                  <h3 className="text-base font-extrabold leading-tight">{template.title}</h3>
                  <span
                    className={`shrink-0 rounded-lg border border-line px-2 py-0.5 text-[0.65rem] font-semibold ${
                      difficultyStyles[template.difficulty] ?? "bg-white"
                    }`}
                  >
                    {template.difficulty}
                  </span>
                </div>
                <p className="mt-1 text-xs font-medium text-neutral-500">
                  {template.groups.length} vibes · {template.groups.length * 4} tiles
                </p>
                <div className="mt-3 grid grid-cols-2 gap-2">
                  <button
                    type="button"
                    disabled={playingId !== null}
                    onClick={() => void playTemplate(template)}
                    className="vg-button-primary h-9 min-h-9 px-2 text-sm"
                  >
                    <Play aria-hidden size={15} />
                    {playingId === template.id ? "Submitting…" : "Submit this"}
                  </button>
                  <button
                    type="button"
                    onClick={() => applyTemplate(template)}
                    className="vg-button-secondary h-9 min-h-9 px-2"
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

      <section ref={formRef} className="vg-panel p-4">
        {loadedTitle && (
          <p className="mb-3 rounded-lg border border-yolk/80 bg-yolk/30 px-3 py-2 text-xs font-semibold">
            Prefilled from “{loadedTitle}”. Edit anything, then submit it for review.
          </p>
        )}
        <PuzzleDraftForm
          key={formKey}
          initialDraft={initialDraft}
          submitLabel="Submit for review"
          onSubmit={async (input) => {
            await publishDraft(input);
            toast.success("Your grid was sent for review.");
          }}
        />
        <p className="mt-4 text-xs font-semibold leading-5 text-neutral-500">
          By creating a grid, you agree to the{" "}
          <Link href="/policy" className="font-semibold text-ink underline decoration-2 underline-offset-4">
            community rules
          </Link>
          ,{" "}
          <Link href="/terms" className="font-semibold text-ink underline decoration-2 underline-offset-4">
            terms
          </Link>
          , and{" "}
          <Link href="/privacy" className="font-semibold text-ink underline decoration-2 underline-offset-4">
            privacy notice
          </Link>
          .
        </p>
      </section>
    </div>
  );
}
