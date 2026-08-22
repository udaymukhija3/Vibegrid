import Link from "next/link";

export default function NotFound() {
  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <div className="vg-dark-panel w-full max-w-md text-center">
        <p className="vg-meta text-coral">404</p>
        <h1 className="mt-2 text-3xl font-black">No vibe here</h1>
        <p className="mt-2 font-semibold text-cream/[.65]">
          This room doesn&apos;t exist. Its invite may have been rotated, or the link may be incomplete.
        </p>
        <Link
          href="/"
          className="vg-primary-button mt-5"
        >
          Try today&apos;s practice
        </Link>
      </div>
    </main>
  );
}
