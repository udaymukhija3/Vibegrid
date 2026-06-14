import { DemoLauncher } from "@/components/DemoLauncher";

export const metadata = {
  title: "VibeGrid Demo",
  description: "Start a seeded VibeGrid demo room with no sign-in."
};

export default function DemoPage() {
  return (
    <main className="min-h-screen px-4 py-5 sm:px-6 lg:px-8">
      <DemoLauncher />
    </main>
  );
}
