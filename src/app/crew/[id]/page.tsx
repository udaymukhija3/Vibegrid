import { CrewRoom } from "@/components/CrewRoom";

export const metadata = {
  title: "Your VibeGrid crew",
  description: "Play today's grid alongside your friends and compare how everyone did."
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
    <main className="min-h-screen px-4 py-5 sm:px-6 lg:px-8">
      <CrewRoom />
    </main>
  );
}
