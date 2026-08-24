"use client";

import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Check,
  Clock3,
  Crown,
  LogOut,
  RefreshCw,
  Send,
  ShieldCheck,
  Sparkles,
  UserMinus,
  Users
} from "lucide-react";
import { toast } from "sonner";
import { VibeCard } from "@/components/VibeCard";
import { VibeComposer } from "@/components/VibeComposer";
import { VibeHeader } from "@/components/VibeHeader";
import {
  CrewsUnavailableError,
  castVibeVote,
  fetchCrewDaily,
  joinCrew,
  leaveCrew,
  removeCrewMember,
  rotateCrewInvite,
  submitVibeCard
} from "@/lib/api";
import { writeClipboardText } from "@/lib/clipboard";
import { readStoredValue, writeStoredValue } from "@/lib/storage";
import type { VibeCrewDaily } from "@/types/vibe";

const BOARD_REFRESH_MS = 30_000;
const DISPLAY_NAME_KEY = "vibegrid:crew:name";
const MAX_DISPLAY_NAME = 24;

export function CrewRoom() {
  const [crewId, setCrewId] = useState<string | null>(null);

  useEffect(() => {
    setCrewId(crewIdFromPath());
  }, []);

  if (crewId === null) {
    return <CrewNotice title="Finding your crew…" message="Opening the private round." />;
  }
  if (crewId === "") {
    return <CrewNotice title="Crew not found" message="That invite link is missing its crew code." />;
  }
  return <CrewDailyView crewId={crewId} />;
}

function CrewDailyView({ crewId }: { crewId: string }) {
  const [daily, setDaily] = useState<VibeCrewDaily | null>(null);
  const [error, setError] = useState("");
  const [unavailable, setUnavailable] = useState(false);
  const loadSequence = useRef(0);

  const load = useCallback(async () => {
    const sequence = ++loadSequence.current;
    try {
      const nextDaily = await fetchCrewDaily(crewId);
      if (sequence !== loadSequence.current) {
        return;
      }
      setDaily(nextDaily);
      setError("");
      setUnavailable(false);
    } catch (loadError) {
      if (sequence !== loadSequence.current) {
        return;
      }
      if (loadError instanceof CrewsUnavailableError) {
        setUnavailable(true);
        return;
      }
      setError(loadError instanceof Error ? loadError.message : "This crew is not available.");
    }
  }, [crewId]);

  useEffect(() => {
    void load();
  }, [load]);

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
  }, [error, load, unavailable]);

  if (unavailable) {
    return (
      <CrewNotice
        title="Crew rounds need Postgres"
        message="The practice round still works here, but private multi-person state is unavailable on this no-database deployment."
      />
    );
  }
  if (error) {
    return <CrewNotice title="Crew not found" message={error} />;
  }
  if (!daily) {
    return <CrewNotice title="Loading the crew…" message="Collecting the latest cards and ballots." />;
  }

  return (
    <div className="vg-shell">
      <VibeHeader compact />
      <CrewHero daily={daily} />
      {!daily.isMember ? (
        <JoinCrewLanding crewId={crewId} daily={daily} onJoined={load} />
      ) : (
        <div className="grid gap-8 pb-12">
          <ResultSection daily={daily} />
          <JudgeSection daily={daily} onReload={load} />
          <MakeSection daily={daily} onReload={load} />
          <CrewRoster daily={daily} onReload={load} />
        </div>
      )}
    </div>
  );
}

