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
    <main className="vg-shell">
      <div className="mx-auto max-w-4xl">
        <Link
          href="/"
          className="vg-header-link"
        >
          <ArrowLeft aria-hidden size={16} />
          Today
        </Link>

        <header className="mt-10 border-b border-cream/[.15] pb-8">
          <p className="vg-meta text-lime">{eyebrow}</p>
          <h1 className="mt-3 text-5xl font-black tracking-[-0.06em] text-cream sm:text-7xl">{title}</h1>
          <p className="mt-5 max-w-2xl font-semibold leading-7 text-cream/[.65]">{intro}</p>
        </header>

        <div className="mt-8 grid gap-7">
          {sections.map((section) => (
            <section key={section.title} className="vg-dark-panel">
              <h2 className="text-xl font-black text-cream">{section.title}</h2>
              <div className="mt-3 grid gap-3 text-sm font-semibold leading-6 text-cream/[.65]">
                {section.body.map((paragraph) => (
                  <p key={paragraph}>{paragraph}</p>
                ))}
              </div>
            </section>
          ))}
        </div>

        <footer className="mt-8 flex flex-wrap gap-5 border-t border-cream/[.15] pt-5 font-mono text-xs font-bold uppercase tracking-[0.08em] text-cream/[.55]">
          <Link href="/policy" className="hover:text-lime">
            Crew rules
          </Link>
          <Link href="/terms" className="hover:text-lime">
            Terms
          </Link>
          <Link href="/privacy" className="hover:text-lime">
            Privacy
          </Link>
        </footer>
      </div>
    </main>
  );
}
