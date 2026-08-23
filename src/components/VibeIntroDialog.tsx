"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowRight, HelpCircle, X } from "lucide-react";
import { readStoredValue, writeStoredValue } from "@/lib/storage";

const INTRO_VERSION = "vibegrid:practice-intro:v1";
const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

type VibeIntroDialogProps = {
  onStart: () => void;
};

export function VibeIntroDialog({ onStart }: VibeIntroDialogProps) {
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const restoreRef = useRef<Element | null>(null);

  const remember = useCallback(() => {
    writeStoredValue(INTRO_VERSION, "seen");
  }, []);

  const close = useCallback(() => {
    remember();
    setOpen(false);
  }, [remember]);

  const start = useCallback(() => {
    remember();
    setOpen(false);
    window.requestAnimationFrame(onStart);
  }, [onStart, remember]);

  useEffect(() => {
    if (!readStoredValue(INTRO_VERSION)) {
      setOpen(true);
    }
  }, []);

  useEffect(() => {
    if (!open) {
      return;
    }

    restoreRef.current = document.activeElement;
    const trigger = triggerRef.current;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    const panel = panelRef.current;
    const first = panel?.querySelector<HTMLElement>("[data-intro-primary]");
    (first ?? panel)?.focus();

    return () => {
      document.body.style.overflow = previousOverflow;
      const restore = trigger ?? restoreRef.current;
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
        event.preventDefault();
        close();
        return;
      }
      if (event.key !== "Tab") {
        return;
      }

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
      } else if (event.shiftKey && active === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && active === last) {
        event.preventDefault();
        first.focus();
      }
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [close, open]);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        className="vg-header-link px-3"
        aria-label="How it works"
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
      >
        <HelpCircle aria-hidden size={17} />
        <span aria-hidden className="hidden sm:inline">How it works</span>
      </button>

      {open && (
        <div
          className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-ink-deep/80 p-3 sm:items-center sm:p-6"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) {
              close();
            }
          }}
        >
          <div
            ref={panelRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="vibe-intro-title"
            aria-describedby="vibe-intro-description"
            tabIndex={-1}
            className="w-full max-w-xl rounded-[1.7rem] border-[3px] border-ink bg-cream p-4 text-ink shadow-[8px_8px_0_var(--violet)] outline-none sm:p-7 sm:shadow-[10px_10px_0_var(--violet)]"
          >
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="font-mono text-[0.68rem] font-bold uppercase tracking-[0.13em] text-violet">
                  Welcome to VibeGrid
                </p>
                <h2 id="vibe-intro-title" className="mt-2 text-3xl font-black leading-[0.94] tracking-[-0.055em] sm:text-5xl">
                  Make the vibe.
                  <br />
                  Let the crew decide.
                </h2>
              </div>
              <button
                type="button"
                aria-label="Close introduction"
                onClick={close}
                className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border-2 border-ink bg-white transition hover:bg-lime sm:h-11 sm:w-11"
              >
                <X aria-hidden size={20} strokeWidth={3} />
              </button>
            </div>

            <p id="vibe-intro-description" className="mt-4 max-w-lg text-sm font-bold leading-6 text-ink/70 sm:mt-5 sm:text-base sm:leading-7">
              Every crew gets a four-column palette sized to the room. There is no right answer—only the card you make and what your friends feel from it.
            </p>

            <ol className="mt-4 grid gap-2 sm:mt-5 sm:grid-cols-3">
              <IntroStep number="01" title="Make" detail="Pick four and give your combination a title." tone="bg-lime" />
              <IntroStep number="02" title="Judge" detail="Come back tomorrow for a blind crew ballot." tone="bg-amber" />
              <IntroStep number="03" title="Reveal" detail="See the authors, votes, and every honest tie." tone="bg-violet-light" />
            </ol>

            <div className="mt-4 flex flex-col-reverse items-stretch gap-2 border-t-2 border-ink/15 pt-4 sm:mt-6 sm:flex-row sm:items-center sm:justify-between sm:gap-3 sm:pt-5">
              <p className="text-xs font-bold leading-5 text-ink/55">
                Practice runs all three stages now. No signup.
              </p>
              <button data-intro-primary type="button" onClick={start} className="vg-primary-button">
                Make today&apos;s card
                <ArrowRight aria-hidden size={18} strokeWidth={3} />
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

function IntroStep({ number, title, detail, tone }: { number: string; title: string; detail: string; tone: string }) {
  return (
    <li className={`rounded-xl border-2 border-ink p-2.5 sm:p-3 ${tone}`}>
      <p className="font-mono text-[0.65rem] font-bold uppercase tracking-[0.12em] text-ink/55">{number}</p>
      <p className="mt-1 text-lg font-black">{title}</p>
      <p className="mt-1 text-xs font-bold leading-5 text-ink/65">{detail}</p>
    </li>
  );
}
