"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import Image from "next/image";
import Link from "next/link";
import clsx from "clsx";
import { Archive, Compass, Flag, Send, Share2, Shuffle, Sparkles, X } from "lucide-react";
import { toast } from "sonner";
import {
  ATTEMPT_STORAGE_PREFIX,
  buildShareGrid,
  buildShareText,
  cleanupStoredAttempts,
  formatElapsedTime,
  formatSeconds,
  MIN_STATS_PLAYERS
} from "@/lib/game";
import {
  fetchPuzzleStats,
  fetchSessionStatus,
  fetchStreak,
  fetchVibes,
  reportPuzzle,
  type SessionStatus,
  type PuzzleStats,
  type StreakSummary
} from "@/lib/api";
import { apiFetch } from "@/lib/http";
import { HowToPlay } from "@/components/HowToPlay";
import type { AttemptSnapshot, GuessResponse, PublicPuzzle, SolvedGroup, Tile, VibeHint } from "@/types/puzzle";

type StoredAttempt = {
  puzzleId: string;
  selectedTileIds: string[];
  solvedGroups: SolvedGroup[];
  revealedGroups: SolvedGroup[];
  // Ordered list of every submitted guess (the four tile ids per guess), kept
  // locally so the result screen can render a spoiler-safe share grid.
  guessHistory: string[][];
  mistakes: number;
  guessCount: number;
  startedAt: string;
  completedAt?: string;
  failed: boolean;
  completed: boolean;
};

const emptyAttempt = (puzzleId: string): StoredAttempt => ({
  puzzleId,
  selectedTileIds: [],
  solvedGroups: [],
  revealedGroups: [],
  guessHistory: [],
  mistakes: 0,
  guessCount: 0,
  startedAt: new Date().toISOString(),
  failed: false,
  completed: false
});

// safeStorage returns localStorage, or null when it is unavailable (private
// mode, blocked cookies/storage). Even reading window.localStorage can throw, so
// every access goes through here and degrades to an in-memory-only session.
function safeStorage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

const asArray = <T,>(value: unknown): T[] => (Array.isArray(value) ? (value as T[]) : []);
const asCount = (value: unknown): number => (typeof value === "number" && Number.isFinite(value) ? value : 0);

// normalizeStoredAttempt defends against tampered or schema-drifted localStorage:
// a valid-JSON-but-wrong-shape blob (e.g. solvedGroups not an array) would crash
// the board on render, so every field is coerced back to a safe type.
function normalizeStoredAttempt(puzzleId: string, parsed: Partial<StoredAttempt>): StoredAttempt {
  return {
    ...emptyAttempt(puzzleId),
    ...parsed,
    puzzleId,
    selectedTileIds: asArray<string>(parsed.selectedTileIds),
    solvedGroups: asArray<SolvedGroup>(parsed.solvedGroups),
    revealedGroups: asArray<SolvedGroup>(parsed.revealedGroups),
    guessHistory: asArray<string[]>(parsed.guessHistory),
    mistakes: asCount(parsed.mistakes),
    guessCount: asCount(parsed.guessCount)
  };
}

const groupColors = [
  "bg-mint text-ink",
  "bg-yolk text-ink",
  "bg-tomato text-ink",
  "bg-plum text-white"
];

// Background-only palette (matching groupColors) for the share-grid squares.
const squareColors = ["bg-mint", "bg-yolk", "bg-tomato", "bg-plum"];

// Standard = guided (reveal one vibe at a time to match); Hard = all vibes
// hidden (the original deduce-it-yourself game). The choice is a per-browser
// preference, not a per-puzzle one.
type GameMode = "standard" | "hard";
const MODE_STORAGE_KEY = "vibegrid_mode";

const reportReasons = [
  { value: "OFFENSIVE", label: "Hateful or abusive" },
  { value: "PERSONAL_INFO", label: "Personal information" },
  { value: "SPAM", label: "Spam or scam" },
  { value: "UNFAIR", label: "Broken or unfair" },
  { value: "COPYRIGHT", label: "Copyright issue" },
  { value: "OTHER", label: "Something else" }
] as const;

type ReportReason = (typeof reportReasons)[number]["value"];

const openingMessages = [
  "Select four tiles for the current vibe.",
  "Start with the tiles that clearly belong together.",
  "Pick four that share the same vibe.",
  "Use the clue, then lock in four tiles.",
  "Find the first group to get moving."
];

function openingMessageForPuzzle(puzzle: PublicPuzzle) {
  if (puzzle.id.startsWith("demo-")) {
    return "Demo room ready. Pick four tiles to begin.";
  }

  const seed = puzzle.publishDate ?? puzzle.id;
  const hash = Array.from(seed).reduce((total, char) => total + char.charCodeAt(0), 0);
  return openingMessages[hash % openingMessages.length];
}

