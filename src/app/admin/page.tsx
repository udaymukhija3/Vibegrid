import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { AdminDesk } from "@/components/AdminDesk";

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
          <p className="text-sm font-bold text-plum">Editor Desk</p>
          <h1 className="mt-2 text-4xl font-black sm:text-5xl">Puzzle pipeline</h1>
        </header>

        <AdminDesk />
      </div>
    </main>
  );
}
