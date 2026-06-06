"use client";

import Image from "next/image";
import { RotateCcw } from "lucide-react";

export default function Error({ reset }: { reset: () => void }) {
  return (
    <main className="min-h-screen px-4 py-5 sm:px-6 lg:px-8">
      <div className="mx-auto flex min-h-[calc(100vh-2.5rem)] max-w-3xl items-center justify-center">
        <div className="w-full rounded border-2 border-ink bg-white p-5 text-center shadow-[0_6px_0_#171717]">
          <Image src="/vibegrid-mark.svg" width={48} height={48} alt="" className="mx-auto rounded" priority />
          <h1 className="mt-3 text-3xl font-black">VibeGrid</h1>
          <p className="mt-3 font-semibold text-neutral-600">The grid could not finish loading.</p>
          <button
            type="button"
            onClick={reset}
            className="mt-4 inline-flex h-11 items-center justify-center gap-2 rounded border-2 border-ink bg-mint px-4 font-black shadow-[0_4px_0_#171717]"
          >
            <RotateCcw aria-hidden size={18} />
            Try again
          </button>
        </div>
      </div>
    </main>
  );
}
