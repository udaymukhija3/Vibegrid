const MAX_REPORTS_PER_PAGE_LOAD = 5;
const MAX_FIELD_LENGTH = 2000;

let sentCount = 0;
const seenMessages = new Set<string>();

// reportClientError ships a browser error to the backend log (POST
// /api/client-errors) so frontend breakage is visible in the server's log
// stream. Best-effort by design: capped per page load, deduped by message,
// and never allowed to throw — an error reporter that errors helps no one.
export function reportClientError(message: string, stack?: string) {
  try {
    const trimmed = message?.trim();
    if (!trimmed || sentCount >= MAX_REPORTS_PER_PAGE_LOAD || seenMessages.has(trimmed)) {
      return;
    }
    seenMessages.add(trimmed);
    sentCount += 1;

    const body = JSON.stringify({
      message: trimmed.slice(0, MAX_FIELD_LENGTH),
      ...(stack ? { stack: stack.slice(0, MAX_FIELD_LENGTH) } : {}),
      url: window.location.href.slice(0, MAX_FIELD_LENGTH)
    });

    // sendBeacon survives page unload; the Blob carries the JSON content type
    // the API requires. Fall back to a keepalive fetch where beacons are
    // unavailable or refused.
    const blob = new Blob([body], { type: "application/json" });
    if (typeof navigator.sendBeacon === "function" && navigator.sendBeacon("/api/client-errors", blob)) {
      return;
    }
    void fetch("/api/client-errors", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body,
      keepalive: true
    }).catch(() => {});
  } catch {
    // Never let diagnostics become a second error.
  }
}
