"use client";

import { useCallback, useEffect, useState } from "react";
import clsx from "clsx";
import type { AdminPuzzle } from "@/types/puzzle";
import {
  createDraftPuzzle,
  fetchAdminPuzzles,
  fetchAnalytics,
  publishPuzzle,
  type PuzzleAnalytics
} from "@/lib/adminApi";
import { formatSeconds } from "@/lib/game";
import { PuzzleDraftForm } from "@/components/PuzzleDraftForm";

const TOKEN_KEY = "vibegrid:adminToken";

const statusStyles: Record<string, string> = {
  PUBLISHED: "bg-mint",
  DRAFT: "bg-yolk",
  ARCHIVED: "bg-neutral-200"
};

export function AdminDesk() {
  const [token, setToken] = useState("");
  const [tokenInput, setTokenInput] = useState("");
  const [puzzles, setPuzzles] = useState<AdminPuzzle[] | null>(null);
  const [publishDates, setPublishDates] = useState<Record<string, string>>({});
  const [analytics, setAnalytics] = useState<Record<string, PuzzleAnalytics>>({});
  const [openAnalyticsId, setOpenAnalyticsId] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const loadPuzzles = useCallback(async (activeToken: string) => {
    setError("");
    try {
      setPuzzles(await fetchAdminPuzzles(activeToken));
    } catch (loadError) {
      setPuzzles(null);
      setError(loadError instanceof Error ? loadError.message : "Could not load puzzles.");
    }
  }, []);

  useEffect(() => {
    const stored = window.localStorage.getItem(TOKEN_KEY);
    if (stored) {
      setToken(stored);
      void loadPuzzles(stored);
    }
  }, [loadPuzzles]);

  function connect() {
    const trimmed = tokenInput.trim();
    if (!trimmed) {
      return;
    }
    window.localStorage.setItem(TOKEN_KEY, trimmed);
    setToken(trimmed);
    setTokenInput("");
    void loadPuzzles(trimmed);
  }

  function disconnect() {
    window.localStorage.removeItem(TOKEN_KEY);
    setToken("");
    setPuzzles(null);
    setNotice("");
    setError("");
  }

  async function toggleAnalytics(puzzleId: string) {
    if (openAnalyticsId === puzzleId) {
      setOpenAnalyticsId(null);
      return;
    }
    setOpenAnalyticsId(puzzleId);
    if (!analytics[puzzleId]) {
      try {
        const data = await fetchAnalytics(token, puzzleId);
        setAnalytics((current) => ({ ...current, [puzzleId]: data }));
      } catch (analyticsError) {
        setError(analyticsError instanceof Error ? analyticsError.message : "Could not load analytics.");
      }
    }
  }

  async function publish(puzzleId: string) {
    const date = publishDates[puzzleId];
    if (!date) {
      setError("Pick a publish date first.");
      return;
    }
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await publishPuzzle(token, puzzleId, date);
      setNotice(`Published for ${date}.`);
      await loadPuzzles(token);
    } catch (publishError) {
      setError(publishError instanceof Error ? publishError.message : "Could not publish.");
    } finally {
      setBusy(false);
    }
  }

  if (!token) {
    return (
      <div className="mt-6 max-w-md rounded border-2 border-ink bg-white p-5 shadow-[0_6px_0_#171717]">
        <h2 className="text-lg font-black">Admin token</h2>
        <p className="mt-1 text-sm text-neutral-600">
          Paste the value of <code className="font-mono">VIBEGRID_ADMIN_TOKEN</code> to manage puzzles.
        </p>
        <input
          type="password"
          value={tokenInput}
          onChange={(event) => setTokenInput(event.target.value)}
          onKeyDown={(event) => event.key === "Enter" && connect()}
          placeholder="admin token"
          className="mt-4 h-11 w-full rounded border-2 border-ink px-3 font-semibold"
        />
        <button
          type="button"
          onClick={connect}
          className="mt-3 inline-flex h-11 items-center justify-center rounded border-2 border-ink bg-mint px-4 font-black shadow-[0_4px_0_#171717]"
        >
          Connect
        </button>
        {error && <p className="mt-3 text-sm font-bold text-tomato">{error}</p>}
      </div>
    );
  }

  return (
    <div className="mt-6 grid gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div aria-live="polite" className="min-h-6 text-sm font-bold">
          {error && <span className="text-tomato">{error}</span>}
          {!error && notice && <span className="text-plum">{notice}</span>}
        </div>
        <button
          type="button"
          onClick={disconnect}
          className="inline-flex h-9 items-center rounded border border-neutral-300 bg-white px-3 text-sm font-black text-neutral-700"
        >
          Sign out
        </button>
      </div>

      <section className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
        <h2 className="text-lg font-black">Pipeline</h2>
        {!puzzles && <p className="mt-3 font-semibold text-neutral-600">Loading puzzles.</p>}
        {puzzles && puzzles.length === 0 && (
          <p className="mt-3 font-semibold text-neutral-600">No puzzles yet. Create the first draft below.</p>
        )}
        <div className="mt-3 divide-y divide-neutral-200">
          {puzzles?.map((puzzle) => (
            <div key={puzzle.id} className="py-4">
              <div className="grid gap-3 sm:grid-cols-[auto_1fr_auto] sm:items-center">
                <span className="font-black">#{puzzle.puzzleNumber}</span>
                <div>
                  <p className="font-black">{puzzle.groups.map((group) => group.name).join(" · ")}</p>
                  <p className="text-sm text-neutral-600">
                    {puzzle.difficulty}
                    {puzzle.publishDate ? ` · ${puzzle.publishDate}` : ""}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <span
                    className={clsx(
                      "rounded px-3 py-1 text-xs font-black uppercase tracking-[0.12em]",
                      statusStyles[puzzle.status] ?? "bg-neutral-200"
                    )}
                  >
                    {puzzle.status}
                  </span>
                  <button
                    type="button"
                    onClick={() => toggleAnalytics(puzzle.id)}
                    aria-expanded={openAnalyticsId === puzzle.id}
                    className="inline-flex h-9 items-center rounded border border-neutral-300 bg-white px-3 text-sm font-black text-neutral-700"
                  >
                    Stats
                  </button>
                  {puzzle.status === "DRAFT" && (
                    <>
                      <input
                        type="date"
                        value={publishDates[puzzle.id] ?? ""}
                        onChange={(event) =>
                          setPublishDates((current) => ({ ...current, [puzzle.id]: event.target.value }))
                        }
                        className="h-9 rounded border-2 border-ink px-2 text-sm font-semibold"
                      />
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => publish(puzzle.id)}
                        className="inline-flex h-9 items-center rounded border-2 border-ink bg-mint px-3 text-sm font-black shadow-[0_3px_0_#171717] disabled:opacity-50"
                      >
                        Publish
                      </button>
                    </>
                  )}
                </div>
              </div>

              {openAnalyticsId === puzzle.id && <AnalyticsPanel data={analytics[puzzle.id]} />}
            </div>
          ))}
        </div>
      </section>

      <section className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
        <PuzzleDraftForm
          submitLabel="Save draft"
          onSubmit={async (input) => {
            const created = await createDraftPuzzle(token, input);
            setNotice(`Draft #${created.puzzleNumber} saved.`);
            setError("");
            await loadPuzzles(token);
          }}
        />
      </section>
    </div>
  );
}

