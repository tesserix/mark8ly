import type { ReactNode } from "react";
import Link from "next/link";

import { Header } from "./Header";
import { Footer } from "./Footer";

/* ============================================================
   MarketingPage — top-level shell for every marketing page.
   Header + <main> + Footer, with the semantic landmark wired
   up for the site-wide skip link.
   ============================================================ */

interface MarketingPageProps {
  children: ReactNode;
}

export function MarketingPage({ children }: MarketingPageProps) {
  return (
    <div className="bg-background text-foreground">
      <Header />
      <main id="main">{children}</main>
      <Footer />
    </div>
  );
}

/* ============================================================
   PageHero — canonical page-opening editorial moment.
   Left-aligned, asymmetric, serif title. Used on every
   marketing page except the home page (which has its own
   bespoke hero).
   ============================================================ */

interface PageHeroProps {
  eyebrow?: string;
  title: ReactNode;
  lede?: ReactNode;
}

export function PageHero({ eyebrow, title, lede }: PageHeroProps) {
  return (
    <section className="pb-16 pt-16 sm:pb-20 sm:pt-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="max-w-3xl">
          {eyebrow ? <p className="eyebrow mb-5">{eyebrow}</p> : null}
          <h1
            className="font-serif font-medium text-foreground"
            style={{
              fontSize: "var(--text-5xl)",
              lineHeight: 1.02,
              letterSpacing: "-0.025em",
            }}
          >
            {title}
          </h1>
          {lede ? (
            <p
              className="mt-6 max-w-xl text-foreground-secondary"
              style={{ fontSize: "var(--text-lg)", lineHeight: 1.55 }}
            >
              {lede}
            </p>
          ) : null}
        </div>
      </div>
    </section>
  );
}

/* ============================================================
   Prose — long-form text wrapper used by legal pages (privacy,
   terms, etc.). Provides correct type hierarchy without any
   card chrome. Just a constrained measure and serif headings.
   ============================================================ */

interface ProseProps {
  children: ReactNode;
}

export function Prose({ children }: ProseProps) {
  return (
    <section className="border-t border-border-subtle pb-24 pt-16">
      <div className="mx-auto max-w-3xl px-6">
        <article
          className="
            prose-editorial
            [&_h2]:mt-12 [&_h2]:mb-4 [&_h2]:font-serif [&_h2]:text-2xl [&_h2]:font-medium [&_h2]:text-foreground
            [&_h3]:mt-8 [&_h3]:mb-3 [&_h3]:text-lg [&_h3]:font-medium [&_h3]:text-foreground
            [&_p]:my-4 [&_p]:leading-relaxed [&_p]:text-foreground-secondary
            [&_ul]:my-4 [&_ul]:list-none [&_ul]:space-y-2 [&_ul]:pl-0
            [&_li]:text-foreground-secondary [&_li]:leading-relaxed [&_li]:pl-6 [&_li]:relative
            [&_li]:before:content-['·'] [&_li]:before:absolute [&_li]:before:left-0 [&_li]:before:text-moss-700 [&_li]:before:font-serif [&_li]:before:text-lg
            [&_strong]:font-medium [&_strong]:text-foreground
            [&_a]:text-foreground [&_a]:underline [&_a]:decoration-moss-700 [&_a]:decoration-2 [&_a]:underline-offset-4
            [&_a:hover]:text-moss-700
          "
        >
          {children}
        </article>
      </div>
    </section>
  );
}

/* ============================================================
   MarketingStub — quiet "in progress" page used for surfaces
   that exist as nav destinations but don't have real content
   yet (blog, help, guides, integrations, legal). Pure type, a
   single hairline, a CTA back to something real.
   ============================================================ */

interface MarketingStubProps {
  eyebrow: string;
  title: ReactNode;
  body: ReactNode;
  primaryCta?: {
    href: string;
    label: string;
  };
  secondaryCta?: {
    href: string;
    label: string;
  };
}

export function MarketingStub({
  eyebrow,
  title,
  body,
  primaryCta,
  secondaryCta,
}: MarketingStubProps) {
  return (
    <MarketingPage>
      <section className="pb-32 pt-20 sm:pb-40 sm:pt-28">
        <div className="mx-auto max-w-3xl px-6">
          <p className="eyebrow mb-6">{eyebrow}</p>
          <h1
            className="font-serif font-medium text-foreground"
            style={{
              fontSize: "var(--text-4xl)",
              lineHeight: 1.05,
              letterSpacing: "-0.02em",
            }}
          >
            {title}
          </h1>
          <p
            className="mt-6 max-w-xl text-foreground-secondary"
            style={{ fontSize: "var(--text-lg)", lineHeight: 1.55 }}
          >
            {body}
          </p>

          {(primaryCta || secondaryCta) && (
            <div className="mt-10 flex flex-wrap items-center gap-x-8 gap-y-4">
              {primaryCta && (
                <Link
                  href={primaryCta.href}
                  className="inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
                >
                  {primaryCta.label}
                </Link>
              )}
              {secondaryCta && (
                <Link href={secondaryCta.href} className="btn-ghost">
                  {secondaryCta.label}
                </Link>
              )}
            </div>
          )}
        </div>
      </section>
    </MarketingPage>
  );
}
