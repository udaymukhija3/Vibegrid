"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { ArrowLeft, Copy, Play, RefreshCw, Share2, Users } from "lucide-react";
import { toast } from "sonner";
import { VibeGridGame } from "@/components/VibeGridGame";
import { useResource } from "@/hooks/useResource";
import { fetchPuzzleById } from "@/lib/api";

const walkthroughSteps = [
  {
    title: "Play the seeded room",
    body: "Use Medium mode to match one vibe at a time. It is the real game loop, not a mock.",
    icon: Play
  },
  {
    title: "Share the result",
    body: "Finish or miss the grid, then copy the spoiler-safe result from the Share button.",
    icon: Share2
  },
  {
    title: "Try a second browser",
    body: "Open this same room link in a private window or another browser to start a separate guest attempt.",
    icon: Users
  }
] as const;

export function DemoWalkthrough() {
  const [room, setRoom] = useState<string | null>(null);

  useEffect(() => {
    setRoom(roomFromPath());
  }, []);

  if (room === null) {
    return <DemoStatus title="Loading demo" message="Finding this room." />;
  }

  return <DemoRoom room={room} />;
}

function DemoRoom({ room }: { room: string }) {
  const puzzleId = `demo-${room}`;
  const loader = useCallback(() => fetchPuzzleById(puzzleId), [puzzleId]);
  const state = useResource(loader, "This demo room is not available.");
  const fallbackRoomUrl = useMemo(
    () => new URL(`/demo/${room}`, process.env.NEXT_PUBLIC_APP_URL ?? "http://localhost:3000").toString(),
    [room]
  );
  const [roomUrl, setRoomUrl] = useState(fallbackRoomUrl);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    const url = new URL(window.location.href);
    url.hash = "";
    setRoomUrl(url.toString());
  }, []);

  async function copyRoomLink() {
    try {
      await writeClipboardText(roomUrl);
      setCopied(true);
      toast.success("Copied demo link.");
    } catch {
      toast.error("Could not copy the demo link.");
    }
  }

  return (
    <div className="grid gap-5">
      <section className="vg-panel mx-auto w-full max-w-6xl p-4 sm:p-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="max-w-2xl">
            <p className="vg-kicker">Public demo</p>
            <h1 className="mt-2 text-3xl font-extrabold leading-tight sm:text-4xl">
              Guided room, no sign-in
            </h1>
            <p className="mt-3 font-medium leading-7 text-neutral-600">
              This seeded room is safe to share. Use the same link in a private window or another
              browser when you want to show a second guest joining from scratch.
            </p>
          </div>

          <div className="grid min-w-0 gap-2 sm:min-w-72">
            <button
              type="button"
              onClick={copyRoomLink}
              className="vg-button-primary"
            >
              <Copy aria-hidden size={16} />
              {copied ? "Copied" : "Copy room link"}
            </button>
            <div className="grid grid-cols-2 gap-2">
              <Link
                href="/demo"
                className="vg-button-secondary"
              >
                <RefreshCw aria-hidden size={15} />
                Fresh demo
              </Link>
              <Link
                href="/"
                className="vg-button-secondary"
              >
                <ArrowLeft aria-hidden size={15} />
                Today
              </Link>
            </div>
            <p
              aria-label="Demo room URL"
              title={roomUrl}
              className="truncate rounded-lg border border-line bg-white px-3 py-2 text-xs font-medium text-neutral-600"
            >
              {roomUrl}
            </p>
          </div>
        </div>

        <div className="mt-5 grid gap-3 md:grid-cols-3">
          {walkthroughSteps.map((step) => {
            const Icon = step.icon;
            return (
              <div key={step.title} className="rounded-lg border border-line bg-white/70 p-3">
                <div className="flex items-center gap-2">
                  <span className="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-yolk/50">
                    <Icon aria-hidden size={16} />
                  </span>
                  <h2 className="text-base font-extrabold">{step.title}</h2>
                </div>
                <p className="mt-2 text-sm font-medium leading-6 text-neutral-600">{step.body}</p>
              </div>
            );
          })}
        </div>
      </section>

      {state.status === "ready" ? (
        <VibeGridGame puzzle={state.data} />
      ) : (
        <DemoStatus
          title={state.status === "loading" ? "Loading demo room" : "Demo room unavailable"}
          message={state.status === "loading" ? "Getting the seeded grid." : state.message}
        />
      )}
    </div>
  );
}

function DemoStatus({ title, message }: { title: string; message: string }) {
  return (
    <div className="mx-auto flex min-h-[24rem] w-full max-w-3xl items-center justify-center">
      <div className="vg-panel w-full p-6 text-center">
        <h1 className="text-3xl font-extrabold">{title}</h1>
        <p className="mt-3 font-medium text-neutral-600">{message}</p>
        {title === "Demo room unavailable" && (
          <Link
            href="/demo"
            className="vg-button-primary mt-4"
          >
            <RefreshCw aria-hidden size={16} />
            Start a fresh demo
          </Link>
        )}
      </div>
    </div>
  );
}

function roomFromPath() {
  const [, room = ""] = window.location.pathname.match(/^\/demo\/([^/]+)/) ?? [];
  return decodeURIComponent(room);
}

async function writeClipboardText(text: string) {
  if (copySelectedText(text)) {
    return;
  }

  if (navigator.clipboard?.writeText) {
    await withTimeout(navigator.clipboard.writeText(text), 600);
    return;
  }

  throw new Error("Clipboard unavailable.");
}

function copySelectedText(text: string) {
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.top = "-1000px";
  textarea.style.opacity = "0";
  document.body.append(textarea);
  textarea.select();

  try {
    return document.execCommand("copy");
  } finally {
    textarea.remove();
  }
}

function withTimeout<T>(promise: Promise<T>, timeoutMs: number) {
  return new Promise<T>((resolve, reject) => {
    const timeout = window.setTimeout(() => reject(new Error("Clipboard timed out.")), timeoutMs);
    promise.then(resolve, reject).finally(() => window.clearTimeout(timeout));
  });
}
