import { PlaySharedPuzzle } from "@/components/PlaySharedPuzzle";

export const dynamicParams = false;

export function generateStaticParams() {
  return [{ id: "__share__" }];
}

export default function SharedPuzzlePage() {
  return (
    <main className="min-h-screen px-4 py-5 sm:px-6 lg:px-8">
      <PlaySharedPuzzle />
    </main>
  );
}