function CrewHero({ daily }: { daily: VibeCrewDaily }) {
  const router = useRouter();
  const [inviteUrl, setInviteUrl] = useState("");
  const [copied, setCopied] = useState(false);
  const [managing, setManaging] = useState(false);
  const [confirmRotate, setConfirmRotate] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const url = new URL(window.location.href);
    url.hash = "";
    url.search = "";
    setInviteUrl(url.toString());
  }, []);

  async function copyInvite() {
    try {
      await writeClipboardText(
        `${daily.crew.name} is building "${daily.today.board.prompt}"\n${daily.today.submittedCount}/${daily.today.memberCount} are in. Make yours: ${inviteUrl}`
      );
      setCopied(true);
      toast.success("Crew invite copied.");
    } catch {
      toast.error("Could not copy the invite.");
    }
  }

  async function rotate() {
    setBusy(true);
    try {
      const rotated = await rotateCrewInvite(daily.crew.inviteCode);
      toast.success("Old invite revoked. This is the new one.");
      router.replace(rotated.joinPath);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not replace the invite.");
      setBusy(false);
    }
  }

  async function leave() {
    setBusy(true);
    try {
      await leaveCrew(daily.crew.inviteCode);
      toast.success(`You left ${daily.crew.name}.`);
      router.push("/crews");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not leave the crew.");
      setBusy(false);
    }
  }

  return (
    <header className="py-8 sm:py-10">
      <div className="flex flex-wrap items-start justify-between gap-5">
        <div>
          <p className="vg-meta text-lime">Private crew</p>
          <h1 className="mt-2 text-4xl font-black tracking-[-0.05em] text-cream sm:text-6xl">{daily.crew.name}</h1>
          <p className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm font-bold text-cream/[.58]">
            <span>Board {String(daily.today.board.boardNumber).padStart(3, "0")}</span>
            {daily.isMember && (
              <span className="inline-flex items-center gap-1.5 text-amber">
                <Sparkles aria-hidden size={15} />
                {daily.crewStreak} day crew streak
              </span>
            )}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={() => void copyInvite()} className="vg-primary-button">
            {copied ? <Check aria-hidden size={17} /> : <Send aria-hidden size={17} />}
            {copied ? "Copied" : "Nudge the crew"}
          </button>
          {daily.isMember && (
            <button type="button" onClick={() => setManaging((value) => !value)} className="vg-secondary-button">
              <ShieldCheck aria-hidden size={17} />
              Manage
            </button>
          )}
        </div>
      </div>

      {managing && (
        <div className="mt-5 rounded-2xl border-2 border-line bg-paper p-4">
          <div className="flex flex-wrap items-center gap-2">
            <Link href="/crews" className="vg-secondary-button">
              <Users aria-hidden size={16} />
              All crews
            </Link>
            {daily.crew.isOwner && !confirmRotate && (
              <button type="button" onClick={() => setConfirmRotate(true)} className="vg-secondary-button">
                <RefreshCw aria-hidden size={16} />
                Replace invite
              </button>
            )}
            <button type="button" disabled={busy} onClick={() => void leave()} className="vg-secondary-button">
              <LogOut aria-hidden size={16} />
              Leave
            </button>
          </div>
          {confirmRotate && (
            <div className="mt-4 border-l-4 border-coral pl-4">
              <p className="font-black">Revoke every old invite link?</p>
              <p className="mt-1 text-sm font-semibold text-cream/[.58]">
                Current members stay. Anyone who has not joined will need the replacement link.
              </p>
              <div className="mt-3 flex gap-2">
                <button type="button" disabled={busy} onClick={() => void rotate()} className="vg-primary-button bg-coral">
                  Replace it
                </button>
                <button type="button" onClick={() => setConfirmRotate(false)} className="vg-secondary-button">
                  Keep it
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </header>
  );
}

function ResultSection({ daily }: { daily: VibeCrewDaily }) {
  const result = daily.result;
  if (!result) {
    return null;
  }
  return (
    <section aria-labelledby="latest-result">
      <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="vg-meta text-lime">Latest result · {result.board.publishDate}</p>
          <h2 id="latest-result" className="mt-2 text-3xl font-black text-cream sm:text-4xl">
            {!result.official
              ? "A quiet round, revealed."
              : result.tied
                ? "The crew split the vote."
                : "The crew picked a winner."}
          </h2>
          {result.tied && (
            <p className="mt-2 max-w-xl text-sm font-semibold leading-6 text-cream/[.58]">
              Nobody takes it outright — the ballots landed level across the top.
            </p>
          )}
        </div>
        <p className="font-mono text-xs font-bold uppercase tracking-[0.08em] text-cream/[.48]">
          {result.submissionCount} cards · {result.voteCount} votes
        </p>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        {result.cards.map((card) => <VibeCard key={card.id} card={card} />)}
      </div>
      {!result.official && (
        <p className="mt-4 rounded-xl border-2 border-amber/60 bg-amber/[.1] px-4 py-3 text-sm font-bold text-cream">
          A round counts once two people have made a card and two have judged. The cards still belong to the crew.
        </p>
      )}
    </section>
  );
}

function JudgeSection({ daily, onReload }: { daily: VibeCrewDaily; onReload: () => Promise<void> }) {
  const judge = daily.judge;
  const [selection, setSelection] = useState(judge?.yourVoteId ?? "");
  const [busy, setBusy] = useState(false);

  if (!judge) {
    return null;
  }
  if (!judge.eligible) {
    return (
      <section className="vg-dark-panel">
        <p className="vg-meta text-violet-light">Yesterday&apos;s ballot</p>
        <h2 className="mt-2 text-2xl font-black text-cream">You sat this one out.</h2>
        <p className="mt-2 max-w-xl text-sm font-semibold leading-6 text-cream/[.58]">
          Only people who made a card get a ballot. The authors and result reveal tomorrow.
        </p>
      </section>
    );
  }

  const activeJudge = judge;

  async function vote() {
    if (!selection || busy || activeJudge.hasVoted) {
      return;
    }
    setBusy(true);
    try {
      await castVibeVote({
        inviteCode: daily.crew.inviteCode,
        boardId: activeJudge.board.id,
        submissionId: selection,
        clientVoteId: crypto.randomUUID()
      });
      toast.success("Ballot locked. Authors reveal tomorrow.");
      await onReload();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not lock that ballot.");
      setBusy(false);
    }
  }

  return (
    <section className="vg-dark-panel" aria-labelledby="judge-heading">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="vg-meta text-violet-light">Judge yesterday · authors hidden</p>
          <h2 id="judge-heading" className="mt-2 text-3xl font-black text-cream">{judge.board.prompt}</h2>
        </div>
        <p className="flex items-center gap-2 text-sm font-semibold text-cream/[.55]">
          <Clock3 aria-hidden size={16} /> One vote. No self-votes.
        </p>
      </div>
      <div className="mt-6 grid gap-4 md:grid-cols-2">
        {(judge.cards ?? []).map((card) => (
          <VibeCard
            key={card.id}
            card={card}
            selectable
            disabled={card.isYours || judge.hasVoted}
            selected={(judge.yourVoteId ?? selection) === card.id}
            onSelect={() => setSelection(card.id)}
          />
        ))}
      </div>
      <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm font-semibold text-cream/[.55]">
          {judge.hasVoted ? "Your vote is locked. The tally stays hidden until reveal." : "Back the card that makes the four fragments feel inevitable."}
        </p>
        {!judge.hasVoted && (
          <button type="button" disabled={!selection || busy} onClick={() => void vote()} className="vg-primary-button">
            <Crown aria-hidden size={17} />
            {busy ? "Locking…" : "Lock my vote"}
          </button>
        )}
      </div>
    </section>
  );
}

function MakeSection({ daily, onReload }: { daily: VibeCrewDaily; onReload: () => Promise<void> }) {
  if (daily.today.submission) {
    return (
      <section aria-labelledby="today-card">
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <div>
            <p className="vg-meta text-amber">Today · locked in</p>
            <h2 id="today-card" className="mt-2 text-3xl font-black text-cream">Your card is waiting for tomorrow.</h2>
          </div>
          <p className="font-mono text-xs font-bold uppercase tracking-[0.08em] text-cream/[.48]">
            {daily.today.submittedCount}/{daily.today.memberCount} in
          </p>
        </div>
        <div className="max-w-2xl">
          <VibeCard card={daily.today.submission} />
        </div>
      </section>
    );
  }

  return (
    <section className="vg-dark-panel" aria-labelledby="make-heading">
      <div className="mb-5 flex items-center justify-between gap-3">
        <div>
          <p className="vg-meta text-amber">Make today</p>
          <h2 id="make-heading" className="sr-only">Make today&apos;s vibe card</h2>
        </div>
        <p className="font-mono text-xs font-bold uppercase tracking-[0.08em] text-cream/[.48]">
          {daily.today.submittedCount}/{daily.today.memberCount} in
        </p>
      </div>
      <VibeComposer
        board={daily.today.board}
        onSubmit={async ({ title, selectedTileIds }) => {
          await submitVibeCard({
            inviteCode: daily.crew.inviteCode,
            boardId: daily.today.board.id,
            title,
            selectedTileIds,
            clientSubmissionId: crypto.randomUUID()
          });
          toast.success("Vibe locked. Nobody sees it until tomorrow.");
          await onReload();
        }}
      />
    </section>
  );
}

function CrewRoster({ daily, onReload }: { daily: VibeCrewDaily; onReload: () => Promise<void> }) {
  const members = daily.members ?? [];
  return (
    <section className="border-t-2 border-cream/[.14] pt-7">
      <div className="flex items-end justify-between gap-3">
        <div>
          <p className="vg-meta">Crew check-in</p>
          <h2 className="mt-2 text-2xl font-black text-cream">{members.filter((member) => member.submittedToday).length} of {members.length} are in.</h2>
        </div>
      </div>
      <ul className="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {members.map((member) => (
          <li key={member.displayName} className="flex items-center justify-between gap-3 rounded-xl border-2 border-line bg-paper px-3 py-3">
            <span className="min-w-0">
              <span className="block truncate font-black text-cream">{member.displayName}{member.isYou ? " · you" : ""}</span>
              <span className={member.submittedToday ? "font-mono text-[0.68rem] font-bold uppercase text-lime" : "font-mono text-[0.68rem] font-bold uppercase text-cream/35"}>
                {member.submittedToday ? "locked in" : "not in yet"}
              </span>
            </span>
            {member.memberId && (
              <RemoveMemberButton
                inviteCode={daily.crew.inviteCode}
                memberId={member.memberId}
                displayName={member.displayName}
                onRemoved={onReload}
              />
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

function RemoveMemberButton({
  inviteCode,
  memberId,
  displayName,
  onRemoved
}: {
  inviteCode: string;
  memberId: string;
  displayName: string;
  onRemoved: () => Promise<void>;
}) {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);

  async function remove() {
    setBusy(true);
    try {
      await removeCrewMember(inviteCode, memberId);
      toast.success(`Removed ${displayName}.`);
      await onRemoved();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not remove that member.");
      setBusy(false);
      setConfirming(false);
    }
  }

  if (confirming) {
    return (
      <span className="flex gap-1">
        <button type="button" disabled={busy} onClick={() => void remove()} className="rounded-lg bg-coral px-2 py-1 text-xs font-black text-ink">
          {busy ? "…" : "Remove"}
        </button>
        <button type="button" onClick={() => setConfirming(false)} className="rounded-lg border border-cream/20 px-2 py-1 text-xs font-bold">
          No
        </button>
      </span>
    );
  }
  return (
    <button
      type="button"
      onClick={() => setConfirming(true)}
      aria-label={`Remove ${displayName}`}
      className="inline-flex h-9 w-9 items-center justify-center rounded-lg border-2 border-line text-cream/45 hover:border-coral hover:text-coral"
    >
      <UserMinus aria-hidden size={15} />
    </button>
  );
}

function JoinCrewLanding({
  crewId,
  daily,
  onJoined
}: {
  crewId: string;
  daily: VibeCrewDaily;
  onJoined: () => Promise<void>;
}) {
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
      toast.success(`You're in ${daily.crew.name}.`);
      await onJoined();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not join this crew.");
      setBusy(false);
    }
  }

  return (
    <div className="grid gap-6 pb-12 lg:grid-cols-[1.1fr_.9fr]">
      <section className="vg-hero-card">
        <p className="vg-meta text-ink/[.55]">Today&apos;s shared prompt</p>
        <h2 className="mt-3 text-4xl font-black leading-[1.02]">{daily.today.board.prompt}</h2>
        <div
          className="mt-6 grid grid-cols-4 gap-2"
          aria-label={`Today's ${daily.today.board.tiles.length} fragments in ${daily.today.board.tiles.length / 4} rows`}
        >
          {daily.today.board.tiles.map((tile) => (
            <span key={tile.id} className="vg-hero-fragment">{tile.text}</span>
          ))}
        </div>
        <p className="mt-5 border-t-2 border-ink/[.15] pt-4 font-mono text-xs font-bold uppercase tracking-[0.08em]">
          {daily.today.submittedCount}/{daily.today.memberCount} have made today&apos;s card
        </p>
      </section>

      <section className="vg-dark-panel self-start">
        <p className="vg-meta text-lime">Private invite</p>
        <h2 className="mt-2 text-3xl font-black text-cream">Join {daily.crew.name}</h2>
        <p className="mt-3 text-sm font-semibold leading-6 text-cream/[.58]">
          Pick a name your friends recognise. No email or password—the membership stays in this browser.
        </p>
        <form onSubmit={submit} className="mt-5 grid gap-3">
          <label className="grid gap-2">
            <span className="vg-meta">Your name in this crew</span>
            <input
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
              maxLength={MAX_DISPLAY_NAME}
              required
              autoComplete="nickname"
              placeholder="e.g. Uday"
              className="vg-title-input"
            />
          </label>
          <button type="submit" disabled={busy || !displayName.trim()} className="vg-primary-button">
            <Users aria-hidden size={17} />
            {busy ? "Joining…" : "Join and make today's vibe"}
          </button>
        </form>
      </section>
    </div>
  );
}

function CrewNotice({ title, message }: { title: string; message: string }) {
  return (
    <div className="vg-shell">
      <VibeHeader />
      <section className="vg-dark-panel mx-auto mt-12 max-w-2xl text-center">
        <h1 className="text-3xl font-black text-cream">{title}</h1>
        <p className="mt-3 font-semibold leading-7 text-cream/[.58]">{message}</p>
        <Link href="/" className="vg-secondary-button mt-5">
          <ArrowLeft aria-hidden size={16} />
          Back to practice
        </Link>
      </section>
    </div>
  );
}

function crewIdFromPath() {
  const match = window.location.pathname.match(/^\/crew\/([^/]+)\/?$/);
  return match?.[1] ? decodeURIComponent(match[1]) : "";
}
