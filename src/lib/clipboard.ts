// Copying a link is the whole invite loop — crew invites, share links, demo
// rooms — so it has to work on the browsers people actually share from.
//
// The execCommand path runs first on purpose: Safari (and iOS in particular)
// only grants navigator.clipboard inside a short user-gesture window and can
// leave the promise pending indefinitely when it declines, which would hang the
// button forever. The legacy path is synchronous and either works or reports
// false immediately, so it is the reliable attempt; the async API is the
// fallback, and it is raced against a timeout so a stalled permission prompt
// surfaces as an error the caller can show instead of a dead button.
export async function writeClipboardText(text: string) {
  if (copySelectedText(text)) {
    return;
  }

  if (navigator.clipboard?.writeText) {
    await withTimeout(navigator.clipboard.writeText(text), 600);
    return;
  }

  throw new Error("Clipboard unavailable.");
}

function copySelectedText(text: string) {
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.top = "-1000px";
  textarea.style.opacity = "0";
  document.body.append(textarea);
  textarea.select();

  try {
    return document.execCommand("copy");
  } finally {
    textarea.remove();
  }
}

function withTimeout<T>(promise: Promise<T>, timeoutMs: number) {
  return new Promise<T>((resolve, reject) => {
    const timeout = window.setTimeout(() => reject(new Error("Clipboard timed out.")), timeoutMs);
    promise.then(resolve, reject).finally(() => window.clearTimeout(timeout));
  });
}
