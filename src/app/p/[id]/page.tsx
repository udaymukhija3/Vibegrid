import { PlaySharedPuzzle } from "@/components/PlaySharedPuzzle";

export default async function SharedPuzzlePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  return (
    <main className="min-h-screen px-4 py-5 sm:px-6 lg:px-8">
      <PlaySharedPuzzle puzzleId={id} />
    </main>
  );
}
