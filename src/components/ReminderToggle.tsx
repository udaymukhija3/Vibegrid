"use client";

import { useCallback, useEffect, useState } from "react";
import { Bell, BellOff, BellRing } from "lucide-react";
import { toast } from "sonner";
import {
  currentPushState,
  disablePush,
  enablePush,
  fetchPushConfig,
  type PushConfig,
  type PushState
} from "@/lib/push";

/**
 * Reminders are the only re-engagement path this product has: no accounts, no
 * email, and a round that does not reveal until two days after you play it.
 *
 * The control renders nothing at all unless the browser can actually do push
 * and the server has VAPID keys configured — an offer that cannot be honoured
 * is worse than no offer. Permission is requested from the click and never on
 * load, because a prompt nobody asked for is how a site gets blocked for good.
 */
export function ReminderToggle() {
  const [config, setConfig] = useState<PushConfig | null>(null);
  const [state, setState] = useState<PushState>("unavailable");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const loaded = await fetchPushConfig().catch(() => ({ enabled: false }) as PushConfig);
      if (cancelled) {
        return;
      }
      setConfig(loaded);
      setState(await currentPushState(loaded).catch(() => "unavailable" as PushState));
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const toggle = useCallback(async () => {
    if (!config || busy) {
      return;
    }
    setBusy(true);
    try {
      if (state === "on") {
        setState(await disablePush());
        toast.success("Reminders off.");
      } else {
        const next = await enablePush(config);
        setState(next);
        if (next === "on") {
          toast.success("Reminders on. We'll nudge you when the crew needs you.");
        } else if (next === "denied") {
          toast.error("Your browser is blocking notifications for this site.");
        }
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not change reminders.");
    } finally {
      setBusy(false);
    }
  }, [busy, config, state]);

  if (state === "unsupported" || state === "unavailable") {
    return null;
  }

  if (state === "denied") {
    return (
      <span className="vg-secondary-button cursor-not-allowed opacity-60" title="Notifications are blocked in your browser settings.">
        <BellOff aria-hidden size={17} />
        Reminders blocked
      </span>
    );
  }

  return (
    <button
      type="button"
      onClick={() => void toggle()}
      disabled={busy}
      aria-pressed={state === "on"}
      className="vg-secondary-button"
    >
      {state === "on" ? <BellRing aria-hidden size={17} /> : <Bell aria-hidden size={17} />}
      {busy ? "Just a sec…" : state === "on" ? "Reminders on" : "Remind me"}
    </button>
  );
}
