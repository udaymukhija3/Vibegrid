"use client";

import { useEffect } from "react";
import { reportClientError } from "@/lib/errorReporting";

// Mounts once in the root layout and forwards uncaught errors and unhandled
// promise rejections to the backend log. Renders nothing.
export function ClientErrorReporter() {
  useEffect(() => {
    function onError(event: ErrorEvent) {
      reportClientError(
        event.message || "Uncaught error",
        event.error instanceof Error ? event.error.stack : undefined
      );
    }
    function onRejection(event: PromiseRejectionEvent) {
      const reason: unknown = event.reason;
      reportClientError(
        reason instanceof Error ? `Unhandled rejection: ${reason.message}` : `Unhandled rejection: ${String(reason)}`,
        reason instanceof Error ? reason.stack : undefined
      );
    }

    window.addEventListener("error", onError);
    window.addEventListener("unhandledrejection", onRejection);
    return () => {
      window.removeEventListener("error", onError);
      window.removeEventListener("unhandledrejection", onRejection);
    };
  }, []);

  return null;
}
