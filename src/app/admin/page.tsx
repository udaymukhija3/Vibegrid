import Link from "next/link";
import { ArrowLeft, CalendarDays, Eye, FilePenLine } from "lucide-react";
import { AdminPuzzlePipeline } from "@/components/AdminPuzzlePipeline";

export default function AdminPage() {
  return (
    <main className="min-h-screen px-4 py-6 sm:px-6 lg:px-8">
      <div className="mx-auto max-w-5xl">
        <Link
          href="/"
          className="inline-flex h-10 items-center gap-2 rounded border border-ink bg-white px-3 text-sm font-semibold shadow-[0_4px_0_#171717]"
        >
          <ArrowLeft aria-hidden size={16} />
          Today
        </Link>

        <header className="mt-8 border-b-4 border-ink pb-5">
          <p className="text-sm font-bold uppercase tracking-[0.16em] text-plum">Editor Desk</p>
          <h1 className="mt-2 text-4xl font-black sm:text-5xl">Puzzle pipeline</h1>
        </header>

        <section className="mt-6 grid gap-4 lg:grid-cols-[1fr_320px]">
          <div className="rounded border-2 border-ink bg-white p-4 shadow-[0_6px_0_#171717]">
            <div className="grid grid-cols-[auto_1fr_auto] items-center gap-3 border-b border-neutral-200 pb-3 text-sm font-black uppercase tracking-[0.12em] text-neutral-500">
              <span>#</span>
              <span>Puzzle</span>
              <span>Status</span>
            </div>

            <AdminPuzzlePipeline />
          </div>

          <aside className="rounded border-2 border-ink bg-ink p-4 text-white shadow-[0_6px_0_#2ec4b6]">
            <h2 className="text-lg font-black">Next admin build</h2>
            <div className="mt-4 grid gap-3 text-sm">
              <div className="flex gap-3">
                <FilePenLine aria-hidden className="mt-0.5 shrink-0 text-yolk" size={18} />
                <p>Draft form for four named groups and sixteen unique tiles.</p>
              </div>
              <div className="flex gap-3">
                <Eye aria-hidden className="mt-0.5 shrink-0 text-mint" size={18} />
                <p>Preview mode that hides solution metadata from the player view.</p>
              </div>
              <div className="flex gap-3">
                <CalendarDays aria-hidden className="mt-0.5 shrink-0 text-tomato" size={18} />
                <p>Publish scheduler with one puzzle per calendar date.</p>
              </div>
            </div>
          </aside>
        </section>
      </div>
    </main>
  );
}
