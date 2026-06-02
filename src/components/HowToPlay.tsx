"use client";

import { useEffect, useState } from "react";
import { HelpCircle, X } from "lucide-react";

const SEEN_KEY = "vibegrid:seenHowTo";

const rules = [
  "Find four groups of four tiles that share a hidden vibe.",
  "Select exactly four tiles, then hit Submit.",
  "A correct group locks and reveals its category name.",
  "Four wrong guesses ends the run — the grid wins.",
  "Solve all four to win, then share your result (no spoilers)."
];

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
          className="fixed inset-0 z-50 flex items-center justify-center bg-ink/40 p-4"
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
            <ul className="mt-4 grid gap-2 text-sm font-semibold text-neutral-700">
              {rules.map((rule) => (
                <li key={rule} className="flex gap-2">
                  <span aria-hidden className="font-black text-plum">
                    →
                  </span>
                  <span>{rule}</span>
                </li>
              ))}
            </ul>
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
