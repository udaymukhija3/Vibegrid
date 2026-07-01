import Link from "next/link";

export default function NotFound() {
  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <div className="vg-panel w-full max-w-md p-6 text-center">
        <p className="text-sm font-semibold text-plum">404</p>
        <h1 className="mt-2 text-3xl font-extrabold">No vibe here</h1>
        <p className="mt-2 font-medium text-neutral-600">
          This grid doesn&apos;t exist. The puzzle you&apos;re after may have moved or never been.
        </p>
        <Link
          href="/"
          className="vg-button-primary mt-5"
        >
          Play today&apos;s puzzle
        </Link>
      </div>
    </main>
  );
}
