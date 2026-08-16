import { afterEach, describe, expect, it, vi } from "vitest";
import { apiFetch, COLD_START_RETRY_TIMEOUT_MS } from "@/lib/http";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function stalledFetch(init?: RequestInit) {
  return new Promise<Response>((_resolve, reject) => {
    init?.signal?.addEventListener("abort", () => {
      reject(Object.assign(new Error("aborted"), { name: "AbortError" }));
    });
  });
}

describe("apiFetch", () => {
  it("retries a timed-out read once with the cold-start budget", async () => {
    vi.useFakeTimers();
    let calls = 0;
    globalThis.fetch = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      calls += 1;
      if (calls === 1) {
        return stalledFetch(init);
      }
      return Promise.resolve(new Response("{}", { status: 200 }));
    }) as typeof fetch;

    const request = apiFetch("/slow", {}, 10);
    await vi.advanceTimersByTimeAsync(10);

    const response = await request;
    expect(response.status).toBe(200);
    expect(calls).toBe(2);
  });

  it("fails when the cold-start retry also times out", async () => {
    vi.useFakeTimers();
    globalThis.fetch = vi.fn((_input: RequestInfo | URL, init?: RequestInit) =>
      stalledFetch(init)
    ) as typeof fetch;

    const request = expect(apiFetch("/slow", {}, 10)).rejects.toMatchObject({
      message: "Request timed out.",
      name: "ApiError",
      timedOut: true
    });
    await vi.advanceTimersByTimeAsync(10);
    await vi.advanceTimersByTimeAsync(COLD_START_RETRY_TIMEOUT_MS);

    await request;
    expect(globalThis.fetch).toHaveBeenCalledTimes(2);
  });

  it("does not retry mutating requests", async () => {
    vi.useFakeTimers();
    globalThis.fetch = vi.fn((_input: RequestInfo | URL, init?: RequestInit) =>
      stalledFetch(init)
    ) as typeof fetch;

    const request = expect(apiFetch("/slow", { method: "POST" }, 10)).rejects.toMatchObject({
      message: "Request timed out.",
      name: "ApiError"
    });
    await vi.advanceTimersByTimeAsync(10);

    await request;
    expect(globalThis.fetch).toHaveBeenCalledTimes(1);
  });

  it("retries an idempotency-keyed mutation with the same key", async () => {
    vi.useFakeTimers();
    const keys: string[] = [];
    let calls = 0;
    globalThis.fetch = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      calls += 1;
      keys.push(new Headers(init?.headers).get("Idempotency-Key") ?? "");
      if (calls === 1) {
        return stalledFetch(init);
      }
      return Promise.resolve(new Response("{}", { status: 200 }));
    }) as typeof fetch;

    const request = apiFetch(
      "/safe-mutation",
      {
        method: "POST",
        headers: { "Idempotency-Key": "retry-safe-key" }
      },
      10
    );
    await vi.advanceTimersByTimeAsync(10);

    const response = await request;
    expect(response.status).toBe(200);
    expect(keys).toEqual(["retry-safe-key", "retry-safe-key"]);
  });
});
