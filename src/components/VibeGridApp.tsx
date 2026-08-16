"use client";

import { VibeGridGame } from "@/components/VibeGridGame";
import { fetchSessionStatus, fetchTodayPuzzle } from "@/lib/api";
import { useResource } from "@/hooks/useResource";

export function VibeGridApp() {
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
