"use client";

import { useEffect, useState } from "react";
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
  return (
    <div className="mx-auto grid min-h-[calc(100vh-2.5rem)] max-w-5xl content-center gap-6">
      <header className="border-b-4 border-ink pb-5">
        <p className="text-sm font-black text-plum">Session start</p>
        <h1 className="mt-2 text-4xl font-black leading-tight sm:text-5xl">VibeGrid</h1>
        <p className="mt-3 max-w-2xl font-semibold leading-7 text-neutral-600">
          Choose how this browser should enter. Player accounts are not part of this build; play uses a
          private guest session cookie, while editor access uses the admin password.
        </p>
      </header>

      <section className="grid gap-3 md:grid-cols-3">
        <button
          type="button"
          onClick={onPlayGuest}
          className="group grid min-h-44 content-between rounded border-2 border-ink bg-mint p-4 text-left shadow-[0_6px_0_#171717] transition hover:-translate-y-0.5"
        >
          <span className="inline-flex h-10 w-10 items-center justify-center rounded border-2 border-ink bg-white">
            <UserRound aria-hidden size={20} />
          </span>
          <span>
            <span className="block text-2xl font-black">Play as guest</span>
            <span className="mt-2 block text-sm font-bold leading-6 text-neutral-700">
              Browser-saved attempts, refresh recovery, streaks when Postgres is attached.
            </span>
          </span>
          <span className="inline-flex items-center gap-2 text-sm font-black">
            Start <ArrowRight aria-hidden size={16} />
          </span>
        </button>

        <Link
          href="/demo"
          className="group grid min-h-44 content-between rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717] transition hover:-translate-y-0.5"
        >
          <span className="inline-flex h-10 w-10 items-center justify-center rounded border-2 border-ink bg-yolk">
            <Compass aria-hidden size={20} />
          </span>
          <span>
            <span className="block text-2xl font-black">Demo room</span>
            <span className="mt-2 block text-sm font-bold leading-6 text-neutral-600">
              A seeded room link for showing separate guest attempts in another browser.
            </span>
          </span>
          <span className="inline-flex items-center gap-2 text-sm font-black">
            Open <ArrowRight aria-hidden size={16} />
          </span>
        </Link>

        <Link
          href="/admin"
          className="group grid min-h-44 content-between rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717] transition hover:-translate-y-0.5"
        >
          <span className="inline-flex h-10 w-10 items-center justify-center rounded border-2 border-ink bg-plum text-white">
            <ShieldCheck aria-hidden size={20} />
          </span>
          <span>
            <span className="block text-2xl font-black">Editor login</span>
            <span className="mt-2 block text-sm font-bold leading-6 text-neutral-600">
              Password-protected puzzle, publishing, analytics, and moderation desk.
            </span>
          </span>
          <span className="inline-flex items-center gap-2 text-sm font-black">
            Sign in <ArrowRight aria-hidden size={16} />
          </span>
        </Link>
      </section>
    </div>
  );
}

function StatusCard({ title, message }: { title: string; message: string }) {
  return (
    <div className="mx-auto flex min-h-[calc(100vh-2.5rem)] max-w-3xl items-center justify-center">
      <div className="w-full rounded border-2 border-ink bg-white p-5 text-center shadow-[0_6px_0_#171717]">
        <h1 className="text-3xl font-black">{title}</h1>
        <p className="mt-3 font-semibold text-neutral-600">{message}</p>
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
