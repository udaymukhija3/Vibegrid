import { CrewRoom } from "@/components/CrewRoom";

export const metadata = {
  title: "Your VibeGrid crew",
  description: "Make today's vibe, judge yesterday's cards, and reveal what your crew picked.",
  robots: { index: false, follow: false }
};

// Crew ids are minted at runtime, so — like demo rooms — the route stays open
// to params the build never saw. The static export emits one placeholder shell
// that serves every crew id, and the component reads the real id from the URL
// (see dynamicRoutes in the Go frontend handler).
export function generateStaticParams() {
  return [{ id: "__crew__" }];
}

export default function CrewPage() {
  return (
    <main className="min-h-screen">
      <CrewRoom />
    </main>
  );
}