export function VibeGridGame({
  puzzle,
  sessionStatus
}: {
  puzzle: PublicPuzzle;
  sessionStatus?: SessionStatus | null;
}) {
  const storageKey = `${ATTEMPT_STORAGE_PREFIX}${puzzle.id}`;
  const isDemoPuzzle = puzzle.id.startsWith("demo-");
  const [attempt, setAttempt] = useState<StoredAttempt>(() => emptyAttempt(puzzle.id));
  const [message, setMessage] = useState(() => openingMessageForPuzzle(puzzle));
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [copied, setCopied] = useState(false);
  const [hasLoadedAttempt, setHasLoadedAttempt] = useState(false);
  const [tileOrder, setTileOrder] = useState(() => puzzle.tiles.map((tile) => tile.id));
  const [stats, setStats] = useState<PuzzleStats | null>(null);
  const [streak, setStreak] = useState<StreakSummary | null>(null);
  const [reportOpen, setReportOpen] = useState(false);
  const [reportReason, setReportReason] = useState<ReportReason>("OFFENSIVE");
  const [reportDetails, setReportDetails] = useState("");
  const [reportContact, setReportContact] = useState("");
  const [isReporting, setIsReporting] = useState(false);
  const [syncState, setSyncState] = useState<"idle" | "syncing" | "error">("idle");
  const [mode, setMode] = useState<GameMode>("standard");
  const [vibes, setVibes] = useState<VibeHint[] | null>(null);
  const [timerNow, setTimerNow] = useState<string | null>(null);
  const [resolvedSession, setResolvedSession] = useState<SessionStatus | null>(sessionStatus ?? null);

  // attemptRef mirrors the latest attempt so event handlers (storage/visibility)
  // can read current state without being re-bound on every change.
  const attemptRef = useRef(attempt);
  useEffect(() => {
    attemptRef.current = attempt;
  }, [attempt]);

  // syncSeq guards against out-of-order responses: only the newest sync applies.
  const syncSeq = useRef(0);

  // pendingGuessIdRef holds the client guess id for the in-flight submission. It
  // persists across network-failure retries so a lost response can't double-count
  // a mistake (the server dedupes by this id); it resets once a guess is settled
  // or the selection changes. submittingRef pauses sync while a guess is in flight.
  const pendingGuessIdRef = useRef<string | null>(null);
  const submittingRef = useRef(false);

  useEffect(() => {
    if (sessionStatus !== undefined) {
      setResolvedSession(sessionStatus);
    }
  }, [sessionStatus]);

  useEffect(() => {
    if (sessionStatus !== undefined) {
      return;
    }

    let cancelled = false;
    fetchSessionStatus()
      .then((status) => {
        if (!cancelled) {
          setResolvedSession(status);
        }
      })
      .catch(() => {
        // The game can still run; attempt sync/submit will surface API failures.
      });
    return () => {
      cancelled = true;
    };
  }, [sessionStatus]);

  useEffect(() => {
    const storage = safeStorage();
    if (!storage) {
      // No durable storage (private mode etc.) — play on with an in-memory board.
      setHasLoadedAttempt(true);
      return;
    }

    try {
      cleanupStoredAttempts(storage);
    } catch {
      // Cleanup is best-effort; never let it block loading the board.
    }

    let storedAttempt: string | null = null;
    try {
      storedAttempt = storage.getItem(storageKey);
    } catch {
      storedAttempt = null;
    }

    if (storedAttempt) {
      try {
        const parsed = JSON.parse(storedAttempt) as Partial<StoredAttempt>;
        if (parsed.puzzleId === puzzle.id) {
          setAttempt(normalizeStoredAttempt(puzzle.id, parsed));
        }
      } catch {
        try {
          storage.removeItem(storageKey);
        } catch {
          // Ignore — a removal failure is harmless.
        }
      }
    }

    setHasLoadedAttempt(true);
  }, [puzzle.id, storageKey]);

  // syncAttempt pulls the server-authoritative attempt and merges it in. The
  // server is the source of truth, so this reconciles whatever a stale tab or
  // localStorage holds. A failure surfaces a visible "Resync" affordance rather
  // than silently leaving stale state on screen.
  const syncAttempt = useCallback(async () => {
    // A guess submission already returns a fresh server snapshot, so don't race a
    // sync against it — the in-flight guess wins and the next focus/storage event
    // (or the monotonic merge) reconciles anything missed.
    if (submittingRef.current) {
      return;
    }
    const seq = ++syncSeq.current;
    setSyncState("syncing");

    try {
      const response = await apiFetch(`/api/attempts/${puzzle.id}`, {
        credentials: "include"
      });

      if (!response.ok) {
        throw new Error(`sync failed: ${response.status}`);
      }

      const serverAttempt = (await response.json()) as AttemptSnapshot;
      if (seq !== syncSeq.current) {
        return; // a newer sync superseded this one
      }

      setAttempt((current) => mergeServerAttempt(current, serverAttempt));
      setSyncState("idle");
    } catch {
      if (seq !== syncSeq.current) {
        return;
      }
      setSyncState("error");
    }
  }, [puzzle.id]);

  // Reconcile with the server once the local board has loaded.
  useEffect(() => {
    if (!hasLoadedAttempt) {
      return;
    }
    void syncAttempt();
  }, [hasLoadedAttempt, syncAttempt]);

  // Live cross-tab sync: a tab that solved the puzzle elsewhere updates without
  // a manual refresh. `storage` fires in *other* tabs when localStorage changes;
  // `visibilitychange` catches anything missed while this tab was backgrounded.
  useEffect(() => {
    if (!hasLoadedAttempt) {
      return;
    }

    function handleVisibility() {
      if (document.visibilityState === "visible") {
        void syncAttempt();
      }
    }

    function handleStorage(event: StorageEvent) {
      if (event.key !== storageKey || !event.newValue) {
        return;
      }
      try {
        const peer = JSON.parse(event.newValue) as StoredAttempt;
        // Only resync when another tab actually advanced the game — a new guess,
        // a win, or a loss — not on every tile toggle it writes to storage.
        const advanced =
          peer.guessCount > attemptRef.current.guessCount || peer.completed || peer.failed;
        if (advanced) {
          void syncAttempt();
        }
      } catch {
        // Ignore unparseable peer state; the next focus or refresh will resync.
      }
    }

    document.addEventListener("visibilitychange", handleVisibility);
    window.addEventListener("storage", handleStorage);
    return () => {
      document.removeEventListener("visibilitychange", handleVisibility);
      window.removeEventListener("storage", handleStorage);
    };
  }, [hasLoadedAttempt, storageKey, syncAttempt]);

  useEffect(() => {
    if (!hasLoadedAttempt) {
      return;
    }

    const storage = safeStorage();
    if (!storage) {
      return;
    }
    try {
      storage.setItem(storageKey, JSON.stringify(attempt));
    } catch {
      // Quota or a security exception — the in-memory board is still authoritative
      // for this tab, and the server holds the durable copy.
    }
  }, [attempt, hasLoadedAttempt, storageKey]);

  // Mode is a per-browser preference. Read it after mount (not in initial state)
  // so the server-rendered markup and first client paint agree; default standard.
  useEffect(() => {
    const saved = safeStorage()?.getItem(MODE_STORAGE_KEY);
    if (saved === "hard" || saved === "standard") {
      setMode(saved);
    }
  }, []);

  useEffect(() => {
    safeStorage()?.setItem(MODE_STORAGE_KEY, mode);
  }, [mode]);

  // Standard mode reveals one vibe (group name) at a time. Fetch just the names —
  // never the tile→group mapping — lazily, only when guided mode is actually used.
  useEffect(() => {
    if (mode !== "standard" || vibes !== null) {
      return;
    }
    let cancelled = false;
    fetchVibes(puzzle.id)
      .then((loaded) => {
        if (!cancelled) {
          setVibes(loaded);
        }
      })
      .catch(() => {
        // A hint fetch failure shouldn't break play; Standard just shows no banner.
      });
    return () => {
      cancelled = true;
    };
  }, [mode, vibes, puzzle.id]);

  const displayedGroups = useMemo(() => {
    const solvedGroupIds = new Set(attempt.solvedGroups.map((group) => group.id));
    const revealedOnly = attempt.revealedGroups.filter((group) => !solvedGroupIds.has(group.id));

    return [...attempt.solvedGroups, ...revealedOnly].sort(
      (left, right) => left.colorIndex - right.colorIndex
    );
  }, [attempt.revealedGroups, attempt.solvedGroups]);

  const displayedTileIds = useMemo(
    () => new Set(displayedGroups.flatMap((group) => group.tileIds)),
    [displayedGroups]
  );

  const selectedTileIds = new Set(attempt.selectedTileIds);
  const isComplete = attempt.completed || attempt.solvedGroups.length === puzzle.groupCount;
  const isOver = isComplete || attempt.failed;
  const elapsedFinishedAt = attempt.completedAt ?? timerNow ?? attempt.startedAt;

  useEffect(() => {
    setTimerNow(new Date().toISOString());
    if (isOver) {
      return;
    }

    const interval = window.setInterval(() => {
      setTimerNow(new Date().toISOString());
    }, 1000);
    return () => window.clearInterval(interval);
  }, [isOver]);

  // In Standard mode the target is the first vibe whose group is still unsolved;
  // solving any group advances it (each group owns a unique colorIndex). Null in
  // Hard mode, before the vibes load, or once the board is over.
  const solvedColorIndexes = useMemo(
    () => new Set(attempt.solvedGroups.map((group) => group.colorIndex)),
    [attempt.solvedGroups]
  );
  const currentVibe =
    mode === "standard" && vibes && !isOver
      ? vibes.find((vibe) => !solvedColorIndexes.has(vibe.colorIndex)) ?? null
      : null;

  useEffect(() => {
    if (!isOver) {
      return;
    }

    let cancelled = false;
    fetchPuzzleStats(puzzle.id)
      .then((loaded) => {
        if (!cancelled) {
          setStats(loaded);
        }
      })
      .catch(() => {
        // Stats are a nice-to-have; a failure should never disrupt the result screen.
      });

    return () => {
      cancelled = true;
    };
  }, [isOver, puzzle.id]);

  // Streaks apply to the daily puzzle only (community puzzles are dateless).
  // Re-fetch when the puzzle is completed so the count bumps immediately.
  useEffect(() => {
    if (!puzzle.publishDate) {
      return;
    }

    let cancelled = false;
    fetchStreak()
      .then((loaded) => {
        if (!cancelled) {
          setStreak(loaded);
        }
      })
      .catch(() => {
        // Streak is a nice-to-have; never block the board on it.
      });

    return () => {
      cancelled = true;
    };
  }, [puzzle.publishDate, isComplete]);

  const tilesById = useMemo(() => new Map(puzzle.tiles.map((tile) => [tile.id, tile])), [puzzle.tiles]);
  const remainingTiles = tileOrder
    .map((tileId) => tilesById.get(tileId))
    .filter((tile): tile is Tile => tile !== undefined)
    .filter((tile) => !displayedTileIds.has(tile.id));

  function toggleTile(tileId: string) {
    if (isOver || displayedTileIds.has(tileId)) {
      return;
    }

    // Changing the selection means the next submit is a different logical guess,
    // so drop any retained id from a prior failed attempt.
    pendingGuessIdRef.current = null;
    setCopied(false);
    setAttempt((current) => {
      const isSelected = current.selectedTileIds.includes(tileId);

      if (isSelected) {
        return {
          ...current,
          selectedTileIds: current.selectedTileIds.filter((selectedTileId) => selectedTileId !== tileId)
        };
      }

      if (current.selectedTileIds.length === 4) {
        return current;
      }

      return {
        ...current,
        selectedTileIds: [...current.selectedTileIds, tileId]
      };
    });
  }

  function shuffleRemaining() {
    setTileOrder((currentOrder) => {
      const unsolved = currentOrder.filter((tileId) => !displayedTileIds.has(tileId));
      const solved = currentOrder.filter((tileId) => displayedTileIds.has(tileId));
      const rotated = unsolved.length > 1 ? [...unsolved.slice(1), unsolved[0]] : unsolved;

      return [...solved, ...rotated];
    });
  }

  async function submitGuess() {
    if (attempt.selectedTileIds.length !== 4 || isSubmitting || isOver) {
      return;
    }

    setIsSubmitting(true);
    submittingRef.current = true;
    // Invalidate any sync already in flight so its (older) snapshot can't land
    // on top of this guess's result.
    syncSeq.current++;
    setCopied(false);

    // Reuse the id from a prior failed attempt at this same selection so a retry
    // after a lost response is deduped server-side rather than counted twice.
    const clientGuessId = pendingGuessIdRef.current ?? crypto.randomUUID();
    pendingGuessIdRef.current = clientGuessId;

    try {
      const response = await apiFetch("/api/guesses", {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify({
          puzzleId: puzzle.id,
          selectedTileIds: attempt.selectedTileIds,
          clientGuessId
        })
      });

      const result = (await response.json()) as GuessResponse;
      // We got a server response, so this guess id is settled — the next submit
      // is a new guess. (Only a network failure keeps the id, in the catch.)
      pendingGuessIdRef.current = null;

      if (!result.ok) {
        // 409 (attempt finished) or an already-locked group means this tab is
        // simply behind another tab. Reconcile silently instead of alarming the
        // player with an error they didn't cause.
        const behindServer =
          response.status === 409 || /already locked/i.test(result.error ?? "");
        if (behindServer) {
          void syncAttempt();
        } else {
          setMessage(result.error);
          toast.error(result.error);
        }
        return;
      }

      setAttempt((current) => {
        // The guess response carries the full server-authoritative history
        // (including this guess), so mergeServerAttempt sets guessHistory for us.
        const nextAttempt = mergeServerAttempt(
          {
            ...current,
            selectedTileIds: []
          },
          result.attempt
        );

        if (result.isCorrect) {
          setMessage(nextAttempt.completed ? "All vibes found. Suspiciously competent." : result.group.name);
          return nextAttempt;
        }

        if (nextAttempt.failed) {
          setMessage("Four misses. The grid wins today.");
        } else {
          setMessage(result.oneAway ? "So close — one away." : "Not that vibe.");
        }
        return nextAttempt;
      });
    } catch {
      // Network failure: we don't know if the server recorded the guess, so keep
      // pendingGuessIdRef — a retry of the same selection reuses the id and is
      // deduped rather than double-counted.
      setMessage("Could not submit. The grid is being dramatic.");
      toast.error("Could not submit that guess.");
    } finally {
      submittingRef.current = false;
      setIsSubmitting(false);
    }
  }

  // colorByTile maps every tile to its group's colour index. Once the puzzle is
  // over, displayedGroups covers all 16 tiles (solved on a win, revealed on a
  // loss), so the share grid can colour the full guess history.
  const colorByTile = useMemo(() => {
    const map: Record<string, number> = {};
    for (const group of displayedGroups) {
      for (const tileId of group.tileIds) {
        map[tileId] = group.colorIndex;
      }
    }
    return map;
  }, [displayedGroups]);

  const shareGrid = useMemo(
    () => (isOver ? buildShareGrid(attempt.guessHistory, colorByTile) : []),
    [isOver, attempt.guessHistory, colorByTile]
  );

  const fallbackShareUrl = useMemo(() => {
    return new URL(`/p/${puzzle.id}`, process.env.NEXT_PUBLIC_APP_URL ?? "http://localhost:3000").toString();
  }, [puzzle.id]);

  const currentShareUrl = useCallback(() => {
    if (typeof window === "undefined") {
      return fallbackShareUrl;
    }

    const url = new URL(window.location.href);
    url.hash = "";
    return url.toString();
  }, [fallbackShareUrl]);

  async function shareResult() {
    const shareText = buildShareText({
      puzzleNumber: puzzle.puzzleNumber,
      mistakes: attempt.mistakes,
      mistakesAllowed: puzzle.mistakesAllowed,
      solvedCount: attempt.solvedGroups.length,
      groupCount: puzzle.groupCount,
      startedAt: attempt.startedAt,
      finishedAt: attempt.completedAt,
      failed: attempt.failed,
      grid: shareGrid,
      shareUrl: currentShareUrl()
    });

    try {
      await navigator.clipboard.writeText(shareText);
      setCopied(true);
      toast.success("Copied result.");
    } catch {
      toast.error("Could not copy result.");
    }
  }

  async function submitReport(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isReporting) {
      return;
    }

    setIsReporting(true);
    try {
      await reportPuzzle({
        puzzleId: puzzle.id,
        reason: reportReason,
        details: reportDetails,
        contact: reportContact
      });
      setReportDetails("");
      setReportContact("");
      setReportReason("OFFENSIVE");
      setReportOpen(false);
      toast.success("Report sent. Thanks for the flag.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not send that report.");
    } finally {
      setIsReporting(false);
    }
  }

  return (
    <div className="mx-auto grid min-h-[calc(100vh-2.5rem)] max-w-6xl grid-rows-[auto_1fr] gap-5">
      <header className="flex flex-wrap items-center justify-between gap-4 border-b-4 border-ink pb-4">
        <div className="flex items-center gap-3">
          <Image src="/vibegrid-mark.svg" width={48} height={48} alt="" className="rounded" priority />
          <div>
            <h1 className="text-3xl font-black leading-none sm:text-4xl">VibeGrid</h1>
            <div className="mt-1 flex flex-wrap items-center gap-2">
              <p className="text-sm font-semibold text-neutral-600">
                {isDemoPuzzle ? "Demo room" : `#${puzzle.puzzleNumber}`}
                {!isDemoPuzzle && puzzle.publishDate ? ` / ${puzzle.publishDate}` : ""}
              </p>
              {streak && streak.currentStreak > 0 && (
                <span
                  title={`Longest streak: ${streak.longestStreak} · Solved: ${streak.totalCompleted}`}
                  className="inline-flex items-center gap-1 rounded border-2 border-ink bg-yolk px-2 py-0.5 text-xs font-black"
                >
                  🔥 {streak.currentStreak} day{streak.currentStreak === 1 ? "" : "s"}
                </span>
              )}
            </div>
          </div>
        </div>

        <nav className="flex items-center gap-2">
          <HowToPlay />
          <Link
            href="/demo"
            aria-label="Demo walkthrough"
            title="Demo walkthrough"
            className="inline-flex h-11 w-11 items-center justify-center rounded border-2 border-ink bg-white shadow-[0_4px_0_#171717]"
          >
            <Compass aria-hidden size={18} />
          </Link>
          <Link
            href="/create"
            aria-label="Make your own"
            title="Make your own"
            className="inline-flex h-11 w-11 items-center justify-center rounded border-2 border-ink bg-white shadow-[0_4px_0_#171717]"
          >
            <Sparkles aria-hidden size={18} />
          </Link>
          <Link
            href="/archive"
            aria-label="Archive"
            title="Archive"
            className="inline-flex h-11 w-11 items-center justify-center rounded border-2 border-ink bg-white shadow-[0_4px_0_#171717]"
          >
            <Archive aria-hidden size={18} />
          </Link>
          <button
            aria-label="Shuffle tiles"
            title="Shuffle tiles"
            className="inline-flex h-11 w-11 items-center justify-center rounded border-2 border-ink bg-white shadow-[0_4px_0_#171717] disabled:opacity-40"
            disabled={isOver || remainingTiles.length < 2}
            type="button"
            onClick={shuffleRemaining}
          >
            <Shuffle aria-hidden size={18} />
          </button>
        </nav>
      </header>

      <section className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="min-w-0">
          {currentVibe && (
            <div
              className={clsx(
                "mb-4 rounded border-2 border-ink p-4 shadow-[0_5px_0_#171717]",
                groupColors[currentVibe.colorIndex % groupColors.length]
              )}
            >
              <p className="text-xs font-black uppercase tracking-wide opacity-80">
                Match this vibe · {attempt.solvedGroups.length + 1} of {puzzle.groupCount}
              </p>
              <p className="mt-1 text-2xl font-black leading-tight">{currentVibe.name}</p>
              <p className="mt-1 text-sm font-bold opacity-80">Pick the 4 tiles that fit it.</p>
            </div>
          )}
          <div className="grid gap-3">
            {displayedGroups.map((group) => {
              const isSolved = attempt.solvedGroups.some((solvedGroup) => solvedGroup.id === group.id);

              return (
                <section
                  key={group.id}
                  className={clsx(
                    "rounded border-2 border-ink p-4 shadow-[0_5px_0_#171717]",
                    groupColors[group.colorIndex % groupColors.length]
                  )}
                >
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <h2 className="text-xl font-black">{group.name}</h2>
                      <p className="mt-1 text-sm font-semibold opacity-80">{group.explanation}</p>
                    </div>
                    <p className="text-sm font-black">
                      {isSolved ? "Locked" : "Revealed"}
                    </p>
                  </div>
                  <div className="mt-4 grid grid-cols-4 gap-1.5 sm:gap-2">
                    {group.tiles.map((tile) => (
                      <div
                        key={tile.id}
                        className="flex min-h-14 items-center justify-center rounded border border-ink bg-white/80 px-1 text-center text-[0.7rem] font-black leading-tight sm:px-2 sm:text-sm"
                      >
                        {tile.text}
                      </div>
                    ))}
                  </div>
                </section>
              );
            })}
          </div>

          <div className="mt-4 grid grid-cols-4 gap-1.5 sm:gap-3">
            {remainingTiles.map((tile) => {
              const isSelected = selectedTileIds.has(tile.id);

              return (
                <button
                  key={tile.id}
                  className={clsx(
                    "flex aspect-square min-h-16 items-center justify-center rounded border-2 border-ink px-1 text-center text-[0.7rem] font-black shadow-tile transition [touch-action:manipulation] sm:aspect-[1.45] sm:min-h-20 sm:px-2 sm:text-lg",
                    isSelected
                      ? "translate-y-1 bg-ink text-white shadow-[0_4px_0_rgba(23,23,23,0.18)]"
                      : "bg-white hover:-translate-y-0.5 hover:bg-yolk"
                  )}
                  type="button"
                  aria-pressed={isSelected}
                  onClick={() => toggleTile(tile.id)}
                >
                  <span className="break-words leading-tight">{tile.text}</span>
                </button>
              );
            })}
          </div>
        </div>

        <aside className="flex flex-col justify-between gap-4 rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
          <div>
            {!isOver && (
              <div className="mb-4">
                <div className="flex gap-1 rounded border-2 border-ink p-1">
                  {(["standard", "hard"] as const).map((option) => (
                    <button
                      key={option}
                      type="button"
                      onClick={() => setMode(option)}
                      aria-pressed={mode === option}
                      className={clsx(
                        "h-9 flex-1 rounded text-sm font-black capitalize",
                        mode === option ? "bg-ink text-white" : "bg-white text-ink"
                      )}
                    >
                      {option}
                    </button>
                  ))}
                </div>
                <p className="mt-1.5 text-xs font-semibold text-neutral-600">
                  {mode === "standard"
                    ? "Guided: we name one vibe at a time — match its 4 tiles."
                    : "Hard: all four vibes hidden. Deduce them yourself."}
                </p>
              </div>
            )}

            <div className="grid grid-cols-2 gap-3">
              <div className="rounded border border-neutral-200 p-3">
                <p className="text-xs font-black text-neutral-500">Selected</p>
                <p className="mt-1 text-2xl font-black">{attempt.selectedTileIds.length}/4</p>
              </div>
              <div className="rounded border border-neutral-200 p-3">
                <p className="text-xs font-black text-neutral-500">Mistakes</p>
                <p className="mt-1 text-2xl font-black">
                  {attempt.mistakes}/{puzzle.mistakesAllowed}
                </p>
              </div>
            </div>

            <div className="mt-4 grid grid-cols-4 gap-2" aria-label="Mistake counter">
              {Array.from({ length: puzzle.mistakesAllowed }).map((_, index) => (
                <div
                  key={index}
                  className={clsx(
                    "h-3 rounded border border-ink",
                    index < attempt.mistakes ? "bg-tomato" : "bg-neutral-100"
                  )}
                />
              ))}
            </div>

            <p className="mt-5 min-h-12 text-lg font-black leading-snug">{message}</p>

            {syncState === "error" && (
              <div className="mt-3 flex items-center justify-between gap-2 rounded border-2 border-tomato bg-tomato/10 px-3 py-2 text-sm font-bold">
                <span>Couldn&apos;t sync — showing saved progress.</span>
                <button
                  type="button"
                  onClick={() => void syncAttempt()}
                  className="inline-flex h-8 shrink-0 items-center justify-center rounded border-2 border-ink bg-white px-2 text-xs font-black shadow-[0_3px_0_#171717]"
                >
                  Resync
                </button>
              </div>
            )}

            <div className="mt-4 border-t border-neutral-200 pt-4 text-sm font-semibold text-neutral-600">
              <p>Elapsed {formatElapsedTime(attempt.startedAt, elapsedFinishedAt)}</p>
              <p className="mt-1">Guesses {attempt.guessCount}</p>
              <p className="mt-1">
                {resolvedSession?.guest.label ?? "Guest session"}. Saved in this browser.
              </p>
              {resolvedSession?.admin.authenticated && (
                <p className="mt-1 text-plum">Editor session active.</p>
              )}
            </div>

            {isOver && stats && stats.players >= MIN_STATS_PLAYERS && (
              <div className="mt-4 rounded border-2 border-ink bg-plum/10 p-3">
                <p className="text-xs font-black text-plum">How others did</p>
                <div className="mt-2 grid grid-cols-2 gap-2 text-sm font-semibold">
                  <p>{Math.round(stats.solveRate * 100)}% solved</p>
                  <p>{stats.players} {stats.players === 1 ? "player" : "players"}</p>
                  <p>~{stats.medianMistakes.toFixed(0)} mistakes</p>
                  {stats.medianSolveSeconds !== undefined && (
                    <p>~{formatSeconds(stats.medianSolveSeconds)} median</p>
                  )}
                </div>
              </div>
            )}

            {isOver && attempt.guessHistory.length > 0 && (
              <div className="mt-4">
                <p className="text-xs font-black text-neutral-500">Your grid</p>
                <div className="mt-2 grid gap-1">
                  {attempt.guessHistory.map((row, rowIndex) => (
                    <div key={rowIndex} className="flex gap-1">
                      {row.map((tileId, tileIndex) => (
                        <span
                          key={`${rowIndex}-${tileIndex}`}
                          className={clsx(
                            "h-5 w-5 rounded-sm border border-ink",
                            squareColors[(colorByTile[tileId] ?? 0) % squareColors.length]
                          )}
                        />
                      ))}
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>

          <div className="grid gap-2">
            {!isOver ? (
              <button
                className="inline-flex h-12 items-center justify-center gap-2 rounded border-2 border-ink bg-mint px-4 font-black shadow-[0_5px_0_#171717] disabled:translate-y-1 disabled:bg-neutral-200 disabled:text-neutral-500 disabled:shadow-[0_2px_0_#171717]"
                type="button"
                disabled={attempt.selectedTileIds.length !== 4 || isSubmitting}
                onClick={submitGuess}
              >
                <Send aria-hidden size={18} />
                Submit
              </button>
            ) : (
              <button
                className="inline-flex h-12 items-center justify-center gap-2 rounded border-2 border-ink bg-yolk px-4 font-black shadow-[0_5px_0_#171717]"
                type="button"
                onClick={shareResult}
              >
                <Share2 aria-hidden size={18} />
                {copied ? "Copied" : "Share"}
              </button>
            )}

            <Link
              href="/create"
              className="inline-flex h-11 items-center justify-center gap-2 rounded border border-neutral-300 bg-white px-4 text-sm font-black text-neutral-700"
            >
              <Sparkles aria-hidden size={16} />
              Make your own
            </Link>
            <button
              type="button"
              onClick={() => setReportOpen((open) => !open)}
              className="inline-flex h-10 items-center justify-center gap-2 rounded border border-neutral-300 bg-white px-4 text-sm font-black text-neutral-700"
            >
              <Flag aria-hidden size={16} />
              Report a problem
            </button>
            {reportOpen && (
              <form className="grid gap-2 border-t border-neutral-200 pt-3" onSubmit={submitReport}>
                <p className="text-xs font-semibold leading-snug text-neutral-600">
                  No login needed. We review the grid id, your reason, and any note you add.
                </p>
                <label className="grid gap-1 text-xs font-black text-neutral-600">
                  Reason
                  <select
                    value={reportReason}
                    onChange={(event) => setReportReason(event.target.value as ReportReason)}
                    className="h-10 rounded border border-neutral-300 bg-white px-2 text-sm font-semibold text-ink"
                  >
                    {reportReasons.map((reason) => (
                      <option key={reason.value} value={reason.value}>
                        {reason.label}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="grid gap-1 text-xs font-black text-neutral-600">
                  What happened?
                  <textarea
                    value={reportDetails}
                    onChange={(event) => setReportDetails(event.target.value)}
                    maxLength={1000}
                    rows={3}
                    placeholder="Tell us what feels unsafe, copied, broken, or unfair."
                    className="resize-none rounded border border-neutral-300 px-2 py-2 text-sm font-semibold text-ink"
                  />
                </label>
                <label className="grid gap-1 text-xs font-black text-neutral-600">
                  Email for reply (optional)
                  <input
                    value={reportContact}
                    onChange={(event) => setReportContact(event.target.value)}
                    maxLength={200}
                    placeholder="Only if you want a reply"
                    className="h-10 rounded border border-neutral-300 px-2 text-sm font-semibold text-ink"
                  />
                </label>
                <div className="grid grid-cols-2 gap-2">
                  <button
                    type="button"
                    onClick={() => setReportOpen(false)}
                    className="inline-flex h-10 items-center justify-center gap-2 rounded border border-neutral-300 bg-white px-3 text-sm font-black text-neutral-700"
                  >
                    <X aria-hidden size={15} />
                    Cancel
                  </button>
                  <button
                    type="submit"
                    disabled={isReporting}
                    className="inline-flex h-10 items-center justify-center gap-2 rounded border-2 border-ink bg-yolk px-3 text-sm font-black shadow-[0_3px_0_#171717] disabled:opacity-50"
                  >
                    <Send aria-hidden size={15} />
                    {isReporting ? "Sending" : "Send"}
                  </button>
                </div>
              </form>
            )}
          </div>
        </aside>
      </section>
    </div>
  );
}

// mergeServerAttempt reconciles local and server state forward-only. The server
// is normally the authoritative superset, so in steady play this just adopts it.
// But the merge never moves the board *backward*: if the server ever reports
// less than we already have — a brand-new/empty session after a cleared or
// expired cookie, or an in-memory store that reset on a redeploy — we keep the
// player's progress instead of wiping a solved board to blank. Solved/revealed
// groups union, terminal flags are sticky, and counts only climb.
function mergeServerAttempt(current: StoredAttempt, serverAttempt: AttemptSnapshot): StoredAttempt {
  const serverHistory = serverAttempt.guessHistory ?? [];
  const solvedGroups = mergeGroupsById(current.solvedGroups, serverAttempt.solvedGroups);
  const revealedGroups = mergeGroupsById(current.revealedGroups, serverAttempt.revealedGroups);
  const failed = current.failed || serverAttempt.failed;
  const completed = current.completed || serverAttempt.completed;

  // Once the server has its own real attempt it owns startedAt; before then we
  // keep the local start so a fresh/empty session can't reset the elapsed timer.
  const serverHasProgress =
    serverAttempt.guessCount > 0 || serverAttempt.solvedGroups.length > 0 || serverAttempt.failed;

  const displayedTileIds = new Set(
    [...solvedGroups, ...revealedGroups].flatMap((group) => group.tileIds)
  );

  return {
    ...current,
    puzzleId: serverAttempt.puzzleId,
    selectedTileIds:
      completed || failed
        ? []
        : current.selectedTileIds.filter((tileId) => !displayedTileIds.has(tileId)),
    solvedGroups,
    revealedGroups,
    mistakes: Math.max(current.mistakes, serverAttempt.mistakes),
    guessCount: Math.max(current.guessCount, serverAttempt.guessCount),
    startedAt: serverHasProgress ? serverAttempt.startedAt : current.startedAt,
    completedAt: current.completedAt ?? serverAttempt.completedAt,
    failed,
    completed,
    // Server owns guess history (so a tab that never saw the guesses rebuilds the
    // share grid), but only adopt it when it is at least as complete as ours.
    guessHistory: serverHistory.length >= current.guessHistory.length ? serverHistory : current.guessHistory
  };
}

// mergeGroupsById unions two group lists by id (the server copy wins for the
// same id, being the fresher content) and sorts by colour for stable display.
function mergeGroupsById(local: SolvedGroup[], server: SolvedGroup[]): SolvedGroup[] {
  const byId = new Map<string, SolvedGroup>();
  for (const group of local) {
    byId.set(group.id, group);
  }
  for (const group of server) {
    byId.set(group.id, group);
  }
  return [...byId.values()].sort((left, right) => left.colorIndex - right.colorIndex);
}
