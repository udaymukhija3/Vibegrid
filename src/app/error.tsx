"use client";

import { useEffect } from "react";
import Image from "next/image";
import { RotateCcw } from "lucide-react";
import { reportClientError } from "@/lib/errorReporting";

export default function Error({
  error,
  reset
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    reportClientError(`Route error boundary: ${error.message}`, error.stack);
  }, [error]);

  return (
    <main className="min-h-screen px-4 py-5 sm:px-6 lg:px-8">
      <div className="mx-auto flex min-h-[calc(100vh-2.5rem)] max-w-3xl items-center justify-center">
        <div className="vg-panel w-full p-6 text-center">
          <Image src="/vibegrid-mark.svg" width={48} height={48} alt="" className="mx-auto rounded" priority />
          <h1 className="mt-3 text-3xl font-extrabold">VibeGrid</h1>
          <p className="mt-3 font-medium text-neutral-600">The grid could not finish loading.</p>
          <button
            type="button"
            onClick={reset}
            className="vg-button-primary mt-4"
          >
            <RotateCcw aria-hidden size={18} />
            Try again
          </button>
        </div>
      </div>
    </main>
  );
}
