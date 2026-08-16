"use client";

import { useEffect, useRef, useState } from "react";
import { fetchPublicConfig } from "@/lib/api";

type TurnstileAPI = {
  render: (container: HTMLElement, options: Record<string, unknown>) => string;
  reset: (widgetId: string) => void;
  remove: (widgetId: string) => void;
};

declare global {
  interface Window {
    turnstile?: TurnstileAPI;
  }
}

let scriptPromise: Promise<TurnstileAPI> | null = null;

function loadTurnstile(): Promise<TurnstileAPI> {
  if (window.turnstile) {
    return Promise.resolve(window.turnstile);
  }
  if (scriptPromise) {
    return scriptPromise;
  }
  scriptPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>("script[data-vibegrid-turnstile]");
    const script = existing ?? document.createElement("script");
    const loaded = () => window.turnstile ? resolve(window.turnstile) : reject(new Error("Turnstile did not load."));
    script.addEventListener("load", loaded, { once: true });
    script.addEventListener("error", () => reject(new Error("Turnstile could not load.")), { once: true });
    if (!existing) {
      script.src = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
      script.async = true;
      script.defer = true;
      script.dataset.vibegridTurnstile = "true";
      document.head.appendChild(script);
    }
  });
  return scriptPromise;
}

export function TurnstileWidget({
  action,
  onTokenChange,
  resetSignal
}: {
  action: "community_create" | "report_create" | "appeal_create";
  onTokenChange: (token: string) => void;
  resetSignal: number;
}) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const widgetRef = useRef<string | null>(null);
  const localBypassRef = useRef(false);
  const [state, setState] = useState<"loading" | "ready" | "local" | "error">("loading");

  useEffect(() => {
    let cancelled = false;
    fetchPublicConfig()
      .then(async ({ turnstileSiteKey }) => {
        if (cancelled) return;
        if (!turnstileSiteKey) {
          localBypassRef.current = true;
          setState("local");
          onTokenChange("local-development-bypass");
          return;
        }
        const turnstile = await loadTurnstile();
        if (cancelled || !containerRef.current) return;
        widgetRef.current = turnstile.render(containerRef.current, {
          sitekey: turnstileSiteKey,
          action,
          theme: "light",
          size: "flexible",
          callback: (token: string) => {
            setState("ready");
            onTokenChange(token);
          },
          "expired-callback": () => onTokenChange(""),
          "error-callback": () => {
            setState("error");
            onTokenChange("");
          }
        });
      })
      .catch(() => {
        if (!cancelled) {
          setState("error");
          onTokenChange("");
        }
      });
    return () => {
      cancelled = true;
      if (widgetRef.current && window.turnstile) {
        window.turnstile.remove(widgetRef.current);
        widgetRef.current = null;
      }
    };
  }, [action, onTokenChange]);

  useEffect(() => {
    if (localBypassRef.current) {
      onTokenChange("local-development-bypass");
      return;
    }
    if (widgetRef.current && window.turnstile) {
      window.turnstile.reset(widgetRef.current);
      onTokenChange("");
    }
  }, [resetSignal, onTokenChange]);

  return (
    <div className="grid gap-1">
      <div ref={containerRef} />
      {state === "loading" && <p className="text-xs font-semibold text-neutral-500">Loading bot check…</p>}
      {state === "local" && <p className="text-xs font-semibold text-neutral-500">Bot check is disabled in local development.</p>}
      {state === "error" && <p role="alert" className="text-xs font-semibold text-tomato">Bot check could not load. Check your connection and try again.</p>}
    </div>
  );
}
