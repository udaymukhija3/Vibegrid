import { VibeGridApp } from "@/components/VibeGridApp";

export const metadata = {
  title: "VibeGrid Demo",
  description: "Try the complete VibeGrid make, judge, and reveal rhythm with no sign-in.",
  robots: { index: false, follow: false }
};

export default function DemoPage() {
  return (
    <main className="min-h-screen">
      <VibeGridApp />
    </main>
  );
}
