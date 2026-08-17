"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ChevronRight, Sparkles, Users } from "lucide-react";
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
          // The list is a convenience; the create form below still works.
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
      toast.success("Crew created. Copy the invite link and send it.");
      router.push(crew.joinPath);
    } catch (error) {
      if (error instanceof CrewsUnavailableError) {
        setUnavailable(true);
        return;
      }
      toast.error(error instanceof Error ? error.message : "Could not make that crew.");
    } finally {
      setBusy(false);
    }
  }

  if (unavailable) {
    return (
      <section className="vg-panel mt-6 p-4">
        <h2 className="text-lg font-extrabold">Crews are off</h2>
        <p className="mt-1 text-sm font-medium text-neutral-600">
          This deployment is running without a database, so crews are unavailable.
        </p>
      </section>
    );
  }

  return (
    <div className="mt-6 grid gap-6">
      {crews && crews.length > 0 && (
        <section className="vg-panel p-4">
          <h2 className="text-lg font-extrabold">Crews you&apos;re in</h2>
          <ul className="mt-3 grid gap-2">
            {crews.map((crew) => (
              <li key={crew.inviteCode}>
                <Link
                  href={crew.joinPath}
                  className="flex items-center justify-between gap-3 rounded-lg border border-line bg-white/70 px-3 py-3 font-semibold hover:border-ink"
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <Users aria-hidden size={16} className="shrink-0 text-neutral-500" />
                    <span className="truncate">{crew.name}</span>
                  </span>
                  <ChevronRight aria-hidden size={16} className="shrink-0 text-neutral-400" />
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      <section className="vg-panel p-4">
        <h2 className="text-lg font-extrabold">Make a crew</h2>
        <p className="mt-1 text-sm font-medium leading-6 text-neutral-600">
          No account needed. You get a link — anyone who opens it joins and plays today&apos;s grid
          with you.
        </p>
        <form className="mt-4 grid gap-3" onSubmit={submit}>
          <label className="grid gap-1 text-xs font-semibold text-neutral-600">
            Crew name
            <input
              value={crewName}
              onChange={(event) => setCrewName(event.target.value)}
              maxLength={MAX_CREW_NAME}
              required
              placeholder="e.g. Sunday Morning Club"
              className="vg-input h-11 font-medium"
            />
          </label>
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
          <button
            type="submit"
            disabled={busy || !crewName.trim() || !displayName.trim()}
            className="vg-button-primary h-11"
          >
            <Sparkles aria-hidden size={16} />
            {busy ? "Making crew…" : "Make crew and get link"}
          </button>
        </form>
      </section>
    </div>
  );
}
