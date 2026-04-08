import Link from "next/link";

import { MarketingPage, PageHero } from "@/components/marketing/primitives";

export const metadata = {
  title: "About",
};

export default function AboutPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="About"
        title={
          <>
            Built for people
            <br />
            who make things.
          </>
        }
        lede="We&rsquo;re a small team building a commerce platform for independent merchants who want a store that looks considered, costs predictably, and stays out of their way."
      />

      <section className="border-t border-border-subtle py-20 sm:py-28">
        <div className="mx-auto grid max-w-6xl gap-12 px-6 lg:grid-cols-[1fr_2fr] lg:gap-16">
          <div>
            <p className="eyebrow mb-5">Why we&rsquo;re building this</p>
          </div>
          <div className="space-y-6 text-foreground-secondary leading-relaxed">
            <p>
              There are dozens of ways to open an online store. Most of them
              are designed for merchants with a developer, a design budget, or
              a long runway. If you don&rsquo;t have any of those, the
              experience is overwhelming, expensive, and slow.
            </p>
            <p>
              Mark8ly is the opposite. The storefront looks like something a
              studio made. The admin only shows what you need. Six months are
              free. After that the price is one honest number.
            </p>
            <p>
              We&rsquo;re not trying to be the biggest platform. We&rsquo;re
              trying to be the one that treats indie merchants like the adults
              they are.
            </p>
          </div>
        </div>
      </section>

      <section className="border-t border-border-subtle py-20 sm:py-28">
        <div className="mx-auto max-w-6xl px-6">
          <div className="mb-12 max-w-2xl">
            <p className="eyebrow mb-5">What we believe</p>
            <h2
              className="font-serif font-medium text-foreground"
              style={{
                fontSize: "var(--text-4xl)",
                lineHeight: 1.05,
                letterSpacing: "-0.02em",
              }}
            >
              Three commitments we won&rsquo;t compromise on.
            </h2>
          </div>

          <ol className="space-y-12">
            {beliefs.map((belief) => (
              <li
                key={belief.number}
                className="grid grid-cols-[auto_1fr] gap-8 border-t border-border-subtle pt-10"
              >
                <span className="font-serif text-3xl font-medium text-moss-700">
                  {belief.number}
                </span>
                <div>
                  <h3 className="font-serif text-2xl text-foreground">
                    {belief.title}
                  </h3>
                  <p className="mt-3 max-w-xl text-foreground-secondary leading-relaxed">
                    {belief.body}
                  </p>
                </div>
              </li>
            ))}
          </ol>
        </div>
      </section>

      <section className="border-t border-border-subtle py-20 sm:py-28">
        <div className="mx-auto grid max-w-6xl gap-12 px-6 lg:grid-cols-[1fr_2fr] lg:gap-16">
          <div>
            <p className="eyebrow mb-5">Where we&rsquo;re based</p>
          </div>
          <div className="space-y-6 text-foreground-secondary leading-relaxed lg:max-w-xl">
            <p>
              Mumbai, India. Remote-first, global by default. We build and
              support Mark8ly from a small distributed team of engineers,
              designers, and merchants.
            </p>
            <p>
              Questions, ideas, or just want to say hello?{" "}
              <Link
                href="/contact"
                className="text-foreground underline decoration-moss-700 decoration-2 underline-offset-4 hover:text-moss-700"
              >
                Get in touch
              </Link>
              .
            </p>
          </div>
        </div>
      </section>

      <section className="border-t border-border-subtle py-20 sm:py-24">
        <div className="mx-auto max-w-6xl px-6">
          <div className="flex flex-wrap items-center gap-x-8 gap-y-4">
            <Link
              href="/onboarding"
              className="inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
            >
              Open your store
            </Link>
            <Link href="/#pricing" className="btn-ghost">
              See the pricing
            </Link>
          </div>
        </div>
      </section>
    </MarketingPage>
  );
}

const beliefs = [
  {
    number: "i.",
    title: "Quiet by design.",
    body: "No popups, no upsells in the admin, no aggressive growth hacks. The storefront and the tools should feel like furniture, not a casino.",
  },
  {
    number: "ii.",
    title: "Yours, fully.",
    body: "No transaction fees from us. Your domain, your data, your customers. Export everything any time. Leave whenever you like — no friction, no hard feelings.",
  },
  {
    number: "iii.",
    title: "Made well.",
    body: "Real merchants shaped the design. Real engineers wrote the code. We build fewer things on purpose so each one works properly.",
  },
];
