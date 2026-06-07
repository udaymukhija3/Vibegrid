import Link from "next/link";
import { ArrowLeft } from "lucide-react";

export type PolicySection = {
  title: string;
  body: string[];
};

export function PolicyPage({
  eyebrow,
  title,
  intro,
  sections
}: {
  eyebrow: string;
  title: string;
  intro: string;
  sections: PolicySection[];
}) {
  return (
    <main className="min-h-screen px-4 py-6 sm:px-6 lg:px-8">
      <div className="mx-auto max-w-3xl">
        <Link
          href="/"
          className="inline-flex h-10 items-center gap-2 rounded border border-ink bg-white px-3 text-sm font-semibold shadow-[0_4px_0_#171717]"
        >
          <ArrowLeft aria-hidden size={16} />
          Today
        </Link>

        <header className="mt-8 border-b-4 border-ink pb-5">
          <p className="text-sm font-bold text-plum">{eyebrow}</p>
          <h1 className="mt-2 text-4xl font-black sm:text-5xl">{title}</h1>
          <p className="mt-3 max-w-2xl font-semibold text-neutral-600">{intro}</p>
        </header>

        <div className="mt-8 grid gap-7">
          {sections.map((section) => (
            <section key={section.title}>
              <h2 className="text-xl font-black">{section.title}</h2>
              <div className="mt-2 grid gap-2 text-sm font-semibold leading-6 text-neutral-700">
                {section.body.map((paragraph) => (
                  <p key={paragraph}>{paragraph}</p>
                ))}
              </div>
            </section>
          ))}
        </div>

        <footer className="mt-8 flex flex-wrap gap-3 border-t border-neutral-300 pt-4 text-sm font-black">
          <Link href="/policy" className="underline decoration-2 underline-offset-4">
            Community rules
          </Link>
          <Link href="/terms" className="underline decoration-2 underline-offset-4">
            Terms
          </Link>
          <Link href="/privacy" className="underline decoration-2 underline-offset-4">
            Privacy
          </Link>
        </footer>
      </div>
    </main>
  );
}
