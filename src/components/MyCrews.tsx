"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowRight, Sparkles, Users } from "lucide-react";
import { toast } from "sonner";
import { Crew, CrewsUnavailableError, createCrew, fetchMyCrews } from "@/lib/api";
import { readStoredValue, writeStoredValue } from "@/lib/storage";

const DISPLAY_NAME_KEY = "vibegrid:crew:name";
const MAX_CREW_NAME = 40;
const MAX_DISPLAY_NAME = 24;

export function MyCrews() {
  const router = useRouter();
  const [crews, setCrews] = useState<Crew[] | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const remembered = useMemo(() => readStoredValue(DISPLAY_NAME_KEY) ?? "", []);
  const [crewName, setCrewName] = useState("");
  const [displayName, setDisplayName] = useState(remembered);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    fetchMyCrews()
      .then((loaded) => {
        if (!cancelled) {
          setCrews(loaded);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setCrews([]);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const name = crewName.trim();
    const member = displayName.trim();
    if (!name || !member || busy) {
      return;
    }
    setBusy(true);
    try {
      const crew = await createCrew(name, member);
      writeStoredValue(DISPLAY_NAME_KEY, member);
      toast.success("Crew created. Now bring in two people.");
      router.push(crew.joinPath);
    } catch (error) {
      if (error instanceof CrewsUnavailableError) {
        setUnavailable(true);
      } else {
        toast.error(error instanceof Error ? error.message : "Could not make that crew.");
      }
      setBusy(false);
    }
  }

  if (unavailable) {
    return (
      <section className="vg-dark-panel">
        <p className="vg-meta text-amber">Database required</p>
        <h2 className="mt-2 text-2xl font-black text-cream">Private crews are off in this runtime.</h2>
        <p className="mt-2 max-w-xl text-sm font-semibold leading-6 text-cream/[.58]">
          The public practice round still works. Connect Postgres to persist memberships, cards, ballots, and results.
        </p>
      </section>
    );
  }

  return (
    <div className="grid gap-7 lg:grid-cols-[.9fr_1.1fr]">
      <section className="vg-dark-panel self-start">
        <p className="vg-meta text-lime">Start a private ritual</p>
        <h2 className="mt-2 text-3xl font-black text-cream">Make a crew.</h2>
        <p className="mt-3 text-sm font-semibold leading-6 text-cream/[.58]">
          One link, no accounts. Three people are enough for an official daily result.
        </p>
        <form className="mt-5 grid gap-4" onSubmit={submit}>
          <label className="grid gap-2">
            <span className="vg-meta">Crew name</span>
            <input
              value={crewName}
              onChange={(event) => setCrewName(event.target.value)}
              maxLength={MAX_CREW_NAME}
              required
              placeholder="Sunday Morning Club"
              className="vg-title-input"
            />
          </label>
          <label className="grid gap-2">
            <span className="vg-meta">Your name in this crew</span>
            <input
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
              maxLength={MAX_DISPLAY_NAME}
              required
              autoComplete="nickname"
              placeholder="Uday"
              className="vg-title-input"
            />
          </label>
          <button
            type="submit"
            disabled={busy || !crewName.trim() || !displayName.trim()}
            className="vg-primary-button"
          >
            <Sparkles aria-hidden size={17} />
            {busy ? "Making…" : "Make crew and get link"}
          </button>
        </form>
      </section>

      <section>
        <div className="flex items-end justify-between gap-3">
          <div>
            <p className="vg-meta text-violet-light">Your memberships</p>
            <h2 className="mt-2 text-3xl font-black text-cream">Return to a crew.</h2>
          </div>
          {crews && <span className="vg-count">{crews.length}</span>}
        </div>

        {crews === null ? (
          <p className="mt-5 font-semibold text-cream/[.5]">Loading your crews…</p>
        ) : crews.length === 0 ? (
          <div className="mt-5 rounded-2xl border-2 border-dashed border-line p-6 text-center">
            <Users aria-hidden className="mx-auto text-cream/30" size={28} />
            <p className="mt-3 font-black text-cream">No crews in this browser yet.</p>
            <p className="mt-1 text-sm font-semibold text-cream/[.48]">Create one here or open a friend&apos;s invite.</p>
          </div>
        ) : (
          <ul className="mt-5 grid gap-3">
            {crews.map((crew) => (
              <li key={crew.inviteCode}>
                <Link href={crew.joinPath} className="group flex items-center justify-between gap-4 rounded-2xl border-2 border-line bg-paper p-4 transition hover:border-lime">
                  <span className="min-w-0">
                    <span className="block truncate text-xl font-black text-cream">{crew.name}</span>
                    <span className="mt-1 block font-mono text-[0.68rem] font-bold uppercase tracking-[0.08em] text-cream/[.42]">
                      {crew.isOwner ? "you own this crew" : "private member"}
                    </span>
                  </span>
                  <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-cream text-ink transition group-hover:bg-lime">
                    <ArrowRight aria-hidden size={18} strokeWidth={3} />
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
