"use client";

import { Toaster } from "sonner";

export function ToastProvider() {
  return (
    <Toaster
      richColors
      position="top-center"
      toastOptions={{
        classNames: {
          toast: "border-2 border-ink font-semibold shadow-[0_5px_0_#171717]",
          title: "font-black",
          description: "font-semibold"
        }
      }}
    />
  );
}
