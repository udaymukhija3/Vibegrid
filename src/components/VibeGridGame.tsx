"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import Image from "next/image";
import Link from "next/link";
import clsx from "clsx";
import { Archive, Flag, Flame, Send, Share2, Shuffle, Sparkles, X } from "lucide-react";
import { toast } from "sonner";
import {
  ATTEMPT_STORAGE_PREFIX,
  buildShareGrid,
  buildShareText,
  cleanupStoredAttempts,
  formatElapsedTime,
  formatSeconds,
  MIN_STATS_PLAYERS,
  normalizeGameMode,
  type GameMode
} from "@/lib/game";
import {
  fetchEasyHint,
  fetchPuzzleStats,
  fetchSessionStatus,
  fetchStreak,
  fetchVibes,
  reportPuzzle,
  type SessionStatus,
  type PuzzleStats,
  type StreakSummary
} from "@/lib/api";
import { TurnstileWidget } from "@/components/TurnstileWidget";
import { apiFetch } from "@/lib/http";
import { HowToPlay } from "@/components/HowToPlay";
import type {
  AttemptSnapshot,
  EasyHintResponse,
  GuessResponse,
  PublicPuzzle,
  SolvedGroup,
  Tile,
  VibeHint
} from "@/types/puzzle";

type StoredAttempt = {
  puzzleId: string;
  mode?: GameMode;
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
  const storedMode =
    parsed.mode === "easy" || parsed.mode === "medium" || parsed.mode === "hard"
      ? parsed.mode
      : undefined;
  return {
    ...emptyAttempt(puzzleId),
    ...parsed,
    puzzleId,
    mode: storedMode,
    selectedTileIds: asArray<string>(parsed.selectedTileIds),
    solvedGroups: asArray<SolvedGroup>(parsed.solvedGroups),
    revealedGroups: asArray<SolvedGroup>(parsed.revealedGroups),
    guessHistory: asArray<string[]>(parsed.guessHistory),
    mistakes: asCount(parsed.mistakes),
    guessCount: asCount(parsed.guessCount)
  };
}

const groupColors = [
  "border-mint/[.70] bg-mint/[.35] text-ink",
  "border-yolk/[.80] bg-yolk/[.35] text-ink",
  "border-tomato/[.75] bg-tomato/[.25] text-ink",
  "border-plum/[.55] bg-plum/[.15] text-ink"
];

// Background-only palette (matching groupColors) for the share-grid squares.
const squareColors = ["bg-mint", "bg-yolk", "bg-tomato", "bg-plum"];

const hintBorderColors = ["border-mint", "border-yolk", "border-tomato", "border-plum"];

const modeOptions: Array<{ value: GameMode; label: string }> = [
  { value: "easy", label: "Easy" },
  { value: "medium", label: "Medium" },
  { value: "hard", label: "Hard" }
];

const modeDescriptions: Record<GameMode, string> = {
  easy: "Guided plus selected tiles and a hint after two guesses.",
  medium: "Guided: we name one vibe at a time.",
  hard: "All four vibes hidden."
};

