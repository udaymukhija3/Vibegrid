import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const originalFetch = globalThis.fetch;

async function loadReporter(sendBeacon: ((url: string, data?: BodyInit | null) => boolean) | undefined) {
  vi.resetModules();
  vi.stubGlobal("window", { location: { href: "http://localhost:3000/p/abc" } });
  vi.stubGlobal("navigator", sendBeacon ? { sendBeacon } : {});
  const reporterModule = await import("@/lib/errorReporting");
  return reporterModule.reportClientError;
}

beforeEach(() => {
  globalThis.fetch = vi.fn(() => Promise.resolve(new Response(null, { status: 202 }))) as typeof fetch;
});

afterEach(() => {
  vi.unstubAllGlobals();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

describe("reportClientError", () => {
  it("sends via beacon with a JSON payload and dedupes repeats", async () => {
    const beacon = vi.fn(() => true);
    const report = await loadReporter(beacon);

    report("TypeError: boom", "at play");
    report("TypeError: boom", "at play");

    expect(beacon).toHaveBeenCalledTimes(1);
    const [url, blob] = beacon.mock.calls[0] as unknown as [string, Blob];
    expect(url).toBe("/api/client-errors");
    expect(blob.type).toBe("application/json");
    const payload = JSON.parse(await blob.text());
    expect(payload).toMatchObject({
      message: "TypeError: boom",
      stack: "at play",
      url: "http://localhost:3000/p/abc"
    });
  });

  it("caps reports per page load", async () => {
    const beacon = vi.fn(() => true);
    const report = await loadReporter(beacon);

    for (let index = 0; index < 10; index += 1) {
      report(`distinct error ${index}`);
    }
    expect(beacon).toHaveBeenCalledTimes(5);
  });

  it("falls back to keepalive fetch when beacons are unavailable and never throws", async () => {
    const report = await loadReporter(undefined);

    expect(() => report("fetch fallback error")).not.toThrow();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/client-errors",
      expect.objectContaining({ method: "POST", keepalive: true })
    );

    globalThis.fetch = vi.fn(() => {
      throw new Error("network gone");
    }) as typeof fetch;
    expect(() => report("second distinct error")).not.toThrow();
  });
});
