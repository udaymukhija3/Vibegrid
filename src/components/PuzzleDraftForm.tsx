"use client";

import { useState } from "react";
import { toast } from "sonner";
import type { Difficulty, DraftPuzzleInput } from "@/types/puzzle";
import {
  MAX_GROUP_EXPLANATION_LENGTH,
  MAX_GROUP_NAME_LENGTH,
  MAX_TILE_TEXT_LENGTH
} from "@/lib/puzzleLimits";

const GROUP_COUNT = 4;
const TILES_PER_GROUP = 4;

const emptyGroup = () => ({ name: "", explanation: "", tiles: Array<string>(TILES_PER_GROUP).fill("") });

export const emptyDraft = (): DraftPuzzleInput => ({
  difficulty: "MEDIUM",
  groups: Array.from({ length: GROUP_COUNT }, emptyGroup)
});

// PuzzleDraftForm renders the four-group / sixteen-tile authoring grid and owns
// its own input state. It is reused by the admin Editor Desk and the public
// "make your own" page; the parent supplies what happens on submit.
export function PuzzleDraftForm({
  onSubmit,
  submitLabel
}: {
  onSubmit: (input: DraftPuzzleInput) => Promise<void>;
  submitLabel: string;
}) {
  const [draft, setDraft] = useState<DraftPuzzleInput>(emptyDraft);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  function updateGroup(groupIndex: number, field: "name" | "explanation", value: string) {
    setDraft((current) => ({
      ...current,
      groups: current.groups.map((group, index) =>
        index === groupIndex ? { ...group, [field]: value } : group
      )
    }));
  }

  function updateTile(groupIndex: number, tileIndex: number, value: string) {
    setDraft((current) => ({
      ...current,
      groups: current.groups.map((group, index) =>
        index === groupIndex
          ? { ...group, tiles: group.tiles.map((tile, position) => (position === tileIndex ? value : tile)) }
          : group
      )
    }));
  }

  async function submit() {
    setBusy(true);
    setError("");
    try {
      await onSubmit(draft);
      setDraft(emptyDraft());
    } catch (submitError) {
      const message = submitError instanceof Error ? submitError.message : "Could not save the puzzle.";
      setError(message);
      toast.error(message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-black">Build a grid</h2>
        <label className="flex items-center gap-2 text-sm font-black">
          Difficulty
          <select
            value={draft.difficulty}
            onChange={(event) =>
              setDraft((current) => ({ ...current, difficulty: event.target.value as Difficulty }))
            }
            className="h-9 rounded border-2 border-ink px-2 font-semibold"
          >
            <option value="EASY">Easy</option>
            <option value="MEDIUM">Medium</option>
            <option value="HARD">Hard</option>
          </select>
        </label>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        {draft.groups.map((group, groupIndex) => (
          <fieldset key={groupIndex} className="rounded border-2 border-ink p-3">
            <legend className="px-1 text-xs font-black text-neutral-500">
              Group {groupIndex + 1}
            </legend>
            <input
              value={group.name}
              onChange={(event) => updateGroup(groupIndex, "name", event.target.value)}
              maxLength={MAX_GROUP_NAME_LENGTH}
              placeholder="Category name (e.g. Corporate séance)"
              className="h-10 w-full rounded border border-neutral-300 px-2 font-bold"
            />
            <input
              value={group.explanation}
              onChange={(event) => updateGroup(groupIndex, "explanation", event.target.value)}
              maxLength={MAX_GROUP_EXPLANATION_LENGTH}
              placeholder="One-line explanation shown on reveal"
              className="mt-2 h-10 w-full rounded border border-neutral-300 px-2 text-sm"
            />
            <div className="mt-2 grid grid-cols-2 gap-2">
              {group.tiles.map((tile, tileIndex) => (
                <input
                  key={tileIndex}
                  value={tile}
                  onChange={(event) => updateTile(groupIndex, tileIndex, event.target.value)}
                  maxLength={MAX_TILE_TEXT_LENGTH}
                  placeholder={`Tile ${tileIndex + 1}`}
                  className="h-10 w-full rounded border border-neutral-300 px-2 text-sm font-semibold"
                />
              ))}
            </div>
          </fieldset>
        ))}
      </div>

      {error && <p className="mt-3 text-sm font-bold text-tomato">{error}</p>}

      <button
        type="button"
        disabled={busy}
        onClick={submit}
        className="mt-4 inline-flex h-11 items-center justify-center rounded border-2 border-ink bg-yolk px-5 font-black shadow-[0_4px_0_#171717] disabled:opacity-50"
      >
        {busy ? "Saving…" : submitLabel}
      </button>
      <p className="mt-2 text-xs text-neutral-500">
        Four groups of four. Keep the wording sharp and fair; repeated tiles are blocked before saving.
      </p>
    </div>
  );
}
