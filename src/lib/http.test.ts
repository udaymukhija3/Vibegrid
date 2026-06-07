import { afterEach, describe, expect, it, vi } from "vitest";
import { apiFetch } from "@/lib/http";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("apiFetch", () => {
  it("aborts stalled requests", async () => {
    vi.useFakeTimers();
    globalThis.fetch = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      return new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener("abort", () => {
          reject(Object.assign(new Error("aborted"), { name: "AbortError" }));
        });
      });
    }) as typeof fetch;

    const request = expect(apiFetch("/slow", {}, 10)).rejects.toMatchObject({
      message: "Request timed out.",
      name: "ApiError"
    });
    await vi.advanceTimersByTimeAsync(10);

    await request;
  });
});
