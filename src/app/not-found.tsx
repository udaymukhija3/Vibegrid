import Link from "next/link";

export default function NotFound() {
  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-md rounded border-2 border-ink bg-white p-6 text-center shadow-[0_6px_0_#171717]">
        <p className="text-sm font-black text-plum">404</p>
        <h1 className="mt-2 text-3xl font-black">No vibe here</h1>
        <p className="mt-2 font-semibold text-neutral-600">
          This grid doesn&apos;t exist. The puzzle you&apos;re after may have moved or never been.
        </p>
        <Link
          href="/"
          className="mt-5 inline-flex h-11 items-center justify-center rounded border-2 border-ink bg-mint px-5 font-black shadow-[0_4px_0_#171717]"
        >
          Play today&apos;s puzzle
        </Link>
      </div>
    </main>
  );
}
