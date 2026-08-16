import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { CreatorClaim } from "@/components/CreatorClaim";

export const metadata = {
  title: "Creator claim · VibeGrid",
  description: "Check, withdraw, or appeal a community VibeGrid submission."
};

export default function CreatorClaimPage() {
  return (
    <main className="min-h-screen px-4 py-6 sm:px-6 lg:px-8">
      <div className="mx-auto max-w-3xl">
        <Link href="/" className="vg-button-secondary">
          <ArrowLeft aria-hidden size={16} />
          Today
        </Link>
        <CreatorClaim />
      </div>
    </main>
  );
}
