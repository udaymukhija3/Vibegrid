import { DemoWalkthrough } from "@/components/DemoWalkthrough";

export const metadata = {
  title: "VibeGrid Demo Room",
  description: "A seeded VibeGrid walkthrough room for public demos."
};

export function generateStaticParams() {
  return [{ room: "__room__" }];
}

export default function DemoRoomPage() {
  return (
    <main className="min-h-screen px-4 py-5 sm:px-6 lg:px-8">
      <DemoWalkthrough />
    </main>
  );
}