const easyThoughtStarters: Record<string, string> = {
  "Sunday reset": "Picture the little rituals that make a week feel possible again.",
  "Sunday scaries": "Think about the mood that creeps in when the weekend starts closing.",
  "Lazy recovery": "Look for the low-energy comforts of doing almost nothing.",
  "Productivity cosplay": "Find the tiles that look productive from far away.",
  "Deep work cosplay": "Picture someone building a fortress around focus.",
  "First date": "Think of two people trying to make awkwardness look effortless.",
  "Remote meeting": "Listen for the phrases and moments that belong inside a video call.",
  "Friends catching up": "Look for the rhythm of a conversation that keeps unfolding.",
  "The planner": "Find the person quietly keeping everyone organized.",
  "The ghost": "Think of someone present in the chat but barely there.",
  "The oversharer": "Picture a message that arrives with every possible detail attached.",
  "The chaos": "Look for the friend who turns the chat sideways.",
  "Gate gremlin": "Picture the traveler hovering before it is actually time to board.",
  "Duty-free daze": "Think of airport shopping with no real plan.",
  "Delay despair": "Look for the small signs that travel momentum has stalled.",
  "Smug frequent flyer": "Find the moves of someone who has done this too many times.",
  "Replaying it": "Picture the late-night loop of one awkward moment after another.",
  "Grand 2am plans": "Think of huge plans made when tomorrow still feels theoretical.",
  "Existential dread": "Look for thoughts that make the room feel much bigger.",
  "Snack logistics": "Find the practical food problem your brain can still solve.",
  "Looking busy": "Picture work theatre: plenty of motion, not much progress.",
  "Meeting bingo": "Listen for office phrases that sound useful but say very little.",
  "Kitchen politics": "Think of tiny shared-space tensions around snacks and coffee.",
  "Friday wind-down": "Look for the slow fade before the week is officially over.",
  "We're fine": "Find messages that feel genuinely relaxed.",
  "We are not fine": "Think of texts where punctuation is doing the real talking.",
  "Overthinking it": "Picture a reply being drafted, deleted, and drafted again.",
  "Unhinged friend": "Look for chaotic friendship energy with no context needed.",
  "New year hopeful": "Think of fresh motivation before reality has weighed in.",
  "The regular": "Find the signs of someone who practically owns the routine.",
  "Mirror crew": "Picture a workout where the phone gets as much attention as the reps.",
  "Cardio escapist": "Look for movement that is really a way to drift off mentally.",
  "Kitchen dweller": "Picture the guest who somehow never leaves the food table.",
  "The connector": "Find the person turning strangers into introductions.",
  "Early ghost": "Think of leaving so cleanly people notice only later.",
  "Last to leave": "Look for the end-of-night energy after everyone else has gone.",
  "Cart guilt": "Picture the moment between wanting the thing and questioning the thing.",
  "Sale brain": "Think of a discount making all judgment temporarily vanish.",
  "Delivery watch": "Look for the restless wait after the order is already on its way.",
  "Return spiral": "Picture the small admin loop after buying the wrong thing.",
  "Open bar arc": "Think of the predictable timeline once drinks are easy to get.",
  "Dance floor": "Find the moment when the music finally wins.",
  "Small talk loop": "Listen for polite questions that keep circling the same ground.",
  "Logistics nerd": "Picture the person who read every practical detail in advance.",
  "Productive avoidance": "Look for chores that appear when the real task is waiting.",
  "The chair": "Think of the in-between place where clothes wait for a verdict.",
  "Doom drawer": "Picture the hiding place for small things with no better home.",
  "Adulting wins": "Find the tiny responsible acts that feel weirdly triumphant."
};

function easyThoughtStarter(name: string) {
  return easyThoughtStarters[name] ?? `Picture the situation or mood behind "${name}", then look for four tiles that share it.`;
}

