"use client";

import { useCallback, useEffect, useState } from "react";
import clsx from "clsx";
import type { AdminPuzzle, Difficulty, DraftPuzzleInput } from "@/types/puzzle";
import { createDraftPuzzle, fetchAdminPuzzles, publishPuzzle } from "@/lib/adminApi";

const TOKEN_KEY = "vibegrid:adminToken";
const GROUP_COUNT = 4;
const TILES_PER_GROUP = 4;

const emptyGroup = () => ({ name: "", explanation: "", tiles: Array<string>(TILES_PER_GROUP).fill("") });

const emptyDraft = (): DraftPuzzleInput => ({
  difficulty: "MEDIUM",
  groups: Array.from({ length: GROUP_COUNT }, emptyGroup)
});

const statusStyles: Record<string, string> = {
  PUBLISHED: "bg-mint",
  DRAFT: "bg-yolk",
  ARCHIVED: "bg-neutral-200"
};

export function AdminDesk() {
  const [token, setToken] = useState("");
  const [tokenInput, setTokenInput] = useState("");
  const [puzzles, setPuzzles] = useState<AdminPuzzle[] | null>(null);
  const [draft, setDraft] = useState<DraftPuzzleInput>(emptyDraft);
  const [publishDates, setPublishDates] = useState<Record<string, string>>({});
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

  function updateGroup(groupIndex: number, field: "name" | "explanation", value: string) {
    setDraft((current) => {
      const groups = current.groups.map((group, index) =>
        index === groupIndex ? { ...group, [field]: value } : group
      );
      return { ...current, groups };
    });
  }

  function updateTile(groupIndex: number, tileIndex: number, value: string) {
    setDraft((current) => {
      const groups = current.groups.map((group, index) => {
        if (index !== groupIndex) {
          return group;
        }
        const tiles = group.tiles.map((tile, position) => (position === tileIndex ? value : tile));
        return { ...group, tiles };
      });
      return { ...current, groups };
    });
  }

  async function submitDraft() {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const created = await createDraftPuzzle(token, draft);
      setDraft(emptyDraft());
      setNotice(`Draft #${created.puzzleNumber} saved.`);
      await loadPuzzles(token);
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : "Could not save the draft.");
    } finally {
      setBusy(false);
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
            <div key={puzzle.id} className="grid gap-3 py-4 sm:grid-cols-[auto_1fr_auto] sm:items-center">
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
          ))}
        </div>
      </section>

      <section className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-black">New draft</h2>
          <label className="flex items-center gap-2 text-sm font-black">
            Difficulty
            <select
              value={draft.difficulty}
              onChange={(event) =>
                setDraft((current) => ({ ...current, difficulty: event.target.value as Difficulty }))
              }
              className="h-9 rounded border-2 border-ink px-2 font-semibold"
            >
              <option value="EASY">EASY</option>
              <option value="MEDIUM">MEDIUM</option>
              <option value="HARD">HARD</option>
            </select>
          </label>
        </div>

        <div className="mt-4 grid gap-4 lg:grid-cols-2">
          {draft.groups.map((group, groupIndex) => (
            <fieldset key={groupIndex} className="rounded border-2 border-ink p-3">
              <legend className="px-1 text-xs font-black uppercase tracking-[0.12em] text-neutral-500">
                Group {groupIndex + 1}
              </legend>
              <input
                value={group.name}
                onChange={(event) => updateGroup(groupIndex, "name", event.target.value)}
                placeholder="Category name (e.g. Corporate séance)"
                className="h-10 w-full rounded border border-neutral-300 px-2 font-bold"
              />
              <input
                value={group.explanation}
                onChange={(event) => updateGroup(groupIndex, "explanation", event.target.value)}
                placeholder="One-line explanation shown on reveal"
                className="mt-2 h-10 w-full rounded border border-neutral-300 px-2 text-sm"
              />
              <div className="mt-2 grid grid-cols-2 gap-2">
                {group.tiles.map((tile, tileIndex) => (
                  <input
                    key={tileIndex}
                    value={tile}
                    onChange={(event) => updateTile(groupIndex, tileIndex, event.target.value)}
                    placeholder={`Tile ${tileIndex + 1}`}
                    className="h-10 w-full rounded border border-neutral-300 px-2 text-sm font-semibold"
                  />
                ))}
              </div>
            </fieldset>
          ))}
        </div>

        <button
          type="button"
          disabled={busy}
          onClick={submitDraft}
          className="mt-4 inline-flex h-11 items-center justify-center rounded border-2 border-ink bg-yolk px-5 font-black shadow-[0_4px_0_#171717] disabled:opacity-50"
        >
          {busy ? "Saving…" : "Save draft"}
        </button>
        <p className="mt-2 text-xs text-neutral-500">
          Four groups of four. All sixteen tiles must be unique; the server validates before saving.
        </p>
      </section>
    </div>
  );
}
