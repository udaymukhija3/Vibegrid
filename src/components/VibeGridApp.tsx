"use client";

import { useEffect, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { ArrowRight, Compass, ShieldCheck, UserRound } from "lucide-react";
import { VibeGridGame } from "@/components/VibeGridGame";
import { fetchSessionStatus, fetchTodayPuzzle } from "@/lib/api";
import { useResource } from "@/hooks/useResource";

const ENTRY_STORAGE_KEY = "vibegrid_entry";

export function VibeGridApp() {
  const [entry, setEntry] = useState<"checking" | "choose" | "guest">("checking");

  useEffect(() => {
    setEntry(safeStorage()?.getItem(ENTRY_STORAGE_KEY) === "guest" ? "guest" : "choose");
  }, []);

  function playAsGuest() {
    safeStorage()?.setItem(ENTRY_STORAGE_KEY, "guest");
    setEntry("guest");
  }

  if (entry === "checking") {
    return <StatusCard title="VibeGrid" message="Checking this browser." />;
  }

  if (entry === "choose") {
    return <EntryScreen onPlayGuest={playAsGuest} />;
  }

  return <TodayGame />;
}

function TodayGame() {
  const puzzleState = useResource(fetchTodayPuzzle, "Could not load today's grid.");
  const sessionState = useResource(fetchSessionStatus, "Could not load this session.");

  if (puzzleState.status === "ready") {
    return (
      <VibeGridGame
        puzzle={puzzleState.data}
        sessionStatus={sessionState.status === "ready" ? sessionState.data : null}
      />
    );
  }

  return (
    <StatusCard
      title="VibeGrid"
      message={puzzleState.status === "loading" ? "Loading today's grid." : puzzleState.message}
    />
  );
}

function EntryScreen({ onPlayGuest }: { onPlayGuest: () => void }) {
  const sampleTiles = [
    "group chat",
    "train window",
    "leftovers",
    "soft launch",
    "rain check",
    "orange peel",
    "voice note",
    "third place",
    "night market",
    "paper cup",
    "walk home",
    "old receipt",
    "inside joke",
    "window seat",
    "playlist",
    "last slice"
  ];

  return (
    <div className="mx-auto grid min-h-[calc(100vh-2.5rem)] max-w-7xl content-center gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <section className="vg-board-sheet">
        <div className="grid gap-5 sm:gap-6">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="flex items-center gap-3">
              <Image src="/vibegrid-mark.svg" width={50} height={50} alt="" className="rounded-lg" priority />
              <div>
                <p className="vg-kicker">Daily semantic grid</p>
                <h1 className="text-4xl font-extrabold leading-tight sm:text-6xl">VibeGrid</h1>
              </div>
            </div>
            <div className="vg-mode-track w-full max-w-sm sm:w-80" aria-hidden="true">
              {["Easy", "Medium", "Hard"].map((label, index) => (
                <span
                  key={label}
                  className={`vg-mode-tab inline-flex items-center justify-center ${
                    index === 1 ? "bg-card text-ink" : "bg-ink text-card/75"
                  }`}
                >
                  {label}
                </span>
              ))}
            </div>
          </div>

          <p className="max-w-2xl text-base font-medium leading-7 text-neutral-700 sm:text-lg">
            Sort the board by feel, not trivia. Four hidden groups, sixteen plain-language tiles,
            and just enough misdirection to make the win feel earned.
          </p>

          {/* order-last on mobile: this is an inert preview that looks identical to the
              real board, so leaving it above the CTA both buries "Play today" under the
              fold and invites taps that do nothing. Desktop keeps the original order. */}
          <div
            className="order-last rounded-lg border border-line bg-white/70 p-2 shadow-tile sm:p-3 lg:order-none"
            aria-hidden="true"
          >
            <div className="grid grid-cols-4 gap-1.5 sm:gap-2">
              {sampleTiles.map((tile, index) => (
                <span
                  key={tile}
                  className={`aspect-square min-h-16 items-center justify-center rounded-lg border border-line px-1 text-center text-[0.68rem] font-semibold leading-tight shadow-tile sm:aspect-[1.35] sm:px-2 sm:text-sm ${
                    // Half the preview is enough to convey the idea; sixteen tiles is a
                    // second full-height board to scroll past on a phone.
                    index >= 8 ? "hidden sm:flex" : "flex"
                  } ${
                    index % 4 === 0
                      ? "bg-mint/30"
                      : index % 4 === 1
                        ? "bg-yolk/[.32]"
                        : index % 4 === 2
                          ? "bg-pool/25"
                          : "bg-card"
                  }`}
                >
                  {tile}
                </span>
              ))}
            </div>
          </div>

          <button type="button" onClick={onPlayGuest} className="vg-button-primary w-full justify-between sm:w-fit">
            <span className="inline-flex items-center gap-2">
              <UserRound aria-hidden size={18} />
              Play today
            </span>
            <ArrowRight aria-hidden size={17} />
          </button>
        </div>
      </section>

      <aside className="vg-control-rail grid content-start gap-3">
        <div className="rounded-lg border border-line bg-card/90 p-4">
          <p className="text-sm font-semibold text-neutral-500">This browser</p>
          <p className="mt-2 text-sm font-medium leading-6 text-neutral-700">
            Guest play saves progress locally. Editor tools stay behind the admin password.
          </p>
        </div>

        <Link href="/demo" className="group grid gap-3 rounded-lg border border-line bg-card/90 p-4 transition hover:-translate-y-0.5 hover:border-ink hover:shadow-lift">
          <span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-yolk/[.55]">
            <Compass aria-hidden size={20} />
          </span>
          <span>
            <span className="block text-xl font-extrabold">Demo room</span>
            <span className="mt-1 block text-sm font-medium leading-6 text-neutral-600">
              A seeded room link for showing separate guest attempts in another browser.
            </span>
          </span>
          <span className="inline-flex items-center gap-2 text-sm font-semibold">
            Open <ArrowRight aria-hidden size={16} />
          </span>
        </Link>

        {/* Editor login is owner-only. Below lg it competes with "Play today" as an
            equal-weight card, so it drops to a quiet link and stops reading like a
            step a new player is meant to take. */}
        <Link
          href="/admin"
          className="inline-flex items-center gap-2 justify-self-start rounded-lg px-1 py-2 text-sm font-semibold text-neutral-500 underline underline-offset-4 lg:hidden"
        >
          <ShieldCheck aria-hidden size={16} />
          Editor login
        </Link>

        <Link
          href="/admin"
          className="group hidden gap-3 rounded-lg border border-line bg-card/90 p-4 transition hover:-translate-y-0.5 hover:border-ink hover:shadow-lift lg:grid"
        >
          <span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-plum/[.15] text-plum">
            <ShieldCheck aria-hidden size={20} />
          </span>
          <span>
            <span className="block text-xl font-extrabold">Editor login</span>
            <span className="mt-1 block text-sm font-medium leading-6 text-neutral-600">
              Password-protected puzzle, publishing, analytics, and moderation desk.
            </span>
          </span>
          <span className="inline-flex items-center gap-2 text-sm font-semibold">
            Sign in <ArrowRight aria-hidden size={16} />
          </span>
        </Link>
      </aside>
    </div>
  );
}

function StatusCard({ title, message }: { title: string; message: string }) {
  return (
    <div className="mx-auto flex min-h-[calc(100vh-2.5rem)] max-w-3xl items-center justify-center">
      <div className="vg-board-sheet w-full text-center">
        <h1 className="text-3xl font-extrabold">{title}</h1>
        <p className="mt-3 font-medium text-neutral-600">{message}</p>
      </div>
    </div>
  );
}

function safeStorage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}