const EASY_HINT_GUESSES = 2;
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
  const [reportTurnstileToken, setReportTurnstileToken] = useState("");
  const [reportTurnstileReset, setReportTurnstileReset] = useState(0);
  const [syncState, setSyncState] = useState<"idle" | "syncing" | "error">("idle");
  const [mode, setMode] = useState<GameMode>("medium");
  const [vibes, setVibes] = useState<VibeHint[] | null>(null);
  const [easyHint, setEasyHint] = useState<EasyHintResponse | null>(null);
  const [easyHintStatus, setEasyHintStatus] = useState<"idle" | "loading" | "error">("idle");
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
  // so the server-rendered markup and first client paint agree; default medium.
  useEffect(() => {
    setMode(normalizeGameMode(safeStorage()?.getItem(MODE_STORAGE_KEY)));
  }, []);

  useEffect(() => {
    safeStorage()?.setItem(MODE_STORAGE_KEY, mode);
  }, [mode]);

  // Once the server has created an attempt, its persisted mode wins over the
  // browser preference. This also reconciles another tab that submitted first.
  useEffect(() => {
    if (attempt.mode) {
      setMode(attempt.mode);
    }
  }, [attempt.mode]);

  // Easy/Medium reveal one vibe (group name) at a time. Fetch just the names —
  // never the tile→group mapping — lazily, only when guided play is used.
  useEffect(() => {
    if (mode === "hard" || vibes !== null) {
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
        // A vibe-name fetch failure shouldn't break guided play; it just shows no banner.
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
    if (mode !== "easy" || isOver || attempt.guessCount < EASY_HINT_GUESSES) {
      setEasyHint(null);
      setEasyHintStatus("idle");
      return;
    }

    let cancelled = false;
    setEasyHintStatus("loading");
    fetchEasyHint(puzzle.id)
      .then((loaded) => {
        if (!cancelled) {
          setEasyHint(loaded);
          setEasyHintStatus("idle");
        }
      })
      .catch(() => {
        // Easy can still play as guided mode if the extra hint read fails.
        if (!cancelled) {
          setEasyHint(null);
          setEasyHintStatus("error");
        }
      });

    return () => {
      cancelled = true;
    };
  }, [attempt.guessCount, attempt.solvedGroups, isOver, mode, puzzle.id]);

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

  // In Easy/Medium mode the target is the first vibe whose group is still unsolved;
  // solving any group advances it (each group owns a unique colorIndex). Null in
  // Hard mode, before the vibes load, or once the board is over.
  const solvedColorIndexes = useMemo(
    () => new Set(attempt.solvedGroups.map((group) => group.colorIndex)),
    [attempt.solvedGroups]
  );
  const currentVibe =
    mode !== "hard" && vibes && !isOver
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
  const selectedTiles = attempt.selectedTileIds
    .map((tileId) => tilesById.get(tileId))
    .filter((tile): tile is Tile => tile !== undefined);
  const guessesUntilEasyHint = Math.max(0, EASY_HINT_GUESSES - attempt.guessCount);
  const unlockedEasyHint = mode === "easy" && easyHint?.available ? easyHint.hint ?? null : null;
  const modeLocked = attempt.guessCount > 0 && attempt.mode !== undefined;

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
          clientGuessId,
          mode
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
      mode,
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

    if (!reportTurnstileToken) {
      toast.error("Complete the bot check before sending.");
      return;
    }
    setIsReporting(true);
    try {
      await reportPuzzle({
        puzzleId: puzzle.id,
        reason: reportReason,
        details: reportDetails,
        contact: reportContact
      }, reportTurnstileToken);
      setReportDetails("");
      setReportContact("");
      setReportReason("OFFENSIVE");
      setReportOpen(false);
      toast.success("Report sent. Thanks for the flag.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not send that report.");
    } finally {
      setIsReporting(false);
      setReportTurnstileToken("");
      setReportTurnstileReset((value) => value + 1);
    }
  }

  const puzzleLabel = isDemoPuzzle ? "Demo room" : `VibeGrid #${puzzle.puzzleNumber}`;
  // The mobile spine shows this directly under the wordmark, where the "VibeGrid"
  // prefix is redundant and long enough to truncate to "VibeGrid ..." at 375px.
  const shortPuzzleLabel = isDemoPuzzle ? "Demo room" : `#${puzzle.puzzleNumber}`;
  const puzzleDate = !isDemoPuzzle && puzzle.publishDate ? puzzle.publishDate : null;
  const solvedCount = attempt.solvedGroups.length;
  const progressLabel = `${solvedCount}/${puzzle.groupCount}`;

  return (
    <div className="vg-desk">
      {/* Below lg the spine is a slim top bar: stacking its two rows costs ~120px of
          board space on a phone, which pushes the tile grid under the fold. */}
      <aside className="vg-spine flex flex-row items-center justify-between gap-3 p-2.5 lg:flex-col lg:items-stretch lg:gap-4 lg:p-3 lg:sticky lg:top-5 lg:h-[calc(100vh-2.5rem)]">
        <div className="flex min-w-0 items-center justify-between gap-3 lg:grid lg:justify-items-center">
          <Link
            href="/"
            aria-label="Play today's grid"
            className="flex min-w-0 items-center gap-2.5 rounded-lg focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-card lg:grid lg:justify-items-center lg:gap-2"
          >
            <Image
              src="/vibegrid-mark.svg"
              width={48}
              height={48}
              alt=""
              className="h-9 w-9 rounded-lg bg-card lg:h-12 lg:w-12"
              priority
            />
            <div className="min-w-0 lg:text-center">
              <h1 className="text-lg font-extrabold leading-none lg:text-base">VibeGrid</h1>
              <p className="mt-0.5 truncate text-xs font-semibold text-card/[.65] lg:hidden">
                {shortPuzzleLabel}
              </p>
            </div>
          </Link>

          <div className="hidden rounded-lg border border-card/[.15] bg-card/10 px-2 py-3 text-center lg:block">
            <p className="text-[0.68rem] font-semibold text-card/[.65]">Solved</p>
            <p className="mt-1 text-xl font-extrabold">{progressLabel}</p>
          </div>
        </div>

        <nav className="flex shrink-0 items-center gap-1.5 lg:grid lg:justify-items-center lg:gap-2" aria-label="Primary navigation">
          <HowToPlay />
          <Link href="/archive" aria-label="Archive" title="Archive" className="vg-icon-button">
            <Archive aria-hidden size={18} />
          </Link>
          <Link href="/create" aria-label="Make your own" title="Make your own" className="vg-icon-button">
            <Sparkles aria-hidden size={18} />
          </Link>
        </nav>
      </aside>

      <main className="vg-board-sheet min-w-0">
        <div className="flex flex-wrap items-start justify-between gap-3 border-b border-line pb-3 sm:pb-4">
          <div>
            {/* The spine already carries the puzzle label below lg. */}
            <p className="vg-kicker hidden lg:block">{puzzleLabel}</p>
            <h2 className="text-xl font-extrabold leading-tight sm:text-5xl lg:mt-1">
              {isOver ? "Result grid" : "Find the hidden sets"}
            </h2>
          </div>
          <div className="grid justify-items-start gap-1 text-sm font-semibold text-neutral-600 sm:justify-items-end">
            {puzzleDate && <p>{puzzleDate}</p>}
            {streak && streak.currentStreak > 0 && (
              <span
                title={`Longest streak: ${streak.longestStreak} · Solved: ${streak.totalCompleted}`}
                className="inline-flex items-center gap-1 rounded-lg border border-yolk/80 bg-yolk/30 px-2 py-1 text-xs text-ink"
              >
                <Flame aria-hidden size={14} />
                {streak.currentStreak} day{streak.currentStreak === 1 ? "" : "s"}
              </span>
            )}
          </div>
        </div>

        {!isOver && (
          <section className="mt-4 lg:hidden" aria-labelledby="mobile-mode-label">
            <p id="mobile-mode-label" className="text-sm font-semibold text-neutral-500">
              Mode
            </p>
            <div className="vg-mode-track mt-2">
              {modeOptions.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => setMode(option.value)}
                  disabled={modeLocked || isSubmitting}
                  aria-pressed={mode === option.value}
                  title={modeLocked ? "Mode is locked for this attempt" : undefined}
                  className={clsx(
                    "vg-mode-tab disabled:cursor-not-allowed disabled:opacity-70",
                    mode === option.value ? "bg-card text-ink" : "bg-ink text-card/75 hover:bg-card/[.12]"
                  )}
                >
                  {option.label}
                </button>
              ))}
            </div>
            <p className="mt-2 text-xs font-medium leading-snug text-neutral-600">
              {modeDescriptions[mode]}
            </p>
          </section>
        )}

        {!isOver && (
          <div
            className={clsx(
              "mt-4 rounded-lg border p-4 shadow-tile",
              currentVibe
                ? groupColors[currentVibe.colorIndex % groupColors.length]
                : "border-line bg-white/[.72] text-ink"
            )}
          >
            <p className="text-xs font-semibold opacity-75">
              {currentVibe ? `Vibe ${solvedCount + 1} of ${puzzle.groupCount}` : "Hard mode"}
            </p>
            <p className="mt-1 text-2xl font-extrabold leading-tight">
              {currentVibe?.name ?? "No named clues"}
            </p>
            <p className="mt-1 text-sm font-medium opacity-80">
              {currentVibe && mode === "easy"
                ? easyThoughtStarter(currentVibe.name)
                : currentVibe
                  ? "Pick the 4 tiles that fit it."
                  : "Solve from the board alone."}
            </p>
          </div>
        )}

        <div className="mt-4 rounded-lg border border-line bg-white/[.65] p-2 shadow-tile sm:p-3">
          <div className="grid gap-3">
            {displayedGroups.map((group) => {
              const isSolved = attempt.solvedGroups.some((solvedGroup) => solvedGroup.id === group.id);

              return (
                <section
                  key={group.id}
                  className={clsx(
                    "rounded-lg border p-3 shadow-tile sm:p-4",
                    groupColors[group.colorIndex % groupColors.length]
                  )}
                >
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <h3 className="text-xl font-extrabold">{group.name}</h3>
                      <p className="mt-1 text-sm font-medium opacity-80">{group.explanation}</p>
                    </div>
                    <p className="rounded-lg border border-white/60 bg-white/[.45] px-2 py-1 text-xs font-semibold">
                      {isSolved ? "Locked" : "Revealed"}
                    </p>
                  </div>
                  <div className="mt-4 grid grid-cols-4 gap-1.5 sm:gap-2">
                    {group.tiles.map((tile) => (
                      <div
                        key={tile.id}
                        className="flex min-h-14 items-center justify-center rounded-lg border border-white/70 bg-card/[.82] px-1 text-center text-[0.7rem] font-semibold leading-tight sm:px-2 sm:text-sm"
                      >
                        {tile.text}
                      </div>
                    ))}
                  </div>
                </section>
              );
            })}
          </div>

          <div className="mt-3 grid grid-cols-4 gap-1.5 sm:gap-3">
            {remainingTiles.map((tile) => {
              const isSelected = selectedTileIds.has(tile.id);

              return (
                <button
                  key={tile.id}
                  className={clsx(
                    "flex aspect-square min-h-16 items-center justify-center rounded-lg border border-line px-1.5 text-center text-[0.8rem] font-semibold shadow-tile transition [touch-action:manipulation] sm:aspect-[1.45] sm:min-h-20 sm:px-2 sm:text-lg",
                    isSelected
                      ? "translate-y-0.5 border-ink bg-ink text-card shadow-none ring-2 ring-pool/70"
                      : "bg-card hover:-translate-y-0.5 hover:border-ink hover:bg-yolk/25 hover:shadow-lift"
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

          {/* Pinned to the bottom of the viewport on phones. Unpinned, Submit sits
              below four rows of tiles, so every guess costs a scroll down to submit
              and a scroll back up to pick. It settles into place at the board's end. */}
          {!isOver && (
            <section
              className="sticky bottom-2 z-20 mt-3 grid gap-1.5 rounded-lg border border-line bg-card/95 p-2.5 shadow-lift backdrop-blur lg:hidden"
              aria-label="Guess controls"
            >
              <div className="flex items-center justify-between gap-3">
                <p className="whitespace-nowrap text-sm font-extrabold">
                  Selected {attempt.selectedTileIds.length}/4
                  <span className="ml-2 text-xs font-semibold text-neutral-500">
                    · {attempt.mistakes}/{puzzle.mistakesAllowed} misses
                  </span>
                </p>
                <button
                  aria-label="Shuffle tiles"
                  title="Shuffle tiles"
                  className="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 rounded-lg border border-line bg-white px-3 text-xs font-semibold shadow-tile disabled:opacity-50"
                  disabled={remainingTiles.length < 2}
                  type="button"
                  onClick={shuffleRemaining}
                >
                  <Shuffle aria-hidden size={15} />
                  Shuffle
                </button>
              </div>

              <div className="grid min-h-10 grid-cols-4 gap-1.5" aria-label="Selected tiles">
                {Array.from({ length: 4 }).map((_, index) => {
                  const tile = selectedTiles[index];
                  return (
                    <div
                      key={tile?.id ?? `mobile-empty-${index}`}
                      className={clsx(
                        "flex min-h-10 items-center justify-center rounded-lg border px-1 text-center text-[0.65rem] font-semibold leading-tight",
                        tile
                          ? "border-ink bg-ink text-card"
                          : "border-neutral-200 bg-neutral-50 text-neutral-400"
                      )}
                    >
                      {tile?.text ?? "Pick tile"}
                    </div>
                  );
                })}
              </div>

              <button
                className="vg-button-primary h-11 w-full"
                type="button"
                disabled={attempt.selectedTileIds.length !== 4 || isSubmitting}
                onClick={submitGuess}
              >
                <Send aria-hidden size={18} />
                {isSubmitting ? "Checking…" : "Submit guess"}
              </button>
              <p
                className={clsx("text-sm font-semibold leading-snug", message ? "min-h-5" : "sr-only")}
                aria-live="polite"
              >
                {message}
              </p>
            </section>
          )}
        </div>
      </main>

      <aside className="vg-control-rail flex flex-col justify-between gap-4 lg:sticky lg:top-5 lg:max-h-[calc(100vh-2.5rem)] lg:overflow-auto">
        <div>
          {!isOver && (
            <div className="hidden lg:block">
              <p className="text-sm font-semibold text-neutral-500">Mode</p>
              <div className="vg-mode-track mt-2">
                {modeOptions.map((option) => (
                  <button
                    key={option.value}
                  type="button"
                  onClick={() => setMode(option.value)}
                  disabled={modeLocked || isSubmitting}
                  aria-pressed={mode === option.value}
                  title={modeLocked ? "Mode is locked for this attempt" : undefined}
                  className={clsx(
                      "vg-mode-tab disabled:cursor-not-allowed disabled:opacity-70",
                      mode === option.value ? "bg-card text-ink" : "bg-ink text-card/75 hover:bg-card/[.12]"
                    )}
                  >
                    {option.label}
                  </button>
                ))}
              </div>
              <p className="mt-2 min-h-8 text-xs font-medium leading-snug text-neutral-600">
                {modeDescriptions[mode]}
              </p>
            </div>
          )}

          {!isOver && (
            <div className="vg-rule mt-4 hidden pt-4 lg:block">
              <p className="text-sm font-semibold text-neutral-500">Selection tray</p>
              <div className="mt-2 grid min-h-24 grid-cols-2 gap-1.5">
                {Array.from({ length: 4 }).map((_, index) => {
                  const tile = selectedTiles[index];
                  return (
                    <div
                      key={tile?.id ?? `empty-${index}`}
                      className={clsx(
                        "flex min-h-10 items-center justify-center rounded-lg border px-2 text-center text-xs font-semibold leading-tight",
                        tile
                          ? "border-ink bg-card text-ink shadow-tile"
                          : "border-neutral-200 bg-neutral-50 text-neutral-400"
                      )}
                    >
                      {tile?.text ?? "Pick tile"}
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {mode === "easy" && !isOver && (
            <div className="vg-rule mt-4 pt-4">
              <p className="text-sm font-semibold text-neutral-500">Easy hint</p>
              {guessesUntilEasyHint > 0 ? (
                <p className="mt-1 text-sm font-medium text-neutral-600">
                  Unlocks after {guessesUntilEasyHint} more{" "}
                  {guessesUntilEasyHint === 1 ? "guess" : "guesses"}.
                </p>
              ) : unlockedEasyHint ? (
                <div
                  className={clsx(
                    "mt-2 rounded-lg border bg-card/80 p-3",
                    hintBorderColors[unlockedEasyHint.colorIndex % hintBorderColors.length]
                  )}
                >
                  <p className="text-sm font-semibold">{unlockedEasyHint.name}</p>
                  <p className="mt-1 text-sm font-medium leading-snug text-neutral-700">
                    {unlockedEasyHint.text}
                  </p>
                </div>
              ) : (
                <p className="mt-1 text-sm font-medium text-neutral-600">
                  {easyHintStatus === "error" ? "Hint is unavailable right now." : "Hint is checking."}
                </p>
              )}
            </div>
          )}

          <div className="mt-4 hidden grid-cols-2 gap-3 lg:grid">
            <div className="vg-stat-cell">
              <p className="text-xs font-semibold text-neutral-500">Selected</p>
              <p className="mt-1 text-2xl font-extrabold">{attempt.selectedTileIds.length}/4</p>
            </div>
            <div className="vg-stat-cell">
              <p className="text-xs font-semibold text-neutral-500">Misses</p>
              <p className="mt-1 text-2xl font-extrabold">
                {attempt.mistakes}/{puzzle.mistakesAllowed}
              </p>
            </div>
          </div>

          <div className="mt-4 hidden grid-cols-4 gap-2 lg:grid" aria-label="Mistake counter">
            {Array.from({ length: puzzle.mistakesAllowed }).map((_, index) => (
              <div
                key={index}
                className={clsx(
                  "h-3 rounded-full border border-line",
                  index < attempt.mistakes ? "bg-tomato" : "bg-neutral-100"
                )}
              />
            ))}
          </div>

          <p className="mt-5 hidden min-h-12 text-lg font-extrabold leading-snug lg:block">{message}</p>

          {syncState === "error" && (
            <div className="mt-3 flex items-center justify-between gap-2 rounded-lg border border-tomato/60 bg-tomato/10 px-3 py-2 text-sm font-semibold">
              <span>Couldn&apos;t sync. Showing saved progress.</span>
              <button
                type="button"
                onClick={() => void syncAttempt()}
                className="inline-flex h-8 shrink-0 items-center justify-center rounded-lg border border-line bg-card px-2 text-xs font-semibold"
              >
                Resync
              </button>
            </div>
          )}

          <div className="vg-rule mt-4 pt-4 text-sm font-medium text-neutral-600">
            <p>Elapsed {formatElapsedTime(attempt.startedAt, elapsedFinishedAt)}</p>
            <p className="mt-1">Guesses {attempt.guessCount}</p>
            <p className="mt-1">
              {resolvedSession?.guest.label ?? "Guest session"}. Saved in this browser.
            </p>
            {resolvedSession?.admin.authenticated && (
              <p className="mt-1 font-semibold text-plum">Editor session active.</p>
            )}
          </div>

          {isOver && stats && stats.players >= MIN_STATS_PLAYERS && (
            <div className="mt-4 rounded-lg border border-plum/30 bg-plum/10 p-3">
              <p className="text-xs font-semibold text-plum">How others did</p>
              <div className="mt-2 grid grid-cols-2 gap-2 text-sm font-medium">
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
              <p className="text-xs font-semibold text-neutral-500">Your grid</p>
              <div className="mt-2 grid gap-1">
                {attempt.guessHistory.map((row, rowIndex) => (
                  <div key={rowIndex} className="flex gap-1">
                    {row.map((tileId, tileIndex) => (
                      <span
                        key={`${rowIndex}-${tileIndex}`}
                        className={clsx(
                          "h-5 w-5 rounded-sm border border-line",
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
          {!isOver && (
            <button
              className="vg-button-secondary hidden h-10 lg:inline-flex"
              type="button"
              disabled={remainingTiles.length < 2}
              onClick={shuffleRemaining}
            >
              <Shuffle aria-hidden size={16} />
              Shuffle tiles
            </button>
          )}
          {!isOver ? (
            <button
              className="vg-button-primary hidden h-12 lg:inline-flex"
              type="button"
              disabled={attempt.selectedTileIds.length !== 4 || isSubmitting}
              onClick={submitGuess}
            >
              <Send aria-hidden size={18} />
              Submit
            </button>
          ) : (
            <button
              className="vg-button-primary h-12 bg-yolk"
              type="button"
              onClick={shareResult}
            >
              <Share2 aria-hidden size={18} />
              {copied ? "Copied" : "Share"}
            </button>
          )}

          <Link href="/create" className="vg-button-secondary h-11">
            <Sparkles aria-hidden size={16} />
            Make your own
          </Link>
          <button
            type="button"
            onClick={() => setReportOpen((open) => !open)}
            className="vg-button-secondary h-10"
          >
            <Flag aria-hidden size={16} />
            Report a problem
          </button>
          {reportOpen && (
            <form className="grid gap-2 border-t border-neutral-200 pt-3" onSubmit={submitReport}>
              <p className="text-xs font-semibold leading-snug text-neutral-600">
                No login needed. Your report goes to the site operator&apos;s moderation queue,
                with the grid id, your reason, and any note you add.
              </p>
              <label className="grid gap-1 text-xs font-semibold text-neutral-600">
                Reason
                <select
                  value={reportReason}
                  onChange={(event) => setReportReason(event.target.value as ReportReason)}
                  className="vg-input h-10 text-sm font-medium"
                >
                  {reportReasons.map((reason) => (
                    <option key={reason.value} value={reason.value}>
                      {reason.label}
                    </option>
                  ))}
                </select>
              </label>
              <label className="grid gap-1 text-xs font-semibold text-neutral-600">
                What happened?
                <textarea
                  value={reportDetails}
                  onChange={(event) => setReportDetails(event.target.value)}
                  maxLength={1000}
                  rows={3}
                  placeholder="Tell us what feels unsafe, copied, broken, or unfair."
                  className="vg-input resize-none py-2 text-sm font-medium"
                />
              </label>
              <label className="grid gap-1 text-xs font-semibold text-neutral-600">
                Email for reply (optional)
                <input
                  value={reportContact}
                  onChange={(event) => setReportContact(event.target.value)}
                  maxLength={200}
                  placeholder="Only if you want a reply"
                  className="vg-input h-10 text-sm font-medium"
                />
              </label>
              <TurnstileWidget
                action="report_create"
                onTokenChange={setReportTurnstileToken}
                resetSignal={reportTurnstileReset}
              />
              <div className="grid grid-cols-2 gap-2">
                <button type="button" onClick={() => setReportOpen(false)} className="vg-button-secondary h-10">
                  <X aria-hidden size={15} />
                  Cancel
                </button>
                <button type="submit" disabled={isReporting} className="vg-button-primary h-10 bg-yolk px-3 text-sm">
                  <Send aria-hidden size={15} />
                  {isReporting ? "Sending" : "Send"}
                </button>
              </div>
            </form>
          )}
        </div>
      </aside>
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
    mode: serverAttempt.mode ?? current.mode,
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
