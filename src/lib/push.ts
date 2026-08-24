import { ApiError, apiFetch } from "@/lib/http";

export type PushConfig = {
  enabled: boolean;
  publicKey?: string;
};

export type PushState = "unsupported" | "unavailable" | "denied" | "off" | "on";

const SERVICE_WORKER_PATH = "/sw.js";

/**
 * Web Push wants the VAPID key as raw bytes, but it travels as base64url. The
 * padding and the two substituted characters both have to be put back before
 * atob will read it.
 */
function decodeVapidKey(base64: string): ArrayBuffer {
  const padded = base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), "=");
  const raw = atob(padded.replace(/-/g, "+").replace(/_/g, "/"));
  // Build the ArrayBuffer directly: applicationServerKey takes a BufferSource,
  // and a bare Uint8Array is not one under the current lib types.
  const buffer = new ArrayBuffer(raw.length);
  const bytes = new Uint8Array(buffer);
  for (let index = 0; index < raw.length; index += 1) {
    bytes[index] = raw.charCodeAt(index);
  }
  return buffer;
}

/**
 * Push needs a service worker, a Push API, and a Notification API. Private
 * windows and older iOS Safari have some of these and not others, so every one
 * is checked rather than assuming they arrive together.
 */
export function pushSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "Notification" in window
  );
}

export async function fetchPushConfig(): Promise<PushConfig> {
  const response = await apiFetch("/api/push/config", { credentials: "include" });
  if (!response.ok) {
    return { enabled: false };
  }
  const payload: unknown = await response.json().catch(() => null);
  if (!payload || typeof payload !== "object" || typeof (payload as PushConfig).enabled !== "boolean") {
    return { enabled: false };
  }
  return payload as PushConfig;
}

/** Reports what the browser currently thinks, without prompting for anything. */
export async function currentPushState(config: PushConfig): Promise<PushState> {
  if (!pushSupported()) {
    return "unsupported";
  }
  if (!config.enabled || !config.publicKey) {
    return "unavailable";
  }
  if (Notification.permission === "denied") {
    return "denied";
  }
  const registration = await navigator.serviceWorker.getRegistration(SERVICE_WORKER_PATH);
  const subscription = await registration?.pushManager.getSubscription();
  return subscription ? "on" : "off";
}

/**
 * Registers the worker, asks for permission, and tells the server where to
 * reach this browser. Permission is only requested from a real click — asking
 * on load is how a site gets permanently blocked.
 */
export async function enablePush(config: PushConfig): Promise<PushState> {
  if (!pushSupported() || !config.enabled || !config.publicKey) {
    return "unavailable";
  }
  const permission = await Notification.requestPermission();
  if (permission !== "granted") {
    return permission === "denied" ? "denied" : "off";
  }

  const registration = await navigator.serviceWorker.register(SERVICE_WORKER_PATH);
  await navigator.serviceWorker.ready;

  const subscription =
    (await registration.pushManager.getSubscription()) ??
    (await registration.pushManager.subscribe({
      // Chrome refuses a subscription that could deliver silently, so every
      // reminder this product sends is a visible one.
      userVisibleOnly: true,
      applicationServerKey: decodeVapidKey(config.publicKey)
    }));

  const payload = subscription.toJSON();
  const response = await apiFetch("/api/push/subscribe", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      endpoint: payload.endpoint,
      keys: { p256dh: payload.keys?.p256dh ?? "", auth: payload.keys?.auth ?? "" }
    })
  });
  if (!response.ok) {
    // Leaving a browser subscribed that the server cannot reach would look like
    // reminders are on while none arrive. Undo it and report honestly.
    await subscription.unsubscribe().catch(() => undefined);
    throw new ApiError("Could not turn on reminders.", response.status);
  }
  return "on";
}

export async function disablePush(): Promise<PushState> {
  if (!pushSupported()) {
    return "unsupported";
  }
  const registration = await navigator.serviceWorker.getRegistration(SERVICE_WORKER_PATH);
  const subscription = await registration?.pushManager.getSubscription();
  if (!subscription) {
    return "off";
  }
  const endpoint = subscription.endpoint;
  await subscription.unsubscribe().catch(() => undefined);
  await apiFetch("/api/push/unsubscribe", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ endpoint })
  }).catch(() => undefined);
  return "off";
}
