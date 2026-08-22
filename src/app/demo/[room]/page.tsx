import { VibeGridApp } from "@/components/VibeGridApp";

export const metadata = {
  title: "VibeGrid Practice",
  description: "Try the complete VibeGrid make, judge, and reveal rhythm with no sign-in.",
  robots: { index: false, follow: false }
};

export function generateStaticParams() {
  return [{ room: "__room__" }];
}

export default function DemoRoomPage() {
  return (
    <main className="min-h-screen">
      <VibeGridApp />
    </main>
  );
}
