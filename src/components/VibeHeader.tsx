import Link from "next/link";
import { Users } from "lucide-react";
import type { ReactNode } from "react";

export function VibeHeader({ compact = false, actions }: { compact?: boolean; actions?: ReactNode }) {
  return (
    <header className="flex items-center justify-between gap-4">
      <Link href="/" className="font-display text-3xl font-black tracking-[-0.06em] text-cream sm:text-4xl">
        VibeGrid
      </Link>
      <div className="flex items-center gap-2">
        {actions}
        {!compact && (
          <Link href="/crews" className="vg-header-link">
            <Users aria-hidden size={17} />
            Crews
          </Link>
        )}
      </div>
    </header>
  );
}
