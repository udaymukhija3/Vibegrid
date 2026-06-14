"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Sparkles } from "lucide-react";

export function DemoLauncher() {
  const router = useRouter();

  useEffect(() => {
    router.replace(`/demo/${newDemoRoomSlug()}`);
  }, [router]);

  return (
    <div className="mx-auto flex min-h-[calc(100vh-2.5rem)] max-w-3xl items-center justify-center">
      <div className="w-full rounded border-2 border-ink bg-white p-5 text-center shadow-[0_6px_0_#171717]">
        <Sparkles aria-hidden className="mx-auto" size={28} />
        <h1 className="mt-3 text-3xl font-black">Starting demo</h1>
        <p className="mt-3 font-semibold text-neutral-600">
          Setting up a fresh seeded room.
        </p>
      </div>
    </div>
  );
}

function newDemoRoomSlug() {
  const webCrypto = globalThis.crypto;

  if (webCrypto?.randomUUID) {
    return `room-${webCrypto.randomUUID().replaceAll("-", "").slice(0, 10)}`;
  }

  if (webCrypto?.getRandomValues) {
    const bytes = new Uint32Array(2);
    webCrypto.getRandomValues(bytes);
    return `room-${Array.from(bytes, (value) => value.toString(36)).join("").slice(0, 10)}`;
  }

  return `room-${Date.now().toString(36)}`;
}
