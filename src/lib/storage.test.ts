import { afterEach, describe, expect, it, vi } from "vitest";
import {
  readSessionValue,
  readStoredValue,
  safeStorage,
  writeSessionValue,
  writeStoredValue
} from "@/lib/storage";

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

// sessionStorage is guarded separately from localStorage, so it needs its own
// blocked/working windows. Both stores are present here: a test that writes to
// one must be able to prove the other stayed empty.
function blockedSessionWindow() {
  const win = memoryWindow();
  Object.defineProperty(win, "sessionStorage", {
    get() {
      throw new DOMException("The operation is insecure.", "SecurityError");
    }
  });
  return win;
}

function memorySessionWindow() {
  const values = new Map<string, string>();
  return {
    ...memoryWindow(),
    sessionStorage: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => {
        values.set(key, value);
      }
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
// boundary caught it, and the whole game was replaced by an error screen. Every
// storage access it makes must still survive being blocked.
//
// The dialog no longer decides *whether* to open from storage — that comes from
// the server ("has this session ever finished a grid?"), because a browser-local
// flag is wiped by incognito, cache clears, and Safari's cap on script-writable
// storage, which re-prompted regulars. Storage now only remembers "I dismissed
// it during this visit".
describe("dismissal memory (as used by HowToPlay)", () => {
  it("does not throw when session storage is blocked, and stays undismissed", () => {
    vi.stubGlobal("window", blockedSessionWindow());

    expect(writeSessionValue("vibegrid:howToDismissed", "1")).toBe(false);
    // Failing this way reopens the dialog on a route change, which is the safe
    // direction: worse to silently never explain the game.
    expect(readSessionValue("vibegrid:howToDismissed")).toBeNull();
  });

  it("remembers a dismissal for the rest of the visit", () => {
    vi.stubGlobal("window", memorySessionWindow());

    expect(readSessionValue("vibegrid:howToDismissed")).toBeNull();
    expect(writeSessionValue("vibegrid:howToDismissed", "1")).toBe(true);
    expect(readSessionValue("vibegrid:howToDismissed")).toBe("1");
  });

  it("is kept out of localStorage, so it cannot outlive the visit", () => {
    const win = memorySessionWindow();
    vi.stubGlobal("window", win);

    writeSessionValue("vibegrid:howToDismissed", "1");

    expect(readStoredValue("vibegrid:howToDismissed")).toBeNull();
  });
});
