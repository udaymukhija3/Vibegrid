"use client";

import { useCallback, useMemo, useState, type FormEvent } from "react";
import Link from "next/link";
import { Send } from "lucide-react";
import { toast } from "sonner";
import { appealPuzzle, fetchPuzzleById } from "@/lib/api";
import { useResource } from "@/hooks/useResource";
import { VibeGridGame } from "@/components/VibeGridGame";

export function PlaySharedPuzzle({ puzzleId: explicitPuzzleId }: { puzzleId?: string }) {
  const puzzleId = useMemo(() => explicitPuzzleId ?? puzzleIdFromPath(), [explicitPuzzleId]);
  const loader = useCallback(() => fetchPuzzleById(puzzleId), [puzzleId]);
  const state = useResource(loader, "This shared grid is not available.");
  const [appealContact, setAppealContact] = useState("");
  const [appealMessage, setAppealMessage] = useState("");
  const [appealBusy, setAppealBusy] = useState(false);
  const [appealSent, setAppealSent] = useState(false);

  if (state.status === "ready") {
    return <VibeGridGame puzzle={state.data} />;
  }

  async function submitAppeal(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!puzzleId || appealBusy) {
      return;
    }

    setAppealBusy(true);
    try {
      await appealPuzzle({
        puzzleId,
        contact: appealContact,
        message: appealMessage
      });
      setAppealSent(true);
      setAppealMessage("");
      toast.success("Appeal sent.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not send that appeal.");
    } finally {
      setAppealBusy(false);
    }
  }

  return (
    <div className="mx-auto flex min-h-[calc(100vh-2.5rem)] max-w-3xl items-center justify-center">
      <div className="vg-panel w-full p-6 text-center">
        <h1 className="text-3xl font-extrabold">VibeGrid</h1>
        <p className="mt-3 font-medium text-neutral-600">
          {state.status === "loading" ? "Loading this grid." : state.message}
        </p>
        {state.status === "error" && (
          <>
            <Link
              href="/"
              className="vg-button-primary mt-4"
            >
              Play today&apos;s puzzle
            </Link>
            <form className="mx-auto mt-5 grid max-w-xl gap-3 border-t border-neutral-200 pt-5 text-left" onSubmit={submitAppeal}>
              <div>
                <h2 className="text-base font-extrabold">Ask for a review</h2>
                <p className="mt-1 text-sm font-medium text-neutral-600">
                  If this was your grid, send a short note and a way to reach you.
                </p>
              </div>
              {appealSent ? (
                <p className="rounded-lg border border-mint/70 bg-mint/20 px-3 py-2 text-sm font-semibold text-ink">
                  Appeal sent. A moderator can reinstate the grid from the admin queue.
                </p>
              ) : (
                <>
                  <label className="grid gap-1 text-xs font-semibold text-neutral-600">
                    Contact
                    <input
                      value={appealContact}
                      onChange={(event) => setAppealContact(event.target.value)}
                      maxLength={200}
                      placeholder="Email or handle"
                      className="vg-input h-10 text-sm font-medium"
                    />
                  </label>
                  <label className="grid gap-1 text-xs font-semibold text-neutral-600">
                    Message
                    <textarea
                      value={appealMessage}
                      onChange={(event) => setAppealMessage(event.target.value)}
                      maxLength={1000}
                      rows={4}
                      required
                      placeholder="Why should this grid be restored?"
                      className="vg-input resize-none py-2 text-sm font-medium"
                    />
                  </label>
                  <button
                    type="submit"
                    disabled={appealBusy || !appealMessage.trim()}
                    className="vg-button-primary bg-yolk"
                  >
                    <Send aria-hidden size={16} />
                    {appealBusy ? "Sending" : "Send appeal"}
                  </button>
                </>
              )}
            </form>
          </>
        )}
      </div>
    </div>
  );
}

function puzzleIdFromPath() {
  if (typeof window === "undefined") {
    return "";
  }

  const [, id = ""] = window.location.pathname.match(/^\/p\/([^/]+)/) ?? [];
  return decodeURIComponent(id);
}
