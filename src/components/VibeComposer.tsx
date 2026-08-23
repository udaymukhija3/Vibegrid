"use client";

import { FormEvent, useId, useMemo, useState } from "react";
import clsx from "clsx";
import { ArrowRight, Check, RotateCcw } from "lucide-react";
import type { VibeBoard } from "@/types/vibe";

type VibeComposerProps = {
  board: VibeBoard;
  onSubmit: (input: { title: string; selectedTileIds: string[] }) => Promise<void> | void;
  submitLabel?: string;
  compact?: boolean;
};

export function VibeComposer({ board, onSubmit, submitLabel = "Lock it in", compact = false }: VibeComposerProps) {
  const [selected, setSelected] = useState<string[]>([]);
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const titleInputId = useId();
  const titleCountId = `${titleInputId}-count`;

  const selectedTiles = useMemo(
    () => selected.map((id) => board.tiles.find((tile) => tile.id === id)).filter(Boolean),
    [board.tiles, selected]
  );

  function toggle(tileId: string) {
    setError("");
    setSelected((current) => {
      if (current.includes(tileId)) {
        return current.filter((id) => id !== tileId);
      }
      if (current.length === 4) {
        return current;
      }
      return [...current, tileId];
    });
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const cleanTitle = title.trim();
    if (selected.length !== 4 || !cleanTitle || busy) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await onSubmit({ title: cleanTitle, selectedTileIds: selected });
    } catch (submissionError) {
      setError(submissionError instanceof Error ? submissionError.message : "That card did not land. Try again.");
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className={clsx("grid gap-5", compact && "gap-4")}>
      <div>
        <div className="flex items-end justify-between gap-3">
          <div>
            <p className="vg-meta">Today&apos;s prompt</p>
            <h2 className="vg-prompt mt-2">{board.prompt}</h2>
          </div>
          <p className="vg-count" aria-live="polite">
            {selected.length}/4
          </p>
        </div>

        <div
          className="vg-fragment-grid mt-5"
          aria-label={`${board.tiles.length} fragments in ${board.tiles.length / 4} rows. Choose four.`}
        >
          {board.tiles.map((tile) => {
            const isSelected = selected.includes(tile.id);
            const unavailable = selected.length === 4 && !isSelected;
            return (
              <button
                key={tile.id}
                type="button"
                aria-pressed={isSelected}
                disabled={unavailable}
                onClick={() => toggle(tile.id)}
                className={clsx("vg-fragment", isSelected && "vg-fragment-selected")}
              >
                <span>{tile.text}</span>
                {isSelected && <Check aria-hidden className="vg-fragment-check" size={16} strokeWidth={3} />}
              </button>
            );
          })}
        </div>
      </div>

      <section className={clsx("vg-composer-tray", selected.length === 4 && "vg-composer-tray-ready")}>
        <div className="flex items-center justify-between gap-3">
          <p className="vg-meta">Your four</p>
          {selected.length > 0 && (
            <button
              type="button"
              className="vg-text-button"
              onClick={() => {
                setSelected([]);
                setTitle("");
              }}
            >
              <RotateCcw aria-hidden size={14} />
              Start over
            </button>
          )}
        </div>

        <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
          {Array.from({ length: 4 }, (_, index) => {
            const tile = selectedTiles[index];
            return (
              <div key={tile?.id ?? index} className={clsx("vg-picked-slot", tile && "vg-picked-slot-filled")}>
                {tile?.text ?? `pick ${index + 1}`}
              </div>
            );
          })}
        </div>

        {selected.length === 4 ? (
          <div className="mt-4 grid gap-2">
            <label htmlFor={titleInputId} className="vg-meta">Name the vibe</label>
            <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
              <input
                id={titleInputId}
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                maxLength={40}
                required
                autoFocus
                placeholder="e.g. Productivity theatre"
                aria-describedby={titleCountId}
                className="vg-title-input"
              />
              <button type="submit" disabled={busy || !title.trim()} className="vg-primary-button">
                {busy ? "Locking…" : submitLabel}
                {!busy && <ArrowRight aria-hidden size={18} strokeWidth={3} />}
              </button>
            </div>
            <span id={titleCountId} className="text-right font-mono text-[0.68rem] text-cream/[.55]">{title.length}/40</span>
          </div>
        ) : (
          <p className="mt-4 text-sm font-semibold text-cream/60">Pick {4 - selected.length} more to name what you made.</p>
        )}

        {error && (
          <p role="alert" className="mt-3 rounded-xl border-2 border-coral bg-coral/[.15] px-3 py-2 text-sm font-bold text-cream">
            {error}
          </p>
        )}
      </section>
    </form>
  );
}
