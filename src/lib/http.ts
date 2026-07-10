export const API_TIMEOUT_MS = 8000;

// Free-tier hosts spin down when idle and take ~20-30s to cold-start, which is
// longer than the normal request budget. When an idempotent read times out, one
// retry with this window lets the instance finish waking instead of stranding
// the player on a dead loading screen.
export const COLD_START_RETRY_TIMEOUT_MS = 45000;

export class ApiError extends Error {
  status?: number;
  timedOut: boolean;

  constructor(message: string, status?: number, timedOut = false) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.timedOut = timedOut;
  }
}

export async function apiFetch(input: RequestInfo | URL, init: RequestInit = {}, timeoutMs = API_TIMEOUT_MS) {
  try {
    return await fetchWithTimeout(input, init, timeoutMs);
  } catch (error) {
    if (!shouldRetryAfterTimeout(error, init)) {
      throw error;
    }
    return fetchWithTimeout(input, init, COLD_START_RETRY_TIMEOUT_MS);
  }
}

// Only reads are retried: they are idempotent and safe to reissue. Mutating
// requests keep single-shot semantics so retry policy stays with their callers
// (guesses already carry idempotency keys and manage their own retries).
function shouldRetryAfterTimeout(error: unknown, init: RequestInit) {
  if (!(error instanceof ApiError) || !error.timedOut) {
    return false;
  }
  const method = (init.method ?? "GET").toUpperCase();
  return method === "GET" && !init.signal?.aborted;
}

async function fetchWithTimeout(input: RequestInfo | URL, init: RequestInit, timeoutMs: number) {
  const controller = new AbortController();
  const timeout = globalThis.setTimeout(() => controller.abort(), timeoutMs);

  try {
    const response = await fetch(input, {
      ...init,
      signal: composeAbortSignals(init.signal, controller.signal)
    });
    return response;
  } catch (error) {
    if (isAbortError(error)) {
      throw new ApiError("Request timed out.", undefined, true);
    }
    throw error;
  } finally {
    globalThis.clearTimeout(timeout);
  }
}

function isAbortError(error: unknown) {
  return typeof error === "object" && error !== null && "name" in error && error.name === "AbortError";
}

function composeAbortSignals(left: AbortSignal | null | undefined, right: AbortSignal) {
  if (!left) {
    return right;
  }

  const controller = new AbortController();
  function abort() {
    controller.abort();
  }

  if (left.aborted || right.aborted) {
    controller.abort();
    return controller.signal;
  }

  left.addEventListener("abort", abort, { once: true });
  right.addEventListener("abort", abort, { once: true });
  return controller.signal;
}
