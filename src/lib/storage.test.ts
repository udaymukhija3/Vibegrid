import { afterEach, describe, expect, it, vi } from "vitest";
import { readStoredValue, safeStorage, writeStoredValue } from "@/lib/storage";

// Safari private browsing and "block all cookies" do not hand back an empty
// store — they throw from the `localStorage` property itself, before any method
// is called. This window reproduces exactly that.
function blockedWindow() {
  const win = {};
  Object.defineProperty(win, "localStorage", {
    get() {
      throw new DOMException("The operation is insecure.", "SecurityError");
    }
  });
  return win;
}

function memoryWindow(overrides: Partial<Storage> = {}) {
  const values = new Map<string, string>();
  return {
    localStorage: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => {
        values.set(key, value);
      },
      ...overrides
    }
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("safeStorage", () => {
  it("returns null instead of propagating a blocked localStorage getter", () => {
    vi.stubGlobal("window", blockedWindow());

    expect(safeStorage()).toBeNull();
  });

  it("returns null when there is no window at all", () => {
    vi.stubGlobal("window", undefined);

    expect(safeStorage()).toBeNull();
  });
});

describe("readStoredValue", () => {
  it("reads a stored value when storage works", () => {
    vi.stubGlobal("window", memoryWindow());
    writeStoredValue("vibegrid:test", "1");

    expect(readStoredValue("vibegrid:test")).toBe("1");
  });

  it("reports a missing key as null", () => {
    vi.stubGlobal("window", memoryWindow());

    expect(readStoredValue("vibegrid:test")).toBeNull();
  });

  it("returns null when storage is blocked", () => {
    vi.stubGlobal("window", blockedWindow());

    expect(readStoredValue("vibegrid:test")).toBeNull();
  });

  it("returns null when getItem itself throws", () => {
    vi.stubGlobal(
      "window",
      memoryWindow({
        getItem: () => {
          throw new DOMException("The operation is insecure.", "SecurityError");
        }
      })
    );

    expect(readStoredValue("vibegrid:test")).toBeNull();
  });
});

describe("writeStoredValue", () => {
  it("reports success when the value persists", () => {
    vi.stubGlobal("window", memoryWindow());

    expect(writeStoredValue("vibegrid:test", "1")).toBe(true);
  });

  it("reports failure when storage is blocked", () => {
    vi.stubGlobal("window", blockedWindow());

    expect(writeStoredValue("vibegrid:test", "1")).toBe(false);
  });

  it("reports failure when setItem throws on its own (quota exceeded)", () => {
    vi.stubGlobal(
      "window",
      memoryWindow({
        setItem: () => {
          throw new DOMException("The quota has been exceeded.", "QuotaExceededError");
        }
      })
    );

    expect(writeStoredValue("vibegrid:test", "1")).toBe(false);
  });
});

// Regression: HowToPlay used to read and write window.localStorage directly, so
// on a storage-blocked browser its mount effect threw a SecurityError, the error
// boundary caught it, and the whole game was replaced by an error screen. The
// first-run check must now survive every storage failure and simply treat the
// visitor as new.
describe("first-run check (as used by HowToPlay)", () => {
  function firstRunCheck(key: string) {
    const unseen = !readStoredValue(key);
    if (unseen) {
      writeStoredValue(key, "1");
    }
    return unseen;
  }

  it("does not throw when storage is blocked, and keeps showing the explainer", () => {
    vi.stubGlobal("window", blockedWindow());

    expect(firstRunCheck("vibegrid:seenHowTo")).toBe(true);
    expect(firstRunCheck("vibegrid:seenHowTo")).toBe(true);
  });

  it("shows the explainer once when storage works", () => {
    vi.stubGlobal("window", memoryWindow());

    expect(firstRunCheck("vibegrid:seenHowTo")).toBe(true);
    expect(firstRunCheck("vibegrid:seenHowTo")).toBe(false);
  });
});
