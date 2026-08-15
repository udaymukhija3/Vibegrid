import type { Metadata } from "next";
import type { ReactNode } from "react";
import { ClientErrorReporter } from "@/components/ClientErrorReporter";
import { ToastProvider } from "@/components/ToastProvider";
import "./globals.css";

// The build inlines this, and NEXT_PUBLIC_APP_URL is not passed as a Docker build
// arg, so the fallback is what production actually ships. Defaulting to localhost
// made every shared link resolve og:image to http://localhost:3000 — dead preview.
const APP_URL = process.env.NEXT_PUBLIC_APP_URL ?? "https://vibegrid.onrender.com";

export const metadata: Metadata = {
  metadataBase: new URL(APP_URL),
  title: "VibeGrid",
  description: "Group the words. Guess the vibe. Try not to overthink it.",
  openGraph: {
    type: "website",
    url: "/",
    siteName: "VibeGrid",
    title: "VibeGrid",
    description: "A daily semantic grouping puzzle.",
    // Must be a raster format: iMessage, WhatsApp, Slack and Twitter all skip
    // SVG og:images, so the previous /vibegrid-mark.svg never rendered.
    images: [{ url: "/og.png", width: 1200, height: 630, alt: "VibeGrid — a daily semantic grouping puzzle" }]
  },
  twitter: {
    card: "summary_large_image",
    title: "VibeGrid",
    description: "A daily semantic grouping puzzle.",
    images: ["/og.png"]
  }
};

export default function RootLayout({
  children
}: Readonly<{
  children: ReactNode;
}>) {
  return (
    <html lang="en">
      <body>
        {children}
        <ToastProvider />
        <ClientErrorReporter />
      </body>
    </html>
  );
}
