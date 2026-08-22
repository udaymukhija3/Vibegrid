"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { CalendarDays, CheckCircle2, LockKeyhole, LogOut, Plus } from "lucide-react";
import { toast } from "sonner";
import {
  checkAdminSession,
  createAdminVibeBoard,
  fetchAdminVibeBoards,
  loginAdmin,
  logoutAdmin
} from "@/lib/adminApi";
import type { VibeBoard } from "@/types/vibe";

const EMPTY_TILES = Array.from({ length: 12 }, () => "");

function nextDate() {
  return new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString().slice(0, 10);
}

export function VibeBoardDesk() {
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [password, setPassword] = useState("");
  const [boards, setBoards] = useState<VibeBoard[]>([]);
  const [publishDate, setPublishDate] = useState("");
  const [prompt, setPrompt] = useState("");
  const [tiles, setTiles] = useState<string[]>(EMPTY_TILES);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    setBoards(await fetchAdminVibeBoards());
  }, []);

  useEffect(() => {
    setPublishDate(nextDate());
    checkAdminSession()
      .then(async (ok) => {
        setAuthenticated(ok);
        if (ok) await refresh();
      })
      .catch(() => setAuthenticated(false));
  }, [refresh]);

  const uniqueTileCount = useMemo(
    () => new Set(tiles.map((tile) => tile.trim().toLocaleLowerCase()).filter(Boolean)).size,
    [tiles]
  );
  const ready = Boolean(publishDate && prompt.trim() && uniqueTileCount === 12 && !busy);

  async function signIn() {
    if (!password.trim()) return;
    setBusy(true);
    setError("");
    try {
      await loginAdmin(password);
      setPassword("");
      setAuthenticated(true);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not sign in.");
    } finally {
      setBusy(false);
    }
  }

  async function signOut() {
    await logoutAdmin().catch(() => undefined);
    setAuthenticated(false);
    setBoards([]);
    setError("");
  }

  async function freezeBoard() {
    if (!ready) return;
    setBusy(true);
    setError("");
    try {
      const created = await createAdminVibeBoard({ publishDate, prompt, tiles });
      toast.success(`Board ${String(created.boardNumber).padStart(3, "0")} is frozen for ${created.publishDate}.`);
      setPrompt("");
      setTiles(EMPTY_TILES);
      setPublishDate(nextDate());
      await refresh();
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "Could not freeze that board.";
      setError(message);
      toast.error(message);
    } finally {
      setBusy(false);
    }
  }

  if (authenticated === null) {
    return <div className="vg-panel mt-8"><p className="vg-meta">Checking editor session…</p></div>;
  }

  if (!authenticated) {
    return (
      <section className="vg-dark-panel mt-8 max-w-lg">
        <LockKeyhole aria-hidden className="text-lime" size={28} />
        <p className="vg-meta mt-5">Private editorial tool</p>
        <h2 className="mt-2 text-3xl font-black tracking-[-0.04em]">Enter the board room.</h2>
        <p className="mt-3 text-sm font-semibold text-paper/[.65]">
          This desk freezes the prompt and 12-fragment palette for a specific date. A frozen date never mutates under players.
        </p>
        <label className="mt-6 block">
          <span className="vg-meta">Admin password</span>
          <input
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            onKeyDown={(event) => event.key === "Enter" && void signIn()}
            className="vg-input mt-2"
          />
        </label>
        <button type="button" className="vg-button vg-button-lime mt-4" disabled={busy} onClick={() => void signIn()}>
          Sign in
        </button>
        {error && <p className="mt-4 text-sm font-bold text-coral">{error}</p>}
      </section>
    );
  }

  return (
    <div className="mt-8 grid gap-8">
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-paper/[.15] pb-5">
        <div>
          <p className="vg-meta text-lime">Editorial contract</p>
          <p className="mt-2 max-w-2xl text-sm font-semibold text-paper/[.65]">
            One evocative prompt. Twelve fragments with range. No hidden answer. The crew supplies the meaning.
          </p>
        </div>
        <button type="button" className="vg-button vg-button-quiet" onClick={() => void signOut()}>
          <LogOut aria-hidden size={16} /> Sign out
        </button>
      </div>

      <section className="vg-dark-panel">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <p className="vg-meta text-lime">Next board</p>
            <h2 className="mt-2 text-3xl font-black tracking-[-0.04em]">Author the palette.</h2>
          </div>
          <p className="vg-meta">{uniqueTileCount}/12 distinct</p>
        </div>

        <div className="mt-6 grid gap-5 lg:grid-cols-[220px_1fr]">
          <label>
            <span className="vg-meta">Publish date</span>
            <input
              type="date"
              value={publishDate}
              min={new Date().toISOString().slice(0, 10)}
              onChange={(event) => setPublishDate(event.target.value)}
              className="vg-input mt-2"
            />
          </label>
          <label>
            <span className="vg-meta">Prompt · {prompt.length}/140</span>
            <input
              value={prompt}
              maxLength={140}
              onChange={(event) => setPrompt(event.target.value)}
              placeholder="Build a first impression that will not survive the evening."
              className="vg-input mt-2"
            />
          </label>
        </div>

        <fieldset className="mt-6">
          <legend className="vg-meta">Fragments · max 28 characters each</legend>
          <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {tiles.map((tile, index) => (
              <label key={index} className="relative">
                <span className="absolute left-3 top-1/2 -translate-y-1/2 font-mono text-[10px] font-bold text-ink/40">
                  {String(index + 1).padStart(2, "0")}
                </span>
                <input
                  value={tile}
                  maxLength={28}
                  onChange={(event) =>
                    setTiles((current) => current.map((value, position) => (position === index ? event.target.value : value)))
                  }
                  className="vg-input pl-10"
                  aria-label={`Fragment ${index + 1}`}
                />
              </label>
            ))}
          </div>
        </fieldset>

        <div className="mt-6 flex flex-wrap items-center justify-between gap-4 border-t border-paper/[.15] pt-5">
          <p className="max-w-xl text-xs font-semibold text-paper/[.55]">
            Freezing is intentionally irreversible. The first stored board for a date wins, so a live round cannot silently change.
          </p>
          <button type="button" className="vg-button vg-button-lime" disabled={!ready} onClick={() => void freezeBoard()}>
            <Plus aria-hidden size={18} /> Freeze board
          </button>
        </div>
        {error && <p className="mt-4 text-sm font-bold text-coral">{error}</p>}
      </section>

      <section>
        <div className="flex items-end justify-between gap-4">
          <div>
            <p className="vg-meta text-violet-light">Frozen history</p>
            <h2 className="mt-2 text-3xl font-black tracking-[-0.04em]">What ships.</h2>
          </div>
          <span className="vg-meta">latest 90</span>
        </div>
        {boards.length === 0 ? (
          <div className="vg-panel mt-5"><p className="font-bold">No database-backed boards yet. The curated fallback bank will serve today.</p></div>
        ) : (
          <div className="mt-5 grid gap-4">
            {boards.map((board) => (
              <article key={board.id} className="vg-card-row">
                <div className="flex flex-wrap items-center gap-3">
                  <span className="vg-tag bg-lime text-ink"><CheckCircle2 aria-hidden size={13} /> frozen</span>
                  <span className="vg-meta"><CalendarDays aria-hidden className="inline" size={13} /> {board.publishDate}</span>
                  <span className="vg-meta">board {String(board.boardNumber).padStart(3, "0")}</span>
                </div>
                <h3 className="mt-3 text-xl font-black">{board.prompt}</h3>
                <p className="mt-3 font-mono text-xs leading-6 text-paper/[.60]">
                  {board.tiles.map((tile) => tile.text).join(" · ")}
                </p>
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
