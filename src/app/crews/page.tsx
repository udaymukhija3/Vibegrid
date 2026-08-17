import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { MyCrews } from "@/components/MyCrews";

export const metadata = {
  title: "Your VibeGrid crews",
  description: "Make a crew, invite your friends, and race the same daily grid."
};

export default function CrewsPage() {
  return (
    <main className="min-h-screen px-4 py-6 sm:px-6 lg:px-8">
      <div className="mx-auto max-w-3xl">
        <Link href="/" className="vg-button-secondary">
          <ArrowLeft aria-hidden size={16} />
          Today
        </Link>

        <header className="mt-8 border-b border-line pb-5">
          <p className="vg-kicker">Play with friends</p>
          <h1 className="vg-page-title mt-2">Your crews</h1>
          <p className="mt-3 max-w-2xl font-medium leading-7 text-neutral-600">
            A crew is a private group playing the same daily grid. Make one, send the link, and the
            board shows how everyone did — result grids stay hidden until you have finished yours.
          </p>
        </header>

        <MyCrews />
      </div>
    </main>
  );
}
