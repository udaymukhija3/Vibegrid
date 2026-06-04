import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { CreatePuzzle } from "@/components/CreatePuzzle";

export const metadata = {
  title: "Make a VibeGrid",
  description: "Build your own daily-style vibe puzzle and share it with friends."
};

export default function CreatePage() {
  return (
    <main className="min-h-screen px-4 py-6 sm:px-6 lg:px-8">
      <div className="mx-auto max-w-4xl">
        <Link
          href="/"
          className="inline-flex h-10 items-center gap-2 rounded border border-ink bg-white px-3 text-sm font-semibold shadow-[0_4px_0_#171717]"
        >
          <ArrowLeft aria-hidden size={16} />
          Today
        </Link>

        <header className="mt-8 border-b-4 border-ink pb-5">
          <p className="text-sm font-bold text-plum">Make your own</p>
          <h1 className="mt-2 text-4xl font-black sm:text-5xl">Build a VibeGrid</h1>
          <p className="mt-3 max-w-2xl font-semibold text-neutral-600">
            Four groups of four. Name the vibes, fill the tiles, and get a private link to send your
            friends. It will not appear in the daily puzzle or the archive.
          </p>
        </header>

        <CreatePuzzle />
      </div>
    </main>
  );
}
