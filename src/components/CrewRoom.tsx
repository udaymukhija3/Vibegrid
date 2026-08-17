"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Check,
  Copy,
  Flame,
  Hourglass,
  LogOut,
  RefreshCw,
  Trophy,
  UserMinus,
  Users,
  X
} from "lucide-react";
import clsx from "clsx";
import { toast } from "sonner";
import {
  CrewBoard,
  CrewBoardEntry,
  CrewsUnavailableError,
  fetchCrewBoard,
  joinCrew,
  leaveCrew,
  removeCrewMember,
  rotateCrewInvite
} from "@/lib/api";
import { formatSeconds } from "@/lib/game";
import { writeClipboardText } from "@/lib/clipboard";
import { readStoredValue, writeStoredValue } from "@/lib/storage";

// The board refreshes on a timer so a crew watching the same daily sees each
// other move without reloading. It is deliberately slow: this is the polling
// fallback, and a friend group of ten on a free-tier instance should not
// generate a request per second between them.
const BOARD_REFRESH_MS = 15_000;

const DISPLAY_NAME_KEY = "vibegrid:crew:name";
const MAX_DISPLAY_NAME = 24;

export function CrewRoom() {
  const [crewId, setCrewId] = useState<string | null>(null);

  useEffect(() => {
    setCrewId(crewIdFromPath());
  }, []);

  if (crewId === null) {
    return <CrewNotice title="Loading crew" message="Finding this crew." />;
  }
  if (crewId === "") {
    return <CrewNotice title="Crew not found" message="That invite link is missing a crew id." />;
  }
  return <CrewBoardView crewId={crewId} />;
}

function CrewBoardView({ crewId }: { crewId: string }) {
  const [board, setBoard] = useState<CrewBoard | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [unavailable, setUnavailable] = useState(false);

  const load = useCallback(async () => {
    try {
      setBoard(await fetchCrewBoard(crewId));
      setError(null);
    } catch (loadError) {
      if (loadError instanceof CrewsUnavailableError) {
        setUnavailable(true);
        return;
      }
      setError("This crew is not available.");
    }
  }, [crewId]);

  useEffect(() => {
    void load();
  }, [load]);

  // Poll while the tab is visible. A backgrounded tab stops asking, so a phone
  // left open overnight is not still hitting the API in the morning.
  useEffect(() => {
    if (unavailable || error) {
      return;
    }
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") {
        void load();
      }
    }, BOARD_REFRESH_MS);

    const onVisible = () => {
      if (document.visibilityState === "visible") {
        void load();
      }
    };
    document.addEventListener("visibilitychange", onVisible);

    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [load, unavailable, error]);

  if (unavailable) {
    return (
      <CrewNotice
        title="Crews are off"
        message="This deployment is running without a database, so crews are unavailable."
      />
    );
  }
  if (error) {
    return <CrewNotice title="Crew not found" message={error} />;
  }
  if (!board) {
    return <CrewNotice title="Loading crew" message="Fetching today's crew board." />;
  }

  return (
    <div className="mx-auto grid w-full max-w-3xl gap-5">
      <CrewHeader board={board} onReload={load} />
      {board.isMember ? (
        <CrewStandings board={board} onReload={load} />
      ) : (
        <JoinCrewForm crewId={crewId} crewName={board.crew.name} onJoined={load} />
      )}
    </div>
  );
}

