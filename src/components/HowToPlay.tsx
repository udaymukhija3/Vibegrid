"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { HelpCircle, X } from "lucide-react";
import { readSessionValue, writeSessionValue } from "@/lib/storage";

// Dismissal is remembered for this visit only. Whether the dialog opens on the
// *next* visit is answered by the server ("have you ever finished a grid?"),
// not by this key — see the auto-open effect.
const DISMISSED_KEY = "vibegrid:howToDismissed";

const rules = [
  "Pick four tiles you think share a vibe, then hit Submit.",
  "Right: the group locks and names itself. Wrong: a mistake.",
  "Four mistakes and the grid wins. Solve all four to win.",
  "Share your result without spoiling the answers."
];

// An illustrative group — deliberately not from any real puzzle, so it teaches
// the idea (and the red-herring twist) without spoiling a live grid.
const exampleTiles = ["meal prep", "face mask", "clean sheets", "to-do list"];

// Everything a Tab press can land on inside the dialog.
const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

// HowToPlay is a help button plus a modal. It opens automatically for anyone who
// has not yet finished a grid, and on demand from the button after that.
//
// hasFinishedAGrid comes from the server (completed dailies for this session):
// null while unknown, so the dialog never flashes past a returning player before
// the answer arrives. A browser-local "seen" flag used to decide this, which was
// wrong in both directions — incognito, a cache clear, or Safari capping
// script-writable storage re-prompted regulars, while a first-timer who cleared
// the dialog once never got it back even mid-learning.
export function HowToPlay({ hasFinishedAGrid }: { hasFinishedAGrid: boolean | null }) {
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  // Where focus was before the dialog opened, so it can be handed back. On the
  // first-visit auto-open there is no trigger, so this may be the body.
  const restoreRef = useRef<Element | null>(null);

  const close = useCallback(() => {
    setOpen(false);
    // Best-effort: if session storage is blocked the dialog can reopen on a
    // route change, which is a smaller failure than never explaining the game.
    writeSessionValue(DISMISSED_KEY, "1");
  }, []);

  useEffect(() => {
    // Wait for the answer. Opening while it is null would flash the dialog at
    // every returning player on every load.
    if (hasFinishedAGrid !== false) {
      return;
    }
    if (readSessionValue(DISMISSED_KEY)) {
      return;
    }
    setOpen(true);
  }, [hasFinishedAGrid]);

  // Move focus into the dialog on open and hand it back on close. Without this,
  // focus stayed on <body> while aria-modal="true" hid the rest of the page from
  // assistive tech — so a screen reader user tabbed onto background controls that
  // AT reported as not existing.
  useEffect(() => {
    if (!open) {
      return;
    }
    restoreRef.current = document.activeElement;
    const trigger = triggerRef.current;
    const restoreTarget = restoreRef.current;
    const panel = panelRef.current;
    const first = panel?.querySelector<HTMLElement>(FOCUSABLE);
    (first ?? panel)?.focus();

    return () => {
      const restore = trigger ?? restoreTarget;
      if (restore instanceof HTMLElement && document.contains(restore)) {
        restore.focus();
      }
    };
  }, [open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        close();
        return;
      }
      if (event.key !== "Tab") {
        return;
      }
      // Keep Tab inside the dialog: aria-modal="true" claims the rest of the
      // page is inert, so focus must not leave.
      const panel = panelRef.current;
      if (!panel) {
        return;
      }
      const items = Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
        (item) => item.offsetParent !== null || item === document.activeElement
      );
      if (items.length === 0) {
        event.preventDefault();
        panel.focus();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;
      if (!panel.contains(active)) {
        event.preventDefault();
        first.focus();
        return;
      }
      if (event.shiftKey && active === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && active === last) {
        event.preventDefault();
        first.focus();
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, close]);

  return (
    <>
      <button
        type="button"
        aria-label="How to play"
        title="How to play"
        onClick={() => setOpen(true)}
        ref={triggerRef}
        className="vg-icon-button"
      >
        <HelpCircle aria-hidden size={18} />
      </button>

      {open && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label="How to play"
          className="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto bg-ink/40 p-4"
          onClick={close}
        >
          <div
            // Explicit text-ink: the dialog can be mounted inside the dark spine
            // rail (text-card), and inherited cream text would vanish on the
            // light panel.
            ref={panelRef}
            tabIndex={-1}
            className="vg-panel w-full max-w-md p-5 text-ink"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="flex items-center justify-between">
              <h2 className="text-2xl font-extrabold">How to play</h2>
              <button
                type="button"
                aria-label="Close"
                onClick={close}
                className="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-card"
              >
                <X aria-hidden size={18} />
              </button>
            </div>

            <p className="mt-2 text-sm font-medium text-neutral-700">
              Sort 16 tiles into 4 hidden groups. Each group shares a{" "}
              <span className="text-plum">vibe</span> — a theme, a mood, a very specific kind of person.
            </p>

            <ul className="mt-3 grid gap-2 text-sm font-medium text-neutral-700">
              {rules.map((rule) => (
                <li key={rule} className="flex gap-2">
                  <span aria-hidden className="font-semibold text-plum">
                    →
                  </span>
                  <span>{rule}</span>
                </li>
              ))}
            </ul>

            <div className="mt-4 rounded-lg border border-line bg-white/70 p-3">
              <p className="text-xs font-semibold text-plum">Example</p>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {exampleTiles.map((tile) => (
                  <span key={tile} className="rounded-lg border border-line bg-card px-2 py-1 text-xs font-semibold">
                    {tile}
                  </span>
                ))}
              </div>
              <p className="mt-2 text-sm font-semibold">
                <span aria-hidden className="text-plum">
                  →
                </span>{" "}
                Sunday reset
              </p>
              <p className="mt-1 text-sm leading-6 text-neutral-600">
                Tiles are built to mislead: a face mask reads as skincare, but here the vibe is a
                lazy Sunday. Expect overlaps — that is the whole game.
              </p>
            </div>

            <button
              type="button"
              onClick={close}
              className="vg-button-primary mt-5 w-full"
            >
              Got it
            </button>
          </div>
        </div>
      )}
    </>
  );
}
