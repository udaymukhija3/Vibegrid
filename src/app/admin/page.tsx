import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { VibeBoardDesk } from "@/components/VibeBoardDesk";

export const metadata = {
  title: "Board room · VibeGrid",
  robots: { index: false, follow: false }
};

export default function AdminPage() {
  return (
    <main className="vg-shell">
      <div className="mx-auto max-w-6xl">
        <Link
          href="/"
          className="vg-button vg-button-quiet"
        >
          <ArrowLeft aria-hidden size={16} />
          Today
        </Link>

        <header className="mt-10 max-w-4xl">
          <p className="vg-meta text-lime">VibeGrid / board room</p>
          <h1 className="mt-4 text-5xl font-black tracking-[-0.06em] sm:text-7xl">Set the constraint.<br />Leave room for people.</h1>
        </header>

        <VibeBoardDesk />
      </div>
    </main>
  );
}