function CrewHeader({ board, onReload }: { board: CrewBoard; onReload: () => Promise<void> }) {
  const router = useRouter();
  const [inviteUrl, setInviteUrl] = useState("");
  const [copied, setCopied] = useState(false);
  const [confirmingRotate, setConfirmingRotate] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const url = new URL(window.location.href);
    url.hash = "";
    url.search = "";
    setInviteUrl(url.toString());
  }, []);

  async function copyInvite() {
    try {
      await writeClipboardText(inviteUrl);
      setCopied(true);
      toast.success("Invite link copied. Send it to your friends.");
    } catch {
      toast.error("Could not copy the invite link.");
    }
  }

  async function rotate() {
    if (busy) {
      return;
    }
    setBusy(true);
    try {
      const rotated = await rotateCrewInvite(board.crew.inviteCode);
      setConfirmingRotate(false);
      toast.success("New invite link. The old one no longer works.");
      // The code is in the URL, so the page has to follow it to its new address.
      router.replace(rotated.joinPath);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not rotate the invite.");
    } finally {
      setBusy(false);
    }
  }

  async function leave() {
    if (busy) {
      return;
    }
    setBusy(true);
    try {
      await leaveCrew(board.crew.inviteCode);
      toast.success(`You left ${board.crew.name}.`);
      router.push("/crews");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not leave that crew.");
      setBusy(false);
    }
  }

  return (
    <section className="vg-panel p-4 sm:p-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="vg-kicker">Crew</p>
          <h1 className="mt-2 truncate text-3xl font-extrabold leading-tight sm:text-4xl">
            {board.crew.name}
          </h1>
          <p className="mt-2 text-sm font-medium text-neutral-600">
            Everyone plays VibeGrid #{board.puzzleNumber} — today&apos;s grid — on their own.
          </p>
        </div>
        <div className="grid min-w-0 gap-2 sm:min-w-64">
          <button type="button" onClick={() => void copyInvite()} className="vg-button-primary bg-yolk">
            {copied ? <Check aria-hidden size={16} /> : <Copy aria-hidden size={16} />}
            {copied ? "Copied" : "Copy invite link"}
          </button>
          <div className="grid grid-cols-2 gap-2">
            <Link href="/" className="vg-button-secondary">
              <ArrowLeft aria-hidden size={15} />
              Play
            </Link>
            <Link href="/crews" className="vg-button-secondary">
              <Users aria-hidden size={15} />
              My crews
            </Link>
          </div>
          {inviteUrl && (
            <p
              aria-label="Crew invite link"
              title={inviteUrl}
              className="truncate rounded-lg border border-line bg-white px-3 py-2 text-xs font-medium text-neutral-600"
            >
              {inviteUrl}
            </p>
          )}
        </div>
      </div>

      {board.isMember && (
        <div className="mt-4 flex flex-wrap items-center gap-2 border-t border-line pt-3">
          {board.crew.isOwner && !confirmingRotate && (
            <button
              type="button"
              onClick={() => setConfirmingRotate(true)}
              className="vg-button-secondary h-9 px-3 text-xs"
            >
              <RefreshCw aria-hidden size={14} />
              New invite link
            </button>
          )}
          <button type="button" onClick={() => void leave()} disabled={busy} className="vg-button-secondary h-9 px-3 text-xs">
            <LogOut aria-hidden size={14} />
            Leave crew
          </button>
        </div>
      )}

      {confirmingRotate && (
        <div className="mt-3 rounded-lg border border-tomato/60 bg-tomato/10 p-3">
          <p className="font-extrabold">Replace the invite link?</p>
          <p className="mt-1 text-sm font-medium leading-6 text-neutral-700">
            Every link you have already shared stops working, so anyone who has not joined yet will
            need the new one. People already in the crew are unaffected — they reach it from My
            crews.
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <button type="button" disabled={busy} onClick={() => void rotate()} className="vg-button-primary h-9 bg-tomato px-3 text-xs">
              {busy ? "Replacing…" : "Replace link"}
            </button>
            <button type="button" disabled={busy} onClick={() => setConfirmingRotate(false)} className="vg-button-secondary h-9 px-3 text-xs">
              Keep current link
            </button>
          </div>
        </div>
      )}
    </section>
  );
}

function CrewStandings({ board, onReload }: { board: CrewBoard; onReload: () => Promise<void> }) {
  const you = board.members.find((member) => member.isYou);
  const finished = board.members.filter((member) => member.solved || member.failed).length;

  return (
    <section className="vg-panel p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline justify-between gap-2 border-b border-line pb-3">
        <h2 className="text-lg font-extrabold">Today&apos;s standings</h2>
        <p className="text-sm font-semibold text-neutral-500">
          {finished}/{board.members.length} done
        </p>
      </div>

      {!board.spoilersUnlocked && (
        <p className="mt-3 rounded-lg border border-yolk/80 bg-yolk/25 px-3 py-2 text-sm font-semibold leading-snug">
          {you?.playing
            ? "Finish today's grid to unlock everyone's result grids."
            : "Play today's grid to unlock everyone's result grids."}
        </p>
      )}

      <ul className="mt-4 grid gap-2">
        {board.members.map((member) => (
          <li key={member.displayName}>
            <CrewMemberRow
              member={member}
              groupCount={board.groupCount}
              inviteCode={board.crew.inviteCode}
              onRemoved={onReload}
            />
          </li>
        ))}
      </ul>

      {board.members.length === 1 && (
        <p className="mt-4 text-sm font-medium leading-6 text-neutral-600">
          You&apos;re the only one here so far. Send the invite link above and the board fills up as
          your friends play.
        </p>
      )}
    </section>
  );
}

