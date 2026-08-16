"use client";

import { useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import { Send, Trash2 } from "lucide-react";
import { toast } from "sonner";
import {
  appealPuzzle,
  fetchCreatorPuzzleStatus,
  withdrawCreatorPuzzle,
  type CreatorPuzzleStatus
} from "@/lib/api";
import { TurnstileWidget } from "@/components/TurnstileWidget";

export function CreatorClaim() {
  const [claim, setClaim] = useState<{ id: string; secret: string } | null>(null);
  const [status, setStatus] = useState<CreatorPuzzleStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmingWithdrawal, setConfirmingWithdrawal] = useState(false);
  const [appealContact, setAppealContact] = useState("");
  const [appealMessage, setAppealMessage] = useState("");
  const [appealSent, setAppealSent] = useState(false);
  const [turnstileToken, setTurnstileToken] = useState("");
  const [turnstileReset, setTurnstileReset] = useState(0);

  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get("id")?.trim() ?? "";
    const secret = window.location.hash.slice(1).trim();
    if (!id || !secret) {
      setError("This private claim link is incomplete.");
      return;
    }
    const loadedClaim = { id, secret };
    setClaim(loadedClaim);
    fetchCreatorPuzzleStatus(id, secret)
      .then(setStatus)
      .catch((loadError) => {
        setError(loadError instanceof Error ? loadError.message : "Could not load this creator claim.");
      });
  }, []);

  async function withdraw() {
    if (!claim || busy) {
      return;
    }
    setBusy(true);
    try {
      setStatus(await withdrawCreatorPuzzle(claim.id, claim.secret));
      setConfirmingWithdrawal(false);
      toast.success("Submission withdrawn.");
    } catch (withdrawError) {
      toast.error(withdrawError instanceof Error ? withdrawError.message : "Could not withdraw this grid.");
    } finally {
      setBusy(false);
    }
  }

  async function submitAppeal(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!claim || busy || !appealMessage.trim()) {
      return;
    }
    if (!turnstileToken) {
      toast.error("Complete the bot check before sending.");
      return;
    }
    setBusy(true);
    try {
      await appealPuzzle(
        { puzzleId: claim.id, contact: appealContact, message: appealMessage },
        claim.secret,
        turnstileToken
      );
      setAppealSent(true);
      setAppealMessage("");
      toast.success("Appeal sent.");
    } catch (appealError) {
      toast.error(appealError instanceof Error ? appealError.message : "Could not send that appeal.");
    } finally {
      setBusy(false);
      setTurnstileToken("");
      setTurnstileReset((value) => value + 1);
    }
  }

  return (
    <section className="vg-panel mt-8 p-5 sm:p-6">
      <p className="vg-kicker">Private creator claim</p>
      <h1 className="vg-page-title mt-2">Submission status</h1>

      {error ? (
        <div className="mt-5 rounded-lg border border-tomato/60 bg-tomato/10 p-4">
          <p className="font-semibold">{error}</p>
          <Link href="/create" className="vg-button-secondary mt-4">Create a new grid</Link>
        </div>
      ) : !status ? (
        <p className="mt-5 font-medium text-neutral-600">Checking this claim.</p>
      ) : (
        <div className="mt-5 grid gap-4">
          <div className="rounded-lg border border-line bg-white/70 p-4">
            <p className="text-sm font-semibold text-neutral-500">VibeGrid #{status.puzzleNumber}</p>
            <p className="mt-1 text-2xl font-extrabold">
              {status.withdrawn ? "Withdrawn" : statusLabel(status.status)}
            </p>
            <p className="mt-2 text-sm font-medium text-neutral-600">
              Updated {new Date(status.updatedAt).toLocaleString()}.
            </p>
          </div>

          {status.playPath && (
            <Link href={status.playPath} className="vg-button-primary">Play and share your grid</Link>
          )}

          {status.canWithdraw && !confirmingWithdrawal && (
            <button type="button" onClick={() => setConfirmingWithdrawal(true)} className="vg-button-secondary">
              <Trash2 aria-hidden size={16} />
              Withdraw submission
            </button>
          )}

          {status.canWithdraw && confirmingWithdrawal && (
            <div className="rounded-lg border border-tomato/60 bg-tomato/10 p-4">
              <p className="font-extrabold">Withdraw this pending grid?</p>
              <p className="mt-1 text-sm font-medium text-neutral-600">It will leave the review queue and cannot be published.</p>
              <div className="mt-3 flex flex-wrap gap-2">
                <button type="button" disabled={busy} onClick={() => void withdraw()} className="vg-button-primary bg-tomato">
                  Confirm withdrawal
                </button>
                <button type="button" disabled={busy} onClick={() => setConfirmingWithdrawal(false)} className="vg-button-secondary">
                  Keep submission
                </button>
              </div>
            </div>
          )}

          {status.canAppeal && (
            <form className="grid gap-3 border-t border-line pt-4" onSubmit={submitAppeal}>
              <div>
                <h2 className="text-lg font-extrabold">Ask for a review</h2>
                <p className="mt-1 text-sm font-medium text-neutral-600">Explain why this archived grid should be reinstated.</p>
              </div>
              {appealSent ? (
                <p className="rounded-lg border border-mint/70 bg-mint/20 p-3 text-sm font-semibold">Appeal sent to the moderation queue.</p>
              ) : (
                <>
                  <label className="grid gap-1 text-xs font-semibold text-neutral-600">
                    Contact (optional)
                    <input value={appealContact} onChange={(event) => setAppealContact(event.target.value)} maxLength={200} className="vg-input h-10" />
                  </label>
                  <label className="grid gap-1 text-xs font-semibold text-neutral-600">
                    Message
                    <textarea value={appealMessage} onChange={(event) => setAppealMessage(event.target.value)} maxLength={1000} rows={4} required className="vg-input resize-none py-2" />
                  </label>
                  <TurnstileWidget
                    action="appeal_create"
                    onTokenChange={setTurnstileToken}
                    resetSignal={turnstileReset}
                  />
                  <button type="submit" disabled={busy || !appealMessage.trim()} className="vg-button-primary bg-yolk">
                    <Send aria-hidden size={16} />
                    {busy ? "Sending" : "Send appeal"}
                  </button>
                </>
              )}
            </form>
          )}

          <p className="text-xs font-semibold leading-5 text-neutral-500">Keep this URL private. Anyone with the complete link can manage this submission.</p>
        </div>
      )}
    </section>
  );
}

function statusLabel(status: CreatorPuzzleStatus["status"]) {
  switch (status) {
    case "PENDING": return "Waiting for review";
    case "PUBLISHED": return "Approved";
    case "ARCHIVED": return "Archived";
    default: return "Draft";
  }
}
