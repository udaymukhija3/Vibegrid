import Link from "next/link";
import { Users } from "lucide-react";

export function VibeHeader({ compact = false }: { compact?: boolean }) {
  return (
    <header className="flex items-center justify-between gap-4">
      <Link href="/" className="font-display text-3xl font-black tracking-[-0.06em] text-cream sm:text-4xl">
        VibeGrid
      </Link>
      {!compact && (
        <Link href="/crews" className="vg-header-link">
          <Users aria-hidden size={17} />
          Crews
        </Link>
      )}
    </header>
  );
}
