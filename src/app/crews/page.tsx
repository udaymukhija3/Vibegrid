import { MyCrews } from "@/components/MyCrews";
import { VibeHeader } from "@/components/VibeHeader";

export const metadata = {
  title: "Your VibeGrid crews",
  description: "Make a private crew, compose a daily vibe, and judge yesterday's cards."
};

export default function CrewsPage() {
  return (
    <main className="min-h-screen">
      <div className="vg-shell">
        <VibeHeader compact />
        <header className="py-10 sm:py-14">
          <p className="vg-meta text-lime">The crew is the game</p>
          <h1 className="mt-3 max-w-4xl text-5xl font-black leading-[0.95] tracking-[-0.06em] text-cream sm:text-7xl">
            Make privately.
            <br />
            Judge honestly.
          </h1>
          <p className="mt-5 max-w-2xl text-base font-semibold leading-7 text-cream/[.62]">
            Each day starts with a shared palette sized to the crew. Every person makes a different card;
            tomorrow the names disappear and the crew chooses what lands.
          </p>
        </header>
        <MyCrews />
      </div>
    </main>
  );
}
