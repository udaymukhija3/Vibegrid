import Link from "next/link";
import { ArrowLeft, Sparkles } from "lucide-react";
import { VibeHeader } from "@/components/VibeHeader";

export const metadata = {
  title: "Make a VibeGrid crew",
  description: "Players make vibe cards inside private crews; the daily fragment board stays editorial."
};

export default function CreatePage() {
  return (
    <main className="min-h-screen">
      <div className="vg-shell">
        <VibeHeader compact />
        <section className="vg-dark-panel mx-auto mt-12 max-w-3xl">
          <p className="vg-meta text-lime">The creation moved into the game</p>
          <h1 className="mt-3 text-4xl font-black text-cream sm:text-6xl">You make the answer now.</h1>
          <p className="mt-5 max-w-2xl font-semibold leading-8 text-cream/[.62]">
            There are no hidden categories to author. VibeGrid supplies twelve human-written
            fragments; every player chooses four and names the interpretation their crew will judge.
          </p>
          <div className="mt-7 flex flex-wrap gap-3">
            <Link href="/crews" className="vg-primary-button">
              <Sparkles aria-hidden size={17} />
              Make a crew
            </Link>
            <Link href="/" className="vg-secondary-button">
              <ArrowLeft aria-hidden size={16} />
              Try practice
            </Link>
          </div>
        </section>
      </div>
    </main>
  );
}
