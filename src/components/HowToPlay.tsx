"use client";

import { useEffect, useState } from "react";
import { HelpCircle, X } from "lucide-react";

const SEEN_KEY = "vibegrid:seenHowTo";

const rules = [
  "Pick four tiles you think share a vibe, then hit Submit.",
  "Right: the group locks and names itself. Wrong: a mistake.",
  "Four mistakes and the grid wins. Solve all four to win.",
  "Share your result without spoiling the answers."
];

// An illustrative group — deliberately not from any real puzzle, so it teaches
// the idea (and the red-herring twist) without spoiling a live grid.
const exampleTiles = ["meal prep", "face mask", "clean sheets", "to-do list"];

// HowToPlay is a help button plus a modal. It opens automatically the first time
// a visitor lands (tracked in localStorage) and on demand after that.
export function HowToPlay() {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (!window.localStorage.getItem(SEEN_KEY)) {
      setOpen(true);
      window.localStorage.setItem(SEEN_KEY, "1");
    }
  }, []);

  useEffect(() => {
    if (!open) {
      return;
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open]);

  return (
    <>
      <button
        type="button"
        aria-label="How to play"
        title="How to play"
        onClick={() => setOpen(true)}
        className="inline-flex h-11 w-11 items-center justify-center rounded border-2 border-ink bg-white shadow-[0_4px_0_#171717]"
      >
        <HelpCircle aria-hidden size={18} />
      </button>

      {open && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label="How to play"
          className="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto bg-ink/40 p-4"
          onClick={() => setOpen(false)}
        >
          <div
            className="w-full max-w-md rounded border-2 border-ink bg-white p-5 shadow-[0_8px_0_#171717]"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="flex items-center justify-between">
              <h2 className="text-2xl font-black">How to play</h2>
              <button
                type="button"
                aria-label="Close"
                onClick={() => setOpen(false)}
                className="inline-flex h-9 w-9 items-center justify-center rounded border border-neutral-300"
              >
                <X aria-hidden size={18} />
              </button>
            </div>

            <p className="mt-2 text-sm font-semibold text-neutral-700">
              Sort 16 tiles into 4 hidden groups. Each group shares a{" "}
              <span className="text-plum">vibe</span> — a theme, a mood, a very specific kind of person.
            </p>

            <ul className="mt-3 grid gap-2 text-sm font-semibold text-neutral-700">
              {rules.map((rule) => (
                <li key={rule} className="flex gap-2">
                  <span aria-hidden className="font-black text-plum">
                    →
                  </span>
                  <span>{rule}</span>
                </li>
              ))}
            </ul>

            <div className="mt-4 rounded border-2 border-ink bg-neutral-50 p-3">
              <p className="text-xs font-black uppercase tracking-[0.14em] text-plum">Example</p>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {exampleTiles.map((tile) => (
                  <span key={tile} className="rounded border border-ink bg-white px-2 py-1 text-xs font-black">
                    {tile}
                  </span>
                ))}
              </div>
              <p className="mt-2 text-sm font-bold">
                <span aria-hidden className="text-plum">
                  →
                </span>{" "}
                Sunday reset
              </p>
              <p className="mt-1 text-sm text-neutral-600">
                Tiles are built to mislead: a face mask reads as skincare, but here the vibe is a
                lazy Sunday. Expect overlaps — that is the whole game.
              </p>
            </div>

            <button
              type="button"
              onClick={() => setOpen(false)}
              className="mt-5 inline-flex h-11 w-full items-center justify-center rounded border-2 border-ink bg-mint font-black shadow-[0_4px_0_#171717]"
            >
              Got it
            </button>
          </div>
        </div>
      )}
    </>
  );
}
