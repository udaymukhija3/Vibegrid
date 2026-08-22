import Link from "next/link";
import { ArrowLeft, Users } from "lucide-react";
import { VibeHeader } from "@/components/VibeHeader";

export const metadata = {
  title: "VibeGrid crew history",
  description: "VibeGrid results belong to the private crew that made them."
};

export default function ArchivePage() {
  return (
    <main className="min-h-screen">
      <div className="vg-shell">
        <VibeHeader compact />
        <section className="vg-dark-panel mx-auto mt-12 max-w-3xl">
          <p className="vg-meta text-violet-light">No global leaderboard</p>
          <h1 className="mt-3 text-4xl font-black text-cream sm:text-6xl">History belongs to the crew.</h1>
          <p className="mt-5 max-w-2xl font-semibold leading-8 text-cream/[.62]">
            VibeGrid no longer publishes a global archive of correct answers. Each crew builds its
            own record from the cards it made and the winners it chose.
          </p>
          <div className="mt-7 flex flex-wrap gap-3">
            <Link href="/crews" className="vg-primary-button">
              <Users aria-hidden size={17} />
              Open your crews
            </Link>
            <Link href="/" className="vg-secondary-button">
              <ArrowLeft aria-hidden size={16} />
              Practice
            </Link>
          </div>
        </section>
      </div>
    </main>
  );
}
