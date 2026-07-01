import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { ArchiveList } from "@/components/ArchiveList";

export default function ArchivePage() {
  return (
    <main className="min-h-screen px-4 py-6 sm:px-6 lg:px-8">
      <div className="mx-auto max-w-3xl">
        <Link
          href="/"
          className="vg-button-secondary"
        >
          <ArrowLeft aria-hidden size={16} />
          Today
        </Link>

        <header className="mt-8 border-b border-line pb-5">
          <p className="text-sm font-semibold text-tomato">Archive</p>
          <h1 className="vg-page-title mt-2">Previous grids</h1>
        </header>

        <ArchiveList />
      </div>
    </main>
  );
}
