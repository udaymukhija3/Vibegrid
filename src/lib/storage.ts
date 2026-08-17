// Browser storage helpers that never throw.
//
// In Safari private browsing, "block all cookies", and other strict privacy
// modes, `window.localStorage` is not merely empty — the *getter itself* raises
// a SecurityError. An unguarded read inside a render or an effect therefore
// escapes to the Next.js error boundary and replaces the whole app with the
// error screen, so every access in the client bundle goes through this module
// and degrades to an in-memory-only session instead.

// safeStorage returns localStorage, or null when it is unavailable. Callers that
// need the raw Storage object (key/length iteration, removeItem) use this and
// wrap their own calls; single-value callers should prefer the read/write
// helpers below.
export function safeStorage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

// readStoredValue answers "what did we persist?" with null meaning "nothing, or
// we are not allowed to know". Reads can fail independently of the getter (a
// storage partition revoked mid-session), so the call is guarded too.
export function readStoredValue(key: string): string | null {
  try {
    return safeStorage()?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

// writeStoredValue reports whether the value actually persisted. Writes fail on
// their own even when storage reads fine — quota exhaustion, or a private-mode
// quota of zero — and losing persistence is never worth losing the page over.
export function writeStoredValue(key: string, value: string): boolean {
  const storage = safeStorage();
  if (!storage) {
    return false;
  }
  try {
    storage.setItem(key, value);
    return true;
  } catch {
    return false;
  }
}

// sessionStorage, guarded the same way. Used for state that should survive a
// client-side route change but deliberately not the visit: "I dismissed this
// dialog" belongs here, because whether it should open *next* time is decided
// by durable server state, not by what this browser remembers.
function safeSessionStorage(): Storage | null {
  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

export function readSessionValue(key: string): string | null {
  try {
    return safeSessionStorage()?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

export function writeSessionValue(key: string, value: string): boolean {
  const storage = safeSessionStorage();
  if (!storage) {
    return false;
  }
  try {
    storage.setItem(key, value);
    return true;
  } catch {
    return false;
  }
}