function CrewMemberRow({
  member,
  groupCount,
  inviteCode,
  onRemoved
}: {
  member: CrewBoardEntry;
  groupCount: number;
  inviteCode: string;
  onRemoved: () => Promise<void>;
}) {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);

  async function remove() {
    if (!member.memberId || busy) {
      return;
    }
    setBusy(true);
    try {
      await removeCrewMember(inviteCode, member.memberId);
      toast.success(`Removed ${member.displayName}.`);
      await onRemoved();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not remove that member.");
      setBusy(false);
      setConfirming(false);
    }
  }

  return (
    <div
      className={clsx(
        "rounded-lg border p-3",
        member.isYou ? "border-ink bg-mint/25" : "border-line bg-white/70"
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="flex min-w-0 items-center gap-2 font-extrabold">
          <StatusIcon member={member} />
          <span className="truncate">{member.displayName}</span>
          {member.isYou && (
            <span className="shrink-0 rounded-lg border border-ink/30 bg-card px-1.5 py-0.5 text-[0.65rem] font-semibold">
              you
            </span>
          )}
        </p>
        <div className="flex items-center gap-2">
          <p className="text-sm font-semibold text-neutral-600">{statusLine(member, groupCount)}</p>
          {/* memberId is only sent to the crew owner, so this control simply
              does not exist for anyone else. */}
          {member.memberId && !confirming && (
            <button
              type="button"
              aria-label={`Remove ${member.displayName}`}
              title={`Remove ${member.displayName}`}
              onClick={() => setConfirming(true)}
              className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-lg border border-line bg-card text-neutral-500 hover:border-tomato hover:text-tomato"
            >
              <UserMinus aria-hidden size={14} />
            </button>
          )}
        </div>
      </div>

      {confirming && (
        <div className="mt-2 flex flex-wrap items-center gap-2 rounded-lg border border-tomato/60 bg-tomato/10 p-2">
          <p className="text-xs font-semibold">Remove {member.displayName} from the crew?</p>
          <button type="button" disabled={busy} onClick={() => void remove()} className="vg-button-primary h-8 bg-tomato px-2 text-xs">
            {busy ? "Removing…" : "Remove"}
          </button>
          <button type="button" disabled={busy} onClick={() => setConfirming(false)} className="vg-button-secondary h-8 px-2 text-xs">
            Cancel
          </button>
        </div>
      )}

      {member.grid && member.grid.length > 0 && (
        <div className="mt-2 grid gap-0.5 text-lg leading-none" aria-label={`${member.displayName}'s result grid`}>
          {member.grid.map((row, index) => (
            <span key={index}>{row}</span>
          ))}
        </div>
      )}
    </div>
  );
}

function StatusIcon({ member }: { member: CrewBoardEntry }) {
  if (member.solved) {
    return <Trophy aria-hidden size={16} className="shrink-0 text-plum" />;
  }
  if (member.failed) {
    return <X aria-hidden size={16} className="shrink-0 text-tomato" />;
  }
  if (member.playing) {
    return <Flame aria-hidden size={16} className="shrink-0 text-yolk" />;
  }
  return <Hourglass aria-hidden size={16} className="shrink-0 text-neutral-400" />;
}

function statusLine(member: CrewBoardEntry, groupCount: number) {
  if (member.solved) {
    const time = member.elapsedSeconds === undefined ? "" : ` in ${formatSeconds(member.elapsedSeconds)}`;
    return `Solved${time} · ${member.mistakes} ${member.mistakes === 1 ? "miss" : "misses"}`;
  }
  if (member.failed) {
    return `Out of misses · ${member.solvedCount}/${groupCount} found`;
  }
  if (member.playing) {
    return `Playing · ${member.solvedCount}/${groupCount} found`;
  }
  return "Not started";
}

function JoinCrewForm({
  crewId,
  crewName,
  onJoined
}: {
  crewId: string;
  crewName: string;
  onJoined: () => Promise<void>;
}) {
  // Reuse the name from the last crew this browser joined, so a second invite
  // is one tap rather than another round of typing.
  const remembered = useMemo(() => readStoredValue(DISPLAY_NAME_KEY) ?? "", []);
  const [displayName, setDisplayName] = useState(remembered);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const name = displayName.trim();
    if (!name || busy) {
      return;
    }
    setBusy(true);
    try {
      await joinCrew(crewId, name);
      writeStoredValue(DISPLAY_NAME_KEY, name);
      toast.success(`You're in. Welcome to ${crewName}.`);
      await onJoined();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not join that crew.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="vg-panel p-4 sm:p-5">
      <h2 className="text-lg font-extrabold">Join {crewName}</h2>
      <p className="mt-1 text-sm font-medium leading-6 text-neutral-600">
        Pick a name your friends will recognise on the board. No account, no email — it is saved in
        this browser.
      </p>
      <form className="mt-4 grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto]" onSubmit={submit}>
        <label className="grid gap-1 text-xs font-semibold text-neutral-600">
          Your name in this crew
          <input
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            maxLength={MAX_DISPLAY_NAME}
            required
            autoComplete="nickname"
            placeholder="e.g. Uday"
            className="vg-input h-11 font-medium"
          />
        </label>
        <button type="submit" disabled={busy || !displayName.trim()} className="vg-button-primary h-11 sm:mt-5">
          <Users aria-hidden size={16} />
          {busy ? "Joining…" : "Join crew"}
        </button>
      </form>
    </section>
  );
}

function CrewNotice({ title, message }: { title: string; message: string }) {
  return (
    <section className="vg-panel mx-auto w-full max-w-3xl p-5">
      <h1 className="text-2xl font-extrabold">{title}</h1>
      <p className="mt-2 font-medium text-neutral-600">{message}</p>
      <Link href="/" className="vg-button-secondary mt-4">
        <ArrowLeft aria-hidden size={15} />
        Today&apos;s grid
      </Link>
    </section>
  );
}

function crewIdFromPath() {
  const [, id = ""] = window.location.pathname.match(/^\/crew\/([^/]+)/) ?? [];
  return decodeURIComponent(id);
}