function AnalyticsPanel({ data }: { data: PuzzleAnalytics | undefined }) {
  if (!data) {
    return <p className="mt-3 text-sm font-semibold text-neutral-600">Loading analytics…</p>;
  }

  const { stats, wrongGuesses } = data;

  if (stats.players === 0) {
    return <p className="mt-3 text-sm font-semibold text-neutral-600">No plays yet.</p>;
  }

  return (
    <div className="mt-3 rounded border-2 border-ink bg-neutral-50 p-3">
      <div className="grid grid-cols-2 gap-2 text-sm font-semibold sm:grid-cols-4">
        <p>{stats.players} {stats.players === 1 ? "player" : "players"}</p>
        <p>{Math.round(stats.solveRate * 100)}% solved</p>
        <p>~{stats.medianMistakes.toFixed(1)} mistakes</p>
        {stats.medianSolveSeconds !== undefined && <p>~{formatSeconds(stats.medianSolveSeconds)} median</p>}
      </div>

      <p className="mt-3 text-xs font-black uppercase tracking-[0.12em] text-neutral-500">
        Most common wrong guesses
      </p>
      {wrongGuesses.length === 0 ? (
        <p className="mt-1 text-sm text-neutral-500">No wrong guesses recorded yet.</p>
      ) : (
        <ul className="mt-2 grid gap-1">
          {wrongGuesses.map((wrong, index) => (
            <li key={index} className="flex items-center justify-between gap-3 text-sm">
              <span className="font-semibold">{wrong.tiles.join(", ")}</span>
              <span className="font-black text-tomato">×{wrong.count}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
