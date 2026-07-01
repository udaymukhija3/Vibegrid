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
    "third place"
  ];

  return (
    <div className="mx-auto grid min-h-[calc(100vh-2.5rem)] max-w-6xl content-center gap-5 lg:grid-cols-[minmax(0,1fr)_22rem]">
      <section className="vg-panel overflow-hidden">
        <div className="grid gap-6 p-5 sm:p-7">
          <div className="flex items-center gap-3">
            <Image src="/vibegrid-mark.svg" width={44} height={44} alt="" className="rounded" priority />
            <div>
              <p className="vg-kicker">Daily semantic grid</p>
              <h1 className="text-4xl font-extrabold leading-tight sm:text-5xl">VibeGrid</h1>
            </div>
          </div>

          <p className="max-w-2xl text-base font-medium leading-7 text-neutral-700">
            Sort the board by feel, not trivia. Four hidden groups, sixteen plain-language tiles,
            and just enough misdirection to make the win feel earned.
          </p>

          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4" aria-hidden="true">
            {sampleTiles.map((tile, index) => (
              <span
                key={tile}
                className={`flex min-h-16 items-center justify-center rounded-lg border border-line px-2 text-center text-sm font-semibold ${
                  index % 3 === 0 ? "bg-mint/35" : index % 3 === 1 ? "bg-yolk/35" : "bg-card"
                }`}
              >
                {tile}
              </span>
            ))}
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

      <aside className="grid content-start gap-3">
        <div className="vg-panel p-4">
          <p className="text-sm font-semibold text-neutral-500">This browser</p>
          <p className="mt-2 text-sm font-medium leading-6 text-neutral-700">
            Guest play saves progress locally. Editor tools stay behind the admin password.
          </p>
        </div>

        <Link href="/demo" className="vg-panel group grid gap-3 p-4 transition hover:-translate-y-0.5 hover:shadow-lift">
          <span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-yolk/55">
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

        <button
          type="button"
          onClick={onPlayGuest}
          className="vg-panel group grid gap-3 p-4 text-left transition hover:-translate-y-0.5 hover:shadow-lift lg:hidden"
        >
          <span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-mint/55">
            <UserRound aria-hidden size={20} />
          </span>
          <span>
            <span className="block text-xl font-extrabold">Play as guest</span>
            <span className="mt-1 block text-sm font-medium leading-6 text-neutral-600">
              Browser-saved attempts, refresh recovery, and no account setup.
            </span>
          </span>
          <span className="inline-flex items-center gap-2 text-sm font-semibold">
            Start <ArrowRight aria-hidden size={16} />
          </span>
        </button>

        <Link
          href="/admin"
          className="vg-panel group grid gap-3 p-4 transition hover:-translate-y-0.5 hover:shadow-lift"
        >
          <span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-plum/15 text-plum">
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
      <div className="vg-panel w-full p-6 text-center">
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
